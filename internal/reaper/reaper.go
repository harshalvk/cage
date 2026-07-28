package reaper

import (
	"context"
	"log/slog"
	"time"

	"github.com/harshalvk/cage/internal/lock"
	"github.com/harshalvk/cage/internal/metrics"
	"github.com/harshalvk/cage/internal/sandbox"
	"github.com/harshalvk/cage/internal/store"
)

type Reaper struct {
	sm       *sandbox.SandboxManager
	store    *store.Store
	interval time.Duration
	lock     *lock.DistributedLock
}

func NewReaper(sm *sandbox.SandboxManager, store *store.Store, interval time.Duration, l *lock.DistributedLock) *Reaper {
	return &Reaper{sm: sm, store: store, interval: interval, lock: l}
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
				// another replica is the leader this tick - nothing to do here
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
		slog.Error("reaper: failed to list expired sandboxes: %v", "error", err)
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
	if err := r.sm.KillSandbox(ctx, sb.ContainerID); err != nil {
		slog.Error("reaper: failed to kill container", "sandbox_id", sb.ID, "error", err)
		return // don't delete the db record if the kill failed; retry next tick
	}
	if err := r.store.Delete(ctx, sb.ID); err != nil {
		slog.Error("reaper: failed to delete sandbox record", "sandbox_id", sb.ID, "error", err)
		return
	}
	metrics.SandboxesReaped.Inc()
}

func (r *Reaper) reapPaused(ctx context.Context, sb *store.Sandbox) {
	slog.Info("reaper: cleaning up abandoned paused sandbox", "sanbox_id", sb.ID)

	if sb.PausedImageID == nil {
		// shouldn't happen, but don't leak the db record over a data inconcsistency
		slog.Error("reaper: paused sandbox has no image id, deleting record anyway", "sandbox_id", sb.ID)
		_ = r.store.Delete(ctx, sb.ID)
		return
	}

	if err := r.sm.RemoveImage(ctx, *sb.PausedImageID); err != nil {
		slog.Error("reaper: failed to remove paused image", "sandbox_id", sb.ID, "image_id", *sb.PausedImageID, "error", err)
		return // retyr next tick, same fail-safe ordering as reapRunning
	}
	if err := r.store.Delete(ctx, sb.ID); err != nil {
		slog.Error("reaper: failed to delete sandbox record", "sandbox_id", sb.ID, "error", err)
		return
	}
	metrics.SandboxesReaped.Inc()
}
