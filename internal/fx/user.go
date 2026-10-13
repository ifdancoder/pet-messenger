// internal/fx/user.go
package fxapp

import (
	"go.uber.org/fx"

	convout "pet-messenger/internal/conversation/application/port/out"
	usecaseauth "pet-messenger/internal/usecase/auth"
	"pet-messenger/internal/user/adapter/in/web"
	"pet-messenger/internal/user/adapter/out/persistence"
	"pet-messenger/internal/user/application/command"
	"pet-messenger/internal/user/application/port/out"
	"pet-messenger/internal/user/application/query"
)

// UserModule provides the user context's own pieces. Bridge functions
// satisfying OTHER contexts' consumer-owned ports against
// *persistence.UserRepository are added here too, as they are introduced
// (Tasks 9-11) — this file, not the consumer's own fx file, is where the
// project's established convention (see internal/fx/conversation.go) puts
// them: the producer's fx module hosts every bridge to its own type.
var UserModule = fx.Module("user",
	fx.Provide(
		persistence.NewUserRepository,

		func(r *persistence.UserRepository) out.UserStorePort { return r },
		func(r *persistence.UserRepository) usecaseauth.UserStore { return r },
		func(r *persistence.UserRepository) convout.UserChecker { return r },

		command.NewUpdateProfileHandler,
		command.NewUploadAvatarHandler,
		query.NewGetByUsernameHandler,
		query.NewServeAvatarHandler,

		web.NewUserHandler,
	),
)
