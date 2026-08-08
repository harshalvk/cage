# 0012. Defer the Firecracker backend, continue on Docker

Date: 2026-07-16

## Status

Accepted

## Context

ADR 0001 identified Docker as a deliberate, scoped-down substitute for Firecracker microVMs, with the expectation that a Firecracker backend would be added once the core system (lifecycle, exec, files, auth, pause/resume, caching, rate limiting, warm pooling) was working end-to-end.

On attempting to begin the Firecracker step, two hard prerequisites became clear: Firecracker requires KVM, which requires a Linux host — it cannot run on Windows directly, and even WSL2 requires nested virtualization support that is inconsistent across hardware/BIOS configurations. Additionally, Firecracker microVMs have no equivalent to Docker's exec/copy APIs; achieving feature parity (exec, file read/write) would require building and embedding a custom guest agent inside the VM's rootfs, communicating over vsock — a substantial, mostly self-contained project in its own right, separate from everything else in this system.

Given this, attempting the Firecracker swap without first settling on and provisioning an appropriate Linux environment would mean starting non-trivial infrastructure work on an unstable foundation.

## Decision

Defer the Firecracker backend. Continue building out remaining production-readiness work (observability, graceful shutdown, horizontal scaling readiness, deployment) on the existing Docker-based `SandboxManager`, to be revisited once a suitable Linux environment (a cloud VM, most likely, per ADR 0001's reasoning about deployment targets) is available for development and testing.

## Consequences

- All work in this phase (Redis caching, rate limiting, warm pooling, structured logging/metrics, graceful shutdown) is backend-agnostic at the seams that matter — `SandboxManager` already sits behind a `DockerClient` interface (ADR 0001), so none of this phase's work needs to be redone when Firecracker is eventually implemented.
- The security/isolation limitation noted in ADR 0001 (shared-kernel, namespace-level isolation rather than VM-level) remains in effect for as long as this decision stands — Cage should not be used to run genuinely untrusted third-party code in its current form.
- When resumed, the Firecracker work should be treated as its own multi-step project (environment setup and manual VM boot, guest agent, Go wrapper, networking, backend selection, snapshot-based pause/resume) rather than a single step, given the scope uncovered here.

## Status

Superseded — Firecracker backend is now implemented (see ADR 0001's `DockerClient` interface seam, and ADR 0015) on the `firecracker` branch. This ADR's *reasoning* for why Docker was the sensible starting point remains valid history; it no longer reflects current status.

## Update — 2026-07-24

Work resumed once a working WSL2 + KVM environment was confirmed available (see ADR 0001 for the original environment blockers). `internal/firecracker` now implements the full `backend.SandboxBackend` interface plus `Pausable` and `ImageCleaner` (ADR 0015). Networking (A4.5) remains deprioritized — vsock alone covers exec/file transfer without it. Not yet merged to `master`; still pending a real (non-faked) end-to-end validation pass and CI strategy for environments without KVM.

## Update — 2026-08-08

Firecracker implementation is functionally complete: full SandboxBackend,
Pausable, and ImageCleaner support, validated against real KVM on WSL2
(not just mocked tests) as of this update — see the fctest scratch program
run confirming a real pause/resume/exec cycle. Warm pool support (ADR 0009)
now works identically on both backends. Template schema updated (ADR — see
the firecracker_rootfs_slug migration) to avoid the Docker-image/rootfs-slug
ambiguity. Setup is documented in docs/firecracker-setup.md. Remaining open
items: no networking (deliberately deprioritized, vsock covers exec/files),
no CI coverage for real KVM behavior (mocked tests only), not yet merged
from the `firecracker` branch to `master`.