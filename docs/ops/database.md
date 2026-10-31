# Hub 数据库运行说明

## 权威位置

```text
/srv/agentcollab/data/agentcollab.db
```

数据库使用 SQLite `journal_mode=DELETE`、`synchronous=FULL`、外键约束和 5 秒 busy timeout。Hub 内所有写事务经过一个写入队列；Runner、浏览器和其他电脑不得直接打开数据库文件。

## 运行边界

Hub 二进制及当前工作目录必须位于本机，例如 `~/.agentcollab/bin/`。Hub 通过配置中的绝对 `storage_root` 访问 SMB。不要以 SMB 目录作为长期运行进程的当前工作目录。

Hub 使用 CGO SQLite 驱动 `github.com/mattn/go-sqlite3`。Runner 不导入数据库包，Windows 和 Linux Runner 仍可使用 `CGO_ENABLED=0` 构建。

## 健康检查

```bash
~/.agentcollab/bin/agentcollab-hub doctor \
  --config /srv/agentcollab/config/hub.yaml
```

成功输出必须包含：

```text
lock=available
sqlite=ok schema_version=2
doctor=ok
```

## 一致性备份

```bash
~/.agentcollab/bin/agentcollab-hub backup \
  --config /srv/agentcollab/config/hub.yaml
```

备份先写入隐藏的 `.partial` 文件，通过 `PRAGMA integrity_check` 后才在 `backups/` 原子发布。命令的标准输出只有最终备份绝对路径，便于脚本接收。

不要在 Hub 正在持有 `data/hub.lock` 时离线打开或备份数据库。首版锁采用 SMB 上验证过的原子锁目录，不依赖该共享盘表现不一致的 advisory `flock`。
