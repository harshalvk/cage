package reaper

import (
	"context"
	"log/slog"
	"time"

	"github.com/harshalvk/cage/internal/backend"
	"github.com/harshalvk/cage/internal/lock"
	"github.com/harshalvk/cage/internal/metrics"
	"github.com/harshalvk/cage/internal/store"
)

type Reaper struct {
	sb       backend.SandboxBackend
	store    *store.Store
	interval time.Duration
	lock     *lock.DistributedLock
}

func NewReaper(sb backend.SandboxBackend, st *store.Store, interval time.Duration, l *lock.DistributedLock) *Reaper {
	return &Reaper{sb: sb, store: st, interval: interval, lock: l}
}

func (r *Reaper) Start(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("reaper stopped")
			return
		case <-ticker.C:
			acquired, err := r.lock.TryAcquire(ctx)
			if err != nil {
				slog.Error("reaper: lock acquire error, skipping this tick", "error", err)
				continue
			}
			if !acquired {
				continue
			}

			r.reap(ctx)

			if err := r.lock.Release(ctx); err != nil {
				slog.Error("reaper: failed to release lock", "error", err)
			}
		}
	}
}

func (r *Reaper) reap(ctx context.Context) {
	expired, err := r.store.ListExpired(ctx)
	if err != nil {
		slog.Error("reaper: failed to list expired sandboxes", "error", err)
		return
	}

	for _, sb := range expired {
		switch sb.Status {
		case store.StatusRunning:
			r.reapRunning(ctx, sb)
		case store.StatusPaused:
			r.reapPaused(ctx, sb)
		}
	}
}

func (r *Reaper) reapRunning(ctx context.Context, sb *store.Sandbox) {
	slog.Info("reaper: killing expired running sandbox", "sandbox_id", sb.ID)
	if err := r.sb.KillSandbox(ctx, sb.ID); err != nil {
		slog.Error("reaper: failed to kill sandbox", "sandbox_id", sb.ID, "error", err)
		return // don't delete the DB record if the kill failed — retry next tick
	}
	if err := r.store.Delete(ctx, sb.ID); err != nil {
		slog.Error("reaper: failed to delete sandbox record", "sandbox_id", sb.ID, "error", err)
		return
	}
	metrics.SandboxesReaped.Inc()
}

// reapPaused garbage-collects an abandoned paused sandbox. Image cleanup
// is an optional backend capability (backend.ImageCleaner) — on a backend
// that doesn't support it (or doesn't support pausing at all), we still
// remove the now-meaningless DB record rather than leaking it forever,
// but log clearly that no underlying resource was cleaned up.
func (r *Reaper) reapPaused(ctx context.Context, sb *store.Sandbox) {
	slog.Info("reaper: cleaning up abandoned paused sandbox", "sandbox_id", sb.ID)

	if sb.PausedImageID == nil {
		slog.Warn("reaper: paused sandbox has no pause reference recorded, deleting record only", "sandbox_id", sb.ID)
		_ = r.store.Delete(ctx, sb.ID)
		return
	}

	cleaner, ok := backend.AsImageCleaner(r.sb)
	if !ok {
		slog.Warn("reaper: active backend does not support image cleanup, deleting record only", "sandbox_id", sb.ID)
		_ = r.store.Delete(ctx, sb.ID)
		return
	}

	if err := cleaner.RemoveImage(ctx, *sb.PausedImageID); err != nil {
		slog.Error("reaper: failed to remove paused image", "sandbox_id", sb.ID, "image_ref", *sb.PausedImageID, "error", err)
		return // retry next tick, same fail-safe ordering as reapRunning
	}
	if err := r.store.Delete(ctx, sb.ID); err != nil {
		slog.Error("reaper: failed to delete sandbox record", "sandbox_id", sb.ID, "error", err)
		return
	}
	metrics.SandboxesReaped.Inc()
}
