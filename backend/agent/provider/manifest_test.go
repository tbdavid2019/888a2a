package provider

import (
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	a2a888pb "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

func validNpmManifest() *a2a888pb.ProviderManifest {
	m := &a2a888pb.ProviderManifest{
		ProviderId:    "test-npm",
		DisplayName:   "Test NPM Provider",
		RuntimeKind:   a2a888pb.RuntimeKind_NPM_PACKAGE,
		AgentProtocol: a2a888pb.AgentProtocol_ACP_V1,
		PlatformTargets: []*a2a888pb.PlatformTarget{
			{OperatingSystem: "linux", Architecture: "amd64"},
		},
		RuntimeConfig: &a2a888pb.ProviderManifest_NpmPackage{
			NpmPackage: &a2a888pb.NpmPackageConfig{
				PackageName:    "@888a2a/test-pkg",
				PackageVersion: "1.2.3",
				Binary:         "test-pkg",
				Integrity:      "sha512-validBase64IntegrityCheckValue==",
			},
		},
		Capabilities: &a2a888pb.ProviderCapabilities{
			ModelDiscovery: true,
			Streaming:      true,
		},
		PermissionProfile: &a2a888pb.PermissionProfile{
			ProcessExecution: true,
		},
		SessionBehavior: &a2a888pb.SessionBehavior{
			Mode: a2a888pb.SessionMode_PERSISTENT,
		},
		ManifestVersion: "1",
	}
	_ = SetManifestDigest(m)
	return m
}

func TestValidateManifestFixtures(t *testing.T) {
	paths, err := filepath.Glob("testdata/provider-manifests/*.json")
	if err != nil {
		t.Fatalf("glob provider manifests: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no provider manifest fixtures found")
	}

	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			manifest, err := LoadManifestFile(path)
			if err != nil {
				t.Fatalf("LoadManifestFile(%s) error = %v", path, err)
			}
			if err := ValidateManifest(manifest); err != nil {
				t.Fatalf("ValidateManifest() error = %v", err)
			}
		})
	}
}

func TestValidateManifestValidationRules(t *testing.T) {
	t.Run("nil manifest", func(t *testing.T) {
		err := ValidateManifest(nil)
		if err == nil || !strings.Contains(err.Error(), "manifest is nil") {
			t.Fatalf("want error containing 'manifest is nil', got %v", err)
		}
	})

	t.Run("missing provider_id", func(t *testing.T) {
		m := validNpmManifest()
		m.ProviderId = ""
		if err := ValidateManifest(m); err == nil || !strings.Contains(err.Error(), "provider_id is required") {
			t.Fatalf("want error containing 'provider_id is required', got %v", err)
		}
	})

	t.Run("missing display_name", func(t *testing.T) {
		m := validNpmManifest()
		m.DisplayName = "   "
		if err := ValidateManifest(m); err == nil || !strings.Contains(err.Error(), "display_name is required") {
			t.Fatalf("want error containing 'display_name is required', got %v", err)
		}
	})

	t.Run("unspecified runtime_kind", func(t *testing.T) {
		m := validNpmManifest()
		m.RuntimeKind = a2a888pb.RuntimeKind_RUNTIME_KIND_UNSPECIFIED
		if err := ValidateManifest(m); err == nil || !strings.Contains(err.Error(), "runtime_kind is required") {
			t.Fatalf("want error containing 'runtime_kind is required', got %v", err)
		}
	})

	t.Run("unspecified agent_protocol", func(t *testing.T) {
		m := validNpmManifest()
		m.AgentProtocol = a2a888pb.AgentProtocol_AGENT_PROTOCOL_UNSPECIFIED
		if err := ValidateManifest(m); err == nil || !strings.Contains(err.Error(), "agent_protocol is required") {
			t.Fatalf("want error containing 'agent_protocol is required', got %v", err)
		}
	})

	t.Run("empty platform_targets", func(t *testing.T) {
		m := validNpmManifest()
		m.PlatformTargets = nil
		if err := ValidateManifest(m); err == nil || !strings.Contains(err.Error(), "at least one platform_target") {
			t.Fatalf("want error containing 'at least one platform_target', got %v", err)
		}
	})

	t.Run("invalid platform_target", func(t *testing.T) {
		m := validNpmManifest()
		m.PlatformTargets = []*a2a888pb.PlatformTarget{
			{OperatingSystem: "", Architecture: "amd64"},
		}
		if err := ValidateManifest(m); err == nil || !strings.Contains(err.Error(), "operating_system is required") {
			t.Fatalf("want error containing 'operating_system is required', got %v", err)
		}

		m.PlatformTargets = []*a2a888pb.PlatformTarget{
			{OperatingSystem: "linux", Architecture: ""},
		}
		if err := ValidateManifest(m); err == nil || !strings.Contains(err.Error(), "architecture is required") {
			t.Fatalf("want error containing 'architecture is required', got %v", err)
		}
	})

	t.Run("missing runtime config", func(t *testing.T) {
		m := validNpmManifest()
		m.RuntimeConfig = nil
		if err := ValidateManifest(m); err == nil || !strings.Contains(err.Error(), "runtime_config is required") {
			t.Fatalf("want error containing 'runtime_config is required', got %v", err)
		}
	})
}

