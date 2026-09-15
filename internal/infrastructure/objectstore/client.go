// Package objectstore wraps the MinIO (S3-compatible) client construction
// with the project's fx lifecycle, mirroring internal/infrastructure/redis:
// a readiness check on construction (fail app start rather than fail on
// first use), no OnStop close — minio-go holds only a pooled http.Client,
// nothing that needs explicit shutdown.
package objectstore

import (
	"context"
	"fmt"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/fx"

	"pet-messenger/internal/config"
)

// NewClient dials cfg.MinIO and ensures the configured bucket exists.
func NewClient(lc fx.Lifecycle, cfg *config.Config) (*minio.Client, error) {
	client, err := minio.New(cfg.MinIO.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinIO.AccessKey, cfg.MinIO.SecretKey, ""),
		Secure: cfg.MinIO.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, cfg.MinIO.Bucket)
	if err != nil {
		return nil, fmt.Errorf("minio bucket exists %s: %w", cfg.MinIO.Bucket, err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, cfg.MinIO.Bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("minio make bucket %s: %w", cfg.MinIO.Bucket, err)
		}
	}
	lc.Append(fx.Hook{})
	return client, nil
}
