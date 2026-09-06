// cmd/api/main.go
package main

import (
	"go.uber.org/fx"

	fxapp "pet-messenger/internal/fx"
)

func main() {
	fx.New(
		fxapp.ConfigModule,
		fxapp.DatabaseModule,
		fxapp.AuthModule,
		fxapp.HTTPModule,
	).Run()
}
