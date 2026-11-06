// Package server wires configuration, storage, services, and the HTTP API
// together and runs the HTTP server.
package server

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/thomasbudiarjo/envily/internal/config"
	"github.com/thomasbudiarjo/envily/internal/crypto"
	"github.com/thomasbudiarjo/envily/internal/httpapi"
	"github.com/thomasbudiarjo/envily/internal/service"
	"github.com/thomasbudiarjo/envily/internal/store"
	"github.com/thomasbudiarjo/envily/internal/version"
	"github.com/thomasbudiarjo/envily/web"
)

// Run serves until ctx is cancelled, then shuts down gracefully.
func Run(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	d, err := OpenDB(ctx, cfg, cfg.AutoMigrate, log)
	if err != nil {
		return err
	}
	defer d.Close()
	kr, err := Keyring(ctx, d, cfg.MasterKey)
	if err != nil {
		return err
	}
	crypto.Zero(cfg.MasterKey) // the keyring holds its own copy
	ready := func(ctx context.Context) error {
		if err := d.Ping(ctx); err != nil {
			return err
		}
		stored, err := d.Meta().Get(ctx, store.MetaKeyCheck)
		if err != nil {
			return err
		}
		sealed, err := base64.StdEncoding.DecodeString(stored)
		if err != nil {
			return crypto.ErrKeyMismatch
		}
		return kr.VerifyKeyCheck(sealed)
	}

	svc := service.New(d, kr, service.Options{
		SessionIdleTimeout: cfg.SessionIdleTimeout, SessionMaxAge: cfg.SessionMaxAge,
		SetupToken: cfg.SetupToken, AuditRetentionDays: cfg.AuditRetentionDays, Logger: log,
	})
	go svc.RunMaintenance(ctx)

	h := httpapi.New(httpapi.Deps{Config: cfg, Logger: log, UI: web.FS(), Ready: ready, Service: svc})
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return err
	}
	log.Info("envily started", "version", version.Version, "addr", ln.Addr().String(),
		"public_url", cfg.PublicURL.String(), "db_driver", cfg.DBDriver)

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
