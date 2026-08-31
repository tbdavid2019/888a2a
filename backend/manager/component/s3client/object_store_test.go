package s3client

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	models "github.com/tbdavid2019/888a2a/backend/generated-go/store"
)

func TestLocalObjectStorePutGetDelete(t *testing.T) {
	root := t.TempDir()
	objects := NewLocalObjectStore(root)

	require.NoError(t, objects.Put(context.Background(), "tenant-a/files/hello.txt", strings.NewReader("hello"), "text/plain"))
	got, err := objects.Get(context.Background(), "tenant-a/files/hello.txt")
	require.NoError(t, err)
	data, err := io.ReadAll(got)
	require.NoError(t, err)
	require.NoError(t, got.Close())
	require.Equal(t, "hello", string(data))

	require.NoError(t, objects.Delete(context.Background(), "tenant-a/files/hello.txt"))
	require.NoError(t, objects.Delete(context.Background(), "tenant-a/files/hello.txt"))
	_, err = objects.Get(context.Background(), "tenant-a/files/hello.txt")
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestLocalObjectStoreRejectsUnsafeKeys(t *testing.T) {
	objects := NewLocalObjectStore(t.TempDir())
	for _, key := range []string{"/absolute", "../escape", "tenant/../../escape", "tenant/..", "tenant\x00file"} {
		err := objects.Put(context.Background(), key, strings.NewReader("x"), "text/plain")
		require.Error(t, err, key)
	}
}

func TestLocalObjectStoreRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "tenant")))
	objects := NewLocalObjectStore(root)
	require.Error(t, objects.Put(context.Background(), "tenant/escape.txt", strings.NewReader("x"), "text/plain"))
	_, err := objects.Get(context.Background(), "tenant/escape.txt")
	require.Error(t, err)
	require.Error(t, objects.Delete(context.Background(), "tenant/escape.txt"))
}

func TestLocalObjectStoreSupportsConcurrentTenantWrites(t *testing.T) {
	objects := NewLocalObjectStore(t.TempDir())
	var wait sync.WaitGroup
	errs := make(chan error, 16)
	for index := range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			key := fmt.Sprintf("tenant-%d/files/value.txt", index)
			if err := objects.Put(context.Background(), key, strings.NewReader(key), "text/plain"); err != nil {
				errs <- err
				return
			}
			reader, err := objects.Get(context.Background(), key)
			if err != nil {
				errs <- err
				return
			}
			defer reader.Close()
			value, err := io.ReadAll(reader)
			if err != nil {
				errs <- err
				return
			}
			if key != string(value) {
				errs <- fmt.Errorf("object value = %q, want %q", value, key)
			}
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}

func TestClientGetObjectStoreFallsBackToLocal(t *testing.T) {
	root := t.TempDir()
	t.Setenv("A2A888_OBJECT_STORAGE_DIR", root)
	client := &Client{store: &fakeS3ConfigStore{cfg: &models.S3ConfigSetting{}}}
	objects, err := client.GetObjectStore(context.Background())
	require.NoError(t, err)
	require.IsType(t, &LocalObjectStore{}, objects)
}

func TestClientGetObjectStoreRejectsPartialS3Config(t *testing.T) {
	client := &Client{store: &fakeS3ConfigStore{cfg: &models.S3ConfigSetting{Endpoint: "https://s3.example"}}}
	_, err := client.GetObjectStore(context.Background())
	require.Error(t, err)
}

func TestClientGetObjectStoreAllowsAWSBucketWithoutCustomEndpoint(t *testing.T) {
	client := &Client{store: &fakeS3ConfigStore{cfg: &models.S3ConfigSetting{Bucket: "bucket"}}}
	objects, err := client.GetObjectStore(context.Background())
	require.NoError(t, err)
	require.IsType(t, &s3ObjectStore{}, objects)
}

type fakeS3ConfigStore struct {
	cfg *models.S3ConfigSetting
}

func (f *fakeS3ConfigStore) GetS3ConfigSetting(context.Context) (*models.S3ConfigSetting, error) {
	return f.cfg, nil
}
