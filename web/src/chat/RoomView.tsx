import { useLocale } from "../i18n/locale";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { Archive, BookOpenText, Buildings, ChatCircleDots, CirclesFour, FileText, LinkSimple, SidebarSimple, SlidersHorizontal, UsersThree, WarningCircle } from "@phosphor-icons/react";
import type { Agent, KnowledgeCollection, Message, ProjectPlan, ProjectPlanStatus, ProjectRepository, ProjectStepStatus, Room, Task, TaskExecutionSource, User, WorkItem, WorkItemReview, WorkItemStatus } from "../api/client";
import type { ConnectionState } from "../realtime/socket";
import { Composer } from "./Composer";
import { MessageItem } from "./MessageItem";
import { RoomOnboarding } from "../agents/RoomOnboarding";
import { commandLabel, conversationTasks, conversationWindow, visibleConversationMessages } from "./conversation";
import { TopicThread } from "./TopicThread";
import { conversationTimeline } from "./topicThreads";
import { ProjectCockpit } from "../projects/ProjectCockpit";
import { activeProjectItemCount } from "../projects/workItemPresentation";

type RoomSurface = "overview" | "chat" | "board" | "knowledge" | "artifacts";

interface RoomViewProps {
  room?: Room;
  currentUser: User;
  members: User[];
  workspaceMembers?: User[];
  workspaceAgents?: Agent[];
  agents: Agent[];
  registeredAgentCount: number;
  messages: Message[];
  tasks: Task[];
  workItems?: WorkItem[];
  projectPlan?: ProjectPlan;
  projectRepository?: ProjectRepository;
  workspaceKnowledge?: KnowledgeCollection;
  roomKnowledge?: KnowledgeCollection;
  knowledgeLoading?: boolean;
  knowledgeError?: string;
  loading: boolean;
  error: string;
  connection: ConnectionState;
  onSend: (body: string, mentionAgentIDs: string[], files: File[], thread?: { reply_to_message_id: string; root_message_id: string }, planStepID?: string, executionSource?: TaskExecutionSource) => Promise<Message>;
  onCreateTask: (input: { target_agent_id: string; title: string; prompt: string; acceptance_criteria: string; plan_step_id?: string; execution_source?: TaskExecutionSource }) => Promise<Task>;
  onCancelTaskRoot: (rootMessageID: string) => Promise<void>;
  onUpdateWorkItem?: (itemID: string, patch: { status: WorkItemStatus }) => Promise<void>;
  onReviewWorkItem?: (itemID: string) => Promise<WorkItemReview>;
  onMergeWorkItem?: (itemID: string) => Promise<void>;
  onStartWorkItemAIReview?: (itemID: string, reviewAgentID: string, verificationAgentID: string) => Promise<void>;
  onRejectWorkItem?: (itemID: string, reason: string) => Promise<void>;
  onSubmitWorkItemWorkspace?: (itemID: string, files: File[]) => Promise<void>;
  onUpdateProjectPlan?: (patch: Partial<{ objective: string; status: ProjectPlanStatus }>) => Promise<void>;
  onCreateProjectStep?: (input: { parent_step_id?: string; title: string; description?: string; weight: number; status?: ProjectStepStatus; sort_order?: number }) => Promise<void>;
  onUpdateProjectStep?: (stepID: string, patch: Partial<{ parent_step_id: string; clear_parent: boolean; title: string; description: string; weight: number; status: ProjectStepStatus; sort_order: number; responsible_user_ids: string[]; responsible_agent_ids: string[] }>) => Promise<void>;
  onDeleteProjectStep?: (stepID: string) => Promise<void>;
  onUploadProjectStepWorkspace?: (stepID: string, files: File[]) => Promise<void>;
  onRemoveProjectStepWorkspace?: (stepID: string) => Promise<void>;
  onBindProjectStepLocal?: (stepID: string) => Promise<void>;
  onRemoveProjectStepLocal?: (stepID: string) => Promise<void>;
  onOpenProjectStepLocal?: (stepID: string) => Promise<void>;
  onUploadKnowledge?: (scope: "workspace" | "room", file: File) => Promise<void>;
  onDeleteKnowledge?: (scope: "workspace" | "room", assetID: string) => Promise<void>;
  onManageAgents: () => void;
  onVerifyAgents: () => Promise<void>;
  verifying: boolean;
  verificationError: string;
  onShowSidebar: () => void;
  onShowDetails: () => void;
  hasWorkspaces?: boolean;
  onJoinWorkspace?: (inviteLink: string) => Promise<void> | void;
  leftPanelVisible?: boolean;
  rightPanelVisible?: boolean;
  onToggleLeftPanel?: () => void;
  onToggleRightPanel?: () => void;
}

