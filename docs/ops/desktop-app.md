# AgentCollab Server 与 Client

AgentCollab 现在明确分成两个安装程序。即使 Server 和使用者位于同一台 Mac，也按两个程序运行，避免把服务器管理、真人聊天和本机 Agent 接入混在一个窗口里。

## 两个程序分别做什么

### AgentCollab Server

只安装在一台长期在线的固定电脑上，负责：

- Hub 服务、账号、工作区、群聊、任务和审计；
- 数据库、附件与安装包存储；
- 团队唯一访问地址；
- `owner` 初始 Hub 管理员；
- 真人邀请、Client 下载和版本信息。

Server 自带本机管理窗口，但它不是聊天客户端，也不会扫描这台电脑的 Codex、Hermes 等 Agent。关闭管理窗口后 Server 留在菜单栏后台运行；从菜单栏明确退出才会停止服务。

### AgentCollab Client

每位真人都安装，包括运行 Server 那台电脑的使用者。Client 负责：

- 打开团队聊天界面；
- 保存团队身份及多个可用入口并登录真人账号；
- 后台运行本机 Agent 服务；
- 扫描并调用这台电脑上的 Codex、Hermes、WorkBuddy、Claude Code、OpenCode、OpenClaw 或 ACP Agent；
- 保存设备身份；应用运行期间网络恢复后自动重连。默认不加入系统开机登录项。
- 把真实任务队列、CLI 进程和原生 Agent 会话汇总为就绪、待执行、执行中、等待上线和不可用五种运行状态。
- Client 异常退出后重新排队未完成任务；支持 resume 的 CLI 使用已保存的原生 session ID 继续群聊上下文。

Client 不包含 Hub，不创建工作区数据库，也不要求用户保持终端或脚本窗口开启。

Server 和 Client 都由用户手动打开，安装或更新不会加入系统开机登录项。关闭窗口时可留在菜单栏后台继续运行；从菜单栏选择“退出”才会停止对应服务。

## 首次部署

1. 在固定主机安装并打开 `AgentCollab Server.app`。
2. 首次设置填写协作中心名称、自动或固定地址、端口和数据目录，并为固定用户名 `owner` 设置密码。
3. Server 启动后点击“进入团队”。同机 Client 会自动识别 Server；owner 登录后创建工作区和群聊。
4. 在目标群聊点击“邀请真人”，把唯一链接发给成员。对方打开后可下载 Client，或直接点“在 Client 中打开”；团队地址、群聊邀请码都由链接带入。
5. 成员登录或注册后自动进入群聊，不再分别填写 Hub 地址、邀请码和设备配对码。
6. 需要贡献本机 Agent 的成员点击“添加我的 Agent”，选择本机 Agent；Client 在后台完成设备连接、扫描和注册。

日常操作不再要求用户理解 Hub 地址、Runner 地址、邀请码和配对码四套概念：

- 真人加入：一个群聊邀请链接；
- Client 加入：打开真人邀请链接；
- Agent 加入：Client 内“添加我的 Agent”。

一次性设备码仍保留在底层，供旧版、纯浏览器和故障恢复使用，不再放在群详情的日常操作区。

## 更新与网络切换

- 真人账号、工作区和消息保存在 Server，更新 Client 不会重新邀请真人。
- Client 的设备令牌和 Agent 配置保存在使用者电脑，覆盖安装不会重新配对。
- Server 为每个团队生成稳定身份，并同时发布当前主地址和局域网候选地址。Client 只会在身份相同的地址之间切换，避免误连同网段的其他团队。
- Client 会保存候选地址、尝试原主机的备用端口，并通过局域网发现恢复 IP 或端口已经变化的 Server；恢复后同步更新本机 Agent 服务地址。
- Client 的团队会话使用独立直连网络，不继承系统 HTTP 代理。这样即使公司代理、VPN 或代理软件没有正确排除当前局域网网段，团队页面也不会因请求被送进代理而白屏；外部网页仍交给系统浏览器和系统网络设置。
- Server 每 5 秒维护健康状态。地址连续稳定三次后才切换，避免 Wi-Fi/VPN 抖动造成反复重启；自动地址模式遇到端口占用时会选择后续可用端口。
- 离开办公网时 Client 显示团队暂时离线并继续后台寻找；回到该团队所在局域网后自动恢复，无需重新配对。
- 重启电脑后需要手动打开 Server（仅主机）和 Client（每位使用者）；应用启动后不需要重新配对。
- Server 的 `/api/v1/hub/info` 提供稳定团队身份、当前主地址、候选地址、登录入口、客户端下载和管理入口；UDP `41729` 仅用于同一局域网内发现公开团队入口，不传递账号或凭证。
- Server 的 `/downloads/client` 是图形 Client 下载中心，`/downloads/client/manifest` 提供正式安装包版本与校验值；无界面兼容包仅从 `/downloads/runner` 发布，不能再冒充普通 Client。

## 从当前本机一体版迁移

1. 先备份当前 Hub 的权威存储目录和配置。
2. 安装 AgentCollab Server。首次设置会检测旧 `mode: host` 配置并预填原权威存储目录和局域网地址；核对后再确认，不要新建第二套生产数据库。
3. 停止旧的 Hub 自启动项，确认只有 Server 管理一个 Hub 进程。
4. 安装新版 AgentCollab Client。它会迁移原 Hub 地址和本机 Runner 身份，但不会再启动 Hub。
5. 以 `owner` 登录，验证工作区、群聊、成员和 Agent ID 均保持不变。

若当前仍使用临时本地存储，先按 `temporary-local-mode.md` 完成权威数据迁移，再让 Server 指向迁移后的目录。

## 开发与构建

构建 Client：

```bash
make desktop-runtime
make desktop-install
make desktop-dev
make desktop-mac
```

构建 macOS Server：

```bash
make server-runtime
make server-install
make server-dev
make server-mac
```

Windows Client 仍在 Windows 构建机原生构建：

```powershell
make desktop-win
```

当前 Server 首先提供 macOS Apple 芯片正式包；Intel Mac 和 Windows Client 由各自构建任务生成后放入同一个下载中心，未发布的平台不会显示假下载按钮。Windows Server 等 macOS 流程验证稳定后再封装。仓库中的 `desktop-release.yml` 分别生成 Client 和 Server 安装包。

若构建机已有经过校验的 Electron ZIP，可用 `ELECTRON_DIST=/absolute/path/electron.zip make server-pack` 复用本地包，避免受 GitHub 下载速度影响。
