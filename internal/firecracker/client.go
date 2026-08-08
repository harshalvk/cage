package firecracker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// apiClient talks to a single Firecracker instance's API socket. Every VM
// gets its own socket file, so every VM gets its own apiClient.
type apiClient struct {
	httpClient *http.Client
}

func newAPIClient(socketPath string) *apiClient {
	return &apiClient{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", socketPath)
				},
			},
		},
	}
}

func (c *apiClient) put(ctx context.Context, path string, body any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://unix"+path, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("firecracker api request failed: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			// Non-fatal — we've already read what we need by this point.
			_ = cerr
		}
	}()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("firecracker api error on %s (status %d): %s", path, resp.StatusCode, respBody)
	}
	return nil
}

func (c *apiClient) setBootSource(ctx context.Context, kernelPath, bootArgs string) error {
	return c.put(ctx, "/boot-source", map[string]string{
		"kernel_image_path": kernelPath,
		"boot_args":         bootArgs,
	})
}

func (c *apiClient) setRootDrive(ctx context.Context, rootfsPath string) error {
	return c.put(ctx, "/drives/rootfs", map[string]any{
		"drive_id":       "rootfs",
		"path_on_host":   rootfsPath,
		"is_root_device": true,
		"is_read_only":   false,
	})
}

func (c *apiClient) setVsock(ctx context.Context, guestCID uint32, udsPath string) error {
	return c.put(ctx, "/vsock", map[string]any{
		"guest_cid": guestCID,
		"uds_path":  udsPath,
	})
}

func (c *apiClient) setMachineConfig(ctx context.Context, vcpuCount, memSizeMiB int64) error {
	return c.put(ctx, "/machine-config", map[string]int64{
		"vcpu_count":   vcpuCount,
		"mem_size_mib": memSizeMiB,
	})
}

func (c *apiClient) patch(ctx context.Context, path string, body any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, "http://unix"+path, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("firecracker api request failed: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			_ = cerr
		}
	}()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("firecracker api error on %s (status %d): %s", path, resp.StatusCode, respBody)
	}
	return nil
}

// pauseVM transitions a running VM to the Paused state - required before
// a snapshot can be taken
func (c *apiClient) pauseVM(ctx context.Context) error {
	return c.patch(ctx, "/vm", map[string]string{"state": "Paused"})
}

// createSnapshot captures the paused VM's full state (device/cpu state to
// snapshotPath, memory contents to memFilePath). the vm must already be
// paused via pauseVM
func (c *apiClient) createSnapshot(ctx context.Context, snapshotPath, memFilePath string) error {
	return c.put(ctx, "/snapshot/create", map[string]string{
		"snapshot_type": "Full",
		"snapshot_path": snapshotPath,
		"mem_file_path": memFilePath,
	})
}

// loadSnapshot restores a VM from a previously created snapshot into a
// freshly spawned (but not yet booted) Firecracker process, resuming
// execution immediately
func (c *apiClient) loadSnapshot(ctx context.Context, snapshotPath, memFilePath string) error {
	return c.put(ctx, "/snapshot/load", map[string]any{
		"snapshot_path": snapshotPath,
		"mem_file_path": memFilePath,
		"resume_vm":     true,
	})
}

func (c *apiClient) startInstance(ctx context.Context) error {
	return c.put(ctx, "/actions", map[string]string{"action_type": "InstanceStart"})
}
