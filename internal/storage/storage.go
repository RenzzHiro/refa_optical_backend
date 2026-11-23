package storage

import (
	"context"
	"errors"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Store interface {
	Put(context.Context, string, io.Reader, int64, string) error
	Open(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}
type Local struct{ root string }

func NewLocal(root string) (*Local, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(abs, 0700); err != nil {
		return nil, err
	}
	return &Local{root: abs}, nil
}
func (s *Local) path(key string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(key))
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", errors.New("invalid storage key")
	}
	target := filepath.Join(s.root, clean)
	rel, err := filepath.Rel(s.root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", errors.New("invalid storage key")
	}
	return target, nil
}
func (s *Local) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	target, err := s.path(key)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}
func (s *Local) Open(_ context.Context, key string) (io.ReadCloser, error) {
	path, err := s.path(key)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}
func (s *Local) Delete(_ context.Context, key string) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

type S3 struct {
	client *minio.Client
	bucket string
}

func NewS3(endpoint, region, access, secret, bucket string, secure bool) (*S3, error) {
	if endpoint == "" || access == "" || secret == "" || bucket == "" {
		return nil, errors.New("S3 endpoint, credentials, and bucket are required")
	}
	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(access, secret, ""), Secure: secure, Region: region})
	if err != nil {
		return nil, err
	}
	exists, err := client.BucketExists(context.Background(), bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err = client.MakeBucket(context.Background(), bucket, minio.MakeBucketOptions{Region: region}); err != nil {
			return nil, err
		}
	}
	return &S3{client: client, bucket: bucket}, nil
}
func (s *S3) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}
func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	return s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
}
func (s *S3) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}
