package firecracker

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// RootfsManager handles the per-sandbox writable rootfs strategy: one
// read-only base image per template, copied fresh for every sandbox so
// concurrent VMs never share a writable file.
type RootfsManager struct {
	// baseDir holds one read-only rootfs file per template, named
	// "<template-slug>.ext4" (e.g. "base.ext4", "python-3.12.ext4").
	baseDir string
	// workDir is where per-sandbox writable copies are created and
	// cleaned up, named "<sandbox-id>.ext4".
	workDir string
}

func NewRootfsManager(baseDir, workDir string) (*RootfsManager, error) {
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create rootfs work dir: %w", err)
	}
	return &RootfsManager{baseDir: baseDir, workDir: workDir}, nil
}

// PrepareForSandbox copies the template's base rootfs into a fresh,
// writable, per-sandbox file. Returns the path to that new file.
func (r *RootfsManager) PrepareForSandbox(sandboxID, templateSlug string) (string, error) {
	basePath := filepath.Join(r.baseDir, templateSlug+".ext4")
	if _, err := os.Stat(basePath); err != nil {
		return "", fmt.Errorf("no base rootfs found for template %q: %w", templateSlug, err)
	}

	destPath := filepath.Join(r.workDir, sandboxID+".ext4")

	src, err := os.Open(basePath)
	if err != nil {
		return "", fmt.Errorf("failed to open base rootfs: %w", err)
	}
	defer func() { _ = src.Close() }()

	dst, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to create sandbox rootfs: %w", err)
	}
	defer func() { _ = dst.Close() }()

	if _, err := io.Copy(dst, src); err != nil {
		if rerr := os.Remove(destPath); rerr != nil && !os.IsNotExist(rerr) {
			// Don't leave a half-written file behind on failure — log if
			// even the cleanup itself failed, but the original copy error
			// is what we actually return to the caller.
			return "", fmt.Errorf("failed to copy rootfs: %w (cleanup also failed: %v)", err, rerr)
		}
		return "", fmt.Errorf("failed to copy rootfs: %w", err)
	}

	return destPath, nil
}

// Cleanup removes a sandbox's writable rootfs file. Safe to call even if
// the file doesn't exist (e.g. cleanup after a failed create).
func (r *RootfsManager) Cleanup(sandboxID string) error {
	path := filepath.Join(r.workDir, sandboxID+".ext4")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove sandbox rootfs: %w", err)
	}
	return nil
}
