// Command migrate applies the embedded SQL migrations.
// The -database flag overrides DATABASE_URL. With no flag, LoadDatabaseURL
// reads that variable from .env and the process environment.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"lumbercalc/backend/db"
	"lumbercalc/backend/internal/config"
)

func main() {
	database := flag.String("database", "", "Postgres URL (default DATABASE_URL from the environment or .env)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, *database); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, dsn string) error {
	if dsn == "" {
		secret, err := config.LoadDatabaseURL(".env")
		if err != nil {
			return err
		}
		dsn = secret.Reveal()
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	applied, err := db.Migrate(ctx, pool)
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		fmt.Println("up to date")
	}
	for _, name := range applied {
		fmt.Println("applied", name)
	}
	return nil
}
