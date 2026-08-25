package a2a

import (
	"os"
	"path/filepath"
	"strings"

	acp "github.com/coder/acp-go-sdk"
	"github.com/pkg/errors"
)

// ActionKind represents a categorized runtime capability request.
type ActionKind string

const (
	ActionRead         ActionKind = "READ"
	ActionWrite        ActionKind = "WRITE"
	ActionShell        ActionKind = "SHELL"
	ActionNetwork      ActionKind = "NETWORK"
	ActionSecret       ActionKind = "SECRET"
	ActionMCP          ActionKind = "MCP"
	ActionUnclassified ActionKind = "UNCLASSIFIED"
)

// PolicyDecision represents the allow/deny outcome of a policy evaluation.
type PolicyDecision string

const (
	DecisionAllow PolicyDecision = "ALLOWED"
	DecisionDeny  PolicyDecision = "DENIED"
)

// RuntimePolicy defines focused runtime safety rules for an Agent.
// Default configurations deny shell, write, network, secret and side-effecting MCP.
type RuntimePolicy struct {
	TenantID            string   `json:"tenant_id"`
	AgentID             string   `json:"agent_id"`
	AllowedRoots        []string `json:"allowed_roots"`
	AllowWorkspaceRead  bool     `json:"allow_workspace_read"`
	AllowWorkspaceWrite bool     `json:"allow_workspace_write"`
	AllowedCommands     []string `json:"allowed_commands"`
	AllowNetwork        bool     `json:"allow_network"`
	AllowSecrets        bool     `json:"allow_secrets"`
	AllowedMCPServers   []string `json:"allowed_mcp_servers"`
	AllowedMCPTools     []string `json:"allowed_mcp_tools"`
}

// DefaultRuntimePolicy creates a safe default policy: read allowed in workspace, high-risk actions denied.
func DefaultRuntimePolicy(agentID string, workspaceRoots []string) *RuntimePolicy {
	return &RuntimePolicy{
		AgentID:             agentID,
		AllowedRoots:        workspaceRoots,
		AllowWorkspaceRead:  true,
		AllowWorkspaceWrite: false,
		AllowedCommands:     nil,
		AllowNetwork:        false,
		AllowSecrets:        false,
		AllowedMCPServers:   nil,
		AllowedMCPTools:     nil,
	}
}

// PermissionRequest represents an incoming permission probe.
type PermissionRequest struct {
	TenantID    string     `json:"tenant_id"`
	WorkID      string     `json:"work_id"`
	AgentID     string     `json:"agent_id"`
	PeerAgentID string     `json:"peer_agent_id"`
	ActionKind  ActionKind `json:"action_kind"`
	ToolName    string     `json:"tool_name"`
	TargetPath  string     `json:"target_path"`
	Command     string     `json:"command"`
	MCPServer   string     `json:"mcp_server"`
	MCPTool     string     `json:"mcp_tool"`
	Description string     `json:"description"`
}

// PermissionResult captures the policy evaluation outcome.
type PermissionResult struct {
	Decision      PolicyDecision `json:"decision"`
	Reason        string         `json:"reason"`
	ActionSummary string         `json:"action_summary"`
	CanonicalPath string         `json:"canonical_path,omitempty"`
}

