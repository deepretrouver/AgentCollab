import { useLocale } from "../i18n/locale";
import { useEffect, useRef, useState, type ChangeEvent, type FormEvent, type KeyboardEvent } from "react";
import { At, CaretDown, ChatsCircle, Command, FileText, PaperPlaneTilt, Paperclip, PlusCircle, Robot, Stop, UserCircle, X } from "@phosphor-icons/react";
import type { Agent, Message, ProjectPlan, Task, TaskExecutionSource, User } from "../api/client";
import { agentDisplayName, agentShortName, groupAgentsByOwner } from "../agents/presentation";
import { TaskComposer } from "../tasks/TaskComposer";
import { conversationCommands } from "./conversation";

interface ComposerProps {
  disabled?: boolean;
  agents: Agent[];
  tasks: Task[];
  currentUser?: User;
  members?: User[];
  projectPlan?: ProjectPlan;
  roomDefaultStepID?: string;
  onSend: (body: string, mentionAgentIDs: string[], files: File[], thread?: { reply_to_message_id: string; root_message_id: string }, planStepID?: string, executionSource?: TaskExecutionSource) => Promise<Message>;
  onCreateTask: (input: { target_agent_id: string; title: string; prompt: string; acceptance_criteria: string; plan_step_id?: string; execution_source?: TaskExecutionSource }) => Promise<Task>;
  onCancelTaskRoot: (rootMessageID: string) => Promise<void>;
  replyTopic?: { rootMessageID: string; senderName: string; preview: string };
  onClearReplyTopic?: () => void;
}

