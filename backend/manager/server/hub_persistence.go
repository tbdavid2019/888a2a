package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	a2agateway "github.com/tbdavid2019/888a2a/backend/a2a"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

type hubStorePersistence struct {
	store *store.Store
}

func (p hubStorePersistence) ListHubAgents(ctx context.Context, hubID string) ([]a2agateway.HubAgentRecord, error) {
	agents, err := p.store.ListHubAgents(ctx, hubID)
	if err != nil {
		return nil, err
	}
	records := make([]a2agateway.HubAgentRecord, 0, len(agents))
	for _, agent := range agents {
		var capabilities []string
		if err := json.Unmarshal([]byte(agent.CapabilitiesJSON), &capabilities); err != nil {
			return nil, err
		}
		records = append(records, a2agateway.HubAgentRecord{
			RegisteredAgent: a2agateway.RegisteredAgent{
				HubID: agent.HubID, AgentID: agent.AgentID, DisplayName: agent.DisplayName,
				ProviderFamily: agent.ProviderFamily, TransportID: agent.TransportID,
				Capabilities: capabilities, AgentCardJSON: agent.AgentCardJSON, State: a2agateway.HubAgentState(agent.State),
				AutomaticExecution: agent.AutomaticExecution, CreatedAt: agent.CreatedAt, LastSeenAt: agent.LastSeenAt.Time,
				ExpiresAt: agent.ExpiresAt, LeaseExpiresAt: agent.LeaseExpiresAt, RevokedAt: nullableTime(agent.RevokedAt), RevokeReason: agent.RevokeReason,
			},
			RegistrationHash: agent.RegistrationKeyHash, TokenHash: agent.AgentTokenHash,
		})
	}
	return records, nil
}

func (p hubStorePersistence) SaveHubAgent(ctx context.Context, record a2agateway.HubAgentRecord) error {
	capabilities, err := json.Marshal(record.Capabilities)
	if err != nil {
		return err
	}
	return p.store.CreateHubAgent(ctx, &store.HubAgentMessage{
		HubID: record.HubID, AgentID: record.AgentID, RegistrationKeyHash: record.RegistrationHash,
		AgentTokenHash: record.TokenHash, DisplayName: record.DisplayName, ProviderFamily: record.ProviderFamily,
		TransportID: record.TransportID, CapabilitiesJSON: string(capabilities), AgentCardJSON: record.AgentCardJSON,
		State: string(record.State), AutomaticExecution: record.AutomaticExecution,
		ExpiresAt: record.ExpiresAt, LeaseExpiresAt: record.LeaseExpiresAt,
	})
}

func (p hubStorePersistence) HeartbeatHubAgent(ctx context.Context, hubID, agentID, tokenHash string, now time.Time, lease time.Duration) error {
	return p.store.HeartbeatHubAgent(ctx, hubID, agentID, tokenHash, now, lease)
}

func (p hubStorePersistence) RotateHubAgent(ctx context.Context, hubID, agentID, tokenHash string, now time.Time) error {
	return p.store.RotateHubAgent(ctx, hubID, agentID, tokenHash, now)
}

func (p hubStorePersistence) DisconnectHubAgent(ctx context.Context, hubID, agentID string, now time.Time) error {
	return p.store.DisconnectHubAgent(ctx, hubID, agentID, now)
}

func (p hubStorePersistence) RevokeHubAgent(ctx context.Context, hubID, agentID, reason string, now time.Time) error {
	return p.store.RevokeHubAgent(ctx, hubID, agentID, reason, now)
}

func (p hubStorePersistence) UpdateHubPolicy(ctx context.Context, hubID, mode string, registrationEnabled, publicConfirmed bool) error {
	return p.store.UpdateHubPolicy(ctx, hubID, mode, registrationEnabled, publicConfirmed)
}

func (p hubStorePersistence) Enqueue(ctx context.Context, item a2agateway.HubInboxItem) (a2agateway.HubInboxEnqueueResult, error) {
	stored, duplicate, err := p.store.CreateHubInboxItem(ctx, &store.HubInboxMessage{
		HubID: item.HubID, TargetAgentID: item.TargetAgentID, RequesterAgentID: item.RequesterAgentID,
		TaskID: item.TaskID, ContextID: item.ContextID, IdempotencyKey: item.IdempotencyKey, Message: item.Message,
	})
	if err != nil {
		return a2agateway.HubInboxEnqueueResult{}, err
	}
	return a2agateway.HubInboxEnqueueResult{Item: convertHubInboxItem(stored), Duplicate: duplicate}, nil
}

