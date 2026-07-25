package cageclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cageclient "github.com/harshalvk/cage/sdk/go"
)

func TestCreateSandbox_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/sandboxes", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(cageclient.Sandbox{
			ID:     "sb-123",
			Status: cageclient.StatusRunning,
		})
	}))
	defer server.Close()

	client := cageclient.New(server.URL, "test-key")
	sb, err := client.CreateSandbox(context.Background(), cageclient.CreateSandboxOptions{Template: "base"})

	require.NoError(t, err)
	assert.Equal(t, "sb-123", sb.ID)
	assert.Equal(t, cageclient.StatusRunning, sb.Status)
}

func TestCreateSandbox_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unknown template: bad-slug", http.StatusBadRequest)
	}))
	defer server.Close()

	client := cageclient.New(server.URL, "test-key")
	_, err := client.CreateSandbox(context.Background(), cageclient.CreateSandboxOptions{Template: "bad-slug"})

	require.Error(t, err)
	var apiErr *cageclient.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
}

func TestExec_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/sandboxes/sb-123/exec", r.URL.Path)

		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		cmd, _ := body["cmd"].([]interface{})
		assert.Equal(t, "echo", cmd[0])

		json.NewEncoder(w).Encode(cageclient.ExecResult{
			Stdout:   "hello\n",
			ExitCode: 0,
		})
	}))
	defer server.Close()

	client := cageclient.New(server.URL, "test-key")
	result, err := client.Exec(context.Background(), "sb-123", []string{"echo", "hello"})

	require.NoError(t, err)
	assert.Equal(t, "hello\n", result.Stdout)
	assert.Equal(t, 0, result.ExitCode)
}

func TestReadFile_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/sandboxes/sb-123/files", r.URL.Path)
		assert.Equal(t, "/tmp/hello.txt", r.URL.Query().Get("path"))

		w.Write([]byte("file contents"))
	}))
	defer server.Close()

	client := cageclient.New(server.URL, "test-key")
	content, err := client.ReadFile(context.Background(), "sb-123", "/tmp/hello.txt")

	require.NoError(t, err)
	assert.Equal(t, "file contents", string(content))
}

func TestListSandboxes_Empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]cageclient.Sandbox{})
	}))
	defer server.Close()

	client := cageclient.New(server.URL, "test-key")
	sandboxes, err := client.ListSandboxes(context.Background())

	require.NoError(t, err)
	assert.Empty(t, sandboxes)
}
