# 0015. Implement Firecracker pause/resume via native VM snapshots

Date: 2026-07-24

## Status

Accepted

## Context

ADR 0005 chose Docker's commit+recreate approach for pause/resume specifically because Docker has no native "suspend and later restore full VM state" primitive — a container's memory contents cannot be captured, only its filesystem via `docker commit`.

Firecracker, unlike Docker, has first-class support for this: a running microVM can be paused (`PATCH /vm` → `Paused`), then its complete state — CPU registers, device state, and full memory contents — can be written to disk via `PUT /snapshot/create`. A new Firecracker process can later load that snapshot (`PUT /snapshot/load`) and resume execution from the exact point it was paused, including in-memory program state that Docker's approach never captured at all.

This meant `backend.Pausable` and `backend.ImageCleaner` (introduced alongside the backend abstraction) could be implemented a second time, on a fundamentally different mechanism, without changing their interface shape — validating that the capability-interface design generalizes rather than being accidentally Docker-shaped.

One constraint drove the pause reference's on-disk layout: Firecracker's snapshot stores a drive's `path_on_host` as an absolute path captured at snapshot time, and does not re-specify it on load — the backing rootfs file must still exist at that *exact* path when the snapshot is restored. This ruled out moving or renaming the per-sandbox rootfs file during a pause, unlike an initially simpler design that would have consolidated all pause-related files into one self-contained directory.

## Decision

Implement pause as: pause the VM, snapshot device+memory state to two files (`snapshot.file`, `memory.file`) in a per-sandbox pause directory, write a small JSON manifest recording the snapshot paths and the *original, untouched* rootfs path, then kill the process to free memory. The pause directory's path is returned as the opaque `pauseRef` the `Pausable` interface expects. Resume reads the manifest, spawns a fresh Firecracker process, reconfigures the host-side vsock socket (a host-only construct not captured in the snapshot), and loads the snapshot to resume execution.

## Consequences

- Firecracker's pause/resume captures genuinely more state than Docker's — a paused-and-resumed sandbox continues mid-computation, not just with its filesystem intact. This is a real capability advantage over the Docker backend, not just a different implementation of the same behavior.
- The rootfs file must survive for the entire duration of a pause, unmoved — `RemoveImage` therefore cleans up both the pause directory *and* the original rootfs file together, since neither is meaningful without the other once a pause is abandoned or successfully resumed.
- Snapshot/restore compatibility is tied to the exact Firecracker binary version used to create it — upgrading the Firecracker binary after sandboxes have been paused risks those snapshots failing to load. This is an operational constraint to account for before ever upgrading Firecracker versions with paused sandboxes outstanding; no mitigation is implemented yet.
- `FirecrackerManager`'s three new API calls (`pauseVM`, `createSnapshot`, `loadSnapshot`) and the process-spawning logic are fully covered by unit tests against fakes (see `manager_test.go`), requiring no real KVM — but the actual snapshot/restore mechanics have only been validated against fakes so far, not yet against a real boot on WSL2. A live end-to-end pause/resume test remains outstanding before this is trusted as production-ready.