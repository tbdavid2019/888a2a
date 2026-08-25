package provider

import (
	"bytes"
	"context"
	"os/exec"
	"strings"

	"github.com/coder/acp-go-sdk"

	a2a888pb "github.com/Ranxy/laelia/backend/generated-go/a2a888"
)

// OpenCodeProvider discovers and launches the opencode CLI's ACP server.
type OpenCodeProvider struct{}

func (*OpenCodeProvider) ID() string          { return "opencode" }
func (*OpenCodeProvider) DisplayName() string { return "OpenCode" }

// Manifest returns the validated manifest for OpenCode.
func (p *OpenCodeProvider) Manifest() *a2a888pb.ProviderManifest {
	m := &a2a888pb.ProviderManifest{
		ProviderId:    p.ID(),
		DisplayName:   p.DisplayName(),
		RuntimeKind:   a2a888pb.RuntimeKind_SYSTEM_EXECUTABLE,
		AgentProtocol: a2a888pb.AgentProtocol_ACP_V1,
		PlatformTargets: []*a2a888pb.PlatformTarget{
			{OperatingSystem: "linux", Architecture: "amd64"},
			{OperatingSystem: "linux", Architecture: "arm64"},
			{OperatingSystem: "darwin", Architecture: "amd64"},
			{OperatingSystem: "darwin", Architecture: "arm64"},
			{OperatingSystem: "windows", Architecture: "amd64"},
		},
		RuntimeConfig: &a2a888pb.ProviderManifest_SystemExecutable{
			SystemExecutable: &a2a888pb.SystemExecutableConfig{
				Executable:           "opencode",
				Arguments:            []string{"acp", "--pure", "--cwd"},
				VersionArgument:      "--version",
				PackageVersion:       "1.0.0",
				InheritedEnvironment: []string{"PATH", "HOME"},
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

// ToolCallAdapter returns the OpenCodeAdapter: opencode's create carries only
// partial {cwd} metadata under a generic title; the real command and full
// RawInput arrive in the first in_progress status update.
func (*OpenCodeProvider) ToolCallAdapter() ToolCallAdapter { return OpenCodeAdapter{} }

// BuildCommand mirrors the launch shape used by the executor integration test
// (acp_executor_test.go: opencode acp --pure --cwd <workspace>).
func (*OpenCodeProvider) BuildCommand(workspaceDir string) (string, []string) {
	return "opencode", []string{"acp", "--pure", "--cwd", workspaceDir}
}

func (p *OpenCodeProvider) Detect(ctx context.Context) (*Detected, bool, error) {
	path, err := exec.LookPath("opencode")
	if err != nil {
		//nolint:nilerr // binary not on PATH -> provider absent, not a probe error
		return nil, false, nil
	}
	version := runVersionCmd(ctx, "opencode", "--version")
	return &Detected{
		ProviderID:     p.ID(),
		DisplayName:    p.DisplayName(),
		Version:        version,
		ExecutablePath: path,
	}, true, nil
}

func (p *OpenCodeProvider) ProbeModels(ctx context.Context, workspaceDir string) ([]ModelOption, bool, error) {
	exe, args := p.BuildCommand(workspaceDir)
	sel, err := probeModelConfigOption(ctx, exe, args, workspaceDir)
	if err != nil {
		return nil, false, err
	}
	if sel == nil {
		return nil, false, nil
	}
	return selectOptionsToModels(sel.Options), true, nil
}

// runVersionCmd runs `<bin> <versionFlag...>` and returns the trimmed stdout
// (combined with stderr). It never returns an error: a missing version is not
// fatal for detection, which has already succeeded via LookPath.
func runVersionCmd(ctx context.Context, bin string, args ...string) string {
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.Output()
	if err != nil {
		// Some CLIs write --version to stderr; fall back to combined output.
		var combined bytes.Buffer
		cmd2 := exec.CommandContext(ctx, bin, args...)
		cmd2.Stdout = &combined
		cmd2.Stderr = &combined
		_ = cmd2.Run()
		return strings.TrimSpace(combined.String())
	}
	return strings.TrimSpace(string(out))
}

func selectOptionsToModels(opts acp.SessionConfigSelectOptions) []ModelOption {
	raw := selectOptions(opts)
	models := make([]ModelOption, 0, len(raw))
	for _, o := range raw {
		m := ModelOption{Value: string(o.Value), Name: o.Name}
		if o.Description != nil {
			m.Description = *o.Description
		}
		models = append(models, m)
	}
	return models
}
