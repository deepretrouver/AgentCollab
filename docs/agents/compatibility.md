# Native Agent verification / 原生 Agent 验证

An adapter definition is not a compatibility guarantee. Successful discovery, successful
authentication and successful task delivery are three separate checks.

适配目录存在不代表已验证可用。发现 CLI、CLI 登录成功、真实任务完成回群是三个独立检查。

## Current evidence / 当前证据（2026-10-31）

| Agent | Evidence / 证据 | Release interpretation / 发布含义 |
| --- | --- | --- |
| OpenCode | Real confirmation plus project branch review, owner merge and post-merge verification passed; task-linked delivery and restart persistence checked. / 真实确认、项目分支审查、管理员合并及合并后校验通过，已检查任务回群关联和重启持久化。 | Tested on one machine with a plain-text fixture, not every tool/model or cross-machine setup. / 单机纯文本示例通过，不代表全部工具、模型或跨机环境。 |
| Codex 0.132.0 | Logged in, but Server task timed out; standalone CLI reported model-directory schema incompatibility and connection retries. / 已登录，但任务超时，独立 CLI 有模型目录解析错误及重试。 | Not accepted. / 未通过验收。 |
| Codex 0.166.0 | Installed separately without replacing the user's CLI; Server task also timed out. / 隔离安装，不替换用户 CLI；平台任务仍超时。 | Not accepted; investigate local configuration and service access. / 未通过，继续排查本机配置及服务访问。 |
| Hermes 0.22.0+5.g9338452 | Real confirmation plus joint OpenCode/Hermes mentions and a context-aware targeted followup passed; rules disabled and tools restricted to clarify. / 真实确认、与 OpenCode 联合点名及基于前文的定向追问通过；关闭规则注入、限制 clarify 工具集。 | Single-machine, same-owner CLI/API checks; project review, full skills, autonomous delegation and cross-machine use not accepted yet. / 单机同主人 CLI/API 检查；项目审查、完整技能、自主委派及跨机仍未验收。 |
| Pi, WorkBuddy, Claude Code, OpenClaw | Adapter definitions exist; no current end-to-end proof is asserted by this matrix. / 有适配定义，本表不宣称当前端到端通过。 | Unverified for this release. / 本次发布未验证。 |

## What counts as acceptance / 验收标准

For Codex 0.166.0, a separate 45-second direct probe ignored user configuration while
retaining the configured model and reasoning effort. MCP errors disappeared, but stream
disconnection persisted and no confirmation was received. This rules out MCP startup as
the sole explanation; it does not identify the final network/provider cause.

Codex 0.166.0 的独立 45 秒探测在保留模型与推理档位的同时忽略用户配置，MCP 错误
消失，但仍出现流连接断开且未收到确认。这排除了“仅由 MCP 启动造成”的解释，
尚未确定最终网络或服务端原因。该探测未修改用户配置文件。

1. The owner connects their own Client and selects the detected native CLI.
2. A real task reaches that device; no mock/model-only replacement is used.
3. The requested Agent returns the expected result to the originating topic.
4. Task identity, Agent identity and execution receipt agree.
5. Failure, timeout and cancellation remain visible rather than counting as delivery.

对应中文：本人连接自己的 Client 并选择原生 CLI；真实任务到达目标电脑；目标 Agent
将结果返回来源话题；任务、Agent 和执行凭证一致；失败、超时、取消不算成功交付。

These checks do not prove that the vendor's desktop app will display the CLI session.
Credentials and native CLI state stay managed by their owner; do not publish them with bug reports.

以上检查不保证厂商桌面软件会展示 CLI 会话。账号凭据及原生 CLI 配置由设备所有者管理，
提交问题时不要附带凭据或未经脱敏的原始日志。
