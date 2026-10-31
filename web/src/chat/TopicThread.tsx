import { ArrowRight, CaretDown, ChatText, ChatsCircle, Check, CheckCircle, Clock, FileArrowUp, GitBranch, Robot, SpinnerGap, Stop, UploadSimple, WarningCircle, XCircle } from "@phosphor-icons/react";
import { useEffect, useRef, useState } from "react";
import type { Agent, Message, Task, User, WorkItem, WorkItemReview, WorkItemStatus } from "../api/client";
import { WorkItemReviewPanel } from "../projects/WorkItemReviewPanel";
import { MessageItem } from "./MessageItem";
import { deliveryForTask, isSystemTaskTitle, taskStatusText } from "./taskDelivery";
import type { TopicThreadData } from "./topicThreads";
import { useLocale } from "../i18n/locale";

interface TopicThreadProps {
  topic: TopicThreadData;
  agents: Agent[];
  members: User[];
  currentUserID: string;
  onReplyTopic?: (root: Message) => void;
  onCancelTopic?: (rootMessageID: string) => Promise<void>;
  workItem?: WorkItem;
  stepTitle?: string;
  highlighted?: boolean;
  onOpenBoard?: () => void;
  onSubmitWorkspace?: (workItemID: string, files: File[]) => Promise<void>;
  onReviewWorkItem?: (workItemID: string) => Promise<WorkItemReview>;
  onUpdateWorkItem?: (workItemID: string, status: WorkItemStatus) => Promise<void>;
  onAddDeliveryFiles?: (rootMessageID: string, files: File[]) => Promise<void>;
  canAcceptWorkItem?: boolean;
}

function topicWorkState(item: WorkItem) {
  if (item.verification_status === "passed") return "合并校验通过";
  if (item.verification_status === "pending") return "合并后校验中";
  if (item.merge_status === "merged") return "已合并主项目";
  if (item.ai_review_status === "pending") return "AI 审查中";
  if (item.ai_review_status === "passed") return "AI 建议合并";
  if (item.status === "review") return "待真人验收";
  if (item.active_task_count > 0) return `${item.active_task_count} 项执行中`;
  if (item.status === "done") return "已完成";
  return "项目会话";
}

function TaskStateIcon({ task, messages }: { task: Task; messages: Message[] }) {
  const syncing = task.state === "delivered" && !deliveryForTask(task, messages);
  if (syncing || task.state === "running" || task.state === "dispatched") return <SpinnerGap className="topic-task-spinner" size={15} weight="bold" />;
  if (task.state === "delivered") return <Check size={15} weight="bold" />;
  if (task.state === "failed" || task.state === "blocked" || task.state === "cancelled") return <WarningCircle size={15} weight="fill" />;
  return <Clock size={15} weight="bold" />;
}

function replyTask(message: Message, tasks: Task[]) {
  const taskID = message.metadata?.task_id;
  return typeof taskID === "string" ? tasks.find((task) => task.id === taskID) : undefined;
}

