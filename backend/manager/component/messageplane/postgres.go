package messageplane

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/tbdavid2019/888a2a/backend/common"
)

// PostgresPlane is the durable application-side MessagePlane. It is an
// internal adapter; public clients never receive its database connection or
// vendor administration surface.
type PostgresPlane struct {
	db *sql.DB
}

// NewPostgresPlane creates a MessagePlane backed by PostgreSQL.
func NewPostgresPlane(db *sql.DB) (*PostgresPlane, error) {
	if db == nil {
		return nil, errors.New("message plane database is required")
	}
	return &PostgresPlane{db: db}, nil
}

func requirePlaneTenant(ctx context.Context, organizationID string) error {
	if strings.TrimSpace(organizationID) == "" {
		return errors.New("organization_id is required")
	}
	if selected, ok := common.GetOrganizationIDFromContext(ctx); ok && selected != "" && selected != organizationID {
		return errors.New("message plane tenant mismatch")
	}
	return nil
}

// IssueCredentials returns a short-lived opaque credential placeholder. The
// production WuKongIM adapter will replace the token issuance implementation;
// tenant and conversation identity remain enforced by this boundary.
func (p *PostgresPlane) IssueCredentials(ctx context.Context, organizationID, conversationID string) (ConnectionCredentials, error) {
	if err := requirePlaneTenant(ctx, organizationID); err != nil {
		return ConnectionCredentials{}, err
	}
	if strings.TrimSpace(conversationID) == "" {
		return ConnectionCredentials{}, errors.New("conversation_id is required")
	}
	return ConnectionCredentials{OrganizationID: organizationID, ConversationID: conversationID, Token: uuid.NewString()}, nil
}

