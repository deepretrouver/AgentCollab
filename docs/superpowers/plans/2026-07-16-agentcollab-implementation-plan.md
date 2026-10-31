# AgentCollab 首版实施计划

- 日期：2026-07-16
- 依据：`docs/superpowers/specs/2026-07-16-agentcollab-design.md`
- 目标仓库：`/srv/agentcollab`
- 当前分支：`codex/agentcollab-poc`
- 计划状态：可执行

## 1. 交付目标

在指定 SMB 仓库中交付一个可本机部署、可扩展到局域网的 AgentCollab 首版：

- Hub 在用户 Mac 上运行，浏览器提供类似 Bloome 的人类与 Agent 群聊；
- macOS、Windows、Linux Runner 通过出站 WebSocket 接入；
- Codex、Hermes、OpenClaw 和自定义 CLI Agent 可注册、收任务和回传结果；
- 群内任务可以派发到其他所有者的电脑；
- 协调 Agent 可以创建子任务，让多个 Agent 分轮提案、评审和汇总；
- Runner 按当前系统用户权限自动执行，不逐任务批准；
- 消息、任务、审计、附件、交付物和备份以 SMB 为唯一权威持久化状态；
- Hub 重启、Runner 断线和事件重发不造成任务重复执行。

## 2. 实现架构与技术栈

### 2.1 后端与 Runner

- 语言：Go；安装后以实际 `go version` 的主次版本写入 `go.mod` 和开发环境文档。
- HTTP：Go 标准库 `net/http` 和 Go 1.22+ 路由模式。
- WebSocket：`github.com/gorilla/websocket`。
- SQLite：Hub 使用 `github.com/mattn/go-sqlite3` 和 CGO。实际 SMB 验证表明纯 Go `modernc.org/sqlite` 在当前 Darwin SMB 路径触发未实现的 `Xsrandomdev`；Runner 不依赖 SQLite，仍可进行纯 Go 跨平台构建。
- 密码与令牌：`golang.org/x/crypto`；密码使用 Argon2id，邀请和设备令牌只保存哈希。
- ID：UUIDv7 或等价的时间有序全局 ID；统一封装在 `internal/idgen`。
- 日志：标准库 `log/slog`，JSON 日志、字段脱敏和轮转。
- 配置：YAML；设备密钥与 Hub 业务配置分开。

### 2.2 Web UI

- React + TypeScript + Vite；
- 原生 `fetch` 和 `WebSocket`，首版不引入大型状态框架；
- Vitest + React Testing Library；
- Playwright 负责浏览器端到端验证；
- 构建产物嵌入 Hub 二进制，部署 Hub 时不要求 Node.js。

### 2.3 本机控制接口

Runner 在 `127.0.0.1` 随机端口启动仅回环可访问的控制接口。每个任务生成一次性任务令牌，并只通过子进程环境变量传递：

```text
AGENTCOLLAB_CONTROL_URL=http://127.0.0.1:<port>
AGENTCOLLAB_TASK_ID=<task-id>
AGENTCOLLAB_TASK_TOKEN=<one-task-token>
```

同包提供 `agentcollabctl` 命令，Agent 可以调用：

```text
agentcollabctl progress --message "正在分析"
agentcollabctl delegate --agent <agent-id> --title <title> --prompt-file <file>
agentcollabctl artifact add <path>
agentcollabctl deliver --summary-file <file>
```

这样，协调 Agent 可以委派任务，但不持有 Hub 管理凭据、Runner 设备凭据或 SMB 凭据。

### 2.4 预期仓库结构

```text
AgentCollab/
├── cmd/
│   ├── hub/
│   ├── runner/
│   └── agentcollabctl/
├── internal/
│   ├── adapters/
│   │   ├── codex/
│   │   ├── customcli/
│   │   ├── hermes/
│   │   └── openclaw/
│   ├── artifacts/
│   ├── auth/
│   ├── config/
│   ├── dispatch/
│   ├── domain/
│   ├── events/
│   ├── httpapi/
│   ├── idgen/
│   ├── localcontrol/
│   ├── realtime/
│   ├── runnerclient/
│   ├── runnerexec/
│   ├── secrets/
│   ├── storage/
│   └── store/sqlite/
├── migrations/
├── web/
├── packaging/
│   ├── macos/
│   ├── windows/
│   └── linux/
├── scripts/
├── tests/
│   ├── e2e/
│   ├── fixtures/
│   └── integration/
├── app/
├── config/
├── data/
├── artifacts/
├── task-files/
├── logs/
├── backups/
└── docs/
```

## 3. 执行规则

每个任务按相同节奏完成：

1. 先添加能描述行为的失败测试；
2. 运行目标测试并记录预期失败；
3. 写最小实现；
4. 运行目标测试、相关测试和静态检查；
5. 更新对应文档；
6. 只提交该任务相关文件；
7. 保持工作树干净后进入下一任务。

通用验证命令：

```bash
go test ./...
go test -race ./...
go vet ./...
npm --prefix web test -- --run
npm --prefix web run build
```

实现期间不得：

