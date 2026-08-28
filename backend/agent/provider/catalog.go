package provider

import (
	"regexp"
	"strings"
)

// Readiness is the conservative capability state shown for a Provider family.
type Readiness string

const (
	ReadinessReady          Readiness = "READY"
	ReadinessDetectedOnly   Readiness = "DETECTED_ONLY"
	ReadinessBridgeRequired Readiness = "BRIDGE_REQUIRED"
	ReadinessPullOnly       Readiness = "PULL_ONLY"
	ReadinessUnavailable    Readiness = "UNAVAILABLE"
	ReadinessPending        Readiness = "PENDING_VERIFICATION"
)

// TransportMode identifies the boundary used to deliver work to a Provider.
type TransportMode string

const (
	TransportACP     TransportMode = "ACP"
	TransportGateway TransportMode = "GATEWAY"
	TransportCLI     TransportMode = "CLI"
	TransportMCP     TransportMode = "MCP"
	TransportPull    TransportMode = "PULL"
)

// CatalogTransport describes one explicit delivery path for a Provider.
type CatalogTransport struct {
	ID             string
	Mode           TransportMode
	Operations     []string
	RequiresBridge bool
	AutoEnabled    bool
	NativeSession  bool
}

// CatalogEntry describes a Provider family without claiming that its runtime
// is installed or ready on the current Machine.
type CatalogEntry struct {
	ID          string
	DisplayName string
	Aliases     []string
	InstallHint string
	Readiness   Readiness
	Transports  []CatalogTransport
}

// CatalogProjection is the safe catalog view consumed by Manager/UI callers.
// It intentionally contains no executable path, native session ID, or secret.
type CatalogProjection struct {
	ID            string
	DisplayName   string
	InstallHint   string
	Readiness     Readiness
	TransportID   string
	TransportMode TransportMode
	Automatic     bool
	FailureReason string
}

var catalogSecretPattern = regexp.MustCompile(`(?i)(bearer\s+|token|password|secret|api[_-]?key)\s*[=:]?\s*[^\s,;]+`)
var catalogPrivatePathPattern = regexp.MustCompile(`(?:^|[\s=(])(?:~|/[^\s,;)]+|[A-Za-z]:\\[^\s,;)]+)`)
var catalogSessionPattern = regexp.MustCompile(`(?i)(session[_-]?id|thread[_-]?id|conversation[_-]?id)\s*[=:]\s*[^\s,;]+`)

// ProjectCatalogEntry merges current host evidence into a catalog entry using
// conservative status rules. Unknown or malformed evidence never upgrades a
// Provider to READY.
func ProjectCatalogEntry(entry CatalogEntry, discovered *Discovered) CatalogProjection {
	projection := CatalogProjection{
		ID:          entry.ID,
		DisplayName: sanitizeCatalogText(entry.DisplayName),
		InstallHint: sanitizeCatalogText(entry.InstallHint),
		Readiness:   entry.Readiness,
	}
	if fallback, ok := nonAutomaticTransport(entry.Transports); ok {
		projection.TransportID = fallback.ID
		projection.TransportMode = fallback.Mode
	} else if len(entry.Transports) > 0 {
		projection.TransportID = entry.Transports[0].ID
		projection.TransportMode = entry.Transports[0].Mode
	}
	if discovered == nil || discovered.ProviderID == "" || NormalizeCatalogID(discovered.ProviderID) != entry.ID {
		if projection.Readiness == ReadinessReady {
			projection.Readiness = ReadinessPending
		}
		return projection
	}
	projection.FailureReason = sanitizeCatalogText(discovered.FailureMessage)
	switch discovered.RuntimeStatus {
	case "READY":
		projection.Readiness = ReadinessDetectedOnly
		if discovered.CompatibilityLevel == "FULL_LOOP_VERIFIED" {
			for _, candidate := range entry.Transports {
				if candidate.AutoEnabled {
					projection.Readiness = ReadinessReady
					projection.Automatic = true
					projection.TransportID = candidate.ID
					projection.TransportMode = candidate.Mode
					break
				}
			}
		}
	case "BROKEN", "QUARANTINED", "UNAVAILABLE":
		projection.Readiness = ReadinessUnavailable
	case "DETECTED", "":
		projection.Readiness = ReadinessDetectedOnly
	default:
		projection.Readiness = ReadinessPending
	}
	return projection
}

func sanitizeCatalogText(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "\r", " "), "\n", " "))
	value = catalogSecretPattern.ReplaceAllString(value, "[redacted]")
	value = catalogSessionPattern.ReplaceAllString(value, "[redacted]")
	value = catalogPrivatePathPattern.ReplaceAllString(value, "[private path]")
	if len(value) > 256 {
		value = value[:256]
	}
	return value
}

// SanitizeCatalogText is the boundary helper for machine and manager
// projections. Discovery errors can contain local paths or credential-shaped
// strings; those details must not cross the provider status API.
func SanitizeCatalogText(value string) string { return sanitizeCatalogText(value) }

func nonAutomaticTransport(transports []CatalogTransport) (CatalogTransport, bool) {
	for _, candidate := range transports {
		if !candidate.AutoEnabled {
			return candidate, true
		}
	}
	return CatalogTransport{}, false
}

