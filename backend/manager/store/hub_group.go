package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrHubGroupNotFound      = errors.New("Hub group not found")
	ErrHubGroupForbidden     = errors.New("operation not permitted for group")
	ErrHubGroupInvalidState  = errors.New("group state does not allow operation")
	ErrHubInvitationNotFound = errors.New("group invitation not found")
)

type HubGroupRecord struct {
	GroupID      string
	HubID        string
	Name         string
	State        string
	OwnerAgentID string
	CreatedAt    time.Time
	ArchivedAt   *time.Time
}

type HubGroupMemberRecord struct {
	HubID     string
	GroupID   string
	AgentID   string
	Role      string
	State     string
	JoinedAt  time.Time
	LeftAt    *time.Time
	RemovedAt *time.Time
}

type HubGroupInvitationRecord struct {
	ID             uint64
	HubID          string
	GroupID        string
	InviterAgentID string
	InviteeAgentID string
	State          string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	RespondedAt    *time.Time
}

type HubGroupDeliveryRecord struct {
	Sequence      uint64
	TargetAgentID string
	State         string
}

type HubGroupMessageRecord struct {
	ID             uint64
	HubID          string
	GroupID        string
	SenderAgentID  string
	ContextID      string
	IdempotencyKey string
	Message        string
	CreatedAt      time.Time
	Deliveries     []HubGroupDeliveryRecord
}

func GenerateGroupID() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "grp-" + hex.EncodeToString(raw[:]), nil
}