- 在本机另建项目源码副本作为权威版本；
- 让 Runner 或浏览器直接访问 SMB；
- 在 SMB 不可用时回落到本地业务数据库；
- 把普通聊天消息直接传给 shell；
- 把邀请明文、设备令牌、任务令牌写入日志；
- 为完成任务自动使用 sudo 或 Administrator 提权。

## 4. 里程碑

| 里程碑 | 可验证结果 | 对应任务 |
| --- | --- | --- |
| M0 工程可构建 | Hub、Runner、Web 空壳可构建，SMB 边界可检查 | 1-2 |
| M1 本机聊天闭环 | 登录、工作区、群聊、实时消息可用 | 3-7 |
| M2 本机 Agent 闭环 | Custom CLI 在 Mac 自动执行并回群交付 | 8-12 |
| M3 多 Agent 协作 | Codex/Hermes/OpenClaw 接入，支持子任务与 brainstorming | 13-16 |
| M4 可靠性闭环 | 断线、重启、重复事件、SMB 故障可恢复 | 17-18 |
| M5 跨平台局域网 | Windows/Linux Runner 加入同群完成任务 | 19-21 |
| M6 首版发布 | 安装包、运维文档和验收证据完整 | 22-23 |

## 5. 分步实施任务

### 任务 1：准备工具链与仓库骨架

**新增文件**

- `.gitignore`
- `go.mod`
- `README.md`
- `Makefile`
- `cmd/hub/main.go`
- `cmd/runner/main.go`
- `cmd/agentcollabctl/main.go`
- `internal/buildinfo/buildinfo.go`
- `internal/buildinfo/buildinfo_test.go`
- `docs/ops/dev-environment.md`

**步骤**

1. 用 Homebrew 安装 Go，因为当前主机已有 Node `v22.22.3`、npm `10.9.8` 和 SQLite `3.51.0`，但尚无 Go：

   ```bash
   brew install go
   go version
   ```

2. 初始化模块：

   ```bash
   go mod init agentcollab.local/agentcollab
   ```

3. 先编写 `buildinfo` 测试，要求三个命令都能输出名称、版本、提交号和构建时间。
4. 实现三个最小命令及 Makefile 的 `build`、`test`、`web-build`、`dist` 目标。
5. `.gitignore` 排除 `node_modules/`、本地构建缓存、二进制、临时任务目录和密钥，但不排除 `data/`、`logs/` 等目录本身；运行时文件用各目录内 `.gitkeep` 保留结构。
6. 在 `docs/ops/dev-environment.md` 记录实际 Go、Node、npm、SQLite 和 macOS 版本。

**验证**

```bash
go test ./internal/buildinfo
go build ./cmd/hub ./cmd/runner ./cmd/agentcollabctl
git status --short
```

**提交**

```text
chore: bootstrap AgentCollab Go workspace
```

### 任务 2：配置、目录和 SMB 启动防线

**新增文件**

- `config/hub.example.yaml`
- `config/runner.example.yaml`
- `internal/config/hub.go`
- `internal/config/runner.go`
- `internal/config/config_test.go`
- `internal/storage/layout.go`
- `internal/storage/guard.go`
- `internal/storage/guard_test.go`
- `cmd/hub/commands/doctor.go`
- `scripts/verify-smb-mount.sh`

**行为要求**

- Hub 配置明确 `storage_root`，首版值为 SMB 的 `AgentCollab` 根目录。
- Hub 启动前验证根目录存在、可读写、不是符号链接逃逸，并创建约定子目录。
- 生成并持有 `data/hub.lock`，第二个 Hub 启动必须失败。
- `hub doctor` 检查挂载、写入探针、目录权限、剩余空间和 SQLite 能力。
- Runner 配置只含 Hub URL、设备凭据路径、Runner 本地目录和 Agent 定义；不得出现 SMB 字段。

**测试顺序**

1. 写“有效 SMB 根目录通过、不可写目录失败、根目录外路径失败、第二实例失败”的表驱动测试。
2. 写配置测试，明确 Runner 配置出现 `storage_root` 时拒绝加载。
3. 实现目录守卫和 doctor 命令。

**验证**

```bash
go test ./internal/config ./internal/storage
go run ./cmd/hub doctor --config config/hub.example.yaml
```

**提交**

```text
feat: enforce Hub storage and Runner boundary
```

### 任务 3：SQLite 迁移、单写者存储和备份

**新增文件**

- `migrations/0001_initial.sql`
- `migrations/0002_indexes.sql`
- `internal/store/sqlite/open.go`
- `internal/store/sqlite/migrate.go`
- `internal/store/sqlite/tx.go`
- `internal/store/sqlite/backup.go`
- `internal/store/sqlite/store_test.go`
- `internal/store/sqlite/backup_test.go`
- `cmd/hub/commands/backup.go`

**Schema**

实现设计规格中的十五类表，并额外包含：

- `schema_migrations`；
- `event_sequence`，为每个工作区生成单调递增事件序号；
- `task_attempts` 唯一约束 `(task_id, attempt_no)`；
- `task_events` 唯一约束 `event_id`；
- `messages` 唯一约束 `message_id`；
- `artifacts` 唯一约束 `(task_id, sha256)`。

