// internal/fx/auth.go
package fxapp

import (
	"go.uber.org/fx"

	httpadapter "pet-messenger/internal/adapter/http"
	infraauth "pet-messenger/internal/infrastructure/auth"
	"pet-messenger/internal/infrastructure/postgres"
	usecaseauth "pet-messenger/internal/usecase/auth"
)

var AuthModule = fx.Module("auth",
	fx.Provide(
		postgres.NewUserRepository,
		postgres.NewSessionRepository,
		infraauth.NewBcryptHasher,
		infraauth.NewRefreshTokenGenerator,
		infraauth.NewJWTIssuer,
		infraauth.NewJWTVerifier,
		usecaseauth.NewService,
		httpadapter.NewAuthHandler,
	),
)
