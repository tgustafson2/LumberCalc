package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"lumbercalc/backend/internal/config"
	"lumbercalc/backend/internal/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.Load(".env")
	if err != nil {
		return err
	}
	log := server.NewLogger(os.Stdout, cfg.LogLevel)
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL.Reveal())
	if err != nil {
		return err
	}
	defer pool.Close()
	h, err := server.New(server.Deps{
		Log:   log,
		Ready: []server.Check{{Name: "postgres", Probe: pool.Ping}},
	})
	if err != nil {
		return err
	}
	return server.Serve(ctx, cfg.HTTPAddr, h, log)
}
