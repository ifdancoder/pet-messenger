// internal/fx/e2ee.go
package fxapp

import (
	"go.uber.org/fx"

	e2eeweb "pet-messenger/internal/e2ee/adapter/in/web"
	e2eepersistence "pet-messenger/internal/e2ee/adapter/out/persistence"
	e2eecommand "pet-messenger/internal/e2ee/application/command"
	e2eeout "pet-messenger/internal/e2ee/application/port/out"
	e2eequery "pet-messenger/internal/e2ee/application/query"
)

var E2EEModule = fx.Module("e2ee",
	fx.Provide(
		// Secondary adapter, provided as its concrete type, because it
		// satisfies six narrow out ports at once (the device/key-envelope
		// read and write ports split in this slice's design spec).
		e2eepersistence.NewDeviceRepository,

		func(r *e2eepersistence.DeviceRepository) e2eeout.DeviceLoader { return r },
		func(r *e2eepersistence.DeviceRepository) e2eeout.DeviceWriter { return r },
		func(r *e2eepersistence.DeviceRepository) e2eeout.DeviceListerForUser { return r },
		func(r *e2eepersistence.DeviceRepository) e2eeout.TrustedDeviceLister { return r },
		func(r *e2eepersistence.DeviceRepository) e2eeout.KeyEnvelopeWriter { return r },
		func(r *e2eepersistence.DeviceRepository) e2eeout.KeyEnvelopeReader { return r },

		// ConversationMemberChecker and CallLoader are provided by
		// ConversationModule and CallModule respectively: each of those
		// contexts owns the concrete adapter that satisfies e2ee's own
		// narrow port, so the bridge lives in that producer's own fx file,
		// not here.

		// CQRS write side.
		e2eecommand.NewRegisterDeviceHandler,
		e2eecommand.NewApproveDeviceHandler,
		e2eecommand.NewPutCallKeyEnvelopesHandler,

		// CQRS read side.
		e2eequery.NewListDevicesHandler,
		e2eequery.NewListConversationDevicesHandler,
		e2eequery.NewGetCallKeyEnvelopeHandler,

		// Primary adapter.
		e2eeweb.NewE2EEDeviceHandler,
	),
)
