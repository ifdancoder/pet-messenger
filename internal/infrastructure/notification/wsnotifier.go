package notification

import (
	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"
)

// WSNotifier publishes a per-user notification payload on core NATS
// (notif.ws.<uid>); a per-pod subscription feeds it to Hub.SendToUser. It
// returns the concrete type so this package never imports adapter/notify
// (which imports this package for NotifIntent) — internal/fx adapts it to the
// notify.WSNotifier interface.
type WSNotifier struct{ nc *natsgo.Conn }

func NewWSNotifier(nc *natsgo.Conn) *WSNotifier { return &WSNotifier{nc: nc} }

func (w *WSNotifier) Notify(userID uuid.UUID, payload []byte) error {
	return w.nc.Publish("notif.ws."+userID.String(), payload)
}