func (s *Store) CreateHubGroup(ctx context.Context, group HubGroupRecord) (HubGroupRecord, error) {
	if group.GroupID == "" {
		id, err := GenerateGroupID()
		if err != nil {
			return HubGroupRecord{}, err
		}
		group.GroupID = id
	}
	if group.CreatedAt.IsZero() {
		group.CreatedAt = time.Now().UTC()
	}
	group.State = "ACTIVE"

	tx, err := s.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return HubGroupRecord{}, err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
INSERT INTO a2a888_hub_group (group_id, hub_id, name, state, owner_agent_id, created_at)
VALUES ($1, $2, $3, $4, $5, $6)`,
		group.GroupID, group.HubID, group.Name, group.State, group.OwnerAgentID, group.CreatedAt)
	if err != nil {
		return HubGroupRecord{}, err
	}

	_, err = tx.ExecContext(ctx, `
INSERT INTO a2a888_hub_group_member (hub_id, group_id, agent_id, role, state, joined_at)
VALUES ($1, $2, $3, 'OWNER', 'ACTIVE', $4)`,
		group.HubID, group.GroupID, group.OwnerAgentID, group.CreatedAt)
	if err != nil {
		return HubGroupRecord{}, err
	}

	if err := tx.Commit(); err != nil {
		return HubGroupRecord{}, err
	}
	return group, nil
}

func (s *Store) FindHubGroup(ctx context.Context, groupID string) (HubGroupRecord, error) {
	var g HubGroupRecord
	var archivedAt sql.NullTime
	err := s.GetDB().QueryRowContext(ctx, `
SELECT group_id, hub_id, name, state, owner_agent_id, created_at, archived_at
FROM a2a888_hub_group WHERE group_id = $1`, groupID).Scan(
		&g.GroupID, &g.HubID, &g.Name, &g.State, &g.OwnerAgentID, &g.CreatedAt, &archivedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return HubGroupRecord{}, ErrHubGroupNotFound
		}
		return HubGroupRecord{}, err
	}
	if archivedAt.Valid {
		g.ArchivedAt = &archivedAt.Time
	}
	return g, nil
}

func (s *Store) ListHubGroups(ctx context.Context, agentID string) ([]HubGroupRecord, error) {
	rows, err := s.GetDB().QueryContext(ctx, `
SELECT g.group_id, g.hub_id, g.name, g.state, g.owner_agent_id, g.created_at, g.archived_at
FROM a2a888_hub_group g
JOIN a2a888_hub_group_member m ON m.group_id = g.group_id
WHERE m.agent_id = $1 AND m.state = 'ACTIVE' AND g.state = 'ACTIVE'
ORDER BY g.created_at DESC`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HubGroupRecord
	for rows.Next() {
		var g HubGroupRecord
		var archivedAt sql.NullTime
		if err := rows.Scan(&g.GroupID, &g.HubID, &g.Name, &g.State, &g.OwnerAgentID, &g.CreatedAt, &archivedAt); err != nil {
			return nil, err
		}
		if archivedAt.Valid {
			g.ArchivedAt = &archivedAt.Time
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) FindHubGroupMember(ctx context.Context, groupID, agentID string) (HubGroupMemberRecord, error) {
	var m HubGroupMemberRecord
	var leftAt, removedAt sql.NullTime
	err := s.GetDB().QueryRowContext(ctx, `
SELECT hub_id, group_id, agent_id, role, state, joined_at, left_at, removed_at
FROM a2a888_hub_group_member WHERE group_id = $1 AND agent_id = $2`, groupID, agentID).Scan(
		&m.HubID, &m.GroupID, &m.AgentID, &m.Role, &m.State, &m.JoinedAt, &leftAt, &removedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return HubGroupMemberRecord{}, ErrHubGroupNotFound
		}
		return HubGroupMemberRecord{}, err
	}
	if leftAt.Valid {
		m.LeftAt = &leftAt.Time
	}
	if removedAt.Valid {
		m.RemovedAt = &removedAt.Time
	}
	return m, nil
}

func (s *Store) ListHubGroupMembers(ctx context.Context, groupID string) ([]HubGroupMemberRecord, error) {
	rows, err := s.GetDB().QueryContext(ctx, `
SELECT hub_id, group_id, agent_id, role, state, joined_at, left_at, removed_at
FROM a2a888_hub_group_member WHERE group_id = $1 AND state = 'ACTIVE'
ORDER BY joined_at ASC`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HubGroupMemberRecord
	for rows.Next() {
		var m HubGroupMemberRecord
		var leftAt, removedAt sql.NullTime
		if err := rows.Scan(&m.HubID, &m.GroupID, &m.AgentID, &m.Role, &m.State, &m.JoinedAt, &leftAt, &removedAt); err != nil {
			return nil, err
		}
		if leftAt.Valid {
			m.LeftAt = &leftAt.Time
		}
		if removedAt.Valid {
			m.RemovedAt = &removedAt.Time
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) CreateHubGroupInvitation(ctx context.Context, inv HubGroupInvitationRecord) (HubGroupInvitationRecord, error) {
	var count int
	if err := s.GetDB().QueryRowContext(ctx, `SELECT count(*) FROM a2a888_hub_group_member WHERE group_id = $1 AND state = 'ACTIVE'`, inv.GroupID).Scan(&count); err != nil {
		return HubGroupInvitationRecord{}, err
	}
	if count >= 32 {
		return HubGroupInvitationRecord{}, errors.New("group member limit reached")
	}
	if inv.CreatedAt.IsZero() {
		inv.CreatedAt = time.Now().UTC()
	}
	if inv.ExpiresAt.IsZero() {
		inv.ExpiresAt = inv.CreatedAt.Add(24 * time.Hour)
	}
	inv.State = "PENDING"
	err := s.GetDB().QueryRowContext(ctx, `
INSERT INTO a2a888_hub_group_invitation (hub_id, group_id, inviter_agent_id, invitee_agent_id, state, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id`,
		inv.HubID, inv.GroupID, inv.InviterAgentID, inv.InviteeAgentID, inv.State, inv.CreatedAt, inv.ExpiresAt).Scan(&inv.ID)
	return inv, err
}

func (s *Store) FindHubGroupInvitation(ctx context.Context, id uint64) (HubGroupInvitationRecord, error) {
	var inv HubGroupInvitationRecord
	var respondedAt sql.NullTime
	err := s.GetDB().QueryRowContext(ctx, `
SELECT id, hub_id, group_id, inviter_agent_id, invitee_agent_id, state, created_at, expires_at, responded_at
FROM a2a888_hub_group_invitation WHERE id = $1`, id).Scan(
		&inv.ID, &inv.HubID, &inv.GroupID, &inv.InviterAgentID, &inv.InviteeAgentID, &inv.State, &inv.CreatedAt, &inv.ExpiresAt, &respondedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return HubGroupInvitationRecord{}, ErrHubInvitationNotFound
		}
		return HubGroupInvitationRecord{}, err
	}
	if respondedAt.Valid {
		inv.RespondedAt = &respondedAt.Time
	}
	return inv, nil
}

func (s *Store) ListHubGroupInvitations(ctx context.Context, inviteeAgentID string) ([]HubGroupInvitationRecord, error) {
	rows, err := s.GetDB().QueryContext(ctx, `
SELECT id, hub_id, group_id, inviter_agent_id, invitee_agent_id, state, created_at, expires_at, responded_at
FROM a2a888_hub_group_invitation
WHERE invitee_agent_id = $1 AND state = 'PENDING' AND expires_at > now()
ORDER BY created_at DESC`, inviteeAgentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HubGroupInvitationRecord
	for rows.Next() {
		var inv HubGroupInvitationRecord
		var respondedAt sql.NullTime
		if err := rows.Scan(&inv.ID, &inv.HubID, &inv.GroupID, &inv.InviterAgentID, &inv.InviteeAgentID, &inv.State, &inv.CreatedAt, &inv.ExpiresAt, &respondedAt); err != nil {
			return nil, err
		}
		if respondedAt.Valid {
			inv.RespondedAt = &respondedAt.Time
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

func (s *Store) AcceptHubGroupInvitation(ctx context.Context, id uint64, agentID string, at time.Time) (HubGroupMemberRecord, error) {
	tx, err := s.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return HubGroupMemberRecord{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var inv HubGroupInvitationRecord
	err = tx.QueryRowContext(ctx, `
SELECT id, hub_id, group_id, inviter_agent_id, invitee_agent_id, state, expires_at
FROM a2a888_hub_group_invitation WHERE id = $1 FOR UPDATE`, id).Scan(
		&inv.ID, &inv.HubID, &inv.GroupID, &inv.InviterAgentID, &inv.InviteeAgentID, &inv.State, &inv.ExpiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return HubGroupMemberRecord{}, ErrHubInvitationNotFound
		}
		return HubGroupMemberRecord{}, err
	}
	if inv.InviteeAgentID != agentID || inv.State != "PENDING" || !inv.ExpiresAt.After(at) {
		return HubGroupMemberRecord{}, ErrHubGroupInvalidState
	}

	var groupState string
	if err := tx.QueryRowContext(ctx, `SELECT state FROM a2a888_hub_group WHERE group_id = $1 FOR UPDATE`, inv.GroupID).Scan(&groupState); err != nil || groupState != "ACTIVE" {
		return HubGroupMemberRecord{}, ErrHubGroupInvalidState
	}

	var memberCount int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM a2a888_hub_group_member WHERE group_id = $1 AND state = 'ACTIVE' FOR UPDATE`, inv.GroupID).Scan(&memberCount); err != nil {
		return HubGroupMemberRecord{}, err
	}
	if memberCount >= 32 {
		return HubGroupMemberRecord{}, ErrHubGroupInvalidState
	}

	_, err = tx.ExecContext(ctx, `
UPDATE a2a888_hub_group_invitation SET state = 'ACCEPTED', responded_at = $2 WHERE id = $1`, id, at)
	if err != nil {
		return HubGroupMemberRecord{}, err
	}

	_, err = tx.ExecContext(ctx, `
INSERT INTO a2a888_hub_group_member (hub_id, group_id, agent_id, role, state, joined_at)
VALUES ($1, $2, $3, 'MEMBER', 'ACTIVE', $4)
ON CONFLICT (hub_id, group_id, agent_id) DO UPDATE
SET state = 'ACTIVE', role = 'MEMBER', joined_at = $4, left_at = NULL, removed_at = NULL`,
		inv.HubID, inv.GroupID, agentID, at)
	if err != nil {
		return HubGroupMemberRecord{}, err
	}

	if err := tx.Commit(); err != nil {
		return HubGroupMemberRecord{}, err
	}
	return HubGroupMemberRecord{
		HubID:    inv.HubID,
		GroupID:  inv.GroupID,
		AgentID:  agentID,
		Role:     "MEMBER",
		State:    "ACTIVE",
		JoinedAt: at,
	}, nil
}

