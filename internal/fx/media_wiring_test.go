package fxapp_test

import (
	"testing"

	"go.uber.org/fx"

	fxapp "pet-messenger/internal/fx"
)

func TestMediaModulesWireTogether(t *testing.T) {
	if err := fx.ValidateApp(
		fxapp.ConfigModule,
		fxapp.RedisModule,
		fxapp.MediaNATSModule,
		fxapp.JoinTicketModule,
		fxapp.SFUModule,
		fxapp.RPCServerModule,
		fxapp.MediaNodeModule,
		fxapp.SignalingModule,
	); err != nil {
		t.Fatalf("media fx graph does not resolve: %v", err)
	}
}
