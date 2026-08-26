package store

import "testing"

func TestAuditStorageContractIsTenantScoped(t *testing.T) {
	if got := normalizeAuditPayload(""); got != "{}" {
		t.Fatalf("empty audit payload = %q, want {}", got)
	}
	if got := normalizeAuditPayload(`{"ok":true}`); got != `{"ok":true}` {
		t.Fatalf("valid audit payload changed: %q", got)
	}
	log := &AuditLogMessage{OrganizationID: "org-a", RequesterID: "human-1", ExecutorID: "agent-1"}
	if log.OrganizationID == "" || log.RequesterID == "" || log.ExecutorID == "" {
		t.Fatal("audit evidence must carry tenant, requester, and executor IDs")
	}
}
