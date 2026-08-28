package a2a

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type HubAgentState string

var ErrHubRegistrationDisabled = errors.New("Hub registration is disabled")

const (
	HubAgentStatePending HubAgentState = "PENDING"
	HubAgentStateOnline  HubAgentState = "ONLINE"
	HubAgentStateOffline HubAgentState = "OFFLINE"
	HubAgentStateRevoked HubAgentState = "REVOKED"
	HubAgentStateExpired HubAgentState = "EXPIRED"
)

// IssuedAgentIdentity contains the token exactly once, at enrollment time.
// Callers must not persist or log this value outside a credential store.
type IssuedAgentIdentity struct {
	HubID      string    `json:"hubId"`
	AgentID    string    `json:"agentId"`
	AgentToken string    `json:"agentToken,omitempty"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

// RegisteredAgent is the non-secret peer metadata used by the directory.
type RegisteredAgent struct {
	HubID              string
	AgentID            string
	DisplayName        string
	ProviderFamily     string
	TransportID        string
	Capabilities       []string
	AgentCardJSON      string
	State              HubAgentState
	AutomaticExecution bool
	LastSeenAt         time.Time
	ExpiresAt          time.Time
	CreatedAt          time.Time
	RevokedAt          *time.Time
	RevokeReason       string

	registrationHash string
	tokenHash        [32]byte
	LeaseExpiresAt   time.Time
}

// HubAgentView is the directory-safe representation of a registered Agent.
// It deliberately omits Agent Card JSON, token hashes, and revocation detail.
type HubAgentView struct {
	HubID              string        `json:"hubId"`
	AgentID            string        `json:"agentId"`
	DisplayName        string        `json:"displayName"`
	ProviderFamily     string        `json:"providerFamily"`
	TransportID        string        `json:"transportId"`
	Capabilities       []string      `json:"capabilities"`
	State              HubAgentState `json:"state"`
	LastSeenAt         time.Time     `json:"lastSeenAt,omitempty"`
	ExpiresAt          time.Time     `json:"expiresAt"`
	AutomaticExecution bool          `json:"automaticExecution"`
}

// HubAgentRecord is the persistence adapter shape. Token and registration
// values are one-way hashes, never plaintext credentials.
type HubAgentRecord struct {
	RegisteredAgent
	RegistrationHash string
	TokenHash        string
}

// HubPersistence is implemented by the Manager store adapter. Keeping this
// boundary here lets the registry remain deterministic in unit tests.
type HubPersistence interface {
	ListHubAgents(context.Context, string) ([]HubAgentRecord, error)
	SaveHubAgent(context.Context, HubAgentRecord) error
	HeartbeatHubAgent(context.Context, string, string, string, time.Time, time.Duration) error
	RotateHubAgent(context.Context, string, string, string, time.Time) error
	DisconnectHubAgent(context.Context, string, string, time.Time) error
	RevokeHubAgent(context.Context, string, string, string, time.Time) error
}

// HubRegistry is the correctness-first registry implementation. Persistence
// is added by the Manager store; the registry deliberately keeps token hashes
// and no plaintext credentials.
type HubRegistry struct {
	mu             sync.Mutex
	policy         HubPolicy
	bootstrapHash  [32]byte
	now            func() time.Time
	agents         map[string]*RegisteredAgent
	byRegistration map[string]string
	persistence    HubPersistence
}

func NewHubRegistry(policy HubPolicy, bootstrapToken string, now func() time.Time) (*HubRegistry, error) {
	return newHubRegistry(policy, bootstrapToken, now, nil)
}

func NewHubRegistryWithPersistence(ctx context.Context, policy HubPolicy, bootstrapToken string, now func() time.Time, persistence HubPersistence) (*HubRegistry, error) {
	registry, err := newHubRegistry(policy, bootstrapToken, now, persistence)
	if err != nil {
		return nil, err
	}
	if persistence == nil {
		return registry, nil
	}
	records, err := persistence.ListHubAgents(ctx, registry.policy.HubID)
	if err != nil {
		return nil, fmt.Errorf("load Hub Agents: %w", err)
	}
	if err := registry.Load(records); err != nil {
		return nil, err
	}
	return registry, nil
}

func newHubRegistry(policy HubPolicy, bootstrapToken string, now func() time.Time, persistence HubPersistence) (*HubRegistry, error) {
	if policy.Mode == "" {
		policy.Mode = HubModeClosed
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	if policy.Mode == HubModeOpen && strings.TrimSpace(bootstrapToken) == "" {
		return nil, errors.New("bootstrap token is required for open Hub")
	}
	if now == nil {
		now = time.Now
	}
	return &HubRegistry{
		policy: policy, bootstrapHash: sha256.Sum256([]byte(bootstrapToken)), now: now,
		agents: make(map[string]*RegisteredAgent), byRegistration: make(map[string]string),
		persistence: persistence,
	}, nil
}

func (r *HubRegistry) Register(bootstrapToken string, declaration AgentDeclaration) (IssuedAgentIdentity, error) {
	return r.RegisterContext(context.Background(), bootstrapToken, declaration)
}

func (r *HubRegistry) RegisterContext(ctx context.Context, bootstrapToken string, declaration AgentDeclaration) (IssuedAgentIdentity, error) {
	if r == nil {
		return IssuedAgentIdentity{}, errors.New("Hub registry is required")
	}
	if !r.policy.RegistrationEnabled {
		return IssuedAgentIdentity{}, ErrHubRegistrationDisabled
	}
	if err := declaration.Validate(); err != nil {
		return IssuedAgentIdentity{}, err
	}
	if r.policy.Mode == HubModeOpen && !r.matchesHash(bootstrapToken, r.bootstrapHash) {
		return IssuedAgentIdentity{}, errors.New("invalid Hub bootstrap token")
	}
	now := r.now()
	registrationHash := hashHubSecretHex(declaration.RegistrationIdempotencyKey)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reconcileLocked(now)
	if existingID, ok := r.byRegistration[registrationHash]; ok {
		existing := r.agents[existingID]
		if existing != nil && existing.State != HubAgentStateRevoked && existing.State != HubAgentStateExpired {
			return IssuedAgentIdentity{HubID: r.policy.HubID, AgentID: existing.AgentID, ExpiresAt: existing.ExpiresAt}, nil
		}
	}
	if int32(len(r.agents)) >= r.policy.MaxRegisteredAgents {
		return IssuedAgentIdentity{}, errors.New("Hub registered Agent limit reached")
	}
	token, err := randomHubToken()
	if err != nil {
		return IssuedAgentIdentity{}, err
	}
	agentID := "agt_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	expires := now.Add(time.Duration(r.policy.RegistrationTTL) * time.Second)
	agent := &RegisteredAgent{
		HubID: r.policy.HubID, AgentID: agentID, DisplayName: declaration.DisplayName,
		ProviderFamily: declaration.ProviderFamily, TransportID: declaration.TransportID,
		Capabilities: append([]string(nil), declaration.Capabilities...), AgentCardJSON: declaration.AgentCardJSON,
		State: HubAgentStatePending, CreatedAt: now, ExpiresAt: expires, registrationHash: registrationHash,
		tokenHash: hashHubSecret(token), LeaseExpiresAt: now.Add(time.Duration(r.policy.PeerLeaseSeconds) * time.Second),
	}
	if r.persistence != nil {
		if err := r.persistence.SaveHubAgent(ctx, HubAgentRecord{RegisteredAgent: *agent, RegistrationHash: registrationHash, TokenHash: hex.EncodeToString(agent.tokenHash[:])}); err != nil {
			return IssuedAgentIdentity{}, fmt.Errorf("persist Hub Agent registration: %w", err)
		}
	}
	r.agents[agentID] = agent
	r.byRegistration[registrationHash] = agentID
	return IssuedAgentIdentity{HubID: r.policy.HubID, AgentID: agentID, AgentToken: token, ExpiresAt: expires}, nil
}

func (r *HubRegistry) Authenticate(agentID, token string) (*RegisteredAgent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	r.reconcileLocked(now)
	agent, ok := r.agents[agentID]
	if !ok || agent.State == HubAgentStateRevoked || agent.State == HubAgentStateExpired || !r.matchesHash(token, agent.tokenHash) {
		return nil, errors.New("invalid Hub Agent credentials")
	}
	return cloneRegisteredAgent(agent), nil
}

func (r *HubRegistry) Heartbeat(agentID, token string) (*RegisteredAgent, error) {
	return r.HeartbeatContext(context.Background(), agentID, token)
}

func (r *HubRegistry) Disconnect(agentID, token string) (*RegisteredAgent, error) {
	return r.DisconnectContext(context.Background(), agentID, token)
}

func (r *HubRegistry) DisconnectContext(ctx context.Context, agentID, token string) (*RegisteredAgent, error) {
	if r == nil {
		return nil, errors.New("Hub registry is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	r.reconcileLocked(now)
	agent, ok := r.agents[agentID]
	if !ok || agent.State == HubAgentStateRevoked || agent.State == HubAgentStateExpired || !r.matchesHash(token, agent.tokenHash) {
		return nil, errors.New("invalid Hub Agent credentials")
	}
	if r.persistence != nil {
		if err := r.persistence.DisconnectHubAgent(ctx, r.policy.HubID, agentID, now); err != nil {
			return nil, err
		}
	}
	agent.State = HubAgentStateOffline
	agent.LastSeenAt = now
	return cloneRegisteredAgent(agent), nil
}

func (r *HubRegistry) HeartbeatContext(ctx context.Context, agentID, token string) (*RegisteredAgent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	r.reconcileLocked(now)
	agent, ok := r.agents[agentID]
	if !ok || agent.State == HubAgentStateRevoked || agent.State == HubAgentStateExpired || !r.matchesHash(token, agent.tokenHash) {
		return nil, errors.New("invalid Hub Agent credentials")
	}
	if r.persistence != nil {
		if err := r.persistence.HeartbeatHubAgent(ctx, r.policy.HubID, agentID, hex.EncodeToString(agent.tokenHash[:]), now, time.Duration(r.policy.PeerLeaseSeconds)*time.Second); err != nil {
			return nil, fmt.Errorf("persist Hub Agent heartbeat: %w", err)
		}
	}
	agent.State = HubAgentStateOnline
	agent.LastSeenAt = now
	agent.LeaseExpiresAt = now.Add(time.Duration(r.policy.PeerLeaseSeconds) * time.Second)
	return cloneRegisteredAgent(agent), nil
}

func (r *HubRegistry) Revoke(agentID, reason string) error {
	return r.RevokeContext(context.Background(), agentID, reason)
}

// RotateToken issues a new per-Agent token and invalidates the previous one.
// The new plaintext token is returned once and is never stored in the registry.
func (r *HubRegistry) RotateToken(agentID string) (string, error) {
	return r.RotateTokenContext(context.Background(), agentID)
}

func (r *HubRegistry) RotateTokenContext(ctx context.Context, agentID string) (string, error) {
	if r == nil {
		return "", errors.New("Hub registry is required")
	}
	token, err := randomHubToken()
	if err != nil {
		return "", err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	agent, ok := r.agents[agentID]
	if !ok || agent.State == HubAgentStateRevoked || agent.State == HubAgentStateExpired {
		return "", errors.New("Hub Agent not found")
	}
	now := r.now()
	if r.persistence != nil {
		if err := r.persistence.RotateHubAgent(ctx, r.policy.HubID, agentID, hashHubSecretHex(token), now); err != nil {
			return "", err
		}
	}
	agent.tokenHash = hashHubSecret(token)
	return token, nil
}

func (r *HubRegistry) RevokeContext(ctx context.Context, agentID, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	agent, ok := r.agents[agentID]
	if !ok {
		return errors.New("Hub Agent not found")
	}
	if agent.State == HubAgentStateRevoked {
		return nil
	}
	now := r.now()
	if r.persistence != nil {
		if err := r.persistence.RevokeHubAgent(ctx, r.policy.HubID, agentID, reason, now); err != nil {
			return err
		}
	}
	agent.State = HubAgentStateRevoked
	agent.RevokedAt = &now
	agent.RevokeReason = strings.TrimSpace(reason)
	return nil
}

// Load imports safe persistence records after a process restart. It rejects
// records for another Hub and never imports plaintext token material.
func (r *HubRegistry) Load(records []HubAgentRecord) error {
	if r == nil {
		return errors.New("Hub registry is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, record := range records {
		if record.HubID != r.policy.HubID || record.AgentID == "" || record.RegistrationHash == "" || record.TokenHash == "" {
			return errors.New("invalid persisted Hub Agent record")
		}
		tokenBytes, err := hex.DecodeString(record.TokenHash)
		if err != nil || len(tokenBytes) != sha256.Size {
			return errors.New("persisted Hub Agent token hash is invalid")
		}
		var tokenHash [32]byte
		copy(tokenHash[:], tokenBytes)
		agent := record.RegisteredAgent
		agent.registrationHash = record.RegistrationHash
		agent.tokenHash = tokenHash
		agent.LeaseExpiresAt = agent.LastSeenAt.Add(time.Duration(r.policy.PeerLeaseSeconds) * time.Second)
		if agent.LastSeenAt.IsZero() {
			agent.LeaseExpiresAt = agent.CreatedAt.Add(time.Duration(r.policy.PeerLeaseSeconds) * time.Second)
		}
		r.agents[agent.AgentID] = &agent
		r.byRegistration[agent.registrationHash] = agent.AgentID
	}
	return nil
}

func (r *HubRegistry) List() []RegisteredAgent {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reconcileLocked(r.now())
	out := make([]RegisteredAgent, 0, len(r.agents))
	for _, agent := range r.agents {
		out = append(out, *cloneRegisteredAgent(agent))
	}
	return out
}

func (r *HubRegistry) ListViews() []HubAgentView {
	agents := r.List()
	views := make([]HubAgentView, 0, len(agents))
	for _, agent := range agents {
		views = append(views, agent.View())
	}
	return views
}

// Reconcile marks registration and peer leases that have expired. It returns
// safe metadata for durable reconciliation; it never restarts an Agent.
func (r *HubRegistry) Reconcile() []RegisteredAgent {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reconcileLocked(r.now())
}

func (r *HubRegistry) matchesHash(value string, expected [32]byte) bool {
	actual := hashHubSecret(value)
	return subtle.ConstantTimeCompare(actual[:], expected[:]) == 1
}

func (r *HubRegistry) reconcileLocked(now time.Time) []RegisteredAgent {
	var expired []RegisteredAgent
	for _, agent := range r.agents {
		if (agent.State == HubAgentStatePending || agent.State == HubAgentStateOnline || agent.State == HubAgentStateOffline) && !now.Before(agent.ExpiresAt) {
			agent.State = HubAgentStateExpired
			expired = append(expired, *cloneRegisteredAgent(agent))
			continue
		}
		if (agent.State == HubAgentStatePending || agent.State == HubAgentStateOnline) && !now.Before(agent.LeaseExpiresAt) {
			agent.State = HubAgentStateOffline
		}
	}
	return expired
}

func cloneRegisteredAgent(agent *RegisteredAgent) *RegisteredAgent {
	if agent == nil {
		return nil
	}
	clone := *agent
	clone.Capabilities = append([]string(nil), agent.Capabilities...)
	clone.registrationHash = ""
	clone.tokenHash = [32]byte{}
	return &clone
}

func (agent RegisteredAgent) View() HubAgentView {
	return HubAgentView{
		HubID: agent.HubID, AgentID: agent.AgentID, DisplayName: agent.DisplayName,
		ProviderFamily: agent.ProviderFamily, TransportID: agent.TransportID,
		Capabilities: append([]string(nil), agent.Capabilities...), State: agent.State,
		LastSeenAt: agent.LastSeenAt, ExpiresAt: agent.ExpiresAt,
		AutomaticExecution: agent.AutomaticExecution,
	}
}

func hashHubSecret(value string) [32]byte { return sha256.Sum256([]byte(value)) }

func hashHubSecretHex(value string) string {
	hash := hashHubSecret(value)
	return hex.EncodeToString(hash[:])
}

func randomHubToken() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate Hub Agent token: %w", err)
	}
	return "hat_" + hex.EncodeToString(data), nil
}
