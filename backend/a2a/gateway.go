package a2a

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/a2aproject/a2a-go/v2/a2asrv/taskstore"
)

// GatewayOptions configures the A2A HTTP+JSON Gateway.
type GatewayOptions struct {
	TaskStore       taskstore.Store
	EventManager    *EventManager
	Directory       *DirectoryService
	BaseURL         string
	DefaultAgentID  string
	ExecutorFactory func(agentID string) a2asrv.AgentExecutor
}

type targetAgentContextKey struct{}

// Gateway implements the tenant-ready A2A 1.0 HTTP+JSON gateway.
type Gateway struct {
	opts     GatewayOptions
	handlers map[string]http.Handler // key: tenant:agentID
	mu       sync.RWMutex
}

// NewGateway creates a new A2A Gateway instance.
func NewGateway(opts GatewayOptions) *Gateway {
	if opts.BaseURL == "" {
		opts.BaseURL = "http://localhost:8181"
	}
	if opts.DefaultAgentID == "" {
		opts.DefaultAgentID = "default"
	}
	return &Gateway{
		opts:     opts,
		handlers: make(map[string]http.Handler),
	}
}

// getOrCreateAgentHandler creates or retrieves an a2asrv REST handler for an agent.
func (g *Gateway) getOrCreateAgentHandler(tenant, agentID string) http.Handler {
	key := tenant + ":" + agentID
	g.mu.RLock()
	h, ok := g.handlers[key]
	g.mu.RUnlock()
	if ok && h != nil {
		return h
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	// Double check
	if h, ok = g.handlers[key]; ok && h != nil {
		return h
	}

	var executor a2asrv.AgentExecutor
	if g.opts.ExecutorFactory != nil {
		executor = g.opts.ExecutorFactory(agentID)
	} else {
		executor = NewAgentExecutor(agentID, nil)
	}

	var srvOpts []a2asrv.RequestHandlerOption
	if g.opts.TaskStore != nil {
		srvOpts = append(srvOpts, a2asrv.WithTaskStore(g.opts.TaskStore))
	}

	handler := a2asrv.NewHandler(executor, srvOpts...)
	restHandler := a2asrv.NewRESTHandler(handler)
	g.handlers[key] = restHandler
	return restHandler
}

// ServeHTTP handles incoming A2A requests with protocol version negotiation and tenant routing.
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Protocol version negotiation
	reqVer := r.Header.Get(a2a.SvcParamVersion)
	negotiatedVer, err := NegotiateProtocolVersion(reqVer)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":             err.Error(),
			"supported_version": ProtocolVersion1_0,
		})
		return
	}
	w.Header().Set(a2a.SvcParamVersion, negotiatedVer)

	path := r.URL.Path

	// Well-known agent card
	if path == "/.well-known/agent-card.json" || path == "/a2a/v1/agent-card" {
		g.serveDefaultCard(w, r)
		return
	}

	// Tenant-scoped routing: /a2a/v1/{tenant}/agents/{agentID}/...
	if strings.HasPrefix(path, "/a2a/v1/") {
		trimmed := strings.TrimPrefix(path, "/a2a/v1/")
		parts := strings.SplitN(trimmed, "/", 4)
		if len(parts) >= 3 && parts[1] == "agents" {
			tenant := parts[0]
			agentID := parts[2]
			subPath := ""
			if len(parts) == 4 {
				subPath = "/" + parts[3]
			}

			// Agent Card for specific agent
			if subPath == "/.well-known/agent-card.json" || subPath == "/agent-card.json" || subPath == "" {
				g.serveAgentCard(w, r, tenant, agentID)
				return
			}

			// Forward request to agent REST handler
			ctx := a2a.AttachTenant(r.Context(), tenant)
			ctx = context.WithValue(ctx, targetAgentContextKey{}, agentID)

			reqClone := r.Clone(ctx)
			reqClone.URL.Path = subPath

			h := g.getOrCreateAgentHandler(tenant, agentID)
			h.ServeHTTP(w, reqClone)
			return
		}
	}

	// Fallback to default agent handler for root or un-namespaced requests
	defaultHandler := g.getOrCreateAgentHandler("default", g.opts.DefaultAgentID)
	defaultHandler.ServeHTTP(w, r)
}

func (g *Gateway) serveDefaultCard(w http.ResponseWriter, r *http.Request) {
	tenant, _ := a2a.TenantFrom(r.Context())
	if tenant == "" {
		tenant = "default"
	}

	card := &a2a.AgentCard{
		Name:        "888a2a Network Gateway",
		Description: "A2A 1.0 Gateway for 888a2a Agent Network",
		Version:     ProtocolVersion1_0,
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface(g.opts.BaseURL+"/a2a/v1/"+tenant+"/agents/"+g.opts.DefaultAgentID, a2a.TransportProtocolHTTPJSON),
		},
		Capabilities: a2a.AgentCapabilities{
			Streaming:         true,
			ExtendedAgentCard: true,
		},
		DefaultInputModes:  []string{"text/plain", "application/json"},
		DefaultOutputModes: []string{"text/plain", "application/json"},
		Provider: &a2a.AgentProvider{
			Org: "888a2a",
			URL: g.opts.BaseURL,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(card)
}

func (g *Gateway) serveAgentCard(w http.ResponseWriter, r *http.Request, tenant, agentID string) {
	if g.opts.Directory != nil {
		caller, ok := CallerFromContext(r.Context())
		if !ok || caller == nil {
			caller = &defaultPublicCaller{tenant: tenant}
		}
		peer, err := g.opts.Directory.GetPeer(r.Context(), caller, tenant, agentID)
		if err == nil && peer != nil && peer.Card != nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(peer.Card)
			return
		}
	}

	// Fallback card if directory lookup did not match
	card := &a2a.AgentCard{
		Name:        agentID,
		Description: "888a2a Agent " + agentID,
		Version:     ProtocolVersion1_0,
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface(g.opts.BaseURL+"/a2a/v1/"+tenant+"/agents/"+agentID, a2a.TransportProtocolHTTPJSON),
		},
		Capabilities: a2a.AgentCapabilities{
			Streaming:         true,
			ExtendedAgentCard: true,
		},
		DefaultInputModes:  []string{"text/plain", "application/json"},
		DefaultOutputModes: []string{"text/plain", "application/json"},
		Provider: &a2a.AgentProvider{
			Org: "888a2a",
			URL: g.opts.BaseURL,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(card)
}

type defaultPublicCaller struct {
	tenant string
}

func (*defaultPublicCaller) GetPrincipalID() string  { return "public" }
func (d *defaultPublicCaller) GetTenantID() string   { return d.tenant }
func (d *defaultPublicCaller) IsAuthenticated() bool { return true }
