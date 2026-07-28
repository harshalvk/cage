package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/harshalvk/cage/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListSandboxes_EmptyReturnsEmptyArray(t *testing.T) {
	st := setupTestStore(t)
	a := NewAPI(nil, st, 0, 0, nil)

	req := httptest.NewRequest(http.MethodGet, "/sandboxes", nil)
	w := httptest.NewRecorder()

	a.ListSandboxes(w, req)

	resp := w.Result()
	bodyBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body []store.Sandbox
	err = json.Unmarshal(bodyBytes, &body)
	require.NoError(t, err)
	assert.Empty(t, body, "should return [] not null for an empty store")
}

func TestGetSandbox_NotFound(t *testing.T) {
	st := setupTestStore(t)
	a := NewAPI(nil, st, 0, 0, nil)

	r := chi.NewRouter()
	r.Get("/sandboxes/{id}", a.GetSandbox)

	nonExistentID := uuid.NewString()
	req := httptest.NewRequest(http.MethodGet, "/sandboxes/"+nonExistentID, nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode)
}

func TestExecCommand_MissingCmd(t *testing.T) {
	st := setupTestStore(t)
	ctx := context.Background()

	sbID := uuid.NewString()
	sb := &store.Sandbox{ID: sbID, ContainerID: "c-1", Status: store.StatusRunning}
	require.NoError(t, st.Save(ctx, sb))

	a := NewAPI(nil, st, 0, 0, nil)

	r := chi.NewRouter()
	r.Post("/sandboxes/{id}/exec", a.ExecCommand)

	body := bytes.NewBufferString(`{"cmd": []}`)
	req := httptest.NewRequest(http.MethodPost, "/sandboxes/"+sbID+"/exec", body)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
}

func TestGetSandbox_MalformedID(t *testing.T) {
	st := setupTestStore(t)
	a := NewAPI(nil, st, 0, 0, nil)

	r := chi.NewRouter()
	r.Get("/sandboxes/{id}", a.GetSandbox)

	req := httptest.NewRequest(http.MethodGet, "/sandboxes/not-a-uuid", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode, "malformed ID should be treated as not found, not a server error")
}

func TestPauseSandbox_WrongStatus(t *testing.T) {
	st := setupTestStore(t)
	ctx := context.Background()

	id := uuid.NewString()
	sb := &store.Sandbox{ID: id, ContainerID: "c-1", Status: store.StatusPaused}
	require.NoError(t, st.Save(ctx, sb))

	a := NewAPI(nil, st, 0, 0, nil)
	r := chi.NewRouter()
	r.Post("/sandboxes/{id}/pause", a.PauseSandbox)

	req := httptest.NewRequest(http.MethodPost, "/sandboxes/"+id+"/pause", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
}

func TestResumeSandbox_WrongStatus(t *testing.T) {
	st := setupTestStore(t)
	ctx := context.Background()

	id := uuid.NewString()
	sb := &store.Sandbox{ID: id, ContainerID: "c-1", Status: store.StatusRunning}
	require.NoError(t, st.Save(ctx, sb))

	a := NewAPI(nil, st, 0, 0, nil)
	r := chi.NewRouter()
	r.Post("/sandboxes/{id}/resume", a.ResumeSandbox)

	req := httptest.NewRequest(http.MethodPost, "/sandboxes/"+id+"/resume", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Result().StatusCode)
}

func TestResumeSandbox_NotFound(t *testing.T) {
	st := setupTestStore(t)
	a := NewAPI(nil, st, 0, 0, nil)

	id := uuid.NewString()
	r := chi.NewRouter()
	r.Post("/sandboxes/{id}/resume", a.ResumeSandbox)

	req := httptest.NewRequest(http.MethodPost, "/sandboxes/"+id+"/resume", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode)
}
