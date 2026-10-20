// internal/notification/application/port/in/process_notification_intent.go
package in

import "context"

// ProcessNotificationIntentCommand mirrors the decoded wire shape of a
// published NotifIntent (see adapter/out/messaging.NotifIntent): this
// context's primary messaging adapter decodes the wire JSON and hands it to
// this command as a plain Go struct, the same wire-DTO/command separation
// every HTTP-driven command in this retrofit already keeps.
type ProcessNotificationIntentCommand struct {
	Kind           string
	ConversationID string
	MessageID      *string
	MessageSeq     *int64
	SenderID       *string
	Preview        string
	RecipientID    *string
	NewTier        string
	CallID         string
	DedupPrefix    string
}

// ProcessNotificationIntentUseCase resolves the intent's recipients,
// records a Notification row for each (idempotent via dedup), and delivers
// it: over WebSocket if the recipient is online, falling back to device
// push otherwise. An unrecognized Kind is silently skipped (not an error):
// JetStream redelivery would never make it recognized, so treating it as a
// retryable failure would retry forever.
type ProcessNotificationIntentUseCase interface {
	Handle(ctx context.Context, cmd ProcessNotificationIntentCommand) error
}
