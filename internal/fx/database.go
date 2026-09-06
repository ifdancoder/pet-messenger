// internal/fx/database.go
package fxapp

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/fx"

	"pet-messenger/internal/config"
	"pet-messenger/internal/database"
)

func newDB(lc fx.Lifecycle, cfg *config.Config) (*sql.DB, error) {
	db, err := sql.Open("pgx", cfg.DB.DSN())
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return db.PingContext(ctx)
		},
		OnStop: func(context.Context) error {
			return db.Close()
		},
	})
	return db, nil
}

var DatabaseModule = fx.Module("database",
	fx.Provide(newDB, database.New),
)
