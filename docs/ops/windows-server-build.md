# Windows Server build / Windows Server 构建

Status: build pipeline prepared; Windows runtime and installer acceptance are pending.
状态：构建流程已准备，Windows 实际运行及安装验收尚未完成。

The Server is separate from the Client. Only the designated team host needs it.
Server 与 Client 独立，只有指定团队主机需要安装 Server。

## Prerequisites / 前提

Use Windows x64 with the Go version in `go.mod`, Node 22, and MinGW-w64 `gcc`
available on PATH. The Hub uses CGO SQLite: a CGO-disabled build is not a usable
Server. GitHub's `windows-2022` image currently lists gcc in its
[official software inventory](https://github.com/actions/runner-images/blob/main/images/windows/Windows2022-Readme.md).

需要 Windows x64、`go.mod` 指定的 Go、Node 22，以及 PATH 中可用的 MinGW-w64
gcc。Hub 使用 CGO SQLite，不能用 Runner 的无 CGO 编译方式代替。

## Build / 构建

From the repository root in PowerShell / 在仓库根目录的 PowerShell 中：

```powershell
npm ci --prefix web
npm ci --prefix server-desktop
./scripts/build-server-windows.ps1
```

This builds the Web UI and Hub, then runs isolated database/invitation/file/restart
acceptance. It does not create a service or a login startup item. A failed command
stops the build; compiler-related environment settings are restored afterwards.

上述命令构建界面和 Hub，再进行隔离数据库、邀请、文件及重启验收；不会安装系统
服务或开机登录项。任何失败都应停止，结束时恢复编译器相关环境设置。

Before packaging, populate `server-desktop/client-packages` with the release's
Mac ARM, Mac Intel and Windows Client ZIPs. The release workflow downloads and
checks all three. Create `server-desktop/runner-packages` even if no optional
headless compatibility archives are shipped. Then run:

打包前需将当前版本三类 Client ZIP 放入上述目录；发布流水线会下载并检查。
兼容 Runner 包可不附带，但其目录需存在。随后运行：

```powershell
./server-desktop/node_modules/electron/dist/electron.exe scripts/smoke-server-desktop.cjs
npm --prefix server-desktop run dist:win -- --x64 --publish never
```

Output: `server-desktop/release/AgentCollab-Server-*.exe`.
Build success is not installer acceptance: test installation, initial setup,
team access from another computer, exit/restart and uninstall on Windows before
advertising verified Windows support. No Windows code-signing identity is configured.

编译成功不等于安装验收通过。正式宣布 Windows 支持前，还需验证安装、首次设置、
另一台电脑接入、退出重启及卸载。目前未配置 Windows 代码签名身份。
