# 社区发布准备与验收

检查日期：2026-10-30。此文件记录发布门槛，不代表已完成正式发布。

## 发布定位

首个社区版本建议标记为 Preview。AgentCollab 面向可信小团队，让真人与各自电脑上的原生 Agent 在共享项目中协作。Server 保存团队数据，Client 连接个人电脑；真人仅使用浏览器也能参与。

## 已验证

- 前端 19 个测试文件、119 项测试通过。
- Client/Server 桌面配置、网络与生命周期相关 19 项 Node 测试通过。
- 前端生产构建通过；存在大于 500 kB 的 JavaScript bundle 提示。
- 示例配置改为相对本地数据目录，不再要求作者公司的 SMB 共享盘。
- 相对存储路径按配置文件位置解析，配置与启动命令回归通过。
- 独立进程验收通过：全新临时数据库、管理员登录、创建工作区和群聊、第二位成员注册、邀请加入、无需邀请再次登录、普通成员访问管理员接口被拒绝。
- Apache-2.0 已落盘；中英文 README、源码快速开始、贡献指南已建立。
- 308 个候选源码文件的初步敏感路径/常见凭证检查通过；旧 Runner 备份不纳入源码。
- 真实服务进程验证：上传群聊知识文件、同群成员下载内容一致、外部账号被拒绝；停止并重启临时 Server 后，成员重新登录、群聊与文件仍完整存在。
- 登录/邀请入口已有中英文切换；英文注册与切换保留表单数据测试通过。主导航和常用发送操作已接入词典，但全界面翻译仍未完成。

## 真实 CLI 验收发现

2026-10-30：通过真实 Runner 调用本机独立 Codex CLI 0.132.0，任务进入执行并向群聊回传失败。原生 CLI 出现模型元数据兼容错误、插件同步超时和模型请求断流，在 120 秒期限内未产生目标回复。因此本项为失败，不能标记真实交付已通过。

已修复原生包装进程取消时子进程可能残留的问题；进程组取消回归和 Runner 全量测试通过。错误回传正在改成分类摘要，防止把原始 stderr 中的本机路径或配置发给群成员。仍需使用当前兼容 CLI 再验收。

## 发布前必须完成

- 在独立数据目录，从零创建 Server、管理员、工作区、群聊。
- 第二个真人通过邀请链接注册，再次登录不依赖过期邀请。
- Client 首次连接、重启恢复、断网重连、退出行为验证。
- 至少一个真实 CLI 从发现、加入、点名到可见交付完整跑通。
- 验证其他成员无法调用未授权 Agent 或访问未授权文件。
- 文件上传、知识库、步骤目录、任务分支、审查合并完整跑通。
- 对每个对外提供的安装包分别完成实际系统验收；未验证平台不标记支持。
- 检查干净源码安装与依赖锁文件，CI 执行 Go、Web 与桌面测试。
- 检查源码与 Git 历史中的凭证、个人信息、内网路径、运行数据及第三方素材授权。
- Apache-2.0 已加入，桌面包声明已同步；仍需完成第三方依赖与素材授权清单核对。
- 发布页面提供已知限制、真实界面演示、安装步骤和问题反馈模板。

## 发布策略

现有目录含大量未提交开发文件与本地历史。公开前准备经过筛选的独立源码快照，避免直接推送开发目录和历史。禁止包含数据库、日志、配对凭证、真实会话、个人资料和旧安装包。

自动化测试通过不等于桌面安装验收通过。发布记录应分别列出自动化验证、人工端到端验证、平台安装验证和未验证项。

### Runner 首次连接隔离回归

首次连接曾将不同配置实例的设备凭证和运行目录写入同一个用户级目录。
现改为新配置旁的 `runner/` 目录；已有配置重新连接时保留原路径，不自动迁移或覆盖凭证。
新增回归覆盖不同版本配置隔离及已有安装路径保持。修改前测试复现失败，修改后
`go test ./cmd/runner -count=1` 和 `node --test desktop/*.test.cjs`（12 项）通过。
这项证据只覆盖配置逻辑，不代表桌面首次配对及重连实机验收已经完成。

### 独立源码目录构建验证

将 Git 可见的现存源码复制到新临时目录（不复制 node_modules、运行数据与安装包），
执行 `npm ci --prefix web`、`make web-build`、`go test ./...`，全部通过。
使用该目录构建的 Hub 执行 `scripts/smoke-community.mjs`，双用户邀请、独立登录、
成员文件访问、外部用户拒绝访问以及 Server 重启后的群聊/文件持久化均通过。
此验证仍使用本机 Go 工具链及模块缓存，不等于离线构建或 Windows 安装验收。

Server 首次设置页已提供中英文切换，包含字段标签、地址模式说明及创建按钮，
并移除过时的 1.0 文案。DOM 回归验证语言切换保留名称、密码、固定地址和配置提交；
桌面逻辑测试现为 20 项通过。后台返回的具体错误信息及其他桌面页面仍需补齐翻译，
因此不能宣称整个产品双语验收完成。

Client 首次加入和重新连接页面也已支持中英文，并保存语言偏好。回归测试验证
邀请 URL 不因语言切换而改变、连接失败后按钮可重试，桌面测试合计 22 项通过。
接入说明改为“尝试发现”，不再承诺任意网络/端口变化都必定无感恢复。

Server 管理页已继承语言偏好并提供切换，覆盖按钮、健康状态、诊断字段、复制地址
反馈和常规操作提示。回归覆盖英文启动、健康状态更新、复制反馈及切回中文，
23 项桌面测试通过。旧运行时的隔离桌面表单至管理页流程再次通过；真实截图显示
英文管理页。具体后台异常文本仍可能是中文，完整产品双语验收仍未完成。

同轮 npm audit 最初检出 7 项构建/测试依赖漏洞（3 moderate、4 high）。
更新兼容范围内的锁文件，并将 Vitest 最低版本提升至 4.1.12 后，完整 Web 依赖审计
为 0 项漏洞；19 个测试文件、120 项测试及生产构建通过。CI 已加入 moderate 以上
漏洞检查。由于本机 npm 10 的 Arborist 解析异常，锁文件生成使用独立 npm 11.6.2，
随后仍使用 npm 10 的 `npm ci` 成功安装并验证。
此结论仅覆盖 Web 依赖；Electron 虽声明为开发依赖但实际随桌面应用交付，不能用
`npm audit --omit=dev` 的结果替代桌面运行时审计。

### 隔离桌面表单到 Hub 验收

新增 `scripts/smoke-server-desktop.cjs`，用 Electron 启动真实 Server 主进程与
preload，填写首次设置表单，经按钮事件/IPC 启动 Hub，检查管理页报告健康且数据
仍在临时目录，捕获真实窗口截图，最后通过退出 IPC 停止 Server 并清理测试数据。
本机 Electron 37.10.3 执行通过，截图确认显示 Community Demo 与回环地址。
脚本启动模式下界面版本会显示 Electron 版本，截图不作为正式发行版展示素材。
新版 45.1.0 官方二进制仍在下载，因此新版兼容性尚未验证。此测试也不覆盖
Client 首次配对和跨电脑接入。系统拒绝修改登录项时产生警告，但未阻止测试完成。

### Client 原生配对验收与发现的问题

`scripts/smoke-client-desktop.cjs` 使用独立 Hub/账号/邀请，启动真实 Electron Client，
校验邀请参数保留，通过受信任页面 IPC 配对并检查隔离凭证和禁止自启动配置。
旧运行时 37.10.3 已通过；认证 fixture 通过 HTTP 完成，尚非用户手工登录全流程。
失败现场保留临时目录，脚本退出会停止测试 Hub 与 Client Runner。

该测试发现并修复两项入口差异：
- 源码 Server 未设置 Hub ID 时不能被 Client 接受；现在持有 Hub 锁后生成并保存
  `data/hub-id`，重启沿用，显式配置优先；空损坏文件报错而不静默换身份。
- 桌面 connect 命令重复了旧路径逻辑，未使用此前修复的 prepareRunnerConfig；
  现在统一调用同一配置初始化函数。修复前测试实际写入开发者旧共享凭证目录，
  影响引用该路径的旧版注册，需要重新配对，未找到可恢复凭证备份。

命令层测试和增强后的社区 smoke（断言 Hub ID 非空且重启不变）均通过。
这些通过不能证明 Windows 或跨电脑网络连接已完成验收。

复核发现生产退出路径使用 `app.exit(0)`，原验收脚本的 `will-quit` 清理不会执行，
且失败退出码可能被覆盖。已在验收入口拦截最终退出：保留生产停止 Hub 的过程，
未完成断言时强制退出码 1，成功时删除测试目录。设置
`AGENTCOLLAB_SMOKE_FORCE_FAILURE=1` 实际验证失败退出码为 1；正常流程重跑验证
退出码 0 并打印临时数据清理成功。先前关于首次测试自动清理的描述应以此修正为准。

### 桌面依赖安全更新

