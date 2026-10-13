package in

import (
	"context"
	"io"

	"github.com/google/uuid"

	"pet-messenger/internal/user/domain"
)

// UploadAvatarCommand carries a new avatar image to store for UserID.
type UploadAvatarCommand struct {
	UserID      uuid.UUID
	ContentType string
	Data        io.Reader
	Size        int64
}

// UploadAvatarUseCase stores a new avatar and updates the user's AvatarURL
// to the serving route for their own username.
type UploadAvatarUseCase interface {
	Handle(ctx context.Context, cmd UploadAvatarCommand) (*domain.User, error)
}
