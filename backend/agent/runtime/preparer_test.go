//nolint:revive // package name is retained for the existing runtime import path.
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tbdavid2019/888a2a/backend/agent/provider"
	a2a888pb "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

type mockRunner struct {
	onRun func(ctx context.Context, cmd string, args []string, dir string) ([]byte, error)
}

func writePackageLock(t *testing.T, dir, integrity string) {
	writePackageLockVersion(t, dir, "0.70.0", integrity)
}

func writePackageLockVersion(t *testing.T, dir, version, integrity string) {
	t.Helper()
	lock := map[string]any{
		"lockfileVersion": 3,
		"packages": map[string]any{
			"": map[string]any{},
			"node_modules/@agentclientprotocol/claude-agent-acp": map[string]any{
				"version":   version,
				"integrity": integrity,
			},
		},
	}
	data, err := json.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "node_modules", ".package-lock.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPreparerRollbackActivatesPreviousVerifiedRuntime(t *testing.T) {
	root := t.TempDir()
	platform := &a2a888pb.PlatformTarget{OperatingSystem: "linux", Architecture: "amd64"}
	callCount := 0
	runner := &mockRunner{onRun: func(_ context.Context, _ string, args []string, dir string) ([]byte, error) {
		callCount++
		target := args[len(args)-1]
		version := target[strings.LastIndex(target, "@")+1:]
		binDir := filepath.Join(dir, "node_modules", ".bin")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(binDir, "claude-agent-acp"), []byte(version), 0o755); err != nil {
			return nil, err
		}
		writePackageLockVersion(t, dir, version, testManifest().GetNpmPackage().GetIntegrity())
		return []byte("ok"), nil
	}}

	preparer := NewPreparer(root, runner)
	first := testManifest()
	first.GetNpmPackage().PackageVersion = "0.70.0"
	if _, err := preparer.Prepare(context.Background(), first, platform); err != nil {
		t.Fatalf("prepare first runtime: %v", err)
	}
	second := testManifest()
	second.GetNpmPackage().PackageVersion = "0.71.0"
	if err := provider.SetManifestDigest(second); err != nil {
		t.Fatal(err)
	}
	if _, err := preparer.Prepare(context.Background(), second, platform); err != nil {
		t.Fatalf("prepare second runtime: %v", err)
	}

	rolledBack, err := preparer.Rollback(context.Background(), second.GetProviderId(), platform)
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if got := rolledBack.GetCacheIdentity().GetPackageVersion(); got != "0.70.0" {
		t.Fatalf("rollback package version = %q, want 0.70.0", got)
	}
	if got, err := os.ReadFile(rolledBack.GetResolvedBinary().GetPath()); err != nil || string(got) != "0.70.0" {
		t.Fatalf("rollback binary = %q, err=%v", got, err)
	}

	preparedAgain, err := preparer.Prepare(context.Background(), second, platform)
	if err != nil {
		t.Fatalf("prepare after rollback: %v", err)
	}
	if got := preparedAgain.GetCacheIdentity().GetPackageVersion(); got != "0.70.0" {
		t.Fatalf("active rollback version = %q, want 0.70.0", got)
	}

	if _, err := preparer.RetryPreparation(context.Background(), second, platform); err != nil {
		t.Fatalf("repair after rollback: %v", err)
	}
	if callCount != 3 {
		t.Fatalf("npm calls = %d, want 3 (two prepares plus repair)", callCount)
	}
}

func (m *mockRunner) Run(ctx context.Context, cmd string, args []string, dir string, _ []string) ([]byte, error) {
	if m.onRun != nil {
		return m.onRun(ctx, cmd, args, dir)
	}
	return []byte("ok"), nil
}

