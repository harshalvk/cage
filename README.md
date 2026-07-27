<div align="center">

# Cage

[![CI](https://github.com/Harshalvk/cage/actions/workflows/ci.yml/badge.svg)](https://github.com/Harshalvk/cage/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](https://go.dev/)
[![Docker](https://img.shields.io/badge/Docker-Image-2496ED?logo=docker&logoColor=white)](https://hub.docker.com/)
[![API Docs](https://img.shields.io/badge/docs-API%20reference-blue)](https://harshalvk.github.io/Cage/api/)

</div>

<img width="1788" height="464" alt="image" src="https://github.com/user-attachments/assets/d463f5f5-9029-4a4f-8c18-2f03d5339ed4" />

**Cage** is an open-source, self-hostable clone of [E2B](https://e2b.dev) — a backend service for spinning up secure, isolated sandboxes to run untrusted or AI-generated code. Built in Go.

> ⚠️ **Work in progress.** Cage is a learning project and is not production-ready. Currently uses Docker containers as the isolation layer, with plans to explore Firecracker microVMs.

## What is Cage?

Cage lets you programmatically create isolated environments ("sandboxes"), run commands inside them, and tear them down — all through a simple REST API. Think of it as the infrastructure layer you'd use to let an AI agent safely execute code without touching your host system.

```bash
# create a sandbox
curl -X POST http://localhost:8080/sandboxes

# → {"id":"a1b2c3...","status":"running","created_at":"2026-07-08T12:00:00Z"}
```

## Features

**Core**

- [x] Sandbox lifecycle management (create, list, get, delete)
- [x] Docker-backed isolation, pluggable behind a `DockerClient` interface
- [x] Command execution inside sandboxes, with streamed stdout/stderr demuxing
- [x] File upload/download to/from sandboxes
- [x] Pause / resume via container commit + recreate (frees memory, not just frozen in place)
- [x] Custom sandbox templates (Python, Node, or your own image)
- [x] Persistent storage (Postgres) for sandbox + API key metadata
- [x] Idle/expiry-based cleanup (background reaper + startup reconciliation)
      **Production readiness**
- [x] API key authentication (hashed, never stored raw)
- [x] Redis-backed caching for auth checks (fail-open on cache errors)
- [x] Redis-backed rate limiting (token bucket, fail-open)
- [x] Warm sandbox pool per template — cuts cold-start latency on creation
- [x] Structured JSON logging (`slog`) + Prometheus metrics
- [x] Graceful shutdown (drains in-flight requests on `SIGTERM`)
- [x] Distributed locking — safe to run multiple replicas (no double-reaping)
      **Developer experience**
- [x] Go SDK (`sdk/go`) — typed client for every endpoint
- [x] CLI (`cage`) — create, exec, files, pause/resume, all from your terminal
- [x] Interactive TUI (`cage tui`) — live sandbox dashboard, Bubble Tea-based
- [x] OpenAPI spec (`openapi.yaml`) documenting every route
      **Not yet built**
- [ ] Firecracker microVM backend (parked — see ADR 0012)
- [ ] Hosted/browsable API docs site
- [ ] CLI distribution via Homebrew/install script

## Project Structure

```
cage/
├── cmd/
│   ├── cage/              # main server entrypoint
│   └── genkey/            # CLI to generate API keys
├── internal/
│   ├── api/                # HTTP handlers, auth + rate-limit + metrics middleware
│   ├── auth/                # API key generation & hashing
│   ├── cache/                # Redis cache wrapper
│   ├── config/                 # env var loading
│   ├── db/                      # migration runner (golang-migrate)
│   ├── lock/                     # Redis-backed distributed lock (leader election)
│   ├── logging/                   # structured slog setup
│   ├── metrics/                     # Prometheus metric definitions
│   ├── pool/                          # sandbox pre-warming pool
│   ├── ratelimit/                       # token bucket rate limiter
│   ├── reaper/                           # background job: expire idle sandboxes
│   ├── reconcile/                          # syncs DB state with Docker on boot
│   ├── sandbox/                              # Docker SDK wrapper (create/exec/pause/files)
│   └── store/                                  # Postgres-backed persistence
├── sdk/
│   └── go/                # cageclient — standalone Go module, the official SDK
├── cli/
│   ├── cmd/                # Cobra commands (sandbox, exec, files, login, tui)
│   └── tui/                  # Bubble Tea interactive dashboard
├── migrations/              # golang-migrate SQL files (paired .up/.down)
├── scripts/                   # git hook scripts
├── docs/
│   └── adr/                     # Architecture Decision Records
├── .github/
│   ├── workflows/ci.yml            # lint, build, test on every push/PR
│   └── CODEOWNERS
├── openapi.yaml               # full API spec for every route
├── docker-compose.yml           # Postgres + Redis + app, for local dev
├── Dockerfile                     # multi-stage build for the server image
├── lefthook.yml                      # pre-commit hooks (format, lint, build)
└── Makefile                            # dev, lint, fmt, migrate, genkey, test targets
```

`internal/` is Go-enforced: nothing outside this module can import it. `sdk/go` and `cli` are deliberately separate Go modules (each has its own `go.mod`) so the SDK can be imported independently without pulling in server-only dependencies like the Docker SDK or pgx.

## Architecture

Cage exposes a REST API that manages the lifecycle of sandboxes. Each sandbox currently maps 1:1 to a Docker container, with an in-memory (soon Postgres-backed) store tracking metadata.

<img width="4415" height="3042" alt="image" src="https://github.com/user-attachments/assets/20dfdd74-b457-4456-a145-b16c73c708f2" />

## Getting Started

### Prerequisites

- Go 1.25+
- Docker + Docker Compose
- [golang-migrate](https://github.com/golang-migrate/migrate) CLI
- [Air](https://github.com/air-verse/air) (optional, for live reload)
- [golangci-lint](https://golangci-lint.run/) (optional, for linting)
- [Lefthook](https://github.com/evilmartians/lefthook) (optional, for git hooks)

### Installation

```bash
git clone https://github.com/harshalvk/cage.git
cd cage
go mod tidy
cp .env.example .env   # fill in real values
make setup             # installs git hooks
make migrate-up         # apply DB schema
```

### Running

**Option A - Full stack via docker compose (postgres + cage, containerized):**

```bash
docker compose up --build
```

**Option B - Local dev (golang on host, postgres in docker, live reload):**

```bash
docker compose up -d cage-postgres
make migrate-up
make dev    # live-reloading dev server
```

### Install the CLI

**macOS/Linux (Homebrew):**

```bash
brew install harshalvk/cage/cage
```

**Windows (Scoop):**

```powershell
scoop bucket add cage https://github.com/harshalvk/cage
scoop install cage
```

**Any platform (install script):**

```bash
curl -sSL https://raw.githubusercontent.com/harshalvk/cage/master/install.sh | bash
```

The API will be available at `http://localhost:8080`.

## Development

| Command                      | Description                      |
| ---------------------------- | -------------------------------- |
| `make dev`                   | Run with live reload (Air)       |
| `make build`                 | Build the binary                 |
| `make lint`                  | Run golangci-lint                |
| `make fmt`                   | Format code                      |
| `make migrate-up`            | Apply DB migrations              |
| `make migrate-down`          | Roll back last migration         |
| `make migrate-create name=X` | Create a new migration pair      |
| `make test`                  | Run tests                        |
| `make genkey name=X`         | Generate a new API key labeled X |

### API Reference

| Method | Endpoint                 | Description                      |
| ------ | ------------------------ | -------------------------------- |
| GET    | `/health`                | Health check                     |
| GET    | `/templates`             | List available sandbox templates |
| POST   | `/sandboxes`             | Create a sandbox                 |
| GET    | `/sandboxes`             | List sandboxes                   |
| GET    | `/sandboxes/{id}`        | Get sandbox details              |
| DELETE | `/sandboxes/{id}`        | Kill and remove a sandbox        |
| POST   | `/sandboxes/{id}/exec`   | Run a command inside a sandbox   |
| POST   | `/sandboxes/{id}/files`  | Write a file into a sandbox      |
| GET    | `/sandboxes/{id}/files`  | Read a file from a sandbox       |
| POST   | `/sandboxes/{id}/pause`  | Pause a running sandbox          |
| POST   | `/sandboxes/{id}/resume` | Resume a paused sandbox          |
| GET    | `/metrics`               | Prometheus metrics               |

Full request/response schemas: [openapi.yaml](openapi.yaml), or browse them at the [hosted API reference](https://harshalvk.github.io/cage/api/).

More runnable examples (curl, Go, Python): [examples/](examples/)

## Authentication

All `/sandboxes` routes require an API key, passed as a Bearer token:

```bash
curl -X POST http://localhost:8080/sandboxes -H "Authorization: Bearer <your-api-key>"
```

`/health` remains public and requires no key

### Generating a key

```bash
make genkey name=local-dev
```

This prints the raw key once - it is never shown again and only its hash is stored

## License

MIT

## Acknowledgements

Inspired by [E2B](https://e2b.dev), an excellent open-source sandbox infrastructure platform. Cage is an independent educational project and is not affiliated with E2B/FoundryLabs.
