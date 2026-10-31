import { act, render, screen } from "@testing-library/react";
import { setLocale } from "../i18n/locale";
import userEvent from "@testing-library/user-event";
import type { Agent, Message, ProjectPlan, User } from "../api/client";
import { Composer } from "./Composer";

afterEach(() => act(() => setLocale("zh-CN")));

test("English discussion mode preserves the command and user topic", async () => {
  setLocale("en");
  const user = userEvent.setup();
  const onSend = vi.fn().mockResolvedValue(sentMessage);
  render(<Composer agents={[agent]} tasks={[]} onSend={onSend} onCreateTask={vi.fn()} onCancelTaskRoot={vi.fn()} />);
  expect(screen.getByRole("button", { name: "Direct mention" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Create structured task" })).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Topic discussion" }));
  await user.type(screen.getByRole("textbox", { name: "Group message" }), "Review 数据源");
  await user.click(screen.getByRole("button", { name: "Send message" }));
  expect(onSend).toHaveBeenCalledWith("/brainstorm Review 数据源", [], []);
});

test("English slash menu preserves restart protocol", async () => {
  setLocale("en");
  const user = userEvent.setup();
  const onSend = vi.fn().mockResolvedValue(sentMessage);
  render(<Composer agents={[agent]} tasks={[]} onSend={onSend} onCreateTask={vi.fn()} onCancelTaskRoot={vi.fn()} />);
  await user.click(screen.getByRole("button", { name: "Open commands" }));
  expect(screen.getByRole("listbox", { name: "Conversation commands" })).toBeInTheDocument();
  await user.click(screen.getByRole("option", { name: /Archive the current context/ }));
  await user.click(screen.getByRole("button", { name: "Send message" }));
  expect(onSend).toHaveBeenCalledWith("/restart", [], []);
});

const agent: Agent = {
  id: "agent-1", display_name: "Codex Local", type: "codex", owner_user_id: "user-1", owner_name: "Owner",
  device_id: "device-1", device_name: "MacBook", os: "darwin", arch: "arm64", channel: "runner/darwin", permission_mode: "admin", status: "online",
  imported: true, available: true, version: "codex 0.132.0", executable: "/opt/homebrew/bin/codex", native_history: "Codex session",
  response_mode: "mention", reply_delay_seconds: 0, role_prompt: "", max_auto_turns: 4,
};

const hermes: Agent = {
  ...agent, id: "agent-2", display_name: "Hermes Chengong", type: "hermes", executable: "/opt/homebrew/bin/hermes",
};
const sentMessage: Message = { id: "message-sent", workspace_id: "workspace-1", room_id: "room-1", sequence: 1, sender_kind: "user", sender_id: "user-1", kind: "chat", body: "sent", created_at: "2026-07-27T10:00:00Z" };
const projectPlan: ProjectPlan = { workspace_id: "workspace-1", objective: "研究", status: "active", progress: 0, can_manage: true, steps: [{ id: "step-1", workspace_id: "workspace-1", title: "市场研究", description: "", weight: 100, status: "in_progress", sort_order: 0, progress: 0, linked_item_count: 0, responsible_user_ids: ["user-1"], responsible_agent_ids: [agent.id], local_bindings: [{ step_id: "step-1", user_id: "user-1", runner_id: "runner-1", device_id: "device-1", device_name: "MacBook", directory_label: "market-research", runner_status: "online", created_at: "2026-08-13T08:00:00Z", updated_at: "2026-08-13T08:00:00Z" }], created_at: "2026-08-13T08:00:00Z", updated_at: "2026-08-13T08:00:00Z" }] };