// Catalog returns the canonical provider catalog. Entries are ordered for a
// stable UI and deterministic Machine responses.
func Catalog() []CatalogEntry {
	entries := []CatalogEntry{
		catalogEntry("openclaw", "OpenClaw", []string{"openclaw-cli"}, "Install OpenClaw and configure its local Gateway", ReadinessBridgeRequired,
			transport("openclaw-gateway", TransportGateway, []string{"push", "stream", "cancel"}, true, false, true),
			transport("openclaw-cli", TransportCLI, []string{"push", "pull"}, true, false, false), transport("openclaw-pull", TransportPull, []string{"pull"}, false, false, false)),
		catalogEntry("hermes", "Hermes", nil, "Install Hermes and configure a local profile", ReadinessBridgeRequired,
			transport("hermes-http", TransportGateway, []string{"push", "stream", "cancel"}, true, false, true), transport("hermes-cli", TransportCLI, []string{"push", "pull"}, true, false, false), transport("hermes-pull", TransportPull, []string{"pull"}, false, false, false)),
		catalogEntry("claude-code", "Claude Code", []string{"claude", "claude-code-acp"}, "Install Claude Code and complete local login", ReadinessReady,
			transport("claude-code-acp", TransportACP, []string{"push", "stream", "cancel"}, false, true, true), transport("claude-code-pull", TransportPull, []string{"pull"}, false, false, false)),
		catalogEntry("codex", "Codex", []string{"codex-cli"}, "Install Codex and configure CODEX_HOME", ReadinessReady,
			transport("codex-acp2", TransportACP, []string{"push", "stream", "cancel"}, false, true, true), transport("codex-pull", TransportPull, []string{"pull"}, false, false, false)),
		catalogEntry("antigravity", "Antigravity (agy)", []string{"agy", "antigravity-cli"}, "Install agy and configure a local project", ReadinessBridgeRequired,
			transport("agy-cli", TransportCLI, []string{"push", "pull"}, true, false, false), transport("agy-mcp", TransportMCP, []string{"push", "pull"}, true, false, false), transport("agy-pull", TransportPull, []string{"pull"}, false, false, false)),
		catalogEntry("deepseek-harness", "DeepSeek Harness", []string{"deepseek"}, "Configure a local DeepSeek Harness profile", ReadinessBridgeRequired,
			transport("deepseek-harness-http", TransportGateway, []string{"push", "stream", "cancel"}, true, false, true), transport("deepseek-harness-pull", TransportPull, []string{"pull"}, false, false, false)),
		catalogEntry("workbuddy", "WorkBuddy", nil, "Install WorkBuddy and configure its local endpoint", ReadinessBridgeRequired,
			transport("workbuddy-http", TransportGateway, []string{"push", "stream", "cancel"}, true, false, true), transport("workbuddy-pull", TransportPull, []string{"pull"}, false, false, false)),
		catalogEntry("qwen-office", "Qwen Office", []string{"千问办公"}, "Install Qwen Office and select an expert kit", ReadinessPending,
			transport("qwen-office-cli", TransportCLI, []string{"push", "pull"}, true, false, false), transport("qwen-office-pull", TransportPull, []string{"pull"}, false, false, false)),
		catalogEntry("dumate", "DuMate", []string{"百度搭子"}, "Install DuMate and configure a local plugin agent", ReadinessPending,
			transport("dumate-http", TransportGateway, []string{"push", "stream", "cancel"}, true, false, true), transport("dumate-pull", TransportPull, []string{"pull"}, false, false, false)),
		catalogEntry("traework", "TraeWork", []string{"trae-work", "trae"}, "Install the standalone Trae CLI", ReadinessPending,
			transport("traework-acp", TransportACP, []string{"push", "stream", "cancel"}, true, false, true), transport("traework-pull", TransportPull, []string{"pull"}, false, false, false)),
		catalogEntry("cline", "Cline", nil, "Install Cline CLI and complete local login", ReadinessPending,
			transport("cline-acp", TransportACP, []string{"push", "stream", "cancel"}, true, false, true), transport("cline-pull", TransportPull, []string{"pull"}, false, false, false)),
		catalogEntry("zeroclaw", "ZeroClaw", nil, "Install ZeroClaw and configure its loopback ACP endpoint", ReadinessPending,
			transport("zeroclaw-acp-ws", TransportACP, []string{"push", "stream", "cancel"}, true, false, true), transport("zeroclaw-pull", TransportPull, []string{"pull"}, false, false, false)),
		catalogEntry("qwen-code", "Qwen Code", []string{"qwen"}, "Install Qwen Code and complete local login", ReadinessPending,
			transport("qwen-code-cli", TransportCLI, []string{"push", "pull"}, true, false, false), transport("qwen-code-pull", TransportPull, []string{"pull"}, false, false, false)),
		catalogEntry("kiro", "Kiro CLI", []string{"kiro-cli"}, "Install Kiro CLI and configure a local profile", ReadinessPending,
			transport("kiro-cli", TransportCLI, []string{"push", "pull"}, true, false, false), transport("kiro-pull", TransportPull, []string{"pull"}, false, false, false)),
		catalogEntry("github-copilot", "GitHub Copilot CLI", []string{"copilot", "github-copilot-cli"}, "Install GitHub Copilot CLI and complete local login", ReadinessPending,
			transport("github-copilot-cli", TransportCLI, []string{"push", "pull"}, true, false, false), transport("github-copilot-pull", TransportPull, []string{"pull"}, false, false, false)),
		catalogEntry("openhands", "OpenHands", nil, "Install OpenHands CLI for explicit Pull use", ReadinessPullOnly,
			transport("openhands-pull", TransportPull, []string{"pull"}, false, false, false)),
		catalogEntry("aider", "Aider", nil, "Install Aider CLI for explicit Pull use", ReadinessPullOnly,
			transport("aider-cli", TransportCLI, []string{"push", "pull"}, true, false, false), transport("aider-pull", TransportPull, []string{"pull"}, false, false, false)),
		catalogEntry("opencode", "OpenCode", nil, "Install OpenCode and enable its ACP runtime", ReadinessReady,
			transport("opencode-acp", TransportACP, []string{"push", "stream", "cancel"}, false, true, true), transport("opencode-pull", TransportPull, []string{"pull"}, false, false, false)),
		catalogEntry("goose", "Goose", nil, "Install Goose CLI and configure a local profile", ReadinessPending,
			transport("goose-acp", TransportACP, []string{"push", "stream", "cancel"}, true, false, true), transport("goose-pull", TransportPull, []string{"pull"}, false, false, false)),
		catalogEntry("gemini", "Gemini CLI", []string{"gemini-cli"}, "Install Gemini CLI and configure a safe local profile", ReadinessPending,
			transport("gemini-cli", TransportCLI, []string{"push", "pull"}, true, false, false), transport("gemini-pull", TransportPull, []string{"pull"}, false, false, false)),
		catalogEntry("cursor", "Cursor", []string{"cursor-agent"}, "Install Cursor Agent CLI for explicit Pull use", ReadinessPullOnly,
			transport("cursor-pull", TransportPull, []string{"pull"}, false, false, false)),
		catalogEntry("grok", "Grok", []string{"grok-cli"}, "Install Grok CLI and configure a local profile", ReadinessPending,
			transport("grok-cli", TransportCLI, []string{"push", "pull"}, true, false, false), transport("grok-pull", TransportPull, []string{"pull"}, false, false, false)),
		catalogEntry("pi", "Pi", []string{"pi-cli", "pi-coding-agent"}, "Use the pinned Pi runtime prepared by the Machine", ReadinessReady,
			transport("pi-cli", TransportCLI, []string{"push", "pull"}, true, false, false), transport("pi-pull", TransportPull, []string{"pull"}, false, false, false)),
		catalogEntry("reasonix", "Reasonix", nil, "Install Reasonix CLI and configure a local session", ReadinessPending,
			transport("reasonix-cli", TransportCLI, []string{"push", "pull"}, true, false, false), transport("reasonix-pull", TransportPull, []string{"pull"}, false, false, false)),
	}
	return cloneCatalog(entries)
}

