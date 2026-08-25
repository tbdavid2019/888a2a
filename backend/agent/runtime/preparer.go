package agentruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Ranxy/laelia/backend/agent/provider"
	a2a888pb "github.com/Ranxy/laelia/backend/generated-go/a2a888"
)

// CommandRunner abstracts shell/process execution for npm installation and verification.
type CommandRunner interface {
	Run(ctx context.Context, cmd string, args []string, dir string, env []string) ([]byte, error)
}

type execRunner struct{}

func (_ *execRunner) Run(ctx context.Context, name string, args []string, dir string, env []string) ([]byte, error) {
	c := exec.CommandContext(ctx, name, args...)
	c.Dir = dir
	if len(env) > 0 {
		c.Env = append(os.Environ(), env...)
	}
	return c.CombinedOutput()
}

// runtimeMeta records the immutable identity and integrity evidence of a prepared package.
type runtimeMeta struct {
	IdentityDigest   string `json:"identity_digest"`
	ManifestDigest   string `json:"manifest_digest"`
	PackageName      string `json:"package_name"`
	PackageVersion   string `json:"package_version"`
	PackageIntegrity string `json:"package_integrity"`
	BinaryName       string `json:"binary_name"`
	BinaryRelPath    string `json:"binary_rel_path"`
	BinarySha256     string `json:"binary_sha256"`
	BinarySizeBytes  int64  `json:"binary_size_bytes"`
	PreparedAtUnix   int64  `json:"prepared_at_unix"`
}

type npmLockPackage struct {
	Version   string `json:"version"`
	Integrity string `json:"integrity"`
}

type npmLockFile struct {
	Packages map[string]npmLockPackage `json:"packages"`
}

// Preparer manages the atomic preparation, verification, quarantine, and local-bin
// resolution of 888a2a agent runtimes.
type Preparer struct {
	paths    *CachePaths
	runner   CommandRunner
	mu       sync.Mutex
	auditMu  sync.Mutex
	auditLog []AuditEvent
}

// NewPreparer initializes a runtime Preparer.
func NewPreparer(rootDir string, runner CommandRunner) *Preparer {
	if runner == nil {
		runner = &execRunner{}
	}
	return &Preparer{
		paths:    NewCachePaths(rootDir),
		runner:   runner,
		auditLog: make([]AuditEvent, 0),
	}
}

// Paths returns the CachePaths used by the preparer.
func (p *Preparer) Paths() *CachePaths {
	return p.paths
}

func (p *Preparer) recordAudit(action, providerID, digest string, success bool, reason, details string) {
	p.auditMu.Lock()
	defer p.auditMu.Unlock()
	p.auditLog = append(p.auditLog, NewAuditEvent(action, providerID, digest, success, reason, details))
}

// AuditLog returns a copy of the audit events recorded so far.
func (p *Preparer) AuditLog() []AuditEvent {
	p.auditMu.Lock()
	defer p.auditMu.Unlock()
	out := make([]AuditEvent, len(p.auditLog))
	copy(out, p.auditLog)
	return out
}

