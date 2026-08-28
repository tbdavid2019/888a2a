package a2a

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestHubRegistryOpenRegistrationAssignsIdempotentIdentity(t *testing.T) {
	clock := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	policy := DefaultHubPolicy()
	policy.Mode = HubModeOpen
	policy.HubID = "hub-private"
	policy.RegistrationEnabled = true
	registry, err := NewHubRegistry(policy, "bootstrap-secret-value", func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	declaration := validAgentDeclaration("codex")
	first, err := registry.Register("bootstrap-secret-value", declaration)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first.AgentID, "agt_") || first.HubID != policy.HubID || first.AgentToken == "" {
		t.Fatalf("issued identity = %+v", first)
	}
	second, err := registry.Register("bootstrap-secret-value", declaration)
	if err != nil || second.AgentID != first.AgentID || second.AgentToken != "" {
		t.Fatalf("idempotent registration = %+v, err=%v", second, err)
	}
	if _, err := registry.Authenticate(first.AgentID, first.AgentToken); err != nil {
		t.Fatalf("issued token authentication: %v", err)
	}
	if _, err := registry.Authenticate(first.AgentID, "wrong-token"); err == nil {
		t.Fatal("wrong token must be rejected")
	}
}

func TestHubRegistryPublicRegistrationRequiresNoBootstrapButIsBounded(t *testing.T) {
	policy := DefaultHubPolicy()
	policy.Mode = HubModePublic
	policy.PublicConfirmed = true
	policy.HubID = "hub-public"
	policy.MaxRegisteredAgents = 1
	policy.RegistrationEnabled = true
	registry, err := NewHubRegistry(policy, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := registry.Register("", validAgentDeclaration("agy"))
	if err != nil || first.AgentToken == "" {
		t.Fatalf("public registration = %+v, err=%v", first, err)
	}
	if _, err := registry.Register("", validAgentDeclaration("agy-2")); err == nil {
		t.Fatal("public registration quota must be enforced")
	}
}

func TestHubRegistryRevokesAndExpiresAgentsWithoutLeakingToken(t *testing.T) {
	clock := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	policy := DefaultHubPolicy()
	policy.Mode = HubModePublic
	policy.PublicConfirmed = true
	policy.HubID = "hub-public"
	policy.RegistrationTTL = 60
	policy.PeerLeaseSeconds = 10
	policy.RegistrationEnabled = true
	registry, err := NewHubRegistry(policy, "", func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	identity, err := registry.Register("", validAgentDeclaration("openclaw"))
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Revoke(identity.AgentID, "operator test"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Authenticate(identity.AgentID, identity.AgentToken); err == nil {
		t.Fatal("revoked token must be rejected")
	}
	expiring, err := registry.Register("", validAgentDeclaration("openclaw-expiring"))
	if err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(61 * time.Second)
	stale := registry.Reconcile()
	if len(stale) != 1 || stale[0].AgentID != expiring.AgentID || stale[0].State != HubAgentStateExpired {
		t.Fatalf("expired agents = %+v", stale)
	}
	public, err := registry.Authenticate(expiring.AgentID, expiring.AgentToken)
	if err == nil || public != nil {
		t.Fatal("expired Agent token must be rejected")
	}
	metadata, err := json.Marshal(registry.List()[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(metadata), identity.AgentToken) {
		t.Fatal("Agent token leaked into public peer metadata")
	}
}

func TestHubRegistryRotatesAgentToken(t *testing.T) {
	policy := DefaultHubPolicy()
	policy.Mode = HubModePublic
	policy.PublicConfirmed = true
	policy.RegistrationEnabled = true
	registry, err := NewHubRegistry(policy, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := registry.Register("", validAgentDeclaration("rotate"))
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := registry.RotateToken(identity.AgentID)
	if err != nil || rotated == identity.AgentToken || rotated == "" {
		t.Fatalf("rotated token=%q err=%v", rotated, err)
	}
	if _, err := registry.Authenticate(identity.AgentID, identity.AgentToken); err == nil {
		t.Fatal("old token must fail after rotation")
	}
	if _, err := registry.Authenticate(identity.AgentID, rotated); err != nil {
		t.Fatalf("rotated token must authenticate: %v", err)
	}
}

func validAgentDeclaration(provider string) AgentDeclaration {
	return AgentDeclaration{
		DisplayName: "test-agent", ProviderFamily: provider, TransportID: provider + "-transport",
		Capabilities: []string{"text"}, AgentCardJSON: `{"name":"test-agent"}`, RegistrationIdempotencyKey: "registration-key-" + provider,
	}
}
