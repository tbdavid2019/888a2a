package server

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	a2agateway "github.com/tbdavid2019/888a2a/backend/a2a"
	"github.com/tbdavid2019/888a2a/backend/common/permission"
	"github.com/tbdavid2019/888a2a/backend/manager/api/auth"
	apiv1 "github.com/tbdavid2019/888a2a/backend/manager/api/v1"
	"github.com/tbdavid2019/888a2a/backend/manager/component/iam"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

func registerHubRoutes(e *echo.Echo, registry *a2agateway.HubRegistry, mailbox a2agateway.HubMailbox, shutdown func(context.Context) error, authorizeBrowser func(*http.Request) bool) {
	var groupStore a2agateway.HubGroupStore
	if gs, ok := mailbox.(a2agateway.HubGroupStore); ok {
		groupStore = gs
	}
	e.Any("/hub/v1/*", echo.WrapHandler(a2agateway.HubHTTPHandler{
		Registry: registry, Mailbox: mailbox, Groups: groupStore, Rate: a2agateway.NewHubRateLimiter(registry.MaxTasksPerMinute(), time.Minute),
		Shutdown: shutdown, AuthorizeBrowser: authorizeBrowser,
	}))
}

func hubBrowserAuthorizer(apiAuth *auth.APIAuthInterceptor, stores *store.Store) func(*http.Request) bool {
	checker := iam.NewManager(stores)
	return func(request *http.Request) bool {
		ctx, err := apiAuth.AuthenticateHTTP(request.Context(), request.Header, request.RemoteAddr)
		if err != nil {
			return false
		}
		user, ok := apiv1.GetUserFromContext(ctx)
		if !ok || user == nil {
			return false
		}
		allowed, err := checker.CheckPermission(ctx, permission.SettingsUpdate, user, nil, nil)
		return err == nil && allowed
	}
}
