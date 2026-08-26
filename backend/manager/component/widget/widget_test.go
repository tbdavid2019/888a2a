package widget

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func testWidgetService(now *time.Time, state string, config Config) *Service {
	return &Service{
		secret: []byte("widget-test-secret"), now: func() time.Time { return *now },
		lookupState:  func(context.Context, string) (string, error) { return state, nil },
		lookupConfig: func(context.Context, string, string) (Config, error) { return config, nil },
	}
}

func TestBootstrapBindsShortLivedSessionToOrganization(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	service := testWidgetService(&now, "ACTIVE", Config{OrganizationID: "org-a", WidgetID: "support", Enabled: true, SessionTTL: time.Minute})
	response, err := service.Bootstrap(context.Background(), "org-a", "support", "")
	require.NoError(t, err)
	require.Equal(t, "org-a", response.OrganizationID)
	require.False(t, response.ExpiresAt.IsZero())
	claims, err := service.VerifySession(response.SessionToken, "org-a", "support")
	require.NoError(t, err)
	require.Equal(t, "org-a", claims.OrganizationID)
	_, err = service.Bootstrap(context.Background(), "org-b", "support", response.SessionToken)
	require.Error(t, err)
}

func TestBootstrapRejectsUnknownOrInactiveOrganizationAndExpiredSession(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	config := Config{OrganizationID: "org-a", WidgetID: "support", Enabled: true, SessionTTL: time.Minute}
	unknown := testWidgetService(&now, "", config)
	unknown.lookupState = func(context.Context, string) (string, error) { return "", errOrganizationNotFound }
	_, err := unknown.Bootstrap(context.Background(), "org-a", "support", "")
	require.Error(t, err)

	inactive := testWidgetService(&now, "SUSPENDED", config)
	_, err = inactive.Bootstrap(context.Background(), "org-a", "support", "")
	require.Error(t, err)

	service := testWidgetService(&now, "ACTIVE", config)
	response, err := service.Bootstrap(context.Background(), "org-a", "support", "")
	require.NoError(t, err)
	now = now.Add(2 * time.Minute)
	_, err = service.VerifySession(response.SessionToken, "org-a", "support")
	require.Error(t, err)
}

func TestHandlerBootstrapsAndRejectsExpiredSession(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	service := testWidgetService(&now, "ACTIVE", Config{OrganizationID: "org-a", WidgetID: "support", Enabled: true, SessionTTL: time.Minute})
	handler := service.Handler()
	request := httptest.NewRequest(http.MethodPost, "/api/widget/bootstrap", strings.NewReader(`{"organization_id":"org-a","widget_id":"support"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	var bootstrap BootstrapResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&bootstrap))
	require.NotEmpty(t, bootstrap.SessionToken)

	now = now.Add(2 * time.Minute)
	existing, _ := json.Marshal(map[string]string{"organization_id": "org-a", "widget_id": "support", "session_token": bootstrap.SessionToken})
	request = httptest.NewRequest(http.MethodPost, "/api/widget/bootstrap", strings.NewReader(string(existing)))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusForbidden, response.Code)
}

var errOrganizationNotFound = &widgetTestError{"organization not found"}

type widgetTestError struct{ message string }

func (e *widgetTestError) Error() string { return e.message }
