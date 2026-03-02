package testutil

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestDB creates a connection to a test database.
// Expects AEGISCLAW_TEST_DATABASE_URL to be set.
func TestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("AEGISCLAW_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://aegisclaw:aegisclaw@localhost:5432/aegisclaw_test?sslmode=disable"
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("skipping test, cannot connect to test database: %v", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("skipping test, cannot ping test database: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
	})

	return pool
}

// CleanupTable truncates a table in the test database.
func CleanupTable(t *testing.T, pool *pgxpool.Pool, table string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
	if err != nil {
		t.Fatalf("failed to cleanup table %s: %v", table, err)
	}
}

// MustExec executes a SQL statement and fails the test on error.
func MustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	_, err := pool.Exec(context.Background(), sql, args...)
	if err != nil {
		t.Fatalf("failed to execute SQL: %v", err)
	}
}
