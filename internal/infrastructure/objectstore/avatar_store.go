package objectstore

import (
	"context"
	"errors"
	"io"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"

	"pet-messenger/internal/config"
)

var ErrAvatarNotFound = errors.New("objectstore: avatar not found")

// AvatarStore is the port UserHandler depends on — an interface, not
// *minio.Client directly, so handler tests use a fake instead of a real
// MinIO.
type AvatarStore interface {
	Put(ctx context.Context, userID uuid.UUID, contentType string, data io.Reader, size int64) error
	Get(ctx context.Context, userID uuid.UUID) (data io.ReadCloser, contentType string, err error)
}

type MinioAvatarStore struct {
	client *minio.Client
	bucket string
}

func NewMinioAvatarStore(client *minio.Client, cfg *config.Config) AvatarStore {
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
			return nil, "", ErrAvatarNotFound
		}
		_ = obj.Close()
		return nil, "", err
	}
	return obj, stat.ContentType, nil
}
