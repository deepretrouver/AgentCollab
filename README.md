# AgentCollab

## 68 秒真实演示

[观看 / 下载演示视频（MP4）](docs/product/videos/agentcollab-real-demo.mp4)

中文界面，静音技术初剪：在隔离的单机环境中，自动化提交真实任务，展示 OpenCode、Hermes 回群及上下文追问。采样回放节奏经过调整，不代表响应速度；确认任务不调用工具或读取文件，执行凭证未展开。不是跨机、完整技能或正式安装包验收。[演示说明](docs/product/videos/README.md)

[English](README.en.md) · [中文快速开始](docs/QUICKSTART.zh-CN.md) · [首次协作指南](docs/USER_GUIDE.zh-CN.md) · [发布验收状态](docs/ops/open-source-readiness.md)

AgentCollab 是一个自托管的人类与多种 Agent 群聊协作平台。Hub 提供工作区、群聊、任务、调度和交付物控制面；每台执行电脑通过出站连接运行 Runner，并接入本机 Codex、Hermes、OpenClaw 或自定义命令行 Agent。

社区版本正在准备公开预览，尚未完成所有平台安装验收。安装、邀请、Agent 接入、项目看板、本机目录、Git 审查、验收和排障流程见 [`docs/USER_MANUAL.md`](docs/USER_MANUAL.md)；其中历史版本信息将随发布验收更新。

## 先理解三个入口

| 入口 | 谁需要 | 做什么 |
| --- | --- | --- |
| Server | 团队指定的一台主机 | 保存账号、群聊、项目与文件，提供团队地址 |
| 浏览器 | 参与讨论、分工和审查的真人 | 打开邀请链接、登录或注册；无需安装 Client |
| Client | 要贡献自己电脑上 Agent 的成员 | 连接本机、发现 CLI、接收任务并交付结果 |

即使 Server 装在你的电脑上，接入本机 Agent 也使用 Client。不要让每个成员安装
Server。首次从一个已安装、已登录的 CLI 开始，确认回复成功后再增加其他 Agent。
应用不要求加入开机登录项；电脑重启后手动打开需要运行的应用。

## 当前阶段

仓库正在按以下已批准文档实施首版：

- `docs/superpowers/specs/2026-07-16-agentcollab-design.md`
- `docs/superpowers/plans/2026-07-16-agentcollab-implementation-plan.md`

当前本地版本已经具备登录、工作区、群聊、Agent 扫描/注册、真实 CLI 任务派发、WebSocket 实时事件、断线补放、执行凭证和内嵌 Web UI。Hub 支持 Hermes、Codex、WorkBuddy/CodeBuddy、Claude Code、OpenCode 与 OpenClaw；Runner 只注册本机实际可执行的 Agent。

这里的“支持”指存在适配实现，不等于本次发布已逐项验收。当前 OpenCode 已在
单机隔离环境通过真实任务回群、项目审查/合并后校验，以及桌面注册入群；Hermes
已通过限制工具的单机真实确认，以及与 OpenCode 同话题点名和上下文追问，尚未验收完整技能与自主委派；Codex
在本机真实调用中仍有超时，其他原生 Agent 尚未完成本轮端到端验收。请先阅读
[原生 Agent 验证矩阵](docs/agents/compatibility.md)。已有 Mac 本机验证也不能代替
Windows、Intel Mac 或跨机局域网验收。

## 群聊协作模式

1. 在工作区中查找并加入本机 Agent；已在其他工作区注册的同一 Runner Agent 可直接复用，无需重新配对。加入后它们进入该工作区的 Agent 库。
2. 新建群聊时选择一个或多个 Agent，或在群详情中从 Agent 库加入。
3. 在输入框上方点击 Agent 后发送消息。点击“全体”，或以“大家 / 每个人 / 所有人 / @全体”开头，可让群内全部 Agent 分别响应。浏览器提交原文和结构化 Agent ID，Hub 保存原文并创建真实 Runner 任务；内部调度 Prompt 不显示在聊天时间线。
4. Agent 若需要另一位 Agent 继续工作，可在回复的单独一行写 `@完整显示名`。Hub 会创建有父子关系的接力任务，并在专家完成后自动把结果回送给委派 Agent。
5. 在左侧群聊成员中展开自己的 Agent，点击“详情 / 群内设置”，可设置本群响应方式和角色：
   - `仅被 @ 时`：只执行明确点名的消息；
   - `主协调 / 自动分诊`：接收普通真人消息，负责拆解、点名专家并汇总专家回传；
   - `回复所有真人消息`：接收所有真人消息；
   - `静默`：保留在群中但不参与调度。

