package v1

import (
	"context"
	"testing"

	"github.com/tbdavid2019/888a2a/backend/common"
)

func TestAuditPrincipalEvidenceUsesRequesterAndExecutor(t *testing.T) {
	ctx := common.SetOrganizationIDToContext(context.Background(), "org-a")
	ctx = common.SetRequesterPrincipalToContext(ctx, common.PrincipalIdentity{ID: "human-1", Type: "human"})
	ctx = common.SetExecutorPrincipalToContext(ctx, common.PrincipalIdentity{ID: "agent-1", Type: "agent"})
	if got := auditOrganizationID(ctx); got != "org-a" {
		t.Fatalf("audit organization = %q, want org-a", got)
	}
	if got := auditPrincipalID(ctx, true); got != "human-1" {
		t.Fatalf("audit requester = %q, want human-1", got)
	}
	if got := auditPrincipalID(ctx, false); got != "agent-1" {
		t.Fatalf("audit executor = %q, want agent-1", got)
	}
}

func TestAuditPrincipalEvidenceDefaultsOrganization(t *testing.T) {
	if got := auditOrganizationID(context.Background()); got != "default" {
		t.Fatalf("audit default organization = %q, want default", got)
	}
}
