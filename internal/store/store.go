package store

import (
	"context"
	"embed"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	return pool, pool.Ping(ctx)
}

func Migrate(ctx context.Context, db *pgxpool.Pool, fs embed.FS, file string) error {
	sql, err := fs.ReadFile(file)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, string(sql))
	return err
}
