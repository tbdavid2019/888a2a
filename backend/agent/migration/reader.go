// Package migration contains bounded readers for state created by the former
// product. New runtime code must write only the 888a2a home.
package migration

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	legacyEnvDir    = "LAE" + "LIA_HOME"
	legacyBaseDir   = ".lae" + "lia"
	newBaseDir      = ".888a2a"
	MigrationMarker = ".migration-complete"
)

// LegacyHomeEnv returns the name of the supported legacy home override.
func LegacyHomeEnv() string { return legacyEnvDir }

// DefaultLegacyHome returns the old default home below userHome.
func DefaultLegacyHome(userHome string) string {
	return filepath.Join(userHome, legacyBaseDir)
}

// TargetHomeForLegacy maps the old default or custom home to the new sibling
// location. This mapping is used only during the one-time import.
func TargetHomeForLegacy(source string) string {
	if filepath.Base(source) == legacyBaseDir {
		return filepath.Join(filepath.Dir(source), newBaseDir)
	}
	return source + ".888a2a"
}

// ImportLegacyHome atomically copies an old machine home into target. It
// refuses symlinks, never overwrites an existing target, and records a marker
// inside the imported tree so the compatibility window is one-time.
func ImportLegacyHome(source, target string) error {
	source, err := filepath.Abs(source)
	if err != nil {
		return fmt.Errorf("resolve legacy home: %w", err)
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve target home: %w", err)
	}
	if source == target {
		return errors.New("legacy and target homes must differ")
	}
	if _, err := os.Stat(filepath.Join(target, MigrationMarker)); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("check migration marker: %w", err)
	}
	if info, err := os.Stat(target); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("target home is not a directory: %s", target)
		}
		// A new home may already contain state. Never merge or overwrite it.
		return writeMarker(target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("check target home: %w", err)
	}
	if info, err := os.Stat(source); err != nil {
		return fmt.Errorf("stat legacy home: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("legacy home is not a directory: %s", source)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return fmt.Errorf("create target parent: %w", err)
	}
	tmp, err := os.MkdirTemp(filepath.Dir(target), ".888a2a-migrate-")
	if err != nil {
		return fmt.Errorf("create migration staging directory: %w", err)
	}
	defer os.RemoveAll(tmp)
	if err := copyTree(source, tmp); err != nil {
		return err
	}
	if err := writeMarker(tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return writeMarker(target)
		}
		return fmt.Errorf("publish migrated home: %w", err)
	}
	return nil
}

func copyTree(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, rel)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("legacy home contains unsupported symlink: %s", rel)
		}
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o700)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read legacy state %s: %w", rel, err)
		}
		if err := os.WriteFile(destination, data, 0o600); err != nil {
			return fmt.Errorf("write migrated state %s: %w", rel, err)
		}
		return nil
	})
}

func writeMarker(root string) error {
	return os.WriteFile(filepath.Join(root, MigrationMarker), []byte("888a2a migration complete\n"), 0o600)
}

// IsLegacyIdentifier reports whether value is an old product identifier.
// It is intentionally small and used by the repository zero-residual gate.
func IsLegacyIdentifier(value string) bool {
	return strings.Contains(strings.ToLower(value), legacyBaseDir[1:])
}
