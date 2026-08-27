package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

// ApprovalStore persists organization approval policy versions, requests and
// immutable decisions. Policy versions are never updated; publish a new
// version when the rule changes.
type ApprovalStore struct{ db *sql.DB }

func NewApprovalStore(db *sql.DB) *ApprovalStore { return &ApprovalStore{db: db} }

// ApprovalIntentHash returns the SHA-256 digest of the canonical action
// binding. The supplied intent_hash field is deliberately excluded.
func ApprovalIntentHash(action *a2a888.BoundAction) (string, error) {
	if action == nil {
		return "", errors.New("approval action is required")
	}
	parameters := action.NormalizedParametersJson
	if parameters == "" && action.NormalizedParameters != nil {
		var err error
		parametersBytes, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(action.NormalizedParameters)
		if err != nil {
			return "", errors.Wrap(err, "marshal normalized approval parameters")
		}
		parameters = string(parametersBytes)
	}
	if parameters != "" {
		var value any
		if err := json.Unmarshal([]byte(parameters), &value); err != nil {
			return "", errors.Wrap(err, "normalized approval parameters must be valid JSON")
		}
		canonicalParameters, err := json.Marshal(value)
		if err != nil {
			return "", errors.Wrap(err, "canonicalize normalized approval parameters")
		}
		parameters = string(canonicalParameters)
	}
	material := struct {
		OrganizationID string `json:"organization_id"`
		WorkspaceID    string `json:"workspace_id"`
		ResourceName   string `json:"resource_name"`
		AgentID        string `json:"agent_id"`
		Skill          string `json:"skill"`
		ActionType     string `json:"action_type"`
		Destination    string `json:"destination"`
		RiskLevel      int32  `json:"risk_level"`
		Parameters     string `json:"normalized_parameters_json"`
		TaskID         string `json:"task_id"`
		CommandID      string `json:"command_id"`
	}{action.OrganizationId, action.WorkspaceId, action.ResourceName, action.AgentId, action.Skill, action.ActionType, action.Destination, int32(action.RiskLevel), parameters, action.TaskId, action.CommandId}
	b, err := json.Marshal(material)
	if err != nil {
		return "", errors.Wrap(err, "marshal approval intent")
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func (s *ApprovalStore) CreatePolicy(ctx context.Context, policy *a2a888.ApprovalPolicy) error {
	if policy == nil || policy.OrganizationId == "" || policy.Name == "" || policy.Version == "" {
		return errors.New("approval policy organization_id, name, and version are required")
	}
	if policy.RequiredApprovals == 0 || policy.TimeoutSeconds == 0 {
		return errors.New("approval policy required_approvals and timeout_seconds must be positive")
	}
	if s == nil || s.db == nil {
		return errors.New("approval store database is required")
	}
	onTimeout := approvalTimeoutDB(policy.OnTimeout)
	if onTimeout == "" {
		return errors.New("approval policy on_timeout is required")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO a2a888_approval_policy (organization_id, name, enabled) VALUES ($1, $2, $3) ON CONFLICT (organization_id, name) DO UPDATE SET enabled = EXCLUDED.enabled, updated_at = now()`, policy.OrganizationId, policy.Name, policy.Enabled)
	if err != nil {
		return errors.Wrap(err, "create approval policy")
	}
	approverPrincipalIDs := nonNilStrings(policy.ApproverPrincipalIds)
	approverGroupIDs := nonNilStrings(policy.ApproverGroupIds)
	approverRoles := nonNilStrings(policy.ApproverRoles)
	escalationPrincipalIDs := nonNilStrings(policy.EscalationPrincipalIds)
	escalationGroupIDs := nonNilStrings(policy.EscalationGroupIds)
	escalationRoles := nonNilStrings(policy.EscalationRoles)
	_, err = s.db.ExecContext(ctx, `INSERT INTO a2a888_approval_policy_version (organization_id, policy_name, version, workspace_id, resource_pattern, agent_id, skill, action_type, destination_pattern, requester_class, risk_level, approver_principal_ids, approver_group_ids, approver_roles, required_approvals, timeout_seconds, on_timeout, escalation_principal_ids, escalation_group_ids, escalation_roles, prohibit_requester_approval, prohibit_agent_owner_sole_approval) VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)`, policy.OrganizationId, policy.Name, policy.Version, policy.WorkspaceId, policy.ResourcePattern, policy.AgentId, policy.Skill, policy.ActionType, policy.DestinationPattern, policy.RequesterClass, int32(policy.RiskLevel), pq.Array(approverPrincipalIDs), pq.Array(approverGroupIDs), pq.Array(approverRoles), policy.RequiredApprovals, policy.TimeoutSeconds, onTimeout, pq.Array(escalationPrincipalIDs), pq.Array(escalationGroupIDs), pq.Array(escalationRoles), policy.ProhibitRequesterApproval, policy.ProhibitAgentOwnerSoleApproval)
	if err != nil {
		return errors.Wrap(err, "create approval policy version")
	}
	return nil
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func (s *ApprovalStore) CreateRequest(ctx context.Context, request *a2a888.ApprovalRequest) error {
	if request == nil || request.OrganizationId == "" || request.Name == "" || request.PolicyName == "" || request.PolicyVersion == "" || request.RequesterPrincipalId == "" || request.Action == nil || request.RequiredApprovals == 0 {
		return errors.New("approval request identity, policy, requester, and action are required")
	}
	if request.ExpiresAt == nil || !request.ExpiresAt.IsValid() || !request.ExpiresAt.AsTime().After(time.Now()) {
		return errors.New("approval request expiry must be in the future")
	}
	hash, err := ApprovalIntentHash(request.Action)
	if err != nil {
		return err
	}
	if request.Action.OrganizationId != request.OrganizationId {
		return errors.New("approval request action intent hash or organization does not match")
	}
	if request.Action.IntentHash != "" && request.Action.IntentHash != hash {
		return errors.New("approval request action intent hash or organization does not match")
	}
	request.Action.IntentHash = hash
	nonce := request.ExecutionNonce
	if nonce == "" {
		nonce, err = randomApprovalNonce()
		if err != nil {
			return err
		}
		request.ExecutionNonce = nonce
	}
	actionJSON, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(request.Action)
	if err != nil {
		return errors.Wrap(err, "marshal approval action")
	}
	state := approvalRequestStateDB(request.State)
	if state == "" {
		state = "PENDING"
	}
	if s == nil || s.db == nil {
		return errors.New("approval store database is required")
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO a2a888_approval_request (organization_id,name,workspace_id,policy_name,policy_version,requester_principal_id,executing_agent_id,executing_agent_owner_id,action_json,intent_hash,state,required_approvals,execution_nonce,expires_at) VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) ON CONFLICT (organization_id,name) DO NOTHING`, request.OrganizationId, request.Name, request.WorkspaceId, request.PolicyName, request.PolicyVersion, request.RequesterPrincipalId, request.ExecutingAgentId, request.ExecutingAgentOwnerId, actionJSON, hash, state, request.RequiredApprovals, nonce, request.ExpiresAt.AsTime())
	if err != nil {
		return errors.Wrap(err, "create approval request")
	}
	return nil
}

func (s *ApprovalStore) CreateDecision(ctx context.Context, decision *a2a888.ApprovalDecision) error {
	if decision == nil || decision.OrganizationId == "" || decision.Name == "" || decision.RequestName == "" || decision.ApproverPrincipalId == "" || decision.IntentHash == "" {
		return errors.New("approval decision identity and intent_hash are required")
	}
	outcome := approvalDecisionOutcomeDB(decision.Outcome)
	if outcome == "" {
		return errors.New("approval decision outcome is required")
	}
	if s == nil || s.db == nil {
		return errors.New("approval store database is required")
	}
	var requestIntentHash string
	err := s.db.QueryRowContext(ctx, `
		SELECT intent_hash FROM a2a888_approval_request
		WHERE organization_id = $1 AND name = $2
	`, decision.OrganizationId, decision.RequestName).Scan(&requestIntentHash)
	if err != nil {
		return errors.Wrap(err, "resolve approval request intent hash")
	}
	if decision.IntentHash != requestIntentHash {
		return errors.New("approval decision intent_hash does not match request")
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO a2a888_approval_decision (organization_id,name,request_name,approver_principal_id,approver_role,outcome,reason,intent_hash) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, decision.OrganizationId, decision.Name, decision.RequestName, decision.ApproverPrincipalId, decision.ApproverRole, outcome, decision.Reason, decision.IntentHash)
	if err != nil {
		return errors.Wrap(err, "create approval decision")
	}
	return nil
}

// GetRequest reads one tenant-scoped approval request, including its bound
// action. The organization is always part of the lookup to prevent tenant
// enumeration through request names.
func (s *ApprovalStore) GetRequest(ctx context.Context, organizationID, name string) (*a2a888.ApprovalRequest, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("approval store database is required")
	}
	if organizationID == "" || name == "" {
		return nil, errors.New("approval request organization_id and name are required")
	}
	var (
		request                         a2a888.ApprovalRequest
		workspaceID, executingAgentID   sql.NullString
		executingAgentOwnerID           sql.NullString
		actionJSON                      []byte
		state                           string
		expiresAt, createdAt, updatedAt time.Time
		completedAt                     sql.NullTime
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT name, workspace_id, policy_name, policy_version,
		       requester_principal_id, executing_agent_id, executing_agent_owner_id,
		       action_json, state, required_approvals, approval_count, execution_nonce,
		       expires_at, created_at, updated_at, completed_at, terminal_reason
		FROM a2a888_approval_request
		WHERE organization_id = $1 AND name = $2
	`, organizationID, name).Scan(
		&request.Name, &workspaceID, &request.PolicyName, &request.PolicyVersion,
		&request.RequesterPrincipalId, &executingAgentID, &executingAgentOwnerID,
		&actionJSON, &state, &request.RequiredApprovals, &request.ApprovalCount,
		&request.ExecutionNonce, &expiresAt, &createdAt, &updatedAt, &completedAt,
		&request.TerminalReason,
	)
	if err != nil {
		return nil, errors.Wrap(err, "get approval request")
	}
	request.OrganizationId = organizationID
	request.WorkspaceId = workspaceID.String
	request.ExecutingAgentId = executingAgentID.String
	request.ExecutingAgentOwnerId = executingAgentOwnerID.String
	request.State = parseApprovalRequestState(state)
	request.ExpiresAt = timestamppb.New(expiresAt)
	request.CreatedAt = timestamppb.New(createdAt)
	request.UpdatedAt = timestamppb.New(updatedAt)
	if completedAt.Valid {
		request.CompletedAt = timestamppb.New(completedAt.Time)
	}
	request.Action = &a2a888.BoundAction{}
	if err := protojson.Unmarshal(actionJSON, request.Action); err != nil {
		return nil, errors.Wrap(err, "decode approval request action")
	}
	return &request, nil
}

// ListDecisions returns immutable decisions for one tenant-scoped request in
// creation order. It is used by the state evaluator before every transition.
func (s *ApprovalStore) ListDecisions(ctx context.Context, organizationID, requestName string) ([]*a2a888.ApprovalDecision, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("approval store database is required")
	}
	if organizationID == "" || requestName == "" {
		return nil, errors.New("approval decision organization_id and request_name are required")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT name, approver_principal_id, approver_role, outcome, reason, intent_hash, created_at
		FROM a2a888_approval_decision
		WHERE organization_id = $1 AND request_name = $2
		ORDER BY created_at, name
	`, organizationID, requestName)
	if err != nil {
		return nil, errors.Wrap(err, "list approval decisions")
	}
	defer rows.Close()
	var decisions []*a2a888.ApprovalDecision
	for rows.Next() {
		var decision a2a888.ApprovalDecision
		var outcome string
		var createdAt time.Time
		if err := rows.Scan(&decision.Name, &decision.ApproverPrincipalId, &decision.ApproverRole, &outcome, &decision.Reason, &decision.IntentHash, &createdAt); err != nil {
			return nil, errors.Wrap(err, "scan approval decision")
		}
		decision.OrganizationId = organizationID
		decision.RequestName = requestName
		decision.Outcome = parseApprovalDecisionOutcome(outcome)
		decision.CreatedAt = timestamppb.New(createdAt)
		decisions = append(decisions, &decision)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, "iterate approval decisions")
	}
	return decisions, nil
}

