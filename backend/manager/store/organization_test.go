package store

import (
	"testing"

	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

func TestParseOrgState(t *testing.T) {
	tests := []struct {
		input    string
		expected a2a888.OrganizationState
	}{
		{"ACTIVE", a2a888.OrganizationState_ORGANIZATION_STATE_ACTIVE},
		{"ORGANIZATION_STATE_ACTIVE", a2a888.OrganizationState_ORGANIZATION_STATE_ACTIVE},
		{"SUSPENDED", a2a888.OrganizationState_ORGANIZATION_STATE_SUSPENDED},
		{"ORGANIZATION_STATE_SUSPENDED", a2a888.OrganizationState_ORGANIZATION_STATE_SUSPENDED},
		{"CLOSED", a2a888.OrganizationState_ORGANIZATION_STATE_CLOSED},
		{"ORGANIZATION_STATE_CLOSED", a2a888.OrganizationState_ORGANIZATION_STATE_CLOSED},
		{"UNKNOWN", a2a888.OrganizationState_ORGANIZATION_STATE_ACTIVE},
	}

	for _, tc := range tests {
		got := parseOrgState(tc.input)
		if got != tc.expected {
			t.Errorf("parseOrgState(%q) = %v; want %v", tc.input, got, tc.expected)
		}
	}
}

func TestParseOrgRole(t *testing.T) {
	tests := []struct {
		input    string
		expected a2a888.OrganizationRole
	}{
		{"OWNER", a2a888.OrganizationRole_ORGANIZATION_ROLE_OWNER},
		{"ADMIN", a2a888.OrganizationRole_ORGANIZATION_ROLE_ADMIN},
		{"MEMBER", a2a888.OrganizationRole_ORGANIZATION_ROLE_MEMBER},
		{"GUEST", a2a888.OrganizationRole_ORGANIZATION_ROLE_GUEST},
		{"UNKNOWN", a2a888.OrganizationRole_ORGANIZATION_ROLE_MEMBER},
	}

	for _, tc := range tests {
		got := parseOrgRole(tc.input)
		if got != tc.expected {
			t.Errorf("parseOrgRole(%q) = %v; want %v", tc.input, got, tc.expected)
		}
	}
}

func TestParseMembershipState(t *testing.T) {
	tests := []struct {
		input    string
		expected a2a888.MembershipState
	}{
		{"ACTIVE", a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE},
		{"SUSPENDED", a2a888.MembershipState_MEMBERSHIP_STATE_SUSPENDED},
		{"INVITED", a2a888.MembershipState_MEMBERSHIP_STATE_INVITED},
		{"UNKNOWN", a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE},
	}

	for _, tc := range tests {
		got := parseMembershipState(tc.input)
		if got != tc.expected {
			t.Errorf("parseMembershipState(%q) = %v; want %v", tc.input, got, tc.expected)
		}
	}
}

func TestTenantCacheKey_PrefixIsolation(t *testing.T) {
	cases := []struct {
		orgID        string
		resourceType string
		key          string
		expected     string
	}{
		{"org-1", "user", "101", "org:org-1:user:101"},
		{"org-2", "user", "101", "org:org-2:user:101"},
		{"", "agent", "agent-1", "org:default:agent:agent-1"},
		{"default", "agent", "agent-1", "org:default:agent:agent-1"},
	}

	for _, tc := range cases {
		got := TenantCacheKey(tc.orgID, tc.resourceType, tc.key)
		if got != tc.expected {
			t.Errorf("TenantCacheKey(%q, %q, %q) = %q; want %q", tc.orgID, tc.resourceType, tc.key, got, tc.expected)
		}
	}

	// Verify cross-tenant keys never collide even with identical resource and key.
	k1 := TenantCacheKey("tenant-alpha", "policy", "res-1")
	k2 := TenantCacheKey("tenant-beta", "policy", "res-1")
	if k1 == k2 {
		t.Fatalf("cross-tenant cache key collision detected: %q == %q", k1, k2)
	}
}

func TestTenantProjectionKey_PrefixIsolation(t *testing.T) {
	cases := []struct {
		orgID          string
		projectionName string
		id             string
		expected       string
	}{
		{"org-1", "chat_summary", "conv-1", "org:org-1:proj:chat_summary:conv-1"},
		{"org-2", "chat_summary", "conv-1", "org:org-2:proj:chat_summary:conv-1"},
		{"", "roster", "channel-1", "org:default:proj:roster:channel-1"},
	}

	for _, tc := range cases {
		got := TenantProjectionKey(tc.orgID, tc.projectionName, tc.id)
		if got != tc.expected {
			t.Errorf("TenantProjectionKey(%q, %q, %q) = %q; want %q", tc.orgID, tc.projectionName, tc.id, got, tc.expected)
		}
	}

	// Verify cross-tenant projection keys never collide.
	p1 := TenantProjectionKey("tenant-alpha", "roster", "chan-1")
	p2 := TenantProjectionKey("tenant-beta", "roster", "chan-1")
	if p1 == p2 {
		t.Fatalf("cross-tenant projection key collision detected: %q == %q", p1, p2)
	}
}

func TestOrganizationStore_InputValidations(t *testing.T) {
	store := NewOrganizationStore(nil)
	ctx := t.Context()

	t.Run("CreateOrganization requires id and slug", func(t *testing.T) {
		_, err := store.CreateOrganization(ctx, nil)
		if err == nil {
			t.Error("expected error for nil org")
		}
		_, err = store.CreateOrganization(ctx, &a2a888.Organization{Id: "", Slug: "slug"})
		if err == nil {
			t.Error("expected error for empty org id")
		}
		_, err = store.CreateOrganization(ctx, &a2a888.Organization{Id: "id", Slug: ""})
		if err == nil {
			t.Error("expected error for empty org slug")
		}
	})

	t.Run("CreateWorkspace requires id, orgID, and slug", func(t *testing.T) {
		_, err := store.CreateWorkspace(ctx, nil)
		if err == nil {
			t.Error("expected error for nil workspace")
		}
		_, err = store.CreateWorkspace(ctx, &a2a888.Workspace{Id: "ws-1", OrganizationId: "", Slug: "slug"})
		if err == nil {
			t.Error("expected error for empty organization_id")
		}
		_, err = store.CreateWorkspace(ctx, &a2a888.Workspace{Id: "", OrganizationId: "org-1", Slug: "slug"})
		if err == nil {
			t.Error("expected error for empty workspace id")
		}
	})

	t.Run("AddMembership requires orgID and principalID", func(t *testing.T) {
		_, err := store.AddMembership(ctx, nil)
		if err == nil {
			t.Error("expected error for nil membership")
		}
		_, err = store.AddMembership(ctx, &a2a888.OrganizationMembership{OrganizationId: "", PrincipalId: "101"})
		if err == nil {
			t.Error("expected error for empty orgID")
		}
		_, err = store.AddMembership(ctx, &a2a888.OrganizationMembership{OrganizationId: "org-1", PrincipalId: ""})
		if err == nil {
			t.Error("expected error for empty principalID")
		}
	})

	t.Run("UpdateMembership requires orgID and principalID", func(t *testing.T) {
		err := store.UpdateMembership(ctx, nil)
		if err == nil {
			t.Error("expected error for nil membership")
		}
		err = store.UpdateMembership(ctx, &a2a888.OrganizationMembership{OrganizationId: "", PrincipalId: "101"})
		if err == nil {
			t.Error("expected error for empty orgID")
		}
	})

	t.Run("ListMemberships requires orgID", func(t *testing.T) {
		_, err := store.ListMemberships(ctx, "")
		if err == nil {
			t.Error("expected error for empty orgID")
		}
	})
}