func TestValidateNpmPackageRules(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(m *a2a888pb.ProviderManifest)
		wantError string
	}{
		{
			name: "missing package_name",
			mutate: func(m *a2a888pb.ProviderManifest) {
				m.GetNpmPackage().PackageName = ""
			},
			wantError: "package_name is required",
		},
		{
			name: "missing binary",
			mutate: func(m *a2a888pb.ProviderManifest) {
				m.GetNpmPackage().Binary = ""
			},
			wantError: "binary is required",
		},
		{
			name: "missing package_version",
			mutate: func(m *a2a888pb.ProviderManifest) {
				m.GetNpmPackage().PackageVersion = ""
			},
			wantError: "package_version is required",
		},
		{
			name: "floating version latest",
			mutate: func(m *a2a888pb.ProviderManifest) {
				m.GetNpmPackage().PackageVersion = "latest"
			},
			wantError: "must be an exact pinned semver",
		},
		{
			name: "floating version next",
			mutate: func(m *a2a888pb.ProviderManifest) {
				m.GetNpmPackage().PackageVersion = "next"
			},
			wantError: "must be an exact pinned semver",
		},
		{
			name: "caret version range",
			mutate: func(m *a2a888pb.ProviderManifest) {
				m.GetNpmPackage().PackageVersion = "^1.2.3"
			},
			wantError: "must be an exact pinned semver",
		},
		{
			name: "tilde version range",
			mutate: func(m *a2a888pb.ProviderManifest) {
				m.GetNpmPackage().PackageVersion = "~1.2.3"
			},
			wantError: "must be an exact pinned semver",
		},
		{
			name: "wildcard version",
			mutate: func(m *a2a888pb.ProviderManifest) {
				m.GetNpmPackage().PackageVersion = "1.2.*"
			},
			wantError: "must be an exact pinned semver",
		},
		{
			name: "comparison operator version",
			mutate: func(m *a2a888pb.ProviderManifest) {
				m.GetNpmPackage().PackageVersion = ">=1.0.0"
			},
			wantError: "must be an exact pinned semver",
		},
		{
			name: "missing integrity",
			mutate: func(m *a2a888pb.ProviderManifest) {
				m.GetNpmPackage().Integrity = ""
			},
			wantError: "integrity is required",
		},
		{
			name: "invalid integrity format without prefix",
			mutate: func(m *a2a888pb.ProviderManifest) {
				m.GetNpmPackage().Integrity = "invalid-integrity-string"
			},
			wantError: "Subresource Integrity",
		},
		{
			name: "invalid integrity format wrong algorithm",
			mutate: func(m *a2a888pb.ProviderManifest) {
				m.GetNpmPackage().Integrity = "md5-Zml4dHVyZQ=="
			},
			wantError: "Subresource Integrity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := validNpmManifest()
			tt.mutate(m)
			err := ValidateManifest(m)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantError)
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantError)
			}
		})
	}

	t.Run("exact valid semver versions", func(t *testing.T) {
		validVersions := []string{
			"0.0.1",
			"1.0.0",
			"1.2.3",
			"10.20.30",
			"1.0.0-alpha",
			"1.0.0-alpha.1",
			"1.0.0-0.3.7",
			"1.0.0-x.7.z.92",
			"1.0.0-beta+exp.sha.5114f85",
			"1.0.0+20130313144700",
		}
		for _, v := range validVersions {
			m := validNpmManifest()
			m.GetNpmPackage().PackageVersion = v
			_ = SetManifestDigest(m)
			if err := ValidateManifest(m); err != nil {
				t.Errorf("version %q should be valid, got error: %v", v, err)
			}
		}
	})

	t.Run("valid SRI algorithms", func(t *testing.T) {
		validSRIs := []string{
			"sha256-47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU=",
			"sha384-d91XMTfngWQ3Th6xpy52Ya8An8N05Q63k0nQ==",
			"sha512-Zml4dHVyZS1wcm92aWRlci1wYWNrYWdlLWludGVncml0eQ==",
		}
		for _, sri := range validSRIs {
			m := validNpmManifest()
			m.GetNpmPackage().Integrity = sri
			_ = SetManifestDigest(m)
			if err := ValidateManifest(m); err != nil {
				t.Errorf("SRI %q should be valid, got error: %v", sri, err)
			}
		}
	})
}

