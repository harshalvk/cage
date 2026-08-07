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
