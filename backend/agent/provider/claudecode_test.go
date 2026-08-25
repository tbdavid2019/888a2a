package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeFakeExecutable(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeCodeDetectRequiresClaudeCLI(t *testing.T) {
	dir := t.TempDir()
	writeFakeExecutable(t, dir, "claude-agent-acp", "exit 0")
	t.Setenv("PATH", dir)

	p := &ClaudeCodeProvider{}
	_, present, err := p.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect with wrapper only: %v", err)
	}
	if present {
		t.Fatal("Claude Code must not be reported present when claude CLI is missing")
	}
}

func TestClaudeCodeDetectReportsClaudeBeforePreparation(t *testing.T) {
	dir := t.TempDir()
	writeFakeExecutable(t, dir, "claude", `echo "1.0.0"`)
	t.Setenv("PATH", dir)
	t.Setenv("CLAUDE_AGENT_ACP_PATH", "")

	p := &ClaudeCodeProvider{}
	info, present, err := p.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect with claude only: %v", err)
	}
	if !present {
		t.Fatal("Claude Code should be reported for runtime preparation when the claude CLI is present")
	}
	if info == nil || info.ExecutablePath != "" {
		t.Fatalf("unprepared Claude detection = %+v, want no executable path", info)
	}
}

func TestClaudeCodeDetectPresentWithClaudeAndPreparedBinary(t *testing.T) {
	dir := t.TempDir()
	writeFakeExecutable(t, dir, "claude-agent-acp", "exit 0")
	writeFakeExecutable(t, dir, "claude", `echo "1.0.0"`)
	t.Setenv("PATH", dir)

	p := &ClaudeCodeProvider{}
	info, present, err := p.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect with claude+claude-agent-acp: %v", err)
	}
	if !present {
		t.Fatal("Claude Code should be present when both claude and claude-agent-acp are installed")
	}
	if info == nil {
		t.Fatal("expected detected info")
	}
	if info.Version != "1.0.0" {
		t.Fatalf("version = %q, want 1.0.0", info.Version)
	}
	if filepath.Base(info.ExecutablePath) != "claude-agent-acp" {
		t.Fatalf("executable path = %q, want claude-agent-acp", info.ExecutablePath)
	}
}

func TestClaudeCodeBuildCommandStrictNoNpxFallback(t *testing.T) {
	p := &ClaudeCodeProvider{}

	// 1. When no local binary exists, return empty (no npx fallback)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("CLAUDE_AGENT_ACP_PATH", "")
	exe, args := p.BuildCommand("/ws")
	if exe != "" || len(args) != 0 {
		t.Fatalf("BuildCommand without local binary must return empty, got %s %v", exe, args)
	}

	// 2. Prioritizes CLAUDE_AGENT_ACP_PATH env override
	t.Setenv("CLAUDE_AGENT_ACP_PATH", "/custom/bin/claude-acp")
	exe, args = p.BuildCommand("/ws")
	if exe != "/custom/bin/claude-acp" || len(args) != 0 {
		t.Fatalf("BuildCommand env override = %s %v", exe, args)
	}

	// 3. Prioritizes local prepared binary on PATH
	t.Setenv("CLAUDE_AGENT_ACP_PATH", "")
	dir := t.TempDir()
	writeFakeExecutable(t, dir, "claude-agent-acp", "exit 0")
	t.Setenv("PATH", dir)
	exe, args = p.BuildCommand("/ws")
	if exe != filepath.Join(dir, "claude-agent-acp") || len(args) != 0 {
		t.Fatalf("BuildCommand local binary on PATH = %s %v", exe, args)
	}
}
