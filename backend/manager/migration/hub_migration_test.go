package migration

import (
	"os"
	"strings"
	"testing"
)

func TestHubRegistrationMigrationIsInFreshAndIncrementalSchemas(t *testing.T) {
	incremental, err := os.ReadFile("migration/1.1/0049##agent-hub-registration.sql")
	if err != nil {
		t.Fatal(err)
	}
	latest, err := os.ReadFile("migration/LATEST.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range []string{"CREATE TABLE IF NOT EXISTS a2a888_hub (", "CREATE TABLE IF NOT EXISTS a2a888_hub_agent (", "uq_a2a888_hub_agent_registration", "uq_a2a888_hub_agent_token"} {
		if !strings.Contains(string(incremental), declaration) || !strings.Contains(string(latest), declaration) {
			t.Fatalf("Hub migration declaration %q missing from incremental or fresh schema", declaration)
		}
	}
	if strings.Contains(string(incremental), "agent_token TEXT") || strings.Contains(string(latest), "agent_token TEXT") {
		t.Fatal("Hub schemas must not contain plaintext agent token columns")
	}
}
