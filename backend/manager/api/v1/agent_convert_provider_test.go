package v1

import (
	"strings"
	"testing"

	storepb "github.com/tbdavid2019/888a2a/backend/generated-go/store"
)

func TestConvertToV1ProvidersSanitizesAndPreservesProviderEvidence(t *testing.T) {
	got := convertToV1Providers([]*storepb.AgentProviderInfo{{
		ProviderId:         "codex",
		DisplayName:        "Codex",
		Version:            "1.2.3",
		ExecutablePath:     "/private/should-not-leave-manager",
		RuntimeStatus:      "DETECTED",
		CompatibilityLevel: "DETECTED",
		FailureMessage:     "Bearer hidden session_id=abc /private/config",
		PackageVersion:     "1.2.3",
		ManifestDigest:     "digest",
	}})
	if len(got) != 1 {
		t.Fatalf("providers = %d, want 1", len(got))
	}
	if got[0].ExecutablePath != "" || got[0].RuntimeStatus != "DETECTED" || got[0].PackageVersion != "1.2.3" || got[0].ManifestDigest != "digest" {
		t.Fatalf("provider evidence was not projected safely: %+v", got[0])
	}
	if strings.Contains(got[0].FailureMessage, "hidden") || strings.Contains(got[0].FailureMessage, "abc") || strings.Contains(got[0].FailureMessage, "/private/config") {
		t.Fatalf("provider failure leaked sensitive detail: %q", got[0].FailureMessage)
	}
}
