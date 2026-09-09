package fxapp

import (
	"go.uber.org/fx"

	presenceinfra "pet-messenger/internal/infrastructure/presence"
	redisinfra "pet-messenger/internal/infrastructure/redis"
)

var RedisModule = fx.Module("redis",
	fx.Provide(
		redisinfra.NewClient,
		presenceinfra.NewRedisPresence,
	),
)
