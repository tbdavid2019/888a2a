package a2a

import (
	"context"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/pkg/errors"

	models "github.com/tbdavid2019/888a2a/backend/generated-go/store"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

var (
	// ErrPeerNotFound indicates no accessible peer matching the ID was found.
	ErrPeerNotFound = errors.New("peer agent not found")
	// ErrUnauthenticatedCaller indicates the caller has no authenticated identity.
	ErrUnauthenticatedCaller = errors.New("unauthenticated caller")
)

// RuntimeReadiness indicates the operational readiness of an agent.
type RuntimeReadiness string

const (
	ReadinessReady       RuntimeReadiness = "READY"
	ReadinessBusy        RuntimeReadiness = "BUSY"
	ReadinessOffline     RuntimeReadiness = "OFFLINE"
	ReadinessUnavailable RuntimeReadiness = "UNAVAILABLE"
)

// PeerProjection represents an Agent Card along with its operational readiness.
type PeerProjection struct {
	AgentResourceID string           `json:"agentResourceId"`
	Tenant          string           `json:"tenant"`
	Readiness       RuntimeReadiness `json:"readiness"`
	Enabled         bool             `json:"enabled"`
	Card            *a2a.AgentCard   `json:"card"`
}

// PeerFilter specifies criteria for filtering peers in the directory.
type PeerFilter struct {
	SkillTag   string `json:"skillTag,omitempty"`
	SkillName  string `json:"skillName,omitempty"`
	InputMode  string `json:"inputMode,omitempty"`
	OutputMode string `json:"outputMode,omitempty"`
	ReadyOnly  bool   `json:"readyOnly,omitempty"`
}

// CallerPrincipal provides caller authorization information.
type CallerPrincipal interface {
	GetPrincipalID() string
	IsAuthenticated() bool
	GetTenantID() string
}

// AgentDirectoryStore is the subset of store needed to query peers.
type AgentDirectoryStore interface {
	ListAgents(ctx context.Context, find *store.FindAgentMessage) ([]*store.AgentMessage, error)
	GetAgentByResourceID(ctx context.Context, resourceID string) (*store.AgentMessage, error)
}

// DirectoryService handles discovery and directory queries for peer agents.
type DirectoryService struct {
	store         AgentDirectoryStore
	baseURL       string
	skills        map[string][]SkillInput
	runtimeStatus func(context.Context, string) ProviderRuntimeStatus
}

type requestBaseURLContextKey struct{}

// NewDirectoryService creates a new agent directory service.
func NewDirectoryService(store AgentDirectoryStore, baseURL string, skills map[string][]SkillInput) *DirectoryService {
	return &DirectoryService{
		store:   store,
		baseURL: baseURL,
		skills:  skills,
	}
}

// SetRuntimeStatusProvider supplies bridge evidence for Agent Card
// projection. A nil provider keeps cards conservative and non-automatic.
func (d *DirectoryService) SetRuntimeStatusProvider(provider func(context.Context, string) ProviderRuntimeStatus) {
	d.runtimeStatus = provider
}

// ComputeReadiness determines the operational readiness of an agent.
func ComputeReadiness(agent *store.AgentMessage) RuntimeReadiness {
	if agent == nil || !agent.Enabled || agent.Deleted {
		return ReadinessUnavailable
	}
	if agent.Status != nil {
		switch agent.Status.State {
		case models.AgentStatus_ONLINE:
			if agent.Status.ActiveSessionId != "" {
				return ReadinessBusy
			}
			return ReadinessReady
		case models.AgentStatus_OFFLINE:
			return ReadinessOffline
		case models.AgentStatus_ERROR, models.AgentStatus_KICKED, models.AgentStatus_CONNECTION_STATE_UNSPECIFIED:
			return ReadinessUnavailable
		}
	}
	return ReadinessReady
}

// ListPeers returns all accessible peers for an authenticated caller matching the filter.
func (d *DirectoryService) ListPeers(ctx context.Context, caller CallerPrincipal, tenant string, filter PeerFilter) ([]*PeerProjection, error) {
	if caller == nil || !caller.IsAuthenticated() {
		return nil, ErrUnauthenticatedCaller
	}

	tenant = strings.TrimSpace(tenant)
	if tenant == "" {
		tenant = "default"
	}

	agents, err := d.store.ListAgents(ctx, &store.FindAgentMessage{
		ShowDeleted: false,
	})
	if err != nil {
		return nil, errors.Wrap(err, "list agents for directory")
	}

	var results []*PeerProjection
	for _, ag := range agents {
		if ag.Deleted {
			continue
		}

		readiness := ComputeReadiness(ag)
		if filter.ReadyOnly && readiness != ReadinessReady {
			continue
		}

		var agentSkills []SkillInput
		if d.skills != nil {
			agentSkills = d.skills[ag.ResourceID]
		}

		baseURL := d.baseURL
		if requestBaseURL, ok := ctx.Value(requestBaseURLContextKey{}).(string); ok && requestBaseURL != "" {
			baseURL = requestBaseURL
		}
		card, err := ProjectAgentCard(ProjectAgentCardOptions{
			Agent:   ag,
			Skills:  agentSkills,
			BaseURL: baseURL,
			Tenant:  tenant,
			Runtime: d.getRuntimeStatus(ctx, ag.ResourceID),
		})
		if err != nil {
			continue
		}

		if !matchesSkillFilter(card, filter) {
			continue
		}

		results = append(results, &PeerProjection{
			AgentResourceID: ag.ResourceID,
			Tenant:          tenant,
			Readiness:       readiness,
			Enabled:         ag.Enabled,
			Card:            card,
		})
	}

	return results, nil
}

// GetPeer retrieves an Agent Card and readiness state for a specific peer.
func (d *DirectoryService) GetPeer(ctx context.Context, caller CallerPrincipal, tenant string, agentResourceID string) (*PeerProjection, error) {
	if caller == nil || !caller.IsAuthenticated() {
		return nil, ErrUnauthenticatedCaller
	}

	tenant = strings.TrimSpace(tenant)
	if tenant == "" {
		tenant = "default"
	}

	agent, err := d.store.GetAgentByResourceID(ctx, agentResourceID)
	if err != nil || agent == nil || agent.Deleted {
		return nil, ErrPeerNotFound
	}

	readiness := ComputeReadiness(agent)
	var agentSkills []SkillInput
	if d.skills != nil {
		agentSkills = d.skills[agent.ResourceID]
	}

	baseURL := d.baseURL
	if requestBaseURL, ok := ctx.Value(requestBaseURLContextKey{}).(string); ok && requestBaseURL != "" {
		baseURL = requestBaseURL
	}
	card, err := ProjectAgentCard(ProjectAgentCardOptions{
		Agent:   agent,
		Skills:  agentSkills,
		BaseURL: baseURL,
		Tenant:  tenant,
		Runtime: d.getRuntimeStatus(ctx, agent.ResourceID),
	})
	if err != nil {
		return nil, errors.Wrap(err, "project peer agent card")
	}

	return &PeerProjection{
		AgentResourceID: agent.ResourceID,
		Tenant:          tenant,
		Readiness:       readiness,
		Enabled:         agent.Enabled,
		Card:            card,
	}, nil
}

func (d *DirectoryService) getRuntimeStatus(ctx context.Context, agentID string) ProviderRuntimeStatus {
	if d.runtimeStatus == nil {
		return ProviderRuntimeStatus{Readiness: "UNVERIFIED"}
	}
	return d.runtimeStatus(ctx, agentID)
}

func matchesSkillFilter(card *a2a.AgentCard, filter PeerFilter) bool {
	if filter.SkillName == "" && filter.SkillTag == "" && filter.InputMode == "" && filter.OutputMode == "" {
		return true
	}

	for _, skill := range card.Skills {
		if filter.SkillName != "" && !strings.EqualFold(skill.Name, filter.SkillName) && !strings.EqualFold(skill.ID, filter.SkillName) {
			continue
		}
		if filter.SkillTag != "" {
			var tagFound bool
			for _, tag := range skill.Tags {
				if strings.EqualFold(tag, filter.SkillTag) {
					tagFound = true
					break
				}
			}
			if !tagFound {
				continue
			}
		}
		if filter.InputMode != "" {
			var modeFound bool
			for _, m := range skill.InputModes {
				if strings.EqualFold(m, filter.InputMode) {
					modeFound = true
					break
				}
			}
			if !modeFound {
				continue
			}
		}
		if filter.OutputMode != "" {
			var modeFound bool
			for _, m := range skill.OutputModes {
				if strings.EqualFold(m, filter.OutputMode) {
					modeFound = true
					break
				}
			}
			if !modeFound {
				continue
			}
		}
		return true
	}
	return false
}