**SQLite 设置**

连接后逐项验证：

```sql
PRAGMA journal_mode=DELETE;
PRAGMA synchronous=FULL;
PRAGMA foreign_keys=ON;
PRAGMA busy_timeout=5000;
```

写事务由 Hub 内单一写入队列串行化。读取可以并发，但连接池上限保持保守。

**测试顺序**

1. 空目录自动迁移；
2. 重复迁移无副作用；
3. 外键和唯一约束生效；
4. 并发写通过单写队列顺序完成；
5. 备份文件通过 `PRAGMA integrity_check`；
6. 模拟备份中断时不发布半成品。

**验证**

```bash
go test ./internal/store/sqlite
BACKUP_FILE="$(go run ./cmd/hub backup --config config/hub.example.yaml)"
sqlite3 "$BACKUP_FILE" 'PRAGMA integrity_check;'
```

**提交**

```text
feat: add single-writer SQLite storage and backups
```

### 任务 4：领域模型和状态机

**新增文件**

- `internal/domain/user.go`
- `internal/domain/workspace.go`
- `internal/domain/room.go`
- `internal/domain/device.go`
- `internal/domain/agent.go`
- `internal/domain/message.go`
- `internal/domain/task.go`
- `internal/domain/artifact.go`
- `internal/domain/task_state_test.go`
- `internal/events/envelope.go`
- `internal/events/envelope_test.go`
- `internal/idgen/idgen.go`
- `internal/idgen/idgen_test.go`

**行为要求**

- 所有领域 ID 有类型封装，避免把 Agent ID 误当设备 ID。
- 任务状态只允许设计规格中的迁移。
- `event_id`、`task_id + attempt` 和工作区序号分别承担全局去重、执行幂等和客户端补拉。
- Artifact 未完成哈希与数据库提交前不能进入可见状态。

**验证**

```bash
go test ./internal/domain ./internal/events ./internal/idgen
```

**提交**

```text
feat: define AgentCollab domain and event contracts
```

### 任务 5：用户认证、管理员初始化和邀请

**新增文件**

- `internal/auth/password.go`
- `internal/auth/session.go`
- `internal/auth/invite.go`
- `internal/auth/middleware.go`
- `internal/auth/auth_test.go`
- `internal/httpapi/auth_handlers.go`
- `internal/httpapi/invite_handlers.go`
- `internal/httpapi/auth_handlers_test.go`
- `cmd/hub/commands/admin.go`

**接口**

- `POST /api/v1/auth/login`
- `POST /api/v1/auth/logout`
- `GET /api/v1/auth/me`
- `POST /api/v1/workspaces/{id}/invites`
- `POST /api/v1/invites/{code}/accept`

**行为要求**

- 首个管理员由 `hub admin create` 建立，密码从安全输入或环境变量读取，不进入命令历史。
- Session 使用随机不透明令牌和 HttpOnly、SameSite Cookie；数据库只保存令牌哈希。
- 邀请码一次性、带到期时间、可撤销、绑定工作区和角色。
- 所有成功和失败的敏感操作生成审计事件，但不记录明文凭据。

**验证**

```bash
go test ./internal/auth ./internal/httpapi -run 'Auth|Invite'
```

**提交**

```text
feat: add human authentication and workspace invites
```

### 任务 6：工作区、群、成员与消息 REST API

**新增文件**

- `internal/httpapi/router.go`
- `internal/httpapi/workspace_handlers.go`
- `internal/httpapi/room_handlers.go`
- `internal/httpapi/message_handlers.go`
- `internal/httpapi/middleware.go`
- `internal/httpapi/api_test.go`
- `internal/store/sqlite/workspaces.go`
- `internal/store/sqlite/rooms.go`
- `internal/store/sqlite/messages.go`

**接口**

- `GET/POST /api/v1/workspaces`
- `GET/POST /api/v1/workspaces/{id}/rooms`
- `GET/POST /api/v1/rooms/{id}/members`
- `GET/POST /api/v1/rooms/{id}/messages`
- `GET /api/v1/rooms/{id}/timeline?after=<sequence>`

**行为要求**

- 每个查询显式带工作区边界；
- 非成员请求返回 404，避免泄露对象存在性；
- 普通消息只能创建 `kind=chat`，不能附带可执行命令字段；
- 分页稳定，按工作区事件序号补拉。

**验证**

```bash
go test ./internal/httpapi -run 'Workspace|Room|Message|Membership'
```

**提交**

```text
feat: add workspace room and message APIs
```

### 任务 7：浏览器实时事件通道

**新增文件**

- `internal/realtime/broker.go`
- `internal/realtime/client.go`
- `internal/realtime/handler.go`
- `internal/realtime/replay.go`
- `internal/realtime/realtime_test.go`

**接口**

- `GET /api/v1/events` 升级 WebSocket；
- 客户端连接时提交最后已确认事件序号；
- Hub 先写数据库，再广播事件；
- 客户端重连先补拉缺失事件，再进入实时流。

**测试场景**

