// internal/fx/auth.go
package fxapp

import (
	"go.uber.org/fx"

	httpadapter "pet-messenger/internal/adapter/http"
	authweb "pet-messenger/internal/auth/adapter/in/web"
	authbcrypt "pet-messenger/internal/auth/adapter/out/bcrypt"
	authjwt "pet-messenger/internal/auth/adapter/out/jwt"
	authpersistence "pet-messenger/internal/auth/adapter/out/persistence"
	authrefreshtoken "pet-messenger/internal/auth/adapter/out/refreshtoken"
	authcommand "pet-messenger/internal/auth/application/command"
	authout "pet-messenger/internal/auth/application/port/out"
	"pet-messenger/internal/config"
	infraauth "pet-messenger/internal/infrastructure/auth"
	"pet-messenger/internal/infrastructure/postgres"
)

func newRefreshTTL(cfg *config.Config) authout.RefreshTTL {
	return authout.RefreshTTL(cfg.JWT.RefreshTTL)
}

var AuthModule = fx.Module("auth",
	fx.Provide(
		authpersistence.NewSessionRepository,
		postgres.NewE2EEDeviceRepository,
		authbcrypt.NewHasher,
		authrefreshtoken.NewGenerator,
		authjwt.NewIssuer,
		infraauth.NewJWTVerifier,
		newRefreshTTL,

		authcommand.NewRegisterHandler,
		authcommand.NewLoginHandler,
		authcommand.NewRefreshHandler,
		authcommand.NewLogoutHandler,

		authweb.NewAuthHandler,
		httpadapter.NewE2EEDeviceHandler,
	),
)
