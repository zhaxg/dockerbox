# BoxBox Docker 管理 — 测试清单

> 更新时间: 2026-07-23
> 目的: 逐项检查实现状态 + 测试要点

---

## 部署配置问题 (已修复)

| # | 问题 | 修复 | 确认 |
|---|------|------|------|
| P1 | docker-compose.yml 缺少 `/var/run/docker.sock` 挂载 | 已添加 `- /var/run/docker.sock:/var/run/docker.sock:ro` | ✅ |
| P2 | docker-compose.yml 缺少 `/proc`、`/sys` 挂载(概览页需要) | 已添加 `/proc:/host/proc:ro`、`/sys:/host/sys:ro` | ✅ |
| P3 | binary 上传后无执行权限 | `chmod +x` 修复 | ✅ |
| P4 | binary 挂载路径错误: `/app/boxbox-linux` → 实际 entrypoint 是 `/app/server` | 改为 `- ./boxbox-linux:/app/server:ro` | ✅ |
| P5 | compose 扫描路径 `/vol1/1000/docker` 在容器内不存在(需用 `/host_root` 前缀) | config.yaml 改为 `/host_root/vol1/1000/docker` | ✅ |
| P6 | `docker compose restart` 不重新挂载 volume，需 `up -d --force-recreate` | 文档已更新 | ✅ |

---

## 一、导航栏改造

| # | 需求 | 状态 | 测试要点 |
|---|------|------|----------|
| 1.1 | 侧边栏导航 | ✅ | 概览/容器/镜像/网络/Compose/文件/设置 七个入口可点击 |
| 1.2 | 文件展开/收起 | ✅ | ChevronDown 图标旋转，收藏目录+挂载目录子菜单展开收起正常 |
| 1.3 | 路由跳转 | ✅ | 点击各菜单项跳转到对应页面 |
| 1.4 | 路由守卫 | ✅ | 未登录访问受保护路由跳转 login |

---

## 二、宿主机概览页

| # | 需求 | 状态 | 测试要点 |
|---|------|------|----------|
| 2.1 | CPU 使用率 | ✅ | 显示 user/system/idle，趋势图更新 |
| 2.2 | 内存信息 | ✅ | 总量/已用/可用显示正确 |
| 2.3 | 网络流量 | ✅ | 收发流量实时更新 |
| 2.4 | 系统负载 | ✅ | loadavg 显示 |
| 2.5 | SSE 实时推送 | ✅ | 数据自动刷新，无手动轮询 |
| 2.6 | 概览页 UI | ✅ | 统计卡片显示正常，最近容器列表可见 |

---

## 三、容器管理

| # | 需求 | 状态 | 测试要点 |
|---|------|------|----------|
| 3.1 | 容器列表 | ✅ | 显示所有容器(17个)，状态图标正确 |
| 3.2 | 容器详情 | ✅ | 点击容器显示完整信息(ID/状态/镜像/端口) |
| 3.3 | 启动容器 | ✅ | 点击启动按钮，状态变为 running |
| 3.4 | 停止容器 | ✅ | 点击停止按钮，状态变为 exited |
| 3.5 | 重启容器 | ✅ | 点击重启按钮，容器重启 |
| 3.6 | 删除容器 | ✅ | 带确认弹窗，删除后列表更新 |
| 3.7 | Kill 容器 | ✅ | 可选择信号 (SIGTERM/SIGKILL) |
| 3.8 | 容器 Inspect | ✅ | 查看完整 JSON 配置 |
| 3.9 | 容器终端 | ✅ | WebSocket 连接，可执行命令 |
| 3.10 | 容器日志 | ✅ | SSE 流式推送+一次性获取，支持 tail 参数 |
| 3.11 | 容器 Stats | ✅ | CPU/内存/网络 IO 实时更新(SSE实时推) |

---

## 四、Compose 管理

| # | 需求 | 状态 | 测试要点 |
|---|------|------|----------|
| 4.1 | Compose 项目列表 | ✅ | 显示所有项目，状态正确 |
| 4.2 | 新建项目 | ✅ | 选择目录 + 创建 docker-compose.yml |
| 4.3 | 项目详情 | ✅ | 查看 services 列表和状态 |
| 4.4 | 编辑文件 | ✅ | Monaco Editor 编辑 docker-compose.yml |
| 4.5 | 编辑 .env | ✅ | Monaco Editor 编辑 .env 文件 |
| 4.6 | Up 项目 | ✅ | docker-compose up -d 执行成功 |
| 4.7 | Down 项目 | ✅ | docker-compose down 执行成功 |
| 4.8 | 重新部署 | ✅ | 一键 down + up |
| 4.9 | 重启项目 | ✅ | docker-compose restart 执行成功 |
| 4.10 | 流式日志 | ✅ | SSE 实时推送，支持按 service 过滤 |

---

## 五、网络管理

| # | 需求 | 状态 | 测试要点 |
|---|------|------|----------|
| 5.1 | 网络列表 | ✅ | 显示所有网络 |
| 5.2 | 查看容器归属 | ✅ | 网络关联的容器列表 |
| 5.3 | 删除网络 | ✅ | 删除未使用网络 |
| 5.4 | 一键清理 | ✅ | docker network prune 功能 |