Server 与 Client 的 Electron 声明同步升级至 `^45.1.0`，并更新两份锁文件的
兼容补丁依赖。两端完整 npm audit 均为 0 项漏洞；桌面 Node 逻辑测试 19 项通过。
CI 分别审计两端完整依赖树（包含 devDependencies）。独立临时目录的 Client
`npm ci` 安装通过。运行时升级后的桌面启动、配对与打包兼容性仍待实机验证，
不能由这些依赖检查代替。

### 原生 OpenCode 链路验收

本机 OpenCode 1.18.30 的 `run --help` 已不提供旧的
`--dangerously-skip-permissions`。默认目录中的显式 admin 适配器改用 `--auto`，
新建与续接会话参数同步修改，目录 revision 更新，并增加回归断言。
这不改变该适配器原有的完整执行权限语义，也不应被当作 OS 沙箱。

`scripts/smoke-community.mjs <hub> <runner> <opencode> opencode` 实测通过：
独立临时 Server/数据库、注册原生 Agent、加入群聊、随机标记任务下发、Runner
调用真实 CLI、群内 Agent 消息包含标记。测试要求不调用工具、不读取文件。
因此已证明一次真实原生 CLI 回复闭环；尚未证明工具操作、桌面扫描交互、
多人并发或其他 CLI 的可用性。临时团队与凭证在结束后自动清理。

### Electron 45 原生桌面回归

2026-10-30：Electron 45.1.0 macOS arm64 ZIP 经镜像获取，执行前 SHA-256
与 Electron 包内官方校验清单一致：
`002b71efcf90e247317d3da486f2d4e51860e9c2889d49f33e229ab2268b0f3b`。
独立临时运行时执行 Server 首次表单、IPC、Hub 启动、健康管理页及退出清理通过；
Client 邀请导航、可信 IPC 配对、隔离凭证与禁止自启动配置检查通过。
23 项桌面逻辑/DOM 回归也通过。

Client harness 仍有 Chromium 缓存目录警告；预创建 userData 未消除，未保留该
无效修补。功能断言通过不等于警告已解决，需在正式打包应用启动路径复核。
Server 环境仍拒绝修改登录项（关闭操作）；未启用自启动。
这些测试尚非正式安装包验收，亦不覆盖 Windows、Intel Mac 或跨机网络。

同日使用经校验的 Electron 45.1.0 和显式 `--config.electronVersion=45.1.0`
生成了 macOS arm64 Client/Server `.app`，两个应用均通过
`codesign --verify --deep --strict`。产物仅位于独立临时目录，未替换用户安装。
必须显式指定版本或使用干净 `npm ci`：本地遗留 node_modules 会让构建器从旧版
Electron 元信息推断版本，即便传入新版 electronDist。
签名是 ad-hoc，不是 Apple Developer ID 签名或公证；尚不能宣称下载后无安全提示。
本次为 `--dir` 构建，不是 DMG 安装、升级或双机验收。Server 的 Client 下载目录
当前为空，最终发行前必须补齐与发行版匹配的客户端包并验证下载入口。

### 工作区侧栏双语流程

2026-10-30：侧栏创建/编辑/删除工作区、创建群聊和选择 Agent、邀请链接加入、
排序置顶、个人资料及 Hub 管理入口接入语言选择。名称排序使用当前语言；用户
输入的工作区、群名、邀请地址和 Agent ID 保持原值，不送入翻译字典。
所有该组件 t() 键均有英文条目。英文新增回归验证群名与 Agent ID 正确提交，
邀请链接完整传递，以及删除必须经过二次确认后调用对应工作区 ID。
侧栏 10 项测试通过，TypeScript 与生产构建通过；新增第二项测试前，完整 Web
19 文件 121 项通过。构建仍有单包大于 500 KB 提示。此轮未做英文布局视觉验收，
右侧管理区和项目计划等完整双语仍未完成。

### Agent 接入双语与选择状态回归

2026-10-30：添加 Agent 弹窗覆盖中英文连接、改绑、离线、网页版限制、原生/ACP
运行时、候选与空结果说明，保留 Agent 名称、版本、设备和可执行路径原值。
浏览器说明纠正为不能直接扫描本机，但仍可调用通过 Client 接入且已授权的 Agent。
工作区有其他设备的注册记录而本机扫描为空时，不再误报“已全部添加”。

新增失败测试先复现关闭重开残留勾选、候选不可用仍提交；修复后关闭清空选择，
提交仅取当前扫描结果中可用且未注册的 ID。补充英文注册身份保持、浏览器说明及
空扫描提示回归。完整 Web 19 文件 127 项通过，类型检查和生产构建通过，
git diff --check 与源码候选扫描通过。尚未替代真实 Client 扫描及跨机 UI 验收。

### 依赖清单与发行包版权文件

2026-10-30：新增 docs/DEPENDENCIES.md，从三个 npm 锁文件按包名/版本合并并保留
Web、Client、Server 使用范围，包含构建依赖。CI 校验清单是否过期、元信息缺失或
许可声明冲突；3 项生成逻辑测试通过。该清单仅为元信息，不替代完整许可文本审查。

实际检查发现 electron-builder 自定义 electronDist 的应用包未保留 Electron 的
LICENSE 与 LICENSES.chromium.html。新增打包 hook，在签名前复制这两个上游文件、
项目 LICENSE 与依赖清单至 resources/notices，源文件缺失或为空则构建失败。
复制和缺失失败测试通过。Client/Server macOS arm64 均重新打包成功，四个文件与
对应源文件逐字节校验一致，两包 codesign --verify --deep --strict 通过。
仍是 ad-hoc 签名，不是 Apple 公证。

Go 完整模块枚举因 proxy.golang.org 网络超时失败，尚未生成完整 Go 清单；
npm 运行时组件的逐包原始许可文本归档也仍需补齐，不能据本轮结果声称许可审查完成。

### 自动扫描异步竞态

2026-10-30：新增回归先复现两个失败：React StrictMode 清理首次 effect 后，
提前设置的 autoScanStarted 阻断后续检查；scanned/scanning 在连接检查期间变化
也会取消检查并阻止再次执行。现在每次有效 effect 都完成连接检查，仅在实际发起
自动扫描时设置“一次扫描”标记。关闭窗口重置标记；扫描失败后不自动无限重试。
三项新增用例覆盖 StrictMode 单次扫描、延迟连接检查与状态变化、失败后的手动重试。
完整 Web 19 文件 130 项通过，TypeScript 与生产构建通过；diff 格式检查通过。
此为组件异步回归，不代表跨机真实扫描已经验收。

### 双语首次协作指南与源码命令复验

2026-10-30：新增 docs/USER_GUIDE.zh-CN.md / USER_GUIDE.en.md，覆盖 Server、
浏览器与 Client 分工，创建工作区/群聊、邀请真人、连接本机、选择 Agent、
授权与首条真实回复验证、项目任务入口及连接状态排障。菜单名称以当前源码为依据；
英文指南明确标注尚未翻译的菜单，并未宣称全平台安装验收完成。
README 和快速开始加入指南入口；补充 Make/rsync、macOS/Linux shell 限定及
不覆盖既有配置/真实数据的说明。19 个相关本地文档链接检查通过。

按文档直接运行旧 bin 自检曾失败（旧 Hub 拒绝相对 storage_root）；执行
`make web-build build` 从当前源码更新嵌入前端及三个二进制后，
`node scripts/smoke-community.mjs bin/agentcollab-hub` 通过真实 Hub 的账号、邀请、
成员下载、外部账号拒绝访问、重启身份及文件/群聊持久化检查。
这证实本次源码构建流程，不证明旧发行包已同步更新或真实多人 UI 流程完成。

### 个人资料双语

2026-10-30：ProfileDialog 的标题、字段、头像校验、保存状态、Hub 权限入口和
不可用提示接入中英文。新增用例在填写中文简介后切到英文，验证未保存内容仍在，
且姓名、简介、当前事项按原文提交。完整 Web 19 文件 131 项通过，类型检查和
生产构建通过，diff 与依赖清单校验通过。MyAgentsSettings 的权限表单本体尚未
完成双语；本次未将其算作完成。发行包尚未重新包含本轮最新 Web 产物。

### Agent 权限表单双语

2026-10-30：MyAgentsSettings 的列表、在线状态、可见范围、调用范围、Skills
分享与撤销、资源说明和错误回退接入中英文。真实 Agent/Skill/资源名称、描述和
来源字符串保持原样，不将用户内容交给翻译字典。新增英文交互测试核对 private、
owner_only 和 report-review 等底层值不变，切换语言不触发任何策略更新。
完整 Web 19 文件 132 项通过，类型检查、生产构建及 diff 检查通过。
此轮只验证前端表单行为，不能替代服务端越权测试或本机 CLI 的权限隔离验证。

### Skill 授权清单变化与后端回归

