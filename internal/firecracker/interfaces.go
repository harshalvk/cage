package firecracker

import (
	"context"
	"time"
)

// fcAPI is the subset of apiClient's methods FirecrackerManager depends on
// Defining this here (rather than testing against *apiClient directly)
// lets tests inject a fake that doesn't need a real firecracker process
// or KVM - same pattern as internal/sandbox.DockerClient
type fcAPI interface {
	setMachineConfig(ctx context.Context, vcpuCount, memSizeMiB int64) error
	setBootSource(ctx context.Context, kernelPath, bootArgs string) error
	setRootDrive(ctx context.Context, rootfsPath string) error
	setVsock(ctx context.Context, guestCID uint32, udsPath string) error
	startInstance(ctx context.Context) error
}

// fsVsock is the subset of vsockClient's methods FirecrackerManager depends
// on for guest communication
type fcVsock interface {
	send(req agentRequest) (*agentResponse, error)
	waitReady(timeout time.Duration) error
}

// processHandle abstracts a running os process - staisfied by *exec.cmd in
// production, and by a fake in tests that never actually spawns anything
type processHandle interface {
	Kill() error
	Wait() error
}
