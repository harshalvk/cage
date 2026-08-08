package sandbox

import (
	"context"
	"fmt"
	"sync"
)

// BackendAdapter wraps SandboxManager (the tested, Docker-SDK-facing type)
// to satisfy backend.SandboxBackend's caller-provided-ID shape, plus the
// optional Pausable, ImageCleaner, and WarmAdopter capability interfaces.
//
// SandboxManager itself is untouched — this adapter only translates
// between "sandbox ID chosen by the caller" and "Docker container ID
// assigned by the Docker daemon," which SandboxManager's existing,
// already-tested methods have no reason to know about.
type BackendAdapter struct {
	sm *SandboxManager

	mu         sync.Mutex
	containers map[string]string // sandboxID -> docker containerID
}

func NewBackendAdapter(sm *SandboxManager) *BackendAdapter {
	return &BackendAdapter{sm: sm, containers: make(map[string]string)}
}

func (a *BackendAdapter) containerFor(sandboxID string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	containerID, ok := a.containers[sandboxID]
	if !ok {
		return "", fmt.Errorf("no container mapped for sandbox %s", sandboxID)
	}
	return containerID, nil
}

func (a *BackendAdapter) register(sandboxID, containerID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.containers[sandboxID] = containerID
}

func (a *BackendAdapter) unregister(sandboxID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.containers, sandboxID)
}

// --- backend.SandboxBackend ---

func (a *BackendAdapter) CreateSandbox(ctx context.Context, sandboxID, image string) error {
	containerID, err := a.sm.CreateSandbox(ctx, image)
	if err != nil {
		return err
	}
	a.register(sandboxID, containerID)
	return nil
}

func (a *BackendAdapter) KillSandbox(ctx context.Context, sandboxID string) error {
	containerID, err := a.containerFor(sandboxID)
	if err != nil {
		return err
	}
	a.unregister(sandboxID)
	return a.sm.KillSandbox(ctx, containerID)
}

func (a *BackendAdapter) ExecCommand(ctx context.Context, sandboxID string, cmd []string) (string, string, int, error) {
	containerID, err := a.containerFor(sandboxID)
	if err != nil {
		return "", "", 0, err
	}
	result, err := a.sm.ExecCommand(ctx, containerID, cmd)
	if err != nil {
		return "", "", 0, err
	}
	return result.Stdout, result.Stderr, result.ExitCode, nil
}

func (a *BackendAdapter) WriteFile(ctx context.Context, sandboxID, path, content string) error {
	containerID, err := a.containerFor(sandboxID)
	if err != nil {
		return err
	}
	return a.sm.WriteFile(ctx, containerID, path, []byte(content))
}

func (a *BackendAdapter) ReadFile(ctx context.Context, sandboxID, path string) (string, error) {
	containerID, err := a.containerFor(sandboxID)
	if err != nil {
		return "", err
	}
	content, err := a.sm.ReadFile(ctx, containerID, path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func (a *BackendAdapter) IsRunning(ctx context.Context, sandboxID string) (bool, error) {
	containerID, err := a.containerFor(sandboxID)
	if err != nil {
		// No mapping at all means it's definitely not running — not an error case.
		return false, nil
	}
	return a.sm.IsRunning(ctx, containerID)
}

// --- backend.Pausable ---

func (a *BackendAdapter) PauseSandbox(ctx context.Context, sandboxID string) (string, error) {
	containerID, err := a.containerFor(sandboxID)
	if err != nil {
		return "", err
	}
	imageID, err := a.sm.PauseSandbox(ctx, containerID)
	if err != nil {
		return "", err
	}
	// No live container while paused — remove the mapping until resume.
	a.unregister(sandboxID)
	return imageID, nil
}

func (a *BackendAdapter) ResumeSandbox(ctx context.Context, sandboxID, imageID string) error {
	containerID, err := a.sm.ResumeSandbox(ctx, imageID)
	if err != nil {
		return err
	}
	a.register(sandboxID, containerID)
	return nil
}

// --- backend.ImageCleaner ---

func (a *BackendAdapter) RemoveImage(ctx context.Context, imageID string) error {
	return a.sm.RemoveImage(ctx, imageID)
}

// --- backend.WarmAdopter ---

/*
AdoptWarmResource re-registers a warm pool's placeholder sandbox id under
the real sandbox id chosen by the api layer. the underlying container is
already running - no provisioning happens here, only a rename of the
internal id->container mapping
*/
func (a *BackendAdapter) AdoptWarmResource(ctx context.Context, sandboxID, placeholderID string) error {
	a.mu.Lock()
	containerID, ok := a.containers[placeholderID]
	if ok {
		delete(a.containers, placeholderID)
		a.containers[sandboxID] = containerID
	}
	a.mu.Unlock()

	if !ok {
		return fmt.Errorf("no container mapped for warm placeholder %s", placeholderID)
	}
	return nil
}
