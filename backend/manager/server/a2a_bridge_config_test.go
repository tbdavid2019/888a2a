package server

import (
	"strings"
	"testing"
)

func TestConfiguredA2ABridgeRequiresExplicitProviderAndWorkspace(t *testing.T) {
	t.Setenv("A2A888_A2A_BRIDGE_AGENT_ID", "agent-a")
	t.Setenv("A2A888_A2A_BRIDGE_PROVIDER", "codex")
	t.Setenv("A2A888_A2A_BRIDGE_WORKDIR", "relative/workdir")
	if _, err := configuredA2ABridge("agent-a"); err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("relative bridge workspace error = %v", err)
	}
	if _, err := configuredA2ABridge("other-agent"); err == nil || !strings.Contains(err.Error(), "no bridge configured") {
		t.Fatalf("wrong agent error = %v", err)
	}
}

func TestConfiguredA2ABridgeDoesNotGuessProvider(t *testing.T) {
	t.Setenv("A2A888_A2A_BRIDGE_AGENT_ID", "agent-a")
	t.Setenv("A2A888_A2A_BRIDGE_PROVIDER", "")
	if _, err := configuredA2ABridge("agent-a"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("missing provider error = %v", err)
	}
}

func TestConfiguredA2ABridgeOpenClawRequiresGatewayURL(t *testing.T) {
	t.Setenv("A2A888_A2A_BRIDGE_AGENT_ID", "agent-a")
	t.Setenv("A2A888_A2A_BRIDGE_PROVIDER", "openclaw")
	t.Setenv("A2A888_OPENCLAW_GATEWAY_URL", "")
	if _, err := configuredA2ABridge("agent-a"); err == nil || !strings.Contains(err.Error(), "GATEWAY_URL") {
		t.Fatalf("missing OpenClaw URL error = %v", err)
	}
}
