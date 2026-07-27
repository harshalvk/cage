# 0013. SDK and CLI as separate Go modules within the same repo

Date: 2026-07-18

## Status

Accepted

## Context

Once external consumers needed to interact with Cage programmatically (rather than via raw HTTP/curl), a Go client library became necessary. The CLI and TUI both needed this same client logic — sandbox lifecycle calls, exec, file transfer — and duplicating HTTP-calling code across a CLI package and any future SDK would mean fixing every bug and adding every new endpoint twice.

A single-module approach (putting the SDK inside `internal/` or as a regular package in the main module) was rejected outright: `internal/` is compiler-enforced as non-importable from outside the module, which is exactly backwards for something meant to be a public dependency. Putting it in a non-internal package of the main module would work mechanically, but would mean anyone running `go get github.com/harshalvk/cage` to use the SDK also transitively pulls in the server's own dependencies (Docker SDK, pgx, golang-migrate, Redis client) that have nothing to do with being an API client.

## Decision

Structure `sdk/go` and `cli` as their own Go modules, each with an independent `go.mod`, inside the same repository (a "multi-module repo" / mono-repo-with-multiple-modules pattern). The CLI depends on the SDK via a `replace` directive pointing at the local relative path, so it always tracks the current in-repo SDK rather than a published version.

## Consequences

- `go get github.com/harshalvk/cage/sdk/go` pulls only the SDK's own minimal dependency set (just `net/http` and the standard library, no Docker/Postgres/Redis clients along for the ride).
- The CLI, TUI, and any example code all import and exercise the same SDK code path — a bug fixed or a feature added in the SDK is immediately available everywhere, with no duplicated HTTP logic to keep in sync.
- The `replace` directive means the SDK is not currently independently versioned or publishable as a standalone tagged release — `cli/go.mod`'s `replace ... => ../../sdk/go` only works when building from within this repo's checkout. If the SDK is ever meant to be consumed by external projects as a real dependency (not just referenced from docs), it will need its own release tags and the `replace` directive removed from any downstream consumer, replaced with a real version constraint.
- This same separate-module pattern was later extended to CLI distribution (see ADR 0014) — GoReleaser only needed to know about `cli/`'s module boundary, not the whole repository, keeping the release surface scoped to exactly what's being distributed.