// Append atomically assigns a per-Organization/per-conversation sequence and
// returns the existing message for an idempotent sender retry.
func (p *PostgresPlane) Append(ctx context.Context, input MessageInput) (Message, error) {
	if err := requirePlaneTenant(ctx, input.OrganizationID); err != nil {
		return Message{}, err
	}
	if strings.TrimSpace(input.ConversationID) == "" || strings.TrimSpace(input.ClientMessageNo) == "" || strings.TrimSpace(input.SenderID) == "" {
		return Message{}, errors.New("conversation_id, client_message_no, and sender_id are required")
	}
	payload := input.Payload
	if len(payload) == 0 {
		payload = []byte(`{}`)
	}
	if !json.Valid(payload) {
		return Message{}, errors.New("message payload must be valid JSON")
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return Message{}, errors.Wrap(err, "begin message append")
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO a2a888_message_cursor (organization_id, conversation_id)
		VALUES ($1, $2)
		ON CONFLICT (organization_id, conversation_id) DO NOTHING
	`, input.OrganizationID, input.ConversationID); err != nil {
		return Message{}, errors.Wrap(err, "create message cursor")
	}
	var existing Message
	err = tx.QueryRowContext(ctx, `
		SELECT message_id, client_msg_no, message_seq, sender_id, payload
		FROM a2a888_message
		WHERE organization_id = $1 AND conversation_id = $2 AND sender_id = $3 AND client_msg_no = $4
	`, input.OrganizationID, input.ConversationID, input.SenderID, input.ClientMessageNo).Scan(
		&existing.MessageID, &existing.ClientMessageNo, &existing.MessageSeq, &existing.SenderID, &existing.Payload,
	)
	if err == nil {
		existing.OrganizationID = input.OrganizationID
		existing.ConversationID = input.ConversationID
		return existing, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Message{}, errors.Wrap(err, "find idempotent message")
	}
	var next uint64
	if err := tx.QueryRowContext(ctx, `
		UPDATE a2a888_message_cursor
		SET next_message_seq = next_message_seq + 1
		WHERE organization_id = $1 AND conversation_id = $2
		RETURNING next_message_seq
	`, input.OrganizationID, input.ConversationID).Scan(&next); err != nil {
		return Message{}, errors.Wrap(err, "allocate message sequence")
	}
	messageID := uuid.New()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO a2a888_message (organization_id, conversation_id, message_id, client_msg_no, message_seq, sender_id, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, input.OrganizationID, input.ConversationID, messageID, input.ClientMessageNo, next, input.SenderID, payload); err != nil {
		return Message{}, errors.Wrap(err, "persist message")
	}
	message := Message{OrganizationID: input.OrganizationID, ConversationID: input.ConversationID, MessageID: messageID.String(), ClientMessageNo: input.ClientMessageNo, MessageSeq: next, SenderID: input.SenderID, Payload: payload}
	if err := projectMessageTx(ctx, tx, message); err != nil {
		return Message{}, errors.Wrap(err, "persist message projection")
	}
	if err := tx.Commit(); err != nil {
		return Message{}, errors.Wrap(err, "commit message append")
	}
	return message, nil
}

// History returns messages strictly after a tenant-bound cursor.
func (p *PostgresPlane) History(ctx context.Context, request HistoryRequest) (HistoryResponse, error) {
	if err := requirePlaneTenant(ctx, request.OrganizationID); err != nil {
		return HistoryResponse{}, err
	}
	if strings.TrimSpace(request.ConversationID) == "" || request.Limit <= 0 || request.Limit > 1000 {
		return HistoryResponse{}, errors.New("conversation_id and a bounded positive limit are required")
	}
	if request.After.OrganizationID != "" && (request.After.OrganizationID != request.OrganizationID || request.After.ConversationID != request.ConversationID) {
		return HistoryResponse{}, errors.New("message cursor tenant or conversation mismatch")
	}
	rows, err := p.db.QueryContext(ctx, `
		SELECT message_id, client_msg_no, message_seq, sender_id, payload
		FROM a2a888_message
		WHERE organization_id = $1 AND conversation_id = $2 AND message_seq > $3
		ORDER BY message_seq ASC
		LIMIT $4
	`, request.OrganizationID, request.ConversationID, request.After.MessageSeq, request.Limit)
	if err != nil {
		return HistoryResponse{}, errors.Wrap(err, "query message history")
	}
	defer rows.Close()
	response := HistoryResponse{NextCursor: request.After}
	for rows.Next() {
		var message Message
		if err := rows.Scan(&message.MessageID, &message.ClientMessageNo, &message.MessageSeq, &message.SenderID, &message.Payload); err != nil {
			return HistoryResponse{}, errors.Wrap(err, "scan message history")
		}
		message.OrganizationID = request.OrganizationID
		message.ConversationID = request.ConversationID
		response.Messages = append(response.Messages, message)
	}
	if err := rows.Err(); err != nil {
		return HistoryResponse{}, errors.Wrap(err, "iterate message history")
	}
	if len(response.Messages) > 0 {
		last := response.Messages[len(response.Messages)-1]
		response.NextCursor = Cursor{OrganizationID: last.OrganizationID, ConversationID: last.ConversationID, MessageSeq: last.MessageSeq}
	}
	return response, nil
}

// ProjectMembership upserts the tenant-scoped membership projection.
func (p *PostgresPlane) ProjectMembership(ctx context.Context, projection MembershipProjection) error {
	if err := requirePlaneTenant(ctx, projection.OrganizationID); err != nil {
		return err
	}
	if projection.ConversationID == "" || projection.PrincipalID == "" || projection.Role == "" {
		return errors.New("conversation_id, principal_id, and role are required")
	}
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO a2a888_message_membership (organization_id, conversation_id, principal_id, role)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (organization_id, conversation_id, principal_id)
		DO UPDATE SET role = EXCLUDED.role, updated_at = now()
	`, projection.OrganizationID, projection.ConversationID, projection.PrincipalID, projection.Role)
	return errors.Wrap(err, "project message membership")
}