- 同一群两个客户端即时收到消息；
- 非成员收不到事件；
- 慢客户端被断开但不阻塞 Hub；
- 重连补拉不重复显示；
- Hub 广播失败不回滚已提交业务事务。

**验证**

```bash
go test ./internal/realtime
```

**提交**

```text
feat: stream persisted room events over WebSocket
```

### 任务 8：Web UI 登录、工作区和群聊

**新增文件**

- `web/package.json`
- `web/package-lock.json`
- `web/tsconfig.json`
- `web/vite.config.ts`
- `web/vitest.config.ts`
- `web/index.html`
- `web/src/main.tsx`
- `web/src/app/App.tsx`
- `web/src/api/client.ts`
- `web/src/realtime/socket.ts`
- `web/src/auth/LoginPage.tsx`
- `web/src/chat/WorkspaceSidebar.tsx`
- `web/src/chat/RoomView.tsx`
- `web/src/chat/Composer.tsx`
- `web/src/chat/MessageItem.tsx`
- `web/src/styles/tokens.css`
- `web/src/styles/app.css`
- `web/src/auth/LoginPage.test.tsx`
- `web/src/chat/RoomView.test.tsx`
- `web/src/chat/MessageItem.test.tsx`
- `internal/httpapi/web_embed.go`

**界面验收**

- 登录后左侧显示工作区和群；
- 中间显示消息流和输入框；
- 消息显示真人或 Agent 身份标签；
- WebSocket 断线时显示状态并自动重连；
- 普通发送和“创建任务”是两个不同操作；
- 页面刷新后从 REST 历史和事件序号恢复，不重复消息；
- 360px 宽浏览器可用，但首版不做原生移动客户端。

**验证**

```bash
npm --prefix web install
npm --prefix web test -- --run
npm --prefix web run build
go test ./internal/httpapi -run WebEmbed
```

**提交**

```text
feat: add browser workspace and group chat UI
```

### 任务 9：Runner 邀请绑定、设备凭据和心跳

**新增文件**

- `internal/auth/device_token.go`
- `internal/httpapi/runner_enroll_handlers.go`
- `internal/runnerclient/client.go`
- `internal/runnerclient/enroll.go`
- `internal/runnerclient/heartbeat.go`
- `internal/runnerclient/reconnect.go`
- `internal/runnerclient/client_test.go`
- `internal/store/sqlite/devices.go`
- `internal/store/sqlite/agents.go`

**接口**

- `POST /api/v1/runner/enroll`
- `GET /api/v1/runner/connect` 升级 WebSocket；
- `POST /api/v1/devices/{id}/revoke`；
- `GET /api/v1/workspaces/{id}/agents`。

**行为要求**

- Runner 用一次性邀请码换取设备 ID 和长期设备令牌；
- 设备令牌只在 Runner 本地密钥文件中保存，Hub 只存哈希；
- Runner 主动连接并指数退避重连；
- 心跳包含 Runner 版本、OS、架构、Agent 健康和当前任务；
- 撤销后现有连接立即失效，重连被拒绝。

**验证**

```bash
go test ./internal/runnerclient ./internal/httpapi -run 'Runner|Enroll|Device'
```

**提交**

```text
feat: enroll and authenticate cross-platform runners
```

### 任务 10：任务 API、派发器和幂等 attempt

**新增文件**

- `internal/httpapi/task_handlers.go`
- `internal/dispatch/dispatcher.go`
- `internal/dispatch/registry.go`
- `internal/dispatch/reconcile.go`
- `internal/dispatch/dispatcher_test.go`
- `internal/store/sqlite/tasks.go`
- `internal/store/sqlite/task_events.go`
- `web/src/tasks/TaskComposer.tsx`
- `web/src/tasks/TaskCard.tsx`
- `web/src/tasks/TaskTimeline.tsx`
- `web/src/tasks/TaskComposer.test.tsx`
- `web/src/tasks/TaskCard.test.tsx`
- `web/src/tasks/TaskTimeline.test.tsx`

**接口**

- `POST /api/v1/rooms/{id}/tasks`
- `GET /api/v1/tasks/{id}`
- `POST /api/v1/tasks/{id}/retry`
- `POST /api/v1/tasks/{id}/cancel`

**行为要求**

- 只有结构化任务进入派发器；
- 派发消息包含 `task_id`、`attempt_no`、目标 Agent、输入附件和验收条件；
- Runner 离线时任务保留队列并显示原因；
- Hub 重发同一 attempt 时 Runner 只执行一次；
- retry 创建新 attempt，不覆盖历史；
- 取消有请求、Runner 确认和最终状态三个事件。

**验证**

```bash
go test ./internal/dispatch ./internal/httpapi -run Task
npm --prefix web test -- --run Task
```

**提交**

```text
feat: dispatch idempotent tasks from group chat
```

### 任务 11：Runner 进程执行与 Custom CLI Adapter

**新增文件**

- `internal/adapters/adapter.go`
- `internal/adapters/registry.go`
- `internal/adapters/customcli/config.go`
- `internal/adapters/customcli/runner.go`
- `internal/adapters/customcli/runner_test.go`
- `internal/runnerexec/executor.go`
- `internal/runnerexec/process_unix.go`
- `internal/runnerexec/process_windows.go`
- `internal/runnerexec/executor_test.go`
- `tests/fixtures/fake-agent/main.go`

