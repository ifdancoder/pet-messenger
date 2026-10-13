package command

import (
	"context"
	"errors"
	"fmt"

	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/application/port/out"
	"pet-messenger/internal/conversation/domain"
	domainuser "pet-messenger/internal/user/domain"
)

// Errors specific to OpenDirect.
var (
	ErrRecipientNotFound = errors.New("conversation: recipient user does not exist")
	ErrCannotDirectSelf  = errors.New("conversation: cannot open a direct conversation with yourself")
)

type openDirectHandler struct {
	conversations out.LoadConversationPort
	writer        out.UpdateConversationPort
	users         out.UserChecker
	events        out.EventPublisherPort
}

// NewOpenDirectHandler wires the OpenDirect use case.
func NewOpenDirectHandler(conversations out.LoadConversationPort, writer out.UpdateConversationPort, users out.UserChecker, events out.EventPublisherPort) in.OpenDirectUseCase {
	return &openDirectHandler{conversations: conversations, writer: writer, users: users, events: events}
}

// Handle returns the direct conversation between cmd.Me and cmd.Other,
// creating it if it does not exist yet. It is idempotent: concurrent
// first-opens converge on one row (the unique direct_key makes the loser's
// insert fail with domain.ErrDirectExists, resolved by re-reading).
func (h *openDirectHandler) Handle(ctx context.Context, cmd in.OpenDirectCommand) (*domain.Conversation, error) {
	if cmd.Me == cmd.Other {
		return nil, ErrCannotDirectSelf
	}
	if _, err := h.users.GetByID(ctx, cmd.Other); err != nil {
		if errors.Is(err, domainuser.ErrNotFound) {
			return nil, ErrRecipientNotFound
		}
		return nil, fmt.Errorf("load recipient: %w", err)
	}

	if existing, err := h.conversations.GetDirect(ctx, cmd.Me, cmd.Other); err == nil {
		return existing, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("lookup direct: %w", err)
	}

	c, members, err := domain.NewDirect(cmd.Me, cmd.Other)
	if err != nil {
		return nil, err // domain.ErrInvalidDirectMembers
	}
	if err := h.writer.Create(ctx, c, members); err != nil {
		if errors.Is(err, domain.ErrDirectExists) {
			raced, err := h.conversations.GetDirect(ctx, cmd.Me, cmd.Other)
			if err != nil {
				return nil, fmt.Errorf("reload direct after create race: %w", err)
			}
			return raced, nil
		}
		return nil, fmt.Errorf("create direct: %w", err)
	}

	publishConversationCreated(ctx, h.events, c, members[0].UserID)
	a, b := members[0].UserID, members[1].UserID
	publishMemberAdded(ctx, h.events, c, a, &b)
	publishMemberAdded(ctx, h.events, c, b, &a)
	return c, nil
}
