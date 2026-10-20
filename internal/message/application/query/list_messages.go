// internal/message/application/query/list_messages.go

// Package query holds the CQRS read side of the message context.
package query

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/message/application/port/in"
	"pet-messenger/internal/message/application/port/out"
	domainmsg "pet-messenger/internal/message/domain"
)

// Page-size bounds for ListMessages.
const (
	DefaultPageLimit = 50
	MaxPageLimit     = 200
)

// ErrConflictingCursors: ListMessages was given both AfterSeq and BeforeSeq.
var ErrConflictingCursors = errors.New("message: specify at most one of after_seq / before_seq")

// PageLimit clamps a client-requested page size: non-positive means "use the
// default"; anything above the max is capped.
func PageLimit(requested int) int {
	switch {
	case requested <= 0:
		return DefaultPageLimit
	case requested > MaxPageLimit:
		return MaxPageLimit
	default:
		return requested
	}
}

type listMessagesHandler struct {
	loader        out.MessageLoader
	history       out.MessageHistoryPort
	conversations out.ConversationMembershipPort
	roster        out.ConversationRosterPort
}

// NewListMessagesHandler wires the ListMessages use case.
func NewListMessagesHandler(loader out.MessageLoader, history out.MessageHistoryPort, conversations out.ConversationMembershipPort, roster out.ConversationRosterPort) in.ListMessagesUseCase {
	return &listMessagesHandler{loader: loader, history: history, conversations: conversations, roster: roster}
}

// Handle returns a page of q.ConversationID's history for a member. At most
// one of q.AfterSeq / q.BeforeSeq may be set: AfterSeq walks forward
// (ascending, the reconnect catch-up read), BeforeSeq walks backward
// (descending, history scroll). Both nil means "newest page" (descending
// from the end). q.Limit is clamped via PageLimit.
func (h *listMessagesHandler) Handle(ctx context.Context, q in.ListMessagesQuery) ([]*domainmsg.Message, error) {
	if _, err := h.conversations.GetMember(ctx, q.ConversationID, q.Actor); err != nil {
		return nil, err // domainconv.ErrNotMember
	}
	if q.AfterSeq != nil && q.BeforeSeq != nil {
		return nil, ErrConflictingCursors
	}
	n := PageLimit(q.Limit)
	var messages []*domainmsg.Message
	var err error
	if q.AfterSeq != nil {
		messages, err = h.history.ListByConversation(ctx, q.ConversationID, *q.AfterSeq, n)
	} else {
		before := int64(0)
		if q.BeforeSeq != nil {
			before = *q.BeforeSeq
		}
		messages, err = h.history.ListBeforeSeq(ctx, q.ConversationID, before, n)
	}
	if err != nil {
		return nil, err
	}
	h.annotateReadStatus(ctx, q.ConversationID, messages)
	return messages, nil
}

// annotateReadStatus sets ReadBy/RecipientCount on each message by comparing
// every other member's last-read Seq against the message's own Seq. It
// never fails the request: a lookup error here is logged and the page is
// returned with read-status left at its zero value.
func (h *listMessagesHandler) annotateReadStatus(ctx context.Context, convID uuid.UUID, messages []*domainmsg.Message) {
	members, err := h.roster.ListMembers(ctx, convID, false, time.Time{}, uuid.Nil, 200)
	if err != nil {
		log.Printf("message: list members for read status %s: %v", convID, err)
		return
	}
	readSeq := make(map[uuid.UUID]int64, len(members))
	for _, member := range members {
		if member.LastReadMessageID == nil {
			continue
		}
		read, err := h.loader.GetByID(ctx, *member.LastReadMessageID)
		if err == nil && read.ConversationID == convID {
			readSeq[member.UserID] = read.Seq
		}
	}
	for _, message := range messages {
		if message.SenderID == nil {
			continue
		}
		for _, member := range members {
			if member.UserID == *message.SenderID {
				continue
			}
			message.RecipientCount++
			if readSeq[member.UserID] >= message.Seq {
				message.ReadBy++
			}
		}
	}
}