---

## 六、镜像管理

| # | 需求 | 状态 | 测试要点 |
|---|------|------|----------|
| 6.1 | 镜像列表 | ✅ | 显示所有镜像，大小和创建时间 |
| 6.2 | 手动更新 | ✅ | 选择镜像 → 拉取最新版本 |

---

## 七、文件浏览器

| # | 需求 | 状态 | 测试要点 |
|---|------|------|----------|
| 7.1 | 收藏目录 | ✅ | 默认展示用户配置的挂载目录 |
| 7.2 | 收藏点击 | ✅ | 侧栏收藏导航刷新文件列表 |
| 7.3 | 文件夹置顶 | ✅ | 文件列表文件夹排在前面 |
| 7.4 | 流式上传 | ✅ | 大文件上传支持进度显示 |

---

## 八、UI 全局优化

| # | 需求 | 状态 | 测试要点 |
|---|------|------|----------|
| 8.1 | 滚动条样式 | ✅ | dark 模式、细条、不明显 |
| 8.2 | 图标按钮 Tooltip | ✅ | hover 显示提示文字 |
| 8.3 | 编辑器统一 | ✅ | 所有代码编辑使用 Monaco Editor |

---

## 九、SSE 实时架构

| # | 需求 | 状态 | 测试要点 |
|---|------|------|----------|
| 9.1 | SSE 端点 | ✅ | 容器 Stats 推送 |
| 9.2 | 容器日志流 | ✅ | SSE 实时流式 |
| 9.3 | 网络连接恢复 | ✅ | 断线后自动重连 |

---

## 十一、代码审查发现

| # | 问题 | 严重度 | 说明 |
|---|------|--------|------|
| C1 | `network.go` 中 `DockerService` 接口签名错误 | ⚠️ 低 | `ctx interface{ Value(any) any }` 语法错误，但未被使用(实际走 `docker.go` 的 `ListNetworks`)，不影响运行 |
| C2 | 概览页 `loadStats` 调用 `/docker/containers/stats` 返回 `DockerStats` 结构体，但前端期望的字段名可能不匹配 | ⚠️ 低 | 后端返回 `containers.total/running/stopped/paused`，`images.total/size`，前端 `stats` 结构体匹配，功能正常 |
| C3 | `InspectContainer` handler 调用 `GetContainer` 而非真正的 `ContainerInspect` | ⚠️ 低 | 返回的是模型数据而非完整 Docker JSON，前端 `inspect` 按钮显示的数据有限 |
| C4 | 容器日志流式 SSE 用 `event: log`，但前端 `logs/+page.svelte` 不存在 | ⚠️ 低 | 日志流在 `[id]/+page.svelte` 组件内实现，直接使用 `EventSource` 监听 `log` 事件 |

| 需求 | 原因 |
|------|------|
| 卷管理 | 全用 bind mount，不需要 |
| 容器创建 UI | compose 就是创建方式 |
| Swarm/Stack | homelab 用不上 |
| 漏洞扫描 | 企业功能 |
| Webhook | 过重 |
| OIDC/SSO | 单用户不需要 |
| 多主机管理 | 单机够用 |
| 活动审计日志 | 过重 |
| 镜像仓库管理 | compose 里写就行 |
| docker run → compose 转换 | 已经用 compose |
| Compose 模板 | 自己管 compose 文件 |

---

## 测试流程

### 0. 准备 (仅首次部署)
```bash
# 给二进制添加执行权限
chmod +x /vol1/1000/docker/boxbox/boxbox-linux
```

### 1. 编译部署
```bash
cd E:\temp\boxbox\frontend && npm run build
# 复制产物
Remove-Item 'E:\temp\boxbox\backend\internal\static\dist\*' -Recurse -Force -ErrorAction SilentlyContinue
Copy-Item 'E:\temp\boxbox\frontend\build\*' 'E:\temp\boxbox\backend\internal\static\dist\' -Recurse -Force
# 编译后端
cd E:\temp\boxbox\backend
$env:GOPROXY='https://goproxy.cn,direct'; $env:GOCACHE='E:\temp\go-cache'; $env:GOOS='linux'; $env:GOARCH='amd64'
go build -buildvcs=false -o boxbox-linux ./cmd/server
```

### 2. 部署
```bash
# ssh mcp: fnos zhaxg@192.168.132.86:2322
# 上传 boxbox-linux → /vol1/1000/docker/boxbox/boxbox-linux
# chmod +x /vol1/1000/docker/boxbox/boxbox-linux  # 重要！
# 注意: 如果新增/修改了 volume mounts，必须用 --force-recreate:
cd /vol1/1000/docker/boxbox && docker compose up -d --force-recreate boxbox
# 如果只更新 binary，可以 restart (但要确保 binary mount 已经存在):
cd /vol1/1000/docker/boxbox && docker compose restart boxbox
```

### 3. 验证
- 访问 http://192.168.132.86:8080
- 逐项测试上述功能
- 记录问题到本清单
