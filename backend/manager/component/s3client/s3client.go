// Package s3client builds and caches the S3 client used by file upload/download.
// It reads the connection details from the persisted S3 setting and rebuilds
// the client whenever the config changes (e.g. after an admin update).
package s3client

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/pkg/errors"

	models "github.com/tbdavid2019/888a2a/backend/generated-go/store"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

// ErrS3NotConfigured is retained for callers that explicitly request the raw
// S3 client. File services use GetObjectStore and fall back to local storage.
var ErrS3NotConfigured = errors.New("s3 not configured")

// Client is a lazily-built, cached S3 client keyed by the S3 config fingerprint.
type Client struct {
	store configStore

	mu          sync.Mutex
	client      *s3.Client
	objectStore ObjectStore
	cfg         *models.S3ConfigSetting
	fingerprint string
}

type configStore interface {
	GetS3ConfigSetting(context.Context) (*models.S3ConfigSetting, error)
}

// New returns an S3 client component backed by the given store.
func New(stores *store.Store) *Client {
	return &Client{store: stores}
}

// Get returns the cached S3 client and its config, building (or rebuilding) it
// from the persisted setting when the config has changed. Returns
// ErrS3NotConfigured when endpoint and bucket are both empty.
func (c *Client) Get(ctx context.Context) (*s3.Client, *models.S3ConfigSetting, error) {
	cfg, err := c.store.GetS3ConfigSetting(ctx)
	if err != nil {
		return nil, nil, err
	}
	if cfg.Endpoint == "" && cfg.Bucket == "" {
		return nil, nil, ErrS3NotConfigured
	}

	fp := fingerprint(cfg)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil && fp == c.fingerprint {
		return c.client, c.cfg, nil
	}

	cli, err := build(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	c.client = cli
	c.cfg = cfg
	c.fingerprint = fp
	return cli, cfg, nil
}

// Invalidate forces the next Get to rebuild the client. Called after the admin
// updates the S3 config.
func (c *Client) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.client = nil
	c.objectStore = nil
	c.cfg = nil
	c.fingerprint = ""
}

// GetObjectStore returns the configured object backend. An entirely empty S3
// setting intentionally falls back to local filesystem storage. A partially
// configured S3 setting is rejected so a typo cannot silently write locally.
func (c *Client) GetObjectStore(ctx context.Context) (ObjectStore, error) {
	cfg, err := c.store.GetS3ConfigSetting(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = &models.S3ConfigSetting{}
	}
	if cfg.Endpoint == "" && cfg.Bucket == "" {
		root := os.Getenv("A2A888_OBJECT_STORAGE_DIR")
		if root == "" {
			root = filepath.Join("data", "objects")
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		if local, ok := c.objectStore.(*LocalObjectStore); ok && local.root == root {
			return local, nil
		}
		local := NewLocalObjectStore(root)
		c.objectStore = local
		return local, nil
	}
	if cfg.Endpoint != "" && cfg.Bucket == "" {
		return nil, fmt.Errorf("incomplete S3 configuration: bucket is required when endpoint is set")
	}
	cli, _, err := c.Get(ctx)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if remote, ok := c.objectStore.(*s3ObjectStore); ok && remote.client == cli && remote.bucket == cfg.Bucket {
		return remote, nil
	}
	remote := &s3ObjectStore{client: cli, bucket: cfg.Bucket}
	c.objectStore = remote
	return remote, nil
}

func build(ctx context.Context, cfg *models.S3ConfigSetting) (*s3.Client, error) {
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}
	awsCfg, err := awscfg.LoadDefaultConfig(ctx,
		awscfg.WithRegion(region),
		awscfg.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")),
		awscfg.WithHTTPClient(s3HTTPClient()),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load aws config")
	}

	s3Opts := []func(*s3.Options){
		func(o *s3.Options) {
			// MinIO and many S3-compatible stores require path-style addressing.
			if cfg.ForcePathStyle {
				o.UsePathStyle = true
			}
		},
	}

	if cfg.Endpoint != "" {
		endpoint := cfg.Endpoint
		// Allow callers to omit the scheme; use_ssl decides http vs https.
		if !strings.Contains(endpoint, "://") {
			scheme := "https"
			if !cfg.UseSsl {
				scheme = "http"
			}
			endpoint = scheme + "://" + endpoint
		}
		endpointURL, err := url.Parse(endpoint)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid s3 endpoint %q", cfg.Endpoint)
		}
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpointURL.String())
			if cfg.ForcePathStyle {
				o.UsePathStyle = true
			}
		})
	}

	// The aws SDK inherits the per-request context from each call; the custom
	// http client below bounds connection and header-read time so a stuck S3
	// endpoint cannot pin a goroutine indefinitely.
	return s3.NewFromConfig(awsCfg, s3Opts...), nil
}

// s3HTTPClient returns an *http.Client with bounded dial and header-read
// timeouts. Without it the SDK uses its default transport, which has no
// response-header deadline: a server that accepts the connection but never
// sends headers blocks the upload/download RPC until the request context
// (often the whole gRPC stream) is cancelled.
func s3HTTPClient() *http.Client {
	return &http.Client{
		Timeout: 0, // overall timeout is governed by the per-call context.
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ResponseHeaderTimeout: 30 * time.Second,
			IdleConnTimeout:       90 * time.Second,
		},
	}
}

func fingerprint(cfg *models.S3ConfigSetting) string {
	return strings.Join([]string{cfg.Endpoint, cfg.Region, cfg.Bucket, cfg.AccessKey, cfg.SecretKey, boolStr(cfg.ForcePathStyle), boolStr(cfg.UseSsl)}, "|")
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// TenantObjectKey prefixes an S3 object key with the organization ID ensuring tenant-isolated storage buckets.
func TenantObjectKey(orgID string, rawKey string) string {
	if orgID == "" {
		orgID = "default"
	}
	orgID = safeObjectKeySegment(orgID)
	rawKey = strings.TrimPrefix(rawKey, "/")
	if strings.HasPrefix(rawKey, orgID+"/") {
		rawKey = strings.TrimPrefix(rawKey, orgID+"/")
	}
	parts := strings.Split(rawKey, "/")
	for i, part := range parts {
		if part == "." || part == ".." {
			parts[i] = "_"
		}
	}
	return orgID + "/" + strings.Join(parts, "/")
}

func safeObjectKeySegment(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 || b.String() == "." || b.String() == ".." {
		return "default"
	}
	return b.String()
}
