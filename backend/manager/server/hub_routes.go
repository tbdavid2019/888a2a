package server

import (
	"context"
	"time"

	"github.com/labstack/echo/v5"

	a2agateway "github.com/tbdavid2019/888a2a/backend/a2a"
)

func registerHubRoutes(e *echo.Echo, registry *a2agateway.HubRegistry, mailbox a2agateway.HubMailbox, shutdown func(context.Context) error) {
	e.Any("/hub/v1/*", echo.WrapHandler(a2agateway.HubHTTPHandler{Registry: registry, Mailbox: mailbox, Rate: a2agateway.NewHubRateLimiter(registry.MaxTasksPerMinute(), time.Minute), Shutdown: shutdown}))
}
