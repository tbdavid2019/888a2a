package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

func approvalStateFixture(now time.Time) (*a2a888.ApprovalPolicy, *a2a888.ApprovalRequest) {
	action := &a2a888.BoundAction{OrganizationId: "org-a", IntentHash: "intent-1"}
	return &a2a888.ApprovalPolicy{OrganizationId: "org-a", RequiredApprovals: 2, OnTimeout: a2a888.ApprovalTimeoutAction_APPROVAL_TIMEOUT_ACTION_DENY}, &a2a888.ApprovalRequest{
		OrganizationId: "org-a", RequiredApprovals: 2, State: a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_PENDING,
		Action: action, ExpiresAt: timestamppb.New(now.Add(time.Hour)),
	}
}

func TestApplyApprovalTransitionQuorumAndExecute(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	policy, request := approvalStateFixture(now)
	first, err := ApplyApprovalTransition(policy, request, nil, []string{"users/1", "users/2"}, "users/1", ApprovalTransitionApprove, now)
	require.NoError(t, err)
	require.Equal(t, a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_PENDING, first.Request.State)
	require.Equal(t, uint32(1), first.Request.ApprovalCount)
	second, err := ApplyApprovalTransition(policy, first.Request, []*a2a888.ApprovalDecision{{ApproverPrincipalId: "1", Outcome: a2a888.ApprovalDecisionOutcome_APPROVAL_DECISION_OUTCOME_APPROVE, IntentHash: "intent-1"}}, []string{"1", "2"}, "2", ApprovalTransitionApprove, now)
	require.NoError(t, err)
	require.Equal(t, a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_APPROVED, second.Request.State)
	executed, err := ApplyApprovalTransition(policy, second.Request, nil, nil, "", ApprovalTransitionExecute, now)
	require.NoError(t, err)
	require.Equal(t, a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_EXECUTED, executed.Request.State)
}

func TestApplyApprovalTransitionRejectsDuplicateAndMismatchedDecision(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	policy, request := approvalStateFixture(now)
	decisions := []*a2a888.ApprovalDecision{{ApproverPrincipalId: "1", IntentHash: "intent-1", Outcome: a2a888.ApprovalDecisionOutcome_APPROVAL_DECISION_OUTCOME_APPROVE}}
	_, err := ApplyApprovalTransition(policy, request, decisions, []string{"1"}, "1", ApprovalTransitionApprove, now)
	require.Error(t, err)
	decisions[0].ApproverPrincipalId = "2"
	decisions[0].IntentHash = "wrong"
	_, err = ApplyApprovalTransition(policy, request, decisions, []string{"2"}, "2", ApprovalTransitionApprove, now)
	require.Error(t, err)
}

func TestApplyApprovalTransitionTimeoutCancelSupersedeAndEscalate(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	policy, request := approvalStateFixture(now)
	request.ExpiresAt = timestamppb.New(now.Add(-time.Minute))
	denied, err := ApplyApprovalTransition(policy, request, nil, nil, "", ApprovalTransitionExpire, now)
	require.NoError(t, err)
	require.Equal(t, "EXPIRED_DENIED", denied.AuditType)

	policy.OnTimeout = a2a888.ApprovalTimeoutAction_APPROVAL_TIMEOUT_ACTION_ESCALATE
	escalated, err := ApplyApprovalTransition(policy, request, nil, nil, "", ApprovalTransitionExpire, now)
	require.NoError(t, err)
	require.True(t, escalated.EscalationRequired)
	require.Equal(t, "EXPIRED_ESCALATED", escalated.AuditType)

	policy, request = approvalStateFixture(now)
	cancelled, err := ApplyApprovalTransition(policy, request, nil, nil, "", ApprovalTransitionCancel, now)
	require.NoError(t, err)
	require.Equal(t, a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_CANCELLED, cancelled.Request.State)
	policy, request = approvalStateFixture(now)
	superseded, err := ApplyApprovalTransition(policy, request, nil, nil, "", ApprovalTransitionSupersede, now)
	require.NoError(t, err)
	require.Equal(t, a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_SUPERSEDED, superseded.Request.State)
}