2026-10-30：失败测试复现当前 Skills 与保存授权数量相等但名称不一致时，
界面误报全选且禁用“全部授权”。改为计算当前扫描结果与授权名称的交集；
不自动增删已有策略，新 Skill 保持未授权，用户点击后才提交当前清单。
另测扫描为空时仍能主动撤销保存的旧授权。完整 Web 133 项通过后新增该边界
用例，个人资料/权限 7 项通过；类型检查和生产构建通过。
当前工作树 `go test ./...` 全部通过（部分包使用测试缓存），`go vet ./...`
通过。这些本机自动检查不代替 Windows/跨机真实安装验证。

### 社区反馈入口准备

2026-10-30：新增中英双语 Bug/Feature Issue 模板和 PR 模板，要求版本、网络类型、
复现与权限影响，明确禁止提交配对码、邀请链接、Cookie、数据库等私人资料。
CONTRIBUTING 补充安全反馈和依赖清单校验说明；docs/agents 记录计划中的 GitHub
跟踪方式和统一术语。未创建远程仓库或标签，未添加定期监控，未创建重复 Agent 指令文件。
两份 Issue front matter 经 YAML 解析校验通过；四项发布辅助脚本测试通过；
源码候选扫描 331 文件无发现，diff 检查通过。私密漏洞反馈渠道仍需建库后确认，
文档没有宣称已启用或承诺响应时限。

### 普通成员真实登录表单与消息验收

2026-10-30：更新隔离 Client harness，不再用 API 在渲染页绕过登录。
fixture API 仅创建临时团队、普通成员和邀请；实际 Electron 45 窗口切换英文、
点击登录标签、填写账号密码、点击提交，等待受邀工作区与 Demo 群聊出现，
打开群聊后等待加载结束，通过真实输入框发送测试消息，并确认消息出现在时间线。
随后仍通过受信任 IPC 配对，核对隔离凭证和禁止自启动设置。完整脚本退出码 0。
过程中修正了测试误用 aria-label 选择器的问题；产品输入框实际以 label/ID 关联。

截图来自真实渲染窗口、临时虚构成员，存放临时验收目录，未发布为正式展示素材。
真实界面仍显示部分中文（右侧、接入提示和会话模式等），不能算完整英文验收。
Chromium 缓存目录警告仍存在；尚未覆盖 GUI 注册表单、GUI 配对按钮、Windows 或
跨电脑网络。当前源码对应的 Hub/Runner runtime 已重建，最终安装包仍需重新封装。

### Client 页面刷新保留验收

2026-10-30：在上述普通成员真实登录与发送链路后，增加发送框清空断言，
完成隔离配对后实际重新加载 Electron 页面。无需再次登录即可打开受邀 Demo
群聊、读取之前发送的消息；英文选择保留，device_id/runner_id 未变化。
Electron 45.1.0 隔离验收退出码 0；完整 Web 19 文件 134 项通过，
git diff --check 通过。本轮没有修改真实账号、配对信息或启动项。

页面重载不是退出进程或电脑重启，本项不能证明系统重启后的自动恢复。
Chromium 缓存警告仍需检查，截图中的邀请提示、成员和右侧工具区仍部分中文。

### 发布工作流安全边界

2026-10-30：桌面发布工作流的 macos-13/macOS-14 替换为 macos-15-intel
与 macos-15，依据 GitHub runner-images 当前支持清单。构建 job 默认只读，
仅 release job 获得内容写权限；electron-builder 明确 --publish never。
产物上传只收集 dmg/exe，未生成安装包时报错，不再上传整个构建目录。
标签发布仅创建 draft prerelease，仍需人工核对安装验收后公开。
YAML 解析和草稿字段断言通过；尚未在 GitHub Actions 真实执行。

仍待解决：Server CI 包尚未汇入跨平台 Client 下载包；Windows Server
安装包未配置。本次修正不构成完整多平台发布验收。
参考：https://github.com/actions/runner-images/blob/main/README.md

### Server CI 汇入跨平台 Client 下载包

2026-10-30：将 macOS Server 构建拆到依赖所有 Client 构建完成的 job。
Mac Client 使用 ditto 将实际 .app 打为 ZIP（Intel 构建目录 mac、ARM 为
mac-arm64）；Windows 将唯一生成的 NSIS EXE 打为 ZIP。Server 收集三种
固定名称 ZIP，并逐个检查非空与 unzip 完整性，再嵌入 extraResources。
Release 依赖 Server job，不能在 Server 打包失败时先发布客户端。

本机验证 YAML 依赖关系、三个文件名与 Hub 下载路由表一致、builder 禁止
自动发布；下载接口相关 Go 测试通过。Windows/macOS CI 尚未远程运行，
不能据此宣称真实跨平台安装通过。Windows Server 封装仍未实现。

### 下载页与实际安装包说明对齐

2026-10-30：回归测试先复现下载页缺少英文且仍要求双击固定 AgentCollab.exe。
现改为 Windows 解压后运行包内 EXE 安装程序，Mac 解压后移入 Applications，
并提供对应中英文说明。移除仅凭 Mac 浏览器 UA 推断 Apple silicon 的推荐标记，
提示在“关于本机”确认芯片；底层 Runner UA 兼容逻辑不变。
下载页面、显式下载、未知平台、manifest、Runner 分离路由测试通过；
未将这些 HTTP 测试视为 Windows 安装或浏览器视觉验收。

### 首次加入与设备账号提示双语验收

2026-10-30：App 接入 useLocale；邀请成功、失效后普通登录、注册完成、
设备未连接和设备账号不一致提示，以及关闭/处理按钮接入中英文。
提示保留原文键并在渲染时翻译，因此无需重新登录即可切换已显示提示；
用户名仍原样插值，未知后端错误不伪造翻译。新增动态语言断言先失败后通过。
完整 Web 134 项、类型检查和生产构建通过；重建 Hub runtime 后，真实
Electron 45.1.0 普通成员英文登录、英文接入提示、发消息、隔离配对和刷新
保留验收通过。旧有缓存警告及生产 JS 大包警告仍存在，最终发行安装包尚未重建。

### Client 缓存警告根因与发布门禁

2026-10-30：新增外层 stderr 检查后，原验收虽然功能退出 0，仍因 cacheError
转为失败。定位到 harness 在异步创建 Hub/fixture 后才 require 产品 main，
Electron 已提前初始化网络服务。唯一改动为在 harness 同步阶段设置隔离
userData；连续两次完整登录/消息/配对/刷新验收无缓存警告。未更改产品缓存策略。

新增 check-client-desktop.cjs 要求子进程退出 0、有结构化 PASS、无缓存初始化
错误，否则失败；空成功进程和不存在的 Electron 路径都验证拒绝通过。
macOS Server 发布 job 现在在封装前编译隔离 Runner 并执行此门禁。
本机 Electron 45.1.0 验证通过，远程 macOS Intel/ARM CI 尚未执行。
前文缓存警告保留为历史记录；本项不代表最终安装包或 Windows 已验收。

### 会话协作模式与斜杠菜单双语

2026-10-30：Composer 的直接点名、专题讨论、结构化任务入口、模式说明、
输入提示、消息/附件标签、命令菜单和停止提示接入翻译。新增英文专题讨论
测试确认 /brainstorm 与用户混合语言原文不被改写；斜杠菜单确认英文说明
仍发送原始 /restart 协议。用户内容和 Agent 身份未翻译。
完整 Web 135 项与生产构建通过，其后补充命令协议用例，Composer 16 项通过。
本项未覆盖 TaskComposer 对话框本体、项目上下文选择器或全部侧栏。

### 结构化任务表单双语

2026-10-30：TaskComposer 的目标 Agent、任务标题/要求/验收、步骤归属、
文件执行来源与只读说明、离线等待、派发/错误/关闭入口接入中英文。
回归先复现英文标签不存在，随后验证英文填写→切中文→切回英文→离线派发，
输入内容与 Agent ID 保持不变，切换语言本身不提交任务。
完整 Web 19 文件 137 项、类型检查及生产构建通过；依赖与 notices 辅助脚本
4 项通过，npm 三份锁文件的依赖清单一致。此为表单与提交参数验收，不能
替代远程 Agent 执行或完整 UI 视觉验收，仍需补齐剩余侧栏/项目页面双语。

### 真实新成员注册与受邀加入

2026-10-30：隔离 Client harness 新增 register 场景；不预建成员账号，
在真实 Electron 页面填写显示名称、用户名、密码，提交创建并加入，等待
受邀工作区和群聊出现后发送消息。后续沿用隔离 IPC 配对与页面刷新保留检查。
register 和已有账号 login 两种场景在 Electron 45.1.0 均通过，且无缓存
初始化错误；发布 workflow 同时执行两种场景。Node 语法、YAML 解析与 diff
检查通过。Hub/团队/邀请仍由 fixture API 建立；配对仍是可信 IPC 而非点击
连接按钮，不能将本次验收说成全 GUI 全流程或跨机验收。

### GUI 本机连接与发现链路

