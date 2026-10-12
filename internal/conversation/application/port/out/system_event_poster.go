package out

import (
	"context"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/domain/message"
)

// SystemEventPoster records a membership / lifecycle change as a system
// message in the conversation's timeline. It no-ops for direct
// conversations and (for member_* events) for channel_broadcast.
// Implemented by *internal/usecase/message.Service. Unchanged from the
// pre-retrofit usecase/conversation.SystemEventPoster — only the
// Conversation type's import path moved.
type SystemEventPoster interface {
	PostSystemEvent(ctx context.Context, conv *domain.Conversation, actorID *uuid.UUID, event domainmsg.SystemEvent, targetID uuid.UUID, detail string) error
}
