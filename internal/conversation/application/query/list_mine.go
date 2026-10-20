package query

import (
	"context"
	"fmt"

	"pet-messenger/internal/conversation/application"
	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/application/port/out"
)

type listMineHandler struct {
	inbox out.InboxViewPort
}

// NewListMineHandler wires the ListMine use case.
func NewListMineHandler(inbox out.InboxViewPort) in.ListMineUseCase {
	return &listMineHandler{inbox: inbox}
}

// Handle returns the caller's conversations newest-activity-first from the
// conversation_inbox projection. See out.InboxEntry's doc comment: the
// keyset sorts on last_message_at, a value that changes under the reader,
// so a conversation that receives new activity while the client is paging
// can jump above the cursor and be skipped by a later page, inherent to
// keyset pagination over an activity timestamp, mitigated client-side by
// re-fetching from the top on new activity rather than only paging down.
func (h *listMineHandler) Handle(ctx context.Context, q in.ListMineQuery) ([]*out.InboxEntry, error) {
	list, err := h.inbox.ListInbox(ctx, q.UserID, q.Before, q.BeforeID, application.EffectiveListLimit(q.Limit))
	if err != nil {
		return nil, fmt.Errorf("list inbox: %w", err)
	}
	return list, nil
}
