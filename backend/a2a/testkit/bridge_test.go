package testkit

import (
	"context"
	"errors"
	"testing"
	"time"

	a2a "github.com/tbdavid2019/888a2a/backend/a2a"
)

func TestFakeBridgeModesShareDeterministicContract(t *testing.T) {
	result := a2a.BridgeResult{
		Outcome: a2a.DeliveryOutcomeDelivered,
		Output:  "fake result",
		Events: []a2a.BridgeEvent{
			{Sequence: 1, Kind: "working"},
			{Sequence: 2, Kind: "completed", Terminal: true},
		},
	}
	for _, bridge := range []*FakeBridge{
		NewFakeACPBridge(result), NewFakeGatewayBridge(result), NewFakeCLIBridge(result),
	} {
		t.Run(bridge.Mode, func(t *testing.T) {
			request := a2a.BridgeRequest{
				OrganizationID: "org-a", CallerID: "caller-a", TaskID: "task-a", ContextID: "ctx-a",
				CorrelationID: "corr-a", BridgeID: bridge.ID(), Input: "hello", MaxOutputBytes: 1024, Timeout: time.Second,
			}
			var count int
			got, err := a2a.ExecuteBridge(context.Background(), bridge, request, func(a2a.BridgeEvent) error { count++; return nil })
			if err != nil || got.Outcome != a2a.DeliveryOutcomeDelivered || count != 2 {
				t.Fatalf("mode=%s result=%+v count=%d err=%v", bridge.Mode, got, count, err)
			}
		})
	}
}

func TestFakeBridgePreservesDeliveryOutcomes(t *testing.T) {
	for _, outcome := range []a2a.DeliveryOutcome{
		a2a.DeliveryOutcomeDelivered,
		a2a.DeliveryOutcomeRejected,
		a2a.DeliveryOutcomeNotDelivered,
		a2a.DeliveryOutcomeUnknown,
	} {
		t.Run(string(outcome), func(t *testing.T) {
			bridge := NewFakeACPBridge(a2a.BridgeResult{Outcome: outcome, Reason: string(outcome)})
			request := fakeRequest(bridge.ID(), "task-"+string(outcome))
			got, err := a2a.ExecuteBridge(context.Background(), bridge, request, nil)
			if err != nil {
				t.Fatalf("ExecuteBridge: %v", err)
			}
			if got.Outcome != outcome {
				t.Fatalf("outcome = %q, want %q", got.Outcome, outcome)
			}
		})
	}
}

func TestFakeBridgeDeduplicatesSameTask(t *testing.T) {
	bridge := NewFakeACPBridge(a2a.BridgeResult{
		Outcome: a2a.DeliveryOutcomeDelivered,
		Output:  "one execution",
		Events:  []a2a.BridgeEvent{{Sequence: 1, Kind: "completed", Terminal: true}},
	})
	request := fakeRequest(bridge.ID(), "same-task")
	var firstEvents, secondEvents int
	first, err := a2a.ExecuteBridge(context.Background(), bridge, request, func(a2a.BridgeEvent) error { firstEvents++; return nil })
	if err != nil {
		t.Fatalf("first ExecuteBridge: %v", err)
	}
	second, err := a2a.ExecuteBridge(context.Background(), bridge, request, func(a2a.BridgeEvent) error { secondEvents++; return nil })
	if err != nil {
		t.Fatalf("second ExecuteBridge: %v", err)
	}
	if first.Output != second.Output || firstEvents != 1 || secondEvents != 0 {
		t.Fatalf("duplicate execution changed result/events: first=%+v second=%+v events=%d/%d", first, second, firstEvents, secondEvents)
	}
	if bridge.Starts() != 2 {
		t.Fatalf("starts = %d, want two independently cleaned-up sessions", bridge.Starts())
	}
}

func TestFakeBridgeCancellationStopsBeforeLateEvents(t *testing.T) {
	bridge := NewFakeACPBridge(a2a.BridgeResult{
		Outcome: a2a.DeliveryOutcomeDelivered,
		Events:  []a2a.BridgeEvent{{Sequence: 1, Kind: "late", Terminal: true}},
	})
	bridge.WaitForRelease = true
	request := fakeRequest(bridge.ID(), "cancel-task")
	session, err := bridge.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	resultCh := make(chan a2a.BridgeResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, invokeErr := session.Invoke(context.Background(), request, func(a2a.BridgeEvent) error {
			t.Errorf("late event emitted after cancellation")
			return nil
		})
		resultCh <- result
		errCh <- invokeErr
	}()
	if err := session.Cancel(context.Background()); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	select {
	case result := <-resultCh:
		if result.Outcome != a2a.DeliveryOutcomeUnknown {
			t.Fatalf("outcome = %q, want unknown", result.Outcome)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled fake session did not stop")
	}
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("invoke error = %v, want context.Canceled", err)
	}
}

func fakeRequest(bridgeID, taskID string) a2a.BridgeRequest {
	return a2a.BridgeRequest{
		OrganizationID: "org-a", CallerID: "caller-a", TaskID: taskID, ContextID: "ctx-" + taskID,
		CorrelationID: "corr-" + taskID, BridgeID: bridgeID, Input: "hello", MaxOutputBytes: 1024, Timeout: time.Second,
	}
}
