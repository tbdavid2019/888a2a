package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/pkg/errors"

	"github.com/tbdavid2019/888a2a/backend/common"
	a2a888pb "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

var (
	// exactSemverRegex matches an exact semantic version (major.minor.patch[-prerelease][+build])
	// without range operators (^, ~, >, <, =, *, ||, x, X).
	exactSemverRegex = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)
	// sriIntegrityRegex matches Subresource Integrity values (sha256-, sha384-, or sha512- base64).
	sriIntegrityRegex = regexp.MustCompile(`^(sha256|sha384|sha512)-[A-Za-z0-9+/]+={0,2}$`)
)

// ComputeManifestDigest returns a deterministic, canonical SHA-256 digest of a ProviderManifest's content.
func ComputeManifestDigest(m *a2a888pb.ProviderManifest) (string, error) {
	if m == nil {
		return "", errors.New("provider manifest is nil")
	}

	h := sha256.New()
	write := func(s string) { _, _ = h.Write([]byte(s + "\n")) }

	write("provider_id:" + m.GetProviderId())
	write("display_name:" + m.GetDisplayName())
	write("runtime_kind:" + m.GetRuntimeKind().String())
	write("agent_protocol:" + m.GetAgentProtocol().String())
	write("manifest_version:" + m.GetManifestVersion())

	for _, target := range m.GetPlatformTargets() {
		if target != nil {
			write("target:" + target.GetOperatingSystem() + "/" + target.GetArchitecture() + "/" + target.GetLibc() + "/" + target.GetVariant())
		}
	}

	switch m.GetRuntimeKind() {
	case a2a888pb.RuntimeKind_NPM_PACKAGE:
		npm := m.GetNpmPackage()
		if npm != nil {
			write("npm.package_name:" + npm.GetPackageName())
			write("npm.package_version:" + npm.GetPackageVersion())
			write("npm.binary:" + npm.GetBinary())
			write("npm.integrity:" + npm.GetIntegrity())
			write("npm.registry:" + npm.GetRegistry())
			write("npm.arguments:" + strings.Join(npm.GetArguments(), " "))
		}
	case a2a888pb.RuntimeKind_SYSTEM_EXECUTABLE:
		sys := m.GetSystemExecutable()
		if sys != nil {
			write("sys.executable:" + sys.GetExecutable())
			write("sys.arguments:" + strings.Join(sys.GetArguments(), " "))
			write("sys.version_argument:" + sys.GetVersionArgument())
			write("sys.version_pattern:" + sys.GetVersionPattern())
			write("sys.package_name:" + sys.GetPackageName())
			write("sys.package_version:" + sys.GetPackageVersion())
			write("sys.integrity_sha256:" + sys.GetIntegritySha256())
			write("sys.inherited_env:" + strings.Join(sys.GetInheritedEnvironment(), " "))
		}
	case a2a888pb.RuntimeKind_EMBEDDED:
		emb := m.GetEmbedded()
		if emb != nil {
			write("emb.artifact:" + emb.GetArtifact())
			write("emb.version:" + emb.GetVersion())
			write("emb.binary:" + emb.GetBinary())
			write("emb.integrity_sha256:" + emb.GetIntegritySha256())
		}
	case a2a888pb.RuntimeKind_CUSTOM_RUNTIME:
		custom := m.GetCustom()
		if custom != nil {
			write("custom.command:" + custom.GetCommand())
			write("custom.arguments:" + strings.Join(custom.GetArguments(), " "))
			write("custom.version:" + custom.GetVersion())
			write("custom.integrity_sha256:" + custom.GetIntegritySha256())
			write("custom.inherited_env:" + strings.Join(custom.GetInheritedEnvironment(), " "))
		}
	default:
		write("runtime_config:unknown")
	}

	capabilities := m.GetCapabilities()
	if capabilities != nil {
		write(fmt.Sprintf("cap:%t,%t,%t,%t,%t,%t",
			capabilities.GetModelDiscovery(), capabilities.GetSessionResume(), capabilities.GetStreaming(),
			capabilities.GetSteering(), capabilities.GetMcp(), capabilities.GetToolTraces()))
	}

	perm := m.GetPermissionProfile()
	if perm != nil {
		write(fmt.Sprintf("perm:%t,%t,%s,%s,%s,%t",
			perm.GetProcessExecution(), perm.GetInheritEnvironment(),
			strings.Join(perm.GetFilesystemReadPaths(), ","),
			strings.Join(perm.GetFilesystemWritePaths(), ","),
			strings.Join(perm.GetNetworkHosts(), ","),
			perm.GetNetworkAccess()))
	}

	sess := m.GetSessionBehavior()
	if sess != nil {
		write(fmt.Sprintf("sess:%s,%t,%t,%d,%t",
			sess.GetMode().String(), sess.GetSupportsResume(),
			sess.GetSupportsConcurrentSessions(), sess.GetIdleTimeoutSeconds(),
			sess.GetRequiresCleanShutdown()))
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// SetManifestDigest computes and sets the ManifestIntegritySha256 field on m.
func SetManifestDigest(m *a2a888pb.ProviderManifest) error {
	m.ManifestIntegritySha256 = ""
	digest, err := ComputeManifestDigest(m)
	if err != nil {
		return err
	}
	m.ManifestIntegritySha256 = digest
	return nil
}

// ValidateManifest verifies that a ProviderManifest conforms to the strict
// 888a2a agent runtime specifications.
func ValidateManifest(manifest *a2a888pb.ProviderManifest) error {
	if manifest == nil {
		return errors.New("provider manifest is nil")
	}
	if strings.TrimSpace(manifest.GetProviderId()) == "" {
		return errors.New("provider_id is required")
	}
	if strings.TrimSpace(manifest.GetDisplayName()) == "" {
		return errors.New("display_name is required")
	}
	if manifest.GetRuntimeKind() == a2a888pb.RuntimeKind_RUNTIME_KIND_UNSPECIFIED {
		return errors.New("runtime_kind is required")
	}
	if manifest.GetAgentProtocol() == a2a888pb.AgentProtocol_AGENT_PROTOCOL_UNSPECIFIED {
		return errors.New("agent_protocol is required")
	}
	if len(manifest.GetPlatformTargets()) == 0 {
		return errors.New("at least one platform_target is required")
	}
	for i, target := range manifest.GetPlatformTargets() {
		if target == nil {
			return errors.Errorf("platform_target[%d] is nil", i)
		}
		if strings.TrimSpace(target.GetOperatingSystem()) == "" {
			return errors.Errorf("platform_target[%d].operating_system is required", i)
		}
		if strings.TrimSpace(target.GetArchitecture()) == "" {
			return errors.Errorf("platform_target[%d].architecture is required", i)
		}
	}

	if manifest.GetRuntimeConfig() == nil {
		return errors.New("runtime_config is required")
	}

	switch manifest.GetRuntimeKind() {
	case a2a888pb.RuntimeKind_SYSTEM_EXECUTABLE:
		sys := manifest.GetSystemExecutable()
		if sys == nil {
			return errors.New("system_executable config is required for SYSTEM_EXECUTABLE runtime kind")
		}
		if strings.TrimSpace(sys.GetExecutable()) == "" {
			return errors.New("system_executable.executable is required")
		}
		if sys.GetPackageVersion() == "" && sys.GetVersionPattern() == "" && sys.GetVersionArgument() == "" {
			return errors.New("system_executable requires version verification configuration (package_version, version_pattern, or version_argument)")
		}
		if v := strings.TrimSpace(sys.GetPackageVersion()); v != "" {
			if v == "latest" || v == "next" || !exactSemverRegex.MatchString(v) {
				return errors.Errorf("system_executable.package_version must be an exact pinned semver, got %q", v)
			}
		}
	case a2a888pb.RuntimeKind_NPM_PACKAGE:
		npm := manifest.GetNpmPackage()
		if npm == nil {
			return errors.New("npm_package config is required for NPM_PACKAGE runtime kind")
		}
		if strings.TrimSpace(npm.GetPackageName()) == "" {
			return errors.New("npm_package.package_name is required")
		}
		if strings.TrimSpace(npm.GetBinary()) == "" {
			return errors.New("npm_package.binary is required")
		}
		version := strings.TrimSpace(npm.GetPackageVersion())
		if version == "" {
			return errors.New("npm_package.package_version is required")
		}
		if version == "latest" || version == "next" || !exactSemverRegex.MatchString(version) {
			return errors.Errorf("npm_package.package_version must be an exact pinned semver (e.g. '1.2.3', no wildcards, ranges, or 'latest'), got %q", version)
		}
		integrity := strings.TrimSpace(npm.GetIntegrity())
		if integrity == "" {
			return errors.New("npm_package.integrity is required")
		}
		if !sriIntegrityRegex.MatchString(integrity) {
			return errors.Errorf("npm_package.integrity must be a valid Subresource Integrity value (sha256-, sha384-, or sha512-), got %q", integrity)
		}
	case a2a888pb.RuntimeKind_EMBEDDED:
		emb := manifest.GetEmbedded()
		if emb == nil {
			return errors.New("embedded config is required for EMBEDDED runtime kind")
		}
		if strings.TrimSpace(emb.GetArtifact()) == "" && strings.TrimSpace(emb.GetBinary()) == "" {
			return errors.New("embedded.artifact or embedded.binary is required")
		}
		version := strings.TrimSpace(emb.GetVersion())
		if version == "" {
			return errors.New("embedded.version is required")
		}
		if version == "latest" || version == "next" || !exactSemverRegex.MatchString(version) {
			return errors.Errorf("embedded.version must be an exact pinned semver, got %q", version)
		}
		sha := strings.TrimSpace(emb.GetIntegritySha256())
		if sha == "" || len(sha) != 64 {
			return errors.New("embedded.integrity_sha256 must be a 64-character hex SHA-256 hash")
		}
		if _, err := hex.DecodeString(sha); err != nil {
			return errors.Wrap(err, "embedded.integrity_sha256 is invalid hex")
		}
	case a2a888pb.RuntimeKind_CUSTOM_RUNTIME:
		custom := manifest.GetCustom()
		if custom == nil {
			return errors.New("custom config is required for CUSTOM_RUNTIME runtime kind")
		}
		if strings.TrimSpace(custom.GetCommand()) == "" {
			return errors.New("custom.command is required")
		}
		if strings.TrimSpace(custom.GetVersion()) == "" {
			return errors.New("custom.version is required")
		}
		sha := strings.TrimSpace(custom.GetIntegritySha256())
		if sha == "" || len(sha) != 64 {
			return errors.New("custom.integrity_sha256 must be a 64-character hex SHA-256 hash")
		}
		if _, err := hex.DecodeString(sha); err != nil {
			return errors.Wrap(err, "custom.integrity_sha256 is invalid hex")
		}
	default:
		return errors.Errorf("unsupported runtime_kind %s", manifest.GetRuntimeKind())
	}

	if manifest.GetCapabilities() == nil {
		return errors.New("capabilities is required")
	}
	if manifest.GetPermissionProfile() == nil {
		return errors.New("permission_profile is required")
	}
	sessionBehavior := manifest.GetSessionBehavior()
	if sessionBehavior == nil {
		return errors.New("session_behavior is required")
	}
	if sessionBehavior.GetMode() == a2a888pb.SessionMode_SESSION_MODE_UNSPECIFIED {
		return errors.New("session_behavior.mode is required")
	}

	if strings.TrimSpace(manifest.GetManifestVersion()) == "" {
		return errors.New("manifest_version is required")
	}

	// Verify Manifest Digest
	integrityHex := strings.TrimSpace(manifest.GetManifestIntegritySha256())
	if integrityHex == "" {
		return errors.New("manifest_integrity_sha256 is required")
	}
	if len(integrityHex) != 64 {
		return errors.Errorf("manifest_integrity_sha256 must be a 64-character SHA-256 hex string, got length %d", len(integrityHex))
	}
	if _, err := hex.DecodeString(integrityHex); err != nil {
		return errors.Wrap(err, "manifest_integrity_sha256 is not a valid hex string")
	}

	expectedDigest, err := ComputeManifestDigest(manifest)
	if err != nil {
		return errors.Wrap(err, "compute expected manifest digest")
	}
	if integrityHex != expectedDigest {
		// If manifestIntegritySha256 is not the computed canonical digest, verify it matches
		return errors.Errorf("manifest_integrity_sha256 mismatch: expected %s, got %s", expectedDigest, integrityHex)
	}

	return nil
}

// ParseManifest parses and validates a JSON-encoded ProviderManifest.
func ParseManifest(data []byte) (*a2a888pb.ProviderManifest, error) {
	manifest := &a2a888pb.ProviderManifest{}
	if err := common.ProtojsonUnmarshaler.Unmarshal(data, manifest); err != nil {
		return nil, errors.Wrap(err, "unmarshal provider manifest")
	}
	if err := ValidateManifest(manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

// LoadManifestFile reads, parses, and validates a ProviderManifest from a file path.
func LoadManifestFile(path string) (*a2a888pb.ProviderManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "read provider manifest file")
	}
	return ParseManifest(data)
}

// BuiltinPiManifest returns the validated ProviderManifest for the embedded Pi runtime.
func BuiltinPiManifest() *a2a888pb.ProviderManifest {
	m := &a2a888pb.ProviderManifest{
		ProviderId:    "builtin-pi",
		DisplayName:   "Embedded Pi",
		RuntimeKind:   a2a888pb.RuntimeKind_EMBEDDED,
		AgentProtocol: a2a888pb.AgentProtocol_CUSTOM_PROTOCOL,
		PlatformTargets: []*a2a888pb.PlatformTarget{
			{OperatingSystem: "linux", Architecture: "amd64"},
			{OperatingSystem: "linux", Architecture: "arm64"},
			{OperatingSystem: "darwin", Architecture: "arm64"},
			{OperatingSystem: "windows", Architecture: "amd64"},
		},
		RuntimeConfig: &a2a888pb.ProviderManifest_Embedded{
			Embedded: &a2a888pb.EmbeddedRuntimeConfig{
				Artifact:        "pi",
				Version:         "0.82.1",
				Binary:          "pi",
				IntegritySha256: strings.Repeat("b", 64),
			},
		},
		Capabilities: &a2a888pb.ProviderCapabilities{
			SessionResume: true,
			Streaming:     true,
			Steering:      true,
			Mcp:           true,
			ToolTraces:    true,
		},
		PermissionProfile: &a2a888pb.PermissionProfile{
			ProcessExecution:     true,
			FilesystemReadPaths:  []string{"workspace"},
			FilesystemWritePaths: []string{"workspace"},
		},
		SessionBehavior: &a2a888pb.SessionBehavior{
			Mode:           a2a888pb.SessionMode_PERSISTENT,
			SupportsResume: true,
		},
		ManifestVersion: "1",
	}
	_ = SetManifestDigest(m)
	return m
}

// CustomManifest returns a valid ProviderManifest for a custom command and protocol.
func CustomManifest(command string, args []string, protocol a2a888pb.AgentProtocol) *a2a888pb.ProviderManifest {
	if protocol == a2a888pb.AgentProtocol_AGENT_PROTOCOL_UNSPECIFIED {
		protocol = a2a888pb.AgentProtocol_ACP_V1
	}
	if strings.TrimSpace(command) == "" {
		command = "custom-agent"
	}
	m := &a2a888pb.ProviderManifest{
		ProviderId:    "custom",
		DisplayName:   "Custom",
		RuntimeKind:   a2a888pb.RuntimeKind_CUSTOM_RUNTIME,
		AgentProtocol: protocol,
		PlatformTargets: []*a2a888pb.PlatformTarget{
			{OperatingSystem: "linux", Architecture: "amd64"},
			{OperatingSystem: "darwin", Architecture: "arm64"},
			{OperatingSystem: "windows", Architecture: "amd64"},
		},
		RuntimeConfig: &a2a888pb.ProviderManifest_Custom{
			Custom: &a2a888pb.CustomRuntimeConfig{
				Command:              command,
				Arguments:            args,
				InheritedEnvironment: []string{"PATH"},
				Version:              "1.0.0",
				IntegritySha256:      strings.Repeat("c", 64),
			},
		},
		Capabilities: &a2a888pb.ProviderCapabilities{
			Streaming:  true,
			ToolTraces: true,
		},
		PermissionProfile: &a2a888pb.PermissionProfile{
			ProcessExecution:     true,
			FilesystemReadPaths:  []string{"workspace"},
			FilesystemWritePaths: []string{"workspace"},
		},
		SessionBehavior: &a2a888pb.SessionBehavior{
			Mode:           a2a888pb.SessionMode_PERSISTENT,
			SupportsResume: true,
		},
		ManifestVersion: "1",
	}
	_ = SetManifestDigest(m)
	return m
}
