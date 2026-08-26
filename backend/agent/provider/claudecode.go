package provider

import (
	"context"
	"os"
	"os/exec"

	a2a888pb "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

// ClaudeCodeProvider discovers and launches the @agentclientprotocol/claude-agent-acp
// npm package via npx, the ACP server wrapper around Claude Code.
type ClaudeCodeProvider struct{}

func (*ClaudeCodeProvider) ID() string          { return "claude-code" }
func (*ClaudeCodeProvider) DisplayName() string { return "Claude Code" }

// Manifest returns the validated manifest for Claude Code.
func (p *ClaudeCodeProvider) Manifest() *a2a888pb.ProviderManifest {
	m := &a2a888pb.ProviderManifest{
		ProviderId:    p.ID(),
		DisplayName:   p.DisplayName(),
		RuntimeKind:   a2a888pb.RuntimeKind_NPM_PACKAGE,
		AgentProtocol: a2a888pb.AgentProtocol_ACP_V1,
		PlatformTargets: []*a2a888pb.PlatformTarget{
			{OperatingSystem: "linux", Architecture: "amd64"},
			{OperatingSystem: "linux", Architecture: "arm64"},
			{OperatingSystem: "darwin", Architecture: "amd64"},
			{OperatingSystem: "darwin", Architecture: "arm64"},
			{OperatingSystem: "windows", Architecture: "amd64"},
		},
		RuntimeConfig: &a2a888pb.ProviderManifest_NpmPackage{
			NpmPackage: &a2a888pb.NpmPackageConfig{
				PackageName:    "@agentclientprotocol/claude-agent-acp",
				PackageVersion: "0.70.0",
				Binary:         "claude-agent-acp",
				Integrity:      "sha512-Psqj6fhV4pQ8IM480zpJ+xGiMMIqNLxlsTj5Mzn+T8KSURCVNJdl0ktcqLMjgHJC/QnOvDdDkFf3xTW9VIV9aQ==",
				Registry:       "https://registry.npmjs.org",
			},
		},
		Capabilities: &a2a888pb.ProviderCapabilities{
			ModelDiscovery: true,
			SessionResume:  true,
			Streaming:      true,
			Steering:       true,
			Mcp:            true,
			ToolTraces:     true,
		},
		PermissionProfile: &a2a888pb.PermissionProfile{
			ProcessExecution:     true,
			InheritEnvironment:   true,
			FilesystemReadPaths:  []string{"workspace"},
			FilesystemWritePaths: []string{"workspace"},
		},
		SessionBehavior: &a2a888pb.SessionBehavior{
			Mode:                  a2a888pb.SessionMode_PERSISTENT,
			SupportsResume:        true,
			RequiresCleanShutdown: true,
		},
		ManifestVersion: "1",
	}
	_ = SetManifestDigest(m)
	return m
}

// ToolCallAdapter returns the DefaultAdapter: claude-code sends an empty-input
// create, then a content-only update carrying the command, then a completed
// status update with the output.
func (*ClaudeCodeProvider) ToolCallAdapter() ToolCallAdapter { return DefaultAdapter{} }

// BuildCommand launches the ACP wrapper from a prepared local binary.
// It never falls back to floating turn-time network downloads.
func (*ClaudeCodeProvider) BuildCommand(_ string) (string, []string) {
	if custom := os.Getenv("CLAUDE_AGENT_ACP_PATH"); custom != "" {
		return custom, nil
	}
	if path, err := exec.LookPath("claude-agent-acp"); err == nil {
		return path, nil
	}
	return "", nil
}

func (p *ClaudeCodeProvider) Detect(ctx context.Context) (*Detected, bool, error) {
	// Claude Code requires the real `claude` CLI and the prepared `claude-agent-acp` binary.
	// We never report presence based solely on npx or allow floating network downloads during turns.
	if _, err := exec.LookPath("claude"); err != nil {
		//nolint:nilerr // claude CLI not on PATH -> provider absent, not a probe error
		return nil, false, nil
	}

	exe, _ := p.BuildCommand("")
	if exe == "" {
		// The host can still be prepared after discovery. Keep the provider
		// visible so Machine discovery can prepare its pinned runtime.
		return &Detected{ProviderID: p.ID(), DisplayName: p.DisplayName(), Version: runVersionCmd(ctx, "claude", "--version")}, true, nil
	}

	version := runVersionCmd(ctx, "claude", "--version")
	return &Detected{
		ProviderID:     p.ID(),
		DisplayName:    p.DisplayName(),
		Version:        version,
		ExecutablePath: exe,
	}, true, nil
}

func (p *ClaudeCodeProvider) ProbeModels(ctx context.Context, workspaceDir string) ([]ModelOption, bool, error) {
	exe, args := p.BuildCommand(workspaceDir)
	if exe == "" {
		return nil, false, nil
	}
	sel, err := probeModelConfigOption(ctx, exe, args, workspaceDir)
	if err != nil {
		return nil, false, err
	}
	if sel == nil {
		return nil, false, nil
	}
	return selectOptionsToModels(sel.Options), true, nil
}
