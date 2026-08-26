//nolint:revive // package name is retained for the existing runtime import path.
package runtime

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/tbdavid2019/888a2a/backend/agent/provider"
	a2a888pb "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

func testManifest() *a2a888pb.ProviderManifest {
	m := &a2a888pb.ProviderManifest{
		ProviderId:    "claude-code",
		DisplayName:   "Claude Code",
		RuntimeKind:   a2a888pb.RuntimeKind_NPM_PACKAGE,
		AgentProtocol: a2a888pb.AgentProtocol_ACP_V1,
		PlatformTargets: []*a2a888pb.PlatformTarget{
			{OperatingSystem: "linux", Architecture: "amd64"},
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
		Capabilities:      &a2a888pb.ProviderCapabilities{Streaming: true},
		PermissionProfile: &a2a888pb.PermissionProfile{ProcessExecution: true},
		SessionBehavior:   &a2a888pb.SessionBehavior{Mode: a2a888pb.SessionMode_PERSISTENT},
		ManifestVersion:   "1",
	}
	_ = provider.SetManifestDigest(m)
	return m
}

func TestComputeCacheIdentityDeterministic(t *testing.T) {
	m := testManifest()
	p := &a2a888pb.PlatformTarget{OperatingSystem: "linux", Architecture: "amd64"}

	id1, err := ComputeCacheIdentity(m, p)
	if err != nil {
		t.Fatalf("compute 1: %v", err)
	}
	id2, err := ComputeCacheIdentity(m, p)
	if err != nil {
		t.Fatalf("compute 2: %v", err)
	}

	if id1.GetIdentityDigest() != id2.GetIdentityDigest() {
		t.Fatalf("cache identity digest not deterministic: %q vs %q", id1.GetIdentityDigest(), id2.GetIdentityDigest())
	}
	if len(id1.GetIdentityDigest()) != 64 {
		t.Fatalf("digest length = %d, want 64", len(id1.GetIdentityDigest()))
	}
	if id1.GetPackageName() != "@agentclientprotocol/claude-agent-acp" {
		t.Errorf("packageName = %q", id1.GetPackageName())
	}
	if id1.GetPackageVersion() != "0.70.0" {
		t.Errorf("packageVersion = %q", id1.GetPackageVersion())
	}
}

func TestCachePaths(t *testing.T) {
	root := "/tmp/a2a888-runtime-test"
	cp := NewCachePaths(root)

	m := testManifest()
	id, err := ComputeCacheIdentity(m, nil)
	if err != nil {
		t.Fatal(err)
	}

	pkgDir := cp.PackageDir(id)
	wantPkg := filepath.Join(root, "packages", id.GetIdentityDigest())
	if pkgDir != wantPkg {
		t.Errorf("PackageDir = %q, want %q", pkgDir, wantPkg)
	}

	stagingDir := cp.StagingDir(id, "sess-1")
	if !strings.HasPrefix(stagingDir, filepath.Join(root, "staging", id.GetIdentityDigest())) {
		t.Errorf("StagingDir = %q", stagingDir)
	}

	quarantineDir := cp.QuarantineDir(id, 12345)
	if !strings.HasPrefix(quarantineDir, filepath.Join(root, "quarantine", id.GetIdentityDigest())) {
		t.Errorf("QuarantineDir = %q", quarantineDir)
	}
}
