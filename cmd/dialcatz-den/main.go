// DialCatz Den: the self-hosted team server for the DialCatz API client. One binary: sync API, admin API and
// the embedded admin panel. Configuration through environment variables (see internal/config).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/catzoidhub/DialCatz-Den/internal/api"
	"github.com/catzoidhub/DialCatz-Den/internal/config"
	"github.com/catzoidhub/DialCatz-Den/internal/db"
	"github.com/catzoidhub/DialCatz-Den/internal/store"
	"github.com/catzoidhub/DialCatz-Den/internal/web"
)

func main() {
	// `dialcatz-den healthcheck`: for Docker's HEALTHCHECK in an image without curl.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		addr := os.Getenv("DIALCATZ_ADDR")
		if addr == "" {
			addr = ":8080"
		}
		if addr[0] == ':' {
			addr = "127.0.0.1" + addr
		}
		res, err := (&http.Client{Timeout: 3 * time.Second}).Get("http://" + addr + "/api/health")
		if err != nil || res.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if err := run(); err != nil {
		slog.Error("stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	database, err := db.Open(cfg.DatabaseURL, cfg.IsPostgres())
	if err != nil {
		return err
	}
	defer database.Close()
	st := store.New(database)
	server, err := api.New(cfg, st)
	if err != nil {
		return err
	}
	// The event stream (/api/v1/workspaces/{id}/events) lifts these timeouts for itself, write by write.
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           server.Router(web.Panel()),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	srv.RegisterOnShutdown(server.Shutdown)
	kind := "SQLite"
	if cfg.IsPostgres() {
		kind = "PostgreSQL"
	}
	n, _ := st.AdminCount()
	slog.Info(config.Product+" "+config.Version, "addr", cfg.Addr, "public", cfg.PublicURL, "database", kind)
	if n == 0 {
		slog.Info(fmt.Sprintf("First start: open %s to create the admin account", cfg.PublicURL))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		slog.Info("shutting down")
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(sctx)
	}
	return nil
}