2026-10-30：去掉 Client harness 中直接生成 Runner 邀请及调用配对 IPC 的
绕行。改为实际点击账号提示的处理按钮，打开添加 Agent 对话框，再点击连接
本机并查找；只读 getState 确认注册且运行，等待扫描完成按钮出现、无错误，
关闭对话框并确认账号连接警告消失。随后核对隔离凭证和刷新后身份不变。
已有账号登录、新账号注册两条真实 Electron 45.1.0 流程均通过且缓存检查干净。
不自动勾选/导入或运行任何扫描到的 Agent；发现结果为空也允许通过，因此
这项只证明 GUI 接入及扫描请求完成，不证明某类 CLI 已被发现或执行成功。
团队/群聊/邀请仍由测试 API 建立；最终跨机安装与真实 Agent 交付仍需独立验收。

### 开源候选扫描边界加固

2026-10-30：原检查器跟随符号链接，断链直接跳过，含 NUL 的归档也直接跳过。
新增回归先复现漏报，再改为 lstat：符号链接统一要求复核，不读取链接外部内容；
ZIP/TAR/GZ/DMG/EXE 等归档与安装包要求复核；非 ENOENT 的元数据读取错误
不再静默通过。报告仅列文件位置与原因，不输出检测到的值或链接目标。
4 项临时 Git fixture 测试通过，包含外部链接内容不读取与模拟 token 不回显；
加入源码 CI。当前 333 个候选文件无发现。该启发式扫描不检查 Git 历史、
不识别所有秘密格式，也不代替最终发布树与素材的人工隐私复核。

### Go 依赖许可原文与随包声明

2026-10-30：新增 GO-NOTICES.md，保留 go.mod 五个声明依赖（含 x/sys）
许可证及 Go runtime 许可证原文。生成器从 GOMODCACHE 精确版本路径读取，
不输出本地路径；拒绝未复核的 replace 和缺失许可证。兼容 Homebrew 将 Go
LICENSE 放在 libexec 上层的布局。此前 go list 元数据查询网络超时，改为
读取现有版本缓存，不以网络失败跳过依赖。
生成与 --check 一致，源码扫描 335 文件无发现；package-notices 已将此文件
加入两个桌面应用的资源声明。CI 在 go mod download 后核对生成结果。
尚需最终重打包核对资源；该模块级清单不是嵌入 C 代码或全部第三方代码的
穷尽法律审计，不能据此宣称所有许可证工作均已完成。

### 当前源码 Mac ARM 应用资源复核

2026-10-30：重建 Web、Hub、Runner 后，以已核对的 Electron 45.1.0 重打
Mac ARM Client 与 Server .app 到隔离临时目录。两个应用内五份声明
（项目 LICENSE、Electron、Chromium、npm 清单、Go notices）逐字节比对通过。
两个应用 codesign --verify --deep --strict 通过；仅 ad-hoc 签名，无 Developer ID
与 Apple 公证，不能视为普通用户无提示安装验收。

本次 Client .app 生成固定名称 ZIP，压缩完整性检查通过，重新封装到 Server；
Server 资源中的 ZIP 与源包相同，签名完整性再验证通过。只有 Mac ARM 下载包，
本地尚无本次 Windows/Intel Client。保留的旧 Runner 兼容包未在本轮重新构建，
因此该 Server 仍是资源验收产物而非可公开的最终发行包。未替换已安装应用。

### 无界面兼容包取消自动启动

2026-10-30：检查发现旧兼容安装器仍创建 macOS LaunchAgent、Windows Startup
快捷方式及 URL 协议。改为手动前台运行，复用现有 Runner 的无参数配对恢复逻辑；
不新增登录项、不修改图形 Client 协议、不自动删除历史启动项。README 中英说明
区分普通图形 Client 与恢复工具，明确关闭终端停止、重启后需手动打开。
打包不再带旧 plist/VBS/后台重连助手，新增 LICENSE 与 Go notices。

Mac/Linux 使用临时 HOME 和假 Runner 实际执行安装器，验证配置保留且无参数
手动启动；Windows 为静态策略检查，合计 3 项通过，未宣称 Windows 实机运行。
五种 Runner 包交叉编译/归档成功，全部检查声明存在且旧自启动助手不在归档中，
已替换 Server 打包输入目录中的旧包。前一轮临时 Server .app 尚需再次封装。

### 当前构建原生交付复验

2026-10-30：使用当前 server-desktop/runtime Hub 与 desktop/runtime Runner，
在独立临时团队再次完成真实 OpenCode 注册、加入群聊、分发确认文本任务及回群。
收紧 smoke 检查：创建响应读取 task 字段，查询同一个 task ID 的 delivered 状态，
检查目标 Agent ID、交付消息 sender ID、metadata 的 task ID/state，以及回复中的
随机确认文本完整行。消息还包含服务器追加的标题与执行凭证，不将整个正文
错误地要求等于原生确认文本。最终严格检查通过。

同轮 Go 全量测试、go vet、diff 空白检查通过；336 个开源候选文件启发式扫描
无发现。此项为 API 驱动的单机原生 CLI 验收，不是全 GUI、多机或所有 Agent
兼容性证明；未修改用户现有团队、安装配置或发布到 GitHub。

### 邀请与接入双语回归

2026-10-30：群工具中的邀请与接入面板接入语言状态，包括加入团队、生成邀请、
打开本机 Agent、复制链接及相关说明和兜底错误。新测试先验证英文入口缺失，
修复后验证英文加入回调收到原始链接，切换中英文不丢失输入，接入按钮仍调用
原注册入口。群权限条件及链接生成逻辑未变。Web 19 文件 138 测试通过，生产
构建通过，仍有约 587 kB 主 JS chunk 警告。其余成员卡片、项目面板等未因此
自动完成翻译；本轮未重打包桌面应用。

### Agent 状态双语与桌面接入复验

2026-10-30：补齐就绪、排队、执行中、不可用、离线等待任务及普通离线六种
状态的英文标签和解释，原有状态分类/颜色不变。原生会话续接说明与最近心跳
也使用当前语言。六组回归覆盖英文说明及切回中文；Web 全量 144 测试通过。
重建 Hub 后，以 Electron 45.1.0 在隔离账号/数据目录运行注册型桌面验收：
英文注册、入群、发消息、GUI 连接本机、刷新保留身份均通过，缓存检查通过。
macOS 输出一条 IMKCFRunLoopWakeUpReliable 系统输入法日志，未导致流程失败。
尚未重新封装最终应用；构建仍提示约 589 kB 主 JS chunk。

### Windows Server 发布流水线缺口

2026-10-30：原流水线只有 Windows Client。新增 Windows x64 Server NSIS 配置，
保持独立 appId/名称/图标，以当前用户安装、不新增登录启动项。新增 Windows
Server job，等待三种 Client 下载归档并随包提供，运行 Hub 真实数据库 smoke
和 Electron Server 引导验收后才打包；草稿发布必须等待此 job 成功。

Windows Hub 依赖 SQLite CGO，不能沿用 Runner 的 CGO_ENABLED=0。新增构建
脚本要求 gcc，开启 CGO，包含创建账号、文件读写与重启持久化测试，并在结束
时恢复调用者的 CGO/CC 环境变量。GitHub Windows 2022 镜像的官方软件清单
当前列有 gcc；运行时仍检查实际可用性，不因缺编译器跳过数据库测试。
本地 PowerShell 解析通过、两项配置约束测试通过，workflow YAML 解析通过。
未在 Windows 执行，尚无该流程产出的已验收 EXE；不得标为 Windows 已发布。

### 桌面回归与 Windows 构建失败路径

2026-10-30：Client/Server 桌面端 23 项回归通过，覆盖主版本配置隔离、网络候选、
引导语言、仪表盘及启动可见性等现有测试范围。新增 PowerShell 临时仓库测试，
实际执行构建脚本的目录复制与 finally，外部 npm/go/node 使用桩函数，不运行
真实编译器。成功、缺编译器、Web 构建失败、Hub 编译失败及数据库验收失败五种
路径均检查调用顺序、停止后续步骤、环境变量和当前目录恢复。连同两项配置检查
共 7 项通过，无跳过；加入 Windows 发布 job。此证据仍不是 Windows 原生运行
或安装验收。GitHub CLI 登录有效，尚未配置远端、创建仓库或上传源码。

### Server 桌面验收的端口隔离

2026-10-30：移除 harness 固定 18081 端口，先设置独立 Electron profile，再由
操作系统分配临时端口，释放后立即启动被测 Hub；仪表盘必须报告同一端口，不能
把自动回退当作原端口成功。两份真实 Electron 验收并行通过，使用不同端口，
退出后独立 TCP 检查均拒绝连接。故意注入失败仍返回退出码 1，不伪报通过。
测试仍有禁用登录项的系统权限日志；没有据此新增登录项。截图为测试 harness
启动，版本标题显示 Electron 版本，不作为正式发行截图使用。

### JavaScript 运行时版权文本

