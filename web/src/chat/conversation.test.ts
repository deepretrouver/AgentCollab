import type { Message, Task } from "../api/client";
import { conversationTasks, conversationWindow, latestTaskBatch, parseBrainstormCommand, parseConversationCommand, visibleConversationMessages } from "./conversation";

function message(sequence: number, body: string): Message {
  return { id: `message-${sequence}`, workspace_id: "workspace-1", room_id: "room-1", sequence, sender_kind: "user", sender_id: "user-1", kind: "chat", body, created_at: `2026-07-16T03:00:0${sequence}Z` };
}

function task(id: string, state: Task["state"], createdAt: string): Task {
  return { id, workspace_id: "workspace-1", room_id: "room-1", collaboration_depth: 0, creator_kind: "user", creator_id: "user-1", target_agent_id: `agent-${id}`, title: `任务 ${id}`, prompt: "执行", acceptance_criteria: "完成", state, current_attempt: 1, created_at: createdAt, updated_at: new Date().toISOString() };
}

test("识别会话命令并将 reset 归一为 restart", () => {
  expect(parseConversationCommand("/NEW 新主题")).toMatchObject({ name: "new", argument: "新主题" });
  expect(parseConversationCommand("/reset")).toMatchObject({ name: "restart" });
  expect(parseConversationCommand("/clear now")).toBeUndefined();
});

test("识别带议题的群聊级专题讨论命令", () => {
  expect(parseBrainstormCommand("/brainstorm 评估股票技能")).toEqual({ topic: "评估股票技能", raw: "/brainstorm 评估股票技能" });
  expect(parseBrainstormCommand("/brainstorm")).toBeUndefined();
  expect(parseConversationCommand("/brainstorm 评估股票技能")).toBeUndefined();
});

test("最近一次命令切断旧消息上下文", () => {
  const result = conversationWindow([message(1, "旧消息"), message(2, "/clear"), message(3, "新消息")]);
  expect(result.previous.map((item) => item.body)).toEqual(["旧消息"]);
  expect(result.current.map((item) => item.body)).toEqual(["新消息"]);
});

test("同一时刻派发的多个 Agent 任务组成一个进度批次", () => {
  const tasks = [task("1", "running", "2026-07-16T03:00:00.000Z"), task("2", "queued", "2026-07-16T03:00:00.500Z")];
  expect(latestTaskBatch(tasks)).toHaveLength(2);
});

test("任务工作台与 Agent 上下文一样遵守最近一次会话边界", () => {
  const boundary = message(2, "/clear");
  const tasks = [task("old", "delivered", "2026-07-16T03:00:01.000Z"), task("current", "running", "2026-07-16T03:00:03.000Z")];
  expect(conversationTasks(tasks, boundary).map((item) => item.id)).toEqual(["current"]);
});

test("旧任务记录恢复一条用户原文并对多 Agent prompt 去重", () => {
  const prompt = "任务：群聊点名\n\n你在 AgentCollab 协作群中被点名。\n\n当前消息：\n@Codex Local @Hermes Chengong 请分别介绍自己\n\n请在群聊中直接给出你的工作结果；";
  const first = { ...message(1, prompt), kind: "task" as const };
  const second = { ...message(2, prompt), kind: "task" as const };
  expect(visibleConversationMessages([first, second]).map((item) => item.body)).toEqual(["@Codex Local @Hermes Chengong 请分别介绍自己"]);
});

test("旧结构化任务恢复为可作为话题根的真人原文", () => {
  const legacy = { ...message(1, "任务：检查项目\n目标 Agent：agent-1\n\n请检查本地项目状态"), kind: "task" as const };
  const [restored] = visibleConversationMessages([legacy]);
  expect(restored).toMatchObject({ id: legacy.id, kind: "chat", body: "请检查本地项目状态", metadata: { task_title: "检查项目", task_source: "legacy" } });
});

test("结构化任务正文即使像命令也不会切断群聊会话", () => {
  const structured = { ...message(1, "/clear"), metadata: { task_source: "structured" as const } };
  expect(conversationWindow([structured]).current).toEqual([structured]);
  expect(visibleConversationMessages([structured])).toEqual([structured]);
});
