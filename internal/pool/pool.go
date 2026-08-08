package pool

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/harshalvk/cage/internal/backend"
	"github.com/harshalvk/cage/internal/metrics"
)

// TemplateConfig describes one template's warm pool
// TemplateRef is passed straight through to backend.SandboxBackend.CreateSandbox -- its meaning
// depends entirely on which backend is active
type TemplateConfig struct {
	Slug        string
	TemplateRef string
	Size        int
}

// Pool maintains a warm set of pre-created, ready-to-use sandboxes per
// template, backed by any backend.SandboxBackend - not tied to docker
// Warm resources are created under a placeholder ID and later renamed to
// the real sandbox ID via backend.WarmAdopter when handed out
type Pool struct {
	sb        backend.SandboxBackend
	templates map[string]TemplateConfig
	warm      map[string]chan string   // slug -> channel of ready container IDs
	refill    map[string]chan struct{} // slug -> signal to top up immediately
}

func New(sb backend.SandboxBackend, templates []TemplateConfig) *Pool {
	p := &Pool{
		sb:        sb,
		templates: make(map[string]TemplateConfig),
		warm:      make(map[string]chan string),
		refill:    make(map[string]chan struct{}),
	}

	for _, t := range templates {
		p.templates[t.Slug] = t
		p.warm[t.Slug] = make(chan string, t.Size)
		p.refill[t.Slug] = make(chan struct{}, t.Size)
	}

	return p
}

// start launches one maintenance goroutine per template. blocks until ctx is cancelled
func (p *Pool) Start(ctx context.Context) {
	for slug, cfg := range p.templates {
		go p.maintain(ctx, slug, cfg)
	}
}

func (p *Pool) maintain(ctx context.Context, slug string, cfg TemplateConfig) {
	// inital fill - done sequentially and deliberately at startup, before
	// traffic arrives, so that first real requests don't pay the cold-start cost

	for i := 0; i < cfg.Size; i++ {
		p.spawnOne(ctx, slug, cfg.TemplateRef)
	}

	// safety-net ticker, in case a refill signal is ever missed or a warm
	// container dies silently between Take() calls
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.refill[slug]:
			p.topUp(ctx, slug, cfg)
		case <-ticker.C:
			p.topUp(ctx, slug, cfg)
		}
	}
}

func (p *Pool) topUp(ctx context.Context, slug string, cfg TemplateConfig) {
	for len(p.warm[slug]) < cfg.Size {
		p.spawnOne(ctx, slug, cfg.TemplateRef)
	}
}

// spawnOne creates a warm resource under a placeholder ID, clearly
// distinguishable from real sandbox UUIDs - makes an orphaned warm
// resource easy to spot during debugging
func (p *Pool) spawnOne(ctx context.Context, slug, templateRef string) {
	placeholderID := "warm-" + uuid.NewString()

	if err := p.sb.CreateSandbox(ctx, placeholderID, templateRef); err != nil {
		slog.Error("pool: failed to warm resource", "template", slug, "error", err)
		return
	}

	select {
	case p.warm[slug] <- placeholderID:
	default:
		// pool was already full by the time we finished creating this one
		// (e.g. a race with the ticker) - dont' leak it, clean it up
		if err := p.sb.KillSandbox(ctx, placeholderID); err != nil {
			slog.Error("pool: failed to discard excess warm resource", "id", placeholderID, "error", err)
		}
	}
}

/*
Take function returns a ready-to-use placeholder-id for the given template, or
(false) if the pool is currently empty and the caller is responsible for adopting
the returned ID (via backend.WarmAdopter) under the real sandbox ID
*/
func (p *Pool) Take(ctx context.Context, slug string) (containerID string, ok bool) {
	ch, exists := p.warm[slug]
	if !exists {
		metrics.PoolMisses.Inc()
		return "", false
	}

	for {
		select {
		case id := <-ch:
			running, err := p.sb.IsRunning(ctx, id)
			if err != nil || !running {
				slog.Warn("pool: discarding dead warm", "id", id, "template", slug)
				continue // try the next one in the channel, if any
			}
			p.triggerRefill(slug)
			metrics.PoolHits.Inc()
			return id, true
		default:
			metrics.PoolMisses.Inc()
			return "", false
		}
	}
}

func (p *Pool) triggerRefill(slug string) {
	select {
	case p.refill[slug] <- struct{}{}:
	default:
		// a refill already pending - no need to queue another signal
	}
}
