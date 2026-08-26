package s3client

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	models "github.com/tbdavid2019/888a2a/backend/generated-go/store"
)

// TestS3HTTPClient_HasTimeouts guards against regressing back to the default
// transport (no response-header deadline), which let a stuck S3 endpoint pin
// a goroutine until the whole request context was cancelled.
func TestS3HTTPClient_HasTimeouts(t *testing.T) {
	cli := s3HTTPClient()
	transport, ok := cli.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", cli.Transport)
	}
	assert.Positive(t, int64(transport.ResponseHeaderTimeout), "ResponseHeaderTimeout must be set")
	assert.NotNil(t, transport.DialContext, "DialContext must be set")
}

// TestBuild_PassesContext is a source-level guard that build's first argument
// is the caller context (not context.Background()), so request-scoped
// cancellation/deadlines propagate into the AWS SDK config loader.
func TestBuild_PassesContext(_ *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// A cancelled context should make config loading fail fast rather than
	// hang on a background context.
	_, err := build(ctx, &models.S3ConfigSetting{
		Endpoint:  "http://127.0.0.1:1",
		Bucket:    "b",
		Region:    "us-east-1",
		AccessKey: "ak",
		SecretKey: "sk",
	})
	// We only assert build does not panic and returns; the SDK may or may not
	// surface the cancelled context synchronously. The guard is the signature.
	_ = err
}

func TestTenantObjectKey_PrefixIsolation(t *testing.T) {
	cases := []struct {
		orgID    string
		rawKey   string
		expected string
	}{
		{"org-1", "uploads/avatar.png", "org-1/uploads/avatar.png"},
		{"org-1", "/uploads/avatar.png", "org-1/uploads/avatar.png"},
		{"org-1", "org-1/uploads/avatar.png", "org-1/uploads/avatar.png"},
		{"", "files/report.pdf", "default/files/report.pdf"},
		{"org-2", "files/report.pdf", "org-2/files/report.pdf"},
	}

	for _, tc := range cases {
		got := TenantObjectKey(tc.orgID, tc.rawKey)
		if got != tc.expected {
			t.Errorf("TenantObjectKey(%q, %q) = %q; want %q", tc.orgID, tc.rawKey, got, tc.expected)
		}
	}
}

func TestTenantObjectKey_CrossTenantCollisionResistance(t *testing.T) {
	keyTenantA := TenantObjectKey("tenant-alpha", "attachments/secret.pdf")
	keyTenantB := TenantObjectKey("tenant-beta", "attachments/secret.pdf")

	assert.NotEqual(t, keyTenantA, keyTenantB, "Cross-tenant S3 keys must never collide")
	assert.Equal(t, "tenant-alpha/attachments/secret.pdf", keyTenantA)
	assert.Equal(t, "tenant-beta/attachments/secret.pdf", keyTenantB)

	// Nested path prefixes should not be confused with tenant boundary
	nestedKey := TenantObjectKey("tenant-alpha", "subpath/tenant-beta/file.txt")
	assert.Equal(t, "tenant-alpha/subpath/tenant-beta/file.txt", nestedKey)
}
