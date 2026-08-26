package daemon

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"github.com/tbdavid2019/888a2a/backend/agent/chattools"
)

// workspaceFor returns the calling agent's persistent working directory
// (<data root>/<machineID>/<agentID>/), the same directory the executor runs
// the agent's shell in. File commands confine local paths to the temp
// subdirectory of this workspace so transient upload/download files never
// clutter the agent's persistent files.
func (s *Server) workspaceFor(agent string) (string, error) {
	agentID := bareAgentID(agent)
	if agentID == "" || agentID == "." || agentID == ".." || strings.ContainsAny(agentID, `/\`) {
		return "", errors.New("agent is required to resolve the file workspace")
	}
	return filepath.Join(s.homeDir, s.machineResourceID, agentID), nil
}

// fileWorkspace resolves the calling agent's working directory, its temp jail
// (<data root>/<machineID>/<agentID>/temp/), and the base for relative paths:
// the CLI process's cwd when available (normally the agent's working
// directory), falling back to the working directory itself.
func (s *Server) fileWorkspace(req Request) (tempDir, base string, err error) {
	workspace, err := s.workspaceFor(req.Agent)
	if err != nil {
		return "", "", err
	}
	tempDir = filepath.Join(workspace, "temp")
	if err := os.MkdirAll(tempDir, 0o700); err != nil {
		return "", "", errors.Wrap(err, "create agent temp workspace")
	}
	base = req.Cwd
	if base == "" || !filepath.IsAbs(base) {
		base = workspace
	}
	return tempDir, base, nil
}

// validateWorkspacePath resolves path against base (the calling CLI process's
// cwd, normally the agent's working directory) and ensures the
// symlink-resolved result stays inside jail (the agent's temp workspace). This
// prevents file commands from reading/writing outside the temp jail.
//
// The previous version, when EvalSymlinks failed (a not-yet-existing download
// target), fell back to the lexical cleaned path. A dangling symlink inside
// the jail pointing outside it (jail/evil → /etc/laelia-shell) then
// passed the ".." check (rel == "evil") and a subsequent os.WriteFile followed
// the symlink out of the jail.
//
// Hardening:
//   - If the final component exists and is a symlink, refuse outright (a write
//     would follow it).
//   - If the final component exists and is a regular file/dir, resolve all
//     symlinks in the full path and confirm the result is inside the jail.
//   - If the final component does not exist (fresh target), resolve the parent
//     directory's symlinks and confirm the parent is inside the jail; the leaf
//     cannot itself be a symlink because it does not exist.
func validateWorkspacePath(base, jail, path string) (string, error) {
	if path == "" {
		return "", errors.New("path is required")
	}
	resolvedJail, err := filepath.EvalSymlinks(jail)
	if err != nil {
		return "", errors.Errorf("failed to resolve workspace jail %q: %v", jail, err)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	cleaned := filepath.Clean(path)

	fi, err := os.Lstat(cleaned)
	switch {
	case err == nil:
		if fi.Mode()&os.ModeSymlink != 0 {
			// A symlink at the leaf — dangling or not — would be followed by a
			// write, so refuse rather than risk escaping the jail.
			return "", errors.Errorf("path %q is a symlink; refusing to follow it outside the workspace", path)
		}
		// Existing regular file/dir: resolve the whole path and confirm it
		// stays inside the jail.
		resolved, lerr := filepath.EvalSymlinks(cleaned)
		if lerr != nil {
			return "", errors.Errorf("failed to resolve path %q: %v", path, lerr)
		}
		if !insideWorkspace(resolvedJail, resolved) {
			return "", errors.Errorf("path %q escapes the agent workspace", path)
		}
		return resolved, nil
	case errors.Is(err, os.ErrNotExist):
		// Fresh target (e.g. a download destination). The leaf cannot be a
		// symlink, but an ancestor might be — resolve the parent and confirm
		// it is inside the jail, then rejoin the leaf onto the resolved parent.
		parent := filepath.Dir(cleaned)
		parentResolved, perr := filepath.EvalSymlinks(parent)
		if perr != nil {
			return "", errors.Errorf("failed to resolve parent directory %q: %v", parent, perr)
		}
		if !insideWorkspace(resolvedJail, parentResolved) {
			return "", errors.Errorf("path %q escapes the agent workspace", path)
		}
		return filepath.Join(parentResolved, filepath.Base(cleaned)), nil
	default:
		return "", errors.Errorf("failed to stat path %q: %v", path, err)
	}
}

// insideWorkspace reports whether target is at or below jail once both are
// cleaned. It does not itself resolve symlinks; callers resolve first.
func insideWorkspace(jail, target string) bool {
	rel, err := filepath.Rel(jail, target)
	if err != nil {
		return false
	}
	// ".." escapes; "../anything" escapes. A leaf literally named "..foo" does
	// not (it has no separator after the leading dots).
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (s *Server) handleFileUpload(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(req Request) (string, *chattools.Error) {
		tempDir, base, werr := s.fileWorkspace(req)
		if werr != nil {
			return "", &chattools.Error{Code: "INVALID_ARGUMENT_FAILED", Message: werr.Error()}
		}
		localPath, err := validateWorkspacePath(base, tempDir, req.LocalPath)
		if err != nil {
			return "", &chattools.Error{Code: "INVALID_ARGUMENT_FAILED", Message: err.Error()}
		}
		data, err := os.ReadFile(localPath)
		if err != nil {
			return "", &chattools.Error{Code: "INVALID_ARGUMENT_FAILED", Message: "failed to read local file: " + err.Error()}
		}
		originalName := req.OriginalName
		if originalName == "" {
			originalName = filepath.Base(localPath)
		}
		text, err := chattools.UploadFile(r.Context(), s.deps(req), chattools.UploadFileInput{
			Conversation: req.Conversation,
			OriginalName: originalName,
			MimeType:     req.MimeType,
			Data:         data,
		})
		return text, asChatError(err)
	})
}

func (s *Server) handleFileDownload(w http.ResponseWriter, r *http.Request) {
	if e := s.authorize(r); e != nil {
		writeError(w, e)
		return
	}
	var req Request
	if !s.decode(w, r, &req) {
		return
	}
	result, err := chattools.DownloadFile(r.Context(), s.deps(req), chattools.DownloadFileInput{
		FileID: req.FileID,
	})
	if err != nil {
		writeError(w, asChatError(err))
		return
	}
	tempDir, base, werr := s.fileWorkspace(req)
	if werr != nil {
		writeError(w, &chattools.Error{Code: "INVALID_ARGUMENT_FAILED", Message: werr.Error()})
		return
	}
	outPath := req.OutPath
	if outPath == "" {
		outPath = filepath.Join(tempDir, result.Name)
	}
	resolved, verr := validateWorkspacePath(base, tempDir, outPath)
	if verr != nil {
		writeError(w, &chattools.Error{Code: "INVALID_ARGUMENT_FAILED", Message: verr.Error()})
		return
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o700); err != nil {
		writeError(w, &chattools.Error{Code: "SERVER_5XX", Message: "failed to create target dir: " + err.Error()})
		return
	}
	if err := os.WriteFile(resolved, result.Data, 0o600); err != nil {
		writeError(w, &chattools.Error{Code: "SERVER_5XX", Message: "failed to write file: " + err.Error()})
		return
	}
	writeOK(w, result.Text+"\nWrote to "+resolved)
}

func (s *Server) handleFileList(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(req Request) (string, *chattools.Error) {
		text, err := chattools.ListFiles(r.Context(), s.deps(req), chattools.ListFilesInput{
			Conversation: req.Conversation,
		})
		return text, asChatError(err)
	})
}
