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
	if err := tx.Commit(); err != nil {
		return Message{}, errors.Wrap(err, "commit message append")
	}
	return Message{OrganizationID: input.OrganizationID, ConversationID: input.ConversationID, MessageID: messageID.String(), ClientMessageNo: input.ClientMessageNo, MessageSeq: next, SenderID: input.SenderID, Payload: append([]byte(nil), payload...)}, nil
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

// Health checks the durable adapter connection.
func (p *PostgresPlane) Health(ctx context.Context) (Health, error) {
	if err := p.db.PingContext(ctx); err != nil {
		return Health{Healthy: false, Detail: err.Error()}, err
	}
	return Health{Healthy: true, Detail: "postgres message plane ready"}, nil
}

var _ MessagePlane = (*PostgresPlane)(nil)
