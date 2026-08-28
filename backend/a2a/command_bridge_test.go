package a2a

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCommandBridgeInvokesBoundedStdinRuntime(t *testing.T) {
	bridge, err := NewCommandBridge(CommandBridgeConfig{
		ID: "cat-test", Executable: "sh", WorkingDir: t.TempDir(),
		Args:          func(BridgeRequest) []string { return []string{"-c", "cat"} },
		InputViaStdin: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := validBridgeRequest()
	request.BridgeID = "cat-test"
	request.Input = "hello bridge"
	var events []BridgeEvent
	result, err := ExecuteBridge(context.Background(), bridge, request, func(event BridgeEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil || result.Outcome != DeliveryOutcomeDelivered || result.Output != request.Input {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(events) != 2 || !events[len(events)-1].Terminal {
		t.Fatalf("events=%+v, want output and terminal events", events)
	}
}

func TestCommandBridgeRejectsRelativeWorkingDirectory(t *testing.T) {
	if _, err := NewCommandBridge(CommandBridgeConfig{ID: "bad", Executable: "sh", WorkingDir: "relative"}); err == nil {
		t.Fatal("expected relative working directory to be rejected")
	}
}

func TestCommandBridgeRealRuntimeGatesAreOptIn(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  string
		make func(string) (AgentBridge, error)
	}{
		{name: "codex", env: "A2A888_RUN_CODEX_BRIDGE_TESTS", make: func(dir string) (AgentBridge, error) {
			return NewCodexCommandBridge(dir)
		}},
		{name: "agy", env: "A2A888_RUN_AGY_BRIDGE_TESTS", make: func(dir string) (AgentBridge, error) {
			return NewAgyCommandBridge(dir)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if os.Getenv(tc.env) != "1" {
				t.Skipf("set %s=1 to run the local %s preflight", tc.env, tc.name)
			}
			bridge, err := tc.make(filepath.Join(t.TempDir(), tc.name))
			if err != nil {
				t.Fatal(err)
			}
			if err := bridge.Preflight(context.Background(), BridgeRequest{
				OrganizationID: "org-a", CallerID: "agent-a", TaskID: "task-a", ContextID: "ctx-a",
				CorrelationID: "corr-a", BridgeID: bridge.ID(), MaxOutputBytes: 1024, Timeout: time.Second,
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCommandBridgeOutputLimitDoesNotReportDelivery(t *testing.T) {
	bridge, err := NewCommandBridge(CommandBridgeConfig{
		ID: "large-test", Executable: "sh", WorkingDir: t.TempDir(),
		Args: func(BridgeRequest) []string { return []string{"-c", "printf 1234567890"} },
	})
	if err != nil {
		t.Fatal(err)
	}
	request := validBridgeRequest()
	request.BridgeID = "large-test"
	request.MaxOutputBytes = 5
	result, err := ExecuteBridge(context.Background(), bridge, request, nil)
	if err == nil || result.Outcome != DeliveryOutcomeUnknown || !strings.Contains(result.Reason, "output limit") {
		t.Fatalf("result=%+v err=%v, want unknown output-limit result", result, err)
	}
}
