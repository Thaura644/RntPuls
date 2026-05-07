package store

import (
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConns = 25
	cfg.MinConns = 5

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return pool, nil
}

func Migrate(ctx context.Context, db *pgxpool.Pool, fs embed.FS, migrationsDir string) error {
	if err := ensureMigrationsTable(ctx, db); err != nil {
		return fmt.Errorf("ensure migrations table: %w", err)
	}

	applied, err := getAppliedMigrations(ctx, db)
	if err != nil {
		return fmt.Errorf("get applied migrations: %w", err)
	}

	files, err := fs.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	migrations, err := parseMigrationFiles(files)
	if err != nil {
		return fmt.Errorf("parse migration files: %w", err)
	}

	for _, m := range migrations {
		if applied[m.Version] {
			continue
		}

		upPath := filepath.Join(migrationsDir, m.UpFile)
		upSQL, err := fs.ReadFile(upPath)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", m.UpFile, err)
		}

		tx, err := db.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", m.Version, err)
		}

		if _, err := tx.Exec(ctx, string(upSQL)); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("apply migration %d (%s): %w", m.Version, m.Name, err)
		}

		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version, name, applied_at) VALUES ($1, $2, now())", m.Version, m.Name); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("record migration %d: %w", m.Version, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.Version, err)
		}
	}

	return nil
}

func Rollback(ctx context.Context, db *pgxpool.Pool, fs embed.FS, migrationsDir string, targetVersion int) error {
	applied, err := getAppliedMigrations(ctx, db)
	if err != nil {
		return fmt.Errorf("get applied migrations: %w", err)
	}

	files, err := fs.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	migrations, err := parseMigrationFiles(files)
	if err != nil {
		return fmt.Errorf("parse migration files: %w", err)
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version > migrations[j].Version
	})

	for _, m := range migrations {
		if !applied[m.Version] || m.Version <= targetVersion {
			continue
		}

		downPath := filepath.Join(migrationsDir, m.DownFile)
		downSQL, err := fs.ReadFile(downPath)
		if err != nil {
			return fmt.Errorf("read down migration %s: %w", m.DownFile, err)
		}

		tx, err := db.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin rollback %d: %w", m.Version, err)
		}

		if _, err := tx.Exec(ctx, string(downSQL)); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("apply rollback %d (%s): %w", m.Version, m.Name, err)
		}

		if _, err := tx.Exec(ctx, "DELETE FROM schema_migrations WHERE version = $1", m.Version); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("remove migration record %d: %w", m.Version, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit rollback %d: %w", m.Version, err)
		}
	}

	return nil
}

type migrationFile struct {
	Version  int
	Name     string
	UpFile   string
	DownFile string
}

func parseMigrationFiles(files []os.DirEntry) ([]migrationFile, error) {
	var migrations []migrationFile
	seen := make(map[int]bool)

	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".sql") {
			continue
		}

		name := f.Name()
		parts := strings.SplitN(name, "_", 2)
		if len(parts) != 2 {
			continue
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		isDown := strings.HasSuffix(parts[1], ".down.sql")
		isUp := strings.HasSuffix(parts[1], ".up.sql")

		if !isUp && !isDown {
			continue
		}

		if !seen[version] {
			baseName := strings.TrimSuffix(parts[1], ".up.sql")
			baseName = strings.TrimSuffix(baseName, ".down.sql")
			migrations = append(migrations, migrationFile{Version: version, Name: baseName})
			seen[version] = true
		}

		for i := range migrations {
			if migrations[i].Version == version {
				if isUp {
					migrations[i].UpFile = name
				} else {
					migrations[i].DownFile = name
				}
				break
			}
		}
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	for _, m := range migrations {
		if m.UpFile == "" {
			return nil, fmt.Errorf("migration %d (%s) missing up file", m.Version, m.Name)
		}
	}

	return migrations, nil
}

func ensureMigrationsTable(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`)
	return err
}

func getAppliedMigrations(ctx context.Context, db *pgxpool.Pool) (map[int]bool, error) {
	rows, err := db.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}
	return applied, nil
}
