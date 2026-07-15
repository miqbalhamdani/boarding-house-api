package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinioStore is a private-bucket Store backed by an S3-compatible object store
// (MinIO in local dev). Buckets are created private and never given a public
// read policy; access is always via short-lived presigned URLs.
type MinioStore struct {
	client *minio.Client
	bucket string
}

// NewMinioStore constructs a MinioStore. It does not create the bucket; call
// EnsureBucket once at startup.
func NewMinioStore(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*MinioStore, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("init object storage client: %w", err)
	}
	return &MinioStore{client: client, bucket: bucket}, nil
}

// EnsureBucket creates the private bucket if it does not already exist. It is
// fail-fast on startup, mirroring the database pool's startup behaviour.
func (s *MinioStore) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check object storage bucket: %w", err)
	}
	if exists {
		return nil
	}
	if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("create object storage bucket: %w", err)
	}
	// Intentionally no public bucket policy: the bucket stays private.
	return nil
}

func (s *MinioStore) Put(ctx context.Context, key string, content io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, content, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("upload object: %w", err)
	}
	return nil
}

func (s *MinioStore) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := s.client.PresignedGetObject(ctx, s.bucket, key, ttl, nil)
	if err != nil {
		return "", fmt.Errorf("presign object url: %w", err)
	}
	return u.String(), nil
}

func (s *MinioStore) Remove(ctx context.Context, key string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("remove object: %w", err)
	}
	return nil
}