func (s *Store) DeclineHubGroupInvitation(ctx context.Context, id uint64, agentID string, at time.Time) error {
	res, err := s.GetDB().ExecContext(ctx, `
UPDATE a2a888_hub_group_invitation SET state = 'DECLINED', responded_at = $3
WHERE id = $1 AND invitee_agent_id = $2 AND state = 'PENDING'`, id, agentID, at)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected != 1 {
		return ErrHubInvitationNotFound
	}
	return nil
}

func (s *Store) RevokeHubGroupInvitation(ctx context.Context, id uint64, inviterID string, at time.Time) error {
	res, err := s.GetDB().ExecContext(ctx, `
UPDATE a2a888_hub_group_invitation SET state = 'REVOKED', responded_at = $3
WHERE id = $1 AND inviter_agent_id = $2 AND state = 'PENDING'`, id, inviterID, at)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected != 1 {
		return ErrHubInvitationNotFound
	}
	return nil
}

func (s *Store) SendHubGroupMessage(ctx context.Context, message HubGroupMessageRecord, maxFanout int) (HubGroupMessageRecord, bool, error) {
	tx, err := s.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return HubGroupMessageRecord{}, false, err
	}
	defer func() { _ = tx.Rollback() }()

	// 1. Check idempotency
	var existing HubGroupMessageRecord
	err = tx.QueryRowContext(ctx, `
SELECT id, hub_id, group_id, sender_agent_id, context_id, idempotency_key, message, created_at
FROM a2a888_hub_group_message
WHERE group_id = $1 AND sender_agent_id = $2 AND idempotency_key = $3`,
		message.GroupID, message.SenderAgentID, message.IdempotencyKey).Scan(
		&existing.ID, &existing.HubID, &existing.GroupID, &existing.SenderAgentID,
		&existing.ContextID, &existing.IdempotencyKey, &existing.Message, &existing.CreatedAt)
	if err == nil {
		dRows, dErr := tx.QueryContext(ctx, `
SELECT sequence, target_agent_id, state FROM a2a888_hub_group_delivery WHERE group_message_id = $1`, existing.ID)
		if dErr == nil {
			defer dRows.Close()
			for dRows.Next() {
				var d HubGroupDeliveryRecord
				if err := dRows.Scan(&d.Sequence, &d.TargetAgentID, &d.State); err == nil {
					existing.Deliveries = append(existing.Deliveries, d)
				}
			}
		}
		_ = tx.Commit()
		return existing, true, nil
	}

	// 2. Validate group active
	var groupState string
	if err := tx.QueryRowContext(ctx, `SELECT state FROM a2a888_hub_group WHERE group_id = $1`, message.GroupID).Scan(&groupState); err != nil {
		return HubGroupMessageRecord{}, false, ErrHubGroupNotFound
	}
	if groupState != "ACTIVE" {
		return HubGroupMessageRecord{}, false, ErrHubGroupInvalidState
	}

	// 3. Validate sender is active member
	var memberState string
	if err := tx.QueryRowContext(ctx, `SELECT state FROM a2a888_hub_group_member WHERE group_id = $1 AND agent_id = $2`, message.GroupID, message.SenderAgentID).Scan(&memberState); err != nil || memberState != "ACTIVE" {
		return HubGroupMessageRecord{}, false, ErrHubGroupForbidden
	}

	// 4. Query active recipients
	mRows, err := tx.QueryContext(ctx, `
SELECT agent_id FROM a2a888_hub_group_member WHERE group_id = $1 AND agent_id != $2 AND state = 'ACTIVE'`, message.GroupID, message.SenderAgentID)
	if err != nil {
		return HubGroupMessageRecord{}, false, err
	}
	var recipients []string
	for mRows.Next() {
		var a string
		if err := mRows.Scan(&a); err == nil {
			recipients = append(recipients, a)
		}
	}
	mRows.Close()

	if len(recipients) == 0 || (maxFanout > 0 && len(recipients) > maxFanout) {
		return HubGroupMessageRecord{}, false, ErrHubGroupInvalidState
	}

	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now().UTC()
	}

	// 5. Insert group message
	err = tx.QueryRowContext(ctx, `
INSERT INTO a2a888_hub_group_message (hub_id, group_id, sender_agent_id, context_id, idempotency_key, message, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id`,
		message.HubID, message.GroupID, message.SenderAgentID, message.ContextID, message.IdempotencyKey, message.Message, message.CreatedAt).Scan(&message.ID)
	if err != nil {
		return HubGroupMessageRecord{}, false, err
	}

	message.Deliveries = make([]HubGroupDeliveryRecord, 0, len(recipients))

	// 6. Fan out to inbox_item and group_delivery
	for _, recipient := range recipients {
		internalKey := fmt.Sprintf("group:%s:%s", message.GroupID, message.IdempotencyKey)
		taskID := uuid.NewString()

		var seq uint64
		err := tx.QueryRowContext(ctx, `
INSERT INTO a2a888_hub_inbox (hub_id, target_agent_id, requester_agent_id, task_id, context_id, idempotency_key, message, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (hub_id, target_agent_id, requester_agent_id, idempotency_key) DO UPDATE SET message = EXCLUDED.message
RETURNING sequence`,
			message.HubID, recipient, message.SenderAgentID, taskID, message.ContextID, internalKey, message.Message, message.CreatedAt).Scan(&seq)
		if err != nil {
			return HubGroupMessageRecord{}, false, err
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO a2a888_hub_group_delivery (sequence, hub_id, group_id, group_message_id, target_agent_id, state)
VALUES ($1, $2, $3, $4, $5, 'PENDING')`,
			seq, message.HubID, message.GroupID, message.ID, recipient)
		if err != nil {
			return HubGroupMessageRecord{}, false, err
		}

		message.Deliveries = append(message.Deliveries, HubGroupDeliveryRecord{
			TargetAgentID: recipient,
			Sequence:      seq,
			State:         "PENDING",
		})
	}

	if err := tx.Commit(); err != nil {
		return HubGroupMessageRecord{}, false, err
	}
	return message, false, nil
}

func (s *Store) ListHubGroupMessages(ctx context.Context, groupID, agentID string, afterID uint64, limit int) ([]HubGroupMessageRecord, error) {
	var joinedAt time.Time
	err := s.GetDB().QueryRowContext(ctx, `
SELECT joined_at FROM a2a888_hub_group_member WHERE group_id = $1 AND agent_id = $2 AND state = 'ACTIVE'`, groupID, agentID).Scan(&joinedAt)
	if err != nil {
		return nil, ErrHubGroupForbidden
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.GetDB().QueryContext(ctx, `
SELECT id, hub_id, group_id, sender_agent_id, context_id, idempotency_key, message, created_at
FROM a2a888_hub_group_message
WHERE group_id = $1 AND id > $2 AND created_at >= $3
ORDER BY id ASC LIMIT $4`, groupID, afterID, joinedAt, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HubGroupMessageRecord
	for rows.Next() {
		var m HubGroupMessageRecord
		if err := rows.Scan(&m.ID, &m.HubID, &m.GroupID, &m.SenderAgentID, &m.ContextID, &m.IdempotencyKey, &m.Message, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) ArchiveHubGroup(ctx context.Context, groupID string, at time.Time) error {
	res, err := s.GetDB().ExecContext(ctx, `
UPDATE a2a888_hub_group SET state = 'ARCHIVED', archived_at = $2
WHERE group_id = $1 AND state = 'ACTIVE'`, groupID, at)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected != 1 {
		return ErrHubGroupNotFound
	}
	return nil
}

func (s *Store) LeaveHubGroup(ctx context.Context, groupID, agentID string, at time.Time) error {
	var role string
	err := s.GetDB().QueryRowContext(ctx, `SELECT role FROM a2a888_hub_group_member WHERE group_id = $1 AND agent_id = $2 AND state = 'ACTIVE'`, groupID, agentID).Scan(&role)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrHubGroupNotFound
		}
		return err
	}
	if role == "OWNER" {
		return ErrHubGroupForbidden
	}
	_, err = s.GetDB().ExecContext(ctx, `UPDATE a2a888_hub_group_member SET state = 'LEFT', left_at = $3 WHERE group_id = $1 AND agent_id = $2`, groupID, agentID, at)
	return err
}

func (s *Store) RemoveHubGroupMember(ctx context.Context, groupID, agentID, targetAgentID string, at time.Time) error {
	var requesterRole string
	err := s.GetDB().QueryRowContext(ctx, `SELECT role FROM a2a888_hub_group_member WHERE group_id = $1 AND agent_id = $2 AND state = 'ACTIVE'`, groupID, agentID).Scan(&requesterRole)
	if err != nil || (requesterRole != "OWNER" && requesterRole != "ADMIN") {
		return ErrHubGroupForbidden
	}
	var targetRole string
	err = s.GetDB().QueryRowContext(ctx, `SELECT role FROM a2a888_hub_group_member WHERE group_id = $1 AND agent_id = $2 AND state = 'ACTIVE'`, groupID, targetAgentID).Scan(&targetRole)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrHubGroupNotFound
		}
		return err
	}
	if targetRole == "OWNER" {
		return ErrHubGroupForbidden
	}
	_, err = s.GetDB().ExecContext(ctx, `UPDATE a2a888_hub_group_member SET state = 'REMOVED', removed_at = $3 WHERE group_id = $1 AND agent_id = $2`, groupID, targetAgentID, at)
	return err
}