2026-10-30：Web、Client、Server 三个完整 npm 依赖树 audit 均返回 0 已知漏洞；
依赖清单与 Go notices 一致。补充 NPM-RUNTIME-NOTICES.md，收录前端实际生产
依赖 React、React DOM、Scheduler、Phosphor 的许可证原文。生成器按三份锁文件
查找生产依赖，核对安装版本；缺少或空白许可证/NOTICE、链接型声明会失败。
测试覆盖版本不一致、缺失/空白声明、版权保留及不泄漏本机目录，均通过。
两个桌面应用 afterPack 新增该资源，源码 CI 核对声明一致性；最终应用仍需
重打包验证。343 文件候选扫描无发现。npm audit 只代表当次数据库检查，不能
证明没有未知漏洞；许可收集不等于完整法律审查。

### 封装应用首次设置实测

2026-10-30：新增 smoke-packaged-server.mjs，直接启动 Mac ARM .app 内的应用
入口，通过仅本机开放的临时调试端口驱动真实引导表单和 IPC，不替换主进程。
核对应用版本 1.3.3（而非 Electron 版本）、隔离数据路径、Hub 健康、语言切换
保持团队 ID、正常退出及端口关闭。两次通过。成功后移除隔离数据，只保留截图。
英文真实仪表盘截图经目视检查后存入 docs/product/screenshots，只有虚构团队
与临时回环地址，无真实账号/凭据/本机目录。中文版截图发现旧文案仍暗示所有
成员安装 Client，尚未纳入正式素材，需后续修正文案并重新封装。完整协作视频
与 Windows/Intel 安装验收未完成。

### Server 双语安装提示一致性

2026-10-30：测试复现中文提示仍写“其他成员只安装 Client”，随后同步修改 HTML
与翻译键，明确浏览器参与、仅接入本机 Agent 才需 Client。JSDOM 回归通过。
重新封装 Mac ARM Server 后，实际应用验收加入两种语言帮助文案断言，首次
设置、健康、切换语言、退出仍通过。新中文截图经目视检查无私人信息，加入
中文演示文档；英文截图同时更新。此项修正前节记录的文案问题，非完整视频。

### 封装验收进入发布门禁

2026-10-30：Mac ARM/Intel 与 Windows Server 发布 job 均在上传安装包前运行
smoke-packaged-server.mjs，限时 3 分钟，失败阻断该 job 和后续草稿发布。
YAML 解析及步骤先后关系检查通过，但远端流水线尚未运行。脚本补充通过真实
IPC 重启 Hub，验证团队 ID、存储位置及端口保持不变；本地 Mac ARM 实际应用
重启验收通过。仍不能替代另一台电脑访问、安装器交互及 Windows 实机验收。

### Codex 真实调用未通过与旧文档纠偏

2026-10-30：本机 Codex CLI 0.132.0 报告已通过 ChatGPT 登录，但两次隔离团队
真实任务均进入 failed，不能标记为可用性验收通过。第二次沿用当前目录启动参数，
只把不受限执行替换为只读沙箱，仍失败；尚不能断言是模型服务、超时或适配问题。
测试补充不输出原始错误正文的分类诊断，下一轮需定位真实失败原因，不能用模拟
返回替代成功。两次临时 Hub/Runner 均已退出并清理临时团队数据。

本机 Agent 文档移除固定私人 Hermes profile 和 Homebrew 路径假设，修正桌面
主版本配置目录及“不加入开机登录”的流程，区分桌面 Client 与高级命令行 Runner。
明确有适配定义不等于已通过真实验收，当前原生执行只适合可信团队。源码隐私扫描、
运行时许可证与封装声明相关 6 项测试通过。本轮未上传源码或重打包应用。

### Codex 独立复现与兼容矩阵

2026-10-30：分类诊断确认平台 Codex 失败为 timeout。0.132.0 独立调用也存在
模型目录 unknown variant max 和连接重试。npm registry 查询版本为 0.166.0，
将其安装在临时审核目录而非覆盖用户全局 CLI；同一平台 smoke 仍超时。
新版独立探测保留用户配置时观察到 MCP 错误；随后只更改为忽略用户配置、保留
同一模型 gpt-6-astra 与 medium 推理档位，45 秒内 MCP 错误消失但仍有流断开，
未收到确认。探测均已结束；不能据此宣称“升级即可解决”或“只是 MCP 问题”。

新增中英兼容矩阵，区分适配定义和真实任务验收。修正文档中“Hub 不接收本地绝对
路径”的过度承诺：执行凭证可能带有 CLI 路径等元数据。Runner、HTTP API、SQLite
三个包测试通过。仍未上传；需要继续完成其他发布关卡，Codex 当前保留未通过状态。

### 真实项目分支、审查与合并验收

2026-10-30：扩展 smoke-community.mjs 的可选 --project 模式，使用独立团队、
真实 Hub/Runner/OpenCode 和纯文本示例目录。首轮全链路通过；第二轮增加任务
关联和重启断言后再次通过。通过上传接口创建步骤目录及分支，真实 Agent 审查
返回 passed；显式合并前 Git main 内容仍为 before。普通成员的合并请求返回
403，Owner 合并后 main 内容为 after；真实合并后校验返回 passed，事项 done。
两条审查/校验任务均 delivered、指向目标 Agent、各有对应回群消息。重启真实
Hub 后事项状态、校验状态及主分支文件仍一致。测试进程均退出，临时团队已清理。

相关目录上传/负责人权限接口测试重新执行通过；脚本语法检查通过，347 个源码
候选隐私扫描无发现。中英演示文档加入可复现命令与模型费用/原生历史说明，兼容
矩阵更新 OpenCode 证据范围。此为单机 CLI/API 集成，不冒充界面录像或跨机验收。

### 合并前审查界面双语

2026-10-30：WorkItemReviewPanel 接入现有语言状态，补齐审查标题、关闭按钮、
文件数量及状态、冲突提示、合并按钮和处理中状态。先以 3 项失败测试复现英文
缺失，再实现翻译；新增文件状态与未知状态保留测试，共 8 项通过。测试验证语言
切换保留原始分支名/文件名，回调不变，冲突或无合并回调时无合并按钮，处理中
禁用按钮。全量 Web 20 文件 152 测试通过；最终文案调整后针对性 8 项和生产构建
再次通过。主 JS 约 589.5 kB 的 chunk 警告仍在。本轮未重打包桌面应用，其余项目
页面尚未因此自动完成英文覆盖。

### 执行时间线双语及失败状态优先级

2026-10-30：ProjectTimeline 的标题、排队/领取/运行/阻塞/失败标签、尝试次数、
任务数量、阶段、空状态及日期格式接入语言状态。7 项回归覆盖五类原生任务状态、
原始中文标题不变、点击回到原话题、语言切换以及失败校验优先级。检查中发现
stageFor 在合并后校验 failed 时仍可能显示“已并入团队底稿”；先复现失败再修复，
现在优先显示“合并后校验未通过”，同一 helper 的办公室卡片也受益。
全量 Web 21 文件 159 测试通过，生产构建通过，主 JS 约 591 kB 警告仍存在。
办公室主视图与详情仍有未翻译文案，本轮未重打包或上传。

### 项目办公室与详情双语

2026-10-30：办公室主视图、步骤卡片、团队席位、待处理/近期活动及事项详情四个
标签页接入现有语言状态；用户标题、目标、分支和文件名不翻译，状态回调不变。
使用临时 TypeScript AST 工具进行静态文案机械替换，随后修正可选字段回调的类型
收窄并补充词条；未向项目新增运行时依赖。3 项界面回归覆盖空视图、详情各页、
状态更新/打开原话题、步骤计数及语言切换。一次组合回归中原有步骤编辑测试超时；
未调高时限，单独重跑 191ms 通过，随后全量 22 文件 162 测试通过。超时原因尚未
确定，不将单次成功视为已消除潜在偶发问题。最终生产构建通过，约 597 kB chunk
警告仍存在。尚需真实渲染布局检查，ProjectCockpit 主结构看板也仍有中文文案。

### 真实 Electron 项目办公室渲染

2026-10-30：Client harness 新增 AGENTCOLLAB_SMOKE_PROJECT=1，用真实 Hub API
创建两个虚构步骤及一个真人事项。重建 Hub 内嵌前端，在 Electron 45.1.0 中完成
英文登录、邀请入群、发消息、连接本机、刷新保留身份后，实际点击协作台与事项。
检查办公室英文内容、步骤归属、水平无溢出和详情页。初次截图捕获了抽屉入场动画，
随后添加等待动画完成、双帧绘制、opacity=1 及 elementFromPoint 顶层命中检查，
再次通过；目视确认稳定态抽屉不透明、未被工具栏遮挡。

实际页面仍有外层导航/侧栏中文，三栏时步骤卡片较窄，截图仅保留临时验收目录，
不作为最终英文宣传素材。Mac 发布 job 的登录型 Client 验收开启该模式；CI 尚未
远端执行，本地已验证两次。没有更改用户团队数据或新增开机项，未重封装正式应用。

