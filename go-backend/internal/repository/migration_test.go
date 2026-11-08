package repository

import (
	"context"
	"database/sql"
	"os"
	"testing"
)

func testGoBackendDSN(t *testing.T) string {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set; skipping migration test")
	}
	return dsn
}

// TestGoBackendMigrations_Idempotent runs go-backend migrations twice and
// verifies idempotency via schema_migrations checksum tracking.
func TestGoBackendMigrations_Idempotent(t *testing.T) {
	dsn := testGoBackendDSN(t)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Start clean.
	db.ExecContext(ctx, `DROP TABLE IF EXISTS schema_migrations CASCADE`)

	// First run.
	if err := runMigrations(db); err != nil {
		t.Fatalf("first runMigrations: %v", err)
	}

	// Verify all expected versions are recorded.
	expected := []string{"001", "002", "003", "004"}
	for _, v := range expected {
		var found string
		err := db.QueryRowContext(ctx, `SELECT version FROM schema_migrations WHERE version=$1`, v).Scan(&found)
		if err != nil {
			t.Errorf("migration %s not recorded in schema_migrations: %v", v, err)
		}
		if found != v {
			t.Errorf("expected version %s, got %s", v, found)
		}
	}

	// Second run: no-op.
	if err := runMigrations(db); err != nil {
		t.Fatalf("second runMigrations (should be no-op): %v", err)
	}

	// Verify no duplicates.
	var count int
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count)
	if count != len(expected) {
		t.Errorf("expected %d schema_migrations rows after idempotent re-run, got %d", len(expected), count)
	}
}