**Custom CLI 配置**

配置项只接受可执行文件和参数数组：

```yaml
agents:
  - id: local-demo
    type: custom-cli
    command: /absolute/path/to/fake-agent
    args: ["--input", "{{prompt_file}}", "--output", "{{output_dir}}"]
    workdir_policy: task
    output_mode: ndjson
```

不接受 `command: "shell string"`，也不经过 `sh -c`、`cmd /c` 或 PowerShell 拼接。

**NDJSON 输出事件**

- `progress`
- `message`
- `artifact`
- `result`
- `error`

**验证**

```bash
go test ./internal/adapters/customcli ./internal/runnerexec
go test ./tests/integration -run CustomCLIEndToEnd
```

**提交**

```text
feat: execute structured tasks through custom CLI adapter
```

### 任务 12：本机回环控制接口与 agentcollabctl

**新增文件**

- `internal/localcontrol/server.go`
- `internal/localcontrol/token.go`
- `internal/localcontrol/handlers.go`
- `internal/localcontrol/server_test.go`
- `cmd/agentcollabctl/commands/progress.go`
- `cmd/agentcollabctl/commands/delegate.go`
- `cmd/agentcollabctl/commands/artifact.go`
- `cmd/agentcollabctl/commands/deliver.go`
- `cmd/agentcollabctl/commands/commands_test.go`

**安全边界**

- 只监听 `127.0.0.1`；
- 任务令牌绑定任务、attempt、Agent 和 Runner 进程；
- 任务结束立即撤销令牌；
- 子进程不能用令牌操作其他任务；
- `delegate` 只能创建当前任务的子任务；
- 输入文件路径必须位于当前任务工作目录；
- 传给 Hub 的事件仍由 Runner 设备身份签名。

**验证**

```bash
go test ./internal/localcontrol ./cmd/agentcollabctl/commands
go test ./tests/integration -run LocalControl
```

**提交**

```text
feat: let running agents report and delegate locally
```

### 任务 13：Artifact 上传、校验和群内交付

**新增文件**

- `internal/artifacts/path.go`
- `internal/artifacts/upload.go`
- `internal/artifacts/store.go`
- `internal/artifacts/artifacts_test.go`
- `internal/httpapi/artifact_handlers.go`
- `internal/httpapi/artifact_handlers_test.go`
- `web/src/artifacts/ArtifactCard.tsx`
- `web/src/artifacts/ArtifactCard.test.tsx`

**行为要求**

- 任务附件写入 `task-files/<workspace>/<task>/`；
- 交付物写入 `artifacts/<workspace>/<task>/<artifact-id>/`；
- 上传采用临时文件，完成 SHA-256、大小和文件名检查后原子发布；
- 默认单文件上限写入 Hub 配置并在 UI 显示；
- 上传中断的临时文件由清理任务回收；
- 下载再次检查工作区与群成员身份；
- `delivered` 状态只在所有声明 Artifact 成功落盘后提交。

**验证**

```bash
go test ./internal/artifacts ./internal/httpapi -run Artifact
go test ./tests/integration -run ArtifactRoundTrip
```

**提交**

```text
feat: return verified task artifacts to the source room
```

### 任务 14：Codex Adapter

**新增文件**

- `internal/adapters/codex/probe.go`
- `internal/adapters/codex/command.go`
- `internal/adapters/codex/events.go`
- `internal/adapters/codex/codex_test.go`
- `docs/agents/codex.md`

**本机已验证入口**

```text
/Applications/ChatGPT.app/Contents/Resources/codex exec
```

Adapter 使用：

- `exec --json` 获取 JSONL 事件；
- `-C <task-workdir>` 指定任务工作目录；
- 按首版默认执行策略加入 `--dangerously-bypass-approvals-and-sandbox`；
- 提示词通过 stdin 传入，避免超长命令行和转义问题；
- 注入 `agentcollabctl` 用法和任务验收条件；
- 解析最终消息、失败状态和进度事件，不依赖彩色终端文本。

测试用 fixture 模拟 Codex JSONL；真实测试只使用无敏感内容的小任务。

**验证**

```bash
go test ./internal/adapters/codex
go test ./tests/integration -run CodexAdapter
```

**提交**

```text
feat: connect local Codex as an AgentCollab agent
```

### 任务 15：Hermes Adapter

**新增文件**

- `internal/adapters/hermes/probe.go`
- `internal/adapters/hermes/command.go`
- `internal/adapters/hermes/events.go`
- `internal/adapters/hermes/hermes_test.go`
- `docs/agents/hermes.md`

**本机已验证入口**

```text
/Users/example/hermes-agent/.venv/bin/hermes
```

首版使用 Hermes 非交互 CLI：

- `--cli --yolo -z <prompt>`；
- 工作目录由 Runner 设置；
- 用户选择的 Hermes profile 通过 Runner Agent 配置注入环境；
- 不复制或迁移 Hermes 的现有 provider、认证和 profile；
- Adapter 捕获 stdout/stderr，转换为统一进度和最终消息；
- 若 Hermes CLI 缺少稳定结构化流，最终结果以退出码和最后有效输出为准，原始脱敏日志作为调试附件。

