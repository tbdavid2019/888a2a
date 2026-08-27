package connector

import "testing"

func TestCapabilityFallbackIsExplicitForUnsupportedOperation(t *testing.T) {
	divergence, err := CapabilityFallback(fixtureConnector{}.Manifest(), CapabilityEdits, "line:source", "internal:destination", "event-1")
	if err != nil {
		t.Fatal(err)
	}
	divergence.OrganizationID = "org-a"
	divergence.InstallationID = "line-a"
	if err := divergence.Validate(); err != nil {
		t.Fatal(err)
	}
}