func (p hubStorePersistence) Find(ctx context.Context, hubID, targetAgentID, requesterAgentID, idempotencyKey string) (a2agateway.HubInboxItem, bool, error) {
	item, err := p.store.FindHubInboxItem(ctx, hubID, targetAgentID, requesterAgentID, idempotencyKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return a2agateway.HubInboxItem{}, false, nil
		}
		return a2agateway.HubInboxItem{}, false, err
	}
	return convertHubInboxItem(item), true, nil
}

func (p hubStorePersistence) PendingCount(ctx context.Context, hubID string) (int, error) {
	return p.store.PendingHubInboxCount(ctx, hubID)
}

func (p hubStorePersistence) Poll(ctx context.Context, hubID, targetAgentID string, afterSequence uint64, limit int) ([]a2agateway.HubInboxItem, error) {
	items, err := p.store.ListHubInbox(ctx, hubID, targetAgentID, afterSequence, limit)
	if err != nil {
		return nil, err
	}
	out := make([]a2agateway.HubInboxItem, 0, len(items))
	for _, item := range items {
		out = append(out, convertHubInboxItem(item))
	}
	return out, nil
}

func (p hubStorePersistence) Acknowledge(ctx context.Context, hubID, targetAgentID string, sequence uint64) error {
	return p.store.AcknowledgeHubInbox(ctx, hubID, targetAgentID, sequence, time.Now().UTC())
}

func (p hubStorePersistence) Cancel(ctx context.Context, hubID, taskID string, now time.Time) error {
	return p.store.CancelHubInbox(ctx, hubID, taskID, now)
}

func (p hubStorePersistence) ListMessagesAdmin(ctx context.Context, hubID, agentID string, beforeSequence uint64, limit int) ([]a2agateway.HubInboxAdminItem, error) {
	items, err := p.store.ListHubInboxAdmin(ctx, hubID, agentID, beforeSequence, limit)
	if err != nil {
		return nil, err
	}
	out := make([]a2agateway.HubInboxAdminItem, 0, len(items))
	for _, item := range items {
		var acknowledgedAt *time.Time
		if item.AcknowledgedAt.Valid {
			acknowledgedAt = &item.AcknowledgedAt.Time
		}
		state := item.State
		if state == "" {
			state = "PENDING"
			if acknowledgedAt != nil {
				state = "ACKNOWLEDGED"
			}
		}
		out = append(out, a2agateway.HubInboxAdminItem{
			Sequence:         item.Sequence,
			HubID:            item.HubID,
			TargetAgentID:    item.TargetAgentID,
			RequesterAgentID: item.RequesterAgentID,
			TaskID:           item.TaskID,
			ContextID:        item.ContextID,
			IdempotencyKey:   item.IdempotencyKey,
			Message:          item.Message,
			State:            state,
			CreatedAt:        item.CreatedAt,
			AcknowledgedAt:   acknowledgedAt,
		})
	}
	return out, nil
}

func convertHubInboxItem(item *store.HubInboxMessage) a2agateway.HubInboxItem {
	if item == nil {
		return a2agateway.HubInboxItem{}
	}
	var acknowledgedAt *time.Time
	if item.AcknowledgedAt.Valid {
		acknowledgedAt = &item.AcknowledgedAt.Time
	}
	return a2agateway.HubInboxItem{
		Sequence: item.Sequence, HubID: item.HubID, TargetAgentID: item.TargetAgentID,
		RequesterAgentID: item.RequesterAgentID, TaskID: item.TaskID, ContextID: item.ContextID,
		IdempotencyKey: item.IdempotencyKey, Message: item.Message, CreatedAt: item.CreatedAt, AcknowledgedAt: acknowledgedAt,
	}
}

func nullableTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

func (p hubStorePersistence) CreateGroup(ctx context.Context, group a2agateway.HubGroup) (a2agateway.HubGroup, error) {
	rec, err := p.store.CreateHubGroup(ctx, store.HubGroupRecord{
		GroupID:      group.GroupID,
		HubID:        group.HubID,
		Name:         group.Name,
		OwnerAgentID: group.OwnerAgentID,
		CreatedAt:    group.CreatedAt,
	})
	if err != nil {
		return a2agateway.HubGroup{}, err
	}
	return a2agateway.HubGroup{
		GroupID:      rec.GroupID,
		HubID:        rec.HubID,
		Name:         rec.Name,
		State:        a2agateway.HubGroupState(rec.State),
		OwnerAgentID: rec.OwnerAgentID,
		CreatedAt:    rec.CreatedAt,
		ArchivedAt:   rec.ArchivedAt,
	}, nil
}

