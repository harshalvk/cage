package firecracker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- fakes ---

type fakeProcess struct {
	killed  bool
	killErr error
}

func (f *fakeProcess) Kill() error { f.killed = true; return f.killErr }
func (f *fakeProcess) Wait() error { return nil }

type fakeAPI struct {
	failAt string // which method should return an error, "" for none
}

func (f *fakeAPI) setMachineConfig(ctx context.Context, vcpu, mem int64) error {
	if f.failAt == "machine-config" {
		return errors.New("boom")
	}
	return nil
}
func (f *fakeAPI) setBootSource(ctx context.Context, kernel, args string) error {
	if f.failAt == "boot-source" {
		return errors.New("boom")
	}
	return nil
}
func (f *fakeAPI) setRootDrive(ctx context.Context, path string) error {
	if f.failAt == "root-drive" {
		return errors.New("boom")
	}
	return nil
}
func (f *fakeAPI) setVsock(ctx context.Context, cid uint32, path string) error {
	if f.failAt == "vsock" {
		return errors.New("boom")
	}
	return nil
}
func (f *fakeAPI) startInstance(ctx context.Context) error {
	if f.failAt == "start-instance" {
		return errors.New("boom")
	}
	return nil
}

func (f *fakeAPI) pauseVM(ctx context.Context) error {
	if f.failAt == "pause-vm" {
		return errors.New("boom")
	}
	return nil
}

func (f *fakeAPI) createSnapshot(ctx context.Context, snapshotPath, memFilePath string) error {
	if f.failAt == "create-snapshot" {
		return errors.New("boom")
	}
	// Simulate real files existing on disk, since RemoveImage/tests may check for them.
	_ = os.WriteFile(snapshotPath, []byte("fake-snapshot"), 0644)
	_ = os.WriteFile(memFilePath, []byte("fake-memory"), 0644)
	return nil
}

func (f *fakeAPI) loadSnapshot(ctx context.Context, snapshotPath, memFilePath string) error {
	if f.failAt == "load-snapshot" {
		return errors.New("boom")
	}
	return nil
}

type fakeVsock struct {
	readyErr error
}

func (f *fakeVsock) send(req agentRequest) (*agentResponse, error) {
	switch req.Type {
	case "exec":
		return &agentResponse{Stdout: "fake output\n", ExitCode: 0}, nil
	case "write_file", "read_file":
		return &agentResponse{Content: "fake content"}, nil
	}
	return &agentResponse{}, nil
}
func (f *fakeVsock) waitReady(timeout time.Duration) error { return f.readyErr }

// --- test setup helpers ---

func newTestManager(t *testing.T, apiFailAt string, vsockErr error) *FirecrackerManager {
	t.Helper()

	baseDir := t.TempDir()
	runDir := t.TempDir()

	// Create a fake base rootfs for the "base" template so PrepareForSandbox succeeds.
	require.NoError(t, os.WriteFile(filepath.Join(baseDir, "base.ext4"), []byte("fake-rootfs"), 0644))

	rootfs, err := NewRootfsManager(baseDir, filepath.Join(runDir, "rootfs"))
	require.NoError(t, err)

	spawnProcess := func(ctx context.Context, bin, apiSocket string) (processHandle, error) {
		// Simulate the api socket "appearing" the way a real firecracker
		// process would, since waitForSocket polls for the file on disk.
		require.NoError(t, os.WriteFile(apiSocket, []byte{}, 0644))
		return &fakeProcess{}, nil
	}

	return NewFirecrackerManagerForTest(
		Config{RunDir: runDir, BootTimeout: 2 * time.Second},
		rootfs,
		spawnProcess,
		func(socketPath string) fcAPI { return &fakeAPI{failAt: apiFailAt} },
		func(udsPath string) fcVsock { return &fakeVsock{readyErr: vsockErr} },
	)
}

// --- tests ---

func TestCreateSandbox_Success(t *testing.T) {
	mgr := newTestManager(t, "", nil)

	err := mgr.CreateSandbox(context.Background(), "sb-1", "base")
	require.NoError(t, err)

	running, err := mgr.IsRunning(context.Background(), "sb-1")
	require.NoError(t, err)
	assert.True(t, running)
}

func TestCreateSandbox_UnknownTemplate(t *testing.T) {
	mgr := newTestManager(t, "", nil)

	err := mgr.CreateSandbox(context.Background(), "sb-1", "does-not-exist")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rootfs prepare failed")
}