// Prepare ensures that the requested provider runtime is installed, verified, and ready
// for execution without floating turn-time downloads.
func (p *Preparer) Prepare(ctx context.Context, manifest *a2a888pb.ProviderManifest, platform *a2a888pb.PlatformTarget) (*a2a888pb.PreparedRuntime, error) {
	if err := provider.ValidateManifest(manifest); err != nil {
		p.recordAudit("VALIDATE", manifest.GetProviderId(), "", false, "invalid manifest", err.Error())
		return nil, errors.Wrap(err, "validate manifest")
	}

	identity, err := ComputeCacheIdentity(manifest, platform)
	if err != nil {
		return nil, errors.Wrap(err, "compute cache identity")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	now := timestamppb.Now()

	// Handle SYSTEM_EXECUTABLE
	if manifest.GetRuntimeKind() == a2a888pb.RuntimeKind_SYSTEM_EXECUTABLE {
		sys := manifest.GetSystemExecutable()
		exePath, err := exec.LookPath(sys.GetExecutable())
		if err != nil {
			p.recordAudit("RESOLVE_SYSTEM", manifest.GetProviderId(), identity.GetIdentityDigest(), false, "executable not found on PATH", err.Error())
			return &a2a888pb.PreparedRuntime{
				ProviderId:    manifest.GetProviderId(),
				CacheIdentity: identity,
				Status: &a2a888pb.RuntimeStatus{
					State:       a2a888pb.RuntimeState_UNAVAILABLE,
					Message:     fmt.Sprintf("executable %q not found on host PATH", sys.GetExecutable()),
					ObservedAt:  now,
					FailureCode: "EXECUTABLE_NOT_FOUND",
				},
				PreparedAt: now,
			}, nil
		}
		if err := verifySystemExecutable(ctx, exePath, sys); err != nil {
			p.recordAudit("VERIFY_SYSTEM", manifest.GetProviderId(), identity.GetIdentityDigest(), false, "system executable verification failed", err.Error())
			return &a2a888pb.PreparedRuntime{
				ProviderId:    manifest.GetProviderId(),
				CacheIdentity: identity,
				Status: &a2a888pb.RuntimeStatus{
					State:       a2a888pb.RuntimeState_BROKEN,
					Message:     err.Error(),
					ObservedAt:  now,
					FailureCode: "SYSTEM_VERSION_MISMATCH",
				},
				PreparedAt: now,
			}, errors.Wrap(err, "verify system executable")
		}

		sha, size, _ := computeFileSha256(exePath)
		resolved := &a2a888pb.ResolvedBinary{
			Path:      exePath,
			Binary:    filepath.Base(exePath),
			Sha256:    sha,
			SizeBytes: uint64(size),
			Version:   identity.GetRuntimeVersion(),
			Source:    "system",
			Arguments: sys.GetArguments(),
		}

		return &a2a888pb.PreparedRuntime{
			ProviderId:     manifest.GetProviderId(),
			CacheIdentity:  identity,
			ResolvedBinary: resolved,
			Status: &a2a888pb.RuntimeStatus{
				State:           a2a888pb.RuntimeState_READY,
				ObservedVersion: identity.GetRuntimeVersion(),
				ObservedAt:      now,
			},
			Compatibility: &a2a888pb.CompatibilityReport{
				Level: a2a888pb.CompatibilityLevel_FUNCTIONALLY_VERIFIED,
				Evidence: []*a2a888pb.CompatibilityEvidence{{
					Version:  identity.GetRuntimeVersion(),
					Platform: identity.GetPlatform(),
					TestedAt: now,
					Details:  "resolved system executable on host PATH",
				}},
			},
			PreparedAt: now,
		}, nil
	}

	// Handle NPM_PACKAGE
	if manifest.GetRuntimeKind() == a2a888pb.RuntimeKind_NPM_PACKAGE {
		return p.prepareNpmPackage(ctx, manifest, identity, now)
	}

	var executable string
	var args []string
	source := "embedded"
	if manifest.GetRuntimeKind() == a2a888pb.RuntimeKind_EMBEDDED {
		emb := manifest.GetEmbedded()
		executable = emb.GetBinary()
	} else {
		custom := manifest.GetCustom()
		executable = custom.GetCommand()
		args = custom.GetArguments()
		source = "custom"
	}
	exePath, err := exec.LookPath(executable)
	if err != nil {
		return &a2a888pb.PreparedRuntime{
			ProviderId:    manifest.GetProviderId(),
			CacheIdentity: identity,
			Status: &a2a888pb.RuntimeStatus{
				State:       a2a888pb.RuntimeState_UNAVAILABLE,
				Message:     fmt.Sprintf("executable %q not found on host PATH", executable),
				ObservedAt:  now,
				FailureCode: "EXECUTABLE_NOT_FOUND",
			},
			PreparedAt: now,
		}, nil
	}
	sha, size, err := computeFileSha256(exePath)
	if err != nil {
		return nil, errors.Wrap(err, "hash local runtime executable")
	}
	return &a2a888pb.PreparedRuntime{
		ProviderId:     manifest.GetProviderId(),
		CacheIdentity:  identity,
		ResolvedBinary: &a2a888pb.ResolvedBinary{Path: exePath, Binary: filepath.Base(exePath), Version: identity.GetRuntimeVersion(), Sha256: sha, SizeBytes: uint64(size), Source: source, Arguments: args},
		Status:         &a2a888pb.RuntimeStatus{State: a2a888pb.RuntimeState_READY, ObservedVersion: identity.GetRuntimeVersion(), ObservedAt: now},
		Compatibility:  &a2a888pb.CompatibilityReport{Level: a2a888pb.CompatibilityLevel_PROTOCOL_READY, Evidence: []*a2a888pb.CompatibilityEvidence{{Version: identity.GetRuntimeVersion(), Platform: identity.GetPlatform(), TestedAt: now, Details: "resolved and hashed local runtime executable"}}},
		PreparedAt:     now,
	}, nil
}

func verifySystemExecutable(ctx context.Context, executablePath string, config *a2a888pb.SystemExecutableConfig) error {
	if config.GetVersionArgument() == "" {
		return nil
	}
	cmd := exec.CommandContext(ctx, executablePath, config.GetVersionArgument())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrap(err, "version probe failed")
	}
	versionOutput := strings.TrimSpace(string(out))
	if pattern := strings.TrimSpace(config.GetVersionPattern()); pattern != "" {
		matched, matchErr := regexp.MatchString(pattern, versionOutput)
		if matchErr != nil {
			return errors.Wrap(matchErr, "invalid version pattern")
		}
		if !matched {
			return errors.Errorf("version %q does not match %q", versionOutput, pattern)
		}
		return nil
	}
	if expected := strings.TrimSpace(config.GetPackageVersion()); expected != "" && !strings.Contains(versionOutput, expected) {
		return errors.Errorf("version %q does not contain expected version %q", versionOutput, expected)
	}
	return nil
}

