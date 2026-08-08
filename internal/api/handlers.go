package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/harshalvk/cage/internal/backend"
	"github.com/harshalvk/cage/internal/pool"
	"github.com/harshalvk/cage/internal/store"
)

type API struct {
	sb               backend.SandboxBackend
	store            *store.Store
	sandboxTTL       time.Duration
	pausedTTL        time.Duration
	pool             *pool.Pool // nil when the active backend has no warm-pool support
	isolationBackend string     // "docker" or "firecracker" — needed to resolve template refs correctly
}

func NewAPI(sb backend.SandboxBackend, st *store.Store, sandboxTTL, pausedTTL time.Duration, p *pool.Pool, isolationBackend string) *API {
	return &API{sb: sb, store: st, sandboxTTL: sandboxTTL, pausedTTL: pausedTTL, pool: p, isolationBackend: isolationBackend}
}

// parseUUID validates that id is a well-formed UUID before it's used in a
// database query. A malformed ID is treated as "not found" (404) rather
// than surfacing a raw database error as a 500 — see the ADR on ID
// validation for why this distinction matters.
func parseUUID(w http.ResponseWriter, id string) (string, bool) {
	if _, err := uuid.Parse(id); err != nil {
		http.Error(w, "sandbox not found", http.StatusNotFound)
		return "", false
	}
	return id, true
}

type CreateSandboxRequest struct {
	Template string `json:"template"`
}

type ExecRequest struct {
	Cmd []string `json:"cmd"`
}

type WriteFileRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// --- Sandbox lifecycle ---