func TestCreateSandbox_APIFailureRollsBack(t *testing.T) {
	tests := []string{"machine-config", "boot-source", "root-drive", "vsock", "start-instance"}

	for _, failAt := range tests {
		t.Run(failAt, func(t *testing.T) {
			mgr := newTestManager(t, failAt, nil)

			err := mgr.CreateSandbox(context.Background(), "sb-1", "base")
			require.Error(t, err)

			// The sandbox should NOT be tracked as running after a rollback.
			running, _ := mgr.IsRunning(context.Background(), "sb-1")
			assert.False(t, running, "sandbox should not be running after a %s failure", failAt)

			// The per-sandbox rootfs copy should have been cleaned up, not leaked.
			_, statErr := os.Stat(filepath.Join(mgr.rootfs.workDir, "sb-1.ext4"))
			assert.True(t, os.IsNotExist(statErr), "rootfs should be cleaned up after a %s failure", failAt)
		})
	}
}

func TestCreateSandbox_GuestAgentNeverReady(t *testing.T) {
	mgr := newTestManager(t, "", errors.New("agent never responded"))

	err := mgr.CreateSandbox(context.Background(), "sb-1", "base")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "guest agent never became ready")

	running, _ := mgr.IsRunning(context.Background(), "sb-1")
	assert.False(t, running)
}

func TestKillSandbox_Success(t *testing.T) {
	mgr := newTestManager(t, "", nil)
	require.NoError(t, mgr.CreateSandbox(context.Background(), "sb-1", "base"))

	err := mgr.KillSandbox(context.Background(), "sb-1")
	require.NoError(t, err)

	running, _ := mgr.IsRunning(context.Background(), "sb-1")
	assert.False(t, running)

	_, statErr := os.Stat(filepath.Join(mgr.rootfs.workDir, "sb-1.ext4"))
	assert.True(t, os.IsNotExist(statErr), "rootfs should be removed after kill")
}

func TestKillSandbox_UnknownSandbox(t *testing.T) {
	mgr := newTestManager(t, "", nil)

	err := mgr.KillSandbox(context.Background(), "does-not-exist")
	assert.Error(t, err)
}

func TestExecCommand_Success(t *testing.T) {
	mgr := newTestManager(t, "", nil)
	require.NoError(t, mgr.CreateSandbox(context.Background(), "sb-1", "base"))

	stdout, _, exitCode, err := mgr.ExecCommand(context.Background(), "sb-1", []string{"echo", "hi"})
	require.NoError(t, err)
	assert.Equal(t, "fake output\n", stdout)
	assert.Equal(t, 0, exitCode)
}

func TestExecCommand_UnknownSandbox(t *testing.T) {
	mgr := newTestManager(t, "", nil)

	_, _, _, err := mgr.ExecCommand(context.Background(), "does-not-exist", []string{"echo", "hi"})
	assert.Error(t, err)
}

func TestWriteFileAndReadFile_Success(t *testing.T) {
	mgr := newTestManager(t, "", nil)
	require.NoError(t, mgr.CreateSandbox(context.Background(), "sb-1", "base"))

	err := mgr.WriteFile(context.Background(), "sb-1", "/tmp/hello.txt", "hello")
	require.NoError(t, err)

	content, err := mgr.ReadFile(context.Background(), "sb-1", "/tmp/hello.txt")
	require.NoError(t, err)
	assert.Equal(t, "fake content", content)
}

func TestIsRunning_UnknownSandboxReturnsFalseNotError(t *testing.T) {
	mgr := newTestManager(t, "", nil)

	running, err := mgr.IsRunning(context.Background(), "never-created")
	require.NoError(t, err)
	assert.False(t, running)
}

func TestPauseSandbox_Success(t *testing.T) {
	mgr := newTestManager(t, "", nil)
	require.NoError(t, mgr.CreateSandbox(context.Background(), "sb-1", "base"))

	pauseRef, err := mgr.PauseSandbox(context.Background(), "sb-1")
	require.NoError(t, err)
	assert.NotEmpty(t, pauseRef)

	// The sandbox should no longer be tracked as a live running instance.
	running, _ := mgr.IsRunning(context.Background(), "sb-1")
	assert.False(t, running)

	// The manifest, snapshot, and memory files should genuinely exist on disk.
	assert.FileExists(t, filepath.Join(pauseRef, "manifest.json"))
	assert.FileExists(t, filepath.Join(pauseRef, "snapshot.file"))
	assert.FileExists(t, filepath.Join(pauseRef, "memory.file"))

	// The rootfs must NOT have been deleted — the snapshot still references it.
	rootfsPath := filepath.Join(mgr.rootfs.workDir, "sb-1.ext4")
	assert.FileExists(t, rootfsPath)
}

