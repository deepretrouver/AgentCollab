import type { Message, Task, WorkItem } from "../api/client";

export function rootMessageForWorkItem(item: WorkItem, messages: Message[]) {
  return messages.find((message) => message.id === item.root_message_id);
}

export function isLocalAgentVerification(item: WorkItem, messages: Message[]) {
  const root = rootMessageForWorkItem(item, messages);
  const title = String(root?.metadata?.task_title ?? item.title).trim();
  return title.endsWith("本地身份验证");
}

/**
 * Only work explicitly created as a task, or assigned to a project step, is a
 * formal delivery. A normal @ mention still has execution records for audit,
 * but stays in the chat timeline instead of polluting the delivery center.
 */
export function isFormalDelivery(item: WorkItem, messages: Message[]) {
  if (isLocalAgentVerification(item, messages)) return false;
  if (item.plan_step_id) return true;
  return rootMessageForWorkItem(item, messages)?.metadata?.task_source === "structured";
}

/**
 * The project surfaces show the lifecycle of formal work, not every Agent
 * invocation. Keeping this separate from delivery readiness lets the office
 * show work that is queued or running without pulling ordinary @ replies into
 * the control plane.
 */
export function formalProjectItems(items: WorkItem[], messages: Message[]) {
  return items.filter((item) => isFormalDelivery(item, messages));
}

export function formalProjectTasks(tasks: Task[], items: WorkItem[], messages: Message[]) {
  const formalItemIDs = new Set(formalProjectItems(items, messages).map((item) => item.id));
  return tasks.filter((task) => {
    if (task.work_item_id) return formalItemIDs.has(task.work_item_id);
    const root = messages.find((message) => message.id === task.root_message_id);
    const title = String(root?.metadata?.task_title ?? task.title).trim();
    return root?.metadata?.task_source === "structured" && !title.endsWith("本地身份验证");
  });
}

export function formalDeliveryItems(items: WorkItem[], messages: Message[]) {
  return formalProjectItems(items, messages).filter((item) => item.delivered_task_count > 0 || item.status === "done");
}

export function activeProjectItemCount(items: WorkItem[]) {
  return items.filter((item) => Boolean(item.plan_step_id) && item.status !== "done" && item.status !== "cancelled").length;
}
