package app

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type rateLimiter struct{}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{}
}

func (r *rateLimiter) init(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS rate_limits (
		key TEXT PRIMARY KEY,
		window_start TIMESTAMPTZ NOT NULL,
		count INTEGER NOT NULL DEFAULT 0
	)`)
	return err
}

func (r *rateLimiter) allow(ctx context.Context, db *pgxpool.Pool, key string, limit int) (bool, error) {
	now := time.Now()

	tx, err := db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin rate limit tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var windowStart time.Time
	var count int

	err = tx.QueryRow(ctx, `SELECT window_start, count FROM rate_limits WHERE key = $1 FOR UPDATE`, key).Scan(&windowStart, &count)
	if err == pgx.ErrNoRows {
		if _, err := tx.Exec(ctx, `INSERT INTO rate_limits (key, window_start, count) VALUES ($1, $2, 1)`, key, now); err != nil {
			return false, fmt.Errorf("insert rate limit: %w", err)
		}
		return true, tx.Commit(ctx)
	}
	if err != nil {
		return false, fmt.Errorf("query rate limit: %w", err)
	}

	if now.Sub(windowStart) >= time.Minute {
		if _, err := tx.Exec(ctx, `UPDATE rate_limits SET window_start = $1, count = 1 WHERE key = $2`, now, key); err != nil {
			return false, fmt.Errorf("reset rate limit: %w", err)
		}
		return true, tx.Commit(ctx)
	}

	if count >= limit {
		return false, tx.Commit(ctx)
	}

	if _, err := tx.Exec(ctx, `UPDATE rate_limits SET count = count + 1 WHERE key = $1`, key); err != nil {
		return false, fmt.Errorf("increment rate limit: %w", err)
	}
	return true, tx.Commit(ctx)
}

func (r *rateLimiter) cleanup(ctx context.Context, db *pgxpool.Pool) error {
	cutoff := time.Now().Add(-5 * time.Minute)
	_, err := db.Exec(ctx, `DELETE FROM rate_limits WHERE window_start < $1`, cutoff)
	return err
}
