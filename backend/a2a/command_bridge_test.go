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
			workdir := filepath.Join(t.TempDir(), tc.name)
			if err := os.MkdirAll(workdir, 0o700); err != nil {
				t.Fatal(err)
			}
			bridge, err := tc.make(workdir)
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

func TestAgyCommandBridgeRealSmokeIsOptIn(t *testing.T) {
	if os.Getenv("A2A888_RUN_AGY_BRIDGE_TESTS") != "1" {
		t.Skip("set A2A888_RUN_AGY_BRIDGE_TESTS=1 to run the local agy smoke gate")
	}
	workdir := os.Getenv("A2A888_AGY_TEST_WORKDIR")
	if workdir == "" {
		workdir = t.TempDir()
	}
	bridge, err := NewAgyCommandBridge(workdir)
	if err != nil {
		t.Fatal(err)
	}
	request := validBridgeRequest()
	request.BridgeID = bridge.ID()
	request.Input = "Reply with exactly: agy-bridge-ok"
	request.MaxOutputBytes = 64 * 1024
	request.Timeout = 2 * time.Minute
	result, err := ExecuteBridge(context.Background(), bridge, request, nil)
	if err != nil || result.Outcome != DeliveryOutcomeDelivered || !strings.Contains(result.Output, "agy-bridge-ok") {
		t.Fatalf("agy smoke result=%+v err=%v", result, err)
	}
}

func TestParseAgyStreamJSON(t *testing.T) {
	parsed, err := parseAgyStreamJSON("{\"event\":\"step_update\",\"step_update\":{\"text_delta\":\"hello \"}}\n{\"event\":\"result\",\"result\":{\"response\":\"hello world\\n\"}}")
	if err != nil || parsed != "hello world\n" {
		t.Fatalf("parsed=%q err=%v", parsed, err)
	}
}
