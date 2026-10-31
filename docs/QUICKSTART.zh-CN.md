# 本地源码快速开始

本流程供开发者体验社区预览版。桌面安装包会在正式发布前另列实际验证的平台。

准备 Git、`go.mod` 指定版本的 Go、Node.js 22.12+、npm、Make、rsync 和 SQLite 所需的 C 编译器。以下命令面向 macOS/Linux shell，不适用于 Windows PowerShell。本流程是全新部署，不要覆盖已有的 `config/hub.yaml`，不要把真实团队数据用于验收测试。在仓库根目录执行：

```sh
npm ci --prefix web
make web-build
make build
mkdir -p config/agentcollab-data
cp config/hub.example.yaml config/hub.yaml
```

编辑 `config/hub.yaml`：将 `listen_address` 设置为 `127.0.0.1:8080`，`public_url` 设置为 `http://127.0.0.1:8080`，`runner_public_urls` 也使用该地址。示例中的 `storage_root: ./agentcollab-data` 相对于配置文件解析，对应 `config/agentcollab-data`。本地测试保持 `require_smb: false`。

先创建管理员，再启动服务。下列第一条命令等待你输入密码，输入完成按回车；密码不会显示。不要把密码写入版本库。

```sh
read -s AGENTCOLLAB_ADMIN_PASSWORD
export AGENTCOLLAB_ADMIN_PASSWORD
bin/agentcollab-hub admin create --config config/hub.yaml --username owner --display-name Owner
unset AGENTCOLLAB_ADMIN_PASSWORD
bin/agentcollab-hub serve --config config/hub.yaml
```

浏览器打开 `http://127.0.0.1:8080`，登录后创建工作区和群聊，通过群聊邀请入口让第二位真人加入。

局域网共享时，监听地址改为 `0.0.0.0:8080`，公开地址改为服务器可访问的局域网地址。其他电脑不能用 `127.0.0.1` 访问你的服务器。

需要提供本机 Agent 的成员使用兼容 Client，执行本机连接与发现流程。浏览器不能扫描其他电脑；各 CLI 应先能在本机独立运行并完成登录。

## macOS 从源码启动 Client

保留第一个终端中的 Hub。在第二个终端进入同一仓库根目录：

```sh
make desktop-runtime desktop-install
mkdir -p build/client-preview
AGENTCOLLAB_CLIENT_DATA_DIR="$PWD/build/client-preview" npm --prefix desktop start
```

这会用独立本地配置启动真实 Client，不会安装应用，也不会添加登录启动项。
填写上述同一个 Server 地址，或粘贴它的邀请链接；登录已有团队账号，点击
“连接本机”和“查找 Agent”。选择已安装并独立登录的工具加入工作区 Agent 库，
再加入群聊，点名做简短测试。后续仍用同一配置目录启动，才能保留设备身份。
结束时明确退出 Client；仅关闭窗口可能仍在托盘运行。

如需构建 `.app`，执行 `make desktop-pack`。Apple Silicon 的输出是
`desktop/release/mac-arm64/AgentCollab.app`，Intel 的输出是
`desktop/release/mac/AgentCollab.app`。能构建不代表已公证或完成平台验收。
Intel、Windows 仍需各自验证，不能把已有 ZIP 当作新版下载。Linux 可运行
源码 Hub，但目前没有经过验收的桌面 Client 安装包。

接下来按[逐步操作指南](USER_GUIDE.zh-CN.md)完成邀请、Agent 接入与首次回复验证。Server 所在电脑若要提供本机 Agent，也需要 Client。

## 从零自检

```sh
node scripts/smoke-community.mjs bin/agentcollab-hub
```

脚本使用临时数据库和空闲本机端口，启动真实 Server，检查账号和邀请流程，结束后关闭进程并清理自己的临时目录。它不代表桌面界面或真实模型调用已经验证。