func TestPreparerNpmAtomicSuccess(t *testing.T) {
	root := t.TempDir()

	runner := &mockRunner{
		onRun: func(_ context.Context, _ string, _ []string, dir string) ([]byte, error) {
			// Simulate npm placing binary into node_modules/.bin/claude-agent-acp
			binDir := filepath.Join(dir, "node_modules", ".bin")
			if err := os.MkdirAll(binDir, 0o755); err != nil {
				return nil, err
			}
			binFile := filepath.Join(binDir, "claude-agent-acp")
			if err := os.WriteFile(binFile, []byte("#!/bin/sh\necho acp\n"), 0o755); err != nil {
				return nil, err
			}
			writePackageLock(t, dir, testManifest().GetNpmPackage().GetIntegrity())
			return []byte("added 1 package in 0.5s"), nil
		},
	}

	preparer := NewPreparer(root, runner)
	m := testManifest()
	platform := &a2a888pb.PlatformTarget{OperatingSystem: "linux", Architecture: "amd64"}

	// 1. Initial Preparation
	runtime, err := preparer.Prepare(context.Background(), m, platform)
	if err != nil {
		t.Fatalf("Prepare error: %v", err)
	}
	if runtime.GetStatus().GetState() != a2a888pb.RuntimeState_READY {
		t.Fatalf("runtime state = %v, want READY", runtime.GetStatus().GetState())
	}
	if runtime.GetResolvedBinary() == nil || runtime.GetResolvedBinary().GetBinary() != "claude-agent-acp" {
		t.Fatalf("resolved binary = %+v", runtime.GetResolvedBinary())
	}
	if runtime.GetResolvedBinary().GetSha256() == "" {
		t.Fatal("resolved binary sha256 should not be empty")
	}

	// Verify .runtime_meta.json was created
	identity, err := ComputeCacheIdentity(m, platform)
	if err != nil {
		t.Fatal(err)
	}
	metaFile := filepath.Join(preparer.Paths().PackageDir(identity), ".runtime_meta.json")
	if _, err := os.Stat(metaFile); err != nil {
		t.Fatalf("metadata file missing: %v", err)
	}

	// 2. Second Call (Idempotent Verified Cache Hit)
	// Change runner to fail if npm is invoked again
	preparer.runner = &mockRunner{
		onRun: func(_ context.Context, _ string, _ []string, _ string) ([]byte, error) {
			t.Fatal("npm should not be invoked on cached runtime")
			return nil, errors.New("unexpected runner call")
		},
	}

	runtime2, err := preparer.Prepare(context.Background(), m, platform)
	if err != nil {
		t.Fatalf("Prepare cached hit error: %v", err)
	}
	if runtime2.GetStatus().GetState() != a2a888pb.RuntimeState_READY {
		t.Fatalf("cached runtime state = %v, want READY", runtime2.GetStatus().GetState())
	}
}

