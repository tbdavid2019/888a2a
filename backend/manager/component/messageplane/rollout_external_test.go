package messageplane

import (
	"context"

	"github.com/stretchr/testify/require"
	"testing"
)

func TestPostgresCollaborationRolloutSwitchAndRollback(t *testing.T) {
	db := requireMessagePlaneDatabase(t)
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `
		INSERT INTO organizations (id, name, slug) VALUES ('org-rollout-b', 'Rollout B', 'rollout-b')
	`)
	require.NoError(t, err)
	selector, err := NewPathSelector(db)
	require.NoError(t, err)

	mode, err := selector.Mode(ctx, "default")
	require.NoError(t, err)
	require.Equal(t, PathModeLegacy, mode)
	require.NoError(t, selector.SetMode(ctx, "default", PathModeDual))
	mode, err = selector.Mode(ctx, "default")
	require.NoError(t, err)
	require.Equal(t, PathModeDual, mode)

	require.NoError(t, selector.SetMode(ctx, "org-rollout-b", PathModeMessagePlane))
	mode, err = selector.Mode(ctx, "org-rollout-b")
	require.NoError(t, err)
	require.Equal(t, PathModeMessagePlane, mode)
	mode, err = selector.Mode(ctx, "default")
	require.NoError(t, err)
	require.Equal(t, PathModeDual, mode)

	// An operator can roll this tenant back independently without changing the
	// rollout mode of another Organization.
	require.NoError(t, selector.SetMode(ctx, "org-rollout-b", PathModeLegacy))
	mode, err = selector.Mode(ctx, "org-rollout-b")
	require.NoError(t, err)
	require.Equal(t, PathModeLegacy, mode)
	mode, err = selector.Mode(ctx, "default")
	require.NoError(t, err)
	require.Equal(t, PathModeDual, mode)
}
