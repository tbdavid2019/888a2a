// Package home resolves the 888a2a data root directory. All machine-local
// state (machine.json, daemon.sock, agent workspaces, pi cache) lives under
// this root.
package home

import (
	"os"
	"path/filepath"

	"github.com/tbdavid2019/888a2a/backend/agent/migration"
)

// EnvDir is the environment variable that overrides the 888a2a data root.
const (
	EnvDir = "A2A888_HOME"
)

// Dir returns the 888a2a data root directory. A legacy home is imported once
// when no current home override is set; all subsequent writes use the new root.
func Dir() string {
	if d := os.Getenv(EnvDir); d != "" {
		if abs, err := filepath.Abs(d); err == nil {
			return abs
		}
		return d
	}
	h, err := os.UserHomeDir()
	if err != nil {
		h = os.Getenv("HOME")
	}
	targetDir := filepath.Join(h, ".888a2a")
	if _, err := os.Stat(targetDir); err == nil {
		return targetDir
	}
	legacyDir := migration.DefaultLegacyHome(h)
	if d := os.Getenv(migration.LegacyHomeEnv()); d != "" {
		legacyDir = d
		targetDir = migration.TargetHomeForLegacy(d)
	}
	if _, err := os.Stat(legacyDir); err == nil {
		if err := migration.ImportLegacyHome(legacyDir, targetDir); err == nil {
			return targetDir
		}
		// Do not start with an empty new home when migration is unsafe or
		// incomplete; preserving access to the old state is fail-safe.
		return legacyDir
	}
	return targetDir
}

// Join returns a path under the 888a2a data root.
func Join(elem ...string) string {
	return filepath.Join(append([]string{Dir()}, elem...)...)
}