func TestPreparerNpmRejectsPackageIntegrityMismatch(t *testing.T) {
	root := t.TempDir()
	runner := &mockRunner{
		onRun: func(_ context.Context, _ string, _ []string, dir string) ([]byte, error) {
			binDir := filepath.Join(dir, "node_modules", ".bin")
			if err := os.MkdirAll(binDir, 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(filepath.Join(binDir, "claude-agent-acp"), []byte("binary"), 0o755); err != nil {
				return nil, err
			}
			writePackageLock(t, dir, "sha512-wrong-integrity=")
			return []byte("ok"), nil
		},
	}

	preparer := NewPreparer(root, runner)
	runtime, err := preparer.Prepare(context.Background(), testManifest(), nil)
	if err == nil {
		t.Fatal("expected package integrity mismatch error")
	}
	if runtime.GetStatus().GetState() != a2a888pb.RuntimeState_QUARANTINED {
		t.Fatalf("state = %v, want QUARANTINED", runtime.GetStatus().GetState())
	}
}

func TestPreparerNpmTamperDetectionQuarantines(t *testing.T) {
	root := t.TempDir()

	runner := &mockRunner{
		onRun: func(_ context.Context, _ string, _ []string, dir string) ([]byte, error) {
			binDir := filepath.Join(dir, "node_modules", ".bin")
			if err := os.MkdirAll(binDir, 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(filepath.Join(binDir, "claude-agent-acp"), []byte("original-code"), 0o755); err != nil {
				return nil, err
			}
			writePackageLock(t, dir, testManifest().GetNpmPackage().GetIntegrity())
			return []byte("ok"), nil
		},
	}

	preparer := NewPreparer(root, runner)
	m := testManifest()
	platform := &a2a888pb.PlatformTarget{OperatingSystem: "linux", Architecture: "amd64"}

	// 1. Initial Preparation -> READY
	r1, err := preparer.Prepare(context.Background(), m, platform)
	if err != nil || r1.GetStatus().GetState() != a2a888pb.RuntimeState_READY {
		t.Fatalf("initial prep failed: %v", err)
	}
	binPath := r1.GetResolvedBinary().GetPath()

	// 2. Tamper the binary on disk!
	if err := os.WriteFile(binPath, []byte("tampered-malicious-content"), 0o755); err != nil {
		t.Fatalf("write tampered file: %v", err)
	}

	// 3. Next Prepare call MUST detect tamper, reject launch, quarantine the package, and return QUARANTINED
	r2, err := preparer.Prepare(context.Background(), m, platform)
	if err == nil {
		t.Fatal("expected error on tampered binary, got nil")
	}
	if r2.GetStatus().GetState() != a2a888pb.RuntimeState_QUARANTINED {
		t.Fatalf("state = %v, want QUARANTINED", r2.GetStatus().GetState())
	}

	quarantined, err := preparer.ListQuarantined()
	if err != nil || len(quarantined) != 1 {
		t.Fatalf("expected 1 quarantined entry, got %v (err: %v)", quarantined, err)
	}

	// Check audit log recorded TAMPER_DETECTED
	var foundTamperAudit bool
	for _, log := range preparer.AuditLog() {
		if log.Action == "TAMPER_DETECTED" {
			foundTamperAudit = true
			break
		}
	}
	if !foundTamperAudit {
		t.Fatal("expected TAMPER_DETECTED audit event")
	}
}

func TestPreparerNpmInterruptionLeavesPreviousIntact(t *testing.T) {
	root := t.TempDir()

	// 1. First successfully prepare version 0.70.0
	runner := &mockRunner{
		onRun: func(_ context.Context, _ string, _ []string, dir string) ([]byte, error) {
			binDir := filepath.Join(dir, "node_modules", ".bin")
			if err := os.MkdirAll(binDir, 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(filepath.Join(binDir, "claude-agent-acp"), []byte("v0.70.0"), 0o755); err != nil {
				return nil, err
			}
			writePackageLock(t, dir, testManifest().GetNpmPackage().GetIntegrity())
			return []byte("ok"), nil
		},
	}

	preparer := NewPreparer(root, runner)
	m := testManifest()
	platform := &a2a888pb.PlatformTarget{OperatingSystem: "linux", Architecture: "amd64"}

	r1, err := preparer.Prepare(context.Background(), m, platform)
	if err != nil || r1.GetStatus().GetState() != a2a888pb.RuntimeState_READY {
		t.Fatalf("initial prep failed: %v, %+v", err, r1)
	}
	v1BinPath := r1.GetResolvedBinary().GetPath()

	// 2. Attempt upgrade to 0.71.0, but install fails / is interrupted
	m2 := testManifest()
	m2.GetNpmPackage().PackageVersion = "0.71.0"
	_ = provider.SetManifestDigest(m2)

	preparer.runner = &mockRunner{
		onRun: func(_ context.Context, _ string, _ []string, _ string) ([]byte, error) {
			return []byte("network timeout"), errors.New("connection reset by peer")
		},
	}

	r2, err := preparer.Prepare(context.Background(), m2, platform)
	if err == nil {
		t.Fatal("expected error on failed install, got nil")
	}
	if r2.GetStatus().GetState() != a2a888pb.RuntimeState_BROKEN {
		t.Fatalf("state = %v, want BROKEN", r2.GetStatus().GetState())
	}

	// 3. Verify that the previous v0.70.0 installation remains completely intact and runnable
	if data, err := os.ReadFile(v1BinPath); err != nil || string(data) != "v0.70.0" {
		t.Fatalf("previous version corrupted: %v, data=%q", err, string(data))
	}
}

func TestPreparerNpmQuarantineOnCorruptedBinary(t *testing.T) {
	root := t.TempDir()

	// Runner succeeds but does not produce the expected binary
	runner := &mockRunner{
		onRun: func(_ context.Context, _ string, _ []string, _ string) ([]byte, error) {
			return []byte("npm success"), nil
		},
	}

	preparer := NewPreparer(root, runner)
	m := testManifest()
	platform := &a2a888pb.PlatformTarget{OperatingSystem: "linux", Architecture: "amd64"}

	r, err := preparer.Prepare(context.Background(), m, platform)
	if err == nil {
		t.Fatal("expected error due to missing binary, got nil")
	}
	if r.GetStatus().GetState() != a2a888pb.RuntimeState_QUARANTINED {
		t.Fatalf("state = %v, want QUARANTINED", r.GetStatus().GetState())
	}

	quarantined, err := preparer.ListQuarantined()
	if err != nil {
		t.Fatalf("ListQuarantined error: %v", err)
	}
	if len(quarantined) != 1 {
		t.Fatalf("quarantined count = %d, want 1", len(quarantined))
	}

	// Remove quarantined item
	if err := preparer.RemoveQuarantined(quarantined[0]); err != nil {
		t.Fatalf("RemoveQuarantined error: %v", err)
	}

	quarantinedAfter, err := preparer.ListQuarantined()
	if err != nil || len(quarantinedAfter) != 0 {
		t.Fatalf("quarantined after removal = %v", quarantinedAfter)
	}
}

func TestPreparerQuarantinePathTraversalRejection(t *testing.T) {
	root := t.TempDir()
	preparer := NewPreparer(root, nil)

	traversalInputs := []string{
		"../../etc/passwd",
		"..",
		"../foo",
		"/root",
		"foo/bar",
		`..\..\windows\system32`,
	}

	for _, input := range traversalInputs {
		err := preparer.RemoveQuarantined(input)
		if err == nil || !strings.Contains(err.Error(), "path traversal rejected") {
			t.Errorf("input %q should have been rejected as path traversal, got %v", input, err)
		}
	}
}

func TestPreparerRetryOperation(t *testing.T) {
	root := t.TempDir()
	callCount := 0

	runner := &mockRunner{
		onRun: func(_ context.Context, _ string, _ []string, dir string) ([]byte, error) {
			callCount++
			if callCount == 1 {
				// First call fails
				return []byte("temporary failure"), errors.New("npm error")
			}
			// Second call succeeds
			binDir := filepath.Join(dir, "node_modules", ".bin")
			_ = os.MkdirAll(binDir, 0o755)
			_ = os.WriteFile(filepath.Join(binDir, "claude-agent-acp"), []byte("ok"), 0o755)
			writePackageLock(t, dir, testManifest().GetNpmPackage().GetIntegrity())
			return []byte("ok"), nil
		},
	}

	preparer := NewPreparer(root, runner)
	m := testManifest()
	platform := &a2a888pb.PlatformTarget{OperatingSystem: "linux", Architecture: "amd64"}

	// 1. Initial failed prep
	r1, err := preparer.Prepare(context.Background(), m, platform)
	if err == nil || r1.GetStatus().GetState() != a2a888pb.RuntimeState_BROKEN {
		t.Fatalf("expected first prep to fail, got %v", err)
	}

	// 2. Retry succeeds
	r2, err := preparer.RetryPreparation(context.Background(), m, platform)
	if err != nil || r2.GetStatus().GetState() != a2a888pb.RuntimeState_READY {
		t.Fatalf("retry failed: %v, state=%v", err, r2.GetStatus().GetState())
	}
}

func TestPreparerAuditLogNoSecrets(t *testing.T) {
	root := t.TempDir()
	runner := &mockRunner{
		onRun: func(_ context.Context, _ string, _ []string, dir string) ([]byte, error) {
			binDir := filepath.Join(dir, "node_modules", ".bin")
			_ = os.MkdirAll(binDir, 0o755)
			_ = os.WriteFile(filepath.Join(binDir, "claude-agent-acp"), []byte("bin"), 0o755)
			writePackageLock(t, dir, testManifest().GetNpmPackage().GetIntegrity())
			return []byte("Bearer secret_api_token_12345"), nil
		},
	}

	preparer := NewPreparer(root, runner)
	m := testManifest()
	_, _ = preparer.Prepare(context.Background(), m, nil)

	logs := preparer.AuditLog()
	if len(logs) == 0 {
		t.Fatal("expected audit logs")
	}

	for _, log := range logs {
		if strings.Contains(log.Details, "secret_api_token_12345") || strings.Contains(log.Reason, "secret_api_token_12345") {
			t.Fatalf("secret leaked in audit log: %+v", log)
		}
	}
}