func (p *Preparer) prepareNpmPackage(ctx context.Context, manifest *a2a888pb.ProviderManifest, identity *a2a888pb.CacheIdentity, now *timestamppb.Timestamp) (*a2a888pb.PreparedRuntime, error) {
	npm := manifest.GetNpmPackage()
	pkgDir := p.paths.PackageDir(identity)
	metaPath := filepath.Join(pkgDir, ".runtime_meta.json")

	// Check if already prepared and verified in immutable cache
	if metaBytes, err := os.ReadFile(metaPath); err == nil {
		var meta runtimeMeta
		if err := json.Unmarshal(metaBytes, &meta); err == nil {
			// Validate that metadata matches request
			if meta.IdentityDigest == identity.GetIdentityDigest() &&
				meta.ManifestDigest == manifest.GetManifestIntegritySha256() &&
				meta.PackageIntegrity == npm.GetIntegrity() {
				binPath := filepath.Join(pkgDir, meta.BinaryRelPath)
				sha, size, err := computeFileSha256(binPath)
				if err == nil && sha == meta.BinarySha256 {
					// Perfectly verified cached binary!
					return &a2a888pb.PreparedRuntime{
						ProviderId:    manifest.GetProviderId(),
						CacheIdentity: identity,
						ResolvedBinary: &a2a888pb.ResolvedBinary{
							Path:      binPath,
							Binary:    meta.BinaryName,
							Sha256:    sha,
							SizeBytes: uint64(size),
							Source:    "npm_prepared_cache",
						},
						Status: &a2a888pb.RuntimeStatus{
							State:           a2a888pb.RuntimeState_READY,
							ObservedVersion: identity.GetRuntimeVersion(),
							ObservedAt:      now,
						},
						Compatibility: &a2a888pb.CompatibilityReport{
							Level: a2a888pb.CompatibilityLevel_FULL_LOOP_VERIFIED,
							Evidence: []*a2a888pb.CompatibilityEvidence{{
								Version:  identity.GetRuntimeVersion(),
								Platform: identity.GetPlatform(),
								TestedAt: now,
								Details:  "verified immutable cached npm package and binary digest",
							}},
						},
						PreparedAt: now,
					}, nil
				}

				// Binary was tampered or corrupted on disk!
				_ = p.Quarantine(identity, pkgDir, fmt.Sprintf("cached binary integrity mismatch on disk: expected %s, got %s", meta.BinarySha256, sha))
				p.recordAudit("TAMPER_DETECTED", manifest.GetProviderId(), identity.GetIdentityDigest(), false, "cached binary modified or corrupted", pkgDir)
				return &a2a888pb.PreparedRuntime{
					ProviderId:    manifest.GetProviderId(),
					CacheIdentity: identity,
					Status: &a2a888pb.RuntimeStatus{
						State:       a2a888pb.RuntimeState_QUARANTINED,
						Message:     "cached binary integrity mismatch: binary was modified or corrupted on disk",
						ObservedAt:  now,
						FailureCode: "INTEGRITY_TAMPER_DETECTED",
					},
					PreparedAt: now,
				}, errors.New("cached binary integrity mismatch: binary was modified or corrupted on disk")
			}
		}
	}

	// Staging directory preparation
	stagingID := fmt.Sprintf("%d", time.Now().UnixNano())
	stagingDir := p.paths.StagingDir(identity, stagingID)
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return nil, errors.Wrap(err, "create staging dir")
	}

	// Ensure cleanup if interrupted or failed before atomic publish
	var published bool
	defer func() {
		if !published {
			_ = os.RemoveAll(stagingDir)
		}
	}()

	p.recordAudit("INSTALLING", manifest.GetProviderId(), identity.GetIdentityDigest(), true, "starting npm installation into staging", stagingDir)

	// Run npm install in staging directory
	targetSpec := fmt.Sprintf("%s@%s", npm.GetPackageName(), npm.GetPackageVersion())
	args := []string{"install", "--no-audit", "--no-fund", "--ignore-scripts", "--prefix", stagingDir}
	if registry := strings.TrimSpace(npm.GetRegistry()); registry != "" {
		args = append(args, "--registry", registry)
	}
	args = append(args, targetSpec)
	out, err := p.runner.Run(ctx, "npm", args, stagingDir, nil)
	if err != nil {
		p.recordAudit("INSTALL_FAILED", manifest.GetProviderId(), identity.GetIdentityDigest(), false, "npm install error", string(out))
		return &a2a888pb.PreparedRuntime{
			ProviderId:    manifest.GetProviderId(),
			CacheIdentity: identity,
			Status: &a2a888pb.RuntimeStatus{
				State:       a2a888pb.RuntimeState_BROKEN,
				Message:     fmt.Sprintf("npm install failed: %v\nOutput: %s", err, string(out)),
				ObservedAt:  now,
				FailureCode: "NPM_INSTALL_FAILED",
			},
			PreparedAt: now,
		}, errors.Wrap(err, "npm install failed")
	}

	if err := verifyNpmPackageIntegrity(stagingDir, npm.GetPackageName(), npm.GetPackageVersion(), npm.GetIntegrity()); err != nil {
		_ = p.Quarantine(identity, stagingDir, fmt.Sprintf("npm package integrity verification failed: %v", err))
		return &a2a888pb.PreparedRuntime{
			ProviderId:    manifest.GetProviderId(),
			CacheIdentity: identity,
			Status: &a2a888pb.RuntimeStatus{
				State:       a2a888pb.RuntimeState_QUARANTINED,
				Message:     fmt.Sprintf("npm package integrity verification failed: %v", err),
				ObservedAt:  now,
				FailureCode: "PACKAGE_INTEGRITY_MISMATCH",
			},
			PreparedAt: now,
		}, errors.Wrap(err, "verify npm package integrity")
	}

	// Verify local binary in staging
	resolvedBin, relPath, err := resolveNpmBinaryInDir(stagingDir, npm.GetBinary())
	if err != nil {
		// Integrity / resolution failure -> Quarantine!
		_ = p.Quarantine(identity, stagingDir, fmt.Sprintf("binary resolution failed: %v", err))
		return &a2a888pb.PreparedRuntime{
			ProviderId:    manifest.GetProviderId(),
			CacheIdentity: identity,
			Status: &a2a888pb.RuntimeStatus{
				State:       a2a888pb.RuntimeState_QUARANTINED,
				Message:     fmt.Sprintf("integrity quarantine: %v", err),
				ObservedAt:  now,
				FailureCode: "INTEGRITY_VERIFICATION_FAILED",
			},
			PreparedAt: now,
		}, errors.Wrap(err, "verify staging binary")
	}

	// Write immutable metadata into staging dir before atomic publication
	meta := &runtimeMeta{
		IdentityDigest:   identity.GetIdentityDigest(),
		ManifestDigest:   manifest.GetManifestIntegritySha256(),
		PackageName:      npm.GetPackageName(),
		PackageVersion:   npm.GetPackageVersion(),
		PackageIntegrity: npm.GetIntegrity(),
		BinaryName:       npm.GetBinary(),
		BinaryRelPath:    relPath,
		BinarySha256:     resolvedBin.GetSha256(),
		BinarySizeBytes:  int64(resolvedBin.GetSizeBytes()),
		PreparedAtUnix:   now.GetSeconds(),
	}
	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, errors.Wrap(err, "marshal runtime meta")
	}
	if err := os.WriteFile(filepath.Join(stagingDir, ".runtime_meta.json"), metaBytes, 0o644); err != nil {
		return nil, errors.Wrap(err, "write runtime meta")
	}

	// Atomic Publish: create parent dir and atomically rename staging dir to final packageDir
	if err := os.MkdirAll(filepath.Dir(pkgDir), 0o755); err != nil {
		return nil, errors.Wrap(err, "mkdir packages parent")
	}
	if err := os.Rename(stagingDir, pkgDir); err != nil {
		return nil, errors.Wrap(err, "atomic rename staging to package dir")
	}
	published = true

	// Final resolved binary inside published path
	resolvedBin.Path = filepath.Join(pkgDir, relPath)

	p.recordAudit("PUBLISHED", manifest.GetProviderId(), identity.GetIdentityDigest(), true, "published verified npm package to cache", pkgDir)

	return &a2a888pb.PreparedRuntime{
		ProviderId:     manifest.GetProviderId(),
		CacheIdentity:  identity,
		ResolvedBinary: resolvedBin,
		Status: &a2a888pb.RuntimeStatus{
			State:           a2a888pb.RuntimeState_READY,
			ObservedVersion: identity.GetRuntimeVersion(),
			ObservedAt:      now,
		},
		Compatibility: &a2a888pb.CompatibilityReport{
			Level: a2a888pb.CompatibilityLevel_FULL_LOOP_VERIFIED,
			Evidence: []*a2a888pb.CompatibilityEvidence{{
				Version:  identity.GetRuntimeVersion(),
				Platform: identity.GetPlatform(),
				TestedAt: now,
				Details:  "atomically prepared and verified pinned npm package",
			}},
		},
		PreparedAt: now,
	}, nil
}