Hermes 自身的 `delegate_task` 和 Kanban 能力不被替换；AgentCollab 的跨电脑子任务通过 `agentcollabctl delegate` 进入 Hub。

**验证**

```bash
go test ./internal/adapters/hermes
go test ./tests/integration -run HermesAdapter
```

**提交**

```text
feat: connect local Hermes as an AgentCollab agent
```

### 任务 16：OpenClaw Adapter 与通用探测策略

**新增文件**

- `internal/adapters/openclaw/probe.go`
- `internal/adapters/openclaw/command.go`
- `internal/adapters/openclaw/openclaw_test.go`
- `internal/adapters/probe/path.go`
- `internal/adapters/probe/path_test.go`
- `docs/agents/openclaw.md`

**行为要求**

- 本机当前未发现 `openclaw` 命令，因此不能把未经验证的命令行参数写死；
- 专用 Adapter 提供 OpenClaw 类型、身份标签、健康探测和配置校验；
- `command` 和 `args` 使用 Runner 配置中的参数数组；
- 安装 OpenClaw 的验证机上先运行 `openclaw --help`，再把已验证参数写入示例配置和契约测试；
- 未安装时 Agent 卡显示 `unavailable: executable not found`，Hub 不派发任务；
- 即使专用参数尚未配置，用户仍可通过 Custom CLI 契约接入 OpenClaw，不影响其他 Agent。

**验证**

```bash
go test ./internal/adapters/openclaw ./internal/adapters/probe
go test ./tests/integration -run OpenClawAdapter
```

真实 OpenClaw 验证是首版发布门槛，不以“代码存在”替代实测。

**提交**

```text
feat: add configurable OpenClaw agent integration
```

### 任务 17：父子任务、协调 Agent 和 Brainstorming 流程

**新增文件**

- `internal/dispatch/delegation.go`
- `internal/dispatch/delegation_test.go`
- `internal/domain/brainstorm.go`
- `internal/domain/brainstorm_test.go`
- `internal/httpapi/brainstorm_handlers.go`
- `web/src/brainstorm/BrainstormCard.tsx`
- `web/src/brainstorm/BrainstormRound.tsx`
- `web/src/brainstorm/BrainstormCard.test.tsx`
- `web/src/brainstorm/BrainstormRound.test.tsx`
- `docs/workflows/brainstorming.md`

**首版协议**

Brainstorming 不是无结构的 Agent 群聊，而是协调任务的一种模式：

1. `brainstorm.start`：协调 Agent 选择参与 Agent 和目标；
2. `proposal.submit`：第一轮参与者相互隔离地提交方案；
3. `review.submit`：第二轮对已汇总提案进行批评或补充；
4. `synthesis.submit`：协调 Agent 汇总结论；
5. `brainstorm.delivered`：在原群发布结果和各轮来源；
6. 用户可把结论转为新的执行任务。

**行为要求**

- 每个子任务保留 `parent_task_id` 和创建它的 Agent；
- 协调 Agent 只能委派给当前群可见且在线或可排队的 Agent；
- 第一轮提案完成前不向其他提案者泄露内容；
- 所有子任务结果回到父任务时间线；
- 子任务失败不会丢失其他参与者结果；
- 汇总任务明确列出缺失或失败的参与者；
- 首版自动执行，但 brainstorming 结论本身不会自动变成新电脑操作，必须创建执行任务。

**验证**

```bash
go test ./internal/domain ./internal/dispatch -run 'Brainstorm|Delegation'
go test ./tests/integration -run MultiAgentBrainstorm
npm --prefix web test -- --run Brainstorm
```

**提交**

```text
feat: orchestrate multi-agent brainstorming and child tasks
```

### 任务 18：重连、Hub 重启和 SMB 故障恢复

**新增文件**

- `internal/dispatch/recovery.go`
- `internal/dispatch/recovery_test.go`
- `internal/storage/health.go`
- `internal/storage/health_test.go`
- `internal/runnerclient/outbox.go`
- `internal/runnerclient/outbox_test.go`
- `docs/ops/storage-recovery.md`
- `docs/ops/hub-recovery.md`

**行为要求**

- Runner 使用有界本地 outbox 保存尚未被 Hub 确认的事件；
- outbox 是传输缓冲，不提供业务查询，不覆盖 Hub 状态；
- Hub 重启后，Runner 报告正在执行和已完成未确认的 attempt；
- Hub 与 Runner 按 `task_id + attempt_no` 对账；
- SMB 健康探测失败后 Hub 停止新的权威写入并返回 `503 storage_unavailable`；
- 恢复前执行写探针、SQLite `quick_check` 和进程锁检查；
- 所有补写按 `event_id` 去重；
- UI 持续显示存储故障，而非假装发送成功。

**验证**

```bash
go test ./internal/dispatch ./internal/storage ./internal/runnerclient -run 'Recover|Reconnect|Outbox|Storage'
go test ./tests/integration -run 'HubRestart|DuplicateDispatch|StoragePause'
```

