import type { Message, Task } from "../api/client";

export type ConversationCommandName = "new" | "clear" | "restart";

export interface ConversationCommand {
  name: ConversationCommandName;
  argument: string;
  raw: string;
}

export const conversationCommands = [
  { name: "brainstorm", usage: "/brainstorm [议题]", description: "发起一次主协调组织的专题讨论" },
  { name: "new", usage: "/new [主题]", description: "在当前群聊中开启一个新的会话段" },
  { name: "restart", usage: "/restart", description: "归档当前上下文并重新开始；/reset 是别名" },
  { name: "clear", usage: "/clear", description: "清理当前视图，并从这里切断后续 Agent 上下文" },
] as const;

export interface BrainstormCommand {
  topic: string;
  raw: string;
}

export function parseBrainstormCommand(input: string): BrainstormCommand | undefined {
  const trimmed = input.trim();
  const match = trimmed.match(/^\/brainstorm(?:\s+(.+))?$/i);
  if (!match?.[1]?.trim()) return undefined;
  return { topic: match[1].trim(), raw: trimmed };
}

export function parseConversationCommand(input: string): ConversationCommand | undefined {
  const trimmed = input.trim();
  const match = trimmed.match(/^\/(new|clear|restart|reset)(?:\s+(.+))?$/i);
  if (!match) return undefined;
  const matchedName = match[1].toLocaleLowerCase();
  if (matchedName !== "new" && match[2]?.trim()) return undefined;
  const name: ConversationCommandName = matchedName === "reset" ? "restart" : matchedName as ConversationCommandName;
  return { name, argument: match[2]?.trim() ?? "", raw: trimmed };
}

export function commandLabel(command: ConversationCommand): string {
  if (command.name === "new") return command.argument ? `新会话：${command.argument}` : "已开启新会话";
  if (command.name === "restart") return "会话已重新开始";
  return "当前视图与 Agent 上下文已清理";
}

export function conversationWindow(messages: Message[]) {
  let boundaryIndex = -1;
  let command: ConversationCommand | undefined;
  messages.forEach((message, index) => {
    if (message.sender_kind !== "user" || message.kind !== "chat") return;
    if (message.metadata?.task_source) return;
    const parsed = parseConversationCommand(message.body);
    if (!parsed) return;
    boundaryIndex = index;
    command = parsed;
  });
  return {
    command,
    boundary: boundaryIndex >= 0 ? messages[boundaryIndex] : undefined,
    previous: boundaryIndex >= 0 ? messages.slice(0, boundaryIndex) : [],
    current: messages.slice(boundaryIndex + 1),
  };
}

function restoredTaskMessage(message: Message): Message | undefined {
  if (message.kind !== "task") return undefined;
  const structured = message.body.match(/^任务：([^\n]+)\n目标 Agent：[^\n]+\n\n([\s\S]+)$/);
  if (structured?.[2]?.trim()) {
    return {
      ...message,
      kind: "chat",
      body: structured[2].trim(),
      metadata: { ...message.metadata, task_title: structured[1].trim(), task_source: "legacy" },
    };
  }
  const match = message.body.match(/(?:^|\n)当前消息：\n([\s\S]*?)\n\n请在群聊中/);
  if (!match?.[1]?.trim()) return undefined;
  return { ...message, kind: "chat", body: match[1].trim(), metadata: { ...message.metadata, task_source: "legacy" } };
}

export function visibleConversationMessages(messages: Message[]): Message[] {
  const savedChatBodies = new Set(messages.filter((message) => message.kind === "chat").map((message) => message.body.trim()));
  const restoredBodies = new Set<string>();
  const visible: Message[] = [];
  for (const message of messages) {
    if (!message.metadata?.task_source && parseConversationCommand(message.body)) continue;
    if (message.kind !== "task") {
      visible.push(message);
      continue;
    }
    const restored = restoredTaskMessage(message);
    if (!restored || savedChatBodies.has(restored.body.trim()) || restoredBodies.has(restored.body.trim())) continue;
    restoredBodies.add(restored.body.trim());
    // Preserve the original message id. Agent deliveries point to it through
    // root_message_id, which is what lets old task records join their topic.
    visible.push(restored);
  }
  return visible;
}

const activeStates = new Set<Task["state"]>(["queued", "dispatched", "running"]);

export function latestTaskBatch(tasks: Task[], boundary?: Message): Task[] {
  const boundaryTime = boundary ? new Date(boundary.created_at).getTime() : Number.NEGATIVE_INFINITY;
  const inConversation = tasks.filter((task) => new Date(task.created_at).getTime() >= boundaryTime);
  if (inConversation.length === 0) return [];
  const newest = Math.max(...inConversation.map((task) => new Date(task.created_at).getTime()));
  const batch = inConversation.filter((task) => newest - new Date(task.created_at).getTime() <= 3_000);
  const recentlyUpdated = Math.max(...batch.map((task) => new Date(task.updated_at).getTime())) > Date.now() - 10 * 60_000;
  return batch.some((task) => activeStates.has(task.state)) || recentlyUpdated ? batch : [];
}

// A conversation boundary must affect the task view as well as the prompt
// context. Otherwise /clear looks like a reset while old work remains mixed
// into the visible task area.
export function conversationTasks(tasks: Task[], boundary?: Message): Task[] {
  const boundaryTime = boundary ? new Date(boundary.created_at).getTime() : Number.NEGATIVE_INFINITY;
  return tasks
    .filter((task) => new Date(task.created_at).getTime() >= boundaryTime)
    .sort((left, right) => new Date(left.created_at).getTime() - new Date(right.created_at).getTime());
}

export function taskIsActive(task: Task): boolean {
  return activeStates.has(task.state);
}