func verifyNpmPackageIntegrity(stagingDir, packageName, expectedVersion, expectedIntegrity string) error {
	lockPath := filepath.Join(stagingDir, "node_modules", ".package-lock.json")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return errors.Wrap(err, "read npm package lock")
	}

	var lock npmLockFile
	if err := json.Unmarshal(data, &lock); err != nil {
		return errors.Wrap(err, "parse npm package lock")
	}
	key := "node_modules/" + packageName
	entry, ok := lock.Packages[key]
	if !ok {
		return errors.Errorf("package %q is missing from npm package lock", packageName)
	}
	if entry.Version == "" {
		return errors.Errorf("package %q has no locked version", packageName)
	}
	if entry.Version != expectedVersion {
		return errors.Errorf("package %q version mismatch: got %q, want %q", packageName, entry.Version, expectedVersion)
	}
	if entry.Integrity != expectedIntegrity {
		return errors.Errorf("package %q integrity mismatch: got %q, want %q", packageName, entry.Integrity, expectedIntegrity)
	}
	return nil
}

func resolveNpmBinaryInDir(baseDir, binaryName string) (*a2a888pb.ResolvedBinary, string, error) {
	candidateRelPaths := []string{
		filepath.Join("node_modules", ".bin", binaryName),
		filepath.Join("node_modules", ".bin", binaryName+".cmd"),
		filepath.Join("bin", binaryName),
		binaryName,
	}

	for _, rel := range candidateRelPaths {
		cand := filepath.Join(baseDir, rel)
		info, err := os.Stat(cand)
		if err == nil && !info.IsDir() {
			sha, size, err := computeFileSha256(cand)
			if err != nil {
				return nil, "", errors.Wrap(err, "compute binary sha256")
			}
			return &a2a888pb.ResolvedBinary{
				Path:      cand,
				Binary:    binaryName,
				Sha256:    sha,
				SizeBytes: uint64(size),
				Source:    "npm_prepared_cache",
			}, rel, nil
		}
	}

	return nil, "", errors.Errorf("binary %q not found in %s", binaryName, baseDir)
}