**提交**

```text
feat: recover tasks across disconnects and SMB outages
```

### 任务 19：日志、审计、脱敏与运维诊断

**新增文件**

- `internal/secrets/redact.go`
- `internal/secrets/redact_test.go`
- `internal/observability/logger.go`
- `internal/observability/health.go`
- `internal/httpapi/admin_health_handlers.go`
- `web/src/admin/HealthPage.tsx`
- `web/src/admin/AuditPage.tsx`
- `docs/ops/logging.md`

**行为要求**

- JSON 日志轮转写入 `logs/`；
- 常见 API key、Bearer token、邀请和设备令牌脱敏；
- 审计页可按用户、Agent、设备、任务和时间筛选；
- 健康页显示 Hub、SMB、SQLite、Runner、Agent 和队列状态；
- 普通管理员看不到设备令牌、任务令牌或 Agent provider 密钥；
- 日志损坏不影响业务数据库运行。

**验证**

```bash
go test ./internal/secrets ./internal/observability ./internal/httpapi -run 'Redact|Health|Audit'
npm --prefix web test -- --run 'Health|Audit'
```

**提交**

```text
feat: add redacted logs health checks and audit trail
```

### 任务 20：macOS Runner 安装与本机部署

**新增文件**

- `packaging/macos/com.agentcollab.runner.plist`
- `packaging/macos/install-runner.sh`
- `packaging/macos/uninstall-runner.sh`
- `packaging/macos/install-hub.sh`
- `packaging/macos/uninstall-hub.sh`
- `docs/install/macos.md`
- `scripts/bootstrap-local.sh`

**行为要求**

- Hub 源码和数据仍在 SMB；
- 编译后的 Hub 必须安装到明确的本地运行目录并从本地工作目录启动，配置指向 SMB 权威根目录；源码和业务状态仍在 SMB，本地二进制可重建；
- Runner 安装到本机应用目录，配置与设备凭据位于 `~/.agentcollab/`；
- launchd 使用当前用户会话运行，不以 root 身份安装系统级守护进程；
- 安装脚本支持重复执行；卸载不删除 Hub 数据、任务历史或 Agent 自身配置；
- `bootstrap-local.sh` 完成 doctor、数据库迁移、管理员创建提示、Hub 启动和本机 Runner 绑定。

**验证**

```bash
make dist
packaging/macos/install-hub.sh --check
packaging/macos/install-runner.sh --check
launchctl print gui/$(id -u)/com.agentcollab.runner
```

完成一次真实 Mac 本机任务：浏览器创建任务 → Codex 或 Hermes 执行 → Artifact 回到群。

**提交**

```text
feat: package local macOS Hub and Runner deployment
```

### 任务 21：Windows 与 Linux 服务适配和发行包

**新增文件**

- `internal/service/service.go`
- `internal/service/service_windows.go`
- `internal/service/service_linux.go`
- `internal/service/service_darwin.go`
- `internal/service/service_test.go`
- `packaging/windows/install-runner.ps1`
- `packaging/windows/uninstall-runner.ps1`
- `packaging/linux/agentcollab-runner.service`
- `packaging/linux/install-runner.sh`
- `packaging/linux/uninstall-runner.sh`
- `docs/install/windows.md`
- `docs/install/linux.md`

**构建矩阵**

```bash
GOOS=darwin GOARCH=arm64 go build -o dist/darwin-arm64/agentcollab-runner ./cmd/runner
GOOS=darwin GOARCH=amd64 go build -o dist/darwin-amd64/agentcollab-runner ./cmd/runner
GOOS=windows GOARCH=amd64 go build -o dist/windows-amd64/agentcollab-runner.exe ./cmd/runner
GOOS=windows GOARCH=arm64 go build -o dist/windows-arm64/agentcollab-runner.exe ./cmd/runner
GOOS=linux GOARCH=amd64 go build -o dist/linux-amd64/agentcollab-runner ./cmd/runner
GOOS=linux GOARCH=arm64 go build -o dist/linux-arm64/agentcollab-runner ./cmd/runner
```

**平台要求**

- Windows 安装脚本创建仅属于当前用户、登录时触发、`RunLevel=Limited` 的计划任务，路径使用 Windows 规则，进程树取消可终止子进程；
- Linux 使用 systemd user service，不要求 root；
- 三个平台的 Runner 配置语义一致；
- Agent executable path 和参数数组允许平台差异；
- 安装包包含 Runner、`agentcollabctl`、示例配置和校验和。

**验证**

```bash
go test ./internal/service
make cross-build
```

交叉编译只证明构建兼容，不能代替任务 22 的真实系统验证。

**提交**

```text
feat: package Runner services for Windows and Linux
```

### 任务 22：局域网连接、真实平台矩阵和端到端验收

**新增文件**

- `tests/e2e/chat.spec.ts`
- `tests/e2e/task.spec.ts`
- `tests/e2e/reconnect.spec.ts`
- `tests/e2e/brainstorm.spec.ts`
- `tests/integration/cross_runner_test.go`
- `scripts/e2e/start-test-hub.sh`
- `scripts/e2e/create-test-runner-config.sh`
- `docs/test/platform-matrix.md`
- `docs/install/lan.md`

