import type { Message, Task, WorkItem } from "../api/client";
import { activeProjectItemCount, formalDeliveryItems, formalProjectItems, formalProjectTasks } from "./workItemPresentation";

const base: WorkItem = {
  id: "work-1", workspace_id: "workspace-1", room_id: "room-1", root_message_id: "message-1",
  kind: "task", title: "事项", description: "事项", status: "review", priority: "normal", merge_status: "none",
  task_count: 1, active_task_count: 0, delivered_task_count: 1, failed_task_count: 0,
  target_agent_ids: [], created_at: "2026-08-28T08:00:00Z", updated_at: "2026-08-28T08:01:00Z",
};

function root(id: string, metadata?: Message["metadata"]): Message {
  return { id, workspace_id: "workspace-1", room_id: "room-1", sequence: 1, sender_kind: "user", sender_id: "user-1", kind: "chat", body: "原文", metadata, created_at: base.created_at };
}

test("正式交付分类排除普通点名和本地身份验证", () => {
  const project = { ...base, id: "project", root_message_id: "project-message", plan_step_id: "step-1" };
  const structured = { ...base, id: "structured", root_message_id: "structured-message" };
  const ordinary = { ...base, id: "ordinary", root_message_id: "ordinary-message" };
  const verification = { ...base, id: "verification", root_message_id: "verification-message", title: "Agent 本地身份验证" };
  const result = formalDeliveryItems([project, structured, ordinary, verification], [
    root("project-message"),
    root("structured-message", { task_source: "structured", task_title: "正式任务" }),
    root("ordinary-message"),
    root("verification-message", { task_source: "structured", task_title: "Agent 本地身份验证" }),
  ]);
  expect(result.map((item) => item.id)).toEqual(["project", "structured"]);
});

test("看板数字只统计归入计划且未结束的事项", () => {
  expect(activeProjectItemCount([
    { ...base, id: "active", plan_step_id: "step-1" },
    { ...base, id: "done", plan_step_id: "step-1", status: "done" },
    { ...base, id: "chat" },
  ])).toBe(1);
});

test("项目总控台只保留正式事项及其执行记录", () => {
  const formal = { ...base, id: "formal", root_message_id: "formal-root" };
  const ordinary = { ...base, id: "ordinary", root_message_id: "ordinary-root" };
  const verification = { ...base, id: "verification", root_message_id: "verification-root" };
  const messages = [
    root("formal-root", { task_source: "structured", task_title: "正式研究" }),
    root("ordinary-root"),
    root("verification-root", { task_source: "structured", task_title: "Agent 本地身份验证" }),
  ];
  const task = (id: string, workItemID: string): Task => ({ id, work_item_id: workItemID, workspace_id: base.workspace_id, room_id: base.room_id, collaboration_depth: 0, creator_kind: "user", creator_id: "user-1", target_agent_id: "agent-1", title: id, prompt: id, acceptance_criteria: "", state: "failed", current_attempt: 1, created_at: base.created_at, updated_at: base.updated_at });

  expect(formalProjectItems([formal, ordinary, verification], messages).map((item) => item.id)).toEqual(["formal"]);
  expect(formalProjectTasks([task("formal-task", "formal"), task("chat-task", "ordinary"), task("verify-task", "verification")], [formal, ordinary, verification], messages).map((item) => item.id)).toEqual(["formal-task"]);
});
