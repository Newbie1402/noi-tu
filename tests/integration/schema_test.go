package integration_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPersistentSchemaIsReady(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var observations, settings sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT to_regclass('public.word_observations'), to_regclass('public.runtime_settings')`).Scan(&observations, &settings); err != nil {
		t.Fatal(err)
	}
	if !observations.Valid || !settings.Valid {
		t.Fatalf("missing tables: word_observations=%v runtime_settings=%v", observations, settings)
	}
}
