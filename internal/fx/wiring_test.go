// internal/fx/wiring_test.go
package fxapp_test

import (
	"testing"

	"go.uber.org/fx"

	fxapp "pet-messenger/internal/fx"
)

// TestModulesWireTogether checks the dependency graph is satisfiable without
// starting the app or touching a real database — fx.ValidateApp only
// inspects constructor signatures, it never calls them.
func TestModulesWireTogether(t *testing.T) {
	if err := fx.ValidateApp(
		fxapp.ConfigModule,
		fxapp.DatabaseModule,
		fxapp.AuthModule,
		fxapp.ConversationModule,
		fxapp.MessageModule,
		fxapp.NotificationModule,
		fxapp.PushModule,
		fxapp.CallModule,
		fxapp.NATSModule,
		fxapp.RedisModule,
		fxapp.WSModule,
		fxapp.HTTPModule,
	); err != nil {
		t.Fatalf("fx graph does not resolve: %v", err)
	}
}
