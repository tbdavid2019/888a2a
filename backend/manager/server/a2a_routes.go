package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/pkg/errors"

	a2agateway "github.com/tbdavid2019/888a2a/backend/a2a"
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
	gateway := a2agateway.NewGateway(a2agateway.GatewayOptions{
		TaskStore:    taskStore,
		EventManager: eventManager,
		Directory:    directory,
		BaseURL:      strings.TrimRight(profileExternalURL, "/"),
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
