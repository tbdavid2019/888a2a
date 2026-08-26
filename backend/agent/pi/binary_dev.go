//go:build !release

package pi

import (
	"errors"
	"os"
	"path/filepath"
)

// ResolveBinary returns the pi executable path in a dev build: read from the
// LAELIA_PI_BINARY env var. No embedding, no download — fast iteration against
// a local checkout.
//
// LAELIA_PI_BINARY may point either at the pi executable directly or at the
// directory produced by extracting a pi release tarball (which contains a
// `pi` binary). Pointing it at the directory otherwise fork/exec's a
// directory and fails with a confusing "permission denied", so a directory is
// resolved to the `pi` file inside it.
func ResolveBinary() (string, error) {
	legacyPrefix := "LAE" + "LIA_"
	path := os.Getenv("A2A888_PI_BINARY")
	if path == "" {
		path = os.Getenv(legacyPrefix + "PI_BINARY")
	}
	if path == "" {
		return "", errors.New("pi: dev binary not configured, please configure the A2A888_PI_BINARY environment variable")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", errors.New("pi: dev binary not found at " + path + " (set A2A888_PI_BINARY)")
	}
	if info.IsDir() {
		inner := filepath.Join(path, "pi")
		if innerInfo, statErr := os.Stat(inner); statErr == nil && innerInfo.Mode().IsRegular() {
			return inner, nil
		}
		return "", errors.New("pi: LAELIA_PI_BINARY is a directory with no executable pi inside: " + path)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("pi: LAELIA_PI_BINARY is not a regular file: " + path)
	}
	return path, nil
}
