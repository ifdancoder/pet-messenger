// internal/fx/user.go
package fxapp

import (
	"go.uber.org/fx"

	authout "pet-messenger/internal/auth/application/port/out"
	convout "pet-messenger/internal/conversation/application/port/out"
	objectstoreinfra "pet-messenger/internal/infrastructure/objectstore"
	"pet-messenger/internal/user/adapter/in/web"
	"pet-messenger/internal/user/adapter/out/persistence"
	"pet-messenger/internal/user/application/command"
	"pet-messenger/internal/user/application/port/out"
	"pet-messenger/internal/user/application/query"
)

// UserModule provides the user context's own pieces, plus every bridge
// from *persistence.UserRepository to another context's consumer-owned
// port (see internal/fx/conversation.go for the project's established
// convention of hosting such bridges in the producer's own fx module).
var UserModule = fx.Module("user",
	fx.Provide(
		persistence.NewUserRepository,
		objectstoreinfra.NewMinioAvatarStore,

		func(r *persistence.UserRepository) out.UserStorePort { return r },
		func(r *persistence.UserRepository) authout.UserStore { return r },
		func(r *persistence.UserRepository) convout.UserChecker { return r },
		func(r *persistence.UserRepository) convout.ProfileBatchViewPort { return r },

		command.NewUpdateProfileHandler,
		command.NewUploadAvatarHandler,
		query.NewGetByUsernameHandler,
		query.NewServeAvatarHandler,

		web.NewUserHandler,
	),
)
