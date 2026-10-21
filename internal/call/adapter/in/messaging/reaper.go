package messaging

import (
	"context"
	"log"
	"time"

	"go.uber.org/fx"

	in "pet-messenger/internal/call/application/port/in"
)

// reapInterval is how often StartReaper sweeps for stale calls. 30s
// bounds how long a call can appear live after its media pod
// hard-crashed.
const reapInterval = 30 * time.Second

// StartReaper runs ReapStaleCallsUseCase on a ticker for the lifetime of
// the api-service process. It is a backstop for the normal path
// (media-service publishes call.<id>.ended on room drain) -- most calls
// never reach it.
func StartReaper(lc fx.Lifecycle, uc in.ReapStaleCallsUseCase) {
	stop := make(chan struct{})
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				t := time.NewTicker(reapInterval)
				defer t.Stop()
				for {
					select {
					case <-stop:
						return
					case <-t.C:
						ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
						if n, err := uc.Handle(ctx, in.ReapStaleCallsCommand{}); err != nil {
							log.Printf("callmessaging: reaper: %v", err)
						} else if n > 0 {
							log.Printf("callmessaging: reaper ended %d stale call(s)", n)
						}
						cancel()
					}
				}
			}()
			return nil
		},
		OnStop: func(context.Context) error { close(stop); return nil },
	})
}
