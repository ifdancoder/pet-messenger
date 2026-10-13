package command

import (
	"context"
	"fmt"

	"pet-messenger/internal/user/application/port/in"
	"pet-messenger/internal/user/application/port/out"
	"pet-messenger/internal/user/domain"
)

type uploadAvatarHandler struct {
	users   out.UserStorePort
	avatars out.AvatarStorePort
}

// NewUploadAvatarHandler wires the UploadAvatar use case.
func NewUploadAvatarHandler(users out.UserStorePort, avatars out.AvatarStorePort) in.UploadAvatarUseCase {
	return &uploadAvatarHandler{users: users, avatars: avatars}
}

// Handle stores cmd's image under cmd.UserID, then points the user's
// AvatarURL at the serving route for their own username.
func (h *uploadAvatarHandler) Handle(ctx context.Context, cmd in.UploadAvatarCommand) (*domain.User, error) {
	u, err := h.users.GetByID(ctx, cmd.UserID)
	if err != nil {
		return nil, err // domain.ErrNotFound
	}
	if err := h.avatars.Put(ctx, cmd.UserID, cmd.ContentType, cmd.Data, cmd.Size); err != nil {
		return nil, fmt.Errorf("store avatar: %w", err)
	}
	u.AvatarURL = "/users/" + u.Username + "/avatar"
	if err := h.users.Update(ctx, u); err != nil {
		return nil, fmt.Errorf("update avatar url: %w", err)
	}
	return u, nil
}
