# 本机 Agent 接入

当前默认适配目录提供以下原生 Agent CLI 的接入定义。存在适配定义不等于已完成当前版本的真实调用验收；发布时以[实际验证矩阵](compatibility.md)为准。桌面用户先按[操作指南](../USER_GUIDE.zh-CN.md)连接自己的电脑，不需要手动运行下面的底层 CLI。

- `Hermes Local`：发现本机 `hermes`，使用当前配置；自定义 profile 必须在本机明确配置，默认并不要求名为 Chengong；
- `Codex Local`：发现本机 `codex`，保存原生 session，并按当前适配策略跳过审批和沙箱；
- `Pi Local`：`pi --print --mode json`，使用真实 Pi CLI，保存原生 session、工具与 Agent Skills；也兼容安装在 Hermes 私有 Node 运行时中的 Pi；
- `WorkBuddy Local`：从已安装的 WorkBuddy 应用包提取其内置 CodeBuddy CLI，以 `-p --output-format json -y` 执行；
- `Claude Code Local`：`claude -p --output-format json`，保存 Claude Code 原生 session；
- `OpenCode Local`：`opencode run --format json`，保存 OpenCode 原生 session；
- `OpenClaw Local`：`openclaw agent --agent main --json`，通过 OpenClaw Gateway 或其嵌入式回退执行。

这份清单是当前 Hub 发布目录的内容，不是 Go 代码里的固定枚举。Server 公开 `/downloads/agent-catalog`；Client 启动后自动同步，并每 5 分钟检查一次新版本。路径候选、启动参数、resume 参数、版本探测、输出解析模式和 Skill 根目录都由 JSON 适配目录声明。Hub 下发的目录会缓存到 `~/.agentcollab/runner/adapter-catalog.json`；当前电脑如需临时覆盖，可放置同结构的 `adapter-catalog.local.json`，按 Agent `type` 覆盖且不会被 Hub 更新冲掉。

因此，新增一个仍使用 `text`、`json-result`、`codex-jsonl`、`pi-jsonl` 或通用 ACP 的终端，发布目录后即可被在线 Client 自动发现，无需重新安装。若厂商引入全新的输出协议，Client 会拒绝未知 `output_mode` 并继续使用上一份有效目录，避免目录更新把现有 Agent 全部弄坏；此时才需要为通用执行层增加一次协议支持并升级 Client。

测试阶段 Agent 的 `permission_mode` 仍为 `admin`，原生 CLI 使用目标电脑当前登录用户的系统权限执行；Web 界面会显示 ADMIN 标签。平台层已经增加跨所有者授权：只有 Agent 主人能改响应模式、可见范围、调用范围和共享 Skill；群主/群管理员只能治理当前群成员或移出 Agent，不能替主人扩大能力授权。日常“移出当前群”不会撤销对方的工作区账号、设备或 Agent 库；未入群账号统一收在“其他工作区账号”中，只有工作区 Owner 能在那里执行破坏性的账号删除。

桌面 Client 配置按主版本隔离：当前 1.x 使用 `~/.agentcollab/v1/runner.yaml` 和 `~/.agentcollab/v1/runner/device.json`。不要把旧版 `~/.agentcollab/runner.yaml` 自动当成新版桌面配置，也不要直接覆盖设备凭据。独立命令行 Runner 可使用不同路径，以实际 `--config` 为准。稳定设备身份默认在 `~/.agentcollab/installation-id`；凭据与身份文件应保持 `0600`。Hub 只保存令牌哈希。Agent 按 Runner 配置中的 `agents[].id` 保持身份，修改显示名不会新增一条候选。

## 用户接入流程

1. 登录后点击“开始扫描与注册”。
2. 在弹窗中点击“开始扫描本机 Agent”。平台不会自动替用户注册 Agent。
3. 用户勾选扫描到的 Agent，点击“注册并加入”。
4. 在群聊输入普通背景信息，使用 `@Agent 名称` 点名执行，或点击“创建任务”提交结构化任务。
5. 需要专题讨论时，先将一名 Agent 设置为“主协调 / 自动分诊”，再发送 `/brainstorm [议题]`。平台会组织其他非静默 Agent 分析，并由主协调最终汇总。
6. 需要使用目标电脑独有的 Skill 时，在群聊中点名能力，例如：`@Hermes Chengong 请使用你本机的 stock-analysis Skill 后给出结论`。Agent 主人在“可用 Agent → 权限设置”中查看完整能力、设置可见/调用范围并选择共享项；其他成员只看到主人已共享的 Skill，不显示本地资源。`仅被 @ / 主协调 / 回复所有真人消息 / 静默` 则在“Agent 成员”中按群设置，且同样只有 Agent 主人能改。