发送 `/brainstorm [议题]` 可发起一次群聊级专题讨论：平台先让唯一的主协调 Agent 开题，再召集群内其他非静默 Agent 独立分析，等全部回传后自动唤醒主协调完成最终汇总。它只作用于当前议题，不会把参与 Agent 改成常驻自动回复；群内尚未设置主协调时会拒绝启动并提示设置。

每个 Runner 会在注册时声明目标电脑上可用的 Skill 与本地资源别名。资源路径由目标 Runner 解析；这不是对所有日志和回复的脱敏保证，执行凭证或原生 Agent 回复仍可能包含本机路径。群聊中可直接发送 `@Agent 请使用你本机的 stock-analysis Skill 和 research-db 完成……`；任务会在该 Agent 所在电脑执行，Runner 在启动真实 CLI 前注入本机 Skill 目录和资源路径，并在交付的折叠执行凭证中显示执行设备、能力目录数量以及任务/交付实际提及的能力名称。

不同 Agent 由 Runner 并行执行；同一个 Agent 的任务保持串行，以免原生 session 并发写入。Agent 接力有根消息、父任务、协作深度和单根任务总量限制，防止相互无限点名。`/new`、`/clear`、`/restart` 和 `/reset` 会切断后续群聊上下文，并清除该群的原生 Agent session。

Codex、Claude Code 和 OpenCode 返回原生 session ID 后，Runner 会在同一群、同一 Agent 的下一次任务中使用各自 CLI 的 resume 参数。聊天交付中的执行凭证会显示 Agent 类型、真实可执行文件、版本、PID、时间和原生 session；因此可以区分“真实调用本地 Agent”与“只调用同名模型”。

Runner 现在提供三层运行时目录，扫描和成员面板会明确显示来源。支持项不再编译成 Client 内的类型白名单；默认适配目录位于 `internal/agentcatalog/default.json`，Hub 通过 `/downloads/agent-catalog` 发布版本化目录，Client 启动时及运行期间每 5 分钟自动校验、缓存并重新发现本机 CLI：

- `原生直连`：现有 Hermes、Codex、WorkBuddy、Claude Code、OpenCode、OpenClaw CLI 适配，使用各工具的参数、登录态和原生历史机制；实际可用性见验证矩阵；
- `ACP 预设`：本机已经安装 `codex-acp`、`claude-agent-acp` 等已知适配器时自动发现，不在线下载依赖；
- `自定义 ACP`：在 Runner 配置中把任意兼容 Agent Client Protocol v1 的 stdio Agent 声明为 `transport: acp`。

ACP 不是同名模型转发层。Runner 会启动目标 Agent 的真实本机进程，完成 `initialize → session/new|resume|load → session/prompt`，接收流式 `session/update`，处理工具权限请求，并在用户停止任务时发送 `session/cancel`。测试阶段 `acp_permission_policy: auto_allow` 会优先选择 Agent 提供的“始终允许/本次允许”；以后可改成 `allow_once` 或 `deny`。完整配置见 `config/runner.example.yaml`。

群聊支持把报告附件与 `@Agent` 点名一并发送。附件由 Hub 做群成员权限控制，目标 Runner 下载到独立任务目录并校验 SHA-256 后，将真实本地路径交给原生 Agent，因此远端用户可以提交报告，让另一台电脑上的 Agent 调用其本机 Skill 完成审查。

每个群聊同时具备“总览、聊天、协作台、知识库、产物”五个轻量视图。知识库用于长期共同参考的规范、历史材料和样例，与聊天里的临时附件、Agent 最终产物分开。每个工作区和群聊在 Server 权威存储根目录下都有真实目录：

```text
workspaces/<workspace-id>/knowledge/
workspaces/<workspace-id>/rooms/<room-id>/knowledge/
workspaces/<workspace-id>/rooms/<room-id>/uploads/
workspaces/<workspace-id>/rooms/<room-id>/deliverables/
```