export function TopicThread({ topic, agents, members, currentUserID, onReplyTopic, onCancelTopic, workItem, stepTitle, highlighted, onOpenBoard, onSubmitWorkspace, onReviewWorkItem, onUpdateWorkItem, onAddDeliveryFiles, canAcceptWorkItem = true }: TopicThreadProps) {
  const { t, locale } = useLocale();
  const count = (key: string, value: number) => t(key).replaceAll("{count}", String(value));
  const [stopping, setStopping] = useState(false);
  const [workspaceBusy, setWorkspaceBusy] = useState(false);
  const [workspaceError, setWorkspaceError] = useState("");
  const [workspaceReview, setWorkspaceReview] = useState<WorkItemReview>();
  const workspaceInput = useRef<HTMLInputElement>(null);
  const deliveryFileInput = useRef<HTMLInputElement>(null);
  const disclosure = useRef<HTMLDetailsElement>(null);
  const agentByID = new Map(agents.map((agent) => [agent.id, agent]));
  const memberNames = new Map(members.map((member) => [member.id, member.display_name]));
  const agentNames = new Map(agents.map((agent) => [agent.id, agent.display_name]));
  const agentTypes = new Map(agents.map((agent) => [agent.id, agent.type]));
  const replies = [...topic.replies].sort((left, right) => left.sequence - right.sequence);
  const repliedTaskIDs = new Set(replies.map((reply) => reply.metadata?.task_id).filter((taskID): taskID is string => typeof taskID === "string"));
  const waitingTasks = topic.tasks.filter((task) => !repliedTaskIDs.has(task.id));
  const active = topic.tasks.filter((task) => task.state === "queued" || task.state === "dispatched" || task.state === "running").length;
  const delivered = topic.tasks.filter((task) => task.state === "delivered" && Boolean(deliveryForTask(task, topic.replies))).length;
  const failed = topic.tasks.filter((task) => task.state === "failed" || task.state === "blocked" || task.state === "cancelled").length;
  const participantNames = [...new Set(topic.tasks.map((task) => agentByID.get(task.target_agent_id)?.display_name).filter(Boolean))].join(locale === "en" ? ", " : "、");
  const countLabel = replies.length > 0 ? count("{count} 条话题回复", replies.length) : count("{count} 项委托", topic.tasks.length);
  const statusLabel = active > 0
    ? `${count("{count} 项执行中", active)}${failed > 0 ? ` · ${count("{count} 项未完成", failed)}` : ""}`
    : topic.tasks.length > 0
      ? failed > 0
        ? `${delivered}/${topic.tasks.length} ${t("成功交付")} · ${count("{count} 项未完成", failed)}`
        : `${delivered}/${topic.tasks.length} ${t("已回群")}`
      : "";
  const taskTitle = String(topic.root.metadata?.task_title ?? workItem?.title ?? "").trim();
  const formalTask = Boolean(workItem && !taskTitle.endsWith("本地身份验证") && (workItem.plan_step_id || topic.root.metadata?.task_source === "structured"));
  const hasDelivered = delivered > 0 || Boolean(workItem && workItem.delivered_task_count > 0);

  useEffect(() => {
    if (highlighted && disclosure.current) disclosure.current.open = true;
  }, [highlighted]);

  async function cancelTopic() {
    if (!onCancelTopic || stopping) return;
    setStopping(true);
    try {
      await onCancelTopic(topic.root.id);
    } finally {
      setStopping(false);
    }
  }

  async function submitWorkspace(files: FileList | null) {
    const selected = Array.from(files ?? []);
    if (!workItem || !selected.length || !onSubmitWorkspace) return;
    setWorkspaceBusy(true); setWorkspaceError("");
    try { await onSubmitWorkspace(workItem.id, selected); }
    catch (reason) { setWorkspaceError(reason instanceof Error ? reason.message : "无法提交任务工作区"); }
    finally { setWorkspaceBusy(false); if (workspaceInput.current) workspaceInput.current.value = ""; }
  }

  async function reviewWorkspace() {
    if (!workItem || !onReviewWorkItem) return;
    setWorkspaceBusy(true); setWorkspaceError("");
    try { setWorkspaceReview(await onReviewWorkItem(workItem.id)); }
    catch (reason) { setWorkspaceError(reason instanceof Error ? reason.message : "无法读取任务分支改动"); }
    finally { setWorkspaceBusy(false); }
  }

  async function updateAcceptance(status: WorkItemStatus) {
    if (!workItem || !onUpdateWorkItem) return;
    setWorkspaceBusy(true); setWorkspaceError("");
    try { await onUpdateWorkItem(workItem.id, status); }
    catch (reason) { setWorkspaceError(reason instanceof Error ? reason.message : "无法更新验收状态"); }
    finally { setWorkspaceBusy(false); }
  }

  async function addDeliveryFiles(files: FileList | null) {
    const selected = Array.from(files ?? []);
    if (!selected.length || !onAddDeliveryFiles) return;
    setWorkspaceBusy(true); setWorkspaceError("");
    try { await onAddDeliveryFiles(topic.root.id, selected); }
    catch (reason) { setWorkspaceError(reason instanceof Error ? reason.message : "无法补充交付文件"); }
    finally { setWorkspaceBusy(false); if (deliveryFileInput.current) deliveryFileInput.current.value = ""; }
  }

  return (
    <section id={`topic-${topic.root.id}`} className={`topic-thread${highlighted ? " is-linked-focus" : ""}`} aria-label={`话题：${topic.root.body.slice(0, 80)}`}>
      <MessageItem
        message={topic.root}
        senderName={memberNames.get(topic.root.sender_id) ?? "群成员"}
        own={topic.root.sender_id === currentUserID}
        variant="topic-root"
      />
      {workItem?.plan_step_id && <div className="topic-link-chain" aria-label="该话题的项目关联">
        <span><GitBranch size={14} />{t("归属步骤")}</span>
        <strong title={stepTitle ?? workItem.plan_step_id}>{stepTitle ?? t("项目步骤")}</strong>
        <ArrowRight size={12} aria-hidden="true" />
        <em>{workItem.active_task_count > 0 && topicWorkState(workItem).endsWith("项执行中") ? count("{count} 项执行中", workItem.active_task_count) : t(topicWorkState(workItem))}</em>
        {onOpenBoard && <button type="button" onClick={onOpenBoard}>{t("在看板查看")}</button>}
      </div>}
      <details ref={disclosure} className="topic-thread-disclosure">
        <summary>
          <ChatsCircle size={18} weight="fill" aria-hidden="true" />
          <span className="topic-thread-summary-copy"><strong>{countLabel}</strong>{participantNames && <small>{participantNames}</small>}</span>
          {statusLabel && <em>{statusLabel}</em>}
          <CaretDown className="topic-thread-caret" size={16} aria-hidden="true" />
        </summary>
        <div className="topic-thread-panel">
          {waitingTasks.length > 0 && <div className="topic-task-list" aria-label="话题执行状态">
            {waitingTasks.map((task) => {
              const agent = agentByID.get(task.target_agent_id);
              return <div className={`topic-task-status topic-task-${task.state}`} key={task.id}>
                <TaskStateIcon task={task} messages={topic.replies} />
                <span><strong>{agent?.display_name ?? "Agent"}</strong>{!isSystemTaskTitle(task.title) && <small>{task.title}</small>}</span>
                <em>{t(taskStatusText(task, topic.replies))}</em>
              </div>;
            })}
          </div>}
          {replies.length > 0 && <div className="topic-thread-replies">
            {replies.map((reply) => {
              const task = replyTask(reply, topic.tasks);
              return <MessageItem
                key={reply.id}
                message={reply}
                senderName={reply.sender_kind === "system" ? "系统" : memberNames.get(reply.sender_id) ?? agentNames.get(reply.sender_id) ?? (reply.sender_kind === "agent" ? "Agent" : "群成员")}
                senderType={agentTypes.get(reply.sender_id)}
                own={reply.sender_id === currentUserID}
                variant="topic-reply"
                taskTitle={task && !isSystemTaskTitle(task.title) ? task.title : undefined}
              />;
            })}
          </div>}
          <div className="topic-thread-actions">
            {onReplyTopic && <button type="button" className="topic-reply-button" onClick={() => onReplyTopic(topic.root)}><ChatText size={16} />{t("回复话题")}</button>}
            {workItem?.plan_step_id && workItem.merge_status !== "merged" && workItem.status !== "review" && <button type="button" className="topic-workspace-button" disabled={workspaceBusy || !onSubmitWorkspace} onClick={() => workspaceInput.current?.click()}><UploadSimple size={16} />{workspaceBusy ? "提交中" : workItem.git_branch ? "更新任务工作区" : "选择目录并提交审查"}</button>}
            {workItem?.git_branch && workItem.merge_status !== "merged" && workItem.status === "review" && (!workItem.ai_review_status || workItem.ai_review_status === "none") && <span className="topic-merged-state"><Robot size={15} />请到“看板”指定 AI 审查</span>}
            {workItem?.git_branch && workItem.merge_status !== "merged" && workItem.status === "review" && workItem.ai_review_status === "pending" && <span className="topic-merged-state"><SpinnerGap className="is-spinning" size={15} />AI 正在审查</span>}
            {workItem?.git_branch && workItem.merge_status !== "merged" && workItem.status === "review" && (workItem.ai_review_status === "passed" || workItem.ai_review_status === "changes_requested" || workItem.ai_review_status === "failed") && <button type="button" className="topic-workspace-button is-review" disabled={workspaceBusy || !onReviewWorkItem} onClick={() => void reviewWorkspace()}><GitBranch size={16} />{workspaceBusy ? "读取改动" : workspaceReview ? "刷新技术详情" : "查看技术详情"}</button>}
            {workItem?.merge_status === "merged" && <span className="topic-merged-state"><Check size={15} />已合并到主工作区</span>}
            {workItem && !formalTask && <section className="topic-disposition-card"><span><ChatText size={18} /><strong>这是普通会话，不需要项目验收</strong><small>回复仍完整保留在聊天中，但不会进入总控台、风险列表或正式交付。</small></span>{canAcceptWorkItem && workItem.status !== "done" && workItem.status !== "cancelled" && active === 0 && <button type="button" disabled={workspaceBusy || !onUpdateWorkItem} onClick={() => void updateAcceptance("cancelled")}>标记已处理</button>}</section>}
            {formalTask && workItem && failed > 0 && active === 0 && workItem.status !== "done" && workItem.status !== "cancelled" && !hasDelivered && <section className="topic-disposition-card is-warning"><span><WarningCircle size={18} /><strong>本事项执行未成功</strong><small>可回到输入框重新委托；若已无需继续，关闭事项后将从待处理列表移除，历史会话仍保留。</small></span>{canAcceptWorkItem && <button type="button" disabled={workspaceBusy || !onUpdateWorkItem} onClick={() => void updateAcceptance("cancelled")}>关闭事项</button>}</section>}
            {formalTask && workItem?.status === "review" && !workItem.git_branch && hasDelivered && <section className="topic-acceptance-card"><span><CheckCircle size={18} weight="duotone" /><strong>{t("文字与文件交付待你验收")}</strong><small>{t("无需 Git 分支；确认结论和附件满足要求即可通过")}</small></span>{canAcceptWorkItem ? <div><button type="button" className="is-primary" disabled={workspaceBusy || !onUpdateWorkItem} onClick={() => void updateAcceptance("done")}><CheckCircle size={16} />{t("验收通过")}</button><button type="button" disabled={workspaceBusy || !onUpdateWorkItem} onClick={() => void updateAcceptance("in_progress")}><XCircle size={16} />{t("退回修改")}</button></div> : <small>{t("等待工作区管理员验收")}</small>}</section>}
            {formalTask && workItem?.status !== "done" && hasDelivered && <button type="button" className="topic-workspace-button" disabled={workspaceBusy || !onAddDeliveryFiles} onClick={() => deliveryFileInput.current?.click()}><FileArrowUp size={16} />{t("补充交付文件")}</button>}
            <input ref={workspaceInput} className="sr-only" type="file" multiple {...({ webkitdirectory: "" } as Record<string, string>)} onChange={(event) => void submitWorkspace(event.target.files)} />
            <input ref={deliveryFileInput} className="sr-only" type="file" multiple accept=".pdf,.doc,.docx,.xls,.xlsx,.csv,.txt,.md,.ppt,.pptx,.png,.jpg,.jpeg,.webp,.zip,.json" onChange={(event) => void addDeliveryFiles(event.target.files)} />
            {active > 0 && topic.root.sender_id === currentUserID && onCancelTopic && <button type="button" className="topic-cancel-button" disabled={stopping} onClick={() => void cancelTopic()}><Stop size={16} weight="fill" />{t(stopping ? "正在停止" : "停止本次委托")}</button>}
          </div>
          {workspaceReview && <WorkItemReviewPanel review={workspaceReview} merging={workspaceBusy} onClose={() => setWorkspaceReview(undefined)} />}
          {workspaceError && <p className="topic-workspace-error" role="alert">{workspaceError}</p>}
        </div>
      </details>
    </section>
  );
}
