package common

import (
	"context"
	"testing"
)

func TestPrincipalEvidenceRemainsDistinct(t *testing.T) {
	ctx := context.Background()
	ctx = SetOrganizationIDToContext(ctx, "org-a")
	ctx = SetWorkspaceIDToContext(ctx, "workspace-a")
	ctx = SetRequesterPrincipalToContext(ctx, PrincipalIdentity{ID: "user-1", OrganizationID: "org-a", Type: "human"})
	ctx = SetExecutorPrincipalToContext(ctx, PrincipalIdentity{ID: "agent-1", OrganizationID: "org-a", Type: "agent"})

	requester, ok := GetRequesterPrincipalFromContext(ctx)
	if !ok || requester.ID != "user-1" || requester.Type != "human" {
		t.Fatalf("requester evidence = %+v, %v", requester, ok)
	}
	executor, ok := GetExecutorPrincipalFromContext(ctx)
	if !ok || executor.ID != "agent-1" || executor.Type != "agent" {
		t.Fatalf("executor evidence = %+v, %v", executor, ok)
	}
	if organizationID, ok := GetOrganizationIDFromContext(ctx); !ok || organizationID != "org-a" {
		t.Fatalf("organization context = %q, %v", organizationID, ok)
	}
	if workspaceID, ok := GetWorkspaceIDFromContext(ctx); !ok || workspaceID != "workspace-a" {
		t.Fatalf("workspace context = %q, %v", workspaceID, ok)
	}
}
