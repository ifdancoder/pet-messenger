package objectstore

import (
	"context"
	"io"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"

	"pet-messenger/internal/platform/config"
	userout "pet-messenger/internal/user/application/port/out"
)

// MinioAvatarStore implements userout.AvatarStorePort.
type MinioAvatarStore struct {
	client *minio.Client
	bucket string
}

// NewMinioAvatarStore constructs the port directly — the same convention
// internal/infrastructure/nats.NewPublisher already uses for
// usecasemsg.MessagePublisher — so no separate fx bridge function is
// needed.
func NewMinioAvatarStore(client *minio.Client, cfg *config.Config) userout.AvatarStorePort {
	return &MinioAvatarStore{client: client, bucket: cfg.MinIO.Bucket}
}

func (s *MinioAvatarStore) key(userID uuid.UUID) string { return userID.String() }

func (s *MinioAvatarStore) Put(ctx context.Context, userID uuid.UUID, contentType string, data io.Reader, size int64) error {
	_, err := s.client.PutObject(ctx, s.bucket, s.key(userID), data, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

func (s *MinioAvatarStore) Get(ctx context.Context, userID uuid.UUID) (io.ReadCloser, string, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, s.key(userID), minio.GetObjectOptions{})
	if err != nil {
		return nil, "", err
	}
	stat, err := obj.Stat()
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" {
			_ = obj.Close()
			return nil, "", userout.ErrAvatarNotFound
		}
		_ = obj.Close()
		return nil, "", err
	}
	return obj, stat.ContentType, nil
}
