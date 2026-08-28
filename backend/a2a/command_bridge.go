package a2a

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// CommandBridgeConfig describes a non-interactive CLI boundary. The caller
// supplies an explicit environment and working directory; the bridge never
// sources shell startup files or guesses a user profile.
type CommandBridgeConfig struct {
	ID            string
	Executable    string
	WorkingDir    string
	Args          func(BridgeRequest) []string
	Environment   []string
	InputViaStdin bool
}

// CommandBridge is a bounded CLI implementation of AgentBridge.
type CommandBridge struct {
	config CommandBridgeConfig
}

// NewCommandBridge validates a CLI bridge configuration without starting it.
func NewCommandBridge(config CommandBridgeConfig) (*CommandBridge, error) {
	if config.ID == "" || config.Executable == "" || config.WorkingDir == "" {
		return nil, errors.New("command bridge requires id, executable, and working directory")
	}
	if !filepath.IsAbs(config.WorkingDir) {
		return nil, errors.New("command bridge working directory must be absolute")
	}
	return &CommandBridge{config: config}, nil
}

func (b *CommandBridge) ID() string { return b.config.ID }

func (b *CommandBridge) Preflight(_ context.Context, request BridgeRequest) error {
	if err := ValidateBridgeRequest(request, b.ID()); err != nil {
		return err
	}
	if _, err := exec.LookPath(b.config.Executable); err != nil {
		return fmt.Errorf("resolve bridge executable %q: %w", b.config.Executable, err)
	}
	info, err := os.Stat(b.config.WorkingDir)
	if err != nil {
		return fmt.Errorf("stat bridge working directory: %w", err)
	}
	if !info.IsDir() {
		return errors.New("command bridge working directory is not a directory")
	}
	return nil
}

func (b *CommandBridge) Start(_ context.Context, _ BridgeRequest) (BridgeSession, error) {
	return &commandBridgeSession{config: b.config}, nil
}

func (b *CommandBridge) Health(_ context.Context) (BridgeHealth, error) {
	if _, err := exec.LookPath(b.config.Executable); err != nil {
		return BridgeHealth{Detail: err.Error()}, nil
	}
	return BridgeHealth{Ready: true, Detail: "CLI executable resolved"}, nil
}

type commandBridgeSession struct {
	config CommandBridgeConfig

	mu  sync.Mutex
	cmd *exec.Cmd
}

func (s *commandBridgeSession) Invoke(ctx context.Context, request BridgeRequest, emit func(BridgeEvent) error) (BridgeResult, error) {
	args := []string(nil)
	if s.config.Args != nil {
		args = s.config.Args(request)
	}
	cmd := exec.Command(s.config.Executable, args...)
	cmd.Dir = s.config.WorkingDir
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH")}, s.config.Environment...)
	if s.config.InputViaStdin {
		cmd.Stdin = strings.NewReader(request.Input)
	}
	output := &limitedBuffer{limit: request.MaxOutputBytes}
	cmd.Stdout = output
	cmd.Stderr = &limitedBuffer{limit: request.MaxOutputBytes}

	s.mu.Lock()
	s.cmd = cmd
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.cmd = nil
		s.mu.Unlock()
	}()

	if err := cmd.Start(); err != nil {
		return BridgeResult{Outcome: DeliveryOutcomeNotDelivered, Reason: "CLI did not start"}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, request.Timeout)
	defer cancel()
	waitErr := waitCommand(runCtx, cmd)
	if waitErr != nil {
		if runCtx.Err() != nil {
			return BridgeResult{Outcome: DeliveryOutcomeUnknown, Reason: "CLI execution deadline or cancellation"}, runCtx.Err()
		}
		if output.truncated {
			return BridgeResult{Outcome: DeliveryOutcomeUnknown, Reason: "bridge output limit exceeded"}, waitErr
		}
		return BridgeResult{Outcome: DeliveryOutcomeUnknown, Reason: "CLI execution outcome is unknown", Output: output.String()}, waitErr
	}
	result := BridgeResult{
		Outcome: DeliveryOutcomeDelivered,
		Output:  output.String(),
		Events:  []BridgeEvent{{Sequence: 1, Kind: "output", Text: output.String()}, {Sequence: 2, Kind: "completed", Terminal: true}},
	}
	if err := ValidateBridgeResult(result); err != nil {
		return BridgeResult{Outcome: DeliveryOutcomeUnknown, Reason: "invalid bridge result"}, err
	}
	for _, event := range result.Events {
		if emit != nil {
			if err := emit(event); err != nil {
				return BridgeResult{Outcome: DeliveryOutcomeUnknown, Reason: "bridge event delivery failed"}, err
			}
		}
	}
	return result, nil
}

func (s *commandBridgeSession) Cancel(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}
	return s.cmd.Process.Kill()
}

func (s *commandBridgeSession) Stop(ctx context.Context) error { return s.Cancel(ctx) }

func waitCommand(ctx context.Context, cmd *exec.Cmd) error {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-done
		return ctx.Err()
	}
}

type limitedBuffer struct {
	data      []byte
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	remaining := b.limit - len(b.data)
	if remaining <= 0 {
		b.truncated = true
		return 0, io.ErrShortWrite
	}
	if len(data) > remaining {
		b.data = append(b.data, data[:remaining]...)
		b.truncated = true
		return remaining, io.ErrShortWrite
	}
	b.data = append(b.data, data...)
	return len(data), nil
}

func (b *limitedBuffer) String() string { return string(b.data) }

// NewCodexCommandBridge creates a read-only Codex CLI bridge. It is separate
// from the existing ACP v2 Provider so A2A callers can opt into CLI transport
// without changing ACP session semantics.
func NewCodexCommandBridge(workingDir string) (*CommandBridge, error) {
	return NewCommandBridge(CommandBridgeConfig{
		ID: "codex-cli", Executable: "codex", WorkingDir: workingDir,
		Args:          func(BridgeRequest) []string { return []string{"exec", "--json", "--sandbox", "read-only"} },
		InputViaStdin: true,
	})
}

// NewAgyCommandBridge creates an explicit Antigravity/agy CLI bridge. The CLI
// process is isolated by the caller's working directory and emits stream-json.
func NewAgyCommandBridge(workingDir string) (*CommandBridge, error) {
	return NewCommandBridge(CommandBridgeConfig{
		ID: "agy-cli", Executable: "agy", WorkingDir: workingDir,
		Args:          func(BridgeRequest) []string { return []string{"--print", "--output-format", "stream-json"} },
		InputViaStdin: true,
	})
}
