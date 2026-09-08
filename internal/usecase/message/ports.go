package message

import (
	"context"

	domainmsg "pet-messenger/internal/domain/message"
)

// MessagePublisher broadcasts a message change to connected clients across pods,
// after the write commits. Implemented by internal/infrastructure/nats.Publisher.
// Delivery is best-effort — the message is already persisted and a reconnecting
// client recovers via ListMessages — so the Service logs a publish error and
// still returns success.
//
// Consumers must order by the message's Seq; NATS/WS arrival order is not
// guaranteed (concurrent sends can interleave between the DB write and the
// publish).
type MessagePublisher interface {
	PublishCreated(ctx context.Context, m *domainmsg.Message) error
	PublishUpdated(ctx context.Context, m *domainmsg.Message) error
	PublishDeleted(ctx context.Context, m *domainmsg.Message) error
}