func catalogEntry(id, displayName string, aliases []string, installHint string, readiness Readiness, transports ...CatalogTransport) CatalogEntry {
	return CatalogEntry{ID: id, DisplayName: displayName, Aliases: aliases, InstallHint: installHint, Readiness: readiness, Transports: transports}
}

func transport(id string, mode TransportMode, operations []string, requiresBridge, autoEnabled, nativeSession bool) CatalogTransport {
	return CatalogTransport{ID: id, Mode: mode, Operations: operations, RequiresBridge: requiresBridge, AutoEnabled: autoEnabled, NativeSession: nativeSession}
}

func cloneCatalog(entries []CatalogEntry) []CatalogEntry {
	out := make([]CatalogEntry, len(entries))
	for i, entry := range entries {
		out[i] = entry
		out[i].Aliases = append([]string(nil), entry.Aliases...)
		out[i].Transports = make([]CatalogTransport, len(entry.Transports))
		copy(out[i].Transports, entry.Transports)
		for j := range out[i].Transports {
			out[i].Transports[j].Operations = append([]string(nil), entry.Transports[j].Operations...)
		}
	}
	return out
}

// NormalizeCatalogID resolves a provider family or transport alias to the
// canonical family ID. Unknown values are normalized but not invented.
func NormalizeCatalogID(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	normalized = strings.Join(strings.Fields(strings.ReplaceAll(normalized, " ", "-")), "-")
	for _, entry := range Catalog() {
		if normalized == entry.ID {
			return entry.ID
		}
		for _, alias := range entry.Aliases {
			aliasID := strings.ToLower(strings.TrimSpace(alias))
			aliasID = strings.ReplaceAll(aliasID, "_", "-")
			aliasID = strings.ReplaceAll(aliasID, " ", "-")
			if normalized == aliasID {
				return entry.ID
			}
		}
		for _, candidate := range entry.Transports {
			if normalized == candidate.ID {
				return entry.ID
			}
		}
	}
	return normalized
}
