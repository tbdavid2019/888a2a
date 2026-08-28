// Package testkit provides deterministic A2A and Agent runtime test doubles.
package testkit

import (
	"context"

	a2a "github.com/tbdavid2019/888a2a/backend/a2a"
)

// FakeBridge is a deterministic bridge double for ACP, Gateway, and CLI
// contract tests. It never starts a process or opens a socket.
type FakeBridge struct {
	BridgeID     string
	Mode         string
	Result       a2a.BridgeResult
	PreflightErr error
	StartErr     error
}

func NewFakeACPBridge(result a2a.BridgeResult) *FakeBridge {
	return &FakeBridge{BridgeID: "fake-acp", Mode: "ACP", Result: result}
}

func NewFakeGatewayBridge(result a2a.BridgeResult) *FakeBridge {
	return &FakeBridge{BridgeID: "fake-gateway", Mode: "GATEWAY", Result: result}
}

func NewFakeCLIBridge(result a2a.BridgeResult) *FakeBridge {
	return &FakeBridge{BridgeID: "fake-cli", Mode: "CLI", Result: result}
}

func (b *FakeBridge) ID() string { return b.BridgeID }

func (b *FakeBridge) Preflight(context.Context, a2a.BridgeRequest) error { return b.PreflightErr }

func (b *FakeBridge) Start(context.Context, a2a.BridgeRequest) (a2a.BridgeSession, error) {
	if b.StartErr != nil {
		return nil, b.StartErr
	}
	return &fakeBridgeSession{result: b.Result}, nil
}

func (b *FakeBridge) Health(context.Context) (a2a.BridgeHealth, error) {
	return a2a.BridgeHealth{Ready: b.PreflightErr == nil, Detail: b.Mode + " fake bridge"}, nil
}

type fakeBridgeSession struct{ result a2a.BridgeResult }

func (s *fakeBridgeSession) Invoke(_ context.Context, _ a2a.BridgeRequest, emit func(a2a.BridgeEvent) error) (a2a.BridgeResult, error) {
	for _, event := range s.result.Events {
		if emit != nil {
			if err := emit(event); err != nil {
				return a2a.BridgeResult{Outcome: a2a.DeliveryOutcomeUnknown, Reason: "fake event consumer failed"}, err
			}
		}
	}
	return s.result, nil
}

func (*fakeBridgeSession) Cancel(context.Context) error { return nil }
func (*fakeBridgeSession) Stop(context.Context) error   { return nil }
