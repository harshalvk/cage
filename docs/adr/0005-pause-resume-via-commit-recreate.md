# 0005. Implement pause/resume via container commit + recreate

Date: 2026-07-13

## Status

Accepted

## Context

Sandboxes need a pause/resume capability — freezing a sandbox's state without keeping it fully "running," then restoring it later. Two approaches were considered:

**Option A — `docker pause`/`docker unpause` (cgroup freezer):** Freezes all processes in the container at the cgroup level almost instantly, with no serialization step. The container remains in Docker's running-container list, simply frozen. The significant drawback: the container's memory stays fully resident in host RAM for the entire time it's paused — this doesn't actually reclaim any resources, it only stops CPU scheduling. For a system meant to hold many idle sandboxes, this doesn't scale.

**Option B — commit + stop + remove, recreate on resume:** Snapshot the container's filesystem into a new Docker image (`docker commit`), then stop and remove the container entirely, freeing its memory. On resume, create and start a fresh container from that committed image, restoring the filesystem state. Slower than Option A (image commit and container recreation both take real time, unlike an near-instant freeze), but genuinely frees memory for sandboxes that are paused for extended periods.

## Decision

Implement Option B: pause commits the container to an image and destroys the container; resume creates a new container from that image and deletes the intermediate image afterward.

## Consequences

- Paused sandboxes consume disk space (the committed image) instead of RAM — a better tradeoff for a system expected to have many idle sandboxes at once relative to actively running ones.
- Resume is meaningfully slower than Option A's near-instant unpause, since it involves real container creation, not just a cgroup state change.
- A committed image that is never resumed (an abandoned paused sandbox) currently has no automatic cleanup — the reaper only expires `running` sandboxes, not `paused` ones. This is a known gap and tracked as follow-up work (extending the reaper to also expire and garbage-collect long-paused sandboxes and their images).
- `container_id` is empty while a sandbox is paused (no live container exists); handlers for exec/read-file/write-file explicitly reject requests against non-running sandboxes with `409 Conflict` rather than failing with a confusing Docker-level "no such container" error.
- This decision can be revisited if Option A's speed becomes more important than Option B's memory efficiency for a given deployment — the two are not mutually exclusive, and a future ADR could introduce Option A as a fast-pause mode alongside this for short-lived pauses.

## Update — 2026-07-21

The reaper now also expires paused sandboxes (via PAUSED_SANDBOX_TTL,
default 24h) and removes their committed image, closing the disk-leak gap
noted above for abandoned pauses that are never resumed.