export function Composer({ disabled, agents, tasks, currentUser, members = [], projectPlan, roomDefaultStepID, onSend, onCreateTask, onCancelTaskRoot, replyTopic, onClearReplyTopic }: ComposerProps) {
  const { t } = useLocale();
  const [body, setBody] = useState("");
  const [selectedAgentIDs, setSelectedAgentIDs] = useState<string[]>([]);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState("");
  const [taskOpen, setTaskOpen] = useState(false);
  const [files, setFiles] = useState<File[]>([]);
  const [activeTaskRootID, setActiveTaskRootID] = useState("");
  const [stopping, setStopping] = useState(false);
  const [planStepID, setPlanStepID] = useState(roomDefaultStepID ?? "");
  const [executionSource, setExecutionSource] = useState<TaskExecutionSource>("official");
  const [interactionMode, setInteractionMode] = useState<"direct" | "discussion">("direct");
  const [expandedMentionOwners, setExpandedMentionOwners] = useState<Record<string, boolean>>({});
  const fileInputRef = useRef<HTMLInputElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const commandInput = body.trimStart();
  const commandToken = commandInput.startsWith("/") ? commandInput.slice(1).split(/\s/, 1)[0].toLocaleLowerCase() : "";
  const commandMenuOpen = commandInput.startsWith("/") && !commandInput.includes(" ");
  const matchingCommands = conversationCommands.filter((command) => command.name.startsWith(commandToken) || (command.name === "restart" && "reset".startsWith(commandToken)));
	const mentionableAgents = agents.filter((agent) => agent.can_invoke !== false && agent.status !== "unavailable" && agent.response_mode !== "muted");
  const knownPeople = currentUser ? [currentUser, ...members.filter((member) => member.id !== currentUser.id)] : members;
  const mentionGroups = groupAgentsByOwner(agents, knownPeople);
  const allAgentsSelected = mentionableAgents.length > 0 && mentionableAgents.every((agent) => selectedAgentIDs.includes(agent.id));
  const selectedStep = projectPlan?.steps.find((step) => step.id === planStepID);
  const officialAvailable = Boolean(selectedStep?.workspace?.last_commit);
  const selectedAgents = selectedAgentIDs.map((id) => agents.find((agent) => agent.id === id)).filter((agent): agent is Agent => Boolean(agent));
  const responsibleLocalAvailable = Boolean(selectedStep && selectedAgents.length > 0 && selectedAgents.every((agent) =>
    selectedStep.responsible_agent_ids.includes(agent.id)
    && selectedStep.local_bindings?.some((binding) => binding.user_id === agent.owner_user_id && binding.runner_status !== "revoked"),
  ));
  const responsibleLocalHint = !selectedStep?.responsible_agent_ids.length
    ? "尚未指定负责 Agent"
    : !selectedStep.local_bindings?.length
      ? "负责人尚未绑定本机目录"
      : selectedAgents.length === 0
        ? "请先点名负责 Agent"
        : !responsibleLocalAvailable
          ? "当前点名中包含非负责 Agent"
          : selectedStep.local_bindings.some((binding) => binding.runner_status === "online")
            ? "负责人目录可查询"
            : "负责人 Client 离线，任务将等待";

  useEffect(() => {
    if (!replyTopic) return;
    textareaRef.current?.focus();
  }, [replyTopic?.rootMessageID]);

  useEffect(() => setPlanStepID(roomDefaultStepID ?? ""), [roomDefaultStepID]);

  useEffect(() => {
    if (!selectedStep) return;
    if (executionSource === "official" && !officialAvailable) {
      setExecutionSource(responsibleLocalAvailable ? "responsible_local" : "none");
    } else if (executionSource === "responsible_local" && !responsibleLocalAvailable) {
      setExecutionSource(officialAvailable ? "official" : "none");
    }
  }, [selectedStep?.id, officialAvailable, responsibleLocalAvailable, executionSource]);

  useEffect(() => {
    if (!activeTaskRootID) return;
    const related = tasks.filter((task) => task.root_message_id === activeTaskRootID);
    const active = related.some((task) => task.state === "queued" || task.state === "dispatched" || task.state === "running");
    if (related.length > 0 && !active) setActiveTaskRootID("");
  }, [activeTaskRootID, tasks]);

  async function submit(event?: FormEvent) {
    event?.preventDefault();
    const rawValue = body.trim();
    const value = interactionMode === "discussion" && rawValue && !rawValue.startsWith("/") ? `/brainstorm ${rawValue}` : rawValue;
    if ((!value && files.length === 0) || sending || disabled) return;
    setError("");
    setSending(true);
    try {
      let created: Message;
      const requestsAgentWork = selectedAgentIDs.length > 0 || /^\/brainstorm\b/i.test(value) || agents.some((agent) => value.toLocaleLowerCase().includes(`@${agent.display_name.toLocaleLowerCase()}`));
      if (replyTopic) {
        const thread = { reply_to_message_id: replyTopic.rootMessageID, root_message_id: replyTopic.rootMessageID };
        created = planStepID
          ? await onSend(value || "请处理随附文件。", selectedAgentIDs, files, thread, planStepID, executionSource)
          : await onSend(value || "请处理随附文件。", selectedAgentIDs, files, thread);
      } else {
        created = planStepID
          ? await onSend(value || "请处理随附文件。", selectedAgentIDs, files, undefined, planStepID, executionSource)
          : await onSend(value || "请处理随附文件。", selectedAgentIDs, files);
      }
	  if (requestsAgentWork) setActiveTaskRootID(created.root_message_id ?? created.id);
      setBody("");
      setSelectedAgentIDs([]);
      setFiles([]);
      setInteractionMode("direct");
      onClearReplyTopic?.();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "消息发送失败");
    } finally {
      setSending(false);
    }
  }

  async function stopCurrentTask() {
    if (!activeTaskRootID || stopping) return;
    setStopping(true);
    setError("");
    try {
      await onCancelTaskRoot(activeTaskRootID);
      setActiveTaskRootID("");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "停止任务失败");
    } finally {
      setStopping(false);
    }
  }

  function addFiles(event: ChangeEvent<HTMLInputElement>) {
    const selectedFiles = Array.from(event.target.files ?? []);
    event.target.value = "";
    if (selectedFiles.length === 0) return;
    if (files.length + selectedFiles.length > 5) {
      setError("每条消息最多发送 5 个附件");
      return;
    }
    const oversized = selectedFiles.find((file) => file.size > 50 * 1024 * 1024);
    if (oversized) {
      setError(`${oversized.name} 超过 50 MB`);
      return;
    }
    setError("");
    setFiles((current) => [...current, ...selectedFiles]);
  }

  function updateBody(value: string) {
    setBody(value);
	if (/(?:^|\s)@(?:全体|everyone|all)(?:\s|$)/i.test(value)) {
	  setSelectedAgentIDs(mentionableAgents.map((agent) => agent.id));
	  return;
	}
    setSelectedAgentIDs((current) => current.filter((agentID) => {
      const agent = agents.find((item) => item.id === agentID);
      return agent ? value.toLocaleLowerCase().includes(`@${agent.display_name.toLocaleLowerCase()}`) : false;
    }));
  }

  function mentionAll() {
	const groupPattern = /(?:^|\s)@(?:全体|everyone|all)(?=\s|$)/ig;
	if (allAgentsSelected) {
	  const mentionableIDs = new Set(mentionableAgents.map((agent) => agent.id));
	  setSelectedAgentIDs((current) => current.filter((agentID) => !mentionableIDs.has(agentID)));
	  setBody((current) => current.replace(groupPattern, " ").replace(/\s+/g, " ").trimStart());
	  return;
	}
	setSelectedAgentIDs(mentionableAgents.map((agent) => agent.id));
	setBody((current) => `${current}${current && !current.endsWith(" ") ? " " : ""}@全体 `);
  }

  function mention(agent: Agent) {
    if (selectedAgentIDs.includes(agent.id)) return;
    setSelectedAgentIDs((current) => [...current, agent.id]);
    setBody((current) => `${current}${current && !current.endsWith(" ") ? " " : ""}@${agent.display_name} `);
  }

  function mentionAgentButton(agent: Agent, owner?: User) {
    const displayName = agentDisplayName(agent, owner);
    const shortName = agentShortName(agent, owner);
    const unavailableReason = agent.can_invoke === false
      ? "Agent 所有者尚未授权你调用"
      : agent.status === "offline"
        ? "设备离线；任务会排队，Client 上线后自动执行"
        : undefined;
    return <button
      key={agent.id}
      type="button"
      className={`mention-agent-button${selectedAgentIDs.includes(agent.id) ? " is-selected" : ""}`}
      aria-label={`${displayName}${agent.status === "offline" ? " · 离线" : ""}`}
      aria-pressed={selectedAgentIDs.includes(agent.id)}
      title={unavailableReason}
      disabled={disabled || agent.can_invoke === false || agent.status === "unavailable" || agent.response_mode === "muted"}
      onClick={() => mention(agent)}
    ><Robot size={14} /><span>{shortName}</span>{agent.status === "offline" && <small>离线</small>}</button>;
  }

  function keyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      void submit();
    }
  }

  return (
    <>
      {taskOpen && <TaskComposer agents={agents} members={knownPeople} projectPlan={projectPlan} initialStepID={planStepID} initialPrompt={body} onClose={() => setTaskOpen(false)} onCreate={async (input) => {
        const task = await onCreateTask(input);
        setActiveTaskRootID(task.root_message_id ?? task.source_message_id ?? "");
        return task;
      }} />}
    <form className="composer" onSubmit={submit}>
	  {replyTopic && <div className="topic-reply-context" role="status"><ChatsCircle size={17} weight="fill" /><span><strong>回复话题</strong><small>{replyTopic.senderName}：{replyTopic.preview}</small></span><button type="button" onClick={onClearReplyTopic} aria-label="退出话题回复"><X size={15} /></button></div>}
	  {!replyTopic && <div className="composer-intent-bar" aria-label="协作方式">
		<div className="composer-intent-options">
		  <button type="button" className={interactionMode === "direct" ? "is-selected" : ""} aria-pressed={interactionMode === "direct"} onClick={() => setInteractionMode("direct")}><At size={15} /><span>{t("直接点名")}</span></button>
		  <button type="button" className={interactionMode === "discussion" ? "is-selected" : ""} aria-pressed={interactionMode === "discussion"} onClick={() => { setInteractionMode("discussion"); setSelectedAgentIDs([]); }}><ChatsCircle size={15} /><span>{t("专题讨论")}</span></button>
		  <button type="button" disabled={disabled || agents.every((agent) => agent.can_invoke === false || agent.status === "unavailable")} onClick={() => setTaskOpen(true)} aria-label={t("创建结构化任务")}><PlusCircle size={15} /><span>{t("结构化任务")}</span></button>
		</div>
		<small>{t(interactionMode === "direct" ? "原文交给你点名的 Agent；不经过主协调改写" : "由主协调组织成员独立分析，再统一汇总")}</small>
	  </div>}
	  {agents.length > 0 && interactionMode === "direct" && <div className="mention-strip" aria-label="点名 Agent">
		<At size={15} />
		<button type="button" className={allAgentsSelected ? "is-selected" : ""} aria-pressed={allAgentsSelected} disabled={disabled || mentionableAgents.length === 0} onClick={mentionAll}>{t("全体")}</button>
		{mentionGroups.map((group) => {
          const hasMultipleAgents = group.agents.length > 1;
          const expanded = expandedMentionOwners[group.ownerID] ?? false;
          return <span className={`mention-owner-group${hasMultipleAgents ? " is-multiple" : " is-single"}`} key={group.ownerID} aria-label={`${group.ownerName} 的 Agent`}>
            {hasMultipleAgents ? <button
              type="button"
              className="mention-owner-toggle"
              aria-expanded={expanded}
              aria-label={`${expanded ? "收起" : "选择"} ${group.ownerName} 名下 ${group.agents.length} 个 Agent`}
              onClick={() => setExpandedMentionOwners((current) => ({ ...current, [group.ownerID]: !expanded }))}
            ><UserCircle size={15} weight="fill" /><span>{group.ownerName}</span><small>{group.agents.length}</small><CaretDown size={13} /></button> : <span className="mention-owner-name"><UserCircle size={15} weight="fill" />{group.ownerName}</span>}
            {(!hasMultipleAgents || expanded) && <span className="mention-owner-agents">{group.agents.map((agent) => mentionAgentButton(agent, group.owner))}</span>}
          </span>;
        })}
	  </div>}
      {commandMenuOpen && matchingCommands.length > 0 && <div className="slash-command-menu" role="listbox" aria-label={t("会话命令")}>
        <header><Command size={16} /><span>{t("会话命令")}</span><small>{t("作用于当前群聊会话")}</small></header>
        {matchingCommands.map((command) => <button key={command.name} type="button" role="option" aria-selected="false" onClick={() => setBody(command.name === "new" || command.name === "brainstorm" ? `/${command.name} ` : `/${command.name}`)}>
          <code>{t(command.usage)}</code><span>{t(command.description)}</span>
        </button>)}
        {commandToken === "reset" && <p>{t("/reset 是 /restart 的兼容别名。")}</p>}
      </div>}
      {files.length > 0 && <div className="composer-attachments" role="region" aria-label="待发送附件">
        {files.map((file, index) => <span key={`${file.name}-${file.size}-${file.lastModified}-${index}`}><FileText size={16} /><strong>{file.name}</strong><small>{formatFileSize(file.size)}</small><button type="button" onClick={() => setFiles((current) => current.filter((_, fileIndex) => fileIndex !== index))} aria-label={`移除附件 ${file.name}`}><X size={14} /></button></span>)}
      </div>}
      {projectPlan && projectPlan.steps.length > 0 && !replyTopic && <section className="composer-project-context" aria-label="本次会话上下文">
        <label className="composer-context-step">
          <span>归属步骤</span>
          <select aria-label="会话归属项目步骤" value={planStepID} onChange={(event) => setPlanStepID(event.target.value)}>
            <option value="">{t("普通会话 · 不计项目进度")}</option>
            {projectPlan.steps.filter((step) => step.status !== "cancelled").map((step) => <option key={step.id} value={step.id}>{step.parent_step_id ? "↳ " : ""}{step.title} · 权重 {step.weight}</option>)}
          </select>
        </label>
        {selectedStep && <>
          <label className="composer-context-source">
            <span>项目资料</span>
            <select aria-label="Agent 执行依据" value={executionSource} onChange={(event) => setExecutionSource(event.target.value as TaskExecutionSource)}>
              <option value="official" disabled={!officialAvailable}>{officialAvailable ? `公共底稿 v${selectedStep.workspace?.revision} · 群内共享只读` : "公共底稿 · 尚未上传"}</option>
              <option value="responsible_local" disabled={!responsibleLocalAvailable}>{responsibleLocalAvailable ? "查询负责人本机最新进展" : `负责人本机 · ${responsibleLocalHint}`}</option>
              <option value="none">不读取文件 · 仅会话上下文</option>
            </select>
          </label>
          <small className="composer-context-status">
            {executionSource === "official"
              ? `${selectedStep.workspace?.source_name} · ${selectedStep.workspace?.file_count ?? 0} 个文件；群成员与群内 Agent 可读，聊天任务只使用临时副本，不能覆盖公共底稿`
              : executionSource === "responsible_local"
                ? `${responsibleLocalHint} · 只汇报，不提交或合并`
                : (!officialAvailable && !responsibleLocalAvailable ? `当前没有可读项目目录：${responsibleLocalHint}` : "本次不读取项目文件，只保留步骤归属")}
          </small>
        </>}
      </section>}
      <label className="sr-only" htmlFor="chat-attachment">{t("添加会话附件")}</label>
      <input ref={fileInputRef} className="sr-only" id="chat-attachment" type="file" multiple accept=".pdf,.doc,.docx,.xls,.xlsx,.csv,.txt,.md,.ppt,.pptx,.png,.jpg,.jpeg,.webp" onChange={addFiles} disabled={disabled || sending} />
      <label className="sr-only" htmlFor="chat-message">{t("群聊消息")}</label>
      <textarea
		ref={textareaRef}
        id="chat-message"
        rows={1}
        maxLength={20_000}
        placeholder={t(replyTopic ? "回复此话题，可用 @ 点名 Agent" : interactionMode === "discussion" ? "写下讨论议题，主协调将组织分析与汇总" : "输入原文，再点名需要回复的 Agent")}
        value={body}
        onChange={(event) => updateBody(event.target.value)}
        onKeyDown={keyDown}
        disabled={disabled || sending}
      />
      <div className="composer-actions">
        <div className="composer-tools">
          <button className="attachment-button" type="button" disabled={disabled || sending || files.length >= 5} onClick={() => fileInputRef.current?.click()} aria-label={t("添加附件")}><Paperclip size={18} /><span>{t("附件")}</span></button>
          <button className="command-button" type="button" disabled={disabled} onClick={() => setBody("/")} aria-label={t("打开会话命令")}><Command size={18} /><span>{t("命令")}</span></button>
        </div>
        {activeTaskRootID ? (
          <button className="send-button send-stop-button" type="button" disabled={disabled || stopping} onClick={() => void stopCurrentTask()} aria-label={t("停止本次委托")} title={t("停止本次委托及其后续协作")}>
            <Stop size={18} weight="fill" />
          </button>
        ) : (
          <button className="send-button" type="submit" disabled={disabled || sending || (!body.trim() && files.length === 0)} aria-label={t("发送消息")}>
            <PaperPlaneTilt size={19} weight="fill" />
          </button>
        )}
      </div>
      {error && <p className="composer-error" role="alert">{t(error)}</p>}
    </form>
    </>
  );
}

function formatFileSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
