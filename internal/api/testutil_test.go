package api

import (
	"time"

	"github.com/harshalvk/cage/internal/backend"
	"github.com/harshalvk/cage/internal/pool"
	"github.com/harshalvk/cage/internal/store"
)

func newTestAPI(sb backend.SandboxBackend, st *store.Store, p *pool.Pool) *API {
	return NewAPI(sb, st, time.Hour, 24*time.Hour, p, "docker")
}
