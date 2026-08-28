package provider

import (
	"strings"
	"testing"
)

func TestProjectCatalogEntryKeepsUnverifiedProviderConservative(t *testing.T) {
	entry := Catalog()[0]
	projected := ProjectCatalogEntry(entry, &Discovered{
		ProviderID:         entry.ID,
		RuntimeStatus:      "DETECTED",
		CompatibilityLevel: "DETECTED",
		FailureMessage:     "Bearer super-secret\n/path/to/private/config session_id=abc123",
		ExecutablePath:     "/private/path/that/must/not/project",
	})
	if projected.Readiness != ReadinessDetectedOnly {
		t.Fatalf("readiness = %q, want DETECTED_ONLY", projected.Readiness)
	}
	if projected.FailureReason == "" || projected.FailureReason == "Bearer super-secret" ||
		strings.Contains(projected.FailureReason, "/path/to/private/config") ||
		strings.Contains(projected.FailureReason, "abc123") {
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
	if projected.Readiness != ReadinessReady || !projected.Automatic || projected.TransportID != "codex-acp2" {
		t.Fatalf("projection = %+v, want verified Codex ACP transport", projected)
	}
}

func TestProjectCatalogEntryDoesNotUpgradeProtocolReadyToAutomaticReady(t *testing.T) {
	entry := Catalog()[3]
	projected := ProjectCatalogEntry(entry, &Discovered{
		ProviderID:         entry.ID,
		RuntimeStatus:      "READY",
		CompatibilityLevel: "PROTOCOL_READY",
	})
	if projected.Readiness == ReadinessReady {
		t.Fatalf("projection = %+v, protocol-only evidence must not claim READY", projected)
	}
	if projected.TransportID == "codex-acp2" {
		t.Fatalf("projection = %+v, automatic transport must not be advertised", projected)
	}
}

func TestProjectCatalogEntryWithoutEvidenceDoesNotClaimReady(t *testing.T) {
	entry := Catalog()[3]
	projected := ProjectCatalogEntry(entry, nil)
	if projected.Readiness == ReadinessReady {
		t.Fatalf("projection = %+v, missing host evidence must not claim READY", projected)
	}
}
