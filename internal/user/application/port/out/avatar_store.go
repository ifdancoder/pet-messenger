package out

import (
	"context"
	"errors"
	"io"

	"github.com/google/uuid"
)

// ErrAvatarNotFound is returned by AvatarStorePort.Get when no avatar
// object exists for the user, and by ServeAvatarUseCase when the user's
// AvatarURL is empty (never uploaded) — the two cases a caller cannot tell
// apart from the outside, and should not need to.
var ErrAvatarNotFound = errors.New("user: avatar not found")

// AvatarStorePort is this context's own driven port for avatar binary
// storage. Implemented by
// internal/infrastructure/objectstore.MinioAvatarStore.
type AvatarStorePort interface {
	Put(ctx context.Context, userID uuid.UUID, contentType string, data io.Reader, size int64) error
	Get(ctx context.Context, userID uuid.UUID) (data io.ReadCloser, contentType string, err error)
}
