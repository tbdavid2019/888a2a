// Package bridge contains provider-specific adapters for A2A delivery.
package bridge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	a2apkg "github.com/tbdavid2019/888a2a/backend/a2a"
	"github.com/tbdavid2019/888a2a/backend/agent/executor"
	"github.com/tbdavid2019/888a2a/backend/agent/provider"
)

// CodexACPBridgeConfig describes an explicit Codex ACP v2 bridge. Runtime
// credentials remain in the process environment and are never copied into a
// bridge request or binding record.
type CodexACPBridgeConfig struct {
	ID         string
	WorkingDir string
	Model      string
}

// CodexACPBridge adapts the existing ACP v2 ThreadExecutor to AgentBridge.
type CodexACPBridge struct {
	config CodexACPBridgeConfig
}

// NewCodexACPBridge creates a Codex ACP v2 bridge with an absolute workspace.
func NewCodexACPBridge(config CodexACPBridgeConfig) (*CodexACPBridge, error) {
	if config.ID == "" {
		config.ID = "codex-acp2"
	}
	if config.WorkingDir == "" || !filepath.IsAbs(config.WorkingDir) {
		return nil, errors.New("Codex ACP bridge working directory must be absolute")
	}
	return &CodexACPBridge{config: config}, nil
}

func (b *CodexACPBridge) ID() string { return b.config.ID }

func (b *CodexACPBridge) Preflight(ctx context.Context, request a2apkg.BridgeRequest) error {
	if err := a2apkg.ValidateBridgeRequest(request, b.ID()); err != nil {
		return err
	}
	if _, err := exec.LookPath("codex"); err != nil {
		return fmt.Errorf("resolve codex executable: %w", err)
	}
	info, err := os.Stat(b.config.WorkingDir)
	if err != nil {
		return fmt.Errorf("stat Codex ACP working directory: %w", err)
	}
	if !info.IsDir() {
		return errors.New("Codex ACP working directory is not a directory")
	}
	return nil
}

func (b *CodexACPBridge) Start(context.Context, a2apkg.BridgeRequest) (a2apkg.BridgeSession, error) {
	return &codexSession{config: b.config}, nil
}

func (b *CodexACPBridge) Health(context.Context) (a2apkg.BridgeHealth, error) {
	if _, err := exec.LookPath("codex"); err != nil {
		return a2apkg.BridgeHealth{Detail: err.Error()}, nil
	}
	return a2apkg.BridgeHealth{Ready: true, Detail: "Codex ACP v2 executable resolved"}, nil
}

type codexSession struct {
	config  CodexACPBridgeConfig
	runtime executor.Runtime
}

func (s *codexSession) Invoke(ctx context.Context, request a2apkg.BridgeRequest, emit func(a2apkg.BridgeEvent) error) (a2apkg.BridgeResult, error) {
	timeoutSeconds := int32(request.Timeout.Seconds())
	if timeoutSeconds <= 0 {
		timeoutSeconds = 1
	}
	cfg := &executor.ThreadConfig{
		Limits: executor.Limits{
			MaxTimeoutSeconds: timeoutSeconds,
			MaxEventCount:     executor.DefaultMaxEventCount,
			MaxOutputBytes:    int64(request.MaxOutputBytes),
			OutputFlushBytes:  executor.DefaultOutputFlushBytes,
			StartupTimeout:    60 * time.Second,
		},
		Provider:   "codex",
		Protocol:   executor.ProtocolV2,
		Model:      s.config.Model,
		WorkingDir: s.config.WorkingDir,
		AllowEnv:   []string{"PATH", "HOME", "CODEX_HOME", "A2A888_HOME"},
		CustomEnv: map[string]string{
			"A2A888_ORGANIZATION_ID":    request.OrganizationID,
			"A2A888_A2A_TASK_ID":        request.TaskID,
			"A2A888_A2A_CONTEXT_ID":     request.ContextID,
			"A2A888_A2A_CORRELATION_ID": request.CorrelationID,
		},
	}
	runtime, err := executor.NewThread(executor.Request{
		CommandID:       request.TaskID,
		AgentID:         request.TaskID,
		AgentResourceID: request.TaskID,
		Profile:         "codex",
		WorkingDir:      s.config.WorkingDir,
		TimeoutSeconds:  timeoutSeconds,
		TurnPrompt:      request.Input,
	}, cfg, &provider.CodexProvider{})
	if err != nil {
		return a2apkg.BridgeResult{Outcome: a2apkg.DeliveryOutcomeRejected, Reason: "Codex ACP v2 runtime rejected"}, err
	}
	s.runtime = runtime
	runtime.Start()
	outputCh := runtime.OutputChannel()
	eventCh := runtime.EventChannel()
	resultCh := runtime.ResultChannel()
	sequence := uint64(0)
	var final executor.Result
	for {
		select {
		case <-ctx.Done():
			runtime.Cancel()
			<-runtime.Done()
			return a2apkg.BridgeResult{Outcome: a2apkg.DeliveryOutcomeUnknown, Reason: "Codex ACP v2 context canceled"}, ctx.Err()
		case output, ok := <-outputCh:
			if !ok {
				outputCh = nil
				continue
			}
			if output.Content == "" {
				continue
			}
			sequence++
			if err := emitCodexEvent(emit, a2apkg.BridgeEvent{Sequence: sequence, Kind: "output", Text: output.Content}); err != nil {
				runtime.Cancel()
				return a2apkg.BridgeResult{Outcome: a2apkg.DeliveryOutcomeUnknown, Reason: "Codex ACP v2 event delivery failed"}, err
			}
		case event, ok := <-eventCh:
			if !ok {
				eventCh = nil
				continue
			}
			text := strings.TrimSpace(event.Text)
			if text == "" {
				text = strings.TrimSpace(event.Summary)
			}
			if text == "" {
				continue
			}
			sequence++
			if err := emitCodexEvent(emit, a2apkg.BridgeEvent{Sequence: sequence, Kind: "event", Text: text}); err != nil {
				runtime.Cancel()
				return a2apkg.BridgeResult{Outcome: a2apkg.DeliveryOutcomeUnknown, Reason: "Codex ACP v2 event delivery failed"}, err
			}
		case result, ok := <-resultCh:
			if !ok {
				return a2apkg.BridgeResult{Outcome: a2apkg.DeliveryOutcomeUnknown, Reason: "Codex ACP v2 returned no result"}, errors.New("Codex ACP v2 returned no result")
			}
			final = result
			<-runtime.Done()
			if final.ErrorMessage != "" {
				return a2apkg.BridgeResult{Outcome: a2apkg.DeliveryOutcomeUnknown, Output: final.FinalSummary, Reason: final.ErrorMessage}, errors.New(final.ErrorMessage)
			}
			return a2apkg.BridgeResult{Outcome: a2apkg.DeliveryOutcomeDelivered, Output: final.FinalSummary}, nil
		}
	}
}

func emitCodexEvent(emit func(a2apkg.BridgeEvent) error, event a2apkg.BridgeEvent) error {
	if emit == nil {
		return nil
	}
	return emit(event)
}

func (s *codexSession) Cancel(context.Context) error {
	if s.runtime != nil {
		s.runtime.Cancel()
	}
	return nil
}

func (s *codexSession) Stop(ctx context.Context) error { return s.Cancel(ctx) }
