package reconcile

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/harshalvk/cage/internal/backend"
	"github.com/harshalvk/cage/internal/lock"
	"github.com/harshalvk/cage/internal/store"
)

func Reconcile(ctx context.Context, sb backend.SandboxBackend, st *store.Store, l *lock.DistributedLock) error {
	acquired, err := l.TryAcquire(ctx)
	if err != nil {
		return fmt.Errorf("reconcile: lock acquire failed: %w", err)
	}
	if !acquired {
		slog.Info("reconcile: another replica is already reconciling, skipping")
		return nil
	}
	defer func() {
		if err := l.Release(ctx); err != nil {
			slog.Error("reconcile: failed to release lock", "error", err)
		}
	}()

	all, err := st.List(ctx)
	if err != nil {
		return err
	}

	for _, s := range all {
		if s.Status != store.StatusRunning {
			continue
		}
		running, err := sb.IsRunning(ctx, s.ID)
		if err != nil {
			slog.Error("reconcile: failed to check sandbox", "sandbox_id", s.ID, "error", err)
			continue
		}
		if !running {
			slog.Info("reconcile: cleaning up stale sandbox record", "sandbox_id", s.ID)
			if err := st.Delete(ctx, s.ID); err != nil {
				slog.Error("reconcile: failed to delete stale sandbox", "sandbox_id", s.ID, "error", err)
			}
		}
	}
	return nil
}
