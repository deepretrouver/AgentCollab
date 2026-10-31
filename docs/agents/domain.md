# Domain documentation / 领域文档约定

Keep one shared vocabulary for the product. The Go Hub, browser frontend and desktop wrappers
are delivery components of the same collaboration system, not separate business domains.
统一维护产品术语；Hub、网页与桌面封装是同一协作系统的交付组件，不另建多套业务模型。

- Server/Hub: shared team service and persisted collaboration data. / 团队服务和共享数据。
- Client/Runner: member-owned local connection and execution. / 成员本机接入与执行。
- Workspace: team/project boundary; room: conversations and selected members. / 工作区为团队或项目边界，群聊承载讨论与选定成员。
- Task delivery is not project acceptance. Agent visibility is not invocation permission. / 任务交付不等于项目验收，Agent 可见不等于可调用。

Use the bilingual user guides for user-facing terminology and keep API values stable during
translation. Record architectural changes beside the relevant existing docs rather than
adding a parallel hierarchy. No new AGENTS.md or CLAUDE.md is introduced by this setup.
用户术语参照双语指南；翻译不改变 API 枚举。架构变化更新对应文档，不重复建立层级。
本次不创建新的 AGENTS.md 或 CLAUDE.md。