// ApplyTransition evaluates and persists an approval transition atomically.
// For decisions, the immutable decision row and request lifecycle update are
// committed together so a crashed Manager cannot acknowledge only one side.
func (s *ApprovalStore) ApplyTransition(ctx context.Context, policy *a2a888.ApprovalPolicy, organizationID, requestName, actorID, reason string, eligible []string, transition ApprovalTransition, now time.Time) (*a2a888.ApprovalRequest, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("approval store database is required")
	}
	request, err := s.GetRequest(ctx, organizationID, requestName)
	if err != nil {
		return nil, err
	}
	decisions, err := s.ListDecisions(ctx, organizationID, requestName)
	if err != nil {
		return nil, err
	}
	result, err := ApplyApprovalTransition(policy, request, decisions, eligible, actorID, transition, now)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "begin approval transition")
	}
	defer func() { _ = tx.Rollback() }()
	var currentState string
	if err := tx.QueryRowContext(ctx, `SELECT state FROM a2a888_approval_request WHERE organization_id = $1 AND name = $2 FOR UPDATE`, organizationID, requestName).Scan(&currentState); err != nil {
		return nil, errors.Wrap(err, "lock approval request")
	}
	if currentState != approvalRequestStateDB(request.State) {
		return nil, errors.New("approval request changed during transition")
	}
	if transition == ApprovalTransitionApprove || transition == ApprovalTransitionDeny {
		outcome := a2a888.ApprovalDecisionOutcome_APPROVAL_DECISION_OUTCOME_APPROVE
		if transition == ApprovalTransitionDeny {
			outcome = a2a888.ApprovalDecisionOutcome_APPROVAL_DECISION_OUTCOME_DENY
		}
		decisionName := fmt.Sprintf("%s/decisions/%x", requestName, sha256.Sum256([]byte(actorID)))
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO a2a888_approval_decision (organization_id,name,request_name,approver_principal_id,approver_role,outcome,reason,intent_hash)
			VALUES ($1,$2,$3,$4,'',$5,$6,$7)
		`, organizationID, decisionName, requestName, actorID, approvalDecisionOutcomeDB(outcome), reason, request.Action.IntentHash); err != nil {
			return nil, errors.Wrap(err, "persist approval decision")
		}
	}
	state := approvalRequestStateDB(result.Request.State)
	if state == "" {
		return nil, errors.New("approval transition produced an invalid state")
	}
	var completedAt any
	if result.Request.CompletedAt != nil && result.Request.CompletedAt.IsValid() {
		completedAt = result.Request.CompletedAt.AsTime()
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE a2a888_approval_request
		SET state = $3, approval_count = $4, completed_at = $5, terminal_reason = $6, updated_at = now()
		WHERE organization_id = $1 AND name = $2
	`, organizationID, requestName, state, result.Request.ApprovalCount, completedAt, result.Request.TerminalReason); err != nil {
		return nil, errors.Wrap(err, "persist approval lifecycle")
	}
	if err := tx.Commit(); err != nil {
		return nil, errors.Wrap(err, "commit approval transition")
	}
	return result.Request, nil
}

