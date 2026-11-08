package repository

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
}

func NewPostgresDB(cfg Config) (*sqlx.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName,
	)

	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	// Run schema migrations with version tracking + advisory lock.
	if err := runMigrations(db.DB); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	// Connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	return db, nil
}

// migration represents a single SQL migration file.
type migration struct {
	version string
	name    string
	body    string
}

// migrationsDir resolves the directory containing .sql migration files. It
// checks the working directory first (module root), then falls back to a
// path relative to this source file.
func migrationsDir() (string, error) {
	// Prefer the working directory (typically module root when running
	// from make/docker compose).
	if _, err := os.Stat(filepath.Join("migrations", "001_base.up.sql")); err == nil {
		return "migrations", nil
	}
	// Fallback: relative to this source file (internal/repository/).
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(wd, "..", "..", "migrations")
	if _, err := os.Stat(filepath.Join(candidate, "001_base.up.sql")); err == nil {
		return candidate, nil
	}
	return "", errors.New("migrations directory not found")
}

func readMigrationFiles(dir string) ([]migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	var all []migration
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", entry.Name(), err)
		}
		all = append(all, migration{
			version: extractVersion(entry.Name()),
			name:    entry.Name(),
			body:    string(body),
		})
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("no migration files found in %s", dir)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].version < all[j].version
	})
	return all, nil
}

// runMigrations applies SQL migrations in version order, using an advisory
// lock so only one instance migrates at a time. Each migration is recorded
// in schema_migrations with its SHA256 checksum.
func runMigrations(db *sql.DB) error {
	mDir, err := migrationsDir()
	if err != nil {
		return err
	}
	all, err := readMigrationFiles(mDir)
	if err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin lock tx: %w", err)
	}
	var lockResult int
	if err := tx.QueryRow("SELECT pg_advisory_xact_lock(hashtext('arxiv-go-backend-schema-migrations'))").Scan(&lockResult); err != nil {
		tx.Rollback()
		return fmt.Errorf("acquire advisory lock: %w", err)
	}

	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			checksum   TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
		tx.Rollback()
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied := make(map[string]string)
	rows, err := tx.Query("SELECT version, checksum FROM schema_migrations")
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var version, checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			tx.Rollback()
			return fmt.Errorf("scan schema_migrations: %w", err)
		}
		applied[version] = checksum
	}

	for _, m := range all {
		checksum := sha256sum(m.body)
		existing, ok := applied[m.version]
		if ok && existing == checksum {
			continue
		}

		if _, err := tx.Exec(m.body); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %s (%s): %w", m.version, m.name, err)
		}
		if ok {
			if _, err := tx.Exec(`UPDATE schema_migrations SET checksum=$1, applied_at=NOW() WHERE version=$2`, checksum, m.version); err != nil {
				tx.Rollback()
				return fmt.Errorf("update schema_migrations %s: %w", m.version, err)
			}
		} else {
			if _, err := tx.Exec(`INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2)`, m.version, checksum); err != nil {
				tx.Rollback()
				return fmt.Errorf("insert schema_migrations %s: %w", m.version, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}

func extractVersion(name string) string {
	var v []byte
	for _, c := range name {
		if c >= '0' && c <= '9' {
			v = append(v, byte(c))
		} else {
			break
		}
	}
	return string(v)
}

func sha256sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func PingDB(db *sqlx.DB) error {
	if err := db.Ping(); err != nil {
		return fmt.Errorf("postgres ping failed: %w", err)
	}
	return nil
}
