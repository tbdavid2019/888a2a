package a2a

import "testing"

func TestParseHubModeDefaultsClosedAndRejectsUnknown(t *testing.T) {
	if got, err := ParseHubMode(""); err != nil || got != HubModeClosed {
		t.Fatalf("empty mode = %q, err=%v; want closed", got, err)
	}
	if got, err := ParseHubMode("open"); err != nil || got != HubModeOpen {
		t.Fatalf("open mode = %q, err=%v", got, err)
	}
	if _, err := ParseHubMode("mesh"); err == nil {
		t.Fatal("unknown mode must be rejected")
	}
}

func TestHubPolicyRejectsPublicWithoutExplicitConfirmation(t *testing.T) {
	policy := DefaultHubPolicy()
	policy.Mode = HubModePublic
	if err := policy.Validate(); err == nil {
		t.Fatal("public mode must require explicit confirmation")
	}
	policy.PublicConfirmed = true
	if err := policy.Validate(); err != nil {
		t.Fatalf("confirmed public policy: %v", err)
	}
}

func TestAgentDeclarationRejectsOversizedUntrustedFields(t *testing.T) {
	declaration := AgentDeclaration{DisplayName: "agent", ProviderFamily: "codex", TransportID: "codex-acp2", RegistrationIdempotencyKey: "key"}
	declaration.DisplayName = string(make([]byte, MaxHubDisplayNameBytes+1))
	if err := declaration.Validate(); err == nil {
		t.Fatal("oversized display name must be rejected")
	}
	declaration.DisplayName = "agent"
	declaration.AgentCardJSON = string(make([]byte, MaxHubAgentCardBytes+1))
	if err := declaration.Validate(); err == nil {
		t.Fatal("oversized Agent Card must be rejected")
	}
}
