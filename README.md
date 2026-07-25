<div align="center">

# Cage

[![CI](https://github.com/Harshalvk/cage/actions/workflows/ci.yml/badge.svg)](https://github.com/Harshalvk/cage/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](https://go.dev/)
[![Docker](https://img.shields.io/badge/Docker-Image-2496ED?logo=docker&logoColor=white)](https://hub.docker.com/)

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

- [x] Sandbox lifecycle management (create, list, get, delete)
- [x] Docker-backed isolation
- [x] Command execution inside sandboxes (stdout/stderr streaming)
- [x] File upload/download to/from sandboxes
- [x] Persistent storage (Postgres) for sandbox metadata
- [x] Idle/expiry-based sandbox cleanup
- [x] API key authentication
- [x] Custom sandbox templates
- [x] Pause/resume support
- [ ] Firecracker microVM backend (long-term goal)

## Project Structure
```bash
cage/
├── cmd/
│   ├── cage/           # main entrypoint — wires everything together and starts the HTTP server
│   └── genkey/         # CLI to generate API keys
├── internal/
│   ├── api/            # HTTP handlers + auth middleware
│   ├── auth/            # API key generation & hashing
│   ├── config/          # env var loading
│   ├── db/               # migration runner (golang-migrate)
│   ├── reaper/           # background job that kills expired sandboxes
│   ├── reconcile/         # syncs DB state with actual Docker state on boot
│   ├── sandbox/           # Docker SDK wrapper — create/exec/read/write files
│   └── store/             # Postgres-backed sandbox + API key persistence
├── migrations/            # golang-migrate SQL files (paired .up/.down)
├── scripts/               # git hook scripts (e.g. commit-msg validation)
├── .air.toml              # live reload config
├── .env.example           # documents required env vars
├── .golangci.yml          # linter config
├── docker-compose.yml     # Postgres + Cage app stack
├── Dockerfile             # multi-stage build for the app image
├── lefthook.yml           # pre-commit/commit-msg hooks
└── Makefile               # dev, lint, fmt, migrate, genkey commands
```

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

The API will be available at `http://localhost:8080`.

## Development

| Command                      | Description                 |
| ---------------------------- | --------------------------- |
| `make dev`                   | Run with live reload (Air)  |
| `make build`                 | Build the binary            |
| `make lint`                  | Run golangci-lint           |
| `make fmt`                   | Format code                 |
| `make migrate-up`            | Apply DB migrations         |
| `make migrate-down`          | Roll back last migration    |
| `make migrate-create name=X` | Create a new migration pair |
| `make test`                  | Run tests                   |
| `make genkey name=X` | Generate a new API key labeled X |

### API Reference

| Method | Endpoint          | Description               |
| ------ | ----------------- | ------------------------- |
| GET    | `/health`         | Health check              |
| POST   | `/sandboxes`      | Create a new sandbox      |
| GET    | `/sandboxes`      | List all sandboxes        |
| GET    | `/sandboxes/{id}` | Get sandbox details       |
| DELETE | `/sandboxes/{id}` | Kill and remove a sandbox |

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
