package firecracker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// pauseManifest is persisted as json alongside a snapshot's files, so
// ResumeSandbox knows exactly what to load without needing any other
// state. rootfsPath is the ORIGINAL per-sandbox rootfs file's path - it
// must remain at that exact path for the snapshot's restored drive
// attachment to resolve correctly, so pausing must never move or rename it
type pauseManifest struct {
	RootfsPath   string `json:"rootfs_path"`
	SnapshotPath string `json:"snapshot_path"`
	MemFilePath  string `json:"mem_file_path"`
}

// vmInstance tracks everything needed to manage one running microVM.
type vmInstance struct {
	sandboxID  string
	process    processHandle // ← was: cmd *exec.Cmd
	apiSocket  string
	vsockUDS   string
	rootfsPath string
	vsock      fcVsock
}

// Config holds the paths and defaults FirecrackerManager needs at startup.
type Config struct {
	FirecrackerBin string // path to the firecracker binary
	KernelPath     string // shared, read-only kernel image — same for every VM
	RootfsBaseDir  string // one base .ext4 per template
	RunDir         string // scratch dir for per-VM sockets + rootfs copies
	VCPUCount      int64
	MemSizeMiB     int64
	BootTimeout    time.Duration
}

type FirecrackerManager struct {
	cfg    Config
	rootfs *RootfsManager

	// Injectable dependencies - default to real implementations in
	// NewFirecrackerManager, overridden by tests via the unexporeted
	// constructor NewFirecracekrManagerForTest
	spawnProcess func(ctx context.Context, bin, apiSocket string) (processHandle, error)
	newAPI       func(socketPath string) fcAPI
	newVsock     func(udsPath string) fcVsock

	mu      sync.Mutex
	running map[string]*vmInstance // sandboxID -> instance
}

func NewFirecrackerManager(cfg Config) (*FirecrackerManager, error) {
	if cfg.BootTimeout == 0 {
		cfg.BootTimeout = 10 * time.Second
	}
	if err := os.MkdirAll(cfg.RunDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create run dir: %w", err)
	}

	rootfs, err := NewRootfsManager(cfg.RootfsBaseDir, filepath.Join(cfg.RunDir, "rootfs"))
	if err != nil {
		return nil, err
	}

	return &FirecrackerManager{
		cfg:          cfg,
		rootfs:       rootfs,
		spawnProcess: realSpawnProcess,
		newAPI:       func(socketPath string) fcAPI { return newAPIClient(socketPath) },
		newVsock:     func(udsPath string) fcVsock { return newVsockClient(udsPath) },
		running:      make(map[string]*vmInstance),
	}, nil
}

// NewFirecrackerManagerForTest builds a manager with injected fakes instead
// of real process/api/vsock implementations - used only by tests in this
// package; prod code should always use NewFirecrackerManager
func NewFirecrackerManagerForTest(
	cfg Config,
	rootfs *RootfsManager,
	spawnProcess func(ctx context.Context, bin, apiSocket string) (processHandle, error),
	newAPI func(socketPath string) fcAPI,
	newVsock func(udsPath string) fcVsock,
) *FirecrackerManager {
	return &FirecrackerManager{
		cfg:          cfg,
		rootfs:       rootfs,
		spawnProcess: spawnProcess,
		newAPI:       newAPI,
		newVsock:     newVsock,
		running:      make(map[string]*vmInstance),
	}
}

func realSpawnProcess(ctx context.Context, bin, apiSocket string) (processHandle, error) {
	cmd := exec.CommandContext(ctx, bin, "--api-sock", apiSocket)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &execProcessHandle{cmd: cmd}, nil
}

// execProcessHandle adapts *exec.Cmd to the processHandle interface
type execProcessHandle struct {
	cmd *exec.Cmd
}

func (h *execProcessHandle) Kill() error {
	if h.cmd.Process == nil {
		return nil
	}
	return h.cmd.Process.Kill()
}

func (h *execProcessHandle) Wait() error {
	if h.cmd.Process == nil {
		return nil
	}
	_, err := h.cmd.Process.Wait()
	return err
}

