# BoxBox Docker 管理集成方案（完整版）

## 目录

- [一、导航栏改造](#一导航栏改造)
- [二、后端设计](#二后端设计)
- [三、前端设计](#三前端设计)
- [四、实施路线图](#四实施路线图)

---

## 一、导航栏改造

### 新导航结构

```
┌─────────────────────────────┐
│  🏠 概览                     │  ← Dashboard（系统概览、容器状态、资源监控）
├─────────────────────────────┤
│  🐳 容器                     │  ← Docker 容器管理
├─────────────────────────────┤
│  📦 Compose                  │  ← Docker Compose 项目管理
├─────────────────────────────┤
│  📁 文件                     │  ← 文件管理（可展开）
│     ├── ⭐ 收藏目录           │
│     ├── 🖥️ This Server      │
│     ├── 📥 Downloads         │
│     ├── 📄 Documents         │
│     ├── 🎵 Music             │
│     ├── 🖼️ Pictures          │
│     └── 🎬 Videos            │
├─────────────────────────────┤
│  ⚙️ 设置                     │  ← 系统设置
└─────────────────────────────┘
```

### 路由规划

```
frontend/src/routes/
├── +layout.svelte                    # 主布局（侧边栏 + 内容区）
├── overview/
│   └── +page.svelte                  # 概览页（仪表盘）
├── containers/
│   ├── +page.svelte                  # 容器列表
│   └── [id]/
│       ├── +page.svelte              # 容器详情
│       ├── logs/+page.svelte         # 容器日志
│       └── exec/+page.svelte         # 容器终端
├── compose/
│   ├── +page.svelte                  # Compose 项目列表
│   └── [id]/
│       └── +page.svelte              # 项目详情/编辑
├── browse/
│   └── +page.svelte                  # 文件浏览（保留现有）
├── settings/
│   └── +page.svelte                  # 设置页（保留现有）
└── login/
    └── +page.svelte                  # 登录页
```

---

## 二、后端设计

### 1. 新增依赖

```go
// backend/go.mod
require (
    // ...existing...
    github.com/docker/docker v27.0.0    // Docker SDK
)
```

### 2. 新增模型 `internal/model/docker.go`

```go
// 容器模型
type Container struct {
    ID        string            `json:"id"`
    Name      string            `json:"name"`
    Image     string            `json:"image"`
    Status    string            `json:"status"`    // running, stopped, paused
    State     string            `json:"state"`
    Created   int64             `json:"created"`
    Ports     []PortBinding     `json:"ports"`
    CPU       float64           `json:"cpu"`       // CPU 使用率 %
    Memory    MemoryUsage       `json:"memory"`
    Labels    map[string]string `json:"labels"`
}

type PortBinding struct {
    HostPort      string `json:"hostPort"`
    ContainerPort string `json:"containerPort"`
    Protocol      string `json:"protocol"`
}

type MemoryUsage struct {
    Usage   int64   `json:"usage"`   // bytes
    Limit   int64   `json:"limit"`
    Percent float64 `json:"percent"`
}

// Compose 项目模型
type ComposeProject struct {
    ID       string `json:"id"`
    Name     string `json:"name"`
    Path     string `json:"path"`
    Status   string `json:"status"`    // running, stopped, partial
    Services int    `json:"services"`
    Running  int    `json:"running"`
    File     string `json:"file"`      // docker-compose.yml 内容
    EnvFile  string `json:"envFile"`   // .env 内容
}
```

### 3. 新增服务 `internal/service/docker.go`

```go
type DockerService interface {
    // 容器管理
    ListContainers(ctx context.Context) ([]Container, error)
    GetContainer(ctx context.Context, id string) (*Container, error)
    StartContainer(ctx context.Context, id string) error
    StopContainer(ctx context.Context, id string) error
    RestartContainer(ctx context.Context, id string) error
    DeleteContainer(ctx context.Context, id string) error
    GetContainerLogs(ctx context.Context, id string, tail int) ([]string, error)
    ExecContainer(ctx context.Context, id string, cmd []string) (io.ReadWriteCloser, error)
    
    // Compose 管理
    ListComposeProjects(ctx context.Context) ([]ComposeProject, error)
    ComposeUp(ctx context.Context, projectPath string) error
    ComposeDown(ctx context.Context, projectPath string) error
    ComposeBuild(ctx context.Context, projectPath string) error
    ComposeLogs(ctx context.Context, projectPath string, tail int) ([]string, error)
    GetComposeFile(ctx context.Context, projectPath string) (string, error)
    SaveComposeFile(ctx context.Context, projectPath string, content string) error
}
```

### 4. 新增处理器 `internal/handler/docker.go`

```go
// 容器 API
r.Route("/containers", func(r chi.Router) {
    r.Get("/", h.ListContainers)           // GET /api/v1/containers
    r.Get("/{id}", h.GetContainer)         // GET /api/v1/containers/{id}
    r.Post("/{id}/start", h.Start)         // POST /api/v1/containers/{id}/start
    r.Post("/{id}/stop", h.Stop)           // POST /api/v1/containers/{id}/stop
    r.Post("/{id}/restart", h.Restart)     // POST /api/v1/containers/{id}/restart
    r.Delete("/{id}", h.Delete)            // DELETE /api/v1/containers/{id}
    r.Get("/{id}/logs", h.GetLogs)         // GET /api/v1/containers/{id}/logs
    r.Get("/{id}/exec", h.ExecWebSocket)  // WS /api/v1/containers/{id}/exec
})

// Compose API
r.Route("/compose", func(r chi.Router) {
    r.Get("/", h.ListProjects)             // GET /api/v1/compose
    r.Post("/{id}/up", h.Up)               // POST /api/v1/compose/{id}/up
    r.Post("/{id}/down", h.Down)           // POST /api/v1/compose/{id}/down
    r.Post("/{id}/build", h.Build)         // POST /api/v1/compose/{id}/build
    r.Get("/{id}/logs", h.GetLogs)         // GET /api/v1/compose/{id}/logs
    r.Get("/{id}/file", h.GetFile)         // GET /api/v1/compose/{id}/file
    r.Put("/{id}/file", h.SaveFile)        // PUT /api/v1/compose/{id}/file
    r.Get("/{id}/env", h.GetEnv)           // GET /api/v1/compose/{id}/env
    r.Put("/{id}/env", h.SaveEnv)          // PUT /api/v1/compose/{id}/env
})
```

---

## 三、前端设计

### 1. Sidebar 组件 `frontend/src/lib/components/Sidebar.svelte`

```svelte
<script lang="ts">
  import {
    LayoutDashboard,
    Container,
    Package,
    FolderOpen,
    Star,
    Settings,
    ChevronDown,
    Server,
    Monitor,
    Download,
    FileText,
    Music,
    Image,
    Video
  } from 'lucide-svelte';
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { settingsStore } from '$lib/stores/settings';

  const navItems = [
    { name: '概览', path: '/overview', icon: LayoutDashboard },
    { name: '容器', path: '/containers', icon: Container },
    { name: 'Compose', path: '/compose', icon: Package },
    { 
      name: '文件', 
      path: '/browse', 
      icon: FolderOpen,
      children: [
        { name: '收藏目录', path: '/browse/favorites', icon: Star, isFavorites: true },
        { name: 'This Server', path: '/browse', icon: Server },
        { name: 'Desktop', path: '/browse/desktop', icon: Monitor },
        { name: 'Downloads', path: '/browse/downloads', icon: Download },
        { name: 'Documents', path: '/browse/documents', icon: FileText },
        { name: 'Music', path: '/browse/music', icon: Music },
        { name: 'Pictures', path: '/browse/pictures', icon: Image },
        { name: 'Videos', path: '/browse/videos', icon: Video }
      ]
    },
    { name: '设置', path: '/settings', icon: Settings }
  ];

  let filesExpanded = $state(true);

  function isActive(path: string): boolean {
    return page.url.pathname.startsWith(path);
  }

  function handleNavigate(path: string) {
    goto(resolve(path));
  }
</script>

<aside class="flex w-[220px] min-w-[220px] flex-col border-r border-border-secondary bg-surface-primary">
  <div class="border-b border-border-secondary px-4 py-3">
    <h1 class="text-lg font-semibold text-text-primary">BoxBox</h1>
  </div>

  <nav class="flex-1 overflow-y-auto py-2">
    {#each navItems as item}
      {#if item.children}
        <button
          type="button"
          class="nav-item {isActive(item.path) ? 'active' : ''}"
          onclick={() => { filesExpanded = !filesExpanded; handleNavigate(item.path); }}
        >
          <item.icon size={18} class="shrink-0 opacity-80" />
          <span class="flex-1 text-left">{item.name}</span>
          <ChevronDown 
            size={14} 
            class="shrink-0 transition-transform {filesExpanded ? '' : '-rotate-90'}" 
          />
        </button>
        
        {#if filesExpanded}
          <div class="ml-4">
            {#if item.children[0].isFavorites}
              <div class="nav-section-title">收藏目录</div>
              {#each $settingsStore.favoriteFolders as fav}
                <button
                  type="button"
                  class="nav-item-sub {isActive(fav.path) ? 'active' : ''}"
                  onclick={() => handleNavigate(`/browse${fav.path}`)}
                >
                  <Star size={14} class="shrink-0 opacity-80" />
                  <span class="flex-1 text-left truncate">{fav.name}</span>
                </button>
              {/each}
            {/if}
            
            {#each item.children.slice(1) as child}
              <button
                type="button"
                class="nav-item-sub {isActive(child.path) ? 'active' : ''}"
                onclick={() => handleNavigate(child.path)}
              >
                <child.icon size={14} class="shrink-0 opacity-80" />
                <span class="flex-1 text-left">{child.name}</span>
              </button>
            {/each}
          </div>
        {/if}
      {:else}
        <button
          type="button"
          class="nav-item {isActive(item.path) ? 'active' : ''}"
          onclick={() => handleNavigate(item.path)}
        >
          <item.icon size={18} class="shrink-0 opacity-80" />
          <span class="flex-1 text-left">{item.name}</span>
        </button>
      {/if}
    {/each}
  </nav>
</aside>

<style>
  .nav-item {
    @apply w-full flex items-center gap-2.5 py-2 px-4 bg-transparent border-none text-text-primary text-[13px] cursor-pointer text-left transition-colors hover:bg-surface-secondary;
  }
  .nav-item.active {
    @apply bg-selection text-white hover:bg-selection-hover;
  }
  .nav-item-sub {
    @apply w-full flex items-center gap-2 py-1.5 px-4 bg-transparent border-none text-text-secondary text-[12px] cursor-pointer text-left transition-colors hover:bg-surface-secondary hover:text-text-primary;
  }
  .nav-item-sub.active {
    @apply text-accent;
  }
  .nav-section-title {
    @apply px-4 py-1 text-[11px] font-medium text-text-muted uppercase;
  }
</style>
```

### 2. 概览页 `frontend/src/routes/overview/+page.svelte`

```svelte
<script lang="ts">
  import { onMount } from 'svelte';
  import { Card, Spinner } from '$lib/components/ui';
  import { Container, Package, HardDrive, Activity } from 'lucide-svelte';
  
  let stats = $state({
    containers: { total: 0, running: 0, stopped: 0 },
    compose: { total: 0, running: 0 },
    disk: { used: 0, total: 0 },
    cpu: 0,
    memory: { used: 0, total: 0 }
  });
  
  let loading = $state(true);

  onMount(async () => {
    // TODO: 从 API 获取数据
    loading = false;
  });
</script>

<div class="p-6">
  <h1 class="mb-6 text-2xl font-semibold text-text-primary">概览</h1>
  
  {#if loading}
    <Spinner />
  {:else}
    <div class="mb-6 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <Card>
        <div class="flex items-center gap-3">
          <div class="rounded-lg bg-blue-500/10 p-2">
            <Container size={24} class="text-blue-500" />
          </div>
          <div>
            <div class="text-2xl font-bold text-text-primary">{stats.containers.running}</div>
            <div class="text-sm text-text-secondary">运行中容器</div>
          </div>
        </div>
      </Card>

      <Card>
        <div class="flex items-center gap-3">
          <div class="rounded-lg bg-green-500/10 p-2">
            <Package size={24} class="text-green-500" />
          </div>
          <div>
            <div class="text-2xl font-bold text-text-primary">{stats.compose.running}</div>
            <div class="text-sm text-text-secondary">运行中 Compose</div>
          </div>
        </div>
      </Card>

      <Card>
        <div class="flex items-center gap-3">
          <div class="rounded-lg bg-orange-500/10 p-2">
            <HardDrive size={24} class="text-orange-500" />
          </div>
          <div>
            <div class="text-2xl font-bold text-text-primary">
              {Math.round(stats.disk.used / 1024 / 1024 / 1024)}GB
            </div>
            <div class="text-sm text-text-secondary">
              / {Math.round(stats.disk.total / 1024 / 1024 / 1024)}GB
            </div>
          </div>
        </div>
      </Card>

      <Card>
        <div class="flex items-center gap-3">
          <div class="rounded-lg bg-purple-500/10 p-2">
            <Activity size={24} class="text-purple-500" />
          </div>
          <div>
            <div class="text-2xl font-bold text-text-primary">{stats.cpu}%</div>
            <div class="text-sm text-text-secondary">CPU 使用率</div>
          </div>
        </div>
      </Card>
    </div>

    <Card title="最近容器">
      <div class="space-y-2">
        <!-- 容器列表 -->
      </div>
    </Card>
  {/if}
</div>
```

### 3. 布局调整 `frontend/src/routes/+layout.svelte`

```svelte
<!-- 修改 isWorkspacePage 判断 -->
const isWorkspacePage = $derived(
  page.url.pathname.startsWith('/browse') || 
  page.url.pathname.startsWith('/settings') ||
  page.url.pathname.startsWith('/overview') ||
  page.url.pathname.startsWith('/containers') ||
  page.url.pathname.startsWith('/compose')
);

<!-- 新增 Sidebar 到工作区布局 -->
{:else if isWorkspacePage}
  <div class="flex h-screen">
    <Sidebar />
    <main class="flex-1 overflow-auto">
      {@render children()}
    </main>
  </div>
```

---

## 四、实施路线图

| 阶段 | 功能 | 工作量 |
|------|------|--------|
| **Phase 1** | 后端 Docker SDK 集成 + 容器列表 API | 2-3 天 |
| **Phase 2** | 前端导航栏改造 + 概览页 | 1-2 天 |
| **Phase 3** | 容器管理页面（列表、操作、日志、终端） | 3-4 天 |
| **Phase 4** | Compose 管理页面 | 3-4 天 |
| **Phase 5** | UI 美化 + 测试 | 2-3 天 |

**总计：约 11-16 天**

---

## 五、关键技术点

1. **Docker Socket 挂载**：容器内需要访问 `/var/run/docker.sock`
2. **WebSocket 终端**：使用 xterm.js + WebSocket 实现交互式 shell
3. **实时监控**：通过 Docker API 定时获取容器资源使用
4. **Compose 项目发现**：扫描指定目录或读取配置文件
5. **安全考虑**：只读模式可选，避免误操作

---

*文档更新时间：2026-07-23*