### 群聊导航与连接说明双语

2026-10-30：群聊顶层协作台入口、三种协作台视图和五类消息连接状态说明接入
语言状态。新增五项回归，保留原始群名，并实际点击协作台与时间线。初次组合
测试发现新增导航测试留下 localStorage 视图选择，影响后续旧测试；通过每项测试
清空隔离存储解决，不改变应用记住用户视图的行为。全量 Web 22 文件 167 项通过，
TypeScript 与生产构建通过；主 JS 598.54 kB 的体积警告仍在。

重新构建内嵌前端 Hub（1.3.3 / 4819e66 dirty / 2026-10-30T10:22:04Z），
运行 AGENTCOLLAB_SMOKE_PROJECT=1 的真实 Electron 45.1.0 Client harness，
邀请、英文登录、发消息、连接本机、刷新保留身份、办公室与详情检查通过。
目视截图确认顶部导航和消息状态英文正常；右侧工具及部分成员标签仍有中文，
办公室三栏下的步骤区域仍拥挤，尚不能宣称全界面英文或最终视觉验收完成。
本轮没有打包新安装包，没有 GitHub 上传。

### 群工具入口双语与默认步骤权限回归

2026-10-30：翻译群工具标题、默认步骤选择/说明、Agent 库空状态与折叠标签、
我的 Agent 设置、邀请入口及高级连接检查说明。新增英文回归先复现缺失，随后
通过；另覆盖默认步骤的可管理/不可管理状态、原始中文步骤名不变及选择/清空
回调。全量 22 文件 170 测试与生产构建通过，主 JS 600.42 kB 警告保留。

Hub 内嵌前端重建时间 2026-10-30T10:27:06Z；真实 Electron 45.1.0 Client
隔离验收通过，并新增侧栏英文入口和普通成员不能修改默认步骤的实际 DOM
断言。成员树、Agent 详情抽屉及其他深层界面仍待语言覆盖，不能据此认定全界面
双语已完成。未更新安装包、未上传 GitHub。

目视同轮截图发现英文默认步骤标签导致选择框越过侧栏边界；将该行改为可换行、
选择框宽度受父容器约束。添加真实 DOM 边界断言，重建 10:29:03Z Hub 后再次
通过完整 Client harness。该尺寸断言覆盖本次 1440×1000 窗口，不代替全尺寸验收。

### Agent 群内设置双语与权限边界

2026-10-30：Agent 详情入口、抽屉标题、四种响应模式、群内角色、共享 Skill
数量和权限说明接入翻译，保留角色原文和现有更新回调。两项英文测试覆盖可管理
与不可管理的 Agent：前者可更新响应模式并跳转个人授权设置，后者无编辑控件
和授权按钮；同时检查关闭抽屉。整块文本断言再次发现原生连接类型漏译，修复后
全量 22 文件 172 项测试通过，TypeScript/生产构建与 git diff --check 通过。
原生 CLI/ACP 类型标记翻译不改变实际调用方式。主 JS 602.01 kB 警告仍保留。
本轮为组件交互验收，尚未重新进行带已注册 Agent 的真实桌面抽屉验收或打包。

### 真实桌面注册、入群与 Agent 设置

2026-10-30：新增可选 AGENTCOLLAB_SMOKE_AGENT_DRAWER=1，在隔离 Hub/Client
中实际扫描本机 OpenCode、勾选注册、加入群、打开英文抽屉、检查四种响应模式与
主人权限入口、关闭，再刷新验证 OpenCode 群成员保留。重建内嵌前端 Hub 时间
10:49:26Z，Electron 45.1.0 两次通过；第二次增加截图与刷新后的成员断言。
未提交模型任务。目视抽屉显示正常，外层成员树仍有中文。截图含本机设备名与
Skill 数量，暂存临时目录、不复制为公开宣传素材。中英文演示文档补充复现命令
和隐私限制；脚本语法检查及 diff 空白检查通过，未重封装安装包或上传。

### 二进制候选文件审计加固

2026-10-30：源码扫描原先静默跳过包含 NUL 的文件，可能把二进制或 UTF-16
配置误算为已检查。新增回归先复现该行为，再改为 binary-file-review，5 项
扫描器测试通过。扫描当前 350 个候选文件返回 8 项人工审核要求（退出码 1，
并非无问题通过）：Client/Server 各自 PNG、ICO、ICNS，以及两张 Server 截图。

已用 file 确认格式、记录 SHA-256，并目视检查两张 PNG 图标及两张 Server
截图；内容分别为应用图标和虚构 Community Demo 团队、回环地址，无可见凭证。
ICO/ICNS 尚需内部图像和元数据检查；目视检查也不能代替二进制元数据审计。
扫描器不会输出密钥值；Git 历史及最终源码快照仍须单独检查，尚未上传。

### 图像资源审核与摘要绑定

2026-10-30：检查上述 8 个资源的 PNG/ICO/ICNS 容器。PNG 未发现尾部附加数据；
图标文本块为三个日期字段，截图含颜色配置。ICNS 声明长度匹配文件，包含图像
块、尺寸/色彩 EXIF 和系统图标属性；ICO 六个连续 DIB 图像覆盖文件长度。
使用 macOS sips 解码 ICO/ICNS 到临时文件，目视确认 Client 人形、Server
服务器图形与 PNG 一致。未更改原图。该审核是格式/元数据与可见内容检查，
不是任意隐写数据的数学排除证明。

scripts/reviewed-public-assets.json 记录 8 项路径与 SHA-256，只认可审过的
精确字节；新增或修改资源必须重新审核并更新记录，禁止自动批量批准。
扫描报告单列 reviewedBinaryFiles，未知二进制仍失败。新增回归验证改名或
追加字节会重新报审；6 项测试通过。当前 351 个源码候选文件扫描退出 0、无
新增发现；最终公开快照、历史与安装包仍需分别审计，未上传。

### 独立源码副本构建与首次使用

2026-10-30：按 Git 候选清单导出 349 个实际文件到全新临时目录，未复制原
Git 历史、依赖目录、安装包和运行数据；候选计数 351 中有两项已删除的旧前端
产物，因此实际文件数为 349。在独立副本执行 npm ci --prefix web 成功，
安装审计 0 已知漏洞；172 项 Web、23 项桌面逻辑、6 项源码扫描器测试通过。
make web-build、go test ./...、go vet ./... 全部通过，构建产物与原工作树
当前前端内容相符，600 kB chunk 警告仍在。源码扫描含 8 个已审核摘要、无发现。

再从该副本编译 Hub，真实 smoke-community 通过首次管理员、工作区/群聊、
第二真人邀请/独立登录、非管理员拒绝、文件上传/成员下载/外部账号拒绝，以及
Server 重启后的群和文件持久化。本轮未调用原生模型、未运行桌面安装包。
此为同一 Mac 上独立依赖目录验证，仍使用系统 Go/Node 和包缓存，不等于一台
全新 Windows/Linux 机器验收。尚未公开上传，正式快照仍须在最终修改后再生成。

### 公开介绍与实测边界对齐

2026-10-30：中英文 README 加入原生 Agent 验证矩阵入口及当前实际范围，明确
有适配器不等于通过端到端验收；列明 OpenCode 单机证据、Codex 超时和其他类型
未验收，不将 Mac 本机证据扩大到其他系统或跨机。中文 README 修正左侧详情
入口、协作台名称、个人 Agent 设置入口和 Hub 账号管理描述；双语用户指南采用
当前 Messages live / Messages auto-syncing 和中文消息同步标签。文档变更后
源码扫描仍无发现，未上传。其余长篇历史手册仍需继续核对，未宣称全部文档完成。

### 历史操作手册的误导性说明修正

2026-10-30：USER_MANUAL 不再自称当前 1.3.1 完整指南，明确标注历史来源与
本轮修订，并优先导向中英文首次协作指南及证据矩阵。删除“所有真人都装 Client”
的矛盾要求；安装版本以实际发布说明为准，下载中心有包不等于系统验收通过。
修正权限入口和响应模式位置，并纠正“不会看到本地路径”的绝对承诺：能力清单
过滤不保证凭证、回复及附件自动脱敏。4 个新增本地链接和 diff 检查通过。
本轮未改运行代码，不以文案修订声称完成剩余功能或平台验收。

### 独立副本发布脚本验证

2026-10-31：在上述独立源码副本继续执行依赖清单、运行依赖声明、安装包声明、
Runner 安装脚本和 Windows Server 打包规则测试，15 项全部通过。依赖清单与
三个 npm lockfile 一致，npm 生产依赖声明及 Go 模块/运行时授权声明检查均退出 0。
这是脚本逻辑与清单一致性验收，不是 Windows 真机安装或签名验收；未增加启动项，
未改用户运行数据，未上传 GitHub。最终源码修改后仍须重新生成公开快照。

### 成员面板语言回归

