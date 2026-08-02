// Package backend defines the interface every sandbox isolation backend
// (Docker, Firecracker, and any future backend) must satisfy, plus a set
// of named, optional capability interfaces for features not every backend
// supports.
//
// This mirrors a well-established Go standard library pattern: io.Writer
// is the baseline contract, but io.ReaderFrom, io.WriterTo etc. are
// optional capabilities some implementations provide and callers detect
// via a type assertion against a named interface (see http.Flusher,
// http.Hijacker for another example of the same pattern). We follow that
// convention here rather than scattering ad-hoc anonymous interface
// assertions through business logic.
package backend

import "context"

// SandboxBackend is the required baseline every isolation backend must
// implement. internal/api, internal/reaper, and internal/reconcile all
// depend on this interface, never on a concrete backend type.
type SandboxBackend interface {
	// CreateSandbox provisions a new sandbox with the given caller-chosen
	// ID, using templateRef to select the base image/rootfs. templateRef's
	// meaning is backend-specific (a Docker image string, a Firecracker
	// rootfs template slug, etc.) — internal/store.Template.Image carries
	// whichever value the active backend expects.
	CreateSandbox(ctx context.Context, sandboxID, templateRef string) error

	// KillSandbox permanently stops and removes a sandbox's resources.
	KillSandbox(ctx context.Context, sandboxID string) error

	// ExecCommand runs cmd inside the sandbox and returns its output.
	ExecCommand(ctx context.Context, sandboxID string, cmd []string) (stdout, stderr string, exitCode int, err error)

	// WriteFile writes plain-text content to path inside the sandbox.
	WriteFile(ctx context.Context, sandboxID, path, content string) error

	// ReadFile reads a file's content from inside the sandbox.
	ReadFile(ctx context.Context, sandboxID, path string) (string, error)

	// IsRunning reports whether the sandbox's underlying resource (container,
	// VM, etc.) actually exists and is running — used by reconcile to
	// detect drift between the database and the backend's real state.
	IsRunning(ctx context.Context, sandboxID string) (bool, error)
}

// Pausable is an optional capability: backends that can suspend a sandbox
// and later restore it implement this. Not every backend can — check with
// AsPausable before calling.
type Pausable interface {
	// PauseSandbox suspends the sandbox and returns an opaque backend-specific
	// reference (a Docker image ID, a Firecracker snapshot path, etc.)
	// needed to resume it later. The caller persists this reference.
	PauseSandbox(ctx context.Context, sandboxID string) (pauseRef string, err error)

	// ResumeSandbox restores a previously paused sandbox from pauseRef,
	// returning the sandbox's backend identity to use for subsequent calls
	// (which may differ from before pausing, e.g. a new container ID).
	ResumeSandbox(ctx context.Context, sandboxID, pauseRef string) error
}

// ImageCleaner is an optional capability: backends that produce a
// reclaimable resource when pausing (e.g. Docker's committed image)
// implement this so abandoned pauses can be garbage collected.
type ImageCleaner interface {
	RemoveImage(ctx context.Context, ref string) error
}

// WarmAdopter is an optional capability: backends that support a
// pre-warmed pool of ready-to-use resources implement this, letting the
// pool hand off an already-running resource under a caller-chosen sandbox
// ID without needing the pool package itself to know backend internals.
type WarmAdopter interface {
	// AdoptWarmResource registers nativeID (e.g. a Docker container ID
	// that's already running, created ahead of time by a warm pool) as
	// the backend identity for sandboxID, without provisioning anything new.
	AdoptWarmResource(ctx context.Context, sandboxID, nativeID string) error
}

// AsPausable checks whether b supports pause/resume.
func AsPausable(b SandboxBackend) (Pausable, bool) {
	p, ok := b.(Pausable)
	return p, ok
}

// AsImageCleaner checks whether b supports removing a pause reference's
// underlying resource.
func AsImageCleaner(b SandboxBackend) (ImageCleaner, bool) {
	c, ok := b.(ImageCleaner)
	return c, ok
}

// AsWarmAdopter checks whether b supports adopting pre-warmed resources.
func AsWarmAdopter(b SandboxBackend) (WarmAdopter, bool) {
	w, ok := b.(WarmAdopter)
	return w, ok
}
