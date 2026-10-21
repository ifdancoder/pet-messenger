package fxapp

import (
	"go.uber.org/fx"

	presenceinfra "pet-messenger/internal/platform/infrastructure/presence"
	redisinfra "pet-messenger/internal/platform/infrastructure/redis"
)

var RedisModule = fx.Module("redis",
	fx.Provide(
		redisinfra.NewClient,
		presenceinfra.NewRedisPresence,
	),
)
