# 临时本地模式

## 用途与边界

本模式只用于 SMB 暂时不可达时继续开发和本机验收。

- 临时源码：`/Users/example/AgentCollab-local`；
- 临时业务数据：`/Users/example/.agentcollab/local-storage`；
- 本机可执行文件：`/Users/example/.agentcollab/bin`；
- Hub 只监听 `127.0.0.1:8080`；
- 临时配置 `config/hub.yaml` 被 Git 忽略，不会覆盖 SMB 配置；
- 不允许本地 Hub 和 SMB Hub 同时对外服务。

## 本地启动

当前机器已安装为 macOS 登录服务：

```bash
launchctl print gui/$(id -u)/com.agentcollab.hub
launchctl print gui/$(id -u)/com.agentcollab.runner
```

手工调试时先停止对应的登录服务，避免两个进程争用同一数据库或设备凭据：

```bash
cd /Users/example/AgentCollab-local
make web-build
make build
./bin/agentcollab-hub doctor --config config/hub.yaml
./bin/agentcollab-hub serve --config config/hub.yaml
```

## SMB 恢复后的迁移顺序

1. 停止本地 Runner 和 Hub，确认 8080 端口与两个本机进程均已释放。
2. 恢复 SMB 挂载，只读检查 SMB 仓库工作树和 SQLite 完整性。
3. 分别备份本地与 SMB 数据库。
4. 将本地 Git 变更以补丁形式应用到 SMB 原分支，保留 SMB 原有提交历史。
5. 只有在 SMB 数据库仍无用户、工作区、群和消息时，才以本地 SQLite 替换它；否则先做数据级合并。
6. 在 SMB 上运行 `doctor`、全量测试和一次登录/发消息验收。
7. 确认 SMB Hub 正常后，将本地副本改为只读归档或删除。

当前需要按顺序应用的源码补丁位于 `~/.agentcollab/migration-20260716/`：

1. `0001-feat-add-browser-workspace-and-group-chat-UI.patch`
2. `0002-docs-record-temporary-local-recovery-mode.patch`
3. `0003-feat-connect-local-Hermes-Codex-and-WorkBuddy-agents.patch`

本机 `~/.agentcollab/runner.yaml`、设备令牌与应用运行状态不迁移到 SMB；它们属于当前电脑的 Client/Runner 状态。

## 冲突防线

- 不直接复制整个本地目录覆盖 SMB 仓库；
- 不复制 `config/hub.yaml`、`data/hub.lock`、日志或临时文件；
- 数据库迁移前后都运行 `PRAGMA integrity_check`；
- 源码迁移与业务数据迁移是两个独立步骤。
