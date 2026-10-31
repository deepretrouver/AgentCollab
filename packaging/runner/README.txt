AgentCollab Runner — 无界面兼容工具 / Headless compatibility tool

普通成员请下载图形版 AgentCollab Client，不要使用此包代替桌面应用。
For normal team use, download the graphical AgentCollab Client instead.

Windows: 解压后运行 Start-AgentCollab.cmd，保持终端打开。
Windows: extract the ZIP, run Start-AgentCollab.cmd and keep the terminal open.
macOS: 在终端运行 sh install-macos.sh / run sh install-macos.sh in Terminal.
Linux: 运行 sh install-linux.sh / run sh install-linux.sh.

首次启动打开浏览器配对页；后续启动复用已保存的配对。
First launch opens browser pairing. Later launches reuse saved registration.
请仅使用自己账号在可信 Server 生成的设备配对码。
Use only your own account's pairing code from your trusted Server.

不创建登录自启动项或后台服务。运行期间自动重连；Ctrl+C 停止，下次手动启动。
No login items or services are installed. Reconnect works while running.
Stop with Ctrl+C; start manually next time.

旧版本已经创建的启动项不会由本包自动删除，请在系统中另行检查。
Startup entries from older versions are not removed by this package.
不要与管理同一设备的 Client 同时运行，以免重复接入。
Do not run alongside a Client managing the same device.

任务使用当前系统用户权限。不要公开配对码或设备凭据。
Tasks run with your OS user permissions. Never publish device credentials.
See LICENSE and GO-NOTICES.md.
