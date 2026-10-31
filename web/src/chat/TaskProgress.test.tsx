import { render, screen } from "@testing-library/react";
import type { Agent, Message, Task } from "../api/client";
import { TaskProgress } from "./TaskProgress";

const agent: Agent = {
  id: "agent-1", display_name: "Codex Local", type: "codex", owner_user_id: "user-1", owner_name: "Owner",
  device_id: "device-1", device_name: "MacBook", os: "darwin", arch: "arm64", channel: "runner/darwin", permission_mode: "admin", status: "online",
  imported: true, available: true, version: "codex 0.132.0", executable: "/opt/homebrew/bin/codex", native_history: "Codex session",
  response_mode: "mention", reply_delay_seconds: 0, role_prompt: "", max_auto_turns: 4,
};
const task: Task = {
  id: "task-1", workspace_id: "workspace-1", room_id: "room-1", collaboration_depth: 0, creator_kind: "user", creator_id: "user-1", target_agent_id: agent.id,
  title: "群聊点名：Codex Local", prompt: "执行", acceptance_criteria: "回复", state: "delivered", current_attempt: 1,
  created_at: "2026-07-16T03:00:00Z", updated_at: "2026-07-16T03:00:20Z",
};
const delivery: Message = {
  id: "message-1", workspace_id: "workspace-1", room_id: "room-1", sequence: 2, sender_kind: "agent", sender_id: agent.id,
  kind: "delivery", body: "交付：群聊点名：Codex Local\n\n回复", metadata: { task_id: task.id, task_state: "delivered" }, created_at: "2026-07-16T03:00:20Z",
};

test("任务已交付但消息未同步时不提前显示完成", () => {
  const { rerender } = render(<TaskProgress tasks={[task]} agents={[agent]} messages={[]} />);
  expect(screen.getByText("交付回群同步中")).toBeInTheDocument();
  expect(screen.getByText("0/1 已回群")).toBeInTheDocument();

  rerender(<TaskProgress tasks={[task]} agents={[agent]} messages={[delivery]} />);
  expect(screen.getByText("已交付到群聊")).toBeInTheDocument();
  expect(screen.getByText("1/1 已回群")).toBeInTheDocument();
});

test("不会把同一 Agent 的无关回复误判为该任务的交付", () => {
  const unrelated: Message = { ...delivery, id: "message-other", body: "交付：另一个任务\n\n无关结果", metadata: { task_id: "task-other", task_state: "delivered" } };
  render(<TaskProgress tasks={[task]} agents={[agent]} messages={[unrelated]} />);
  expect(screen.getByText("交付回群同步中")).toBeInTheDocument();
});

test("失败消息不计入成功交付", () => {
  const failedTask: Task = { ...task, state: "failed" };
  const failure: Message = {
    ...delivery,
    body: "任务失败：群聊点名：Codex Local\n\n本机尚未登录。",
    metadata: { task_id: failedTask.id, task_state: "failed" },
  };
  render(<TaskProgress tasks={[failedTask]} agents={[agent]} messages={[failure]} />);
  expect(screen.getByText("0/1 成功交付 · 1 项未完成")).toBeInTheDocument();
});
