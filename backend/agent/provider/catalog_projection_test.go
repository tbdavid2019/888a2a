package provider

import "testing"

func TestProjectCatalogEntryKeepsUnverifiedProviderConservative(t *testing.T) {
	entry := Catalog()[0]
	projected := ProjectCatalogEntry(entry, &Discovered{
		ProviderID:         entry.ID,
		RuntimeStatus:      "DETECTED",
		CompatibilityLevel: "DETECTED",
		FailureMessage:     "Bearer super-secret\n/path/to/private/config",
		ExecutablePath:     "/private/path/that/must/not/project",
	})
	if projected.Readiness != ReadinessDetectedOnly {
		t.Fatalf("readiness = %q, want DETECTED_ONLY", projected.Readiness)
	}
	if projected.FailureReason == "" || projected.FailureReason == "Bearer super-secret" || projected.FailureReason != "[redacted] /path/to/private/config" {
		t.Fatalf("failure reason was not safely sanitized: %q", projected.FailureReason)
	}
	if projected.TransportID == "" || projected.TransportMode == "" {
		t.Fatal("projection must retain a transport boundary")
	}
}

func TestProjectCatalogEntrySelectsOnlyVerifiedAutomaticTransport(t *testing.T) {
	entry := Catalog()[3]
	projected := ProjectCatalogEntry(entry, &Discovered{
		ProviderID:         entry.ID,
		RuntimeStatus:      "READY",
		CompatibilityLevel: "FULL_LOOP_VERIFIED",
	})
	if projected.Readiness != ReadinessReady || projected.TransportID != "codex-acp2" {
		t.Fatalf("projection = %+v, want verified Codex ACP transport", projected)
	}
}
