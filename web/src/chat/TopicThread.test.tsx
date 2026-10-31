import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { Agent, Message, Task, User } from "../api/client";
import { TopicThread } from "./TopicThread";
import { setLocale } from "../i18n/locale";

afterEach(() => setLocale("zh-CN"));

test("English topic controls preserve original messages and reply dispatch", async () => {
  setLocale("en");
  const onReplyTopic = vi.fn();
  render(<TopicThread topic={{ root, replies: [reply], tasks: [task] }} agents={[agent]} members={[owner]} currentUserID={owner.id} onReplyTopic={onReplyTopic} />);
  expect(screen.getByText(root.body)).toBeInTheDocument();
  expect(screen.getByText("1/1 returned to topic")).toBeInTheDocument();
  await userEvent.setup().click(screen.getByText("1 topic replies"));
  expect(screen.getByText("项目状态正常")).toBeInTheDocument();
  await userEvent.setup().click(screen.getByRole("button", { name: "Reply to topic" }));
  expect(onReplyTopic).toHaveBeenCalledWith(root);
});

const owner: User = { id: "user-1", username: "owner", display_name: "Owner", role: "admin" };
const agent: Agent = {
  id: "agent-1", display_name: "Codex Local", type: "codex", owner_user_id: owner.id, owner_name: owner.display_name,
  device_id: "device-1", device_name: "MacBook", os: "darwin", arch: "arm64", channel: "runner/darwin", permission_mode: "admin", status: "online",
  imported: true, available: true, version: "codex", executable: "codex", native_history: "Codex", response_mode: "mention", reply_delay_seconds: 0, role_prompt: "", max_auto_turns: 4,
};
const root: Message = { id: "message-root", workspace_id: "workspace-1", room_id: "room-1", sequence: 1, sender_kind: "user", sender_id: owner.id, kind: "chat", body: "请检查本地项目状态", created_at: "2026-07-25T03:00:00Z" };
const task: Task = { id: "task-1", workspace_id: "workspace-1", room_id: "room-1", root_message_id: root.id, source_message_id: root.id, collaboration_depth: 0, creator_kind: "user", creator_id: owner.id, target_agent_id: agent.id, title: "群聊点名：Codex Local", prompt: root.body, acceptance_criteria: "返回结论", state: "delivered", current_attempt: 1, created_at: root.created_at, updated_at: "2026-07-25T03:00:10Z" };
const reply: Message = { id: "message-reply", workspace_id: "workspace-1", room_id: "room-1", sequence: 2, sender_kind: "agent", sender_id: agent.id, kind: "delivery", body: "交付：群聊点名：Codex Local\n\n项目状态正常", root_message_id: root.id, reply_to_message_id: root.id, metadata: { task_id: task.id, task_state: "delivered" }, created_at: task.updated_at };

test("Agent 回复默认折叠在真人委托下，展开后不显示系统生成标题", () => {
  const onReplyTopic = vi.fn();
  render(<TopicThread topic={{ root, replies: [reply], tasks: [task] }} agents={[agent]} members={[owner]} currentUserID={owner.id} onReplyTopic={onReplyTopic} />);

  expect(screen.getByText("请检查本地项目状态")).toBeInTheDocument();
  const summary = screen.getByText("1 条话题回复");
  expect(summary.closest("details")).not.toHaveAttribute("open");

  fireEvent.click(summary);
  expect(summary.closest("details")).toHaveAttribute("open");
  expect(screen.getByText("项目状态正常")).toBeInTheDocument();
  expect(screen.queryByText("群聊点名：Codex Local")).not.toBeInTheDocument();

  fireEvent.click(screen.getByRole("button", { name: "回复话题" }));
  expect(onReplyTopic).toHaveBeenCalledWith(root);
});

test("委托人可从话题内停止尚未结束的 Agent 工作", async () => {
  const user = userEvent.setup();
  const onCancelTopic = vi.fn().mockResolvedValue(undefined);
  render(<TopicThread topic={{ root, replies: [], tasks: [{ ...task, state: "running" }] }} agents={[agent]} members={[owner]} currentUserID={owner.id} onCancelTopic={onCancelTopic} />);

  await user.click(screen.getByText("1 项委托"));
  await user.click(screen.getByRole("button", { name: "停止本次委托" }));

  expect(onCancelTopic).toHaveBeenCalledWith(root.id);
});

