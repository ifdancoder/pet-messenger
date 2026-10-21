package fxapp

import (
	"go.uber.org/fx"

	objectstoreinfra "pet-messenger/internal/platform/infrastructure/objectstore"
)

var ObjectStoreModule = fx.Module("objectstore",
	fx.Provide(objectstoreinfra.NewClient),
)
