# etcd-admin

基于 Go + [CloudWeGo Hertz](https://github.com/cloudwego/hertz) 的 **etcd v3 可视化管理面板**。后端以 etcd 官方 gRPC 客户端连接目标集群，前端为零依赖的原生 HTML/CSS/JS，编译期通过 `go:embed` 把全部静态资源打入二进制，**部署只需分发单个可执行文件，运行时不依赖任何外部目录**。

## 功能一览

- **连接管理**：启动时自动探测 etcd，失败不退出进程（lazy 模式后台重连）；页面上可测试连接、热切换集群、主动断开，连接信息只存服务端内存，重启即失效
- **键值管理**：前缀游标分页、按 `/` 分层的键树浏览、键名/键值包含搜索、单键详情、JSON 美化与高亮、历史版本回溯与行级 Diff（LCS）、导入/导出 JSON、事务原子批量删除、本地书签
- **实时监听**：基于 SSE（`text/event-stream`）的 Watch 事件推送，15 秒心跳保活，支持前缀监听、自动滚动、离开页面自动释放 watcher
- **租约管理**：租约列表、授予（Grant）、单次续期（KeepAliveOnce）、吊销（Revoke），卡片内本地秒级 TTL 倒计时，每 10 秒回源校准
- **认证与 RBAC**：auth 开关、用户增删改密、角色增删、用户授撤角色、角色授撤键权限（READ / WRITE / READWRITE，支持前缀）
- **集群维护**：多端点状态与成员列表、告警查询/解除（NOSPACE）、历史版本压缩（Compact）、逐端点碎片整理（Defragment）
- **指标监控**：后端每 10 秒采样一次，内存保留最近 90 个点（约 15 分钟），前端纯 SVG 绘制 DB 大小、Revision、键数量、租约数量四张趋势图，5 秒轮询刷新
- **界面体验**：深色/浅色双主题、玻璃拟态风格、移动端抽屉式导航（响应式断点 1024px / 860px）、Toast 与自定义 Modal（无原生 alert）

## 技术栈

| 组件 | 版本 / 说明 |
| --- | --- |
| Go | 1.26.0（`go.mod` 声明） |
| Web 框架 | github.com/cloudwego/hertz v0.10.6（默认 netpoll 网络库，自带 panic recovery 中间件） |
| etcd 客户端 | go.etcd.io/etcd/client/v3 v3.7.3（gRPC，依赖 google.golang.org/grpc v1.83.2） |
| 前端 | 原生 HTML + CSS + JavaScript，无框架、无 npm、无构建步骤 |
| 资源嵌入 | Go 标准库 `embed`（`//go:embed`） |
| 容器 | 两阶段 Dockerfile，`CGO_ENABLED=0` 纯静态二进制，Alpine 运行镜像 |

## 目录结构

```
etcd-admin/
├── main.go                     # 进程入口：配置加载、连接初始化、采样器、Hertz 启动、优雅退出
├── go.mod / go.sum
├── Dockerfile                  # 两阶段构建，运行镜像仅含单个二进制
├── docker-compose.yml          # 端口 8769:8080，依赖外部网络 etcd-internal
├── .github/workflows/          # release 分支推送镜像到 ghcr.io 并回调 webhook
├── internal/
│   ├── config/config.go        # 环境变量配置加载
│   ├── etcdcli/
│   │   ├── client.go           # Client：对 clientv3.Client 的薄封装
│   │   └── manager.go          # Manager：RWMutex 保护的连接生命周期管理
│   ├── handler/                # HTTP 业务处理器（7 个 Handler + 统一响应）
│   │   ├── response.go         # APIResp、ok/fail、哨兵错误
│   │   ├── connection.go       # 连接查询/测试/切换/断开
│   │   ├── kv.go               # 键值列表/详情/写入/删除/批量删除
│   │   ├── kv_browser.go       # 键树/搜索/历史/导入/导出
│   │   ├── lease.go            # 租约 CRUD 与续期
│   │   ├── cluster.go          # 集群状态/告警/压缩/碎片整理
│   │   ├── auth.go             # auth 开关、用户、角色、权限
│   │   ├── watch.go            # SSE Watch 推送
│   │   └── metrics.go          # 指标时序与实时概览
│   ├── router/
│   │   ├── router.go           # 全部路由注册与静态资源缓存策略中间件
│   │   └── static.go           # 内嵌静态资源处理器（MIME/HEAD/404/防穿越）
│   └── sampler/sampler.go      # 内存环形采样器（容量 90，10 秒间隔）
└── web/
    ├── web.go                  # //go:embed static entry.html index.html
    ├── entry.html              # 未连接时的连接入口页
    ├── index.html              # 主控制台（7 个视图单页）
    └── static/
        ├── css/style.css       # 681 行，双主题变量 + 响应式
        └── js/                 # 9 个 IIFE 模块，挂载到 window.App
```

## 快速开始

### 前置要求

- Go 1.26.0+（本地构建）
- 一个可通过 gRPC 访问的 etcd v3 集群（默认端口 2379）

### 本地运行

```bash
# 直接运行
go run .

# 或构建出单二进制后运行（前端资源已嵌入，可拷贝到任意目录执行）
go build -o etcd-admin .
./etcd-admin
```

启动后监听 `:8080`，浏览器访问 <http://localhost:8080/>：

- 后端持有可用连接时，`/` 返回主控制台 `index.html`
- 未持有连接时，`/` 返回连接入口页 `entry.html`；入口页也始终可通过 `/entry` 访问

### 配置（全部来自环境变量）

配置加载逻辑见 [internal/config/config.go](internal/config/config.go)：`SERVER_ADDR` 空串回落内置默认值；`ETCD_ENDPOINTS` 按逗号拆分并逐项 Trim，空项直接丢弃；用户名与密码原样读取不做 Trim；`STATIC_CACHE_MAX_AGE` 经 `strconv.Atoi` 解析，非法或非正数回落 0。

| 环境变量 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `ETCD_ENDPOINTS` | 否 | 空 | etcd gRPC 端点，多个用英文逗号分隔；为空时进程仍正常启动但不自动连接，浏览器访问后进入连接入口页手动填写 |
| `SERVER_ADDR` | 否 | `:8080` | Hertz HTTP 监听地址 |
| `ETCD_USERNAME` | 否 | 空（匿名） | etcd 认证用户名，直接读取不做 Trim 回落 |
| `ETCD_PASSWORD` | 否 | 空 | etcd 认证密码，直接读取不做 Trim 回落 |
| `STATIC_CACHE_MAX_AGE` | 否 | `0` | `/static` 资源浏览器缓存秒数。`0` 为开发模式（`no-cache, no-store, must-revalidate` 每次回源）；正数为生产模式（`public, max-age=N, immutable`）。Docker 镜像内置 `31536000`（一年） |

示例：

```bash
SERVER_ADDR=":8080" \
ETCD_ENDPOINTS="127.0.0.1:2379,127.0.0.1:2381" \
ETCD_USERNAME="root" \
ETCD_PASSWORD="secret" \
./etcd-admin
```

> 未设置 `ETCD_ENDPOINTS` 时，启动日志会打印 `初始连接 etcd 失败 ([]): etcdclient: no available endpoints` 警告，这是预期行为：HTTP 服务正常监听，在入口页手动连接即可。

### 容器运行

```bash
# 构建镜像
docker build -t etcd-admin .

# 运行（容器内监听 8080，映射到宿主 8769）
docker run -d --name etcd-admin \
  -p 8769:8080 \
  -e ETCD_ENDPOINTS="your-etcd:2379" \
  etcd-admin
```

镜像内置 `STATIC_CACHE_MAX_AGE=31536000`，静态资源默认走一年期 immutable 长缓存。需要回退到每次回源时显式覆盖即可：

```bash
docker run -d --name etcd-admin \
  -p 8769:8080 \
  -e ETCD_ENDPOINTS="your-etcd:2379" \
  -e STATIC_CACHE_MAX_AGE=0 \
  etcd-admin
```

使用仓库自带的 compose（注意它依赖**已存在的外部 Docker 网络** `etcd-internal`，且未注入任何环境变量，容器启动后处于未连接状态，需在入口页填写 etcd 地址；也可自行在 compose 中增加 `environment: ETCD_ENDPOINTS`）：

```bash
docker compose up -d
```

## 页面使用指南

主控制台侧边栏共 7 个视图，移动端（宽度 ≤ 860px）侧边栏收起为抽屉，通过顶部 ☰ 按钮呼出。

1. **仪表盘**：各端点健康卡片（版本、DB 大小、Revision、Leader、Learner）、集群成员表（Leader/Learner 角标）、告警查看与解除、Compact 与 Defragment 维护操作（前端二次确认弹窗）
2. **键值浏览**：左侧键树（按 `/` 分层，目录点击折叠/按前缀浏览，叶子直接定位单键）+ 书签区；右侧分页表格（每页 100 条，游标加载更多）。点击 Key 打开详情弹窗，支持 JSON 美化、复制值、历史版本 Diff、编辑；顶部支持新建键、含值搜索、JSON 导入导出、勾选事务批量删除
3. **实时监听**：输入前缀后建立 SSE 连接（留空监听全集群），事件流展示 PUT/DELETE、键值、modRevision、version、lease 与时间戳，支持自动滚动与一键清空
4. **租约管理**：创建 TTL 租约、查看每个租约剩余时间与绑定键、续期一次、吊销（吊销会连带删除绑定键，需确认）
5. **用户与角色**：查看 auth 开关状态并开启/关闭；用户新建、改密、删除、授撤角色；角色新建、删除、授撤键权限
6. **指标监控**：6 张实时指标卡片 + 4 张 SVG 趋势图，进入页面立即拉取并每 5 秒轮询，离开自动停止
7. **连接设置**：查看当前连接详情，对目标端点做不影响当前连接的连通性测试，或保存并切换到新连接；顶栏「断开连接」会清空服务端持有的客户端并跳回入口页

前端持久化在浏览器 `localStorage`，共三个键：

| Key | 内容 |
| --- | --- |
| `etcd-console-theme` | 主题（dark / light），默认 dark |
| `etcd-bookmarks` | 键值书签数组 |
| `etcd-watch-prefix` | 上次使用的 Watch 前缀 |

## HTTP API 参考

所有接口统一前缀 `/api`，统一响应信封：

```json
{ "code": 0, "message": "ok", "data": { } }
```

`code = 0` 表示成功；失败时 `code` 等于 HTTP 状态码（400 / 404 / 500 / 502 / 503），`message` 为错误文本，`data` 省略。所有 int64/uint64 标识（租约 ID、Leader、成员 ID）均序列化为**字符串**，避免超出 JS `Number.MAX_SAFE_INTEGER` 时精度丢失。

路由按是否依赖 etcd 连接分两层：`/api/health`、`/api/status`、`/api/connection/*` 为公共路由，未连接时也可访问（用于探测、建立、断开连接）；其余业务路由统一挂载 `RequireConnection` 守卫，当 Manager 未持有客户端时直接返回 `503` 与 `{"code":503,"message":"etcd 未连接，请先在连接入口页建立连接"}`，请求不会进入具体 handler。

### 健康与连接管理

| 方法 | 路径 | 说明 | 关键参数 |
| --- | --- | --- | --- |
| GET | `/api/health` | 存活探针，固定返回 ok | - |
| GET | `/api/status` | 轻量连接状态（入口页用） | - |
| GET | `/api/connection/current` | 当前连接信息 + 首端点实时 Status | - |
| POST | `/api/connection/test` | 一次性探测，不改变当前连接，10s 超时 | body: `{endpoints, username, password}` |
| POST | `/api/connection/connect` | 建立新连接并原子替换旧连接，10s 超时 | 同上 |
| POST | `/api/connection/disconnect` | 断开并释放客户端，进入未连接状态 | - |

`endpoints` 为逗号分隔字符串，服务端拆分并 Trim 空白；空列表返回 400（`endpoints 不能为空`）。

### 集群与维护

| 方法 | 路径 | 说明 | 关键参数 |
| --- | --- | --- | --- |
| GET | `/api/cluster/status` | 逐端点 Status + 成员列表 | - |
| GET | `/api/cluster/alarms` | 当前告警列表 | - |
| POST | `/api/cluster/alarms/disarm` | 解除 NOSPACE 告警 | body: `{memberId?}`，缺省解除全部 |
| POST | `/api/maintenance/compact` | 压缩至指定 revision | body: `{revision}`，必须 > 0 |
| POST | `/api/maintenance/defrag` | 对所有端点逐个碎片整理，返回每端点结果 | - |

### 键值管理

| 方法 | 路径 | 说明 | 关键参数 |
| --- | --- | --- | --- |
| GET | `/api/kv` | 前缀分页列表 | query: `prefix`、`limit`（默认 100，上限 500）、`keysOnly`、`start`（游标，取上页 `nextKey`） |
| GET | `/api/kv/tree` | 按 `/` 分层的键树 | query: `prefix`；扫描上限 5000 键，超限返回 `truncated:true` |
| GET | `/api/kv/search` | 包含匹配搜索 | query: `keyword`（必填）、`prefix`、`inValue` |
| GET | `/api/kv/history` | 历史版本回溯 | query: `key`（必填）、`limit`（默认 30，上限 100） |
| GET | `/api/kv/detail` | 单键完整内容 | query: `key`（必填），不存在返回 404 |
| GET | `/api/kv/export` | 导出前缀下键值为 JSON 附件 | query: `prefix`；文件名 `etcd-<prefix>.json` |
| PUT | `/api/kv` | 写入/更新键 | body: `{key, value, leaseId?}`，`leaseId` 为字符串，`"0"`/空表示永久键 |
| DELETE | `/api/kv` | 删除单键或前缀 | query: `key`（必填）、`byPrefix` |
| POST | `/api/kv/import` | 批量导入 | body: `{overwrite:bool, items:[{key,value,leaseId?}]}`；非覆盖模式用事务保证仅写不存在的键 |
| POST | `/api/kv/batch-delete` | 事务原子批量删除 | body: `{keys:[]}`，返回 `{deleted, committed}` |

### 实时监听（SSE）

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/watch` | 建立 `text/event-stream` 长连接，query: `prefix`（留空全量监听） |

事件流约定（每条 `data:` 为一个 JSON）：

| 事件 type | 含义 |
| --- | --- |
| `CONNECTED` | watcher 建立成功，携带 `prefix` |
| `PUT` / `DELETE` | etcd 变更事件，含 `key`、`value`（仅 PUT）、`modRevision`、`version`、`lease`、`timestamp`（毫秒） |
| `ERROR` | watch 通道错误，推送后服务端关闭连接 |

无事件时服务端每 15 秒发送 SSE keep-alive 注释帧；客户端断开即自动取消 watcher 释放资源。

### 租约管理

| 方法 | 路径 | 说明 | 关键参数 |
| --- | --- | --- | --- |
| GET | `/api/leases` | 全部租约及 TTL、授予 TTL、绑定键 | - |
| POST | `/api/leases` | 创建租约 | body: `{ttl}` 秒，必须 > 0 |
| POST | `/api/leases/:id/keepalive` | 续期一次（KeepAliveOnce） | path: `id` |
| DELETE | `/api/leases/:id` | 吊销租约，绑定键同步删除 | path: `id` |

### 认证、用户与角色

| 方法 | 路径 | 说明 | 关键参数 |
| --- | --- | --- | --- |
| GET | `/api/auth/status` | auth 开关与 authRevision | - |
| POST | `/api/auth/enable` `/api/auth/disable` | 开启/关闭认证 | - |
| GET | `/api/auth/users` | 用户列表及角色 | - |
| POST | `/api/auth/users` | 创建用户 | body: `{name, password}` |
| DELETE | `/api/auth/users` | 删除用户 | query: `name` |
| POST | `/api/auth/users/password` | 修改密码 | body: `{name, password}` |
| POST | `/api/auth/users/roles` | 授予角色 | body: `{user, role}` |
| DELETE | `/api/auth/users/roles` | 撤销角色 | query: `user`、`role` |
| GET | `/api/auth/roles` | 角色列表及权限 | - |
| POST | `/api/auth/roles` | 创建角色 | body: `{name}` |
| DELETE | `/api/auth/roles` | 删除角色 | query: `name` |
| POST | `/api/auth/roles/permissions` | 授予键权限 | body: `{role, key, prefix, perm}`，`perm` 取 `read`/`write`/`readwrite` |
| DELETE | `/api/auth/roles/permissions` | 撤销键权限 | query: `role`、`key`、`rangeEnd` |

### 指标

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/metrics/series` | 内存中的采样序列（最多 90 个点） |
| GET | `/api/metrics/overview` | 一次聚合端点状态、键/租约/成员/告警计数与全部采样点 |

### 页面与静态资源

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET / HEAD | `/` | 按连接状态动态返回 `index.html` 或 `entry.html`，始终带 `Cache-Control: no-cache` |
| GET / HEAD | `/entry` | 始终返回连接入口页，始终带 `Cache-Control: no-cache` |
| GET / HEAD | `/static/*filepath` | 内嵌静态资源，MIME 按扩展名判定并兜底内容嗅探；缓存策略由 `STATIC_CACHE_MAX_AGE` 控制（开发态 `no-cache, no-store, must-revalidate`，生产态 `public, max-age=N, immutable`） |

## 架构与关键实现

### 启动流程（[main.go](main.go)）

1. `config.Load()` 从环境变量装配配置
2. `etcdcli.NewManager` 建立连接：内部 `dial` 先 `clientv3.New`（DialTimeout 8 秒）再对首个端点做一次 8 秒超时的 `Status` 探测，确保连接真实可用
3. 探测失败时**进程不退出**：记录警告日志并改用 `NewLazyManager`——跳过连通性校验直接构造客户端，由 gRPC 在后台持续重连；若连客户端构造都失败，则得到一个 `cli == nil` 的空 Manager，等待页面手动重连
4. 启动采样器 goroutine：容量 90、间隔 10 秒，启动立即采一次
5. `server.Default` 创建 Hertz（内置 recovery 中间件，panic 会打印堆栈并返回 500，不会拖垮进程），注册路由后在 goroutine 中 Run
6. 阻塞等待 `SIGINT/SIGTERM`，收到后依次停止采样器、`h.Shutdown`、关闭 etcd 客户端

### 连接切换的并发安全（[internal/etcdcli/manager.go](internal/etcdcli/manager.go)）

`Manager` 用 `sync.RWMutex` 保护客户端指针。`Reconnect` 采用「先建新连、成功后替换」策略：新连接探测失败时旧连接原样保留；替换成功后才异步关闭旧客户端。`Disconnect` 先置空指针再关闭客户端，避免请求打到正在关闭的连接上。

### 连接守卫中间件（[internal/handler/connection.go](internal/handler/connection.go) 与 [internal/router/router.go](internal/router/router.go)）

业务 handler 全部通过 `manager.Get()` 取客户端后直接解引用，断开连接后客户端为 nil。为避免在每个 handler 内重复判空，路由注册时把全部业务路由收口到 `api.Group("/", handler.RequireConnection(m))` 子组：中间件统一检查 `m.IsConnected()`，未连接即 `AbortWithStatusJSON(503, ...)` 中止处理链；连接管理类路由不挂载该守卫，保证断连状态下仍可重新建立连接。Hertz 自带的 recovery 中间件仍作为最后一道兜底。

### 采样器（[internal/sampler/sampler.go](internal/sampler/sampler.go)）

纯内存环形缓冲：每个样本含 `ts/dbSize/revision/keys/leases/members`，首个端点 Status 失败时直接跳过该轮（不产生脏数据点），序列超容量后丢弃最旧样本。进程重启历史即清空，设计取舍见文末已知问题。

### 静态资源嵌入（[web/web.go](web/web.go) 与 [internal/router/static.go](internal/router/static.go)）

`web/web.go` 以 `//go:embed static entry.html index.html` 显式声明嵌入内容，因此 `.hertz.gz` 等本地产物不会进入二进制。由于 Hertz 原生 `app.FS` 仅支持本地 OS 文件系统，静态请求由自定义处理器服务：`path.Join` 清洗 URL 路径并配合 `embed.FS` 自身校验防止目录穿越，HEAD 请求只回响应头。

缓存策略按环境分两档（`router.go` 中间件读取 `STATIC_CACHE_MAX_AGE`）：

- **开发模式（值为 0，默认）**：`/static/` 响应带 `Cache-Control: no-cache, no-store, must-revalidate` 与 `Pragma/Expires`，浏览器每次回源，本地改 JS/CSS 即时生效
- **生产模式（值为正整数，Docker 镜像内置 31536000）**：返回 `public, max-age=N, immutable`。两个 HTML 入口页（`/`、`/entry`）始终带 `Cache-Control: no-cache`，保证页面里带 `?v=` 版本号的资源引用每次都拿到最新值；资源内容变更时随 HTML 一起升级版本号，URL 变化即新缓存条目，旧资源自然过期。`entry.html` 与 `index.html` 的全部 12 处静态引用均已带统一版本号

### 前端模块

9 个 JS 文件均为 IIFE，通过 `window.App` 暴露公共能力（`api/get/post/put/del`、Toast、Modal、主题、格式化、JSON 高亮、LCS 行级 Diff、SVG 折线图）与 `pages` 页面注册表。所有动态 HTML 渲染前经 `esc()` 做 HTML 实体转义；视图切换派发 `view:enter` / `view:leave` 事件，各页面据此启停轮询与 SSE，避免后台空转。

## 构建、验证与发布

```bash
go build ./...      # 编译全部包
go vet ./...        # 静态检查
go test ./...       # 当前仓库尚无测试文件，各包输出 [no test files]
```

Docker 镜像由 [.github/workflows/docker-image.yml](.github/workflows/docker-image.yml) 在 **push 到 `release` 分支**时自动构建并推送至 `ghcr.io/<owner>/<repo>:latest`，随后向作者自建服务回调两个部署 webhook。

## 已知问题与风险（基于源码现状整理）

1. **管理面板自身没有任何鉴权**：能访问 HTTP 端口的调用方即可执行删键、Compact、Defrag、关闭 auth 等高危操作；etcd 账号密码以明文保存在 `Manager` 内存字段中。只能借助网络隔离/反向代理鉴权兜底，不建议暴露到公网。
2. **大规模集群存在 5000 键硬上限**：`scanCap = 5000` 同时约束键树、搜索与导出；键树超限有 `truncated` 提示，但搜索与导出没有显式截断标记。搜索是服务端拉回上限内数据后做内存 `strings.Contains` 匹配，并非 etcd 服务端检索，结果可能不完整。
3. **历史版本接口延迟随深度线性增长**：`/api/kv/history` 通过逐轮 `WithRev(rev-1)` 串行 GET 回溯，最多 100 次 gRPC 往返。
4. **指标只存内存且窗口固定**：重启即丢失，仅覆盖约 15 分钟（90 点 × 10 秒），无法满足长期容量规划；这是为适配 frp 隧道无法访问 etcd 原生 `/metrics` 的取舍。
5. **静态资源版本号依赖手工维护**：生产模式 immutable 长缓存的正确性，依赖两个 HTML 中的 `?v=` 版本号随资源内容同步升级；修改 `web/static` 后若忘记同步版本号，已访问过的浏览器在 `max-age` 窗口内会持续使用旧缓存，应急可设置 `STATIC_CACHE_MAX_AGE=0` 回退到每次回源。
6. **硬编码残留**：`entry.html` 表单默认值（主机 `etcd`、端口 `2379`）、`kv.js` 中键树对名为 `kitex` 的路径默认展开的特判。


9. **SSE 无连接数限制**：每个 `/api/watch` 连接对应一个 etcd watcher，空前缀即监听全集群，大量并发监听会给 etcd 带来压力。