func (p hubStorePersistence) FindGroup(ctx context.Context, groupID string) (a2agateway.HubGroup, error) {
	rec, err := p.store.FindHubGroup(ctx, groupID)
	if err != nil {
		return a2agateway.HubGroup{}, err
	}
	return a2agateway.HubGroup{
		GroupID:      rec.GroupID,
		HubID:        rec.HubID,
		Name:         rec.Name,
		State:        a2agateway.HubGroupState(rec.State),
		OwnerAgentID: rec.OwnerAgentID,
		CreatedAt:    rec.CreatedAt,
		ArchivedAt:   rec.ArchivedAt,
	}, nil
}

func (p hubStorePersistence) ListGroups(ctx context.Context, agentID string) ([]a2agateway.HubGroup, error) {
	recs, err := p.store.ListHubGroups(ctx, agentID)
	if err != nil {
		return nil, err
	}
	out := make([]a2agateway.HubGroup, 0, len(recs))
	for _, rec := range recs {
		out = append(out, a2agateway.HubGroup{
			GroupID:      rec.GroupID,
			HubID:        rec.HubID,
			Name:         rec.Name,
			State:        a2agateway.HubGroupState(rec.State),
			OwnerAgentID: rec.OwnerAgentID,
			CreatedAt:    rec.CreatedAt,
			ArchivedAt:   rec.ArchivedAt,
		})
	}
	return out, nil
}

func (p hubStorePersistence) FindMember(ctx context.Context, groupID, agentID string) (a2agateway.HubGroupMember, error) {
	m, err := p.store.FindHubGroupMember(ctx, groupID, agentID)
	if err != nil {
		return a2agateway.HubGroupMember{}, err
	}
	return a2agateway.HubGroupMember{
		HubID:     m.HubID,
		GroupID:   m.GroupID,
		AgentID:   m.AgentID,
		Role:      a2agateway.HubGroupRole(m.Role),
		State:     a2agateway.HubMembershipState(m.State),
		JoinedAt:  m.JoinedAt,
		LeftAt:    m.LeftAt,
		RemovedAt: m.RemovedAt,
	}, nil
}

func (p hubStorePersistence) ListMembers(ctx context.Context, groupID string) ([]a2agateway.HubGroupMember, error) {
	recs, err := p.store.ListHubGroupMembers(ctx, groupID)
	if err != nil {
		return nil, err
	}
	out := make([]a2agateway.HubGroupMember, 0, len(recs))
	for _, m := range recs {
		out = append(out, a2agateway.HubGroupMember{
			HubID:     m.HubID,
			GroupID:   m.GroupID,
			AgentID:   m.AgentID,
			Role:      a2agateway.HubGroupRole(m.Role),
			State:     a2agateway.HubMembershipState(m.State),
			JoinedAt:  m.JoinedAt,
			LeftAt:    m.LeftAt,
			RemovedAt: m.RemovedAt,
		})
	}
	return out, nil
}

func (p hubStorePersistence) CreateInvitation(ctx context.Context, inv a2agateway.HubGroupInvitation) (a2agateway.HubGroupInvitation, error) {
	rec, err := p.store.CreateHubGroupInvitation(ctx, store.HubGroupInvitationRecord{
		HubID:          inv.HubID,
		GroupID:        inv.GroupID,
		InviterAgentID: inv.InviterAgentID,
		InviteeAgentID: inv.InviteeAgentID,
		CreatedAt:      inv.CreatedAt,
		ExpiresAt:      inv.ExpiresAt,
	})
	if err != nil {
		return a2agateway.HubGroupInvitation{}, err
	}
	inv.ID = rec.ID
	inv.State = a2agateway.HubInvitationState(rec.State)
	return inv, nil
}

func (p hubStorePersistence) FindInvitation(ctx context.Context, id uint64) (a2agateway.HubGroupInvitation, error) {
	rec, err := p.store.FindHubGroupInvitation(ctx, id)
	if err != nil {
		return a2agateway.HubGroupInvitation{}, err
	}
	return a2agateway.HubGroupInvitation{
		ID:             rec.ID,
		HubID:          rec.HubID,
		GroupID:        rec.GroupID,
		InviterAgentID: rec.InviterAgentID,
		InviteeAgentID: rec.InviteeAgentID,
		State:          a2agateway.HubInvitationState(rec.State),
		CreatedAt:      rec.CreatedAt,
		ExpiresAt:      rec.ExpiresAt,
		RespondedAt:    rec.RespondedAt,
	}, nil
}

