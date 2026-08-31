package s3client

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// ObjectStore is the narrow blob storage contract used by file services.
type ObjectStore interface {
	Put(context.Context, string, io.Reader, string) error
	Get(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}

// LocalObjectStore stores objects below root. Object keys are relative POSIX
// paths and are never allowed to escape root, including through symlinks.
type LocalObjectStore struct {
	root string
}

func NewLocalObjectStore(root string) *LocalObjectStore { return &LocalObjectStore{root: root} }

func (s *LocalObjectStore) Put(ctx context.Context, key string, src io.Reader, _ string) error {
	path, err := s.objectPath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create object directory: %w", err)
	}
	if err := s.ensureContained(filepath.Dir(path)); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("object path is a symlink")
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect object path: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".object-*")
	if err != nil {
		return fmt.Errorf("create object temporary file: %w", err)
	}
	tmpName := tmp.Name()
	removeTemp := true
	defer func() {
		_ = tmp.Close()
		if removeTemp {
			_ = os.Remove(tmpName)
		}
	}()
	if err := copyWithContext(ctx, tmp, src); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync object: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close object temporary file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("publish object: %w", err)
	}
	removeTemp = false
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func (s *LocalObjectStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	path, err := s.objectPath(key)
	if err != nil {
		return nil, err
	}
	if err := s.ensureContained(path); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("object path is not a regular file")
	}
	return os.Open(path)
}

func (s *LocalObjectStore) Delete(_ context.Context, key string) error {
	path, err := s.objectPath(key)
	if err != nil {
		return err
	}
	if err := s.ensureContained(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("object path is not a regular file")
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete object: %w", err)
	}
	return syncDirectory(filepath.Dir(path))
}

func (s *LocalObjectStore) objectPath(key string) (string, error) {
	if key == "" || strings.ContainsRune(key, '\x00') || filepath.IsAbs(key) {
		return "", fmt.Errorf("invalid object key")
	}
	clean := filepath.Clean(filepath.FromSlash(key))
	if clean == "." || clean != filepath.FromSlash(key) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid object key")
	}
	root, err := filepath.Abs(s.root)
	if err != nil {
		return "", fmt.Errorf("resolve object root: %w", err)
	}
	return filepath.Join(root, clean), nil
}

func (s *LocalObjectStore) ensureContained(path string) error {
	root, err := filepath.Abs(s.root)
	if err != nil {
		return fmt.Errorf("resolve object root: %w", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("resolve object root symlinks: %w", err)
	}
	if err == nil {
		root = resolvedRoot
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("resolve object path symlinks: %w", err)
		}
		resolvedParent, parentErr := filepath.EvalSymlinks(filepath.Dir(path))
		if parentErr != nil {
			return fmt.Errorf("resolve object parent symlinks: %w", parentErr)
		}
		resolvedPath = filepath.Join(resolvedParent, filepath.Base(path))
	}
	if resolvedPath != root && !strings.HasPrefix(resolvedPath, root+string(filepath.Separator)) {
		return fmt.Errorf("object path escapes storage root")
	}
	return nil
}

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) error {
	buf := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, err := dst.Write(buf[:n]); err != nil {
				return fmt.Errorf("write object: %w", err)
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("read object: %w", readErr)
		}
	}
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open object directory: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync object directory: %w", err)
	}
	return nil
}

type s3ObjectStore struct {
	client *s3.Client
	bucket string
}

func (s *s3ObjectStore) Put(ctx context.Context, key string, body io.Reader, contentType string) error {
	input := &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key), Body: body}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}
	_, err := s.client.PutObject(ctx, input)
	return err
}

func (s *s3ObjectStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

func (s *s3ObjectStore) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	return err
}