2026-10-31：群聊成员标题、真人/Agent 计数、整体展开收起按钮及无 Agent 提示
接入语言字典。新增英文回归覆盖计数、展开/收起的可访问状态和内容恢复；中文
原有行为保持。完整 Web 套件 22 文件、173 项通过，TypeScript/Vite 构建通过。
600 kB 主脚本警告仍存在。此轮没有把尚未翻译的角色管理等细节标记为完成，
未重封装桌面程序，未将自动化测试替代最终安装包和真实界面验收。

### 首次群聊引导双语与状态语义

2026-10-31：RoomOnboarding 的空库、已注册未入群、单 Agent、多 Agent 和检查中
状态接入中英文。删除仅按成员数量显示“本地多 Agent 已就绪”的不实保证；提示
检查会提交真实任务，应分别查看成功/失败。说明普通真人聊天无需 Client，去掉
过时的右侧面板定位及注册即“可用”的表述。保持单 Agent 可聊天、两个成员才可
点击检查的现有行为。新增三项测试覆盖两个入口、语言切换、动态计数、检查中禁用
及用户名称/错误文本原样保留；首次测试因按钮单复数不匹配失败，修正预期后，
引导与 RoomView 共 14 项通过，TypeScript 和 diff 检查通过。本轮未重新跑完整
套件或桌面安装包，不宣称整体双语验收完成，未上传。

### 验证任务部分失败的反馈修复

2026-10-31：检查真实按钮调用链发现 Promise.all 任一失败会隐藏同批已创建任务的
即时回显，并向未处理的点击事件抛出异常。改为分别收集结果，保留成功任务和消息，
错误带目标 Agent 名称；刷新任务列表失败时提示勿重复提交。全部派发失败则不刷新。
同时修复成功消息出现后引导页消失、导致剩余失败提示也消失的问题：已有消息时
仍显示验证错误。新增三项派发测试（部分失败、顺序、同步异常）和一项消息区
错误可见性回归。派发/引导/App 8 项通过；后续派发/RoomView 15 项通过，类型和
diff 检查通过。仍需最终安装包真实网络故障验收，不以 mock 测试声称已完成。

### 整体回归与桌面首次引导复验

2026-10-31：默认并发的完整前端测试出现 8 个 fork worker 启动超时，不能作为
通过证据。进程结束后用 maxWorkers=2 重跑，24 文件、180 项全部通过；将该
上限写入 Vitest 配置后，再用标准 npm run test:run --prefix web 执行，同样
180 项通过。未提高断言超时或跳过测试。当前候选源码扫描 findings 为空。

重建 webdist 和隔离验收 Hub（本轮 go build 未附版本 ldflags，非发布二进制），
真实 Electron 45.1.0 Client 两次通过英文邀请登录、首次引导、成员展开收起、
真人发消息、GUI 连接、刷新保留会话/语言/设备身份。验收脚本新增引导英文断言；
截图先等待两帧，避免 DOM 更新而截图仍是旧画面。第二张截图已目视确认消息
显示，仍发现角色标签和成员摘要中文、顶部提示遮挡部分导航，不能当作全界面
双语完成或最终宣传素材。截图保留在临时验收目录，未加入公开资源或上传。

### 顶部提示遮挡修复

2026-10-31：按真实截图复现，加入提示与设备账号提示原用 fixed 定位覆盖群标题
和导航。加入 app-layout 容器，使提示占据正常布局高度，三栏占用剩余空间；
设备说明允许换行。App 两项测试、类型检查和构建通过，重建隔离 Hub 后真实
Electron 45.1.0 两次验收通过。新增几何断言：提示底边不越过主区域顶边、群
标题命中测试不被遮挡；第二次增加关闭邀请提示后主区回收高度的检查。仍完成
发消息、连接、刷新保留设备身份。目视截图确认导航完整可见。未验收所有窄屏
布局，角色标签翻译仍待完成；未重新发布安装包，未上传 GitHub。

### 消息身份与成员状态双语

2026-10-31：消息的真人/任务/交付/失败标签、专题说明、附件可访问标签和执行
凭证入口接入语言字典，时间随界面语言格式化；解析中文协议标记保持原样。
新增英文回归验证用户名、正文、任务标题（含 $&）、凭证内容不被翻译或替换，
凭证仍默认折叠。成员树的群主/管理员/成员选项及 Agent 在线计数同步翻译，
既有权限分支不变。MessageItem/TopicThread 15 项、成员面板/消息 42 项通过；
新增在线计数断言后成员面板 36 项通过。类型及 diff 检查通过。本轮未重新
打包或目视验收新翻译布局，仍有其他深层操作文案未覆盖，未上传。

### 第二种原生 Agent 的真实确认

2026-10-31：扩展隔离 smoke-community 的确认模式以接入本机 Hermes（不开放
其 --project 模式）。首轮使用 none 工具集被 Hermes 以退出码 2 拒绝，属于
验收参数错误，不是产品交付成功。查阅本机 CLI 帮助/源码后改为 clarify 工具集，
关闭规则注入、移除显式 yolo；one-shot 自身绕过审批，不能称为操作系统沙箱。

第二轮 Hermes 0.22.0+5.g9338452 真实随机确认通过 Runner 回到正确 Agent/任务，
退出 0；同时 HTTP 邀请、权限拒绝、文件及重启持久化检查通过。未调用项目技能，
未更改用户模型配置；模型/原生历史可能保留。临时团队自动清理。双语演示文档、
兼容矩阵和 README 已写明限定范围。OpenCode/Hermes 分别成功不等于共同讨论
或跨机协作成功，仍需联合验收；尚未上传。

### 联合点名与上下文断层修复

2026-10-31：新增 --pair-hermes 隔离验收。同一 Runner 注册两个独立原生 Agent，
一条消息点名 OpenCode/Hermes，断言恰好两个任务、各自身份和原话题绑定，然后
只点名 Hermes 追问前文随机串（追问不包含完整串）。连续两次发现前两项成功、
追问丢失上下文。第二次诊断确认 Server task.prompt 含前文串，但真实回复没有。

定位 Runner buildPrompt 提取“当前消息”后丢弃了历史、角色与协作目录。新增
失败回归先复现，再保留独立协作上下文块，去掉重复身份包装与末尾当前消息副本；
用户原文仍在第一段。补充普通任务不虚构历史、汇总回调保留专家结果的回归。
Runner 全套和随后 go test ./... 通过。重建隔离 Runner 后第三次真实联合测试
退出 0：两个原生任务回同话题，Hermes 定向追问成功找回前文串；权限/文件/重启
基础验收也通过。中英文演示步骤、兼容矩阵、README 已更新。

该结果仍是单机同主人 API 驱动测试，不证明跨机、跨主人或自主多轮委派。Hub
提供的是有长度和条数上限的近期上下文，不是所有永久历史。当前修复未更新用户
安装包或后台进程；后续须重新封装并验收。未上传 GitHub。

### 上下文修复后的 Mac ARM 候选包

2026-10-31：重建最新 Web/Hub/Runner，候选标识为 1.3.3 /
4819e66-context-candidate（工作树候选标签，不是假称新 Git 提交）。使用现有
Electron 45.1.0 和 electron-builder 26.15.3 在临时目录封装 Client/Server。
未覆盖 /Applications 或用户数据，未增加登录启动项。两包 codesign --verify
--deep --strict 通过，但仅 ad-hoc 签名，无 Developer ID/公证。

Client 内 Runner SHA-256 与构建产物一致：
cf02b34792ccba595198e8f3fc69596fa03ac0ec3d9ea9d53b526f8cf55ec7d3。
Server 内 Hub 与构建产物一致：
34e7c1451b92ddbe41feb7682f15728662a13a8829904a97d8ef9a58072591a3。
Server 内下载 Client ZIP 与新候选一致：
ae80e15f2579b961185c78c7812b488ff91442cc98f1c614b900f9ba9b502ba8。
临时打包配置明确只纳入新 Mac ARM Client，不把旧版多平台 Runner 压缩包混入。

真实候选 Server 的首次安装表单、Hub 健康、重启团队保留、中英切换、退出均
通过 smoke-packaged-server；资源含 notices。Client 包内版本和二进制一致性
已查，尚需对实际 Client .app（不是源码 Electron harness）做完整交互验收。
临时包装配置不是最终跨平台发布流水线；仍须处理正式源码快照/版本及演示。
当前未上传 GitHub。

### 真实 Client 包重开与身份保持

2026-10-31：新增 smoke-packaged-client.mjs，启动候选 .app 内真实可执行文件，
通过回环 CDP 检查真实渲染器/IPC，而非替换源码 main 的 Electron harness。
临时 Hub 和 Client profile；实际邀请链接、英文登录、连接按钮、扫描列表、
Browser.close 正常退出，再无邀请参数启动。三轮逐步增强后通过：账号/语言
保留、自动恢复 Runner，Hub/Runner/device 三个标识均非空且前后一致。未增加
开机启动，未触碰原团队。未执行模型任务，因此该结果不替代此前原生调用验收。

