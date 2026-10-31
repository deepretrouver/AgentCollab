# AgentCollab 大版本升级规则

大版本升级必须让管理员从首次启动开始完整验收，不能把旧数据、旧登录状态或旧 Runner 身份静默带入新版本。

## 版本隔离

- Server bundle ID 使用 `com.agentcollab.server.v<major>`。
- Client bundle ID 使用 `com.agentcollab.desktop.v<major>`。
- Server 数据位于操作系统的 `AgentCollab Server v<major>` 用户数据目录。
- Client 会话位于操作系统的 `AgentCollab v<major>` 用户数据目录。
- Runner 配置位于 `~/.agentcollab/v<major>/runner.yaml`，设备凭证也保存在同一主版本目录。
- 新大版本不得自动读取以前主版本的配置；旧版导入必须由管理员明确发起，并保留独立存档。

## 本机升级顺序

1. 正常退出旧 Server 与 Client，并停止旧 Runner/Hub 启动项。
2. 将旧应用、Application Support 和对应 `~/.agentcollab/v<old-major>` 整体归档，不直接删除。
3. 安装新 Server 与 Client；同一台主机只运行一个主版本，避免端口和设备心跳互相竞争。
4. 首次只启动 Server，确认没有 Hub 配置和数据库，然后创建新的 owner。
5. 新建工作区与群聊，再启动 Client、登录新账号、连接本机并扫描 Agent。
6. 完成消息、任务、看板、附件和重启恢复验收后，才能把新版本发给其他成员。

小版本和补丁版本可以在同一主版本命名空间内原地更新，但更新前仍应备份 Server 数据库。大版本历史数据以后通过单独的“导入存档”流程处理，不能恢复为自动迁移。