目录使用稳定 ID，不依赖可修改的工作区或群聊名称。上传到“工作区共享”的资料可供同一工作区各群引用；上传到“本群资料”的文件只服务当前群。目标 Runner 领取任务时会下载这两层知识资产到任务的 `knowledge/` 目录，校验大小和 SHA-256，再把本地真实路径交给被点名的原生 Agent。旧版 `inbox/` 中的会话附件继续可读；升级后新会话附件写入对应群聊的 `uploads/`。

Runner 在向 Hub 报告成功或失败前，会先把 `task_id + attempt + 回执` 写入本机 `completion-outbox`。若网络恰好在 Agent 做完后中断，Client 重连时先补交回执，再恢复未完成任务；Hub 对同一任务的完成请求幂等返回原消息。因此“结果已经生成但 HTTP 回包丢失”不会再把同一任务重新执行一遍，也不会在群里生成两条交付。

Client 每次启动会扫描一次本机目录，并在后续心跳中把完整目录同步给 Hub。新增一个使用现有输出格式的 CLI，只需更新 Hub 的 Agent 适配目录；各 Client 会自动下载有效目录、保留上一份可用缓存并重新发现，不需要重装、删除设备、重新配对或重建工作区。只有新 CLI 采用 Client 尚未实现的通信/输出协议时才需要升级 Client；优先兼容 ACP 的 Agent 可直接复用通用 ACP 执行器。

## Server、Client 与加入流程

AgentCollab 明确拆成两个安装程序：团队指定主机运行 `AgentCollab Server`；需要接入本机 Agent 的成员安装 `AgentCollab Client`，同一台主机可同时运行两者。只参与讨论和审查的真人可以直接使用浏览器。Server 是账号、群聊、任务、审计、安装包与数据的唯一权威；Client 只承载聊天界面和当前电脑的本机 Agent 服务，不会创建或启动 Hub。

日常只保留三条清楚的路径：

1. 管理员首次打开 Server，设置协作中心名称、自动或固定地址、数据目录和默认 `owner` 管理员；以后管理页只需确认运行正常并点击“进入团队”。
2. 群主点击“邀请真人”，把一个群聊邀请链接发给对方。邀请页可直接下载或打开 Client，链接自动带入团队地址和群聊邀请码；成员只需登录或注册。
3. 每位用户在自己的 Client 中点击“添加我的 Agent”，Client 自动连接当前电脑并发现本机真实 Agent。用户选择哪些 Agent 进入工作区 Agent 库，再将它们加入需要的群聊。

设备一次性码仍作为旧版和故障恢复能力保留在底层，但不再出现在群详情的常规流程里。团队身份、主地址和候选地址由 Server 的 `/api/v1/hub/info` 统一声明；Client 下载入口为 `/downloads/client`，旧 `/downloads/runner` 地址只作兼容。

## 账号、Agent 与能力权限

- 平台角色与协作角色分开：`Hub 管理员`可管理账号和 Hub 管理员名单，不会自动获得任一工作区或群聊的访问权；首个 Hub 管理员由本地 `hub admin create` 创建，之后由已有 Hub 管理员在“个人资料 → 管理 Hub 账号权限”中提升已注册账号。工作区与群聊的权限仍完全由下列协作角色决定。
- 真人群聊只设三种角色：`群主`、`管理员`、`成员`。创建群聊的人自动成为群主；群主可设置群管理员；群主和管理员可邀请、添加和移出当前群成员。群内移出不会删除对方的工作区账号、设备或 Agent 库，历史消息继续保留。
- 当前群成员只在“真人成员”中显示一次；未入群账号统一放在“其他工作区账号”中。工作区 `Owner` 只有在这里才能改变工作区身份或彻底删除账号、设备和 Agent。若需删除一个当前群成员，先将其移出当前群，再到该区域执行工作区级删除。
- 群主或群管理员可治理群成员及移出 Agent，但不能改写别人 Agent 的响应方式、可见范围、调用范围或共享 Skill；这些设置始终由 Agent 主人控制。
- 设备和 Agent 永远归生成其配对码的登录账号所有。Agent 主人通过右侧“我的 Agent 设置”入口独立设置“仅自己可见 / 工作区可见”“仅自己调用 / 所在群成员调用”，并勾选允许其他成员使用的 Skills；每张群内 Agent 卡片会直接显示当前权限摘要。
- `仅被 @ / 主协调 / 回复所有真人消息 / 静默` 是按群生效的响应方式，只能由该 Agent 主人在“Agent 成员”中修改。响应方式不会绕过调用授权：未开放给群成员的 Agent 不会自动回复其他真人。
- 非主人看到的是过滤后的能力清单：只显示主人共享的 Skill，不显示本地资源；未授权 Agent 在 @ 列表和结构化任务中不可选，Hub 也会拒绝绕过界面的调用请求。
- 旧版 Client 不具备能力过滤协议，Hub 会拒绝对它发起跨所有者任务；目标电脑更新到 `0.2.0` 或更高版本后才能开放给群成员。
- 默认扫描名称使用 `用户名 + Agent 品牌`，例如 `alice Codex`。新版 Client 在操作系统用户的固定 `~/.agentcollab/installation-id` 保存设备身份，不再跟 Runner 配置目录或安装包位置绑定；改设备名、换配置路径或重复配对都会复用原 Device/Runner。Agent 另外以本地配置 ID 保持身份，修改显示名不会重复注册。扫描列表只显示最近在线且真实可执行的候选，离线旧设备不会再造成虚增。
- 群聊优先使用 WebSocket 实时更新；网络切换或 Hub 重启时会自动退回“消息自动同步”（定时拉取），继续补齐消息并在连接恢复后回到“消息实时同步”，不会把可用的轮询通道长期显示成故障重连。

