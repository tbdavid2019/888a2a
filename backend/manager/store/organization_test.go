package store

import (
	"testing"

	a2a888 "github.com/Ranxy/laelia/backend/generated-go/a2a888"
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