首两轮截图过早，第三轮等待发现列表完成并经过两帧后采集；目视确认英文扫描
列表正常出现，含真实本机路径和设备名，仅作临时证据，不可直接公开。成功运行
清理账号/设备凭据、临时数据库和配置，仅留下截图；失败路径保留诊断数据的
限制已写入双语演示文档。当前仍不是网络下载隔离标记、Developer ID、公证或
Windows/Intel 真机验收。未上传 GitHub。

### 打包入口防止旧文件混入

2026-10-31：检查正式 Make 入口发现两个本地 Mac 打包目标会自动复制已有的
同版本 Windows ZIP，无法证明其来自本轮构建。删除这条隐式复用；多平台下载
应来自显式的、已验收的同轮构建产物，不能凭文件存在就带入 Server。
桌面依赖安装改为 npm ci，遵守已审核锁文件。同时修复 Intel Mac 的打包目录：
Electron Builder x64 默认输出 mac，而不是 mac-x64。

新增 mac-package.test.cjs 通过 make -n 检查展开后的配方：两个目标均不复用
旧 Windows ZIP、两个安装入口使用锁文件、Intel 路径正确。先出现预期失败，
修复后 5 项通过，并纳入 Source checks。该测试不执行安装或构建，不等于
Intel 真机验收。未改动已安装应用、用户数据或启动项；未上传 GitHub。

### 最新源码导出的独立构建验收

2026-10-31：从 Git 候选文件（包含当前新增文件、排除忽略文件及已删除文件）
复制出新的临时源码目录，初始约 5.7 MB，不包含旧 .git 历史、node_modules、
安装包、运行配置或团队数据库。从该副本执行以下命令并全部退出 0：

```bash
npm ci --prefix web
npm run test:run --prefix web
make web-build
go test ./...
go vet ./...
go build -o bin/agentcollab-hub ./cmd/hub
node scripts/smoke-community.mjs bin/agentcollab-hub
node --test desktop/*.test.cjs server-desktop/*.test.cjs scripts/*.test.cjs scripts/*.test.mjs
node scripts/dependency-inventory.mjs --check
node scripts/npm-runtime-notices.mjs --check
node scripts/go-notices.mjs --check
```

结果：网页 24 文件/181 测试、桌面与发布辅助脚本 49 测试通过，无跳过；Go
测试及 vet 通过。Hub smoke 验证新账号/工作区/群聊、第二真人邀请、独立重新
登录、权限拒绝、知识文件上传/成员下载/外人拒绝，以及重启后的群聊和文件保留。
三类授权清单与锁文件/模块匹配。前端仍有约 606 kB 主包的体积警告，未隐藏。

这是同一台 Mac 上的新目录验证，仍使用已有系统编译器、npm/Go 缓存及网络，
不冒充全新操作系统安装或 GitHub 干净克隆验证。此轮没有真实模型调用，不替代
此前 OpenCode/Hermes 原生执行证据；新导出的源码也尚未上传 GitHub。

### 实际原生项目截图及话题语言修补

2026-10-31：给项目 smoke 增加可选 Electron 网页截图步骤，仅在真实原生审查、
授权合并、独立验证与重启断言通过后运行。第一次业务检查通过但截图导航失败：
已完成事项不在执行中列表，应从最近流转打开。修正导航和语言选择后重跑通过，
人工检查原始截图，加入双语演示文档。图片只含虚构用户、Demo Device、随机
确认串及样例相对路径；未展开本机执行凭证或暴露实际文件目录。

这是 API 编排下的真实 OpenCode 审查和网页结果，不声称改动是 Agent 编写，
不声称已完成视频或跨机验收。截图保留可见的翻译缺口，不做后期伪造。
随后在源码修补话题计数、交付状态、回复/停止、步骤关联与验收说明的语言调用。
新增英文测试确认用户和 Agent 中文原文不变、回复仍指向原话题。
此源码修补尚未重新封装；已公开候选截图记录的是修补前的实际状态。

### 发布复核：规范与需求分开检查

2026-10-31：以 4819e66d5df84bdf5dfb9f2104b9f81cdada5b89 为固定基点，
包括未跟踪候选文件，执行两条独立只读复核。不是全量安全审计。

规范轴发现：隔离数据目录启动仍可能调用系统级旧服务迁移、协议注册和登录项
修改。已在 Client/Server 为这些操作加入隔离模式保护；生产仍不新增开机启动。
Windows 生产启动按可执行名 taskkill 所有 Runner 也可能误杀其他团队任务，
已移除此无所有权判断的终止动作；Client 只管理自己启动的 Runner。新增四项
防回归检查先失败后通过，加上桌面测试合计 27 项通过。隔离源码 Client 的登录、
连接、重载与项目详情流程再次通过。新保护尚需重新封装后验收；不能追溯性声称
旧测试完全没有系统级副作用。注册与账号角色接口抽样未发现普通成员提权。

需求轴仍待处理：下载 Client 构建后缺少真实封装包的 CI 闸门；Windows Hub/
Runner 未统一写入版本和提交标识；源码快速开始没有完整的 Client 获取路径。
发布工作流目前仅生成 draft/prerelease，不是自动公开正式发布。这三个缺口
必须修正或明确阻止相应产物进入公开下载。尚未上传 GitHub。

### 发布流程复核缺口修正

2026-10-31：Mac/Windows Client 下载归档之前加入 smoke-packaged-client，检查
实际构建的应用而非源码 harness。失败不继续归档；Windows 先构建带 SQLite 的
测试 Hub。新增工作流顺序断言，禁止 continue-on-error 绕过该关卡。GitHub 实际
运行尚未发生，因此这是已配置的闸门，不是两种系统均已通过的证明。

新增 native-build-info.mjs，要求 Client/Server 版本一致、合法 Git 提交及 UTC
构建时间。Windows Hub、Runner 及流水线采用其 ldflags；元数据生成失败阻止
构建。PowerShell 控制流测试覆盖正常与 npm/metadata/go/node/compiler 失败，
确认恢复环境和工作目录；与元数据及闸门测试合计 12 项通过。本机实际编译
Runner 后 version 输出 1.3.3、当前基点提交和构建时间。当前工作树未提交，
这个基点不代表所有改动已入 Git；最终发布须用干净快照的新提交重新构建。

中英文快速开始补充 macOS Client 源码启动、独立 profile、连接团队和发现 Agent
的完整路径，以及 Apple Silicon/Intel 输出位置；继续明确未验收系统边界。
没有修改已安装应用或增加开机启动，尚未上传 GitHub。

### Public source and Linux CI follow-up / 公开源码与 Linux CI 复核

2026-10-31: The clean source snapshot is public at
https://github.com/deeptrouver/AgentCollab (initial commit `509fbd11`).
Earlier statements above about not uploading describe earlier checkpoints.
No downloadable release has been published. Source-check run `41157853136`
failed on Linux mount detection and a desktop build dependency advisory;
neither failure is waived.

The mount parser now distinguishes Linux's `type` field and rw/ro options
from macOS's filesystem-first parentheses, including macOS paths containing
` type `. Regression fixtures cover root mounts, CIFS, sibling boundaries,
and preservation of the macOS path. Local storage/doctor tests pass; Linux
runtime confirmation must come from the subsequent CI run.

公开源码不等于安装包已验收。此次修正 Linux 挂载格式识别，并保留 macOS
特殊目录名行为；本机存储和诊断测试通过，仍需远程 Linux CI 确认。
桌面打包依赖安全审计尚未解决，不发布安装包，也不关闭审计绕过问题。

The follow-up run `41158549587` passed its Linux `test` job, including Go
tests, vet, the built Hub community smoke and public-source audit. Desktop
dependency audit jobs still fail. Linux mount runtime acceptance is now
confirmed; the overall workflow is not green.

Dependency remediation in progress: both desktop manifests constrain
minimatch versions >=9 to 11.0.2, preserving legacy function-style callers.
The updated Client builds on macOS arm64 and its isolated packaged smoke
passes invitation, login, connection, clean exit and restored account/device
connection after reopening. All 23 desktop unit tests and dependency/runtime
notice checks pass. This is build-tool compatibility evidence, not a completed
security exception or a new released binary: bundled native binaries still
carry the previous source snapshot and must be rebuilt for the final release.

Linux 主测试任务已通过。现代接口的依赖升级已通过本机实际打包与重新打开接入
测试；旧版 minimatch 的安全告警仍保留，尚未批准豁免，也未发布安装包。

The same updated build dependencies also produced a macOS arm64 Server that
passed its isolated packaged acceptance: first-run configuration, healthy Hub,
restart preserving the team, English/Chinese switch, and clean exit. A two-case
lockfile regression test now guards both desktop dependency resolutions. See
`minimatch-advisory-review.md` for the outstanding legacy-source review; no
security gate has been weakened by this partial mitigation.
