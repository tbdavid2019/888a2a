package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRetentionRequiresBoundedInput(t *testing.T) {
	var services *Store
	_, err := services.RedactExpiredConnectorEvents(context.Background(), "", time.Now(), 10)
	require.Error(t, err)
}

func TestRetentionHoldRequiresReason(t *testing.T) {
	var services *Store
	require.Error(t, services.AddRetentionHold(context.Background(), "org-a", "connector_event", "install:event", ""))
}
