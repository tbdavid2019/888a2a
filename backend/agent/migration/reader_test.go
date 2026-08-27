package migration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImportLegacyHomePreservesMachineSessionAndWorkspace(t *testing.T) {
	source := t.TempDir()
	target := filepath.Join(t.TempDir(), ".888a2a")
	files := map[string]string{
		"machine.json":                              `{"machine_id":"machines/m1","refresh_token":"secret"}`,
		"machines/m1/agents/a1/acp-session.json":    `{"session_id":"session-1"}`,
		"machines/m1/agents/a1/workspace/README.md": "workspace survives",
		"machines/m1/agents/a1/context-state.json":  `{"fingerprint":"fp-1"}`,
	}
	for name, contents := range files {
		path := filepath.Join(source, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	}

	require.NoError(t, ImportLegacyHome(source, target))
	for name, want := range files {
		got, err := os.ReadFile(filepath.Join(target, name))
		require.NoError(t, err, name)
		require.Equal(t, want, string(got), name)
	}

	marker, err := os.Stat(filepath.Join(target, MigrationMarker))
	require.NoError(t, err)
	require.False(t, marker.IsDir())
}

func TestImportLegacyHomeDoesNotOverwriteExistingTarget(t *testing.T) {
	source := t.TempDir()
	target := filepath.Join(t.TempDir(), ".888a2a")
	require.NoError(t, os.MkdirAll(source, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(source, "machine.json"), []byte("old"), 0o600))
	require.NoError(t, os.MkdirAll(target, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(target, "machine.json"), []byte("new"), 0o600))

	require.NoError(t, ImportLegacyHome(source, target))
	got, err := os.ReadFile(filepath.Join(target, "machine.json"))
	require.NoError(t, err)
	require.Equal(t, "new", string(got))
}