当前能力授权在 Hub 调度和 Runner 上下文层强制执行；原生 CLI 子进程仍使用设备登录用户的系统权限。若要对不可信成员开放，还需增加本机逐任务批准和操作系统级目录沙箱。

局域网仍建议为 Server 做 DHCP 地址保留或使用公司 DNS 名称，例如 `http://agentcollab.office:8080`。同时，Server 会发布稳定团队身份、多个当前入口并提供局域网发现；Client 会校验团队身份，在 IP 或自动端口变化后寻找并切换到同一 Server，随后同步恢复本机 Agent 服务。Client 的团队会话采用独立直连网络，不继承可能拦截内网地址的系统 HTTP 代理；外部网页始终交给系统浏览器。离开办公网时只会暂时离线，回到团队所在网络后自动恢复，不会重新配对。公网阶段应把 Server 放在 HTTPS 反向代理或可信 VPN 后，不应直接向互联网暴露裸 HTTP 端口。目标 Agent 执行时只使用其所在电脑的本机权限、Skills 和资源路径，Server 只收到能力别名与最终交付/执行凭证。

## 数据边界

本机执行不等于本机信息绝不离开电脑。执行凭证可能包含 CLI 路径、设备名、原生
会话 ID；Agent 回复也可能引用文件路径。分享群聊、导出记录或提交公开 Issue 前，
请检查并遮盖私人信息。能力目录中的路径处理不代表所有交付正文都已脱敏。

Hub 权威持久化目录由管理员配置，例如：

```text
/srv/agentcollab
```

Runner 不直接访问 SMB。设备本地只保留 Hub/Runner 可执行文件、Agent 依赖、设备凭据、进程工作目录、临时任务目录和可重建缓存。长期运行的 Hub 从本地目录启动，通过绝对路径访问 SMB 权威存储。

首次体验推荐使用本地磁盘。迁移时先停止 Server，完整备份数据再切换存储位置；不要让两个 Server 同时写入同一份数据库。

## 开发命令

```bash
make test
make vet
make build
make dist
```

桌面应用分开构建：

```bash
# 每位用户安装的聊天与本机 Agent 客户端
make desktop-runtime desktop-install desktop-mac

# 固定主机安装的 macOS Server
make server-runtime server-install server-mac
```

构建产物写入被 Git 忽略的 `bin/` 和 `dist/`。

`make runner-dist` 和 `make runner-packages` 继续生成无界面 Runner，供服务器环境、兼容部署和故障恢复使用。它们只发布到 `/downloads/runner`，不会再占用普通成员使用的 Client 入口。`/downloads/client` 与 `/downloads/client/manifest` 只提供封装后的图形桌面 Client；覆盖安装会保留原团队地址、设备身份和配对凭据。

Server/Client 的正式边界与迁移步骤见 `docs/ops/desktop-app.md`，Hub 本机部署与启停方法见 `docs/ops/local-hub.md`，临时离线模式见 `docs/ops/temporary-local-mode.md`。