func TestPauseSandbox_UnknownSandbox(t *testing.T) {
	mgr := newTestManager(t, "", nil)

	_, err := mgr.PauseSandbox(context.Background(), "does-not-exist")
	assert.Error(t, err)
}

func TestPauseSandbox_SnapshotFailureCleansUpPauseDir(t *testing.T) {
	mgr := newTestManager(t, "create-snapshot", nil)
	require.NoError(t, mgr.CreateSandbox(context.Background(), "sb-1", "base"))

	_, err := mgr.PauseSandbox(context.Background(), "sb-1")
	require.Error(t, err)

	pauseDir := filepath.Join(mgr.cfg.RunDir, "pauses", "sb-1")
	_, statErr := os.Stat(pauseDir)
	assert.True(t, os.IsNotExist(statErr), "pause dir should be cleaned up after a snapshot failure")
}

func TestResumeSandbox_Success(t *testing.T) {
	mgr := newTestManager(t, "", nil)
	require.NoError(t, mgr.CreateSandbox(context.Background(), "sb-1", "base"))

	pauseRef, err := mgr.PauseSandbox(context.Background(), "sb-1")
	require.NoError(t, err)

	err = mgr.ResumeSandbox(context.Background(), "sb-1", pauseRef)
	require.NoError(t, err)

	running, err := mgr.IsRunning(context.Background(), "sb-1")
	require.NoError(t, err)
	assert.True(t, running)

	// Confirm it's genuinely usable again post-resume.
	stdout, _, _, err := mgr.ExecCommand(context.Background(), "sb-1", []string{"echo", "hi"})
	require.NoError(t, err)
	assert.Equal(t, "fake output\n", stdout)
}

func TestResumeSandbox_MissingManifest(t *testing.T) {
	mgr := newTestManager(t, "", nil)

	err := mgr.ResumeSandbox(context.Background(), "sb-1", "/nonexistent/pause/dir")
	assert.Error(t, err)
}

func TestResumeSandbox_MissingRootfs(t *testing.T) {
	mgr := newTestManager(t, "", nil)
	require.NoError(t, mgr.CreateSandbox(context.Background(), "sb-1", "base"))

	pauseRef, err := mgr.PauseSandbox(context.Background(), "sb-1")
	require.NoError(t, err)

	// Simulate the rootfs having been deleted out from under the pause.
	require.NoError(t, os.Remove(filepath.Join(mgr.rootfs.workDir, "sb-1.ext4")))

	err = mgr.ResumeSandbox(context.Background(), "sb-1", pauseRef)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rootfs referenced by snapshot is missing")
}

func TestRemoveImage_CleansUpSnapshotAndRootfs(t *testing.T) {
	mgr := newTestManager(t, "", nil)
	require.NoError(t, mgr.CreateSandbox(context.Background(), "sb-1", "base"))

	pauseRef, err := mgr.PauseSandbox(context.Background(), "sb-1")
	require.NoError(t, err)

	rootfsPath := filepath.Join(mgr.rootfs.workDir, "sb-1.ext4")
	assert.FileExists(t, rootfsPath) // sanity check before cleanup

	err = mgr.RemoveImage(context.Background(), pauseRef)
	require.NoError(t, err)

	_, statErr := os.Stat(pauseRef)
	assert.True(t, os.IsNotExist(statErr), "pause dir should be removed")

	_, statErr = os.Stat(rootfsPath)
	assert.True(t, os.IsNotExist(statErr), "rootfs should be removed")
}

func TestRemoveImage_NonexistentRefIsNotAnError(t *testing.T) {
	mgr := newTestManager(t, "", nil)

	err := mgr.RemoveImage(context.Background(), filepath.Join(mgr.cfg.RunDir, "pauses", "never-existed"))
	assert.NoError(t, err, "removing an already-gone pause ref should be a no-op, not an error")
}
