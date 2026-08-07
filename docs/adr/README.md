# Architecture Decision Records

This folder records the significant technical decisions made while building Cage — what was decided, why, and what alternatives were considered. The goal is that anyone (including future-me) can understand _why_ the codebase looks the way it does without having to reverse-engineer it from the code alone.

## Index

| ADR                                               | Title                                                    |
| ------------------------------------------------- | -------------------------------------------------------- |
| [0001](0001-use-docker-instead-of-firecracker.md) | Use Docker instead of Firecracker for sandbox isolation  |
| [0002](0002-postgres-for-sandbox-metadata.md)     | Use Postgres for sandbox metadata storage                |
| [0003](0003-tar-based-file-transfer.md)           | Use tar archives for file transfer to/from sandboxes     |
| [0004](0004-hash-api-keys-not-store-raw.md)       | Hash API keys before storing, never store raw keys       |
| [0005](0005-pause-resume-via-commit-recreate.md)  | Implement pause/resume via container commit + recreate   |
| [0006](0006-internal-package-layout.md)           | Use cmd/ + internal/ package layout                      |
| [0007](0007-redis-cache-fail-open.md)             | Cache API key validation in Redis                        |
| [0008](0008-token-bucket-rate-limiting.md)        | Token bucket rate limiting via atomic Redis Lua script   |
| [0009](0009-sandbox-warm-pool.md)                 | Pre-warm a pool of idle sandbox containers per template  |
| [0010](0010-structured-logging-and-metrics.md)    | Structured logging via slog and metrics via Prometheus   |
| [0011](0011-graceful-shutdown.md)                 | Graceful shutdown via signal-driven context cancellation |
| [0012](0012-defer-firecracker.md)                 | Defer the Firecracker backend, continue on Docker        |
| [0013](0013-sdk-cli-separate-modules.md)          | SDK and CLI as separate Go modules within the same repo  |
| [0014](0014-goreleaser-cli-distribution.md)       | SDK and CLI as separate Go modules within the same repo  |
| [0015](0015-firecracker-snapshot-pause-resume.md) | Implement Firecracker pause/resume via native VM snapshots | Accepted |

## When to write a new ADR

Write one when a decision:

- Is hard to reverse later (data model, storage engine, isolation primitive)
- Was chosen over a real alternative you seriously considered
- Would confuse a new contributor if left unexplained ("why isn't this just using an ORM?")

Don't write one for routine implementation details that don't involve a real tradeoff.

## Format

Copy `template.md`, number it sequentially, and fill in each section. Keep it short — a good ADR is a page or less. Once written, an ADR is rarely edited; if a decision is later reversed, write a **new** ADR that supersedes the old one and mark the old one's status as `Superseded by ADR-00XX`, rather than editing history.