func TestValidateManifestIntegrityAndMetadata(t *testing.T) {
	t.Run("missing capabilities", func(t *testing.T) {
		m := validNpmManifest()
		m.Capabilities = nil
		if err := ValidateManifest(m); err == nil || !strings.Contains(err.Error(), "capabilities is required") {
			t.Fatalf("want error containing 'capabilities is required', got %v", err)
		}
	})

	t.Run("missing permission_profile", func(t *testing.T) {
		m := validNpmManifest()
		m.PermissionProfile = nil
		if err := ValidateManifest(m); err == nil || !strings.Contains(err.Error(), "permission_profile is required") {
			t.Fatalf("want error containing 'permission_profile is required', got %v", err)
		}
	})

	t.Run("missing session_behavior", func(t *testing.T) {
		m := validNpmManifest()
		m.SessionBehavior = nil
		if err := ValidateManifest(m); err == nil || !strings.Contains(err.Error(), "session_behavior is required") {
			t.Fatalf("want error containing 'session_behavior is required', got %v", err)
		}
	})

	t.Run("unspecified session mode", func(t *testing.T) {
		m := validNpmManifest()
		m.SessionBehavior.Mode = a2a888pb.SessionMode_SESSION_MODE_UNSPECIFIED
		if err := ValidateManifest(m); err == nil || !strings.Contains(err.Error(), "session_behavior.mode is required") {
			t.Fatalf("want error containing 'session_behavior.mode is required', got %v", err)
		}
	})

	t.Run("missing manifest_version", func(t *testing.T) {
		m := validNpmManifest()
		m.ManifestVersion = ""
		if err := ValidateManifest(m); err == nil || !strings.Contains(err.Error(), "manifest_version is required") {
			t.Fatalf("want error containing 'manifest_version is required', got %v", err)
		}
	})

	t.Run("invalid manifest_integrity_sha256 length", func(t *testing.T) {
		m := validNpmManifest()
		m.ManifestIntegritySha256 = "short-hash"
		if err := ValidateManifest(m); err == nil || !strings.Contains(err.Error(), "64-character SHA-256") {
			t.Fatalf("want error containing '64-character SHA-256', got %v", err)
		}
	})

	t.Run("invalid manifest_integrity_sha256 non-hex", func(t *testing.T) {
		m := validNpmManifest()
		m.ManifestIntegritySha256 = strings.Repeat("z", 64)
		if err := ValidateManifest(m); err == nil || !strings.Contains(err.Error(), "not a valid hex string") {
			t.Fatalf("want error containing 'not a valid hex string', got %v", err)
		}
	})
}