test("从斜杠菜单选择并发送 restart", async () => {
  const user = userEvent.setup();
  const onSend = vi.fn().mockResolvedValue(sentMessage);
  render(<Composer agents={[agent]} tasks={[]} onSend={onSend} onCreateTask={vi.fn()} onCancelTaskRoot={vi.fn()} />);

  await user.click(screen.getByRole("button", { name: "打开会话命令" }));
  expect(screen.getByRole("listbox", { name: "会话命令" })).toBeInTheDocument();
  await user.click(screen.getByRole("option", { name: /restart/ }));
  await user.click(screen.getByRole("button", { name: "发送消息" }));

  expect(onSend).toHaveBeenCalledWith("/restart", [], []);
});

test("从斜杠菜单发起专题讨论", async () => {
  const user = userEvent.setup();
  const onSend = vi.fn().mockResolvedValue(sentMessage);
  render(<Composer agents={[agent, hermes]} tasks={[]} onSend={onSend} onCreateTask={vi.fn()} onCancelTaskRoot={vi.fn()} />);

  await user.click(screen.getByRole("button", { name: "打开会话命令" }));
  await user.click(screen.getByRole("option", { name: /brainstorm/ }));
  await user.type(screen.getByRole("textbox", { name: "群聊消息" }), "评估股票技能");
  await user.click(screen.getByRole("button", { name: "发送消息" }));

  expect(onSend).toHaveBeenCalledWith("/brainstorm 评估股票技能", [], []);
});

test("专题讨论入口由主协调组织，而直接点名保持用户原文", async () => {
  const user = userEvent.setup();
  const onSend = vi.fn().mockResolvedValue(sentMessage);
  render(<Composer agents={[agent, hermes]} tasks={[]} onSend={onSend} onCreateTask={vi.fn()} onCancelTaskRoot={vi.fn()} />);

  expect(screen.getByText("原文交给你点名的 Agent；不经过主协调改写")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "专题讨论" }));
  expect(screen.getByText("由主协调组织成员独立分析，再统一汇总")).toBeInTheDocument();
  await user.type(screen.getByRole("textbox", { name: "群聊消息" }), "评估股票技能");
  await user.click(screen.getByRole("button", { name: "发送消息" }));

  expect(onSend).toHaveBeenCalledWith("/brainstorm 评估股票技能", [], []);
});

test("结构化任务从协作方式入口打开", async () => {
  const user = userEvent.setup();
  render(<Composer agents={[agent]} tasks={[]} onSend={vi.fn()} onCreateTask={vi.fn()} onCancelTaskRoot={vi.fn()} />);

  await user.click(screen.getByRole("button", { name: "创建结构化任务" }));
  expect(screen.getByText("交给指定 Agent 执行并回传群聊")).toBeInTheDocument();
});

test("点击 Agent 后发送结构化提及", async () => {
  const user = userEvent.setup();
  const onSend = vi.fn().mockResolvedValue(sentMessage);
  render(<Composer agents={[agent]} tasks={[]} onSend={onSend} onCreateTask={vi.fn()} onCancelTaskRoot={vi.fn()} />);

  await user.click(screen.getByRole("button", { name: agent.display_name }));
  await user.type(screen.getByRole("textbox", { name: "群聊消息" }), "请检查当前实现");
  await user.click(screen.getByRole("button", { name: "发送消息" }));

  expect(onSend).toHaveBeenCalledWith(expect.stringContaining(`@${agent.display_name}`), [agent.id], []);
});

test("项目会话明确选择负责人本机最新进展并传给任务路由", async () => {
  const user = userEvent.setup();
  const onSend = vi.fn().mockResolvedValue(sentMessage);
  render(<Composer agents={[agent]} tasks={[]} projectPlan={projectPlan} onSend={onSend} onCreateTask={vi.fn()} onCancelTaskRoot={vi.fn()} />);

  await user.selectOptions(screen.getByLabelText("会话归属项目步骤"), "step-1");
  expect(screen.getByLabelText("Agent 执行依据")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: agent.display_name }));
  await user.selectOptions(screen.getByLabelText("Agent 执行依据"), "responsible_local");
  await user.type(screen.getByRole("textbox", { name: "群聊消息" }), "汇报尚未提交的最新进展");
  await user.click(screen.getByRole("button", { name: "发送消息" }));

  expect(onSend).toHaveBeenCalledWith(expect.stringContaining("最新进展"), [agent.id], [], undefined, "step-1", "responsible_local");
});

