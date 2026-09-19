// internal/fx/auth.go
package fxapp

import (
	"go.uber.org/fx"

	httpadapter "pet-messenger/internal/adapter/http"
	"pet-messenger/internal/config"
	infraauth "pet-messenger/internal/infrastructure/auth"
	objectstoreinfra "pet-messenger/internal/infrastructure/objectstore"
	"pet-messenger/internal/infrastructure/postgres"
	usecaseauth "pet-messenger/internal/usecase/auth"
)

func newRefreshTTL(cfg *config.Config) usecaseauth.RefreshTTL {
	return usecaseauth.RefreshTTL(cfg.JWT.RefreshTTL)
}

var AuthModule = fx.Module("auth",
	fx.Provide(
		postgres.NewUserRepository,
		postgres.NewSessionRepository,
		postgres.NewE2EEDeviceRepository,
		infraauth.NewBcryptHasher,
		infraauth.NewRefreshTokenGenerator,
		infraauth.NewJWTIssuer,
		infraauth.NewJWTVerifier,
		newRefreshTTL,
		usecaseauth.NewService,
		httpadapter.NewAuthHandler,
		httpadapter.NewUserHandler,
		httpadapter.NewE2EEDeviceHandler,
		objectstoreinfra.NewMinioAvatarStore,
	),
)
