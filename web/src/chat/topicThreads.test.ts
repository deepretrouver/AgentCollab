import type { Message, Task } from "../api/client";
import { conversationTimeline } from "./topicThreads";

const root: Message = {
  id: "message-root", workspace_id: "workspace-1", room_id: "room-1", sequence: 1,
  sender_kind: "user", sender_id: "user-1", kind: "chat", body: "请评估这份报告", created_at: "2026-07-25T03:00:00Z",
};

const reply: Message = {
  id: "message-reply", workspace_id: "workspace-1", room_id: "room-1", sequence: 2,
  sender_kind: "agent", sender_id: "agent-1", kind: "delivery", body: "交付：群聊点名：Codex\n\n结论",
  root_message_id: root.id, reply_to_message_id: root.id, metadata: { task_id: "task-1", task_state: "delivered" }, created_at: "2026-07-25T03:00:10Z",
};

const task: Task = {
  id: "task-1", workspace_id: "workspace-1", room_id: "room-1", root_message_id: root.id, source_message_id: root.id,
  collaboration_depth: 0, creator_kind: "user", creator_id: "user-1", target_agent_id: "agent-1", title: "群聊点名：Codex",
  prompt: "执行", acceptance_criteria: "交付结论", state: "delivered", current_attempt: 1,
  created_at: root.created_at, updated_at: reply.created_at,
};

test("将同一真人原文、任务与 Agent 交付收进同一话题", () => {
  const timeline = conversationTimeline([root, reply], [task]);
  expect(timeline).toHaveLength(1);
  expect(timeline[0]).toMatchObject({ kind: "topic", topic: { root, replies: [reply], tasks: [task] } });
});

test("没有根关联的普通群聊消息保持在时间线中", () => {
  const ordinary: Message = { ...root, id: "message-ordinary", sequence: 3, body: "普通讨论" };
  const timeline = conversationTimeline([root, ordinary, reply], [task]);
  expect(timeline).toHaveLength(2);
  expect(timeline[1]).toMatchObject({ kind: "message", message: ordinary });
});

test("已归入项目步骤的真人消息即使没有 Agent 回复也成为可追溯话题", () => {
  const timeline = conversationTimeline([root], [], [root.id]);
  expect(timeline).toEqual([{ kind: "topic", topic: { root, replies: [], tasks: [] } }]);
});
