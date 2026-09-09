// internal/fx/notification.go
package fxapp

import (
	"context"

	"go.uber.org/fx"

	httpadapter "pet-messenger/internal/adapter/http"
	"pet-messenger/internal/config"
	"pet-messenger/internal/infrastructure/postgres"
	"pet-messenger/internal/infrastructure/push"
	usecasenotif "pet-messenger/internal/usecase/notification"
)

var NotificationModule = fx.Module("notification",
	fx.Provide(
		postgres.NewNotificationRepository,
		usecasenotif.NewService,
		httpadapter.NewNotificationHandler,
	),
)

var PushModule = fx.Module("push",
	fx.Provide(
		postgres.NewPushTokenRepository,
		httpadapter.NewPushTokenHandler,
		provideMultiPusher,
	),
)

// provideMultiPusher builds the per-platform device pusher from config. A
// misconfigured provider (e.g. bad FCM credentials) is a hard boot failure —
// returning the error lets fx fail fast rather than start with a silently
// degraded push subsystem.
func provideMultiPusher(cfg *config.Config) (push.Pusher, error) {
	return push.NewMultiPusher(context.Background(), cfg.Push)
}
