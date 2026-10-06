package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/AndrewDMcC/estate-agency/internal/config"
	"github.com/AndrewDMcC/estate-agency/internal/rightmove"
	"github.com/AndrewDMcC/estate-agency/internal/store"
	"github.com/AndrewDMcC/estate-agency/internal/web"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return err
	}
	st, err := store.Open(filepath.Join(cfg.DataDir, "agency.db"))
	if err != nil {
		return err
	}
	defer st.Close()

	if cfg.SeedDemo {
		seeded, err := st.SeedDemo()
		if err != nil {
			return err
		}
		if seeded {
			log.Info("seeded demo data")
		}
	}

	rm, err := rightmove.New(cfg.Rightmove, log)
	if err != nil {
		return err
	}
	srv, err := web.New(st, cfg, rm, log)
	if err != nil {
		return err
	}
	if cfg.AdminPassword == "" {
		log.Warn("ADMIN_PASSWORD is not set; the admin area is disabled")
	}

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Addr, "rightmove_mode", rm.Mode())
		errCh <- httpSrv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpSrv.Shutdown(shutdownCtx)
	}
	return nil
}