func computeFileSha256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}

// Quarantine moves a failed or corrupted directory into the quarantine area.
func (p *Preparer) Quarantine(identity *a2a888pb.CacheIdentity, sourceDir, reason string) error {
	dest := p.paths.QuarantineDir(identity, time.Now().UnixNano())
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	p.recordAudit("QUARANTINE", identity.GetProviderId(), identity.GetIdentityDigest(), false, reason, dest)
	return os.Rename(sourceDir, dest)
}

// ListQuarantined returns the list of quarantined directory names.
func (p *Preparer) ListQuarantined() ([]string, error) {
	qDir := filepath.Join(p.paths.Root, "quarantine")
	entries, err := os.ReadDir(qDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// RemoveQuarantined removes a quarantined item by its directory name or digest prefix.
// It strictly validates against path traversal attacks.
func (p *Preparer) RemoveQuarantined(name string) error {
	clean := filepath.Clean(name)
	if clean == "" || clean == "." || clean == ".." || filepath.Base(clean) != clean || strings.ContainsAny(clean, `/\`) {
		return errors.New("invalid quarantine entry name: path traversal rejected")
	}

	qDir := filepath.Join(p.paths.Root, "quarantine")
	target := filepath.Join(qDir, clean)
	rel, err := filepath.Rel(qDir, target)
	if err != nil || strings.HasPrefix(rel, "..") {
		return errors.New("quarantine path traversal rejected")
	}

	p.recordAudit("REMOVE_QUARANTINE", "", name, true, "operator removed quarantined package", target)
	return os.RemoveAll(target)
}

// RetryPreparation removes any existing cache/quarantine for this identity and re-runs Prepare.
func (p *Preparer) RetryPreparation(ctx context.Context, manifest *a2a888pb.ProviderManifest, platform *a2a888pb.PlatformTarget) (*a2a888pb.PreparedRuntime, error) {
	identity, err := ComputeCacheIdentity(manifest, platform)
	if err != nil {
		return nil, errors.Wrap(err, "compute cache identity")
	}
	p.mu.Lock()
	pkgDir := p.paths.PackageDir(identity)
	_ = os.RemoveAll(pkgDir)
	p.mu.Unlock()
	return p.Prepare(ctx, manifest, platform)
}
