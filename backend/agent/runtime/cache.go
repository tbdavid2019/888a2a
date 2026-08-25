package agentruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pkg/errors"

	"github.com/Ranxy/laelia/backend/agent/provider"
	a2a888pb "github.com/Ranxy/laelia/backend/generated-go/a2a888"
)

// CurrentPlatform returns the PlatformTarget for the running machine.
func CurrentPlatform() *a2a888pb.PlatformTarget {
	return &a2a888pb.PlatformTarget{
		OperatingSystem: runtime.GOOS,
		Architecture:    runtime.GOARCH,
	}
}

// ComputeCacheIdentity generates a deterministic, content-addressed CacheIdentity
// from a validated ProviderManifest and target platform.
func ComputeCacheIdentity(manifest *a2a888pb.ProviderManifest, platform *a2a888pb.PlatformTarget) (*a2a888pb.CacheIdentity, error) {
	if err := provider.ValidateManifest(manifest); err != nil {
		return nil, errors.Wrap(err, "validate manifest for cache identity")
	}
	if platform == nil {
		platform = CurrentPlatform()
	}

	h := sha256.New()
	h.Write([]byte("provider_id:" + manifest.GetProviderId() + "\n"))
	h.Write([]byte("runtime_kind:" + manifest.GetRuntimeKind().String() + "\n"))
	h.Write([]byte("agent_protocol:" + manifest.GetAgentProtocol().String() + "\n"))
	h.Write([]byte("os:" + platform.GetOperatingSystem() + "\n"))
	h.Write([]byte("arch:" + platform.GetArchitecture() + "\n"))
	h.Write([]byte("manifest_version:" + manifest.GetManifestVersion() + "\n"))
	h.Write([]byte("manifest_sha256:" + manifest.GetManifestIntegritySha256() + "\n"))

	identity := &a2a888pb.CacheIdentity{
		ProviderId:     manifest.GetProviderId(),
		ManifestDigest: manifest.GetManifestIntegritySha256(),
		Platform:       platform,
	}

	switch manifest.GetRuntimeKind() {
	case a2a888pb.RuntimeKind_NPM_PACKAGE:
		npm := manifest.GetNpmPackage()
		identity.PackageName = npm.GetPackageName()
		identity.PackageVersion = npm.GetPackageVersion()
		identity.RuntimeVersion = npm.GetPackageVersion()
		identity.Integrity = npm.GetIntegrity()

		h.Write([]byte("npm_package:" + npm.GetPackageName() + "\n"))
		h.Write([]byte("npm_version:" + npm.GetPackageVersion() + "\n"))
		h.Write([]byte("npm_binary:" + npm.GetBinary() + "\n"))
		h.Write([]byte("npm_integrity:" + npm.GetIntegrity() + "\n"))
	case a2a888pb.RuntimeKind_SYSTEM_EXECUTABLE:
		sys := manifest.GetSystemExecutable()
		identity.RuntimeVersion = sys.GetPackageVersion()
		if identity.RuntimeVersion == "" {
			identity.RuntimeVersion = "system"
		}
		h.Write([]byte("sys_executable:" + sys.GetExecutable() + "\n"))
		h.Write([]byte("sys_version:" + identity.RuntimeVersion + "\n"))
	case a2a888pb.RuntimeKind_EMBEDDED:
		emb := manifest.GetEmbedded()
		identity.RuntimeVersion = emb.GetVersion()
		h.Write([]byte("emb_artifact:" + emb.GetArtifact() + "\n"))
		h.Write([]byte("emb_version:" + emb.GetVersion() + "\n"))
	case a2a888pb.RuntimeKind_CUSTOM_RUNTIME:
		custom := manifest.GetCustom()
		identity.RuntimeVersion = custom.GetVersion()
		h.Write([]byte("custom_command:" + custom.GetCommand() + "\n"))
		h.Write([]byte("custom_args:" + strings.Join(custom.GetArguments(), " ") + "\n"))
	default:
		return nil, fmt.Errorf("unsupported runtime kind %s", manifest.GetRuntimeKind())
	}

	identity.IdentityDigest = hex.EncodeToString(h.Sum(nil))
	return identity, nil
}

// CachePaths provides standardized paths within the machine runtime root directory.
type CachePaths struct {
	Root string
}

// NewCachePaths initializes cache paths rooted at rootDir.
func NewCachePaths(rootDir string) *CachePaths {
	return &CachePaths{Root: rootDir}
}

// PackagesDir returns the directory where immutable verified packages are stored.
func (c *CachePaths) PackagesDir() string {
	return filepath.Join(c.Root, "packages")
}

// PackageDir returns the immutable directory for a specific cache identity.
func (c *CachePaths) PackageDir(identity *a2a888pb.CacheIdentity) string {
	return filepath.Join(c.PackagesDir(), identity.GetIdentityDigest())
}

// StagingDir returns the temporary staging directory for atomic installation.
func (c *CachePaths) StagingDir(identity *a2a888pb.CacheIdentity, sessionID string) string {
	return filepath.Join(c.Root, "staging", fmt.Sprintf("%s.%s", identity.GetIdentityDigest(), sessionID))
}

// QuarantineDir returns the quarantine destination directory for corrupted packages.
func (c *CachePaths) QuarantineDir(identity *a2a888pb.CacheIdentity, timestamp int64) string {
	return filepath.Join(c.Root, "quarantine", fmt.Sprintf("%s.%d", identity.GetIdentityDigest(), timestamp))
}
