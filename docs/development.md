# Development

## Prerequisites

- Go >= 1.22
- [air](https://github.com/air-verse/air) — Go hot-reload (`go install github.com/air-verse/air@latest`)
- [bun](https://bun.sh) — JS runtime & package manager
- Docker socket or SSH access to a Docker host

## Start

```bash
# Backend (Go + air hot-reload)
cd backend
PATH="$HOME/go/bin:$PATH" air -c .air.toml

# Frontend (Vite dev server)
cd frontend
bun run dev
```

Or use the combined script:

```bash
bun run dev
```

## Ports

| Service  | Port  | Notes                        |
|----------|-------|------------------------------|
| Frontend | 8080  | Vite dev server              |
| Backend  | 8081  | Go HTTP server               |

Frontend proxies `/api/*` → `localhost:8081` and `/ws/*` → `ws://localhost:8081` (configured in `vite.config.ts`).

## Backend Config

Config file: `backend/config.yaml`

Key fields:
- `port` — backend listen port (default `8081`)
- `jwt_secret` — dev secret (override in production)
- `users` — map of username → password
- `dockerhosts` — Docker host connections (socket or SSH)

Environment variable `BOXBOX_DEV_MODE=true` is set automatically by air.

## Frontend

Framework: SvelteKit + Tailwind CSS v4

Dev server starts on `http://localhost:8080`. Login with credentials from `config.yaml`.

## Build

```bash
# Frontend only
bun run build

# Full docker image
docker compose build
```
