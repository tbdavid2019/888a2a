// Package home resolves the 888a2a data root directory. All machine-local
// state (machine.json, daemon.sock, agent workspaces, pi cache) lives under
// this root.
package home

import (
	"os"
	"path/filepath"
)

// EnvDir is the environment variable that overrides the 888a2a data root.
const (
	EnvDir       = "A2A888_HOME"
	LegacyEnvDir = "LAE" + "LIA_HOME"
)

// Dir returns the 888a2a data root directory. If A2A888_HOME (or legacy fallback)
// is set, it is used directly (made absolute); otherwise it defaults to ~/.888a2a.
func Dir() string {
	if d := os.Getenv(EnvDir); d != "" {
		if abs, err := filepath.Abs(d); err == nil {
			return abs
		}
		return d
	}
	if d := os.Getenv(LegacyEnvDir); d != "" {
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
	legacyDir := filepath.Join(h, ".lae"+"lia")
	if _, err := os.Stat(legacyDir); err == nil {
		if err := copyLegacyDir(legacyDir, targetDir); err == nil {
			return targetDir
		}
		// Preserve access to the existing state when migration cannot complete.
		return legacyDir
	}
	return targetDir
}

func copyLegacyDir(source, target string) error {
	tmp, err := os.MkdirTemp(filepath.Dir(target), ".888a2a-migrate-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	err = filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(tmp, rel)
		if entry.Type()&os.ModeSymlink != 0 {
			return os.ErrInvalid
		}
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o700)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(destination, data, 0o600); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		if _, statErr := os.Stat(target); statErr == nil {
			return nil
		}
		return err
	}
	return nil
}

// Join returns a path under the 888a2a data root.
func Join(elem ...string) string {
	return filepath.Join(append([]string{Dir()}, elem...)...)
}
