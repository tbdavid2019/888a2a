package v1

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/tbdavid2019/888a2a/backend/common"
	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

func TestOrganizationService_Unauthenticated(t *testing.T) {
	svc := &OrganizationService{}
	ctx := context.Background()

	_, err := svc.ListOrganizations(ctx, connect.NewRequest(&a2a888.ListOrganizationsRequest{}))
	if err == nil || connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("ListOrganizations without auth error = %v; want CodeUnauthenticated", err)
	}

	_, err = svc.GetOrganization(ctx, connect.NewRequest(&a2a888.GetOrganizationRequest{OrganizationId: "default"}))
	if err == nil || connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("GetOrganization without auth error = %v; want CodeUnauthenticated", err)
	}

	_, err = svc.SwitchOrganization(ctx, connect.NewRequest(&a2a888.SwitchOrganizationRequest{OrganizationId: "default"}))
	if err == nil || connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("SwitchOrganization without auth error = %v; want CodeUnauthenticated", err)
	}

	_, err = svc.ListWorkspaces(ctx, connect.NewRequest(&a2a888.ListWorkspacesRequest{OrganizationId: "default"}))
	if err == nil || connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("ListWorkspaces without auth error = %v; want CodeUnauthenticated", err)
	}

	_, err = svc.ListMemberships(ctx, connect.NewRequest(&a2a888.ListMembershipsRequest{OrganizationId: "default"}))
	if err == nil || connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("ListMemberships without auth error = %v; want CodeUnauthenticated", err)
	}
}

func TestOrganizationService_SwitchOrganization_EmptyTarget(t *testing.T) {
	svc := &OrganizationService{}
	user := &store.UserMessage{
		ID:                    1,
		Email:                 "test@example.com",
		DefaultOrganizationID: "default",
	}
	ctx := context.WithValue(context.Background(), common.UserContextKey, user)

	_, err := svc.SwitchOrganization(ctx, connect.NewRequest(&a2a888.SwitchOrganizationRequest{OrganizationId: ""}))
	if err == nil || connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("SwitchOrganization with empty id error = %v; want CodeInvalidArgument", err)
	}
}

func TestCanManageOrganization(t *testing.T) {
	for _, tc := range []struct {
		name string
		role a2a888.OrganizationRole
		want bool
	}{
		{name: "owner", role: a2a888.OrganizationRole_ORGANIZATION_ROLE_OWNER, want: true},
		{name: "admin", role: a2a888.OrganizationRole_ORGANIZATION_ROLE_ADMIN, want: true},
		{name: "member", role: a2a888.OrganizationRole_ORGANIZATION_ROLE_MEMBER, want: false},
		{name: "guest", role: a2a888.OrganizationRole_ORGANIZATION_ROLE_GUEST, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := canManageOrganization(tc.role); got != tc.want {
				t.Fatalf("canManageOrganization(%v) = %v, want %v", tc.role, got, tc.want)
			}
		})
	}
}

func TestActiveOrganizationIDRejectsUnknownCandidate(t *testing.T) {
	orgs := []*a2a888.Organization{
		{Id: "org-a"},
		{Id: "org-b"},
	}
	if got := activeOrganizationID(orgs, "org-unknown", "org-b"); got != "org-b" {
		t.Fatalf("activeOrganizationID returned %q, want org-b", got)
	}
	if got := activeOrganizationID(orgs, "org-unknown", "org-unknown"); got != "org-a" {
		t.Fatalf("activeOrganizationID returned %q, want first accessible organization org-a", got)
	}
}
