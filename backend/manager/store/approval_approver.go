package store

import (
	"fmt"
	"sort"
	"strings"

	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

// ResolveEligibleApprovalApprovers resolves the current decision boundary for
// an approval policy. It deliberately re-evaluates memberships and group
// payloads supplied by the caller, so a suspended, invited, or removed member
// cannot decide based on eligibility at request creation time.
func ResolveEligibleApprovalApprovers(policy *a2a888.ApprovalPolicy, requesterID, agentOwnerID string, memberships []*a2a888.OrganizationMembership, groups []*GroupMessage) ([]string, error) {
	if policy == nil || policy.OrganizationId == "" {
		return nil, fmt.Errorf("approval policy organization_id is required")
	}

	active := make(map[string]*a2a888.OrganizationMembership)
	for _, membership := range memberships {
		if membership == nil || membership.OrganizationId != policy.OrganizationId || membership.PrincipalId == "" {
			continue
		}
		if membership.State != a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE {
			continue
		}
		active[normalizeApprovalPrincipal(membership.PrincipalId)] = membership
	}

	groupMembers := make(map[string]map[string]struct{})
	for _, group := range groups {
		if group == nil || group.OrganizationID != policy.OrganizationId || group.ID == "" || group.Payload == nil {
			continue
		}
		members := make(map[string]struct{})
		for _, member := range group.Payload.Members {
			if member == nil {
				continue
			}
			principalID := normalizeApprovalPrincipal(member.Member)
			if principalID != "" {
				members[principalID] = struct{}{}
			}
		}
		for _, identifier := range []string{group.ID, group.Email, "groups/" + group.ID, "groups/" + group.Email} {
			if identifier != "" {
				groupMembers[identifier] = members
			}
		}
	}

	eligible := make(map[string]struct{})
	add := func(principalID string) {
		principalID = normalizeApprovalPrincipal(principalID)
		if _, ok := active[principalID]; ok {
			eligible[principalID] = struct{}{}
		}
	}
	for _, principalID := range policy.ApproverPrincipalIds {
		add(principalID)
	}
	for _, groupID := range policy.ApproverGroupIds {
		for principalID := range groupMembers[groupID] {
			add(principalID)
		}
	}
	for _, role := range policy.ApproverRoles {
		for principalID, membership := range active {
			if approvalRoleMatches(role, membership.Role) {
				eligible[principalID] = struct{}{}
			}
		}
	}
	if policy.ProhibitRequesterApproval {
		delete(eligible, normalizeApprovalPrincipal(requesterID))
	}
	if policy.ProhibitAgentOwnerSoleApproval && len(eligible) == 1 {
		delete(eligible, normalizeApprovalPrincipal(agentOwnerID))
	}

	result := make([]string, 0, len(eligible))
	for principalID := range eligible {
		result = append(result, principalID)
	}
	sort.Strings(result)
	return result, nil
}

func normalizeApprovalPrincipal(principalID string) string {
	return strings.TrimPrefix(strings.TrimSpace(principalID), "users/")
}

func approvalRoleMatches(configured string, role a2a888.OrganizationRole) bool {
	configured = normalizeApprovalRole(configured)
	actual := normalizeApprovalRole(role.String())
	return configured != "" && configured == actual
}

func normalizeApprovalRole(role string) string {
	role = strings.ToUpper(strings.TrimSpace(role))
	role = strings.TrimPrefix(role, "ROLES/")
	role = strings.TrimPrefix(role, "ROLE/")
	role = strings.TrimPrefix(role, "ORGANIZATION_ROLE_")
	return strings.TrimPrefix(role, "ROLE_")
}
