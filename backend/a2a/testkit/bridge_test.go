package testkit

import (
	"context"
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
