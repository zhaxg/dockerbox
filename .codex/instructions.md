# Project Instructions

## Critical Rules

- **NEVER proactively kill/stop frontend or backend processes.** Air (backend hot reload) and Vite (frontend dev server) run as long-lived background processes. Do not `pkill`, `kill`, or stop them unless explicitly asked by the user.
- When rebuilding Go code, use `go build -o /tmp/boxbox-server ./cmd/server` and let the running server stay alive. Only restart when the user asks.
- When rebuilding frontend, use `npm run build` and copy to `internal/static/dist/`. Do not kill Vite.
- If a port is occupied, ask the user before killing the process occupying it.

## Dev Mode

- Backend: `BOXBOX_DEV_MODE=true /tmp/boxbox-server` on port 8081
- Frontend: Vite dev server on port 8080, proxies `/api` and `/ws` to 8081
- Production: embed frontend into Go binary, single port 8080

## Architecture

- Remote hosts use SFTP for file access (not Docker containers)
- Local hosts use filesystem directly
- Path resolution: `X-Host-ID` header or `?host=` query param
