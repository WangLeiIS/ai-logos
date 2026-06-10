package store

import (
	"context"
	"fmt"
	"io"
	"time"

	"irollhub/model"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOClient struct {
	client *minio.Client
	bucket string
}

func NewMinIOClient(cfg model.MinIOConfig) (*MinIOClient, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio init: %w", err)
	}
	return &MinIOClient{client: client, bucket: cfg.Bucket}, nil
}

func (m *MinIOClient) EnsureBucket(ctx context.Context) error {
	exists, err := m.client.BucketExists(ctx, m.bucket)
	if err != nil {
		return err
	}
	if !exists {
		return m.client.MakeBucket(ctx, m.bucket, minio.MakeBucketOptions{})
	}
	return nil
}

func (m *MinIOClient) Upload(ctx context.Context, objectKey string, reader io.Reader, size int64) error {
	_, err := m.client.PutObject(ctx, m.bucket, objectKey, reader, size, minio.PutObjectOptions{
		ContentType: "application/zip",
	})
	return err
}

func (m *MinIOClient) PresignedGetURL(ctx context.Context, objectKey string) (string, error) {
	url, err := m.client.PresignedGetObject(ctx, m.bucket, objectKey, 5*time.Minute, nil)
	if err != nil {
		return "", err
	}
	return url.String(), nil
}

func (m *MinIOClient) Delete(ctx context.Context, objectKey string) error {
	return m.client.RemoveObject(ctx, m.bucket, objectKey, minio.RemoveObjectOptions{})
}

// DeleteMultiple removes multiple objects from MinIO. If any deletion fails, it
// returns the first error encountered and stops.
func (m *MinIOClient) DeleteMultiple(ctx context.Context, keys []string) error {
	for _, k := range keys {
		if err := m.Delete(ctx, k); err != nil {
			return err
		}
	}
	return nil
}
