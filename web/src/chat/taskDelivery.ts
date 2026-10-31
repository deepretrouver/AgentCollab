import type { Message, Task } from "../api/client";

export const taskStateText: Record<Task["state"], string> = {
	queued: "等待设备上线或领取任务",
  dispatched: "Runner 已接收",
  running: "原生 Agent 执行中",
  delivered: "已交付到群聊",
  failed: "执行失败",
  blocked: "等待处理",
  cancelled: "已取消",
};

// A task may only be marked visibly delivered by its own receipt. Matching an
// arbitrary later message from the same Agent caused false completions in the
// old task panel.
export function deliveryForTask(task: Task, messages: Message[]) {
  const exact = messages.find((message) => (
    message.sender_id === task.target_agent_id
    && (message.kind === "delivery" || message.kind === "progress")
    && message.metadata?.task_id === task.id
  ));
  if (exact) return exact;

  // Older records do not have task_id metadata. Keep this narrow fallback for
  // historical rooms rather than linking any later Agent reply.
  const legacyPrefix = task.state === "failed" ? `任务失败：${task.title}` : `交付：${task.title}`;
  return messages.find((message) => (
    message.sender_id === task.target_agent_id
    && (message.kind === "delivery" || message.kind === "progress")
    && message.body.startsWith(legacyPrefix)
  ));
}

export function taskStatusText(task: Task, messages: Message[]) {
  if (task.state === "delivered" && !deliveryForTask(task, messages)) return "交付回群同步中";
  return taskStateText[task.state];
}

export function isSystemTaskTitle(title: string): boolean {
  return /^(群聊点名：|Agent 接力：|Agent 回传：)/.test(title);
}

export function taskTopicRootID(task: Task): string | undefined {
  return task.root_message_id ?? task.source_message_id;
}
