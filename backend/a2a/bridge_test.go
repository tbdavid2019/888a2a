package a2a

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeBridgeSession struct {
	result     BridgeResult
	invokeErr  error
	cancelled  bool
	stopped    bool
	release    <-chan struct{}
	cancelCh   chan struct{}
	cancelOnce sync.Once
}

func (s *fakeBridgeSession) Invoke(ctx context.Context, _ BridgeRequest, emit func(BridgeEvent) error) (BridgeResult, error) {
	if s.release != nil {
		select {
		case <-s.release:
		case <-s.cancelCh:
			return BridgeResult{Outcome: DeliveryOutcomeUnknown, Reason: "fake session canceled"}, context.Canceled
		case <-ctx.Done():
			return BridgeResult{Outcome: DeliveryOutcomeUnknown, Reason: "fake context canceled"}, ctx.Err()
		}
	}
	for _, event := range s.result.Events {
		if err := emit(event); err != nil {
			return BridgeResult{Outcome: DeliveryOutcomeUnknown, Reason: "stream consumer failed"}, err
		}
	}
	return s.result, s.invokeErr
}

func (s *fakeBridgeSession) Cancel(context.Context) error {
	s.cancelled = true
	if s.cancelCh != nil {
		s.cancelOnce.Do(func() { close(s.cancelCh) })
	}
	return nil
}

func (s *fakeBridgeSession) Stop(context.Context) error {
	s.stopped = true
	return nil
}

type fakeBridge struct {
	preflightErr   error
	startErr       error
	session        *fakeBridgeSession
	waitForRelease bool
	release        chan struct{}
	releaseOnce    sync.Once
	mu             sync.Mutex
}

func (*fakeBridge) ID() string                                       { return "fake" }
func (f *fakeBridge) Preflight(context.Context, BridgeRequest) error { return f.preflightErr }
func (f *fakeBridge) Start(context.Context, BridgeRequest) (BridgeSession, error) {
	if f.startErr != nil {
		return nil, f.startErr
	}
	if f.waitForRelease {
		f.mu.Lock()
		if f.release == nil {
			f.release = make(chan struct{})
		}
		release := f.release
		f.mu.Unlock()
		return &fakeBridgeSession{result: f.session.result, release: release, cancelCh: make(chan struct{})}, nil
	}
	return f.session, nil
}
func (*fakeBridge) Health(context.Context) (BridgeHealth, error) {
	return BridgeHealth{Ready: true}, nil
}

func (f *fakeBridge) Release() {
	f.mu.Lock()
	if f.release == nil {
		f.release = make(chan struct{})
	}
	release := f.release
	f.mu.Unlock()
	f.releaseOnce.Do(func() { close(release) })
}

func validBridgeRequest() BridgeRequest {
	return BridgeRequest{
		OrganizationID: "org-a", CallerID: "agent-a", TaskID: "task-a", ContextID: "ctx-a",
		CorrelationID: "corr-a", BridgeID: "fake", Input: "hello", MaxOutputBytes: 1024,
		Timeout: 5 * time.Second,
	}
}

func TestExecuteBridgeStreamsAndStopsSession(t *testing.T) {
	session := &fakeBridgeSession{result: BridgeResult{
		Outcome: DeliveryOutcomeDelivered,
		Output:  "done",
		Events:  []BridgeEvent{{Sequence: 1, Kind: "working"}, {Sequence: 2, Kind: "completed", Terminal: true}},
	}}
	bridge := &fakeBridge{session: session}
	var got []BridgeEvent
	result, err := ExecuteBridge(context.Background(), bridge, validBridgeRequest(), func(event BridgeEvent) error {
		got = append(got, event)
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteBridge: %v", err)
	}
	if result.Outcome != DeliveryOutcomeDelivered || len(got) != 2 {
		t.Fatalf("result=%+v events=%+v", result, got)
	}
	if !session.stopped {
		t.Fatal("bridge session must be stopped after execution")
	}
}

func TestExecuteBridgeRejectsInvalidIdentityBeforeStart(t *testing.T) {
	started := false
	bridge := &fakeBridge{session: &fakeBridgeSession{}, startErr: errors.New("must not start")}
	request := validBridgeRequest()
	request.OrganizationID = ""
	_, err := ExecuteBridge(context.Background(), bridge, request, func(BridgeEvent) error {
		started = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "organization_id") || started {
		t.Fatalf("invalid identity result: err=%v started=%v", err, started)
	}
}

func TestExecuteBridgeDoesNotHideUnknownOutcome(t *testing.T) {
	session := &fakeBridgeSession{result: BridgeResult{Outcome: DeliveryOutcomeUnknown, Reason: "connection lost"}}
	result, err := ExecuteBridge(context.Background(), &fakeBridge{session: session}, validBridgeRequest(), nil)
	if err != nil {
		t.Fatalf("ExecuteBridge: %v", err)
	}
	if result.Outcome != DeliveryOutcomeUnknown || result.Reason != "connection lost" {
		t.Fatalf("unknown outcome was changed: %+v", result)
	}
}

func TestValidateBridgeResultRejectsInvalidEventSequenceAndOutput(t *testing.T) {
	result := BridgeResult{
		Outcome: DeliveryOutcomeDelivered,
		Output:  strings.Repeat("x", MaxBridgeOutputBytes+1),
		Events:  []BridgeEvent{{Sequence: 2}, {Sequence: 1}},
	}
	if err := ValidateBridgeResult(result); err == nil {
		t.Fatal("expected invalid bridge result error")
	}
}
