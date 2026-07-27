# DockerBox Project Guide

## Architecture

- **Frontend**: SvelteKit + Tailwind CSS v4, dev server on port 8080
- **Backend**: Go + Chi router, dev server on port 8081
- **Config**: `backend/config.yaml` (port, jwt_secret, users, dockerhosts)

## Start Services

```bash
# Backend (hot-reload)
cd backend && PATH="$HOME/go/bin:$PATH" air -c .air.toml

# Frontend (Vite dev)
cd frontend && bun run dev
```

## API Conventions

- All API routes under `/api/v1/`
- Auth: JWT Bearer token in `Authorization` header
- Docker host routing: pass `hostId` as **query parameter** (NOT header)
  - `?hostId=host210` or `?hostId=host086`
  - Fallback to default host if not provided
  - `getHostID(r)` helper reads query param first, then header

## Docker API

- Frontend: use `dockerApi` from `$lib/api/docker` (auto-injects hostId)
- Backend handler: `internal/handler/docker.go`
- Service: `internal/service/docker.go`
- `PruneImages` returns `{ deleted, spaceMB, message }`
- `PruneNetworks` returns `{ deleted, message }`
- `ImagesPrune` uses `dangling=false` to clean all unused images

## Frontend Patterns

- Toast notifications: `import { toastStore } from '$lib/stores/toast.svelte'`
- API client: `import { api } from '$lib/api/client'`
- Docker API: `import { dockerApi } from '$lib/api/docker'`
- Host selection stored in `localStorage.getItem('currentHostId')`

## File Structure

```
frontend/src/
  lib/api/client.ts      # Base API client
  lib/api/docker.ts      # Docker API with hostId injection
  lib/stores/toast.svelte.ts  # Toast notifications
  routes/containers/     # Container management
  routes/compose/        # Compose project management
  routes/overview/       # Dashboard overview
  routes/hosts/          # Docker host configuration

backend/internal/
  handler/docker.go      # Docker HTTP handlers
  service/docker.go      # Docker service (SSH/socket)
  handler/handler.go     # getHostID() helper
```
