# 开发环境基线

- 记录日期：2026-07-16
- 主机：macOS 26.5.2，Build 25F84，arm64
- Go：go1.26.5 darwin/arm64，通过 Homebrew 安装
- Node.js：v22.22.3
- npm：10.9.8
- SQLite CLI：3.51.0
- Apple clang：17.0.0，用于 Hub 的 SQLite CGO 驱动
- Shell：zsh

源码、文档和权威持久化目录位于 SMB 的 AgentCollab 根目录。Go 模块缓存、Homebrew 工具链和其他可重建依赖保留在本机。

部署后的 Hub 二进制会嵌入 Web UI，因此运行 Hub 不要求目标机器安装 Node.js。开发 Web UI 时才需要 Node.js 和 npm。

长期运行的 Hub 和 Runner 二进制必须复制到本机运行目录，并以本地目录作为当前工作目录；它们通过绝对 `storage_root` 访问 SMB。源码与权威业务状态仍只保留在 SMB。

## SMB 开发注意事项

在该 SMB 仓库中不要并行运行多个 `go test`、`go build` 或 `go mod` 命令。实测并发 Go 命令会让 SMB 对 `go.mod` 的读锁返回 `resource temporarily unavailable`。这些命令必须串行运行；Go 模块缓存仍使用本机默认目录。

Hub SQLite 使用 `github.com/mattn/go-sqlite3`。纯 Go `modernc.org/sqlite` 在本机 APFS 测试可用，但在当前 Darwin SMB 挂载上会触发其未实现的 `Xsrandomdev` 路径，因此已排除。Runner 不导入 Hub 数据库包，不受 CGO 约束。