**局域网步骤**

1. Hub 绑定明确的内网地址，不使用自动广播发现；
2. macOS 防火墙只允许选定 Hub 端口；
3. 生成用于局域网测试的 Hub 地址和邀请码；
4. Windows 和 Linux Runner 用出站连接加入；
5. 浏览器确认设备、OS、所有者和 Agent 渠道标签；
6. 从 Mac 群聊向 Windows 或 Linux Agent 派任务并收回交付物。

**真实系统门槛**

- macOS arm64：Codex 和 Hermes 真实任务；
- Windows amd64：Custom CLI 和取消任务；
- Linux amd64：Custom CLI、断线重连和 systemd 重启；
- OpenClaw：在实际安装主机上至少完成一次任务；
- macOS amd64、Windows arm64、Linux arm64：有设备则实测；无设备只标记“构建通过，未验证”。

**自动化验证**

```bash
go test ./...
go test -race ./...
go vet ./...
npm --prefix web test -- --run
npm --prefix web run build
npm --prefix web run e2e
```

**故障演练**

- Runner 执行中断网后恢复；
- Hub 执行中重启；
- 重发同一 dispatch；
- SMB 短时断开再挂载；
- 设备凭据撤销；
- Agent executable 被移除；
- 大文件、损坏文件和上传中断；
- 群成员被移除后继续请求任务。

每项在 `docs/test/platform-matrix.md` 记录日期、系统、Runner 版本、结果和证据路径。

**提交**

```text
test: verify LAN and cross-platform collaboration
```

### 任务 23：备份恢复演练、发布清单和首版标记

**新增文件**

- `docs/ops/backup-restore.md`
- `docs/ops/troubleshooting.md`
- `docs/release/v0.1.0-checklist.md`
- `CHANGELOG.md`
- `scripts/release/check.sh`
- `scripts/release/checksums.sh`

**发布检查**

1. 在 SMB `backups/` 生成一致性备份；
2. 在隔离测试目录恢复 Hub 数据；
3. 对恢复数据库执行 `integrity_check`；
4. 确认恢复后消息、任务、审计和 Artifact 索引一致；
5. 确认当前 SMB 根目录外没有权威业务数据库；
6. 运行完整测试和跨平台构建；
7. 生成每个发行包的 SHA-256；
8. 在全新本机用户环境按文档完成一次安装；
9. 在群内完成“协调 Agent → 远端 Agent → Artifact → 汇总”的验收任务；
10. 只有所有首版门槛通过后才创建 `v0.1.0` 标签。

**验证**

```bash
scripts/release/check.sh
git status --short
```

**提交**

```text
docs: finalize AgentCollab v0.1.0 operations and release evidence
```

## 6. 最小可运行切片

完整首版较大，第一次可运行部署不等待全部任务完成。按以下切片验收：

### Slice A：本机浏览器群聊

完成任务 1-8。结果是 Hub 在 SMB 上保存状态，用户能登录、建群和实时聊天。

### Slice B：本机 Agent 自动执行

完成任务 9-13。结果是 Custom CLI Agent 能在 Mac 自动接任务并把文件返回群聊。

### Slice C：真实 Codex/Hermes 与多 Agent 协作

完成任务 14-17。结果是 Codex、Hermes 和已配置 OpenClaw 可加入群，协调 Agent 能跨 Agent 委派和 brainstorming。

### Slice D：可恢复的局域网跨平台首版

完成任务 18-23。结果是 Windows/Linux Runner 可加入，故障可恢复，并具备安装、备份和发布证据。

每个 Slice 完成时必须进行一次实际浏览器验收，不能只依赖单元测试。

## 7. 风险与停止条件

出现以下情况时停止继续叠加功能，先解决根因：

- SMB 上 SQLite 的锁或一致性测试无法稳定通过；
- Hub 在 SMB 短断线后产生无法解释的状态差异；
- 同一 task attempt 被执行两次；
- Runner 的设备凭据或任务令牌出现在日志；
- 普通群聊可以绕过结构化任务触发本机命令；
- Windows/Linux 只能编译、无法在真实系统运行；
- Agent Adapter 依赖解析彩色终端文本且无法稳定识别最终结果；
- Artifact 数据库记录与磁盘文件不一致。

若 SMB SQLite 的真实故障演练不稳定，首选替代不是本地回落，而是把数据库服务化：在单 Hub 主机运行 PostgreSQL，并将经验证可放置的备份和 Artifact 留在 SMB；变更前必须更新设计规格并取得确认。

## 8. 首次实施批次

下一次开始编码时只执行任务 1-3：

1. 安装并记录 Go 工具链；
2. 建立可构建的 Hub、Runner、agentcollabctl 骨架；
3. 落实 SMB 启动防线；
4. 完成 SQLite 迁移、单写者和备份测试；
5. 提交每个任务并报告测试证据。

完成任务 1-3 后进行一次短验收，再进入领域、认证和群聊实现，避免在存储基础不可靠时扩大代码量。
