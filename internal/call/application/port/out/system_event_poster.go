package out

import (
	"context"

	"github.com/google/uuid"

	conversation "pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/message/domain"
)

// SystemEventPoster posts a system timeline message (call started/ended)
// into a conversation. Re-declared here (rather than imported from
// internal/conversation or internal/message) to avoid an
// application-layer-to-application-layer import across contexts. Its
// signature matches
// *internal/message/application/command.PostSystemEventHandler.PostSystemEvent.
type SystemEventPoster interface {
	PostSystemEvent(ctx context.Context, conv *conversation.Conversation, actorID *uuid.UUID, event domainmsg.SystemEvent, targetID uuid.UUID, detail string) error
}
