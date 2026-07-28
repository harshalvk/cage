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
	"github.com/harshalvk/cage/internal/pool"
	"github.com/harshalvk/cage/internal/sandbox"
	"github.com/harshalvk/cage/internal/store"
)

type API struct {
	sm         *sandbox.SandboxManager
	store      *store.Store
	sandboxTTL time.Duration
	pausedTTL  time.Duration
	pool       *pool.Pool
}

type ExecRequest struct {
	Cmd []string `json:"cmd"`
}

type WriteFileRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"` // for now plain text; base64 later for binary
}
type CreateSandboxRequest struct {
	Template string `json:"template"`
}

func NewAPI(sm *sandbox.SandboxManager, store *store.Store, sandboxTTL time.Duration, pausedTTL time.Duration, p *pool.Pool) *API {
	return &API{sm: sm, store: store, sandboxTTL: sandboxTTL, pausedTTL: pausedTTL, pool: p}
}

func (a *API) CreateSandbox(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateSandboxRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Template == "" {
		req.Template = "base"
	}

	tmpl, err := a.store.GetTemplateBySlug(ctx, req.Template)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if tmpl == nil {
		http.Error(w, fmt.Sprintf("unknown templates: %s", req.Template), http.StatusBadRequest)
		return
	}

	var containerID string
	var fromPool bool

	if containerID, fromPool = a.pool.Take(ctx, tmpl.Slug); !fromPool {
		// cold path - pool was empty, pay the full create cost
		containerID, err = a.sm.CreateSandbox(ctx, tmpl.Image)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	sb := &store.Sandbox{
		ID:           uuid.NewString(),
		ContainerID:  containerID,
		Status:       store.StatusRunning,
		CreatedAt:    timeNow(),
		ExpiresAt:    timeNow().Add(a.sandboxTTL),
		TemplateSlug: tmpl.Slug,
	}

	if err := a.store.Save(r.Context(), sb); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("X-Sandbox-Warm-Start", fmt.Sprintf("%t", fromPool)) // useful for debugging/measuring
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(sb); err != nil {
		slog.Error("failed to encode response: %v", "error", err)
	}
}

func (a *API) GetSandbox(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if _, err := uuid.Parse(id); err != nil {
		http.Error(w, "sandbox not found", http.StatusNotFound)
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
		slog.Error("failed to encode response: %v", "error", err)
	}
}

func (a *API) DeleteSandbox(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sb, err := a.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if sb == nil {
		http.Error(w, "sandbox not found", http.StatusNotFound)
		return
	}

	if err := a.sm.KillSandbox(r.Context(), sb.ContainerID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := a.store.Delete(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) ListSandboxes(w http.ResponseWriter, r *http.Request) {
	sandboxes, err := a.store.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if sandboxes == nil {
		http.Error(w, "sandboxes not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(sandboxes); err != nil {
		slog.Error("failed to encode response: %v", "error", err)
	}
}

func (a *API) ExecCommand(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
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

	result, err := a.sm.ExecCommand(r.Context(), sb.ContainerID, req.Cmd)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		slog.Error("failed to encode response: %v", "error", err)
	}
}

func (a *API) WriteFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
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

	if err := a.sm.WriteFile(r.Context(), sb.ContainerID, req.Path, []byte(req.Content)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *API) ReadFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sb, err := a.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if sb != nil {
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

	content, err := a.sm.ReadFile(r.Context(), sb.ContainerID, path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	if _, err := w.Write(content); err != nil {
		slog.Error("failed to write response: %v", "error", err)
	}
}

func (a *API) ListTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := a.store.ListTemplate(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(templates); err != nil {
		slog.Error("failed to encode response: %v", "error", err)
	}
}

func (a *API) PauseSandbox(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
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

	imageID, err := a.sm.PauseSandbox(r.Context(), sb.ContainerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sb.Status = store.StatusPaused
	sb.PausedImageID = &imageID
	sb.ContainerID = "" // this is consider as no live contaier while paused
	sb.ExpiresAt = timeNow().Add(a.pausedTTL)

	if err := a.store.Save(r.Context(), sb); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(sb); err != nil {
		slog.Error("failed to encode response: %v", "error", err)
	}
}

func (a *API) ResumeSandbox(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
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
		http.Error(w, "sandbox is paused but has no associated image - inconsisten state", http.StatusInternalServerError)
		return
	}

	if sb.PausedImageID == nil {
		http.Error(w, "sandbox is paued but has no associated image", http.StatusInternalServerError)
		return
	}
	oldImageID := *sb.PausedImageID

	newContainerID, err := a.sm.ResumeSandbox(r.Context(), oldImageID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sb.ContainerID = newContainerID
	sb.Status = store.StatusRunning
	sb.PausedImageID = nil
	sb.ExpiresAt = timeNow().Add(a.sandboxTTL)

	if err := a.store.Save(r.Context(), sb); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// clean-up the non-unneeded snapshot image in the background
	// dont block the response on it, and a failure there shouldn't fail the resume
	go func() {
		if err := a.sm.RemoveImage(context.Background(), oldImageID); err != nil {
			slog.Error("failed to clean up paused image %s: %v", "image_id", oldImageID, "error", err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(sb); err != nil {
		slog.Error("failed to encode response: %v", "error", err)
	}
}
