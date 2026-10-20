package query

import (
	"context"
	"io"

	"pet-messenger/internal/user/application/port/in"
	"pet-messenger/internal/user/application/port/out"
)

type serveAvatarHandler struct {
	users   out.UserStorePort
	avatars out.AvatarStorePort
}

// NewServeAvatarHandler wires the ServeAvatar use case.
func NewServeAvatarHandler(users out.UserStorePort, avatars out.AvatarStorePort) in.ServeAvatarUseCase {
	return &serveAvatarHandler{users: users, avatars: avatars}
}

// Handle resolves q.Username to a user, then streams their stored avatar.
// An empty AvatarURL (never uploaded) is reported the same as a missing
// object, via the shared out.ErrAvatarNotFound, matching today's handler,
// which does not distinguish the two cases either.
func (h *serveAvatarHandler) Handle(ctx context.Context, q in.ServeAvatarQuery) (io.ReadCloser, string, error) {
	u, err := h.users.GetByUsername(ctx, q.Username)
	if err != nil {
		return nil, "", err // domain.ErrNotFound
	}
	if u.AvatarURL == "" {
		return nil, "", out.ErrAvatarNotFound
	}
	return h.avatars.Get(ctx, u.ID)
}
