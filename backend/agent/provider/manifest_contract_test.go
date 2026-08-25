package provider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Ranxy/laelia/backend/common"
	a2a888pb "github.com/Ranxy/laelia/backend/generated-go/a2a888"
)

func TestProviderManifestContractFixtures(t *testing.T) {
	paths, err := filepath.Glob("testdata/provider-manifests/*.json")
	if err != nil {
		t.Fatalf("glob provider manifests: %v", err)
	}
	if len(paths) != 4 {
		t.Fatalf("provider manifest fixture count = %d, want 4", len(paths))
	}

	kinds := make(map[a2a888pb.RuntimeKind]string, len(paths))
	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}

			manifest := &a2a888pb.ProviderManifest{}
			if err := common.ProtojsonUnmarshaler.Unmarshal(raw, manifest); err != nil {
				t.Fatalf("decode fixture: %v", err)
			}
			assertManifestContract(t, manifest)

			encoded, err := protojson.Marshal(manifest)
			if err != nil {
				t.Fatalf("encode fixture: %v", err)
			}
			roundTrip := &a2a888pb.ProviderManifest{}
			if err := common.ProtojsonUnmarshaler.Unmarshal(encoded, roundTrip); err != nil {
				t.Fatalf("decode round trip: %v", err)
			}
			if !proto.Equal(manifest, roundTrip) {
				t.Fatal("manifest changed during proto JSON round trip")
			}

			if prior, ok := kinds[manifest.GetRuntimeKind()]; ok {
				t.Fatalf("runtime kind %s already covered by %s", manifest.GetRuntimeKind(), prior)
			}
			kinds[manifest.GetRuntimeKind()] = filepath.Base(path)
		})
	}

	wantKinds := []a2a888pb.RuntimeKind{
		a2a888pb.RuntimeKind_SYSTEM_EXECUTABLE,
		a2a888pb.RuntimeKind_NPM_PACKAGE,
		a2a888pb.RuntimeKind_EMBEDDED,
		a2a888pb.RuntimeKind_CUSTOM_RUNTIME,
	}
	for _, kind := range wantKinds {
		if _, ok := kinds[kind]; !ok {
			t.Errorf("runtime kind %s has no contract fixture", kind)
		}
	}
}

func assertManifestContract(t *testing.T, manifest *a2a888pb.ProviderManifest) {
	t.Helper()
	if manifest.GetProviderId() == "" || manifest.GetDisplayName() == "" {
		t.Fatal("provider identity is required")
	}
	if manifest.GetRuntimeKind() == a2a888pb.RuntimeKind_RUNTIME_KIND_UNSPECIFIED {
		t.Fatal("runtime kind is required")
	}
	if manifest.GetAgentProtocol() == a2a888pb.AgentProtocol_AGENT_PROTOCOL_UNSPECIFIED {
		t.Fatal("agent protocol is required")
	}
	if len(manifest.GetPlatformTargets()) == 0 {
		t.Fatal("at least one platform target is required")
	}
	if manifest.GetRuntimeConfig() == nil {
		t.Fatal("one runtime configuration is required")
	}
	switch manifest.GetRuntimeKind() {
	case a2a888pb.RuntimeKind_SYSTEM_EXECUTABLE:
		if manifest.GetSystemExecutable() == nil {
			t.Fatal("system runtime kind requires system executable config")
		}
	case a2a888pb.RuntimeKind_NPM_PACKAGE:
		if manifest.GetNpmPackage() == nil {
			t.Fatal("npm runtime kind requires npm package config")
		}
	case a2a888pb.RuntimeKind_EMBEDDED:
		if manifest.GetEmbedded() == nil {
			t.Fatal("embedded runtime kind requires embedded config")
		}
	case a2a888pb.RuntimeKind_CUSTOM_RUNTIME:
		if manifest.GetCustom() == nil {
			t.Fatal("custom runtime kind requires custom config")
		}
	default:
		t.Fatalf("unsupported runtime kind %s", manifest.GetRuntimeKind())
	}
	if manifest.GetCapabilities() == nil || manifest.GetPermissionProfile() == nil || manifest.GetSessionBehavior() == nil {
		t.Fatal("capabilities, permission profile, and session behavior are required")
	}
	if manifest.GetManifestVersion() == "" || len(manifest.GetManifestIntegritySha256()) != 64 {
		t.Fatal("manifest version and SHA-256 integrity are required")
	}
	if npm := manifest.GetNpmPackage(); npm != nil {
		if npm.GetPackageVersion() == "" || npm.GetPackageVersion() == "latest" || strings.ContainsAny(npm.GetPackageVersion(), "*^~<>") {
			t.Fatalf("npm fixture version %q is not exact", npm.GetPackageVersion())
		}
		if !strings.HasPrefix(npm.GetIntegrity(), "sha512-") {
			t.Fatalf("npm fixture integrity %q is not SRI sha512", npm.GetIntegrity())
		}
	}
}

func TestPreparedRuntimeContractRoundTrip(t *testing.T) {
	observedAt := timestamppb.New(time.Unix(1_700_000_000, 0))
	runtime := &a2a888pb.PreparedRuntime{
		ProviderId: "fixture-npm-acp",
		CacheIdentity: &a2a888pb.CacheIdentity{
			IdentityDigest: "sha256:cache",
			ProviderId:     "fixture-npm-acp",
			ManifestDigest: "sha256:manifest",
			RuntimeVersion: "1.2.3",
			Platform:       &a2a888pb.PlatformTarget{OperatingSystem: "linux", Architecture: "amd64"},
			PackageName:    "@888a2a/fixture-acp",
			PackageVersion: "1.2.3",
			Integrity:      "sha512-fixture",
		},
		ResolvedBinary: &a2a888pb.ResolvedBinary{
			Path:    "/runtime-cache/fixture-acp",
			Binary:  "fixture-acp",
			Version: "1.2.3",
			Sha256:  "sha256:binary",
		},
		Status: &a2a888pb.RuntimeStatus{
			State:           a2a888pb.RuntimeState_READY,
			ObservedVersion: "1.2.3",
			ObservedAt:      observedAt,
		},
		Compatibility: &a2a888pb.CompatibilityReport{
			Level: a2a888pb.CompatibilityLevel_FULL_LOOP_VERIFIED,
			Evidence: []*a2a888pb.CompatibilityEvidence{{
				Version:  "1.2.3",
				Platform: &a2a888pb.PlatformTarget{OperatingSystem: "linux", Architecture: "amd64"},
				TestedAt: observedAt,
				Details:  "deterministic fixture passed the full runtime loop",
			}},
		},
		PreparedAt: observedAt,
	}

	encoded, err := protojson.Marshal(runtime)
	if err != nil {
		t.Fatalf("encode prepared runtime: %v", err)
	}
	roundTrip := &a2a888pb.PreparedRuntime{}
	if err := common.ProtojsonUnmarshaler.Unmarshal(encoded, roundTrip); err != nil {
		t.Fatalf("decode prepared runtime: %v", err)
	}
	if !proto.Equal(runtime, roundTrip) {
		t.Fatal("prepared runtime changed during proto JSON round trip")
	}
}
