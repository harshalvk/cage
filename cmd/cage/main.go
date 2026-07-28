package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/harshalvk/cage/internal/api"
	"github.com/harshalvk/cage/internal/cache"
	"github.com/harshalvk/cage/internal/config"
	"github.com/harshalvk/cage/internal/db"
	"github.com/harshalvk/cage/internal/lock"
	"github.com/harshalvk/cage/internal/logging"
	"github.com/harshalvk/cage/internal/pool"
	"github.com/harshalvk/cage/internal/ratelimit"
	"github.com/harshalvk/cage/internal/reaper"
	"github.com/harshalvk/cage/internal/reconcile"
	"github.com/harshalvk/cage/internal/sandbox"
	"github.com/harshalvk/cage/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal startup error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// Root context, cancelled on SIGTERM/SIGINT — everything downstream
	// (reaper, pool, in-flight request handling) derives from this.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.LoadConfig()
	if err != nil {
		return err // slog not initialized yet, main() logs this with slog anyway since it's still structured
	}

	logger := logging.New(cfg.LogLevel)
	slog.SetDefault(logger)
	logger.Info("starting cage", "port", cfg.Port, "warm_pool_size", cfg.WarmPoolSize)

	if err := db.RunMigrations(cfg.DatabaseURL); err != nil {
		return err
	}

	st, err := store.NewStore(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}

	sm, err := sandbox.NewSandboxManager()
	if err != nil {
		return err
	}

	c, err := cache.New(cfg.RedisURL)
	if err != nil {
		return err
	}
	defer func() {
		if err := c.Close(); err != nil {
			slog.Error("failed to close cache client", "error", err)
		}
	}()

	reaperLock := lock.New(c.RawClient(), "reaper", 30*time.Second)
	reconcileLock := lock.New(c.RawClient(), "reconcile", 60*time.Second)

	if err := reconcile.Reconcile(ctx, sm, st, reconcileLock); err != nil {
		slog.Error("reconcile failed", "error", err)
	}

	rp := reaper.NewReaper(sm, st, cfg.ReaperInterval, reaperLock)
	go rp.Start(ctx)

	templates, err := st.ListTemplate(ctx)
	if err != nil {
		return err
	}

	poolConfigs := make([]pool.TemplateConfig, 0, len(templates))
	for _, t := range templates {
		poolConfigs = append(poolConfigs, pool.TemplateConfig{
			Slug:  t.Slug,
			Image: t.Image,
			Size:  cfg.WarmPoolSize,
		})
	}
	warmPool := pool.New(sm, poolConfigs)
	warmPool.Start(ctx)
	logger.Info("warm pool ready")

	limiter := ratelimit.NewLimiter(c.RawClient(), 20, 5)

	a := api.NewAPI(sm, st, cfg.SandboxTTL, cfg.PausedTTL, warmPool)

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(api.RequestLogger)
	r.Use(api.MetricsMiddleware)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			slog.Error("failed to encode health response", "error", err)
		}
	})

	r.Get("/templates", a.ListTemplates)
	r.Route("/metrics", func(r chi.Router) {
		r.Use(api.MetricsAuthMiddleware(cfg.MetricsToken))
		r.Handle("/", promhttp.Handler())
	})

	r.Route("/sandboxes", func(r chi.Router) {
		r.Use(api.AuthMiddleware(st, c))
		r.Use(api.RateLimitMiddleware(limiter))
		r.Post("/", a.CreateSandbox)
		r.Get("/", a.ListSandboxes)
		r.Get("/{id}", a.GetSandbox)
		r.Delete("/{id}", a.DeleteSandbox)
		r.Post("/{id}/exec", a.ExecCommand)
		r.Post("/{id}/files", a.WriteFile)
		r.Get("/{id}/files", a.ReadFile)
		r.Post("/{id}/pause", a.PauseSandbox)
		r.Post("/{id}/resume", a.ResumeSandbox)
	})

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second, // generous — exec/file ops can take a while
		IdleTimeout:  60 * time.Second,
	}

	// Run the server in a goroutine so this function can block on ctx.Done() below
	serveErr := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err // server died on its own (e.g. port already in use)
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining connections")
	}

	// Give in-flight requests up to 15s to finish before forcing shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed, forcing close", "error", err)
		return srv.Close()
	}

	logger.Info("shutdown complete")
	return nil
}
