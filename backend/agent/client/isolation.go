package client

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pkgerrors "github.com/pkg/errors"

	"github.com/tbdavid2019/888a2a/backend/agent/executor"
	"github.com/tbdavid2019/888a2a/backend/agent/home"
)

var (
	ErrPathEscape       = pkgerrors.New("path escapes agent workspace confinement")
	ErrCrossAgentAccess = pkgerrors.New("cross-agent access is denied")
)

// ConfinePathToAgentWorkspace verifies that a requested path resolves strictly
// inside the agent's dedicated workspace directory, preventing directory traversal
// and symlink escapes.
func ConfinePathToAgentWorkspace(machineID, agentID, requestedPath string) (string, error) {
	if strings.TrimSpace(machineID) == "" || strings.TrimSpace(agentID) == "" {
		return "", pkgerrors.New("machineID and agentID are required")
	}

	baseDir := filepath.Clean(executor.AgentWorkingDir(machineID, agentID))
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return "", pkgerrors.Wrap(err, "failed to initialize agent workspace directory")
	}

	var target string
	if filepath.IsAbs(requestedPath) {
		target = filepath.Clean(requestedPath)
	} else {
		target = filepath.Clean(filepath.Join(baseDir, requestedPath))
	}

	// Lexical confinement check.
	rel, err := filepath.Rel(baseDir, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", pkgerrors.Wrapf(ErrPathEscape, "target path %q is outside workspace %q", target, baseDir)
	}

	// Symlink resolution check for existing paths.
	if realTarget, err := filepath.EvalSymlinks(target); err == nil {
		realBase, err := filepath.EvalSymlinks(baseDir)
		if err == nil {
			realRel, err := filepath.Rel(realBase, realTarget)
			if err != nil || realRel == ".." || strings.HasPrefix(realRel, ".."+string(filepath.Separator)) {
				return "", pkgerrors.Wrapf(ErrPathEscape, "symlink target %q escapes workspace %q", realTarget, realBase)
			}
		}
	}

	return target, nil
}

// BuildIsolatedEnvironment constructs an environment variable slice restricted
// to the designated agent, avoiding any leakage of peer credentials.
func BuildIsolatedEnvironment(agentID, daemonSocket, sessionToken, binaryDir string, extraEnv []string) []string {
	env := []string{
		fmt.Sprintf("A2A888_AGENT=%s", agentID),
		fmt.Sprintf("A2A888_DAEMON_SOCKET=%s", daemonSocket),
		fmt.Sprintf("A2A888_SESSION_TOKEN=%s", sessionToken),
	}

	path := binaryDir
	if existingPath := os.Getenv("PATH"); existingPath != "" {
		if path != "" {
			path = path + string(os.PathListSeparator) + existingPath
		} else {
			path = existingPath
		}
	}
	if path != "" {
		env = append(env, fmt.Sprintf("PATH=%s", path))
	}

	if homeDir := os.Getenv(home.EnvDir); homeDir != "" {
		env = append(env, fmt.Sprintf("%s=%s", home.EnvDir, homeDir))
	}

	// Add safe allowed extra env items that do not attempt to override agent identity.
	for _, e := range extraEnv {
		if strings.HasPrefix(e, "A2A888_AGENT=") || strings.HasPrefix(e, "A2A888_SESSION_TOKEN=") {
			continue
		}
		env = append(env, e)
	}

	return env
}

// AssertAgentOwnership verifies that an operation originating from sourceAgent
// is allowed to access targetAgent resources (strict self-access only).
func AssertAgentOwnership(sourceAgentID, targetAgentID string) error {
	if bareAgentID(sourceAgentID) != bareAgentID(targetAgentID) {
		return pkgerrors.Wrapf(ErrCrossAgentAccess, "agent %q cannot access agent %q", sourceAgentID, targetAgentID)
	}
	return nil
}
