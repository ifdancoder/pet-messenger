package out

import (
	"context"

	"pet-messenger/internal/conversation/domain"
)

// EventPublisherPort publishes a domain event for asynchronous consumers
// (the inbox projector, and any future subscriber) after the write that
// produced it has committed. Implemented by
// internal/conversation/adapter/out/messaging.EventPublisher.
type EventPublisherPort interface {
	Publish(ctx context.Context, event domain.Event) error
}
