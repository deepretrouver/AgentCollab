# Hub 本机部署

## 运行边界

- 源码、SQLite、日志、附件和备份位于 SMB；
- Hub 可执行文件位于本机，不从 SMB 直接常驻运行；
- Hub 运行时持有 `data/hub.lock`，同一份数据只允许一个 Hub 写入；
- Runner 不访问 SMB，后续只通过 Hub API/WebSocket 连接。

## 构建本机可执行文件

在仓库根目录执行：

```bash
mkdir -p "$HOME/.agentcollab/bin"
go build -o "$HOME/.agentcollab/bin/agentcollab-hub" ./cmd/hub
```

## 启动前检查

```bash
"$HOME/.agentcollab/bin/agentcollab-hub" doctor \
  --config "/srv/agentcollab/config/hub.yaml"
```

预期末行为 `doctor=ok`。SMB 未挂载、数据库不完整或已有 Hub 持锁时必须停止启动。

## 初始化管理员

Hub 停止时执行一次。密码通过环境变量传入，不写入命令行和配置文件：

```bash
read -s AGENTCOLLAB_ADMIN_PASSWORD
export AGENTCOLLAB_ADMIN_PASSWORD
"$HOME/.agentcollab/bin/agentcollab-hub" admin create \
  --config "/srv/agentcollab/config/hub.yaml" \
  --username owner \
  --display-name "Owner"
unset AGENTCOLLAB_ADMIN_PASSWORD
```

## 启动与验证

```bash
"$HOME/.agentcollab/bin/agentcollab-hub" serve \
  --config "/srv/agentcollab/config/hub.yaml"
```

默认只监听 `127.0.0.1:8080`。另一个终端检查：

```bash
curl --fail http://127.0.0.1:8080/healthz
```

正常停止使用 `Ctrl-C` 或发送 `SIGTERM`。Hub 会先停止 HTTP/WebSocket，再关闭 SQLite 并释放 SMB 单实例锁。

## 备份

Hub 停止后执行：

```bash
"$HOME/.agentcollab/bin/agentcollab-hub" backup \
  --config "/srv/agentcollab/config/hub.yaml"
```

命令返回已通过 SQLite 完整性检查的备份路径。
