// Command api loads config, connects to Postgres, and serves HTTP until SIGINT or SIGTERM.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"lumbercalc/backend/db"
	"lumbercalc/backend/internal/api"
	"lumbercalc/backend/internal/clerkauth"
	"lumbercalc/backend/internal/config"
	"lumbercalc/backend/internal/server"
	"lumbercalc/backend/internal/store"
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
	st := store.New(pool)
	clerk, err := clerkauth.New(cfg.ClerkSecretKey, cfg.ClerkAuthorizedParties, log)
	if err != nil {
		return err
	}
	v1, err := api.New(api.Deps{
		Log:          log,
		Authenticate: clerk.Authenticate,
		EnsureUser:   st.EnsureUser,
		DisplayName:  clerk.DisplayName,
		Save:         st.Save,
		Get:          st.Get,
		List:         st.List,
		Delete:       st.Delete,
	})
	if err != nil {
		return err
	}
	h, err := server.New(server.Deps{
		Log: log,
		Ready: []server.Check{
			{Name: "postgres", Probe: pool.Ping},
			{Name: "migrations", Probe: func(ctx context.Context) error { return db.CheckApplied(ctx, pool) }},
		},
		V1:          v1,
		CORSOrigins: cfg.ClerkAuthorizedParties,
	})
	if err != nil {
		return err
	}
	return server.Serve(ctx, cfg.HTTPAddr, h, log)
}
