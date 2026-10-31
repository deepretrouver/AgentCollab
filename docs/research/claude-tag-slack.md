# Claude Tag × AgentCollab 交互调研

日期：2026-07-16

## 结论

Claude Tag 的核心不是把 Agent 放进一个平铺聊天室，而是把频道作为共享入口、把线程作为独立工作会话、把 checklist 作为执行进度面。AgentCollab 应采用相同的状态边界，但把单一 Claude 扩展为同一父任务下的多个本地 Agent 子会话。

## Claude Tag 的官方行为

- 在频道中 `@Claude` 会保证接单；Claude 已加入的线程可以不重复提及，任何成员都能补充上下文或纠偏。
- 每个线程运行独立 session。长任务先确认接单，再更新 checklist，最后把结果、文件或链接交付回原线程。
- `@Claude !restart` 会归档当前 session 并建立新 session，而不是删除 Slack 历史；命令作用域由所在的线程或频道决定。
- 固定命令还包括 `!help`、`!mute`、`!unmute` 和 `!feedback`。
- 频道中的 Claude 使用组织配置的 Agent Identity、工具与权限；DM 使用个人账号与个人连接。
- Claude Tag 能以环境存在方式选择性响应，但显式 `@Claude` 才是保证响应的动作。

## AgentCollab 当前落地

- `/new [主题]`：在当前群聊开启新的会话段。
- `/restart`：归档当前注入上下文并重新开始；`/reset` 为别名。
- `/clear`：清理当前视图并建立上下文边界。旧消息保留，可手动展开审计。
- 多 Agent 同时点名后，Hub 返回同一批任务的真实状态：等待 Runner、已领取、执行中、已完成、失败或阻塞。
- Runner 当前串行执行同一设备上的任务，因此界面必须显示排队状态，不能让用户把等待误判为漏回。

## 下一阶段

1. 为每次群聊派发创建父线程，多个 Agent 任务成为该线程下的子会话。
2. 允许任何群成员在线程中继续追问或纠偏，不必重复点名。
3. 把进度从状态列表升级为 Agent 可更新的 checklist，并保留工具调用和交付物链接。
4. 增加 `/help`、线程级 `/mute`、`/unmute`，以及管理员可审计的 session 归档。

## 官方来源

- [What is Claude Tag?](https://support.claude.com/en/articles/15594475-what-is-claude-tag)
- [Work with Claude Tag](https://claude.com/docs/claude-tag/overview)
- [How Claude Tag works](https://claude.com/docs/claude-tag/concepts/how-it-works)
- [Commands Claude Tag understands](https://claude.com/docs/claude-tag/users/commands)
- [Control when Claude Tag responds](https://claude.com/docs/claude-tag/users/when-claude-responds)
- [Use Claude in Slack](https://support.claude.com/en/articles/12461605-use-claude-in-slack)
