package store

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

// ApprovalTransition is an explicit lifecycle command. Callers must persist
// the returned request and audit event together; no implicit state mutation is
// performed by this pure evaluator.
type ApprovalTransition string

const (
	ApprovalTransitionApprove   ApprovalTransition = "APPROVE"
	ApprovalTransitionDeny      ApprovalTransition = "DENY"
	ApprovalTransitionExpire    ApprovalTransition = "EXPIRE"
	ApprovalTransitionCancel    ApprovalTransition = "CANCEL"
	ApprovalTransitionSupersede ApprovalTransition = "SUPERSEDE"
	ApprovalTransitionExecute   ApprovalTransition = "EXECUTE"
)

// ApprovalTransitionResult contains the next immutable request snapshot and
// the audit classification required by the persistence layer.
type ApprovalTransitionResult struct {
	Request            *a2a888.ApprovalRequest
	AuditType          string
	EscalationRequired bool
}

// ApplyApprovalTransition validates one approval lifecycle transition. It
// rechecks expiry, actor eligibility, decision intent, and quorum on every call
// so stale UI state cannot authorize an action.
func ApplyApprovalTransition(policy *a2a888.ApprovalPolicy, request *a2a888.ApprovalRequest, decisions []*a2a888.ApprovalDecision, eligible []string, actorID string, transition ApprovalTransition, now time.Time) (ApprovalTransitionResult, error) {
	if policy == nil || request == nil || policy.OrganizationId == "" || request.OrganizationId == "" || policy.OrganizationId != request.OrganizationId {
		return ApprovalTransitionResult{}, fmt.Errorf("approval policy and request organization must match")
	}
	if request.Action == nil || request.Action.IntentHash == "" {
		return ApprovalTransitionResult{}, fmt.Errorf("approval request intent binding is required")
	}
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	next, ok := proto.Clone(request).(*a2a888.ApprovalRequest)
	if !ok {
		return ApprovalTransitionResult{}, fmt.Errorf("clone approval request")
	}
	if next.State == a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_UNSPECIFIED {
		next.State = a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_PENDING
	}
	if transition != ApprovalTransitionExpire && transition != ApprovalTransitionExecute && next.State == a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_PENDING && requestExpired(next, now) {
		return ApprovalTransitionResult{}, fmt.Errorf("approval request has expired")
	}

	switch transition {
	case ApprovalTransitionApprove, ApprovalTransitionDeny:
		if next.State != a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_PENDING {
			return ApprovalTransitionResult{}, fmt.Errorf("approval decision requires a pending request")
		}
		if !containsApprovalPrincipal(eligible, actorID) {
			return ApprovalTransitionResult{}, fmt.Errorf("approver is not eligible")
		}
		for _, decision := range decisions {
			if decision != nil && normalizeApprovalPrincipal(decision.ApproverPrincipalId) == normalizeApprovalPrincipal(actorID) {
				return ApprovalTransitionResult{}, fmt.Errorf("approver has already decided")
			}
			if decision != nil && decision.IntentHash != next.Action.IntentHash {
				return ApprovalTransitionResult{}, fmt.Errorf("approval decision intent does not match request")
			}
		}
		if transition == ApprovalTransitionDeny {
			next.State = a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_DENIED
			next.TerminalReason = "approval denied"
			return ApprovalTransitionResult{Request: next, AuditType: "DECISION_DENIED"}, nil
		}
		next.ApprovalCount = uint32(countApprovals(decisions) + 1)
		required := next.RequiredApprovals
		if required == 0 {
			required = policy.RequiredApprovals
		}
		if required == 0 {
			return ApprovalTransitionResult{}, fmt.Errorf("approval quorum must be positive")
		}
		if next.ApprovalCount >= required {
			next.State = a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_APPROVED
			next.TerminalReason = "approval quorum reached"
		}
		return ApprovalTransitionResult{Request: next, AuditType: "DECISION_APPROVED"}, nil

	case ApprovalTransitionExpire:
		if next.State != a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_PENDING || !requestExpired(next, now) {
			return ApprovalTransitionResult{}, fmt.Errorf("only an expired pending request can expire")
		}
		next.State = a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_EXPIRED
		next.CompletedAt = timestamppb.New(now)
		if policy.OnTimeout == a2a888.ApprovalTimeoutAction_APPROVAL_TIMEOUT_ACTION_ESCALATE {
			next.TerminalReason = "approval expired; escalation required"
			return ApprovalTransitionResult{Request: next, AuditType: "EXPIRED_ESCALATED", EscalationRequired: true}, nil
		}
		next.TerminalReason = "approval expired"
		return ApprovalTransitionResult{Request: next, AuditType: "EXPIRED_DENIED"}, nil

	case ApprovalTransitionCancel:
		if next.State != a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_PENDING {
			return ApprovalTransitionResult{}, fmt.Errorf("only a pending request can be cancelled")
		}
		next.State = a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_CANCELLED
		next.CompletedAt = timestamppb.New(now)
		next.TerminalReason = "approval cancelled"
		return ApprovalTransitionResult{Request: next, AuditType: "CANCELLED"}, nil

	case ApprovalTransitionSupersede:
		if next.State != a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_PENDING {
			return ApprovalTransitionResult{}, fmt.Errorf("only a pending request can be superseded")
		}
		next.State = a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_SUPERSEDED
		next.CompletedAt = timestamppb.New(now)
		next.TerminalReason = "approval superseded"
		return ApprovalTransitionResult{Request: next, AuditType: "SUPERSEDED"}, nil

	case ApprovalTransitionExecute:
		if next.State != a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_APPROVED {
			return ApprovalTransitionResult{}, fmt.Errorf("only an approved request can execute")
		}
		next.State = a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_EXECUTED
		next.CompletedAt = timestamppb.New(now)
		next.TerminalReason = "approved action executed"
		return ApprovalTransitionResult{Request: next, AuditType: "EXECUTED"}, nil

	default:
		return ApprovalTransitionResult{}, fmt.Errorf("unsupported approval transition %q", transition)
	}
}

func requestExpired(request *a2a888.ApprovalRequest, now time.Time) bool {
	return request.ExpiresAt == nil || !request.ExpiresAt.IsValid() || !request.ExpiresAt.AsTime().After(now)
}

func countApprovals(decisions []*a2a888.ApprovalDecision) int {
	count := 0
	for _, decision := range decisions {
		if decision != nil && decision.Outcome == a2a888.ApprovalDecisionOutcome_APPROVAL_DECISION_OUTCOME_APPROVE {
			count++
		}
	}
	return count
}

func containsApprovalPrincipal(eligible []string, principalID string) bool {
	wanted := normalizeApprovalPrincipal(principalID)
	for _, candidate := range eligible {
		if wanted != "" && wanted == normalizeApprovalPrincipal(candidate) {
			return true
		}
	}
	return false
}