const connectionText: Record<ConnectionState, string> = {
  connecting: "消息同步中",
  live: "消息实时同步",
  polling: "消息自动同步",
  retrying: "消息连接恢复中",
  offline: "消息暂不可用",
};

const connectionDescription: Record<ConnectionState, string> = {
  connecting: "正在建立群聊消息连接。",
  live: "新消息会实时显示。",
  polling: "实时通道暂时不可用，群消息仍会自动同步，不影响 Agent 在线状态。",
  retrying: "正在恢复群聊消息连接，消息仍会自动补同步。",
  offline: "浏览器暂时无法连接 Hub，请检查当前网络。",
};

export function RoomView(props: RoomViewProps) {
  const { t, locale, setLocale } = useLocale();
  const endRef = useRef<HTMLDivElement>(null);
  const [showEarlier, setShowEarlier] = useState(false);
  const [replyTopic, setReplyTopic] = useState<{ rootMessageID: string; senderName: string; preview: string }>();
	const [layoutMenuOpen, setLayoutMenuOpen] = useState(false);
  const [workspaceInvite, setWorkspaceInvite] = useState("");
  const [workspaceJoinError, setWorkspaceJoinError] = useState("");
  const [joiningWorkspace, setJoiningWorkspace] = useState(false);
  const [surface, setSurface] = useState<RoomSurface>("chat");
  const [focusedTopicID, setFocusedTopicID] = useState("");
  const memberNames = new Map(props.members.map((member) => [member.id, member.display_name]));
  const agentNames = new Map(props.agents.map((agent) => [agent.id, agent.display_name]));
  const agentTypes = new Map(props.agents.map((agent) => [agent.id, agent.type]));
  memberNames.set(props.currentUser.id, props.currentUser.display_name);
  const conversation = conversationWindow(props.messages);
  const previousMessages = visibleConversationMessages(conversation.previous);
  const currentMessages = visibleConversationMessages(conversation.current);
  const currentTasks = conversationTasks(props.tasks, conversation.boundary);
  const currentTimeline = conversationTimeline(currentMessages, currentTasks, (props.workItems ?? []).flatMap((item) => item.root_message_id ? [item.root_message_id] : []));

  useEffect(() => {
    if (surface === "chat") endRef.current?.scrollIntoView({ block: "end" });
  }, [props.messages.length, conversation.boundary?.sequence, surface]);

  useEffect(() => setShowEarlier(false), [props.room?.id, conversation.boundary?.sequence]);
  useEffect(() => setReplyTopic(undefined), [props.room?.id, conversation.boundary?.sequence]);
  useEffect(() => setFocusedTopicID(""), [props.room?.id, conversation.boundary?.sequence]);
  useEffect(() => {
    const saved = props.room?.id ? window.localStorage.getItem(`agentcollab.room-surface.${props.room.id}`) : null;
    setSurface(saved === "overview" || saved === "board" || saved === "knowledge" || saved === "artifacts" ? saved : "chat");
  }, [props.room?.id]);

  async function joinWorkspace(event: FormEvent) {
    event.preventDefault();
    if (!workspaceInvite.trim() || !props.onJoinWorkspace) return;
    setWorkspaceJoinError("");
    setJoiningWorkspace(true);
    try {
      await props.onJoinWorkspace(workspaceInvite.trim());
    } catch (reason) {
      setWorkspaceJoinError(reason instanceof Error ? reason.message : "无法加入团队");
    } finally {
      setJoiningWorkspace(false);
    }
  }

  function selectSurface(next: RoomSurface) {
    setSurface(next);
    if (props.room) window.localStorage.setItem(`agentcollab.room-surface.${props.room.id}`, next);
  }

  function openChat(rootMessageID?: string) {
    selectSurface("chat");
    if (rootMessageID) setFocusedTopicID(rootMessageID);
  }

  useEffect(() => {
    if (surface !== "chat" || !focusedTopicID) return;
    const scrollTimer = window.setTimeout(() => document.getElementById(`topic-${focusedTopicID}`)?.scrollIntoView({ behavior: "smooth", block: "center" }), 40);
    const clearTimer = window.setTimeout(() => setFocusedTopicID(""), 2400);
    return () => { window.clearTimeout(scrollTimer); window.clearTimeout(clearTimer); };
  }, [surface, focusedTopicID, currentTimeline.length]);

	const layoutControl = <div className="layout-control">
		<select aria-label="Language / 语言" value={locale} onChange={event => setLocale(event.target.value === "en" ? "en" : "zh-CN")}><option value="zh-CN">中文</option><option value="en">English</option></select>
		<button className="layout-control-trigger" type="button" onClick={() => setLayoutMenuOpen((current) => !current)} aria-haspopup="menu" aria-expanded={layoutMenuOpen} aria-label={t("视图布局")} title={t("视图布局")}><SlidersHorizontal size={18} /></button>
		{layoutMenuOpen && <div className="layout-control-menu" role="menu" aria-label={t("视图布局")}>
			<button type="button" role="menuitemcheckbox" aria-checked={props.leftPanelVisible !== false} onClick={() => { props.onToggleLeftPanel?.(); setLayoutMenuOpen(false); }}><SidebarSimple size={17} /><span>{t("工作区")}</span><em>{props.leftPanelVisible === false ? t("已隐藏") : t("显示中")}</em></button>
			<button type="button" role="menuitemcheckbox" aria-checked={props.rightPanelVisible !== false} onClick={() => { props.onToggleRightPanel?.(); setLayoutMenuOpen(false); }}><UsersThree size={17} /><span>{t("群详情")}</span><em>{props.rightPanelVisible === false ? t("已隐藏") : t("显示中")}</em></button>
		</div>}
	</div>;

  if (!props.room) {
    if (props.hasWorkspaces === false) {
      return (
        <main className="room-view room-unselected first-workspace-state">
          <button className="mobile-icon-button sidebar-trigger" onClick={props.onShowSidebar} aria-label={t("显示工作区")}>
            <SidebarSimple size={21} />
          </button>
          <div className="empty-panel-controls">{layoutControl}</div>
          <section className="first-workspace-card">
            <span className="first-workspace-icon"><LinkSimple size={24} /></span>
            <p className="section-kicker">首次进入</p>
            <h2>加入已有团队</h2>
            <p>注册账号不会自动进入其他人的工作区。请粘贴群主发来的完整邀请链接，Client 会自动找到对应团队并完成加入。</p>
            <form onSubmit={(event) => void joinWorkspace(event)}>
              <label htmlFor="first-workspace-invite">团队邀请链接</label>
              <input id="first-workspace-invite" value={workspaceInvite} onChange={(event) => setWorkspaceInvite(event.target.value)} placeholder="http://团队地址/?join=…" autoFocus />
              {workspaceJoinError && <p className="form-error" role="alert">{workspaceJoinError}</p>}
              <button type="submit" disabled={joiningWorkspace || !workspaceInvite.trim()}>{joiningWorkspace ? "正在加入" : "加入团队"}</button>
            </form>
            <button className="first-workspace-secondary" type="button" onClick={props.onShowSidebar}>或者创建自己的工作区</button>
          </section>
        </main>
      );
    }
    return (
      <main className="room-view room-unselected">
        <button className="mobile-icon-button sidebar-trigger" onClick={props.onShowSidebar} aria-label={t("显示工作区")}>
          <SidebarSimple size={21} />
        </button>
        <div className="empty-panel-controls">{layoutControl}</div>
        <div className="empty-symbol" aria-hidden="true">AC</div>
        <h2>选择一个群聊</h2>
        <p>在左侧选择群，或先创建工作区和第一个群。</p>
      </main>
    );
  }

  return (
    <main className="room-view">
      <header className="room-header">
        <button className="mobile-icon-button" onClick={props.onShowSidebar} aria-label={t("显示工作区")}>
          <SidebarSimple size={21} />
        </button>
        <div className="room-heading">
          <h1># {props.room.name}</h1>
          <span className={`connection-state connection-${props.connection}`} title={t(connectionDescription[props.connection])} aria-label={`${t("群聊消息状态")}：${t(connectionText[props.connection])}。${t(connectionDescription[props.connection])}`}>{t(connectionText[props.connection])}</span>
        </div>
        {layoutControl}
        <button className="mobile-icon-button" onClick={props.onShowDetails} aria-label={t("显示群成员")}>
          <UsersThree size={21} />
        </button>
      </header>

      <nav className="room-surface-tabs" aria-label={t("群聊项目视图")}>
        <button type="button" className={surface === "overview" ? "is-active" : ""} onClick={() => selectSurface("overview")}><CirclesFour size={17} />{t("总览")}</button>
        <button type="button" className={surface === "chat" ? "is-active" : ""} onClick={() => selectSurface("chat")}><ChatCircleDots size={17} />{t("聊天")}</button>
        <button type="button" className={surface === "board" ? "is-active" : ""} onClick={() => selectSurface("board")}><Buildings size={17} />{t("协作台")}{activeProjectItemCount(props.workItems ?? []) > 0 && <span>{activeProjectItemCount(props.workItems ?? [])}</span>}</button>
        <button type="button" className={surface === "knowledge" ? "is-active" : ""} onClick={() => selectSurface("knowledge")}><BookOpenText size={17} />{t("知识库")}</button>
        <button type="button" className={surface === "artifacts" ? "is-active" : ""} onClick={() => selectSurface("artifacts")}><FileText size={17} />{t("产物")}</button>
      </nav>

      {surface === "chat" ? <>
      <section className="message-list" aria-live="polite" aria-busy={props.loading}>
        {props.loading && (
          <div className="message-skeletons" aria-label="正在加载消息">
            <span /><span /><span />
          </div>
        )}
        {!props.loading && props.error && (
          <div className="inline-state error-state" role="alert">
            <WarningCircle size={24} />
            <div><strong>无法载入消息</strong><p>{props.error}</p></div>
          </div>
        )}
        {props.messages.length > 0 && props.verificationError && <p className="onboarding-error" role="alert">{props.verificationError}</p>}
        {!props.loading && !props.error && props.messages.length === 0 && (
          <RoomOnboarding registeredCount={props.registeredAgentCount} agents={props.agents} verifying={props.verifying} verificationError={props.verificationError} onManageAgents={props.onManageAgents} onShowMembers={props.onShowDetails} onVerify={props.onVerifyAgents} />
        )}
        {!props.loading && !props.error && showEarlier && previousMessages.map((message) => (
          <MessageItem
            key={message.id}
            message={message}
            senderName={memberNames.get(message.sender_id) ?? agentNames.get(message.sender_id) ?? (message.sender_kind === "agent" ? "Agent" : "群成员")}
            senderType={agentTypes.get(message.sender_id)}
            own={message.sender_id === props.currentUser.id}
          />
        ))}
        {!props.loading && !props.error && conversation.boundary && conversation.command && (
          <div className="conversation-boundary" role="status">
            <span><Archive size={18} weight="duotone" /></span>
            <div><strong>{commandLabel(conversation.command)}</strong><small>旧消息仍保留用于审计，但不会进入后续 Agent 上下文。</small></div>
            {previousMessages.length > 0 && <button type="button" onClick={() => setShowEarlier((current) => !current)}>{showEarlier ? "收起旧消息" : `查看之前 ${previousMessages.length} 条`}</button>}
          </div>
        )}
        {!props.loading && !props.error && currentTimeline.map((item) => item.kind === "topic" ? (
          <TopicThread key={item.topic.root.id} topic={item.topic} agents={props.agents} members={props.members} currentUserID={props.currentUser.id} workItem={props.workItems?.find((workItem) => workItem.root_message_id === item.topic.root.id)} stepTitle={props.projectPlan?.steps.find((step) => step.id === props.workItems?.find((workItem) => workItem.root_message_id === item.topic.root.id)?.plan_step_id)?.title} highlighted={focusedTopicID === item.topic.root.id} onOpenBoard={() => selectSurface("board")} onSubmitWorkspace={props.onSubmitWorkItemWorkspace} onReviewWorkItem={props.onReviewWorkItem} onUpdateWorkItem={async (itemID, status) => props.onUpdateWorkItem?.(itemID, { status })} onAddDeliveryFiles={async (rootMessageID, files) => { await props.onSend("补充交付文件", [], files, { reply_to_message_id: rootMessageID, root_message_id: rootMessageID }); }} canAcceptWorkItem={Boolean(props.projectPlan?.can_manage || props.currentUser.workspace_role === "owner" || props.currentUser.workspace_role === "admin" || (!props.currentUser.workspace_role && props.currentUser.role === "admin"))} onCancelTopic={props.onCancelTaskRoot} onReplyTopic={(root) => setReplyTopic({ rootMessageID: root.id, senderName: memberNames.get(root.sender_id) ?? "群成员", preview: root.body.replace(/\s+/g, " ").slice(0, 54) })} />
        ) : (
          <MessageItem
            key={item.message.id}
            message={item.message}
            senderName={memberNames.get(item.message.sender_id) ?? agentNames.get(item.message.sender_id) ?? (item.message.sender_kind === "agent" ? "Agent" : "群成员")}
            senderType={agentTypes.get(item.message.sender_id)}
            own={item.message.sender_id === props.currentUser.id}
          />
        ))}
        <div ref={endRef} />
      </section>
	  <Composer disabled={props.loading || Boolean(props.error)} agents={props.agents} tasks={props.tasks} currentUser={props.currentUser} members={props.members} projectPlan={props.projectPlan} roomDefaultStepID={props.room.plan_step_id} onSend={props.onSend} onCreateTask={props.onCreateTask} onCancelTaskRoot={props.onCancelTaskRoot} replyTopic={replyTopic} onClearReplyTopic={() => setReplyTopic(undefined)} />
	  </> : <section className="project-surface"><ProjectCockpit view={surface} defaultBoardMode="office" currentUser={props.currentUser} plan={props.projectPlan} repository={props.projectRepository} items={props.workItems ?? []} tasks={props.tasks} messages={props.messages} members={props.workspaceMembers ?? props.members} agents={props.workspaceAgents ?? props.agents} roomAgents={props.agents} workspaceKnowledge={props.workspaceKnowledge} roomKnowledge={props.roomKnowledge} knowledgeLoading={props.knowledgeLoading} knowledgeError={props.knowledgeError} onUploadKnowledge={props.onUploadKnowledge} onDeleteKnowledge={props.onDeleteKnowledge} onUpdateStatus={async (itemID, status) => props.onUpdateWorkItem?.(itemID, { status })} onReviewWorkItem={props.onReviewWorkItem} onMergeWorkItem={props.onMergeWorkItem} onStartAIReview={props.onStartWorkItemAIReview} onRejectWorkItem={props.onRejectWorkItem} onSubmitWorkItemWorkspace={props.onSubmitWorkItemWorkspace} onUpdatePlan={props.onUpdateProjectPlan} onCreateStep={props.onCreateProjectStep} onUpdateStep={props.onUpdateProjectStep} onDeleteStep={props.onDeleteProjectStep} onUploadStepWorkspace={props.onUploadProjectStepWorkspace} onRemoveStepWorkspace={props.onRemoveProjectStepWorkspace} onBindStepLocal={props.onBindProjectStepLocal} onRemoveStepLocal={props.onRemoveProjectStepLocal} onOpenStepLocal={props.onOpenProjectStepLocal} onOpenChat={openChat} /></section>}
    </main>
  );
}
