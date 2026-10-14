// internal/message/application/command/send_message.go

// Package command holds the CQRS write side of the message context: one
// handler per use case, each implementing the matching port/in use case and
// depending only on the port/out interfaces it actually needs.
package command

import (
	"context"
	"errors"
	"fmt"
	"log"

	"pet-messenger/internal/message/application/port/in"
	"pet-messenger/internal/message/application/port/out"
	domainmsg "pet-messenger/internal/message/domain"
)

// Sentinel errors.
var (
	// ErrCannotPost: the caller is a member but their role may not post here
	// (e.g. a subscriber in a broadcast channel).
	ErrCannotPost = errors.New("message: role may not post to this conversation")
	// ErrReplyNotFound: the reply_to target does not exist or is in another
	// conversation.
	ErrReplyNotFound = errors.New("message: reply target does not exist in this conversation")
	// ErrConversationArchived: SendMessage was attempted against an archived
	// conversation.
	ErrConversationArchived = errors.New("message: conversation is archived")
)

type sendMessageHandler struct {
	loader        out.MessageLoader
	writer        out.MessageWriter
	conversations out.ConversationMembershipPort
	publisher     out.MessagePublisher
	notif         out.NotificationProducer
}

// NewSendMessageHandler wires the SendMessage use case.
func NewSendMessageHandler(loader out.MessageLoader, writer out.MessageWriter, conversations out.ConversationMembershipPort, publisher out.MessagePublisher, notif out.NotificationProducer) in.SendMessageUseCase {
	return &sendMessageHandler{loader: loader, writer: writer, conversations: conversations, publisher: publisher, notif: notif}
}

// Handle posts a text message from cmd.Actor to cmd.ConversationID.
// idempotencyKey is optional ("" = none): a retry with the same key returns
// the original message instead of creating a duplicate. ReplyToID, when
// set, must reference a message in the same conversation.
func (h *sendMessageHandler) Handle(ctx context.Context, cmd in.SendMessageCommand) (*domainmsg.Message, error) {
	conv, err := h.conversations.GetByID(ctx, cmd.ConversationID)
	if err != nil {
		return nil, err // domainconv.ErrNotFound
	}
	if conv.IsArchived() {
		return nil, ErrConversationArchived
	}
	member, err := h.conversations.GetMember(ctx, cmd.ConversationID, cmd.Actor)
	if err != nil {
		return nil, err // domainconv.ErrNotMember
	}
	if !conv.CanPost(member) {
		return nil, ErrCannotPost
	}
	if cmd.ReplyToID != nil {
		target, err := h.loader.GetByID(ctx, *cmd.ReplyToID)
		if err != nil {
			if errors.Is(err, domainmsg.ErrNotFound) {
				return nil, ErrReplyNotFound
			}
			return nil, fmt.Errorf("load reply target: %w", err)
		}
		if target.ConversationID != cmd.ConversationID {
			return nil, ErrReplyNotFound
		}
	}

	m, err := domainmsg.NewText(cmd.ConversationID, cmd.Actor, cmd.Content, cmd.ReplyToID)
	if err != nil {
		return nil, err // ErrEmptyContent / ErrContentTooLong / ...
	}
	if cmd.IdempotencyKey != "" {
		key := cmd.IdempotencyKey
		m.IdempotencyKey = &key
	}

	if err := h.writer.Create(ctx, m); err != nil {
		if errors.Is(err, domainmsg.ErrDuplicateIdempotencyKey) {
			existing, err := h.loader.GetByIdempotencyKey(ctx, cmd.ConversationID, cmd.Actor, cmd.IdempotencyKey)
			if err != nil {
				return nil, fmt.Errorf("reload after idempotency race: %v", err)
			}
			return existing, nil
		}
		return nil, fmt.Errorf("create message: %w", err)
	}
	if err := h.publisher.PublishCreated(ctx, m); err != nil {
		log.Printf("message: publish created %s: %v", m.ID, err)
	}
	if err := h.notif.MessageSent(ctx, m, conv); err != nil {
		log.Printf("message: notify message-sent %s: %v", m.ID, err)
	}
	return m, nil
}
