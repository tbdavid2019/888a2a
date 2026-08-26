package messageplane

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWuKongIMExternalReadinessGate is the opt-in production boundary check.
// It skips when no controlled WuKongIM endpoint is configured; a fake server
// cannot prove the selected engine is deployed and ready.
func TestWuKongIMExternalReadinessGate(t *testing.T) {
	baseURL := os.Getenv("A2A888_WUKONGIM_URL")
	if baseURL == "" {
		t.Skip("set A2A888_WUKONGIM_URL to a controlled internal WuKongIM endpoint")
	}
	adapter, err := NewWuKongIMAdapter(WuKongIMConfig{BaseURL: baseURL})
	require.NoError(t, err)
	health, err := adapter.Health(context.Background())
	require.NoError(t, err)
	require.True(t, health.Healthy)
}