Runner 配置的 `resources` 是设备级资源目录；`agents[].skill_roots` 可补充某一 Agent 的 Skill 目录。适配目录也会声明各 Agent 的常用 Skill 目录。资源路径在目标 Runner 构造提示词时使用，但不能因此宣称所有路径都不离开本机：执行凭证可能向 Hub 提交可执行文件路径、设备名和原生会话 ID。Agent 主人可在右侧“共享与调用权限”中勾选允许其他成员使用的 Skill；非主人任务只注入这些 Skill，所有本地资源均不注入。原生 CLI 仍以 Runner 系统用户权限执行，所以这属于平台调度边界，不等同于操作系统沙箱；面对不可信成员还需本机逐任务批准与目录级隔离。
7. 被点名 Agent 会收到最近 24 条群聊上下文，结果以该 Agent 身份回到来源群聊。

## 会话附件与跨机审查

群聊输入框支持把最多 5 个附件与 `@Agent` 点名作为同一条消息发送，单文件上限 50 MB。当前允许 PDF、Word、Excel、CSV、TXT、Markdown、PPT 和常见图片；不接收可执行文件或脚本。

例如：选择授信报告后发送 `@Hermes Chengong 请调用投委会审查 Skill 审查附件报告并给出正式审查意见`。浏览器先把文件上传到 Hub；Hub 将附件绑定到发送者、工作区、群聊和消息；只有群成员可从时间线下载。任务被目标 Runner 领取后，Runner 使用设备令牌下载关联附件到 `~/.agentcollab/runner/tasks/<task-id>/attempt-<n>/attachments/`，核对大小和 SHA-256，再把真实本地绝对路径交给原生 Agent。目标 Agent 因而可以调用自己电脑上的 Skill 和解析工具读取报告，结论与执行凭证仍回到原群聊。

验证时应同时查看：原消息的附件卡片、任务进度、目标设备执行凭证，以及目标 Runner 日志中的 `task_attachment=<task-id>`。只显示文件名但没有 Runner 下载校验记录，不算附件执行链路跑通。

## 项目知识库

会话附件适合“这一次消息要处理的文件”；群聊顶部“知识库”适合需要被真人和 Agent 反复参考的项目资产。知识库分为“工作区共享”和“本群资料”。Hub 将两层资产保存在 Server 的真实工作区/群聊目录中，并在本群每次任务被领取时一并下发。Runner 把文件下载到 `~/.agentcollab/runner/tasks/<task-id>/attempt-<n>/knowledge/`，校验大小和 SHA-256 后，把实际本地路径写入原生 Agent 提示；日志会记录 `task_knowledge=<task-id>`。

知识库文件本身是成员明确上传到协作空间的共享资产，不等于 Agent 所有者电脑上的任意本地资源。跨所有者任务仍只可使用主人授权的 Skills；知识库不会绕过 Agent 权限或扩大本机目录读取范围。

## 真人加入与其他电脑接入

先区分两个动作：

- **真人邀请**：决定谁能用自己的账号登录、看到工作区/群聊、阅读 Agent 任务与交付；
- **Runner 配对**：决定某台电脑和其本机 Agent 归哪个真人账号所有。

管理员不应替另一个真人生成 Runner 配对码，否则设备和 Agent 会错误地归到管理员账号。正确顺序是：管理员先在群成员区生成真人邀请；对方创建或登录自己的 Hub 账号并加入群聊；对方再用自己的账号生成设备配对码。

真人端：

1. 打开管理员发来的群聊加入链接，选择“创建新账号”或“已有账号登录”。成功后，该账号会同时加入目标工作区和群聊。
2. 已经拿到分开的 Hub 地址和真人邀请码时，也可以在左侧工作区标题旁点击“加入其他 Hub”手动填写。浏览器会跳到该 Hub；账号属于各 Hub，首次进入新的 Hub 时需要在该 Hub 创建账号。
3. 同一群的真人成员看到同一条对话时间线、群成员、已入群 Agent、任务进度和交付。Agent 卡片中的“某某所有”用于识别 Agent 的真实所有者。

Hub 端：