// Evaluate applies focused runtime policy rules against a permission request.
func (p *RuntimePolicy) Evaluate(req PermissionRequest) PermissionResult {
	if p == nil {
		return PermissionResult{
			Decision:      DecisionDeny,
			Reason:        "no runtime policy configured; denying by default",
			ActionSummary: string(req.ActionKind),
		}
	}

	switch req.ActionKind {
	case ActionRead:
		if !p.AllowWorkspaceRead {
			return PermissionResult{
				Decision:      DecisionDeny,
				Reason:        "workspace read is disabled by policy",
				ActionSummary: "read: " + req.TargetPath,
			}
		}
		if req.TargetPath == "" {
			return PermissionResult{
				Decision:      DecisionDeny,
				Reason:        "target path cannot be empty for read action",
				ActionSummary: "read: <empty>",
			}
		}
		canonical, err := ValidatePathConfinement(req.TargetPath, p.AllowedRoots)
		if err != nil {
			return PermissionResult{
				Decision:      DecisionDeny,
				Reason:        "path confinement violation: " + err.Error(),
				ActionSummary: "read: " + req.TargetPath,
			}
		}
		return PermissionResult{
			Decision:      DecisionAllow,
			Reason:        "read permitted within workspace root",
			ActionSummary: "read: " + canonical,
			CanonicalPath: canonical,
		}

	case ActionWrite:
		if !p.AllowWorkspaceWrite {
			return PermissionResult{
				Decision:      DecisionDeny,
				Reason:        "filesystem write denied by default runtime policy",
				ActionSummary: "write: " + req.TargetPath,
			}
		}
		if req.TargetPath == "" {
			return PermissionResult{
				Decision:      DecisionDeny,
				Reason:        "target path cannot be empty for write action",
				ActionSummary: "write: <empty>",
			}
		}
		canonical, err := ValidatePathConfinement(req.TargetPath, p.AllowedRoots)
		if err != nil {
			return PermissionResult{
				Decision:      DecisionDeny,
				Reason:        "path confinement violation: " + err.Error(),
				ActionSummary: "write: " + req.TargetPath,
			}
		}
		return PermissionResult{
			Decision:      DecisionAllow,
			Reason:        "filesystem write permitted within workspace root",
			ActionSummary: "write: " + canonical,
			CanonicalPath: canonical,
		}

	case ActionShell:
		if len(p.AllowedCommands) == 0 {
			return PermissionResult{
				Decision:      DecisionDeny,
				Reason:        "shell execution denied by default runtime policy",
				ActionSummary: "shell: " + req.Command,
			}
		}
		for _, allowed := range p.AllowedCommands {
			if allowed == "*" || req.Command == allowed || strings.HasPrefix(req.Command, allowed+" ") {
				return PermissionResult{
					Decision:      DecisionAllow,
					Reason:        "shell command matches allowlisted prefix: " + allowed,
					ActionSummary: "shell: " + req.Command,
				}
			}
		}
		return PermissionResult{
			Decision:      DecisionDeny,
			Reason:        "shell command not in allowlisted command list",
			ActionSummary: "shell: " + req.Command,
		}

	case ActionNetwork:
		if !p.AllowNetwork {
			return PermissionResult{
				Decision:      DecisionDeny,
				Reason:        "network access denied by default runtime policy",
				ActionSummary: "network: " + req.ToolName,
			}
		}
		return PermissionResult{
			Decision:      DecisionAllow,
			Reason:        "network access permitted by policy",
			ActionSummary: "network: " + req.ToolName,
		}

	case ActionSecret:
		if !p.AllowSecrets {
			return PermissionResult{
				Decision:      DecisionDeny,
				Reason:        "secret and credential access denied by default runtime policy",
				ActionSummary: "secret: " + req.ToolName,
			}
		}
		return PermissionResult{
			Decision:      DecisionAllow,
			Reason:        "secret access permitted by policy",
			ActionSummary: "secret: " + req.ToolName,
		}

	case ActionMCP:
		if len(p.AllowedMCPServers) == 0 && len(p.AllowedMCPTools) == 0 {
			return PermissionResult{
				Decision:      DecisionDeny,
				Reason:        "side-effecting MCP action denied by default runtime policy",
				ActionSummary: "mcp: " + req.MCPServer + "/" + req.MCPTool,
			}
		}
		for _, s := range p.AllowedMCPServers {
			if s == "*" || s == req.MCPServer {
				return PermissionResult{
					Decision:      DecisionAllow,
					Reason:        "MCP server allowlisted: " + req.MCPServer,
					ActionSummary: "mcp: " + req.MCPServer + "/" + req.MCPTool,
				}
			}
		}
		for _, t := range p.AllowedMCPTools {
			if t == "*" || t == req.MCPTool {
				return PermissionResult{
					Decision:      DecisionAllow,
					Reason:        "MCP tool allowlisted: " + req.MCPTool,
					ActionSummary: "mcp: " + req.MCPServer + "/" + req.MCPTool,
				}
			}
		}
		return PermissionResult{
			Decision:      DecisionDeny,
			Reason:        "MCP server or tool not in allowlist",
			ActionSummary: "mcp: " + req.MCPServer + "/" + req.MCPTool,
		}

	case ActionUnclassified:
		fallthrough
	default:
		return PermissionResult{
			Decision:      DecisionDeny,
			Reason:        "unclassified high-risk action denied by default runtime policy",
			ActionSummary: string(req.ActionKind) + ": " + req.ToolName,
		}
	}
}

// ValidatePathConfinement checks that path is strictly within one of the allowed workspace roots.
// It canonicalizes symlinks on Linux and macOS, prevents ../ traversal, and rejects escaping symlinks.
func ValidatePathConfinement(path string, allowedRoots []string) (string, error) {
	if len(allowedRoots) == 0 {
		return "", errors.New("no workspace roots configured")
	}

	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		return "", errors.Errorf("path must be absolute: %s", path)
	}

	fi, err := os.Lstat(cleaned)
	switch {
	case err == nil:
		if fi.Mode()&os.ModeSymlink != 0 {
			// Resolve symlink target and verify it stays inside allowed roots
			target, rerr := filepath.EvalSymlinks(cleaned)
			if rerr != nil {
				return "", errors.Wrapf(rerr, "path %s is a broken symlink", path)
			}
			if !isInsideRoots(target, allowedRoots) {
				return "", errors.Errorf("path %s is a symlink pointing outside allowed roots to %s", path, target)
			}
			return target, nil
		}
		resolved, rerr := filepath.EvalSymlinks(cleaned)
		if rerr != nil {
			return "", errors.Wrapf(rerr, "failed resolving canonical path for %s", path)
		}
		if !isInsideRoots(resolved, allowedRoots) {
			return "", errors.Errorf("path %s is outside allowed workspace roots", path)
		}
		return resolved, nil

	case errors.Is(err, os.ErrNotExist):
		// For not-yet-existing paths, verify nearest existing ancestor is inside roots
		parent := filepath.Dir(cleaned)
		parentResolved, perr := filepath.EvalSymlinks(parent)
		if perr != nil {
			// If immediate parent does not exist, walk up to nearest existing ancestor
			ancestor := parent
			for {
				next := filepath.Dir(ancestor)
				if next == ancestor {
					return "", errors.Errorf("path %s has no valid existing ancestor", path)
				}
				ancestor = next
				if aResolved, aerr := filepath.EvalSymlinks(ancestor); aerr == nil {
					if !isInsideRoots(aResolved, allowedRoots) {
						return "", errors.Errorf("path %s ancestor %s is outside allowed roots", path, aResolved)
					}
					break
				}
			}
			return cleaned, nil
		}
		if !isInsideRoots(parentResolved, allowedRoots) {
			return "", errors.Errorf("path %s parent %s is outside allowed workspace roots", path, parentResolved)
		}
		return filepath.Join(parentResolved, filepath.Base(cleaned)), nil

	default:
		return "", errors.Wrapf(err, "stat path %s", path)
	}
}

