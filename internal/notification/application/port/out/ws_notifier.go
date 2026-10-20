// internal/notification/application/port/out/ws_notifier.go
package out

import (
	"github.com/google/uuid"
)

// WSNotifier delivers a notification payload to a user's live WebSocket
// connections across the pod fleet. Implemented by
// internal/notification/adapter/out/messaging.Producer.
type WSNotifier interface {
	Notify(userID uuid.UUID, payload []byte) error
}
