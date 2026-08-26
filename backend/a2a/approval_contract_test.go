package a2a

import (
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	a2a888pb "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

func TestApprovalContractsRoundTrip(t *testing.T) {
	now := timestamppb.Now()
	request := &a2a888pb.ApprovalRequest{
		Name:                  "organizations/acme/approvalRequests/req-1",
		OrganizationId:        "acme",
		WorkspaceId:           "workspace-1",
		PolicyName:            "organizations/acme/approvalPolicies/prod-deploy",
		PolicyVersion:         "3",
		RequesterPrincipalId:  "agent-requester",
		ExecutingAgentId:      "agent-deployer",
		ExecutingAgentOwnerId: "user-owner",
		State:                 a2a888pb.ApprovalRequestState_APPROVAL_REQUEST_STATE_PENDING,
		RequiredApprovals:     2,
		ExecutionNonce:        "nonce-1",
		ExpiresAt:             now,
		CreatedAt:             now,
		Action: &a2a888pb.BoundAction{
			OrganizationId:           "acme",
			WorkspaceId:              "workspace-1",
			ResourceName:             "services/payments",
			AgentId:                  "agent-deployer",
			Skill:                    "release",
			ActionType:               "deploy",
			Destination:              "production",
			RiskLevel:                a2a888pb.ApprovalRiskLevel_APPROVAL_RISK_LEVEL_CRITICAL,
			NormalizedParameters:     mustStruct(t, map[string]any{"version": "v2.1"}),
			NormalizedParametersJson: `{"version":"v2.1"}`,
			IntentHash:               "sha256:fixed-intent",
			TaskId:                   "task-1",
			CommandId:                "command-1",
		},
	}

	encoded, err := protojson.Marshal(request)
	if err != nil {
		t.Fatalf("encode approval request: %v", err)
	}
	roundTrip := &a2a888pb.ApprovalRequest{}
	if err := protojson.Unmarshal(encoded, roundTrip); err != nil {
		t.Fatalf("decode approval request: %v", err)
	}
	if !proto.Equal(request, roundTrip) {
		t.Fatal("approval request changed during proto JSON round trip")
	}
}

func TestApprovalRequestStateMachine(t *testing.T) {
	states := []a2a888pb.ApprovalRequestState{
		a2a888pb.ApprovalRequestState_APPROVAL_REQUEST_STATE_PENDING,
		a2a888pb.ApprovalRequestState_APPROVAL_REQUEST_STATE_APPROVED,
		a2a888pb.ApprovalRequestState_APPROVAL_REQUEST_STATE_DENIED,
		a2a888pb.ApprovalRequestState_APPROVAL_REQUEST_STATE_EXPIRED,
		a2a888pb.ApprovalRequestState_APPROVAL_REQUEST_STATE_CANCELLED,
		a2a888pb.ApprovalRequestState_APPROVAL_REQUEST_STATE_SUPERSEDED,
		a2a888pb.ApprovalRequestState_APPROVAL_REQUEST_STATE_EXECUTED,
	}
	transitions := map[a2a888pb.ApprovalRequestState]map[a2a888pb.ApprovalRequestState]bool{
		states[0]: {
			states[1]: true,
			states[2]: true,
			states[3]: true,
			states[4]: true,
			states[5]: true,
		},
		states[1]: {states[6]: true},
	}
	for _, from := range states {
		for _, to := range states {
			allowed := transitions[from]
			wantAllowed := false
			if allowed != nil {
				wantAllowed = allowed[to]
			}
			if got := approvalTransitionAllowed(from, to); got != wantAllowed {
				t.Errorf("transition %s -> %s = %t, want %t", from, to, got, wantAllowed)
			}
		}
	}
}

func approvalTransitionAllowed(from, to a2a888pb.ApprovalRequestState) bool {
	switch from {
	case a2a888pb.ApprovalRequestState_APPROVAL_REQUEST_STATE_PENDING:
		return to == a2a888pb.ApprovalRequestState_APPROVAL_REQUEST_STATE_APPROVED ||
			to == a2a888pb.ApprovalRequestState_APPROVAL_REQUEST_STATE_DENIED ||
			to == a2a888pb.ApprovalRequestState_APPROVAL_REQUEST_STATE_EXPIRED ||
			to == a2a888pb.ApprovalRequestState_APPROVAL_REQUEST_STATE_CANCELLED ||
			to == a2a888pb.ApprovalRequestState_APPROVAL_REQUEST_STATE_SUPERSEDED
	case a2a888pb.ApprovalRequestState_APPROVAL_REQUEST_STATE_APPROVED:
		return to == a2a888pb.ApprovalRequestState_APPROVAL_REQUEST_STATE_EXECUTED
	default:
		return false
	}
}

func mustStruct(t *testing.T, values map[string]any) *structpb.Struct {
	t.Helper()
	result, err := structpb.NewStruct(values)
	if err != nil {
		t.Fatalf("build normalized parameters: %v", err)
	}
	return result
}
