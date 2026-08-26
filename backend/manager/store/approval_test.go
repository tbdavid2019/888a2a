package store

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

func TestApprovalIntentHashIsStableAndBound(t *testing.T) {
	action := &a2a888.BoundAction{
		OrganizationId: "org-a", WorkspaceId: "ws-a", ResourceName: "payments/1",
		AgentId: "agent-a", Skill: "payments", ActionType: "CHARGE",
		Destination: "stripe", RiskLevel: a2a888.ApprovalRiskLevel_APPROVAL_RISK_LEVEL_HIGH,
		NormalizedParametersJson: `{"amount":100,"currency":"USD"}`, TaskId: "task-a", CommandId: "command-a",
	}
	hash, err := ApprovalIntentHash(action)
	if err != nil {
		t.Fatal(err)
	}
	if len(hash) != 64 {
		t.Fatalf("hash length = %d, want SHA-256 hex", len(hash))
	}
	if again, err := ApprovalIntentHash(action); err != nil || again != hash {
		t.Fatalf("hash is not stable: %q, %v", again, err)
	}
	action.Destination = "attacker.example"
	changed, err := ApprovalIntentHash(action)
	if err != nil {
		t.Fatal(err)
	}
	if changed == hash {
		t.Fatal("changing a bound parameter did not change intent hash")
	}
	first, err := ApprovalIntentHash(&a2a888.BoundAction{OrganizationId: "org-a", NormalizedParametersJson: `{"b":2,"a":1}`})
	if err != nil {
		t.Fatal(err)
	}
	second, err := ApprovalIntentHash(&a2a888.BoundAction{OrganizationId: "org-a", NormalizedParametersJson: ` { "a": 1, "b": 2 } `})
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("equivalent normalized parameters must have the same intent hash")
	}
}

func TestApprovalStoreValidatesRequestBinding(t *testing.T) {
	s := NewApprovalStore(nil)
	ctx := t.Context()
	if err := s.CreateRequest(ctx, nil); err == nil {
		t.Fatal("nil request must be rejected")
	}
	request := &a2a888.ApprovalRequest{
		OrganizationId: "org-a", Name: "requests/1", PolicyName: "payments", PolicyVersion: "1",
		RequesterPrincipalId: "principal-a", Action: &a2a888.BoundAction{OrganizationId: "org-b"},
		ExpiresAt: timestamppb.New(time.Now().Add(time.Hour)),
	}
	if err := s.CreateRequest(ctx, request); err == nil {
		t.Fatal("cross-tenant action must be rejected")
	}
}