test("项目会话把公共底稿说明为群内共享只读副本", async () => {
  const planWithDraft: ProjectPlan = {
    ...projectPlan,
    steps: [{ ...projectPlan.steps[0], workspace: {
      step_id: "step-1", source_name: "研究底稿", file_count: 12, total_bytes: 2048,
      revision: 3, last_commit: "abc123", created_at: "2026-08-15T08:00:00Z", updated_at: "2026-08-15T08:00:00Z",
    } }],
  };
  render(<Composer agents={[agent]} tasks={[]} projectPlan={planWithDraft} onSend={vi.fn()} onCreateTask={vi.fn()} onCancelTaskRoot={vi.fn()} />);

  await userEvent.setup().selectOptions(screen.getByLabelText("会话归属项目步骤"), "step-1");
  expect(screen.getByRole("option", { name: "公共底稿 v3 · 群内共享只读" })).toBeInTheDocument();
  expect(screen.getByText(/聊天任务只使用临时副本，不能覆盖公共底稿/)).toBeInTheDocument();
  expect(screen.queryByText("项目管理员")).not.toBeInTheDocument();
  expect(screen.queryByText("步骤负责人")).not.toBeInTheDocument();
});

test("点名条按真人所有者组织 Agent；单 Agent 直接展示，多 Agent 按需展开", async () => {
  const user = userEvent.setup();
  const owner: User = { id: "user-1", username: "owner", display_name: "郭昱科", role: "admin" };
  const teammate: User = { id: "user-2", username: "alice", display_name: "Alice", role: "member" };
  const teammateAgent: Agent = { ...hermes, id: "agent-3", owner_user_id: "user-2", owner_name: "Alice", display_name: "Alice Hermes" };
  render(<Composer agents={[agent, hermes, teammateAgent]} tasks={[]} currentUser={owner} members={[owner, teammate]} onSend={vi.fn()} onCreateTask={vi.fn()} onCancelTaskRoot={vi.fn()} />);

  expect(screen.getByRole("button", { name: "选择 郭昱科 名下 2 个 Agent" })).toHaveAttribute("aria-expanded", "false");
  expect(screen.queryByRole("button", { name: "Codex Local" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Alice Hermes" })).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "选择 郭昱科 名下 2 个 Agent" }));
  expect(screen.getByRole("button", { name: "Codex Local" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Hermes Chengong" })).toBeInTheDocument();
});

test("点击全体后向所有在线 Agent 发送结构化提及", async () => {
  const user = userEvent.setup();
  const onSend = vi.fn().mockResolvedValue(sentMessage);
  render(<Composer agents={[agent, hermes]} tasks={[]} onSend={onSend} onCreateTask={vi.fn()} onCancelTaskRoot={vi.fn()} />);

  await user.click(screen.getByRole("button", { name: "全体" }));
  await user.type(screen.getByRole("textbox", { name: "群聊消息" }), "每人一句介绍");
  await user.click(screen.getByRole("button", { name: "发送消息" }));

  expect(onSend).toHaveBeenCalledWith("@全体 每人一句介绍", [agent.id, hermes.id], []);
});

test("可先点名离线 Agent，任务排队并在设备上线后执行", async () => {
	const user = userEvent.setup();
	const onSend = vi.fn().mockResolvedValue(sentMessage);
	const offline = { ...agent, status: "offline" as const };
	render(<Composer agents={[offline]} tasks={[]} onSend={onSend} onCreateTask={vi.fn()} onCancelTaskRoot={vi.fn()} />);

	await user.click(screen.getByRole("button", { name: "Codex Local · 离线" }));
	await user.type(screen.getByRole("textbox", { name: "群聊消息" }), "回办公室上线后检查项目");
	await user.click(screen.getByRole("button", { name: "发送消息" }));

	expect(onSend).toHaveBeenCalledWith(expect.stringContaining("回办公室上线后检查项目"), [offline.id], []);
});

test("结构化任务可选择离线 Agent 并显示等待上线", async () => {
	const user = userEvent.setup();
	const offline = { ...agent, status: "offline" as const };
	render(<Composer agents={[offline]} tasks={[]} onSend={vi.fn()} onCreateTask={vi.fn()} onCancelTaskRoot={vi.fn()} />);

	await user.click(screen.getByRole("button", { name: "创建结构化任务" }));
	expect(screen.getByRole("option", { name: /离线，等待上线/ })).toBeInTheDocument();
	expect(screen.getByRole("button", { name: "创建并等待上线" })).toBeInTheDocument();
});

test("派发 Agent 委托后发送按钮变为停止，并取消本次委托", async () => {
  const user = userEvent.setup();
  const onSend = vi.fn().mockResolvedValue(sentMessage);
  const onCancelTaskRoot = vi.fn().mockResolvedValue(undefined);
  render(<Composer agents={[agent]} tasks={[]} onSend={onSend} onCreateTask={vi.fn()} onCancelTaskRoot={onCancelTaskRoot} />);

  await user.click(screen.getByRole("button", { name: agent.display_name }));
  await user.type(screen.getByRole("textbox", { name: "群聊消息" }), "请做长任务");
  await user.click(screen.getByRole("button", { name: "发送消息" }));
  await user.click(screen.getByRole("button", { name: "停止本次委托" }));

  expect(onCancelTaskRoot).toHaveBeenCalledWith(sentMessage.id);
});

test("附件与点名 Agent 在同一条消息发送", async () => {
  const user = userEvent.setup();
  const onSend = vi.fn().mockResolvedValue(sentMessage);
  render(<Composer agents={[hermes]} tasks={[]} onSend={onSend} onCreateTask={vi.fn()} onCancelTaskRoot={vi.fn()} />);
  const report = new File(["investment report"], "授信报告.pdf", { type: "application/pdf" });

  await user.upload(screen.getByLabelText("添加会话附件"), report);
  expect(screen.getByRole("region", { name: "待发送附件" })).toHaveTextContent("授信报告.pdf");
  await user.click(screen.getByRole("button", { name: hermes.display_name }));
  await user.type(screen.getByRole("textbox", { name: "群聊消息" }), "请调用投委会审查 Skill 审查这份报告");
  await user.click(screen.getByRole("button", { name: "发送消息" }));

  expect(onSend).toHaveBeenCalledWith(expect.stringContaining("投委会审查"), [hermes.id], [report]);
});

test("回复话题时将真人消息绑定到原始话题根", async () => {
  const user = userEvent.setup();
  const onSend = vi.fn().mockResolvedValue({ ...sentMessage, root_message_id: "message-root" });
  const onClearReplyTopic = vi.fn();
  render(<Composer agents={[agent]} tasks={[]} onSend={onSend} onCreateTask={vi.fn()} onCancelTaskRoot={vi.fn()} replyTopic={{ rootMessageID: "message-root", senderName: "YKK", preview: "请评估这份报告" }} onClearReplyTopic={onClearReplyTopic} />);

  expect(screen.getByText("回复话题")).toBeInTheDocument();
  await user.type(screen.getByRole("textbox", { name: "群聊消息" }), "我补充一下风险点");
  await user.click(screen.getByRole("button", { name: "发送消息" }));

  expect(onSend).toHaveBeenCalledWith("我补充一下风险点", [], [], { reply_to_message_id: "message-root", root_message_id: "message-root" });
  expect(onClearReplyTopic).toHaveBeenCalledTimes(1);
});
