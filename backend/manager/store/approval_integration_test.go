package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

func TestApprovalSchemaIntentAndImmutability(t *testing.T) {
	services, _ := requireCommandEventIntegrationStore(t)
	ctx := context.Background()
	approvalStore := NewApprovalStore(services.GetDB())
	policy := &a2a888.ApprovalPolicy{
		Name: "organizations/default/approvalPolicies/deploy", OrganizationId: "default", Version: "1",
		RequiredApprovals: 1, TimeoutSeconds: 300, OnTimeout: a2a888.ApprovalTimeoutAction_APPROVAL_TIMEOUT_ACTION_DENY, Enabled: true,
	}
	require.NoError(t, approvalStore.CreatePolicy(ctx, policy))
	request := &a2a888.ApprovalRequest{
		Name: "organizations/default/approvalRequests/request-1", OrganizationId: "default", PolicyName: policy.Name, PolicyVersion: "1",
		RequesterPrincipalId: "user-1", RequiredApprovals: 1,
		Action:    &a2a888.BoundAction{OrganizationId: "default", ResourceName: "deployments/prod", ActionType: "deploy", NormalizedParametersJson: `{"version":"1.2.3"}`},
		ExpiresAt: timestamppb.New(time.Now().Add(time.Hour)),
	}
	require.NoError(t, approvalStore.CreateRequest(ctx, request))
	require.NotEmpty(t, request.ExecutionNonce)
	require.NotEmpty(t, request.Action.IntentHash)
	require.NoError(t, approvalStore.CreateDecision(ctx, &a2a888.ApprovalDecision{
		Name: "organizations/default/approvalRequests/request-1/decisions/1", OrganizationId: "default", RequestName: request.Name,
		ApproverPrincipalId: "user-2", Outcome: a2a888.ApprovalDecisionOutcome_APPROVAL_DECISION_OUTCOME_APPROVE, IntentHash: request.Action.IntentHash,
	}))
	_, err := services.GetDB().ExecContext(ctx, `UPDATE a2a888_approval_request SET intent_hash = 'tampered' WHERE organization_id = 'default' AND name = $1`, request.Name)
	require.Error(t, err)
	_, err = services.GetDB().ExecContext(ctx, `UPDATE a2a888_approval_decision SET reason = 'tampered' WHERE organization_id = 'default' AND name = $1`, "organizations/default/approvalRequests/request-1/decisions/1")
	require.Error(t, err)
	_, err = services.GetDB().ExecContext(ctx, `INSERT INTO a2a888_approval_request (organization_id,name,policy_name,policy_version,requester_principal_id,action_json,intent_hash,required_approvals,execution_nonce,expires_at) VALUES ('other-tenant','bad','missing','1','user','{}','hash',1,'nonce',now() + interval '1 hour')`)
	require.Error(t, err)
}

func TestApprovalTransitionPersistsDecisionAndUnblocksWaiter(t *testing.T) {
	services, _ := requireCommandEventIntegrationStore(t)
	ctx := context.Background()
	approvalStore := NewApprovalStore(services.GetDB())
	policy := &a2a888.ApprovalPolicy{
		Name: "organizations/default/approvalPolicies/runtime", OrganizationId: "default", Version: "1",
		RequiredApprovals: 1, TimeoutSeconds: 300, OnTimeout: a2a888.ApprovalTimeoutAction_APPROVAL_TIMEOUT_ACTION_DENY, Enabled: true,
	}
	require.NoError(t, approvalStore.CreatePolicy(ctx, policy))
	request := &a2a888.ApprovalRequest{
		Name: "organizations/default/approvalRequests/runtime-1", OrganizationId: "default", PolicyName: policy.Name, PolicyVersion: "1",
		RequesterPrincipalId: "runtime-user", RequiredApprovals: 1,
		Action:    &a2a888.BoundAction{OrganizationId: "default", AgentId: "agent-1", ActionType: "SHELL", Destination: "terminal", NormalizedParametersJson: `{"command":"git status"}`},
		ExpiresAt: timestamppb.New(time.Now().Add(time.Hour)),
	}
	require.NoError(t, approvalStore.CreateRequest(ctx, request))
	updated, err := approvalStore.ApplyTransition(ctx, policy, "default", request.Name, "approver-1", "approved for the bounded test action", []string{"approver-1"}, ApprovalTransitionApprove, time.Now())
	require.NoError(t, err)
	require.Equal(t, a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_APPROVED, updated.State)
	decisions, err := approvalStore.ListDecisions(ctx, "default", request.Name)
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	waited, err := approvalStore.WaitForDecision(ctx, "default", request.Name, request.Action.IntentHash)
	require.NoError(t, err)
	require.Equal(t, ApprovalWaitAllow, waited.Decision)
}
