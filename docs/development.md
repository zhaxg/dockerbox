# Development

本地开发前后端分离，各自热重载。

## Prerequisites

- Go 1.25+
- Node.js 20+
- [Air](https://github.com/air-verse/air)：

```bash
go install github.com/air-verse/air@latest
```

## 启动开发

两个终端分别驻留后台：

**终端 1 — 后端（air 热重载）**

```bash
cd backend
~/go/bin/air
```

Air 监听 `cmd/` 和 `internal/` 下 `.go` 文件，改动后自动编译重启。默认 `BOXBOX_DEV_MODE=true`，不嵌入前端，只跑 API（`localhost:8081`）。

**终端 2 — 前端（Vite 热重载）**

```bash
cd frontend
npm install
npm run dev
```

Vite dev server 跑在 `http://localhost:8080`，自动代理 `/api` 和 `/ws` 到后端 `localhost:8081`。

## 发布构建

```bash
cd frontend && npm run build
cd ../backend && go build -o boxbox ./cmd/server
```

去掉 `dev_mode`（默认 false），前端通过 `go:embed` 嵌入 Go 二进制。

## 配置

| 文件 | 说明 |
|------|------|
| `backend/config.yaml` | 后端配置 |
| `backend/.air.toml` | Air 热重载配置 |

`config.yaml` 关键字段：

```yaml
dev_mode: true     # 开发模式，跳过前端嵌入
port: 8081
users:
  admin: admin123
```

## Repository Layout

```text
backend/      Go API server and embedded static host
frontend/     SvelteKit app compiled to static files
docs/         Markdown project documentation
scripts/      Local development helpers
Dockerfile    Single-container production build
```

## Backend Patterns

Handler -> Service -> Model/Filesystem：

- Handlers 解析请求、写响应
- Services 管业务逻辑和文件操作
- Models 定义请求/响应/领域类型
- 常量放 `backend/internal/config/constants.go`
- 文件操作走 `internal/pkg/filesystem`

## Frontend Patterns

- Svelte 5 runes（`$state`、`$derived`）
- API 调用放 `frontend/src/lib/api`
- UI 组件放 `frontend/src/lib/components/ui`
- 格式化放 `frontend/src/lib/utils/format.ts`