test("失败回复不会显示成全部已回群", () => {
  const failedTask: Task = { ...task, state: "failed" };
  const failure: Message = {
    ...reply,
    body: "任务失败：群聊点名：Codex Local\n\n本机尚未登录。",
    metadata: { task_id: failedTask.id, task_state: "failed" },
  };
  render(<TopicThread topic={{ root, replies: [failure], tasks: [failedTask] }} agents={[agent]} members={[owner]} currentUserID={owner.id} />);
  expect(screen.getByText("0/1 成功交付 · 1 项未完成")).toBeInTheDocument();
});

test("关联计划的任务可从话题内选择目录提交审查", async () => {
  const user = userEvent.setup();
  const onSubmitWorkspace = vi.fn().mockResolvedValue(undefined);
  const item = { id: "work-1", workspace_id: "workspace-1", room_id: "room-1", root_message_id: root.id, plan_step_id: "step-1", git_branch: "issue/message-root", merge_status: "open" as const, kind: "task" as const, title: root.body, description: root.body, status: "in_progress" as const, priority: "normal" as const, task_count: 1, active_task_count: 0, delivered_task_count: 1, failed_task_count: 0, target_agent_ids: [agent.id], created_at: root.created_at, updated_at: task.updated_at };
  const { container } = render(<TopicThread topic={{ root, replies: [reply], tasks: [task] }} agents={[agent]} members={[owner]} currentUserID={owner.id} workItem={item} onSubmitWorkspace={onSubmitWorkspace} />);
  await user.click(screen.getByText("1 条话题回复"));
  const input = container.querySelector('input[type="file"]') as HTMLInputElement;
  const file = new File(["ok"], "result.md", { type: "text/markdown" });
  fireEvent.change(input, { target: { files: [file] } });
  expect(onSubmitWorkspace).toHaveBeenCalledWith(item.id, [file]);
});

test("待验收任务在原话题提示到看板发起 AI 审查", async () => {
  const onOpenBoard = vi.fn();
  const item = { id: "work-1", workspace_id: "workspace-1", room_id: "room-1", root_message_id: root.id, plan_step_id: "step-1", git_branch: "issue/message-root", merge_status: "open" as const, kind: "task" as const, title: root.body, description: root.body, status: "review" as const, priority: "normal" as const, task_count: 1, active_task_count: 0, delivered_task_count: 1, failed_task_count: 0, target_agent_ids: [agent.id], created_at: root.created_at, updated_at: task.updated_at };
  render(<TopicThread topic={{ root, replies: [reply], tasks: [task] }} agents={[agent]} members={[owner]} currentUserID={owner.id} workItem={item} stepTitle="组合构建" highlighted onOpenBoard={onOpenBoard} />);

  expect(screen.getByLabelText("该话题的项目关联")).toHaveTextContent("归属步骤组合构建待真人验收");
  expect(screen.getByLabelText(`话题：${root.body}`)).toHaveClass("is-linked-focus");
  await userEvent.setup().click(screen.getByRole("button", { name: "在看板查看" }));
  expect(onOpenBoard).toHaveBeenCalledTimes(1);
  await userEvent.setup().click(screen.getByText("1 条话题回复"));
  expect(screen.getByText("请到“看板”指定 AI 审查")).toBeInTheDocument();
});

test("纯文字交付显示验收通过、退回修改和补充交付文件", async () => {
  const user = userEvent.setup();
  const onUpdateWorkItem = vi.fn().mockResolvedValue(undefined);
  const onAddDeliveryFiles = vi.fn().mockResolvedValue(undefined);
  const item = { id: "work-text", workspace_id: "workspace-1", room_id: "room-1", root_message_id: root.id, plan_step_id: "step-1", merge_status: "none" as const, kind: "task" as const, title: root.body, description: root.body, status: "review" as const, priority: "normal" as const, task_count: 1, active_task_count: 0, delivered_task_count: 1, failed_task_count: 0, target_agent_ids: [agent.id], created_at: root.created_at, updated_at: task.updated_at };
  const { container } = render(<TopicThread topic={{ root, replies: [reply], tasks: [task] }} agents={[agent]} members={[owner]} currentUserID={owner.id} workItem={item} onUpdateWorkItem={onUpdateWorkItem} onAddDeliveryFiles={onAddDeliveryFiles} />);

  await user.click(screen.getByText("1 条话题回复"));
  expect(screen.getByText("文字与文件交付待你验收")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "验收通过" }));
  expect(onUpdateWorkItem).toHaveBeenCalledWith(item.id, "done");

  const inputs = container.querySelectorAll('input[type="file"]');
  const deliveryInput = inputs[inputs.length - 1] as HTMLInputElement;
  const report = new File(["report"], "report.md", { type: "text/markdown" });
  fireEvent.change(deliveryInput, { target: { files: [report] } });
  expect(onAddDeliveryFiles).toHaveBeenCalledWith(root.id, [report]);
});

