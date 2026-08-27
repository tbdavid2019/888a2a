package store

import (
	"testing"

	"github.com/stretchr/testify/require"

	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

func TestApprovalBindingBuildsStableActionAndRequestName(t *testing.T) {
	binding := ApprovalBinding{
		OrganizationID:     "org-a",
		RequesterPrincipal: "user-a",
		PolicyName:         "policies/high-risk",
		PolicyVersion:      "3",
		ExecutingAgentID:   "agent-a",
	}
	action := &a2a888.BoundAction{
		OrganizationId:           binding.OrganizationID,
		AgentId:                  binding.ExecutingAgentID,
		ActionType:               "SHELL",
		Destination:              "terminal",
		TaskId:                   "work-a",
		NormalizedParametersJson: `{"command":"git status"}`,
	}
	first, err := ApprovalIntentHash(action)
	require.NoError(t, err)
	require.NotEmpty(t, first)
	require.Equal(t, ApprovalRequestName(binding, first), ApprovalRequestName(binding, first))
	require.NotEqual(t, ApprovalRequestName(binding, first), ApprovalRequestName(ApprovalBinding{OrganizationID: "org-b", RequesterPrincipal: "user-a", PolicyName: binding.PolicyName, PolicyVersion: binding.PolicyVersion, ExecutingAgentID: binding.ExecutingAgentID}, first))
}
