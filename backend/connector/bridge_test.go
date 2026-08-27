package connector

import "testing"

func TestBridgeRequiresExplicitTenantPolicy(t *testing.T) {
	policy := BridgePolicy{OrganizationID: "org-a", Source: "line:conversation-1", Destinations: map[string]bool{"internal:conversation-1": true}}
	if !policy.Allows("org-a", "line:conversation-1", "internal:conversation-1") {
		t.Fatal("explicit bridge should be allowed")
	}
	if policy.Allows("org-b", "line:conversation-1", "internal:conversation-1") {
		t.Fatal("cross-tenant bridge must be rejected")
	}
}

func TestDivergenceRequiresReason(t *testing.T) {
	if err := (Divergence{OrganizationID: "org-a", InstallationID: "install-a", Source: "source", Destination: "destination", EventID: "event"}).Validate(); err == nil {
		t.Fatal("divergence without reason was accepted")
	}
}