1. `listen_address` 使用 `0.0.0.0:8080` 才能接受局域网连接；`runner_public_urls` 填目标电脑可访问的地址。
2. 已加入的真人用自己的账号打开群聊右侧“接入我的电脑 / Agent”，点击“我的配对码”。配对码 30 分钟有效且只能成功使用一次。
3. 在自己的目标电脑使用界面中的“统一客户端下载链接”和短期配对码。链接为 `<Hub地址>/downloads/runner`，Hub 会按浏览器系统自动选择包（macOS 使用通用包；Windows/Linux 也可通过 `?platform=...` 显式选择）。配对码不应发到公开群或长期文档。

普通用户使用团队入口提供的桌面 Client 安装包，打开团队链接并登录自己的账号，再点击“连接本机”。此流程不需要用户手工抄写设备配对码，也不需要保持多个终端。当前不添加开机登录项；电脑重启后请手动打开 Client。关闭窗口与从菜单完全退出不同：后台运行时可以继续接任务，完全退出后不能执行任务。

下面的 `pair`、`connect` 和 Runner 压缩包属于高级命令行接入，不是桌面 Client 的正常安装步骤。手动 Runner 安装脚本不会添加登录项，通常需要保持其终端运行；不要与桌面 Client 同时管理同一份设备凭据。

已安装 Client 时，也可先打开浏览器配对页：

```bash
agentcollab-runner pair --hub-url 'http://<Hub局域网IP>:8080' --device-name '研究室电脑'
```

自动化环境仍可使用不经过网页的 `connect`，例如 macOS/Linux：

```bash
printf '%s\n' '<一次性接入码>' | agentcollab-runner connect \
  --hub-url 'http://<Hub局域网IP>:8080' \
  --device-name '研究室电脑' \
  --invite-code-stdin
agentcollab-runner serve --config "$HOME/.agentcollab/runner.yaml"
```

开发机运行 `make runner-packages` 可生成 `dist/runner-packages/` 下带安装脚本的 macOS 通用、Windows、Linux 压缩包。将这些包发布到 Hub 的 `<storage_root>/artifacts/runner-packages/` 后，每位成员都可使用 `/downloads/runner` 和自己生成的配对码；安装脚本会自动启动 `agentcollab-runner pair` 浏览器配对页，配对成功后自动写配置、注册设备、扫描本机 Agent 并启动 Runner。目标电脑上的 Hermes、Codex、WorkBuddy、Claude Code、OpenCode、OpenClaw 仍需由其所有者正常安装并完成各自登录。

`connect` 会在 `~/.agentcollab/runner.yaml` 建立本机配置，并把设备凭据保存到 `~/.agentcollab/runner/device.json`。Runner 只需要主动访问 Hub，不需要给目标电脑开放入站端口。连接后，设备所有者点击“扫描新增”，勾选 Agent 加入当前工作区的 Agent 库，再加入所需群聊。已在同一 Hub 其他工作区注册、且仍在线的本机 Agent 可以直接复用到当前工作区，不需要重新配对或重装 Client；每个工作区仍独立保存 Agent 可见性、调用范围、共享 Skill 与群内响应方式。扫描接口只返回最近在线、可真实执行并归当前账号所有的候选；已离线的历史 Runner 仍保留审计关系，但不会污染扫描列表。

配对码只用于首次接入：Client 把选定的 Hub 地址保存在 `~/.agentcollab/runner.yaml`，把本机设备令牌保存在 `~/.agentcollab/runner/device.json`。电脑重启、休眠唤醒、带离办公室或临时断网后都不需要重新填写地址或配对码；无参数启动 Client 会优先使用这些已保存信息，只有本机从未配对或凭据被删除时才打开配对页。

运行状态由 Client 心跳、真实任务队列和原生会话记录共同推导，不是对“电脑是否开机”的直接探测：Client 运行时会持续轮询 Hub，并每 10 秒发送心跳；Hub 超过 30 秒未收到心跳才判断连接中断。界面统一显示“就绪 / 待执行 / 执行中 / 等待上线 / 不可用”：就绪表示可以接任务，待执行表示已有任务排队，执行中来自该本地 Agent 的真实 `dispatched/running` 任务，等待上线表示任务已保留但 Client 暂未连接，不可用表示 Client 在线但本机没有可执行 CLI。状态会按同一 Runner 和稳定本地 Agent 身份聚合，因此同一 Agent 复用到不同工作区也保持一致。详情提示会给出最后心跳；存在原生 session ID 时还会标明下次任务可续接同一 Agent 会话。