test("未归入项目步骤的明确任务跳回话题后也能验收", async () => {
  const user = userEvent.setup();
  const onUpdateWorkItem = vi.fn().mockResolvedValue(undefined);
  const structuredRoot: Message = { ...root, metadata: { task_source: "structured", task_title: "独立研究任务" } };
  const item = { id: "work-standalone", workspace_id: "workspace-1", room_id: "room-1", root_message_id: root.id, merge_status: "none" as const, kind: "task" as const, title: "独立研究任务", description: structuredRoot.body, status: "review" as const, priority: "normal" as const, task_count: 1, active_task_count: 0, delivered_task_count: 1, failed_task_count: 0, target_agent_ids: [agent.id], created_at: root.created_at, updated_at: task.updated_at };

  render(<TopicThread topic={{ root: structuredRoot, replies: [reply], tasks: [task] }} agents={[agent]} members={[owner]} currentUserID={owner.id} workItem={item} onUpdateWorkItem={onUpdateWorkItem} />);
  await user.click(screen.getByText("1 条话题回复"));
  await user.click(screen.getByRole("button", { name: "验收通过" }));
  expect(onUpdateWorkItem).toHaveBeenCalledWith(item.id, "done");
});

test("普通点名回复留在聊天中但不伪装成待验收任务", async () => {
  const user = userEvent.setup();
  const onUpdateWorkItem = vi.fn().mockResolvedValue(undefined);
  const item = { id: "work-chat", workspace_id: "workspace-1", room_id: "room-1", root_message_id: root.id, merge_status: "none" as const, kind: "task" as const, title: root.body, description: root.body, status: "review" as const, priority: "normal" as const, task_count: 1, active_task_count: 0, delivered_task_count: 1, failed_task_count: 0, target_agent_ids: [agent.id], created_at: root.created_at, updated_at: task.updated_at };

  render(<TopicThread topic={{ root, replies: [reply], tasks: [task] }} agents={[agent]} members={[owner]} currentUserID={owner.id} workItem={item} onUpdateWorkItem={onUpdateWorkItem} />);
  await user.click(screen.getByText("1 条话题回复"));
  expect(screen.queryByRole("button", { name: "验收通过" })).not.toBeInTheDocument();
  expect(screen.getByText("这是普通会话，不需要项目验收")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "标记已处理" }));
  expect(onUpdateWorkItem).toHaveBeenCalledWith(item.id, "cancelled");
});

test("从总控台跳回原话题时自动展开验收区域", () => {
  const structuredRoot: Message = { ...root, metadata: { task_source: "structured", task_title: "独立研究任务" } };
  const item = { id: "work-highlighted", workspace_id: "workspace-1", room_id: "room-1", root_message_id: root.id, merge_status: "none" as const, kind: "task" as const, title: "独立研究任务", description: structuredRoot.body, status: "review" as const, priority: "normal" as const, task_count: 1, active_task_count: 0, delivered_task_count: 1, failed_task_count: 0, target_agent_ids: [agent.id], created_at: root.created_at, updated_at: task.updated_at };

  render(<TopicThread highlighted topic={{ root: structuredRoot, replies: [reply], tasks: [task] }} agents={[agent]} members={[owner]} currentUserID={owner.id} workItem={item} onUpdateWorkItem={vi.fn()} />);
  expect(screen.getByRole("button", { name: "验收通过" })).toBeVisible();
  expect(screen.getByRole("button", { name: "退回修改" })).toBeVisible();
});
