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
