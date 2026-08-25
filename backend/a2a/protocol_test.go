package a2a

import (
	"testing"

	officiala2a "github.com/a2aproject/a2a-go/v2/a2a"
)

func TestSupportedProtocolReportsOfficialA2AOnePointZero(t *testing.T) {
	report := SupportedProtocol()

	if report.Version != string(officiala2a.Version) {
		t.Fatalf("protocol version = %q, want official SDK version %q", report.Version, officiala2a.Version)
	}
	if report.Version != "1.0" {
		t.Fatalf("protocol version = %q, want A2A 1.0", report.Version)
	}
	if report.SDKModule != officialSDKModulePath {
		t.Fatalf("SDK module = %q, want %q", report.SDKModule, officialSDKModulePath)
	}
	if report.SDKVersion != "v2.4.0" {
		t.Fatalf("SDK version = %q, want pinned version v2.4.0", report.SDKVersion)
	}
}
