// Package command holds the CQRS write side of the conversation context:
// one handler per command, each implementing the matching port/in use case
// and depending only on the port/out interfaces it actually needs.
package command

import (
	"context"
	"log"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/application/port/out"
	"pet-messenger/internal/conversation/domain"
)

// Every publish*/logPostSystemEventError helper is best-effort: the write
// it describes has already committed, so a failure is logged and never
// turned into an error the caller has to handle.

func publishConversationCreated(ctx context.Context, events out.EventPublisherPort, c *domain.Conversation, actorID uuid.UUID) {
	if err := events.Publish(ctx, domain.ConversationCreated{ConversationID: c.ID, Kind: c.Kind, ActorID: actorID}); err != nil {
		log.Printf("conversation: publish ConversationCreated %s: %v", c.ID, err)
	}
}

// publishMemberAdded publishes a MemberAdded event carrying c's own fields
// so the inbox projector never needs a second read. peer is the other
// participant's id for a direct conversation, nil otherwise.
func publishMemberAdded(ctx context.Context, events out.EventPublisherPort, c *domain.Conversation, userID uuid.UUID, peer *uuid.UUID) {
	ev := domain.MemberAdded{
		ConversationID: c.ID, UserID: userID, Kind: c.Kind, Title: c.Title,
		AvatarURL: c.AvatarURL, Public: c.Public, ConversationCreatedAt: c.CreatedAt,
		DirectPeerUserID: peer,
	}
	if err := events.Publish(ctx, ev); err != nil {
		log.Printf("conversation: publish MemberAdded %s/%s: %v", c.ID, userID, err)
	}
}

func publishMemberRemoved(ctx context.Context, events out.EventPublisherPort, conversationID, userID uuid.UUID) {
	if err := events.Publish(ctx, domain.MemberRemoved{ConversationID: conversationID, UserID: userID}); err != nil {
		log.Printf("conversation: publish MemberRemoved %s/%s: %v", conversationID, userID, err)
	}
}

func publishMemberTierChanged(ctx context.Context, events out.EventPublisherPort, conversationID, userID uuid.UUID, tier domain.AdminTier) {
	if err := events.Publish(ctx, domain.MemberTierChanged{ConversationID: conversationID, UserID: userID, NewTier: tier}); err != nil {
		log.Printf("conversation: publish MemberTierChanged %s/%s: %v", conversationID, userID, err)
	}
}

// logPostSystemEventError logs a failed SystemEventPoster.PostSystemEvent
// call. The mutation has already committed, so the caller still returns
// success.
func logPostSystemEventError(conversationID uuid.UUID, err error) {
	log.Printf("conversation: post system event on %s: %v", conversationID, err)
}

// logPublishError logs a failed EventPublisherPort.Publish call for a
// handler that branches on which event to publish rather than calling one
// of the publish* helpers above directly.
func logPublishError(conversationID uuid.UUID, err error) {
	log.Printf("conversation: publish event on %s: %v", conversationID, err)
}
