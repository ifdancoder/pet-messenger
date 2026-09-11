package callevents

import (
	"context"
	"log"
	"time"

	"go.uber.org/fx"

	usecasecall "pet-messenger/internal/usecase/call"
)

// reapInterval is how often StartReaper sweeps for stale calls. 30s bounds
// how long a call can appear live after its media pod hard-crashed.
const reapInterval = 30 * time.Second

// StartReaper runs Service.ReapStaleCalls on a ticker for the lifetime of the
// api-service process. It is a backstop for the normal path (media-service
// publishes call.<id>.ended on room drain) — most calls never reach it.
func StartReaper(lc fx.Lifecycle, svc *usecasecall.Service) {
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
						if n, err := svc.ReapStaleCalls(ctx); err != nil {
							log.Printf("callevents: reaper: %v", err)
						} else if n > 0 {
							log.Printf("callevents: reaper ended %d stale call(s)", n)
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