func isInsideRoots(target string, allowedRoots []string) bool {
	target = filepath.Clean(target)
	for _, root := range allowedRoots {
		if root == "" {
			continue
		}
		resolvedRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			resolvedRoot = filepath.Clean(root)
		} else {
			resolvedRoot = filepath.Clean(resolvedRoot)
		}

		if target == resolvedRoot || strings.HasPrefix(target, resolvedRoot+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

// EvaluateACPPermission inspects an ACP permission request and selects the appropriate option ID.
func (p *RuntimePolicy) EvaluateACPPermission(params acp.RequestPermissionRequest) (acp.PermissionOptionId, PolicyDecision, string) {
	req := ClassifyACPRequest(params)
	result := p.Evaluate(req)

	if result.Decision == DecisionAllow {
		for _, opt := range params.Options {
			if opt.Kind == acp.PermissionOptionKindAllowOnce || opt.Kind == acp.PermissionOptionKindAllowAlways {
				return opt.OptionId, DecisionAllow, result.Reason
			}
		}
	}

	// Decision is DENY or no allow option available: pick reject option
	for _, opt := range params.Options {
		if opt.Kind == acp.PermissionOptionKindRejectOnce || opt.Kind == acp.PermissionOptionKindRejectAlways {
			return opt.OptionId, DecisionDeny, result.Reason
		}
	}

	if len(params.Options) > 0 {
		return params.Options[0].OptionId, result.Decision, result.Reason
	}
	return "", result.Decision, result.Reason
}

// ClassifyACPRequest extracts a PermissionRequest from an ACP RequestPermissionRequest.
func ClassifyACPRequest(params acp.RequestPermissionRequest) PermissionRequest {
	req := PermissionRequest{
		ActionKind: ActionUnclassified,
	}

	if params.ToolCall.Kind != nil {
		switch *params.ToolCall.Kind {
		case acp.ToolKindRead:
			req.ActionKind = ActionRead
		case acp.ToolKindEdit:
			req.ActionKind = ActionWrite
		case acp.ToolKindExecute:
			req.ActionKind = ActionShell
		case acp.ToolKindDelete, acp.ToolKindMove, acp.ToolKindSearch, acp.ToolKindThink, acp.ToolKindFetch, acp.ToolKindSwitchMode, acp.ToolKindOther:
			req.ActionKind = ActionUnclassified
		default:
			req.ActionKind = ActionUnclassified
		}
	}

	// Extract details from ToolCall
	if params.ToolCall.Title != nil && *params.ToolCall.Title != "" {
		req.ToolName = *params.ToolCall.Title
		titleLower := strings.ToLower(*params.ToolCall.Title)
		if req.ActionKind == ActionUnclassified {
			if strings.Contains(titleLower, "read") || strings.Contains(titleLower, "view") {
				req.ActionKind = ActionRead
			} else if strings.Contains(titleLower, "write") || strings.Contains(titleLower, "edit") || strings.Contains(titleLower, "replace") {
				req.ActionKind = ActionWrite
			} else if strings.Contains(titleLower, "bash") || strings.Contains(titleLower, "shell") || strings.Contains(titleLower, "exec") || strings.Contains(titleLower, "command") {
				req.ActionKind = ActionShell
			} else if strings.Contains(titleLower, "mcp") {
				req.ActionKind = ActionMCP
			}
		}
	}

	if params.ToolCall.RawInput != nil {
		if inputMap, ok := params.ToolCall.RawInput.(map[string]any); ok {
			if p, ok := inputMap["path"].(string); ok {
				req.TargetPath = p
			} else if p, ok := inputMap["file"].(string); ok {
				req.TargetPath = p
			}
			if cmd, ok := inputMap["command"].(string); ok {
				req.Command = cmd
			}
			if s, ok := inputMap["server"].(string); ok {
				req.MCPServer = s
			}
			if t, ok := inputMap["tool"].(string); ok {
				req.MCPTool = t
			}
		}
	}

	return req
}