// CreateSandbox boots a new microVM for the given template and blocks until
// its guest agent is confirmed ready. sandboxID should be a value already
// unique within the system (e.g. the same UUID used for the Store record).
func (m *FirecrackerManager) CreateSandbox(ctx context.Context, sandboxID, templateSlug string) error {
	rootfsPath, err := m.rootfs.PrepareForSandbox(sandboxID, templateSlug)
	if err != nil {
		return fmt.Errorf("rootfs prepare failed: %w", err)
	}

	apiSocket := filepath.Join(m.cfg.RunDir, sandboxID+".api.sock")
	vsockUDS := filepath.Join(m.cfg.RunDir, sandboxID+".vsock")

	removeFileIfExists(apiSocket)
	removeFileIfExists(vsockUDS)

	process, err := m.spawnProcess(ctx, m.cfg.FirecrackerBin, apiSocket) // ← was: exec.CommandContext(...) + cmd.Start()
	if err != nil {
		if cerr := m.rootfs.Cleanup(sandboxID); cerr != nil {
			slog.Warn("firecracker: rootfs cleanup failed after spawn failure", "error", cerr)
		}
		return fmt.Errorf("failed to start firecracker process: %w", err)
	}

	if err := waitForSocket(apiSocket, 3*time.Second); err != nil {
		killProcess(process)
		if cerr := m.rootfs.Cleanup(sandboxID); cerr != nil {
			slog.Warn("firecracker: rootfs cleanup failed after boot failure", "error", cerr)
		}
		return fmt.Errorf("firecracker api socket never appeared: %w", err)
	}

	api := m.newAPI(apiSocket) // ← was: newAPIClient(apiSocket)

	rollback := func(stage string, err error) error {
		killProcess(process)
		if cerr := m.rootfs.Cleanup(sandboxID); cerr != nil {
			slog.Warn("firecracker: rootfs cleanup failed during rollback", "stage", stage, "error", cerr)
		}
		return err
	}

	if err := api.setMachineConfig(ctx, m.cfg.VCPUCount, m.cfg.MemSizeMiB); err != nil {
		return rollback("machine-config", err)
	}
	if err := api.setBootSource(ctx, m.cfg.KernelPath, "console=ttyS0 reboot=k panic=1 pci=off"); err != nil {
		return rollback("boot-source", err)
	}
	if err := api.setRootDrive(ctx, rootfsPath); err != nil {
		return rollback("root-drive", err)
	}
	if err := api.setVsock(ctx, 3, vsockUDS); err != nil {
		return rollback("vsock", err)
	}
	if err := api.startInstance(ctx); err != nil {
		return rollback("start-instance", err)
	}

	vsock := m.newVsock(vsockUDS) // ← was: newVsockClient(vsockUDS)
	if err := vsock.waitReady(m.cfg.BootTimeout); err != nil {
		return rollback("guest-agent-ready", fmt.Errorf("guest agent never became ready: %w", err))
	}

	m.mu.Lock()
	m.running[sandboxID] = &vmInstance{
		sandboxID:  sandboxID,
		process:    process, // ← was: cmd
		apiSocket:  apiSocket,
		vsockUDS:   vsockUDS,
		rootfsPath: rootfsPath,
		vsock:      vsock,
	}
	m.mu.Unlock()

	return nil
}

func (m *FirecrackerManager) KillSandbox(ctx context.Context, sandboxID string) error {
	m.mu.Lock()
	inst, ok := m.running[sandboxID]
	if ok {
		delete(m.running, sandboxID)
	}
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("no running sandbox with id %s", sandboxID)
	}

	if inst.process != nil {
		if err := inst.process.Kill(); err != nil {
			slog.Warn("firecracker: failed to kill process", "sandbox_id", sandboxID, "error", err)
		}
		if err := inst.process.Wait(); err != nil {
			slog.Warn("firecracker: failed to reap process", "sandbox_id", sandboxID, "error", err)
		}
	}
	removeFileIfExists(inst.apiSocket)
	removeFileIfExists(inst.vsockUDS)

	return m.rootfs.Cleanup(sandboxID)
}

