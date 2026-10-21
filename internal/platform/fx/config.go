// internal/fx/config.go
package fxapp

import (
	"go.uber.org/fx"

	"pet-messenger/internal/platform/config"
)

var ConfigModule = fx.Module("config",
	fx.Provide(config.Load),
)
