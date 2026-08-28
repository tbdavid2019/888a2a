// Package testkit provides deterministic A2A and Agent runtime test doubles.
package testkit

import (
	"context"
	"sync"

	a2a "github.com/tbdavid2019/888a2a/backend/a2a"
)

// FakeBridge is a deterministic bridge double for ACP, Gateway, and CLI
// contract tests. It never starts a process or opens a socket.
type FakeBridge struct {
	BridgeID       string
	Mode           string
	Result         a2a.BridgeResult
	PreflightErr   error
	StartErr       error
	WaitForRelease bool

	mu          sync.Mutex
	starts      int
	executed    map[string]a2a.BridgeResult
	release     chan struct{}
	releaseOnce sync.Once
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
	b.mu.Lock()
	b.starts++
	if b.release == nil {
		b.release = make(chan struct{})
	}
	b.mu.Unlock()
	return &fakeBridgeSession{
		bridge:  b,
		result:  b.Result,
		cancel:  make(chan struct{}),
		release: b.release,
	}, nil
}

func (b *FakeBridge) Health(context.Context) (a2a.BridgeHealth, error) {
	return a2a.BridgeHealth{Ready: b.PreflightErr == nil, Detail: b.Mode + " fake bridge"}, nil
}

type fakeBridgeSession struct {
	bridge  *FakeBridge
	result  a2a.BridgeResult
	cancel  chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *fakeBridgeSession) Invoke(ctx context.Context, request a2a.BridgeRequest, emit func(a2a.BridgeEvent) error) (a2a.BridgeResult, error) {
	s.bridge.mu.Lock()
	if s.bridge.executed == nil {
		s.bridge.executed = make(map[string]a2a.BridgeResult)
	}
	if previous, ok := s.bridge.executed[request.TaskID]; ok {
		s.bridge.mu.Unlock()
		return previous, nil
	}
	s.bridge.mu.Unlock()

	if s.bridge.WaitForRelease {
		select {
		case <-ctx.Done():
			return a2a.BridgeResult{Outcome: a2a.DeliveryOutcomeUnknown, Reason: "fake context canceled"}, ctx.Err()
		case <-s.cancel:
			return a2a.BridgeResult{Outcome: a2a.DeliveryOutcomeUnknown, Reason: "fake session canceled"}, context.Canceled
		case <-s.release:
		}
	}
	select {
	case <-ctx.Done():
		return a2a.BridgeResult{Outcome: a2a.DeliveryOutcomeUnknown, Reason: "fake context canceled"}, ctx.Err()
	case <-s.cancel:
		return a2a.BridgeResult{Outcome: a2a.DeliveryOutcomeUnknown, Reason: "fake session canceled"}, context.Canceled
	default:
	}
	for _, event := range s.result.Events {
		select {
		case <-ctx.Done():
			return a2a.BridgeResult{Outcome: a2a.DeliveryOutcomeUnknown, Reason: "fake context canceled"}, ctx.Err()
		case <-s.cancel:
			return a2a.BridgeResult{Outcome: a2a.DeliveryOutcomeUnknown, Reason: "fake session canceled"}, context.Canceled
		default:
		}
		if emit != nil {
			if err := emit(event); err != nil {
				return a2a.BridgeResult{Outcome: a2a.DeliveryOutcomeUnknown, Reason: "fake event consumer failed"}, err
			}
		}
	}
	s.bridge.mu.Lock()
	s.bridge.executed[request.TaskID] = s.result
	s.bridge.mu.Unlock()
	return s.result, nil
}

func (s *fakeBridgeSession) Cancel(context.Context) error {
	s.once.Do(func() { close(s.cancel) })
	return nil
}
func (s *fakeBridgeSession) Stop(ctx context.Context) error { return s.Cancel(ctx) }

// ReleaseFakeBridge unblocks sessions configured with WaitForRelease. Tests
// should call it exactly once per blocked invocation.
func (b *FakeBridge) Release() {
	b.mu.Lock()
	if b.release == nil {
		b.release = make(chan struct{})
	}
	release := b.release
	b.mu.Unlock()
	b.releaseOnce.Do(func() { close(release) })
}

// Starts reports how many sessions this fake bridge created.
func (b *FakeBridge) Starts() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.starts
}
