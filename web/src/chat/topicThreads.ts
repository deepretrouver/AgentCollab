import type { Message, Task } from "../api/client";
import { taskTopicRootID } from "./taskDelivery";

export interface TopicThreadData {
  root: Message;
  replies: Message[];
  tasks: Task[];
}

export type ConversationTimelineItem =
  | { kind: "message"; message: Message }
  | { kind: "topic"; topic: TopicThreadData };

// The database already gives every routed task and agent response a stable
// root_message_id. Keep that structure in the chat instead of duplicating it
// in a separate task ledger below the conversation.
export function conversationTimeline(messages: Message[], tasks: Task[], projectRootIDs: Iterable<string> = []): ConversationTimelineItem[] {
  const roots = new Map(messages
    .filter((message) => message.sender_kind === "user" && message.kind === "chat")
    .map((message) => [message.id, message]));
  const repliesByRoot = new Map<string, Message[]>();
  const tasksByRoot = new Map<string, Task[]>();
  const projectRoots = new Set(projectRootIDs);

  for (const message of messages) {
    const rootID = message.root_message_id;
    if (!rootID || !roots.has(rootID) || rootID === message.id) continue;
    const replies = repliesByRoot.get(rootID) ?? [];
    replies.push(message);
    repliesByRoot.set(rootID, replies);
  }
  for (const task of tasks) {
    const rootID = taskTopicRootID(task);
    if (!rootID || !roots.has(rootID)) continue;
    const topicTasks = tasksByRoot.get(rootID) ?? [];
    topicTasks.push(task);
    tasksByRoot.set(rootID, topicTasks);
  }

  const childIDs = new Set([...repliesByRoot.values()].flatMap((replies) => replies.map((message) => message.id)));
  return messages.flatMap((message): ConversationTimelineItem[] => {
    if (childIDs.has(message.id)) return [];
    const replies = repliesByRoot.get(message.id) ?? [];
    const topicTasks = tasksByRoot.get(message.id) ?? [];
    if (message.sender_kind === "user" && (replies.length > 0 || topicTasks.length > 0 || projectRoots.has(message.id))) {
      return [{ kind: "topic", topic: { root: message, replies, tasks: topicTasks } }];
    }
    return [{ kind: "message", message }];
  });
}
