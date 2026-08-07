package firecracker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

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
