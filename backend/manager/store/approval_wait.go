package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/pkg/errors"
	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

const defaultApprovalPollInterval = 250 * time.Millisecond

// ApprovalBinding supplies the durable identity needed to bind an external
// permission request to one organization approval policy.
type ApprovalBinding struct {
	OrganizationID      string
	WorkspaceID         string
	RequesterPrincipal  string
	ExecutingAgentID    string
	ExecutingAgentOwner string
	PolicyName          string
	PolicyVersion       string
	RequiredApprovals   uint32
	Timeout             time.Duration
}

// ApprovalRequestName returns a stable name for one action intent. The action
// hash is part of the name so changed parameters cannot reuse an old decision.
func ApprovalRequestName(binding ApprovalBinding, intentHash string) string {
	seed := binding.OrganizationID + "\x00" + binding.RequesterPrincipal + "\x00" + binding.PolicyName + "\x00" + intentHash
	sum := sha256.Sum256([]byte(seed))
	return "organizations/" + binding.OrganizationID + "/approvalRequests/" + hex.EncodeToString(sum[:])
}

type ApprovalWaitDecision string

const (
	ApprovalWaitAllow ApprovalWaitDecision = "ALLOW"
	ApprovalWaitDeny  ApprovalWaitDecision = "DENY"
)

type ApprovalWaitResult struct {
	Decision ApprovalWaitDecision
	Reason   string
}

// WaitForDecision waits on durable state, so a Manager restart or a second
// replica can resume the same approval request. The request identity and
// intent hash are checked on every poll to prevent confused-deputy reuse.
func (s *ApprovalStore) WaitForDecision(ctx context.Context, organizationID, requestName, intentHash string) (ApprovalWaitResult, error) {
	if s == nil || s.db == nil {
		return ApprovalWaitResult{Decision: ApprovalWaitDeny, Reason: "approval store is not configured"}, errors.New("approval store is not configured")
	}
	if organizationID == "" || requestName == "" || intentHash == "" {
		return ApprovalWaitResult{Decision: ApprovalWaitDeny, Reason: "approval wait identity is incomplete"}, errors.New("approval wait identity is incomplete")
	}
	ticker := time.NewTicker(defaultApprovalPollInterval)
	defer ticker.Stop()
	for {
		request, err := s.GetRequest(ctx, organizationID, requestName)
		if err != nil {
			return ApprovalWaitResult{Decision: ApprovalWaitDeny, Reason: "approval request could not be read"}, err
		}
		if request.Action == nil || request.Action.IntentHash != intentHash {
			return ApprovalWaitResult{Decision: ApprovalWaitDeny, Reason: "approval intent changed"}, errors.New("approval intent changed")
		}
		switch request.State {
		case a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_APPROVED:
			return ApprovalWaitResult{Decision: ApprovalWaitAllow, Reason: "durable approval quorum reached"}, nil
		case a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_DENIED,
			a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_EXPIRED,
			a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_CANCELLED,
			a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_SUPERSEDED,
			a2a888.ApprovalRequestState_APPROVAL_REQUEST_STATE_EXECUTED:
			reason := request.TerminalReason
			if reason == "" {
				reason = "approval did not authorize the action"
			}
			return ApprovalWaitResult{Decision: ApprovalWaitDeny, Reason: reason}, nil
		}

		select {
		case <-ctx.Done():
			return ApprovalWaitResult{Decision: ApprovalWaitDeny, Reason: "approval wait cancelled"}, ctx.Err()
		case <-ticker.C:
		}
	}
}