// ReconcileConversation repairs missing or divergent message projections and
// channel memberships for one tenant-bound conversation. Unknown memberships
// are quarantined with an audit record instead of being deleted blindly.
func (p *PostgresPlane) ReconcileConversation(ctx context.Context, organizationID, conversationID string, expected []MembershipProjection) (ReconciliationReport, error) {
	if err := requirePlaneTenant(ctx, organizationID); err != nil {
		return ReconciliationReport{}, err
	}
	if strings.TrimSpace(conversationID) == "" {
		return ReconciliationReport{}, errors.New("conversation_id is required")
	}
	for _, member := range expected {
		if member.OrganizationID != organizationID || member.ConversationID != conversationID || member.PrincipalID == "" || member.Role == "" {
			return ReconciliationReport{}, errors.New("membership projection does not match reconciliation tenant")
		}
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return ReconciliationReport{}, errors.Wrap(err, "begin message reconciliation")
	}
	defer tx.Rollback()
	report := ReconciliationReport{}
	actual := make(map[string]string)
	rows, err := tx.QueryContext(ctx, `
		SELECT principal_id, role
		FROM a2a888_message_membership
		WHERE organization_id = $1 AND conversation_id = $2
	`, organizationID, conversationID)
	if err != nil {
		return report, errors.Wrap(err, "query message memberships")
	}
	for rows.Next() {
		var principalID, role string
		if err := rows.Scan(&principalID, &role); err != nil {
			_ = rows.Close()
			return report, errors.Wrap(err, "scan message membership")
		}
		actual[principalID] = role
	}
	if err := rows.Close(); err != nil {
		return report, errors.Wrap(err, "close message memberships")
	}
	expectedIDs := make(map[string]bool, len(expected))
	for _, member := range expected {
		expectedIDs[member.PrincipalID] = true
		if role, ok := actual[member.PrincipalID]; ok && role == member.Role {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO a2a888_message_membership (organization_id, conversation_id, principal_id, role)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (organization_id, conversation_id, principal_id)
			DO UPDATE SET role = EXCLUDED.role, updated_at = now()
		`, organizationID, conversationID, member.PrincipalID, member.Role); err != nil {
			return report, errors.Wrap(err, "repair message membership")
		}
		if err := recordReconciliationTx(ctx, tx, organizationID, conversationID, "MEMBERSHIP", member.PrincipalID, "REPAIRED", "missing or divergent membership"); err != nil {
			return report, err
		}
		report.Repaired++
	}
	for principalID := range actual {
		if expectedIDs[principalID] {
			continue
		}
		if err := recordReconciliationTx(ctx, tx, organizationID, conversationID, "MEMBERSHIP", principalID, "QUARANTINED", "membership is not present in authoritative projection"); err != nil {
			return report, err
		}
		report.Quarantined++
	}
	messageRows, err := tx.QueryContext(ctx, `
		SELECT message_id, conversation_id, client_msg_no, message_seq, sender_id, payload
		FROM a2a888_message
		WHERE organization_id = $1 AND conversation_id = $2
		ORDER BY message_seq ASC
	`, organizationID, conversationID)
	if err != nil {
		return report, errors.Wrap(err, "query canonical messages")
	}
	for messageRows.Next() {
		var message Message
		if err := messageRows.Scan(&message.MessageID, &message.ConversationID, &message.ClientMessageNo, &message.MessageSeq, &message.SenderID, &message.Payload); err != nil {
			_ = messageRows.Close()
			return report, errors.Wrap(err, "scan canonical message")
		}
		message.OrganizationID = organizationID
		matches, err := projectionMatchesTx(ctx, tx, message)
		if err != nil {
			_ = messageRows.Close()
			return report, err
		}
		if matches {
			continue
		}
		if err := projectMessageTx(ctx, tx, message); err != nil {
			_ = messageRows.Close()
			return report, errors.Wrap(err, "repair message projection")
		}
		if err := recordReconciliationTx(ctx, tx, organizationID, conversationID, "MESSAGE", message.MessageID, "REPAIRED", "missing or divergent message projection"); err != nil {
			_ = messageRows.Close()
			return report, err
		}
		report.Repaired++
	}
	if err := messageRows.Err(); err != nil {
		_ = messageRows.Close()
		return report, errors.Wrap(err, "iterate canonical messages")
	}
	if err := messageRows.Close(); err != nil {
		return report, errors.Wrap(err, "close canonical messages")
	}
	if err := tx.Commit(); err != nil {
		return report, errors.Wrap(err, "commit message reconciliation")
	}
	return report, nil
}

// AdvanceProjectionCursor advances a durable consumer cursor monotonically.
// ConsumerType is normally "device" or "agent"; organization and
// conversation are always part of the key so replay cannot cross tenants.
func (p *PostgresPlane) AdvanceProjectionCursor(ctx context.Context, consumerType, consumerID string, cursor Cursor) (Cursor, error) {
	if err := requirePlaneTenant(ctx, cursor.OrganizationID); err != nil {
		return Cursor{}, err
	}
	if strings.TrimSpace(cursor.ConversationID) == "" || strings.TrimSpace(consumerType) == "" || strings.TrimSpace(consumerID) == "" {
		return Cursor{}, errors.New("conversation and consumer cursor identity are required")
	}
	var sequence uint64
	err := p.db.QueryRowContext(ctx, `
		INSERT INTO a2a888_message_projection_cursor
			(organization_id, conversation_id, consumer_type, consumer_id, message_seq)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (organization_id, conversation_id, consumer_type, consumer_id) DO UPDATE
		SET message_seq = GREATEST(a2a888_message_projection_cursor.message_seq, EXCLUDED.message_seq), updated_at = now()
		RETURNING message_seq
	`, cursor.OrganizationID, cursor.ConversationID, consumerType, consumerID, cursor.MessageSeq).Scan(&sequence)
	if err != nil {
		return Cursor{}, errors.Wrap(err, "advance message projection cursor")
	}
	return Cursor{OrganizationID: cursor.OrganizationID, ConversationID: cursor.ConversationID, MessageSeq: sequence}, nil
}

// Health checks the durable adapter connection.
func (p *PostgresPlane) Health(ctx context.Context) (Health, error) {
	if err := p.db.PingContext(ctx); err != nil {
		return Health{Healthy: false, Detail: err.Error()}, err
	}
	return Health{Healthy: true, Detail: "postgres message plane ready"}, nil
}

var _ MessagePlane = (*PostgresPlane)(nil)

type ReconciliationReport struct {
	Repaired    int
	Quarantined int
}

type projectionPayload struct {
	Content      string          `json:"content"`
	Attachments  json.RawMessage `json:"attachments"`
	Mentions     json.RawMessage `json:"mentions"`
	ThreadRootID string          `json:"thread_root_id"`
	Reactions    json.RawMessage `json:"reactions"`
}

func decodeProjectionPayload(payload []byte) (projectionPayload, error) {
	var projection projectionPayload
	if err := json.Unmarshal(payload, &projection); err != nil {
		return projectionPayload{}, errors.Wrap(err, "decode message projection payload")
	}
	if len(projection.Attachments) == 0 {
		projection.Attachments = json.RawMessage(`[]`)
	}
	if len(projection.Mentions) == 0 {
		projection.Mentions = json.RawMessage(`[]`)
	}
	if len(projection.Reactions) == 0 {
		projection.Reactions = json.RawMessage(`[]`)
	}
	return projection, nil
}

func projectMessageTx(ctx context.Context, tx *sql.Tx, message Message) error {
	projection, err := decodeProjectionPayload(message.Payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO a2a888_message_projection (
			organization_id, message_id, conversation_id, client_msg_no, message_seq,
			sender_id, content, attachments, mentions, thread_root_id, reactions
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULLIF($10, ''), $11)
		ON CONFLICT (organization_id, message_id) DO UPDATE SET
			conversation_id = EXCLUDED.conversation_id,
			client_msg_no = EXCLUDED.client_msg_no,
			message_seq = EXCLUDED.message_seq,
			sender_id = EXCLUDED.sender_id,
			content = EXCLUDED.content,
			attachments = EXCLUDED.attachments,
			mentions = EXCLUDED.mentions,
			thread_root_id = EXCLUDED.thread_root_id,
			reactions = EXCLUDED.reactions
	`, message.OrganizationID, message.MessageID, message.ConversationID, message.ClientMessageNo, message.MessageSeq,
		message.SenderID, projection.Content, projection.Attachments, projection.Mentions, projection.ThreadRootID, projection.Reactions)
	return err
}

func projectionMatchesTx(ctx context.Context, tx *sql.Tx, message Message) (bool, error) {
	projection, err := decodeProjectionPayload(message.Payload)
	if err != nil {
		return false, err
	}
	var content, attachments, mentions, threadRoot, reactions string
	err = tx.QueryRowContext(ctx, `
		SELECT content, attachments::text, mentions::text, COALESCE(thread_root_id, ''), reactions::text
		FROM a2a888_message_projection
		WHERE organization_id = $1 AND message_id = $2
	`, message.OrganizationID, message.MessageID).Scan(&content, &attachments, &mentions, &threadRoot, &reactions)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, errors.Wrap(err, "query message projection")
	}
	return content == projection.Content && canonicalJSON([]byte(attachments)) == canonicalJSON(projection.Attachments) && canonicalJSON([]byte(mentions)) == canonicalJSON(projection.Mentions) && threadRoot == projection.ThreadRootID && canonicalJSON([]byte(reactions)) == canonicalJSON(projection.Reactions), nil
}

func recordReconciliationTx(ctx context.Context, tx *sql.Tx, organizationID, conversationID, resourceType, resourceID, action, detail string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO a2a888_message_reconciliation (organization_id, conversation_id, resource_type, resource_id, action, detail)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, organizationID, conversationID, resourceType, resourceID, action, detail)
	return errors.Wrap(err, "record message reconciliation")
}

func canonicalJSON(value []byte) string {
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return string(value)
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return string(value)
	}
	return string(encoded)
}
