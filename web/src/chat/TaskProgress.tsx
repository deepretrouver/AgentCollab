import { CaretDown, Check, Clock, FileText, Robot, SpinnerGap, WarningCircle } from "@phosphor-icons/react";
import type { Agent, Message, Task } from "../api/client";
import { deliveryForTask, taskStateText } from "./taskDelivery";

interface TaskProgressProps {
  tasks: Task[];
  agents: Agent[];
  messages: Message[];
}

function StateIcon({ state, syncing }: { state: Task["state"]; syncing: boolean }) {
  if (syncing) return <SpinnerGap className="task-state-spinner" size={15} weight="bold" />;
  if (state === "delivered") return <Check size={15} weight="bold" />;
  if (state === "failed" || state === "blocked" || state === "cancelled") return <WarningCircle size={15} weight="fill" />;
  if (state === "running" || state === "dispatched") return <SpinnerGap className="task-state-spinner" size={15} weight="bold" />;
  return <Clock size={15} weight="bold" />;
}

function taskRequest(task: Task, messages: Message[]) {
  const source = task.source_message_id ? messages.find((message) => message.id === task.source_message_id) : undefined;
  if (!source) return "由群聊点名生成。任务上下文仅在目标 Agent 的本机执行时使用。";
  if (source.kind === "chat") return source.body;

  // User-created structured tasks store a private task event whose final
  // paragraph is still the user's original requirement. Never expose an
  // orchestration prompt from an automatic @ mention here.
  const structured = source.body.match(/^任务：[^\n]+\n目标 Agent：[^\n]+\n\n([\s\S]+)$/);
  return structured?.[1]?.trim() || "由群聊点名生成。任务上下文仅在目标 Agent 的本机执行时使用。";
}

function taskAttachments(task: Task, messages: Message[]) {
  const source = task.source_message_id ? messages.find((message) => message.id === task.source_message_id) : undefined;
  return source?.attachments ?? [];
}

function statusText(task: Task, hasVisibleDelivery: boolean) {
  if (task.state === "delivered" && !hasVisibleDelivery) return "交付回群同步中";
  return taskStateText[task.state];
}

export function TaskProgress({ tasks, agents, messages }: TaskProgressProps) {
  if (tasks.length === 0) return null;
  const agentByID = new Map(agents.map((agent) => [agent.id, agent]));
  const completed = tasks.filter((task) => task.state === "delivered" && Boolean(deliveryForTask(task, messages))).length;
  const active = tasks.filter((task) => task.state === "queued" || task.state === "dispatched" || task.state === "running").length;
  const failed = tasks.filter((task) => task.state === "failed" || task.state === "blocked" || task.state === "cancelled").length;
  const summary = active > 0
    ? `${active} 项进行中${failed > 0 ? ` · ${failed} 项未完成` : ""}`
    : failed > 0
      ? `${completed}/${tasks.length} 成功交付 · ${failed} 项未完成`
      : `${completed}/${tasks.length} 已回群`;
  return (
    <section className="task-progress" aria-label="本会话任务">
      <header>
        <span><Robot size={17} weight="fill" />本会话任务</span>
        <strong>{summary}</strong>
      </header>
      <div className="task-progress-list">
        {tasks.map((task) => {
          const agent = agentByID.get(task.target_agent_id);
          const delivery = deliveryForTask(task, messages);
          const syncing = task.state === "delivered" && !delivery;
          const attachments = taskAttachments(task, messages);
          return (
            <details className={`task-progress-row task-state-${task.state}${syncing ? " task-state-syncing" : ""}`} key={task.id} open={task.state === "queued" || task.state === "dispatched" || task.state === "running"}>
              <summary>
                <StateIcon state={task.state} syncing={syncing} />
                <span><strong>{task.title}</strong><small>{agent?.display_name ?? "Agent"}{agent?.device_name ? ` · ${agent.device_name}` : ""}</small></span>
                <em>{statusText(task, Boolean(delivery))}</em>
                <CaretDown className="task-details-caret" size={15} aria-hidden="true" />
              </summary>
              <div className="task-progress-details">
                <div><span>任务来源</span><p>{taskRequest(task, messages)}</p></div>
                {task.plan_step_id && <div><span>项目资料</span><p>{task.execution_source === "responsible_local" ? "负责人本机最新进展 · 仅汇报，不自动提交" : task.execution_source === "none" ? "不读取文件 · 仅会话上下文" : "公共底稿 · 群内共享只读副本"}</p></div>}
                {task.acceptance_criteria && <div><span>验收标准</span><p>{task.acceptance_criteria}</p></div>}
                {attachments.length > 0 && <div className="task-source-attachments"><span>随任务发送</span>{attachments.map((attachment) => <a key={attachment.id} href={attachment.download_url} download={attachment.filename}><FileText size={14} />{attachment.filename}</a>)}</div>}
                {delivery && <small className="task-delivery-linked">该 Agent 的交付已显示在群聊消息中。</small>}
              </div>
            </details>
          );
        })}
      </div>
    </section>
  );
}
