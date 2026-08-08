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
 - [x] Pluggable isolation backends — Docker (default) or Firecracker microVMs, see [docs/firecracker-setup.md](docs/firecracker-setup.md)
  
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
- [x] CLI (`cage`) — create, exec, files, pause/resume, TUI, all from your terminal
- [x] Interactive TUI (`cage tui`) — live sandbox dashboard, Bubble Tea-based, animated splash screen
- [x] OpenAPI spec (`openapi.yaml`) + hosted, browsable API reference (Scalar + GitHub Pages)
- [x] Runnable examples in curl, Go, Python, and TypeScript
- [x] CLI distribution — Homebrew (macOS/Linux), Scoop (Windows), and a fallback install script
**Not yet built**
- [ ] Firecracker microVM backend (parked — see ADR 0012)
- [ ] Official TypeScript/Python SDKs (examples currently use raw HTTP)
- [ ] Checksum verification in the install script

## Architecture

Cage exposes a REST API that manages the lifecycle of sandboxes. Each sandbox currently maps 1:1 to a Docker container, with an in-memory (soon Postgres-backed) store tracking metadata.

<img width="4415" height="3042" alt="image" src="https://github.com/user-attachments/assets/20dfdd74-b457-4456-a145-b16c73c708f2" />

See [docs/adr](docs/adr/) for the reasoning behind each major architectural decision.

## Project Structure

```
cage/
├── cmd/
│   ├── cage/                 # main server entrypoint
│   └── genkey/               # CLI to generate API keys
├── internal/
│   ├── api/                  # HTTP handlers, auth + rate-limit + metrics middleware
│   │  └── openapi.yaml       # full API spec for every route
│   ├── auth/                 # API key generation & hashing
│   ├── cache/                # Redis cache wrapper
│   ├── config/               # env var loading
│   ├── db/                   # migration runner (golang-migrate)
│   ├── lock/                 # Redis-backed distributed lock (leader election)
│   ├── logging/              # structured slog setup
│   ├── metrics/              # Prometheus metric definitions
│   ├── pool/                 # sandbox pre-warming pool
│   ├── ratelimit/            # token bucket rate limiter
│   ├── reaper/               # background job: expire idle sandboxes
│   ├── reconcile/            # syncs DB state with Docker on boot
│   ├── sandbox/              # Docker SDK wrapper (create/exec/pause/files)
│   └── store/                # Postgres-backed persistence
├── sdk/
│   └── go/                   # cageclient — standalone Go module, the official SDK
├── cli/
│   ├── cmd/                  # Cobra commands (sandbox, exec, files, login, tui)
│   └── tui/                  # Bubble Tea interactive dashboard
├── migrations/               # golang-migrate SQL files (paired .up/.down)
├── scripts/                  # git hook scripts
├── docs/
│   └── adr/                  # Architecture Decision Records
├── .github/
│   ├── workflows/ci.yml      # lint, build, test on every push/PR
│   └── CODEOWNERS
├── docker-compose.yml        # Postgres + Redis + app, for local dev
├── Dockerfile                # multi-stage build for the server image
├── lefthook.yml              # pre-commit hooks (format, lint, build)
└── Makefile                  # dev, lint, fmt, migrate, genkey, test targets
```

`internal/` is Go-enforced: nothing outside this module can import it. `sdk/go` and `cli` are deliberately separate Go modules (each has its own `go.mod`) so the SDK can be imported independently without pulling in server-only dependencies like the Docker SDK or pgx.

## Getting Started

### Prerequisites

- Go 1.25+
- Docker + Docker Compose
- [golang-migrate](https://github.com/golang-migrate/migrate) CLI
- [Air](https://github.com/air-verse/air) (optional, for live reload)
- [golangci-lint](https://golangci-lint.run/) (optional, for linting)
- [Lefthook](https://github.com/evilmartians/lefthook) (optional, for git hooks)

### Run the server

```bash
git clone https://github.com/harshalvk/cage.git
cd cage
go mod tidy
cp .env.example .env      # fill in real values
make setup                # installs git hooks
docker compose up -d cage-postgres cage-redis
make migrate-up
make dev                   # live-reloading server on :8080
```

Or the full containerized stack:

```bash
docker compose up --build
```

### Use the CLI

```bash
cd cli
go build -o cage .

./cage login --api-key=<key-from-make-genkey>
./cage sandbox create --template python-3.12
./cage sandbox ls
./cage exec <id> -- python3 --version
./cage tui              # interactive dashboard

```

### Install the CLI

**macOS/Linux (Homebrew):**
\`\`\`bash
brew install harshalvk/cage/cage
\`\`\`

**Windows (Scoop):**
\`\`\`powershell
scoop bucket add cage https://github.com/harshalvk/scoop-cage
scoop install cage
\`\`\`

**Any platform (install script):**
\`\`\`bash
curl -sSL https://raw.githubusercontent.com/harshalvk/cage/main/install.sh | bash
\`\`\`

### Use the SDK

```bash
go get github.com/harshalvk/cage/sdk/go
```

```go
client := cageclient.New("http://localhost:8080", "your-api-key")
sb, _ := client.CreateSandbox(ctx, cageclient.CreateSandboxOptions{Template: "python-3.12"})
result, _ := client.Exec(ctx, sb.ID, []string{"python3", "-c", "print('hello')"})
```

## Authentication

All `/sandboxes` routes require an API key as a Bearer token. `/health` and `/templates` are public.

```bash
make genkey name=local-dev
```

```bash
curl -X POST http://localhost:8080/sandboxes \
  -H "Authorization: Bearer <your-api-key>"
```

## Isolation Backends

Cage supports two backends for actually running sandboxes, selected via `ISOLATION_BACKEND`:

| Backend | Isolation | Status |
|---------|-----------|--------|
| `docker` (default) | Container (shared kernel) | Stable |
| `firecracker` | microVM (KVM-based) | Available, not yet merged to `master` — see [docs/firecracker-setup.md](docs/firecracker-setup.md) |

Both backends support the full sandbox lifecycle, exec, and file transfer identically. Pause/resume works on both, via different mechanisms — see [ADR 0005](docs/adr/0005-pause-resume-via-commit-recreate.md) and [ADR 0015](docs/adr/0015-firecracker-snapshot-pause-resume.md).


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

More runnable examples (curl, Go, Python): [examples/](examples/)

This prints the raw key once - it is never shown again and only its hash is stored

## License

MIT

## Acknowledgements

Inspired by [E2B](https://e2b.dev), an excellent open-source sandbox infrastructure platform. Cage is an independent educational project and is not affiliated with E2B/FoundryLabs.
