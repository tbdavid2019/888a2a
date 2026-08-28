package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/labstack/echo/v5"
	"github.com/pkg/errors"

	a2agateway "github.com/tbdavid2019/888a2a/backend/a2a"
	runtimebridge "github.com/tbdavid2019/888a2a/backend/agent/bridge"
	"github.com/tbdavid2019/888a2a/backend/common"
	"github.com/tbdavid2019/888a2a/backend/manager/api/auth"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

type a2aHTTPCaller struct {
	principalID string
	tenantID    string
}

func (c a2aHTTPCaller) GetPrincipalID() string { return c.principalID }
func (c a2aHTTPCaller) GetTenantID() string    { return c.tenantID }
func (a2aHTTPCaller) IsAuthenticated() bool    { return true }

func registerA2ARoutes(e *echo.Echo, stores *store.Store, profileExternalURL string, apiAuth *auth.APIAuthInterceptor) {
	eventManager := a2agateway.NewEventManager(stores)
	taskStore := a2agateway.NewDurableTaskStore(stores, eventManager)
	directory := a2agateway.NewDirectoryService(stores, strings.TrimRight(profileExternalURL, "/"), nil)
	directory.SetRuntimeStatusProvider(func(ctx context.Context, agentID string) a2agateway.ProviderRuntimeStatus {
		bridge, err := configuredA2ABridge(agentID)
		if err != nil {
			return a2agateway.ProviderRuntimeStatus{Readiness: "BRIDGE_REQUIRED"}
		}
		health, err := bridge.Health(ctx)
		if err != nil || !health.Ready {
			return a2agateway.ProviderRuntimeStatus{ProviderID: strings.TrimSpace(os.Getenv("A2A888_A2A_BRIDGE_PROVIDER")), Readiness: "UNAVAILABLE"}
		}
		return a2agateway.ProviderRuntimeStatus{
			ProviderID:   strings.TrimSpace(os.Getenv("A2A888_A2A_BRIDGE_PROVIDER")),
			TransportID:  bridge.ID(),
			Readiness:    "READY",
			Automatic:    true,
			Capabilities: []string{"push", "stream", "cancel"},
		}
	})
	gateway := a2agateway.NewGateway(a2agateway.GatewayOptions{
		TaskStore:    taskStore,
		EventManager: eventManager,
		Directory:    directory,
		BaseURL:      strings.TrimRight(profileExternalURL, "/"),
		ExecutorFactory: func(agentID string) a2asrv.AgentExecutor {
			bridge, err := configuredA2ABridge(agentID)
			if err != nil {
				return a2agateway.NewUnavailableAgentExecutor(agentID)
			}
			executor, err := a2agateway.NewBridgeAgentExecutor(agentID, bridge)
			if err != nil {
				return a2agateway.NewUnavailableAgentExecutor(agentID)
			}
			return executor
		},
		Authenticate: func(ctx context.Context, request *http.Request, tenant, _ string) (context.Context, error) {
			if strings.TrimSpace(tenant) == "" {
				return nil, errors.New("A2A tenant is required")
			}
			header := request.Header.Clone()
			if header.Get("X-Organization-ID") == "" && header.Get("X-Tenant-ID") == "" {
				header.Set("X-Organization-ID", tenant)
			}
			authenticated, err := apiAuth.AuthenticateHTTP(ctx, header, request.RemoteAddr)
			if err != nil {
				return nil, err
			}
			if selected, ok := common.GetOrganizationIDFromContext(authenticated); !ok || selected != tenant {
				return nil, errors.New("A2A organization does not match the request path")
			}
			identity, ok := common.GetRequesterPrincipalFromContext(authenticated)
			if !ok || identity.ID == "" {
				identity, ok = common.GetExecutorPrincipalFromContext(authenticated)
			}
			if !ok || identity.ID == "" || identity.OrganizationID != tenant {
				return nil, errors.New("A2A caller identity is unavailable")
			}
			return a2agateway.WithCaller(authenticated, a2aHTTPCaller{principalID: identity.ID, tenantID: tenant}), nil
		},
	})

	e.Any("/.well-known/agent-card.json", echo.WrapHandler(gateway))
	e.Any("/a2a/v1/*", echo.WrapHandler(gateway))
}

// configuredA2ABridge returns an explicitly opted-in local runtime bridge.
// Manager never guesses a provider, executable path, or tenant binding.
func configuredA2ABridge(agentID string) (a2agateway.AgentBridge, error) {
	target := strings.TrimSpace(os.Getenv("A2A888_A2A_BRIDGE_AGENT_ID"))
	if target == "" {
		target = "default"
	}
	if agentID != target {
		return nil, errors.New("no bridge configured for agent")
	}
	providerID := strings.ToLower(strings.TrimSpace(os.Getenv("A2A888_A2A_BRIDGE_PROVIDER")))
	switch providerID {
	case "codex":
		workdir := strings.TrimSpace(os.Getenv("A2A888_A2A_BRIDGE_WORKDIR"))
		if workdir == "" || !filepath.IsAbs(workdir) {
			return nil, errors.New("A2A888_A2A_BRIDGE_WORKDIR must be an absolute path")
		}
		return runtimebridge.NewCodexACPBridge(runtimebridge.CodexACPBridgeConfig{
			ID: "codex-acp2", WorkingDir: workdir, Model: strings.TrimSpace(os.Getenv("A2A888_CODEX_MODEL")),
		})
	case "agy", "antigravity":
		return configuredCommandBridge(a2agateway.NewAgyCommandBridge)
	case "openclaw":
		baseURL := strings.TrimSpace(os.Getenv("A2A888_OPENCLAW_GATEWAY_URL"))
		if baseURL == "" {
			return nil, errors.New("A2A888_OPENCLAW_GATEWAY_URL is required")
		}
		return a2agateway.NewOpenClawBridge(a2agateway.OpenClawBridgeConfig{
			ID:      "openclaw-gateway",
			BaseURL: baseURL,
			AgentID: strings.TrimSpace(os.Getenv("A2A888_OPENCLAW_AGENT_ID")),
			Token: func(context.Context) (string, error) {
				token := strings.TrimSpace(os.Getenv("A2A888_OPENCLAW_GATEWAY_TOKEN"))
				if token == "" {
					return "", errors.New("A2A888_OPENCLAW_GATEWAY_TOKEN is required")
				}
				return token, nil
			},
		})
	default:
		return nil, errors.New("A2A888_A2A_BRIDGE_PROVIDER is not configured")
	}
}

func configuredCommandBridge(factory func(string) (*a2agateway.CommandBridge, error)) (a2agateway.AgentBridge, error) {
	workdir := strings.TrimSpace(os.Getenv("A2A888_A2A_BRIDGE_WORKDIR"))
	if workdir == "" || !filepath.IsAbs(workdir) {
		return nil, errors.New("A2A888_A2A_BRIDGE_WORKDIR must be an absolute path")
	}
	return factory(workdir)
}
