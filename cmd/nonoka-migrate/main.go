package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"

	"nonoka-im/internal/data"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := flag.String("dsn", os.Getenv("POSTGRES_DSN"), "PostgreSQL DSN")
	flag.Parse()
	if *dsn == "" {
		fmt.Fprintln(os.Stderr, "postgres DSN is required (-dsn or POSTGRES_DSN)")
		os.Exit(2)
	}
	db, err := sql.Open("pgx", *dsn)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	// The CLI explicitly intends to migrate; do not let DB_MIGRATE=off silently
	// turn it into a no-op.
	if os.Getenv("DB_MIGRATE") == "" {
		_ = os.Setenv("DB_MIGRATE", "on")
	}
	if err := data.Migrate(context.Background(), db); err != nil {
		panic(err)
	}
	fmt.Println("migrations applied")
}
