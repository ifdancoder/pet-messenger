package messaging

import "pet-messenger/internal/conversation/adapter/out/persistence"

// NewInboxProjectorFromRepository wires InboxProjector against the
// concrete persistence.InboxRepository. A thin fx-friendly wrapper around
// NewInboxProjector, needed only because that constructor's own parameter
// type (inboxWriter) is unexported by design.
func NewInboxProjectorFromRepository(r *persistence.InboxRepository) *InboxProjector {
	return NewInboxProjector(r)
}