func (p hubStorePersistence) ListInvitations(ctx context.Context, inviteeAgentID string) ([]a2agateway.HubGroupInvitation, error) {
	recs, err := p.store.ListHubGroupInvitations(ctx, inviteeAgentID)
	if err != nil {
		return nil, err
	}
	out := make([]a2agateway.HubGroupInvitation, 0, len(recs))
	for _, rec := range recs {
		out = append(out, a2agateway.HubGroupInvitation{
			ID:             rec.ID,
			HubID:          rec.HubID,
			GroupID:        rec.GroupID,
			InviterAgentID: rec.InviterAgentID,
			InviteeAgentID: rec.InviteeAgentID,
			State:          a2agateway.HubInvitationState(rec.State),
			CreatedAt:      rec.CreatedAt,
			ExpiresAt:      rec.ExpiresAt,
			RespondedAt:    rec.RespondedAt,
		})
	}
	return out, nil
}

func (p hubStorePersistence) AcceptInvitation(ctx context.Context, id uint64, agentID string, at time.Time) (a2agateway.HubGroupMember, error) {
	m, err := p.store.AcceptHubGroupInvitation(ctx, id, agentID, at)
	if err != nil {
		return a2agateway.HubGroupMember{}, err
	}
	return a2agateway.HubGroupMember{
		HubID:    m.HubID,
		GroupID:  m.GroupID,
		AgentID:  m.AgentID,
		Role:     a2agateway.HubGroupRole(m.Role),
		State:    a2agateway.HubMembershipState(m.State),
		JoinedAt: m.JoinedAt,
	}, nil
}

func (p hubStorePersistence) DeclineInvitation(ctx context.Context, id uint64, agentID string, at time.Time) error {
	return p.store.DeclineHubGroupInvitation(ctx, id, agentID, at)
}

func (p hubStorePersistence) RevokeInvitation(ctx context.Context, id uint64, inviterID string, at time.Time) error {
	return p.store.RevokeHubGroupInvitation(ctx, id, inviterID, at)
}

func (p hubStorePersistence) SendGroupMessage(ctx context.Context, message a2agateway.HubGroupMessage, maxFanout int) (a2agateway.HubGroupMessage, bool, error) {
	rec, dup, err := p.store.SendHubGroupMessage(ctx, store.HubGroupMessageRecord{
		HubID:          message.HubID,
		GroupID:        message.GroupID,
		SenderAgentID:  message.SenderAgentID,
		ContextID:      message.ContextID,
		IdempotencyKey: message.IdempotencyKey,
		Message:        message.Message,
		CreatedAt:      message.CreatedAt,
	}, maxFanout)
	if err != nil {
		return a2agateway.HubGroupMessage{}, false, err
	}
	deliveries := make([]a2agateway.HubGroupDeliverySummary, 0, len(rec.Deliveries))
	for _, d := range rec.Deliveries {
		deliveries = append(deliveries, a2agateway.HubGroupDeliverySummary{
			TargetAgentID: d.TargetAgentID,
			Sequence:      d.Sequence,
			State:         d.State,
		})
	}
	return a2agateway.HubGroupMessage{
		ID:             rec.ID,
		HubID:          rec.HubID,
		GroupID:        rec.GroupID,
		SenderAgentID:  rec.SenderAgentID,
		ContextID:      rec.ContextID,
		IdempotencyKey: rec.IdempotencyKey,
		Message:        rec.Message,
		Trust:          "UNTRUSTED_DATA",
		CreatedAt:      rec.CreatedAt,
		Deliveries:     deliveries,
	}, dup, nil
}

func (p hubStorePersistence) ListGroupMessages(ctx context.Context, groupID, agentID string, afterID uint64, limit int) ([]a2agateway.HubGroupMessage, error) {
	recs, err := p.store.ListHubGroupMessages(ctx, groupID, agentID, afterID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]a2agateway.HubGroupMessage, 0, len(recs))
	for _, rec := range recs {
		out = append(out, a2agateway.HubGroupMessage{
			ID:             rec.ID,
			HubID:          rec.HubID,
			GroupID:        rec.GroupID,
			SenderAgentID:  rec.SenderAgentID,
			ContextID:      rec.ContextID,
			IdempotencyKey: rec.IdempotencyKey,
			Message:        rec.Message,
			Trust:          "UNTRUSTED_DATA",
			CreatedAt:      rec.CreatedAt,
		})
	}
	return out, nil
}

func (p hubStorePersistence) ArchiveGroup(ctx context.Context, groupID string, at time.Time) error {
	return p.store.ArchiveHubGroup(ctx, groupID, at)
}

func (p hubStorePersistence) LeaveGroup(ctx context.Context, groupID, agentID string, at time.Time) error {
	return p.store.LeaveHubGroup(ctx, groupID, agentID, at)
}

func (p hubStorePersistence) RemoveMember(ctx context.Context, groupID, agentID, targetAgentID string, at time.Time) error {
	return p.store.RemoveHubGroupMember(ctx, groupID, agentID, targetAgentID, at)
}