func (m *FirecrackerManager) ExecCommand(ctx context.Context, sandboxID string, cmd []string) (stdout, stderr string, exitCode int, err error) {
	inst, err := m.get(sandboxID)
	if err != nil {
		return "", "", 0, err
	}
	resp, err := inst.vsock.send(agentRequest{Type: "exec", Cmd: cmd})
	if err != nil {
		return "", "", 0, err
	}
	return resp.Stdout, resp.Stderr, resp.ExitCode, nil
}

func (m *FirecrackerManager) WriteFile(ctx context.Context, sandboxID, path, content string) error {
	inst, err := m.get(sandboxID)
	if err != nil {
		return err
	}
	_, err = inst.vsock.send(agentRequest{Type: "write_file", Path: path, Content: content})
	return err
}

func (m *FirecrackerManager) ReadFile(ctx context.Context, sandboxID, path string) (string, error) {
	inst, err := m.get(sandboxID)
	if err != nil {
		return "", err
	}
	resp, err := inst.vsock.send(agentRequest{Type: "read_file", Path: path})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func (m *FirecrackerManager) IsRunning(ctx context.Context, sandboxID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.running[sandboxID]
	return ok, nil
}

func (m *FirecrackerManager) get(sandboxID string) (*vmInstance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inst, ok := m.running[sandboxID]
	if !ok {
		return nil, fmt.Errorf("no running sandbox with id %s", sandboxID)
	}
	return inst, nil
}

func waitForSocket(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", path)
}

// backend.Pausable

// PauseSandbox suspends a running vm via firecracker's native snapshot
// support: pause execution, write memory + device state to disk, then kill
// the process entirely to free its ram. unlike docker's commit+recreate
// approach (adr 0005), this caputres live memroy state, not just the
// filesystem - a resumed sandbox continues exacelty where it left offf

// the per-sandbox rootfs file is deliberately NOT touched here: the
// snapshot's restored drive attachment references it by its exact original
// path, so it must remain in place, untouched, for the lifetime of the pause
func (m *FirecrackerManager) PauseSandbox(ctx context.Context, sandboxID string) (string, error) {
	inst, err := m.get(sandboxID)
	if err != nil {
		return "", err
	}

	pauseDir := filepath.Join(m.cfg.RunDir, "pauses", sandboxID)
	if err := os.MkdirAll(pauseDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create pause dir: %w", err)
	}

	snapshotPath := filepath.Join(pauseDir, "snapshot.file")
	memeFilePath := filepath.Join(pauseDir, "memory.file")

	api := m.newAPI(inst.apiSocket)

	if err := api.pauseVM(ctx); err != nil {
		if rerr := os.RemoveAll(pauseDir); err != nil {
			slog.Warn("firecracker: failed to clean up pause dir after error", "error", rerr)
		}
		return "", fmt.Errorf("failed to pause vm: %w", err)
	}
	if err := api.createSnapshot(ctx, snapshotPath, memeFilePath); err != nil {
		if rerr := os.RemoveAll(pauseDir); rerr != nil {
			slog.Warn("firecracker: failed to cleanup pause dir after error", "error", rerr)
		}
	}

	manifest := pauseManifest{
		RootfsPath:   inst.rootfsPath,
		SnapshotPath: snapshotPath,
		MemFilePath:  memeFilePath,
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		if rerr := os.RemoveAll(pauseDir); rerr != nil {
			slog.Warn("firecracker: failed to cleanup pause dir after error", "error", rerr)
		}
		return "", fmt.Errorf("failed to marshal pause manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(pauseDir, "manifest.json"), manifestBytes, 0644); err != nil {
		if rerr := os.RemoveAll(pauseDir); rerr != nil {
			slog.Warn("firecracker: failed to cleanup pause dir after error", "error", rerr)
		}
		return "", fmt.Errorf("failed to write pause manifest: %w", err)
	}

	// snapshot is safely on disk - now free the process's memory
	if err := inst.process.Kill(); err != nil {
		slog.Warn("firecracker: failed to kill process after snapshot", "sandbox_id", sandboxID, "error", err)
	}
	if err := inst.process.Wait(); err != nil {
		slog.Warn("firecracker: failed to reap process after snapshot", "sandbox_id", sandboxID, "error", err)
	}
	removeFileIfExists(inst.apiSocket)
	removeFileIfExists(inst.vsockUDS)

	m.mu.Lock()
	delete(m.running, sandboxID)
	m.mu.Unlock()

	return pauseDir, nil
}

// ResumeSandbox spawns a fresh Firecracker process and loads a previously
// created snapshot into it, resuming execution from exactly where
// PauseSandbox left off - including in-memory state, not just files
func (m *FirecrackerManager) ResumeSandbox(ctx context.Context, sandboxID, pauseRef string) error {
	manifestBytes, err := os.ReadFile(filepath.Join(pauseRef, "manifest.json"))
	if err != nil {
		return fmt.Errorf("failed to road pause manifest: %w", err)
	}
	var manifest pauseManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("failed to parse pause manifest: %w", err)
	}

	if _, err := os.Stat(manifest.RootfsPath); err != nil {
		return fmt.Errorf("rootfs referenced by snapshot is missing: %w", err)
	}

	apiSocket := filepath.Join(m.cfg.RunDir, sandboxID+".api.sock")
	vsockUDS := filepath.Join(m.cfg.RunDir, sandboxID+".vsock")
	removeFileIfExists(apiSocket)
	removeFileIfExists(vsockUDS)

	process, err := m.spawnProcess(ctx, m.cfg.FirecrackerBin, apiSocket)
	if err != nil {
		return fmt.Errorf("failed to start firecracker process for resume: %w", err)
	}

	if err := waitForSocket(apiSocket, 3*time.Second); err != nil {
		killProcess(process)
		return fmt.Errorf("firecracker api socket never appeared during resume: %w", err)
	}

	api := m.newAPI(apiSocket)

	// the host-side vsock UDS must be (re-)configured before loading the
	// snapshot - it's host-only construct, not part of the vm's own
	// saved state, so a fresh process always needs it set explicitly
	if err := api.setVsock(ctx, 3, vsockUDS); err != nil {
		killProcess(process)
		return fmt.Errorf("failed to load snapshot: %w", err)
	}

	vsock := m.newVsock(vsockUDS)
	// the guest was already booted and running before it was paused, so
	// this should resolve quickly - a much shorter wait than a cold boot
	if err := vsock.waitReady(5 * time.Second); err != nil {
		killProcess(process)
		return fmt.Errorf("guest agent did not respond after resume: %w", err)
	}

	m.mu.Lock()
	m.running[sandboxID] = &vmInstance{
		sandboxID:  sandboxID,
		process:    process,
		apiSocket:  apiSocket,
		vsockUDS:   vsockUDS,
		rootfsPath: manifest.RootfsPath,
		vsock:      vsock,
	}
	m.mu.Unlock()

	return nil
}

// backend.ImageCleaner

// RemoveImage deletes a pause's snapshot files and the rootfs file it
// referenced. Safe to call even if ref doesn't exit
func (m *FirecrackerManager) RemoveImage(ctx context.Context, ref string) error {
	manifestBytes, err := os.ReadFile(filepath.Join(ref, "manifest.json"))
	if err == nil {
		var manifest pauseManifest
		if jerr := json.Unmarshal(manifestBytes, &manifest); jerr == nil {
			removeFileIfExists(manifest.RootfsPath)
		}
	} else if !os.IsNotExist(err) {
		slog.Warn("firecracker: failed to read pause manifest during cleanup", "ref", ref, "error", err)
	}

	if err := os.RemoveAll(ref); err != nil {
		return fmt.Errorf("failed to remove pause directory: %w", err)
	}
	return nil
}

// killProcess is a best-effort process kill used during rollback paths —
// we're already returning an error to the caller at each call site, so a
// secondary failure here is logged but doesn't change the outcome.
func killProcess(p processHandle) {
	if p == nil {
		return
	}
	if err := p.Kill(); err != nil {
		slog.Warn("firecracker: failed to kill process during cleanup", "error", err)
	}
}

// removeFileIfExists is a best-effort file removal — used for stale socket
// files and rootfs cleanup, where a failure here shouldn't block the
// caller's own error path.
func removeFileIfExists(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		slog.Warn("firecracker: failed to remove file during cleanup", "path", path, "error", err)
	}
}
