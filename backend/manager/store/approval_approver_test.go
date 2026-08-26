package store

import (
	"testing"

	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
	models "github.com/tbdavid2019/888a2a/backend/generated-go/store"
)

func TestResolveEligibleApprovalApprovers(t *testing.T) {
	group := &GroupMessage{
		OrganizationID: "org-1",
		ID:             "finance",
		Payload: &models.GroupPayload{Members: []*models.GroupMember{
			{Member: "users/1"},
			{Member: "users/2"},
		}},
	}
	policy := &a2a888.ApprovalPolicy{
		OrganizationId:            "org-1",
		ApproverPrincipalIds:      []string{"3"},
		ApproverGroupIds:          []string{"groups/finance"},
		ApproverRoles:             []string{"roles/approver"},
		ProhibitRequesterApproval: true,
	}
	memberships := []*a2a888.OrganizationMembership{
		{OrganizationId: "org-1", PrincipalId: "1", State: a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE, Role: a2a888.OrganizationRole_ORGANIZATION_ROLE_MEMBER},
		{OrganizationId: "org-1", PrincipalId: "2", State: a2a888.MembershipState_MEMBERSHIP_STATE_SUSPENDED, Role: a2a888.OrganizationRole_ORGANIZATION_ROLE_APPROVER},
		{OrganizationId: "org-1", PrincipalId: "3", State: a2a888.MembershipState_MEMBERSHIP_STATE_INVITED, Role: a2a888.OrganizationRole_ORGANIZATION_ROLE_MEMBER},
		{OrganizationId: "org-1", PrincipalId: "4", State: a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE, Role: a2a888.OrganizationRole_ORGANIZATION_ROLE_APPROVER},
	}
	got, err := ResolveEligibleApprovalApprovers(policy, "1", "4", memberships, []*GroupMessage{group})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"4"}; !sameStrings(got, want) {
		t.Fatalf("eligible approvers = %v, want %v", got, want)
	}
}

func TestResolveEligibleApprovalApproversExcludesRemovedGroupMember(t *testing.T) {
	policy := &a2a888.ApprovalPolicy{OrganizationId: "org-1", ApproverGroupIds: []string{"group-1"}}
	memberships := []*a2a888.OrganizationMembership{
		{OrganizationId: "org-1", PrincipalId: "1", State: a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE},
	}
	got, err := ResolveEligibleApprovalApprovers(policy, "", "", memberships, []*GroupMessage{{OrganizationID: "org-1", ID: "group-1", Payload: &models.GroupPayload{}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("eligible approvers = %v, want none", got)
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