离线或等待上线表示“Client 未启动或当前网络无法访问 Hub”中的一种，而不是笼统判断电脑关机。网络临时中断后，Client 会自行重试，网络恢复后重新在线。Client 重启时，Hub 会把旧进程中断的 `dispatched/running` 任务重新排队；支持 resume 的 Agent 会继续使用该群聊保存的原生 session ID。Windows 和新版 macOS 安装包会注册本机 `agentcollab://reconnect`，设备所有人可在网页点击“重新连接本机”或使用系统开始菜单/应用程序中的 Client 控制入口；它只能控制当前电脑，不能远程开机或唤醒另一台已停止的电脑。局域网 Hub 应使用固定地址：为 Hub 电脑做 DHCP 地址保留，或使用公司 DNS 名称；否则 Hub 自己的局域网 IP 改变时才需要更新一次 `runner_public_urls`，不是每台 Client 重新配对。

离线 Agent 仍可被明确 `@` 或选为结构化任务目标；Hub 会把任务保持为“等待设备上线或领取任务”，目标 Client 恢复后按原顺序领取。任务发起人可以在对应话题中取消尚未完成的委托。自动响应模式仍应谨慎使用，避免长期离线的“回复所有真人消息”Agent积累过多排队任务。

验证跨机执行不能只看 Agent 名称。应在交付的执行凭证中同时核对：目标设备名、原生 Agent/CLI 路径、PID、原生 session ID、本机能力数量，以及命中的 Skill/资源名。这样才能证明任务确实在对方 Runner 上执行。

Hub 是唯一的协作中枢，不等于群主身份：群主/管理员可变更，而 Hub 在当前局域网阶段可由用户 Mac 临时承担；跨网时应迁到一台固定的 HTTPS 服务器并迁移其权威数据。公网部署必须使用 HTTPS 反向代理或可信 VPN，并把 `runner_public_urls` 改成外网 HTTPS 地址。远程成员只有在 Agent 主人开放“所在群成员调用”后才能派发任务，且只获得主人勾选的 Skill；但原生 CLI 最终仍以目标电脑登录用户权限执行，正式面对不可信成员前还应增加逐任务本地批准、操作系统沙箱和更细审计。

Runner 在 `~/.agentcollab/runner/tasks/<task-id>/attempt-<n>/` 中启动原生 CLI。Hub 本身不调用同名模型 API，也不会用其他模型冒充 Agent。

## 如何验证是真实调用

每条 Agent 交付末尾都有“执行凭证”，至少包括：原生 Agent 类型、CLI 绝对路径、版本、进程 PID、开始和结束时间。支持原生会话的 CLI 还会返回 session ID。

- Codex：用执行凭证中的 session ID 在 `~/.codex/sessions/` 搜索对应 JSONL；也可以用 `codex exec resume <session-id>` 继续。Codex 桌面端是否展示该任务取决于它是否索引 CLI session，不能把桌面端列表作为唯一证据。
- OpenCode：进入凭证对应的任务目录运行 `opencode session list`，应能看到同一 session ID 和 `AgentCollab <task-id>` 标题。
- Claude Code：原生 JSON 结果包含 `session_id`，可用 `claude --resume <session-id>` 验证。其桌面端是否同步由 Claude Code 自身决定。
- OpenClaw：调用的是 `openclaw agent`；成功时核对 OpenClaw 会话，失败时群聊仍会返回真实 CLI 的失败结果和执行凭证。
- WorkBuddy：调用的是 WorkBuddy 应用内置的 CodeBuddy CLI。该 CLI 是否把无头任务同步到 WorkBuddy 桌面历史由 WorkBuddy 自身决定。
- Hermes：调用的是当前 `HERMES_PROFILE` 下的 Hermes CLI，而不是单独复刻其模型配置。

因此，“群聊里有独立 Agent 身份 + 可执行文件路径/PID + 原生 session 记录”共同构成真实性证据。不同厂商 GUI 的历史同步属于第二层能力，不保证每种 Agent 都能在桌面软件里看到。

当前原生适配仍存在“默认可执行”的权限边界：Runner 子进程拥有所在系统用户权限。只应接入可信的小团队，不应宣传为适合不可信租户的隔离执行平台；面向不可信成员前需补齐逐任务批准、目录白名单和系统级隔离。