func (a *API) CreateSandbox(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateSandboxRequest
	_ = json.NewDecoder(r.Body).Decode(&req) // body is optional; defaults below cover a missing/empty one
	if req.Template == "" {
		req.Template = "base"
	}

	tmpl, err := a.store.GetTemplateBySlug(ctx, req.Template)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if tmpl == nil {
		http.Error(w, fmt.Sprintf("unknown template: %s", req.Template), http.StatusBadRequest)
		return
	}

	templateRef, err := tmpl.ResolveRef(a.isolationBackend)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sandboxID := uuid.NewString()
	fromPool := false

	// Try the warm pool first, but only if both (a) this API instance has
	// one configured, and (b) the active backend actually supports adopting
	// a pre-warmed resource. Neither is guaranteed — e.g. a Firecracker
	// backend currently has no pool at all (a.pool == nil).
	if a.pool != nil {
		if adopter, ok := backend.AsWarmAdopter(a.sb); ok {
			if nativeID, ok := a.pool.Take(ctx, tmpl.Slug); ok {
				if err := adopter.AdoptWarmResource(ctx, sandboxID, nativeID); err != nil {
					slog.Warn("failed to adopt warm resource, falling back to cold create", "error", err)
				} else {
					fromPool = true
				}
			}
		}
	}

	if !fromPool {
		if err := a.sb.CreateSandbox(ctx, sandboxID, templateRef); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	sb := &store.Sandbox{
		ID:           sandboxID,
		ContainerID:  sandboxID, // sandbox ID is now the single identity handed to the backend
		Status:       store.StatusRunning,
		CreatedAt:    timeNow(),
		ExpiresAt:    timeNow().Add(a.sandboxTTL),
		TemplateSlug: tmpl.Slug,
	}
	if err := a.store.Save(ctx, sb); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("X-Sandbox-Warm-Start", fmt.Sprintf("%t", fromPool))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(sb); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func (a *API) GetSandbox(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	sb, err := a.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if sb == nil {
		http.Error(w, "sandbox not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(sb); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func (a *API) ListSandboxes(w http.ResponseWriter, r *http.Request) {
	sandboxes, err := a.store.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(sandboxes); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func (a *API) DeleteSandbox(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	sb, err := a.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if sb == nil {
		http.Error(w, "sandbox not found", http.StatusNotFound)
		return
	}

	if err := a.sb.KillSandbox(r.Context(), sb.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := a.store.Delete(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) ListTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := a.store.ListTemplate(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(templates); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

// --- Exec & files ---

func (a *API) ExecCommand(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	sb, err := a.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if sb == nil {
		http.Error(w, "sandbox not found", http.StatusNotFound)
		return
	}
	if sb.Status != store.StatusRunning {
		http.Error(w, fmt.Sprintf("sandbox is %q, not running", sb.Status), http.StatusConflict)
		return
	}

	var req ExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Cmd) == 0 {
		http.Error(w, "cmd is required", http.StatusBadRequest)
		return
	}

	stdout, stderr, exitCode, err := a.sb.ExecCommand(r.Context(), sb.ID, req.Cmd)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	result := struct {
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
		ExitCode int    `json:"exit_code"`
	}{Stdout: stdout, Stderr: stderr, ExitCode: exitCode}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func (a *API) WriteFile(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	sb, err := a.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if sb == nil {
		http.Error(w, "sandbox not found", http.StatusNotFound)
		return
	}
	if sb.Status != store.StatusRunning {
		http.Error(w, fmt.Sprintf("sandbox is %q, not running", sb.Status), http.StatusConflict)
		return
	}

	var req WriteFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}

	if err := a.sb.WriteFile(r.Context(), sb.ID, req.Path, req.Content); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) ReadFile(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	sb, err := a.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if sb == nil {
		http.Error(w, "sandbox not found", http.StatusNotFound)
		return
	}
	if sb.Status != store.StatusRunning {
		http.Error(w, fmt.Sprintf("sandbox is %q, not running", sb.Status), http.StatusConflict)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path query param is required", http.StatusBadRequest)
		return
	}

	content, err := a.sb.ReadFile(r.Context(), sb.ID, path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	if _, err := w.Write([]byte(content)); err != nil {
		slog.Error("failed to write response", "error", err)
	}
}

// --- Pause / resume ---
//
// Pause/resume is an optional backend capability (backend.Pausable) — not
// every isolation backend can suspend and restore a sandbox. These
// handlers check for the capability explicitly and return 501 Not
// Implemented on a backend that lacks it, rather than assuming Docker.

func (a *API) PauseSandbox(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	sb, err := a.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if sb == nil {
		http.Error(w, "sandbox not found", http.StatusNotFound)
		return
	}
	if sb.Status != store.StatusRunning {
		http.Error(w, fmt.Sprintf("cannot pause sandbox in status %q", sb.Status), http.StatusConflict)
		return
	}

	pausable, ok := backend.AsPausable(a.sb)
	if !ok {
		http.Error(w, "pause/resume is not supported on the active isolation backend", http.StatusNotImplemented)
		return
	}

	pauseRef, err := pausable.PauseSandbox(r.Context(), sb.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sb.Status = store.StatusPaused
	sb.PausedImageID = &pauseRef
	sb.ExpiresAt = timeNow().Add(a.pausedTTL)

	if err := a.store.Save(r.Context(), sb); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(sb); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func (a *API) ResumeSandbox(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	sb, err := a.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if sb == nil {
		http.Error(w, "sandbox not found", http.StatusNotFound)
		return
	}
	if sb.Status != store.StatusPaused {
		http.Error(w, fmt.Sprintf("cannot resume sandbox in status %q", sb.Status), http.StatusConflict)
		return
	}
	if sb.PausedImageID == nil {
		http.Error(w, "sandbox is paused but has no pause reference recorded — inconsistent state", http.StatusInternalServerError)
		return
	}

	pausable, ok := backend.AsPausable(a.sb)
	if !ok {
		http.Error(w, "pause/resume is not supported on the active isolation backend", http.StatusNotImplemented)
		return
	}

	pauseRef := *sb.PausedImageID

	if err := pausable.ResumeSandbox(r.Context(), sb.ID, pauseRef); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sb.Status = store.StatusRunning
	sb.PausedImageID = nil
	sb.ExpiresAt = timeNow().Add(a.sandboxTTL)

	if err := a.store.Save(r.Context(), sb); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Best-effort cleanup of the now-unneeded pause resource, if the
	// backend supports it. Failure here doesn't fail the resume itself.
	if cleaner, ok := backend.AsImageCleaner(a.sb); ok {
		go func() {
			if err := cleaner.RemoveImage(context.Background(), pauseRef); err != nil {
				slog.Error("failed to clean up pause resource after resume", "pause_ref", pauseRef, "error", err)
			}
		}()
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(sb); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}