func parseApprovalRequestState(value string) a2a888.ApprovalRequestState {
	return a2a888.ApprovalRequestState(a2a888.ApprovalRequestState_value["APPROVAL_REQUEST_STATE_"+value])
}

func parseApprovalDecisionOutcome(value string) a2a888.ApprovalDecisionOutcome {
	return a2a888.ApprovalDecisionOutcome(a2a888.ApprovalDecisionOutcome_value["APPROVAL_DECISION_OUTCOME_"+value])
}

func randomApprovalNonce() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", errors.Wrap(err, "generate approval execution nonce")
	}
	return hex.EncodeToString(b), nil
}

func approvalTimeoutDB(v a2a888.ApprovalTimeoutAction) string {
	if v == a2a888.ApprovalTimeoutAction_APPROVAL_TIMEOUT_ACTION_ESCALATE {
		return "ESCALATE"
	}
	if v == a2a888.ApprovalTimeoutAction_APPROVAL_TIMEOUT_ACTION_DENY {
		return "DENY"
	}
	return ""
}

func approvalRequestStateDB(v a2a888.ApprovalRequestState) string {
	if v == 0 {
		return ""
	}
	return strings.TrimPrefix(v.String(), "APPROVAL_REQUEST_STATE_")
}

func approvalDecisionOutcomeDB(v a2a888.ApprovalDecisionOutcome) string {
	if v == 0 {
		return ""
	}
	return strings.TrimPrefix(v.String(), "APPROVAL_DECISION_OUTCOME_")
}
