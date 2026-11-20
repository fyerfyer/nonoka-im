package data

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"strings"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// migrationLockKey serializes migrations across gateway/worker instances.
const migrationLockKey = 0x6e6f6e6f6b61 // "nonoka"

// Migrate applies all pending goose migrations. It is idempotent and safe to
// call from every service on startup; DB_MIGRATE=off disables the automatic
// run for environments that apply migrations out-of-band.
func Migrate(ctx context.Context, db *sql.DB) error {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("DB_MIGRATE")), "off") {
		return nil
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration lock connection: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockKey)
	}()

	goose.SetBaseFS(migrationFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("configure migration dialect: %w", err)
	}
	goose.SetLogger(goose.NopLogger())
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
