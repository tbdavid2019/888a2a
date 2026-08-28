package bridge

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	a2apkg "github.com/tbdavid2019/888a2a/backend/a2a"
)

func TestNewCodexACPBridgeRequiresAbsoluteWorkspace(t *testing.T) {
	if _, err := NewCodexACPBridge(CodexACPBridgeConfig{WorkingDir: "relative"}); err == nil {
		t.Fatal("relative workspace must be rejected")
	}
	bridge, err := NewCodexACPBridge(CodexACPBridgeConfig{WorkingDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	request := a2apkg.BridgeRequest{
		OrganizationID: "org", CallerID: "caller", TaskID: "task", ContextID: "ctx", CorrelationID: "corr",
		BridgeID: bridge.ID(), Input: "hello", MaxOutputBytes: 1024, Timeout: time.Second,
	}
	if err := bridge.Preflight(context.Background(), request); err != nil && os.Getenv("A2A888_RUN_CODEX_ACP_TESTS") != "1" {
		t.Logf("Codex preflight unavailable locally: %v", err)
	}
}

func TestCodexACPBridgeRealGateIsOptIn(t *testing.T) {
	if os.Getenv("A2A888_RUN_CODEX_ACP_TESTS") != "1" {
		t.Skip("set A2A888_RUN_CODEX_ACP_TESTS=1 to run the local Codex ACP v2 gate")
	}
	workingDir := os.Getenv("A2A888_CODEX_TEST_WORKDIR")
	if workingDir == "" {
		workingDir = filepath.Join(t.TempDir(), "workspace")
		if err := os.MkdirAll(workingDir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	bridge, err := NewCodexACPBridge(CodexACPBridgeConfig{WorkingDir: workingDir, Model: os.Getenv("A2A888_CODEX_MODEL")})
	if err != nil {
		t.Fatal(err)
	}
	request := a2apkg.BridgeRequest{
		OrganizationID: "local-test", CallerID: "local-test", TaskID: "codex-acp-gate", ContextID: "codex-acp-context", CorrelationID: "codex-acp-correlation",
		BridgeID: bridge.ID(), Input: "Reply with exactly: codex-acp-ok", MaxOutputBytes: 64 * 1024, Timeout: 2 * time.Minute,
	}
	result, err := a2apkg.ExecuteBridge(context.Background(), bridge, request, nil)
	if err != nil || result.Outcome != a2apkg.DeliveryOutcomeDelivered {
		t.Fatalf("Codex ACP gate result=%+v err=%v", result, err)
	}
}
