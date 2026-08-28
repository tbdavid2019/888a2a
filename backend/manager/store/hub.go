package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
)

var ErrHubAgentAlreadyExists = errors.New("Hub Agent already exists")

type HubMessage struct {
	HubID                  string
	Mode                   string
	BootstrapTokenHash     string
	RegistrationEnabled    bool
	PublicConfirmed        bool
	RegistrationTTLSeconds int
	PeerLeaseSeconds       int
	MaxRegisteredAgents    int
	MaxTasksPerMinute      int
	MaxConcurrentTasks     int
	MaxPayloadBytes        int64
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type HubAgentMessage struct {
	HubID               string
	AgentID             string
	RegistrationKeyHash string
	AgentTokenHash      string
	DisplayName         string
	ProviderFamily      string
	TransportID         string
	CapabilitiesJSON    string
	AgentCardJSON       string
	State               string
	AutomaticExecution  bool
	LastSeenAt          sql.NullTime
	ExpiresAt           time.Time
	LeaseExpiresAt      time.Time
	RevokedAt           sql.NullTime
	RevokeReason        string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (s *Store) UpsertHub(ctx context.Context, hub *HubMessage) error {
	if hub == nil || hub.HubID == "" {
		return errors.New("Hub is required")
	}
	_, err := s.GetDB().ExecContext(ctx, `
		INSERT INTO a2a888_hub (hub_id, mode, bootstrap_token_hash, registration_enabled, public_confirmed,
			registration_ttl_seconds, peer_lease_seconds, max_registered_agents, max_tasks_per_minute,
			max_concurrent_tasks, max_payload_bytes, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,now())
		ON CONFLICT (hub_id) DO UPDATE SET mode=EXCLUDED.mode,
			bootstrap_token_hash=EXCLUDED.bootstrap_token_hash, registration_enabled=EXCLUDED.registration_enabled,
			public_confirmed=EXCLUDED.public_confirmed, registration_ttl_seconds=EXCLUDED.registration_ttl_seconds,
			peer_lease_seconds=EXCLUDED.peer_lease_seconds, max_registered_agents=EXCLUDED.max_registered_agents,
			max_tasks_per_minute=EXCLUDED.max_tasks_per_minute, max_concurrent_tasks=EXCLUDED.max_concurrent_tasks,
			max_payload_bytes=EXCLUDED.max_payload_bytes, updated_at=now()`,
		hub.HubID, hub.Mode, hub.BootstrapTokenHash, hub.RegistrationEnabled, hub.PublicConfirmed,
		hub.RegistrationTTLSeconds, hub.PeerLeaseSeconds, hub.MaxRegisteredAgents, hub.MaxTasksPerMinute,
		hub.MaxConcurrentTasks, hub.MaxPayloadBytes)
	return err
}

func (s *Store) GetHub(ctx context.Context, hubID string) (*HubMessage, error) {
	var hub HubMessage
	err := s.GetDB().QueryRowContext(ctx, `SELECT hub_id, mode, bootstrap_token_hash, registration_enabled, public_confirmed,
		registration_ttl_seconds, peer_lease_seconds, max_registered_agents, max_tasks_per_minute,
		max_concurrent_tasks, max_payload_bytes, created_at, updated_at
		FROM a2a888_hub WHERE hub_id=$1`, hubID).Scan(
		&hub.HubID, &hub.Mode, &hub.BootstrapTokenHash, &hub.RegistrationEnabled, &hub.PublicConfirmed,
		&hub.RegistrationTTLSeconds, &hub.PeerLeaseSeconds, &hub.MaxRegisteredAgents, &hub.MaxTasksPerMinute,
		&hub.MaxConcurrentTasks, &hub.MaxPayloadBytes, &hub.CreatedAt, &hub.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &hub, nil
}

func (s *Store) CreateHubAgent(ctx context.Context, agent *HubAgentMessage) error {
	if agent == nil {
		return errors.New("Hub Agent is required")
	}
	_, err := s.GetDB().ExecContext(ctx, `INSERT INTO a2a888_hub_agent
		(hub_id, agent_id, registration_key_hash, agent_token_hash, display_name, provider_family, transport_id,
		 capabilities, agent_card_json, state, automatic_execution, last_seen_at, expires_at, lease_expires_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,$10,$11,$12,$13,$14,now(),now())`,
		agent.HubID, agent.AgentID, agent.RegistrationKeyHash, agent.AgentTokenHash, agent.DisplayName,
		agent.ProviderFamily, agent.TransportID, agent.CapabilitiesJSON, agent.AgentCardJSON, agent.State,
		agent.AutomaticExecution, nullTimeArg(agent.LastSeenAt), agent.ExpiresAt, agent.LeaseExpiresAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrHubAgentAlreadyExists
		}
		return err
	}
	return nil
}

func (s *Store) GetHubAgentByRegistrationHash(ctx context.Context, hubID, hash string) (*HubAgentMessage, error) {
	return s.getHubAgent(ctx, `registration_key_hash=$2`, hubID, hash)
}

func (s *Store) GetHubAgentByTokenHash(ctx context.Context, hubID, hash string) (*HubAgentMessage, error) {
	return s.getHubAgent(ctx, `agent_token_hash=$2`, hubID, hash)
}

func (s *Store) ListHubAgents(ctx context.Context, hubID string) ([]*HubAgentMessage, error) {
	rows, err := s.GetDB().QueryContext(ctx, `SELECT hub_id, agent_id, registration_key_hash, agent_token_hash,
		display_name, provider_family, transport_id, capabilities::text, agent_card_json::text, state,
		automatic_execution, last_seen_at, expires_at, lease_expires_at, revoked_at, revoke_reason, created_at, updated_at
		FROM a2a888_hub_agent WHERE hub_id=$1 ORDER BY created_at, agent_id`, hubID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var agents []*HubAgentMessage
	for rows.Next() {
		agent := new(HubAgentMessage)
		if err := rows.Scan(&agent.HubID, &agent.AgentID, &agent.RegistrationKeyHash, &agent.AgentTokenHash, &agent.DisplayName,
			&agent.ProviderFamily, &agent.TransportID, &agent.CapabilitiesJSON, &agent.AgentCardJSON, &agent.State,
			&agent.AutomaticExecution, &agent.LastSeenAt, &agent.ExpiresAt, &agent.LeaseExpiresAt, &agent.RevokedAt, &agent.RevokeReason,
			&agent.CreatedAt, &agent.UpdatedAt); err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return agents, nil
}

func (s *Store) getHubAgent(ctx context.Context, predicate, hubID, value string) (*HubAgentMessage, error) {
	var agent HubAgentMessage
	err := s.GetDB().QueryRowContext(ctx, `SELECT hub_id, agent_id, registration_key_hash, agent_token_hash,
		display_name, provider_family, transport_id, capabilities::text, agent_card_json::text, state,
		automatic_execution, last_seen_at, expires_at, lease_expires_at, revoked_at, revoke_reason, created_at, updated_at
		FROM a2a888_hub_agent WHERE hub_id=$1 AND `+predicate, hubID, value).Scan(
		&agent.HubID, &agent.AgentID, &agent.RegistrationKeyHash, &agent.AgentTokenHash, &agent.DisplayName,
		&agent.ProviderFamily, &agent.TransportID, &agent.CapabilitiesJSON, &agent.AgentCardJSON, &agent.State,
		&agent.AutomaticExecution, &agent.LastSeenAt, &agent.ExpiresAt, &agent.LeaseExpiresAt, &agent.RevokedAt, &agent.RevokeReason,
		&agent.CreatedAt, &agent.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &agent, nil
}

func (s *Store) HeartbeatHubAgent(ctx context.Context, hubID, agentID, tokenHash string, now time.Time, lease time.Duration) error {
	result, err := s.GetDB().ExecContext(ctx, `UPDATE a2a888_hub_agent
		SET state='ONLINE', last_seen_at=$4, updated_at=$4, lease_expires_at=$4 + ($5 * INTERVAL '1 second')
		WHERE hub_id=$1 AND agent_id=$2 AND agent_token_hash=$3 AND state NOT IN ('REVOKED','EXPIRED') AND expires_at>$4 AND lease_expires_at>$4`,
		hubID, agentID, tokenHash, now, int64(lease/time.Second))
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return errors.New("Hub Agent heartbeat rejected")
	}
	return nil
}

func (s *Store) RotateHubAgent(ctx context.Context, hubID, agentID, tokenHash string, now time.Time) error {
	result, err := s.GetDB().ExecContext(ctx, `UPDATE a2a888_hub_agent SET agent_token_hash=$3, updated_at=$4
		WHERE hub_id=$1 AND agent_id=$2 AND state NOT IN ('REVOKED','EXPIRED') AND expires_at>$4`, hubID, agentID, tokenHash, now)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return errors.New("Hub Agent token rotation rejected")
	}
	return nil
}

func (s *Store) DisconnectHubAgent(ctx context.Context, hubID, agentID string, now time.Time) error {
	result, err := s.GetDB().ExecContext(ctx, `UPDATE a2a888_hub_agent SET state='OFFLINE', last_seen_at=$3, updated_at=$3
		WHERE hub_id=$1 AND agent_id=$2 AND state NOT IN ('REVOKED','EXPIRED') AND expires_at>$3`, hubID, agentID, now)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return errors.New("Hub Agent disconnect rejected")
	}
	return nil
}

func (s *Store) RevokeHubAgent(ctx context.Context, hubID, agentID, reason string, now time.Time) error {
	result, err := s.GetDB().ExecContext(ctx, `UPDATE a2a888_hub_agent SET state='REVOKED', revoked_at=$3, revoke_reason=$4, updated_at=$3
		WHERE hub_id=$1 AND agent_id=$2 AND state <> 'REVOKED'`, hubID, agentID, now, reason)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return errors.New("Hub Agent not found")
	}
	return nil
}

func nullTimeArg(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time
}