func TestValidateOtherRuntimeKinds(t *testing.T) {
	t.Run("system executable missing executable", func(t *testing.T) {
		m := validNpmManifest()
		m.RuntimeKind = a2a888pb.RuntimeKind_SYSTEM_EXECUTABLE
		m.RuntimeConfig = &a2a888pb.ProviderManifest_SystemExecutable{
			SystemExecutable: &a2a888pb.SystemExecutableConfig{
				Executable: "",
			},
		}
		if err := ValidateManifest(m); err == nil || !strings.Contains(err.Error(), "executable is required") {
			t.Fatalf("want error containing 'executable is required', got %v", err)
		}
	})

	t.Run("embedded missing artifact and binary", func(t *testing.T) {
		m := validNpmManifest()
		m.RuntimeKind = a2a888pb.RuntimeKind_EMBEDDED
		m.RuntimeConfig = &a2a888pb.ProviderManifest_Embedded{
			Embedded: &a2a888pb.EmbeddedRuntimeConfig{
				Artifact: "",
				Binary:   "",
			},
		}
		if err := ValidateManifest(m); err == nil || !strings.Contains(err.Error(), "artifact or embedded.binary is required") {
			t.Fatalf("want error containing 'artifact or embedded.binary is required', got %v", err)
		}
	})

	t.Run("custom runtime missing command", func(t *testing.T) {
		m := validNpmManifest()
		m.RuntimeKind = a2a888pb.RuntimeKind_CUSTOM_RUNTIME
		m.RuntimeConfig = &a2a888pb.ProviderManifest_Custom{
			Custom: &a2a888pb.CustomRuntimeConfig{
				Command: "",
			},
		}
		if err := ValidateManifest(m); err == nil || !strings.Contains(err.Error(), "custom.command is required") {
			t.Fatalf("want error containing 'custom.command is required', got %v", err)
		}
	})

	t.Run("mismatched runtime kind and config", func(t *testing.T) {
		m := validNpmManifest()
		m.RuntimeKind = a2a888pb.RuntimeKind_SYSTEM_EXECUTABLE
		// Config is still NpmPackage
		if err := ValidateManifest(m); err == nil || !strings.Contains(err.Error(), "system_executable config is required") {
			t.Fatalf("want error containing 'system_executable config is required', got %v", err)
		}
	})
}

func TestParseManifest(t *testing.T) {
	t.Run("invalid json", func(t *testing.T) {
		_, err := ParseManifest([]byte("{invalid-json"))
		if err == nil {
			t.Fatal("expected unmarshal error, got nil")
		}
	})

	t.Run("valid manifest json round trip", func(t *testing.T) {
		base := validNpmManifest()
		data, err := proto.Marshal(base)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		_ = data // proto.Marshal test
	})
}

func TestBuiltinAndCustomManifestsValid(t *testing.T) {
	for _, p := range Default().All() {
		t.Run(p.ID(), func(t *testing.T) {
			m := p.Manifest()
			if m == nil {
				t.Fatalf("provider %s returned nil manifest", p.ID())
			}
			if err := ValidateManifest(m); err != nil {
				t.Fatalf("provider %s manifest invalid: %v", p.ID(), err)
			}
		})
	}

	t.Run("builtin-pi", func(t *testing.T) {
		m := BuiltinPiManifest()
		if err := ValidateManifest(m); err != nil {
			t.Fatalf("builtin-pi manifest invalid: %v", err)
		}
	})

	t.Run("custom", func(t *testing.T) {
		m := CustomManifest("my-agent", []string{"--flag"}, a2a888pb.AgentProtocol_ACP_V2)
		if err := ValidateManifest(m); err != nil {
			t.Fatalf("custom manifest invalid: %v", err)
		}
	})
}

func TestRegistryResolveRuntimeManifest(t *testing.T) {
	r := Default()

	// Builtin provider
	m, err := r.ResolveRuntimeManifest("opencode", "", nil, a2a888pb.AgentProtocol_AGENT_PROTOCOL_UNSPECIFIED)
	if err != nil || m == nil || m.GetProviderId() != "opencode" {
		t.Fatalf("resolve opencode: %v, %+v", err, m)
	}

	// Builtin pi
	m, err = r.ResolveRuntimeManifest("builtin-pi", "", nil, a2a888pb.AgentProtocol_AGENT_PROTOCOL_UNSPECIFIED)
	if err != nil || m == nil || m.GetProviderId() != "builtin-pi" {
		t.Fatalf("resolve builtin-pi: %v, %+v", err, m)
	}

	// Custom provider
	m, err = r.ResolveRuntimeManifest("custom", "test-bin", []string{"serve"}, a2a888pb.AgentProtocol_ACP_V2)
	if err != nil || m == nil || m.GetProviderId() != "custom" || m.GetAgentProtocol() != a2a888pb.AgentProtocol_ACP_V2 {
		t.Fatalf("resolve custom: %v, %+v", err, m)
	}

	// Unknown provider
	_, err = r.ResolveRuntimeManifest("nonexistent-provider", "", nil, a2a888pb.AgentProtocol_AGENT_PROTOCOL_UNSPECIFIED)
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
}
