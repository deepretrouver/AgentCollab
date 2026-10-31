import { useCallback, useEffect, useRef, useState } from "react";
import { api, ApiError, type Agent, type HumanInvite, type KnowledgeCollection, type Message, type ProjectPlan, type ProjectPlanStatus, type ProjectRepository, type ProjectStepStatus, type Room, type RunnerInvite, type Task, type TaskExecutionSource, type User, type WorkItem, type WorkItemStatus, type Workspace } from "../api/client";
import { LoginPage } from "../auth/LoginPage";
import { ProfileDialog, type ProfileSection } from "../profile/ProfileDialog";
import { HubAccountsDialog } from "../admin/HubAccountsDialog";
import { AddAgentDialog } from "../agents/AddAgentDialog";
import { dispatchVerification } from "../agents/verification";
import { RoomInspector } from "../chat/RoomInspector";
import { RoomView } from "../chat/RoomView";
import { WorkspaceSidebar } from "../chat/WorkspaceSidebar";
import { parseConversationCommand, taskIsActive } from "../chat/conversation";
import { connectRoomEvents, type ConnectionState } from "../realtime/socket";
import { useLocale } from "../i18n/locale";

function mergeMessage(messages: Message[], incoming: Message): Message[] {
  if (messages.some((message) => message.id === incoming.id || message.sequence === incoming.sequence)) return messages;
  return [...messages, incoming].sort((left, right) => left.sequence - right.sequence);
}

function mergeTask(tasks: Task[], incoming: Task): Task[] {
  const existing = tasks.findIndex((task) => task.id === incoming.id);
  if (existing < 0) return [...tasks, incoming].sort((left, right) => new Date(left.created_at).getTime() - new Date(right.created_at).getTime());
  return tasks.map((task, index) => index === existing ? incoming : task);
}

function mergeWorkItem(items: WorkItem[], incoming: WorkItem): WorkItem[] {
  const existing = items.findIndex((item) => item.id === incoming.id);
  if (existing < 0) return [...items, incoming];
  return items.map((item, index) => index === existing ? incoming : item);
}

function storedPanelState(key: string) {
  return window.localStorage.getItem(key) !== "hidden";
}

type Theme = "light" | "dark";
type DesktopOwnershipIssue = "unconnected" | "other-account";

function storedTheme(): Theme {
  return window.localStorage.getItem("agentcollab.theme") === "dark" ? "dark" : "light";
}

function invitationError(reason: unknown) {
  if (reason instanceof ApiError && reason.status === 409) return "邀请链接已使用或已过期，请让邀请人重新生成。";
  return reason instanceof Error ? reason.message : "无法加入群聊";
}

function invitationAlreadyUsed(reason: unknown) {
  return reason instanceof ApiError && reason.status === 409;
}

const expiredInvitationLoginNotice = "该邀请已使用或过期，已按普通账号登录。若你已经在群聊中可继续使用；否则请让邀请人重新邀请。";

function invitationCodeFromURL() {
  return new URLSearchParams(window.location.search).get("join")?.trim() ?? "";
}

function adminRequestedFromURL() {
  return new URLSearchParams(window.location.search).get("admin") === "1";
}

export function App() {
  const { t } = useLocale();
  const [session, setSession] = useState<"loading" | "anonymous" | "authenticated">("loading");
  const [user, setUser] = useState<User>();
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [workspace, setWorkspace] = useState<Workspace>();
  const [rooms, setRooms] = useState<Room[]>([]);
  const [room, setRoom] = useState<Room>();
  const [messages, setMessages] = useState<Message[]>([]);
	const [members, setMembers] = useState<User[]>([]);
	const [workspaceMembers, setWorkspaceMembers] = useState<User[]>([]);
  const [discoveredAgents, setDiscoveredAgents] = useState<Agent[]>([]);
  const [workspaceAgents, setWorkspaceAgents] = useState<Agent[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [workItems, setWorkItems] = useState<WorkItem[]>([]);
  const [projectPlan, setProjectPlan] = useState<ProjectPlan>();
  const [projectRepository, setProjectRepository] = useState<ProjectRepository>();
  const [workspaceKnowledge, setWorkspaceKnowledge] = useState<KnowledgeCollection>({ assets: [], directory: "" });
  const [roomKnowledge, setRoomKnowledge] = useState<KnowledgeCollection>({ assets: [], directory: "" });
  const [knowledgeLoading, setKnowledgeLoading] = useState(false);
  const [knowledgeError, setKnowledgeError] = useState("");
  const [roomsLoading, setRoomsLoading] = useState(false);
  const [messagesLoading, setMessagesLoading] = useState(false);
  const [messagesReady, setMessagesReady] = useState(false);
  const [messageError, setMessageError] = useState("");
  const [connection, setConnection] = useState<ConnectionState>("offline");
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [leftPanelVisible, setLeftPanelVisible] = useState(() => storedPanelState("agentcollab.left-panel"));
  const [rightPanelVisible, setRightPanelVisible] = useState(() => storedPanelState("agentcollab.right-panel"));
  const [theme, setTheme] = useState<Theme>(storedTheme);
  const [addAgentOpen, setAddAgentOpen] = useState(false);
  const [scanCompleted, setScanCompleted] = useState(false);
  const [scanning, setScanning] = useState(false);
  const [verifying, setVerifying] = useState(false);
  const [verificationError, setVerificationError] = useState("");
  const [inviteCode, setInviteCode] = useState(invitationCodeFromURL);
  const [joinNotice, setJoinNotice] = useState("");
	const [desktopOwnershipIssue, setDesktopOwnershipIssue] = useState<DesktopOwnershipIssue>();
	const [profileOpen, setProfileOpen] = useState(false);
	const [profileSection, setProfileSection] = useState<ProfileSection>("profile");
	const [hubAccountsOpen, setHubAccountsOpen] = useState(adminRequestedFromURL);
  const messageCursorRef = useRef(0);

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    document.documentElement.style.colorScheme = theme;
    window.localStorage.setItem("agentcollab.theme", theme);
  }, [theme]);

  useEffect(() => {
    if (session !== "authenticated" || workspaces.length > 0 || leftPanelVisible) return;
    window.localStorage.setItem("agentcollab.left-panel", "visible");
    setLeftPanelVisible(true);
  }, [session, workspaces.length, leftPanelVisible]);

  function clearInvitationURL() {
    const next = new URL(window.location.href);
    next.searchParams.delete("join");
    window.history.replaceState({}, "", `${next.pathname}${next.search}${next.hash}`);
    setInviteCode("");
  }

	const inspectDesktopOwnership = useCallback(async () => {
		const desktop = window.agentCollabDesktop;
		if (!desktop || !user) {
			setDesktopOwnershipIssue(undefined);
			return;
		}
		try {
			const state = await desktop.getState();
			if (!state.runnerRegistered || !state.runnerId) {
				setDesktopOwnershipIssue("unconnected");
				return;
			}
			const ownership = await api.runnerOwnership(state.runnerId);
			setDesktopOwnershipIssue(ownership.owned_by_current_user ? undefined : "other-account");
		} catch {
			// The Add Agent flow performs the same check and shows the actionable
			// recovery state. A transient Hub restart should not create a stale banner.
			setDesktopOwnershipIssue(undefined);
		}
	}, [user]);

	useEffect(() => {
		const desktop = window.agentCollabDesktop;
		if (session !== "authenticated" || !desktop || !user) return;
		void inspectDesktopOwnership();
		return desktop.onStatus(() => { void inspectDesktopOwnership(); });
	}, [inspectDesktopOwnership, session, user]);

  const acceptMessage = useCallback((incoming: Message) => {
    messageCursorRef.current = Math.max(messageCursorRef.current, incoming.sequence);
    setMessages((current) => mergeMessage(current, incoming));
  }, []);

  const acceptMessages = useCallback((incoming: Message[]) => {
    if (incoming.length === 0) return;
    messageCursorRef.current = incoming.reduce((highest, message) => Math.max(highest, message.sequence), messageCursorRef.current);
    setMessages((current) => incoming.reduce((next, message) => mergeMessage(next, message), current));
  }, []);

  const loadWorkspaces = useCallback(async () => {
    const next = await api.workspaces();
    setWorkspaces(next);
    setWorkspace((current) => next.find((item) => item.id === current?.id) ?? next[0]);
  }, []);

  useEffect(() => {
    let active = true;
    void api.me().then(async (currentUser) => {
      if (!active) return;
      setUser(currentUser);
      setSession("authenticated");
      if (inviteCode) {
        try {
          await api.acceptInvite(inviteCode);
          if (!active) return;
          clearInvitationURL();
          setJoinNotice("已使用你的账号加入受邀群聊。使用 AgentCollab Client 扫描后，即可接入你自己电脑上的 Agent。");
        } catch (reason) {
          if (!active) return;
          if (invitationAlreadyUsed(reason)) {
            clearInvitationURL();
            setJoinNotice(expiredInvitationLoginNotice);
          } else {
            setJoinNotice(invitationError(reason));
          }
        }
      }
      await loadWorkspaces();
    }).catch(() => {
      if (active) setSession("anonymous");
    });
    return () => { active = false; };
  }, [inviteCode, loadWorkspaces]);

  useEffect(() => {
    if (!workspace) {
      setRooms([]);
      setRoom(undefined);
		setDiscoveredAgents([]);
		setWorkspaceMembers([]);
      setScanCompleted(false);
      setWorkspaceAgents([]);
      setAgents([]);
      setTasks([]);
      setWorkItems([]);
      setProjectPlan(undefined);
      setProjectRepository(undefined);
      setWorkspaceKnowledge({ assets: [], directory: "" });
      setRoomKnowledge({ assets: [], directory: "" });
      messageCursorRef.current = 0;
      return;
    }
    let active = true;
    setRoomsLoading(true);
	void Promise.all([api.rooms(workspace.id), api.agents(workspace.id), api.workspaceMembers(workspace.id), api.projectPlan(workspace.id), api.projectRepository(workspace.id).catch(() => undefined)]).then(([next, registeredAgents, nextMembers, nextProjectPlan, nextProjectRepository]) => {
      if (!active) return;
      setRooms(next);
		setWorkspaceAgents(registeredAgents);
		setWorkspaceMembers(nextMembers);
		setProjectPlan(nextProjectPlan);
		setProjectRepository(nextProjectRepository);
      setDiscoveredAgents([]);
      setScanCompleted(false);
      setRoom((current) => next.find((item) => item.id === current?.id) ?? next[0]);
    }).catch(() => {
      if (active) {
        setRooms([]);
        setRoom(undefined);
      }
    }).finally(() => {
      if (active) setRoomsLoading(false);
    });
    return () => { active = false; };
  }, [workspace]);

  useEffect(() => {
    if (!workspace) {
      setWorkspaceKnowledge({ assets: [], directory: "" });
      return;
    }
    let active = true;
    void api.workspaceKnowledge(workspace.id).then((collection) => {
      if (active) setWorkspaceKnowledge(collection);
    }).catch(() => {
      if (active) setWorkspaceKnowledge({ assets: [], directory: "" });
    });
    return () => { active = false; };
  }, [workspace?.id]);

  useEffect(() => {
    if (!room) {
      setMessages([]);
      setMembers([]);
      setAgents([]);
      setTasks([]);
      setWorkItems([]);
      setRoomKnowledge({ assets: [], directory: "" });
      messageCursorRef.current = 0;
      setMessagesReady(false);
      return;
    }
    let active = true;
    setMessagesLoading(true);
    setMessagesReady(false);
    setMessageError("");
    Promise.all([api.messages(room.id), api.members(room.id), api.roomAgents(room.id), api.tasks(room.id), api.workItems(room.id)]).then(([nextMessages, nextMembers, nextAgents, nextTasks, nextWorkItems]) => {
      if (!active) return;
      setMessages(nextMessages);
      messageCursorRef.current = nextMessages.reduce((highest, message) => Math.max(highest, message.sequence), 0);
      setMembers(nextMembers);
      setAgents(nextAgents);
      setTasks(nextTasks);
      setWorkItems(nextWorkItems);
      setMessagesReady(true);
    }).catch((reason) => {
      if (!active) return;
      setMessageError(reason instanceof Error ? reason.message : "无法读取群聊");
    }).finally(() => {
      if (active) setMessagesLoading(false);
    });
    return () => { active = false; };
  }, [room]);

  useEffect(() => {
    if (!room) {
      setRoomKnowledge({ assets: [], directory: "" });
      return;
    }
    let active = true;
    setKnowledgeLoading(true);
    setKnowledgeError("");
    void api.roomKnowledge(room.id).then((collection) => {
      if (active) setRoomKnowledge(collection);
    }).catch((reason) => {
      if (!active) return;
      setRoomKnowledge({ assets: [], directory: "" });
      setKnowledgeError(reason instanceof Error ? reason.message : "无法读取知识库");
    }).finally(() => {
      if (active) setKnowledgeLoading(false);
    });
    return () => { active = false; };
  }, [room?.id]);

  useEffect(() => {
    if (!room) return;
    let active = true;
    let timer: number | undefined;
    const refresh = async () => {
      try {
        const [next, nextWorkItems, missedMessages] = await Promise.all([api.tasks(room.id), api.workItems(room.id), api.messages(room.id, messageCursorRef.current)]);
        if (!active) return;
        setTasks(next);
        setWorkItems(nextWorkItems);
        acceptMessages(missedMessages);
        timer = window.setTimeout(refresh, next.some(taskIsActive) ? 1_000 : 5_000);
      } catch {
        if (active) timer = window.setTimeout(refresh, 5_000);
      }
    };
    timer = window.setTimeout(refresh, 1_000);
    return () => {
      active = false;
      if (timer !== undefined) window.clearTimeout(timer);
    };
  }, [room, acceptMessages]);

	// Agent online state is derived from a short Runner heartbeat window. Keep the
	// visible library and room member status fresh even when the user does not
	// open the inspector or manually press refresh after a Hub restart.
	useEffect(() => {
		if (!workspace) return;
		let active = true;
		let timer: number | undefined;
		const refreshAgentPresence = async () => {
			try {
				const [nextWorkspaceAgents, nextRoomAgents] = await Promise.all([
					api.agents(workspace.id),
					room ? api.roomAgents(room.id) : Promise.resolve<Agent[] | undefined>(undefined),
				]);
				if (!active) return;
				setWorkspaceAgents(nextWorkspaceAgents);
				if (nextRoomAgents) setAgents(nextRoomAgents);
			} catch {
				// A transient Hub restart must not leave an unhandled rejection or stop
				// presence reconciliation. The next short poll will recover it.
			} finally {
				if (active) timer = window.setTimeout(refreshAgentPresence, 5_000);
			}
		};
		void refreshAgentPresence();
		return () => {
			active = false;
			if (timer !== undefined) window.clearTimeout(timer);
		};
	}, [workspace?.id, room?.id]);

  useEffect(() => {
    if (!room || !messagesReady) return;
    const after = messages.reduce((highest, message) => Math.max(highest, message.sequence), 0);
    return connectRoomEvents({
      roomID: room.id,
      after,
      onMessage: acceptMessage,
      onState: setConnection,
    });
    // The initial sequence is intentionally captured once after REST hydration.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [room?.id, messagesReady, acceptMessage]);

  async function login(username: string, password: string) {
    try {
      const currentUser = await api.login(username, password);
      if (inviteCode) {
        try {
          await api.acceptInvite(inviteCode);
          clearInvitationURL();
          setJoinNotice("已使用你的账号加入受邀群聊。接下来可接入你自己的电脑和 Agent。");
        } catch (reason) {
          if (invitationAlreadyUsed(reason)) {
            clearInvitationURL();
            setJoinNotice(expiredInvitationLoginNotice);
          } else {
            await api.logout().catch(() => undefined);
            throw new Error(invitationError(reason));
          }
        }
      }
      setUser(currentUser);
      setSession("authenticated");
      await loadWorkspaces();
    } catch (reason) {
      if (reason instanceof ApiError && reason.status === 401) throw new Error("用户名或密码不正确");
      throw reason;
    }
  }

  async function register(input: { username: string; display_name: string; password: string }) {
    try {
      if (inviteCode) {
        const result = await api.registerWithInvite(inviteCode, input);
        clearInvitationURL();
        setUser(result.user);
        setJoinNotice("账号已创建并加入群聊。你能看到共享对话、事项看板和 Agent 交付；需要接入本机 Agent 时，在 Client 中点“连接本机”即可。");
      } else {
        const currentUser = await api.register(input);
        setUser(currentUser);
        setJoinNotice("账号已创建。你可以新建自己的工作区，或通过邀请链接加入其他人的协作网络。");
      }
      setSession("authenticated");
      await loadWorkspaces();
    } catch (reason) {
      throw new Error(invitationError(reason));
    }
  }

	async function updateProfile(input: Pick<User, "display_name" | "avatar_url" | "bio" | "current_focus">) {
		const updated = await api.updateProfile(input);
		setUser(updated);
		setMembers((current) => current.map((member) => member.id === updated.id ? { ...member, ...updated } : member));
		setWorkspaceMembers((current) => current.map((member) => member.id === updated.id ? { ...member, ...updated } : member));
	}

	const loadHubUsers = useCallback(() => api.hubUsers(), []);
	const updateHubUserRole = useCallback((userID: string, role: User["role"]) => api.updateHubUserRole(userID, role), []);
	const archiveHubUser = useCallback((userID: string) => api.archiveHubUser(userID), []);

  async function logout() {
    try { await api.logout(); } finally {
      setUser(undefined);
      setSession("anonymous");
      setWorkspaces([]);
      setWorkspace(undefined);
      setRoom(undefined);
    }
  }

  async function createWorkspace(name: string) {
    const created = await api.createWorkspace(name);
    setWorkspaces((current) => [...current, created]);
    setWorkspace(created);
  }

  async function updateWorkspace(workspaceID: string, name: string) {
    const updated = await api.updateWorkspace(workspaceID, name);
    setWorkspaces((current) => current.map((item) => item.id === updated.id ? updated : item));
    setWorkspace((current) => current?.id === updated.id ? updated : current);
  }

  async function deleteWorkspace(workspaceID: string) {
    await api.deleteWorkspace(workspaceID);
    const next = await api.workspaces();
    setWorkspaces(next);
    setWorkspace((current) => current?.id === workspaceID ? next[0] : current);
  }

  async function createRoom(name: string, agentIDs: string[]) {
    if (!workspace) return;
    const created = await api.createRoom(workspace.id, name);
    for (const agentID of agentIDs) await api.addRoomAgent(created.id, agentID);
    setRooms((current) => [...current, created]);
    setRoom(created);
  }

  async function joinHub(inviteLink: string) {
	const withScheme = /^[a-z][a-z0-9+.-]*:\/\//i.test(inviteLink) ? inviteLink : `http://${inviteLink}`;
	let target: URL;
	try {
	  target = new URL(withScheme);
	} catch {
	  throw new Error("邀请链接格式不正确");
	}
	if (target.protocol !== "http:" && target.protocol !== "https:") throw new Error("邀请链接必须使用 http 或 https");
	if (!target.searchParams.get("join")?.trim()) throw new Error("这不是完整的 AgentCollab 邀请链接");
	if (window.agentCollabDesktop?.openTeamLink) {
	  await window.agentCollabDesktop.openTeamLink(target.toString());
	  return;
	}
	window.location.assign(target.toString());
  }

  async function setRoomPinned(target: Room, pinned: boolean) {
    const updated = await api.setRoomPinned(target.id, pinned);
    setRooms((current) => current.map((item) => item.id === updated.id ? updated : item));
    setRoom((current) => current?.id === updated.id ? updated : current);
  }

  async function sendMessage(body: string, mentionAgentIDs: string[], files: File[], thread?: { reply_to_message_id: string; root_message_id: string }, planStepID?: string, executionSource?: TaskExecutionSource) {
    if (!room) throw new Error("请先选择群聊");
    const attachmentIDs: string[] = [];
    for (const file of files) attachmentIDs.push((await api.uploadAttachment(room.id, file)).id);
    const command = parseConversationCommand(body);
    if (command) {
      const created = await api.sendMessage(room.id, command.raw, [], attachmentIDs);
      acceptMessage(created);
	  setTasks(await api.tasks(room.id));
	  setWorkItems(await api.workItems(room.id));
	  if (workspace) setProjectPlan(await api.projectPlan(workspace.id));
      return created;
    }
    const structuredMentions = mentionAgentIDs.length > 0 ? mentionAgentIDs : agents
	  .filter((agent) => body.toLocaleLowerCase().includes(`@${agent.display_name.toLocaleLowerCase()}`))
	  .map((agent) => agent.id);
    const created = await api.sendMessage(room.id, body, structuredMentions, attachmentIDs, thread, planStepID, planStepID ? executionSource : undefined);
    acceptMessage(created);
	setTasks(await api.tasks(room.id));
    setWorkItems(await api.workItems(room.id));
	if (workspace) setProjectPlan(await api.projectPlan(workspace.id));
    return created;
  }

  async function createTask(input: { target_agent_id: string; title: string; prompt: string; acceptance_criteria: string; plan_step_id?: string; execution_source?: TaskExecutionSource }) {
    if (!room) throw new Error("请先选择群聊");
    const created = await api.createTask(room.id, input);
    acceptMessage(created.message);
    setTasks((current) => mergeTask(current, created.task));
    setWorkItems(await api.workItems(room.id));
	if (workspace) setProjectPlan(await api.projectPlan(workspace.id));
    return created.task;
  }

  async function uploadKnowledge(scope: "workspace" | "room", file: File) {
    if (!workspace || !room) throw new Error("请先选择工作区和群聊");
    setKnowledgeError("");
    if (scope === "workspace") {
      await api.uploadWorkspaceKnowledge(workspace.id, file);
      setWorkspaceKnowledge(await api.workspaceKnowledge(workspace.id));
    } else {
      await api.uploadRoomKnowledge(room.id, file);
      setRoomKnowledge(await api.roomKnowledge(room.id));
    }
  }

  async function deleteKnowledge(scope: "workspace" | "room", assetID: string) {
    if (!workspace || !room) return;
    setKnowledgeError("");
    if (scope === "workspace") {
      await api.deleteWorkspaceKnowledge(workspace.id, assetID);
      setWorkspaceKnowledge(await api.workspaceKnowledge(workspace.id));
    } else {
      await api.deleteRoomKnowledge(room.id, assetID);
      setRoomKnowledge(await api.roomKnowledge(room.id));
    }
  }

  async function cancelTaskRoot(rootMessageID: string) {
    if (!room) throw new Error("请先选择群聊");
    const cancelled = await api.cancelTaskRoot(room.id, rootMessageID);
    setTasks((current) => cancelled.tasks.reduce((next, task) => mergeTask(next, task), current));
    if (cancelled.message?.id) acceptMessage(cancelled.message);
    setWorkItems(await api.workItems(room.id));
  }

  async function updateWorkItem(itemID: string, patch: { status: WorkItemStatus }) {
    const updated = await api.updateWorkItem(itemID, patch);
    setWorkItems((current) => mergeWorkItem(current, updated));
  }

  async function reviewWorkItem(itemID: string) {
    return api.reviewWorkItem(itemID);
  }

  async function mergeReviewedWorkItem(itemID: string) {
    const updated = await api.mergeWorkItem(itemID);
    setWorkItems((current) => mergeWorkItem(current, updated));
    if (workspace) setProjectPlan(await api.projectPlan(workspace.id));
  }

  async function startWorkItemAIReview(itemID: string, reviewAgentID: string, verificationAgentID: string) {
    const updated = await api.startWorkItemAIReview(itemID, reviewAgentID, verificationAgentID);
    setWorkItems((current) => mergeWorkItem(current, updated));
    if (room) setTasks(await api.tasks(room.id));
  }

  async function rejectWorkItem(itemID: string, reason: string) {
    const updated = await api.rejectWorkItem(itemID, reason);
    setWorkItems((current) => mergeWorkItem(current, updated));
  }

  async function submitWorkItemWorkspace(itemID: string, files: File[]) {
    const updated = await api.submitWorkItemWorkspace(itemID, files);
    setWorkItems((current) => mergeWorkItem(current, updated));
  }

  async function updateProjectPlan(patch: Partial<{ objective: string; status: ProjectPlanStatus }>) {
    if (!workspace) throw new Error("请先选择工作区");
    setProjectPlan(await api.updateProjectPlan(workspace.id, patch));
  }

  async function createProjectStep(input: { parent_step_id?: string; title: string; description?: string; weight: number; status?: ProjectStepStatus; sort_order?: number }) {
    if (!workspace) throw new Error("请先选择工作区");
    await api.createProjectStep(workspace.id, input);
    setProjectPlan(await api.projectPlan(workspace.id));
  }

  async function updateProjectStep(stepID: string, patch: Partial<{ parent_step_id: string; clear_parent: boolean; title: string; description: string; weight: number; status: ProjectStepStatus; sort_order: number; responsible_user_ids: string[]; responsible_agent_ids: string[] }>) {
    await api.updateProjectStep(stepID, patch);
    if (workspace) setProjectPlan(await api.projectPlan(workspace.id));
  }

  async function deleteProjectStep(stepID: string) {
    await api.deleteProjectStep(stepID);
    if (workspace) setProjectPlan(await api.projectPlan(workspace.id));
  }

  async function uploadProjectStepWorkspace(stepID: string, files: File[]) {
    await api.uploadProjectStepWorkspace(stepID, files);
    if (workspace) setProjectPlan(await api.projectPlan(workspace.id));
  }

  async function removeProjectStepWorkspace(stepID: string) {
	await api.removeProjectStepWorkspace(stepID);
	if (workspace) setProjectPlan(await api.projectPlan(workspace.id));
  }

  async function bindProjectStepLocal(stepID: string) {
	if (!workspace || !window.agentCollabDesktop?.bindProjectStep || !window.agentCollabDesktop.unbindProjectStep) throw new Error("请更新并使用最新版 AgentCollab Client 绑定本机目录");
	const state = await window.agentCollabDesktop.getState();
	if (!state.runnerRegistered || !state.runnerId || !state.deviceId) throw new Error("请先在 Client 中连接这台电脑");
	const local = await window.agentCollabDesktop.bindProjectStep({ stepId: stepID, workspaceId: workspace.id });
	if (!local) return;
	try {
	  await api.saveProjectStepLocalBinding(stepID, { runner_id: state.runnerId, device_id: state.deviceId, directory_label: local.directoryLabel });
	} catch (error) {
	  await window.agentCollabDesktop.unbindProjectStep({ stepId: stepID }).catch(() => false);
	  throw error;
	}
	setProjectPlan(await api.projectPlan(workspace.id));
  }

  async function removeProjectStepLocal(stepID: string) {
	if (!workspace || !window.agentCollabDesktop?.unbindProjectStep) throw new Error("请更新并使用最新版 AgentCollab Client 解除本机目录");
	const state = await window.agentCollabDesktop.getState();
	if (!state.runnerId) throw new Error("这台电脑尚未连接");
	await api.removeProjectStepLocalBinding(stepID, state.runnerId);
	await window.agentCollabDesktop.unbindProjectStep({ stepId: stepID });
	setProjectPlan(await api.projectPlan(workspace.id));
  }

  async function openProjectStepLocal(stepID: string) {
	if (!window.agentCollabDesktop?.openProjectStep) throw new Error("请更新并使用最新版 AgentCollab Client 打开本机目录");
	await window.agentCollabDesktop.openProjectStep({ stepId: stepID });
  }

  async function updateRoomProjectStep(planStepID?: string) {
    if (!room) throw new Error("请先选择群聊");
    await api.updateRoomProjectStep(room.id, planStepID);
    const next = { ...room, plan_step_id: planStepID };
    setRoom(next);
    setRooms((current) => current.map((item) => item.id === next.id ? next : item));
  }

  async function registerAgents(agentIDs: string[]) {
    if (!workspace) return;
    for (const agentID of agentIDs) {
      await api.importAgent(workspace.id, agentID);
    }
    setDiscoveredAgents(await api.agentCandidates(workspace.id));
    setWorkspaceAgents(await api.agents(workspace.id));
  }

  async function addRoomAgent(agentID: string) {
    if (!room) return;
    await api.addRoomAgent(room.id, agentID);
    setAgents(await api.roomAgents(room.id));
  }

  async function removeRoomAgent(agentID: string) {
    if (!room) return;
    await api.removeRoomAgent(room.id, agentID);
    setAgents(await api.roomAgents(room.id));
  }

  async function removeWorkspaceAgent(agentID: string) {
    if (!workspace) return;
    await api.removeAgent(workspace.id, agentID);
    const [registered, candidates] = await Promise.all([api.agents(workspace.id), api.agentCandidates(workspace.id)]);
    setWorkspaceAgents(registered);
    setDiscoveredAgents(candidates);
    if (room) setAgents(await api.roomAgents(room.id));
  }

  async function createRunnerInvite(): Promise<RunnerInvite> {
    if (!workspace) throw new Error("请先选择工作区");
    return api.createRunnerInvite(workspace.id);
  }

	async function refreshAgentStatus() {
		if (!workspace) return;
		const [nextWorkspaceAgents, nextRoomAgents] = await Promise.all([
			api.agents(workspace.id),
			room ? api.roomAgents(room.id) : Promise.resolve<Agent[] | undefined>(undefined),
		]);
		setWorkspaceAgents(nextWorkspaceAgents);
		if (nextRoomAgents) setAgents(nextRoomAgents);
	}

  async function createHumanInvite(): Promise<HumanInvite> {
    if (!workspace || !room) throw new Error("请先选择工作区和群聊");
    return api.createHumanInvite(workspace.id, room.id);
  }

  function togglePanel(side: "left" | "right") {
    if (side === "left") {
      setLeftPanelVisible((current) => {
        window.localStorage.setItem("agentcollab.left-panel", current ? "hidden" : "visible");
        return !current;
      });
      return;
    }
    setRightPanelVisible((current) => {
      window.localStorage.setItem("agentcollab.right-panel", current ? "hidden" : "visible");
      return !current;
    });
  }

	async function updateRoomAgentSettings(agentID: string, patch: Partial<Pick<Agent, "response_mode" | "role_prompt">>) {
	if (!room) return;
	const agent = agents.find((item) => item.id === agentID);
	if (!agent) return;
	await api.updateRoomAgentSettings(room.id, agentID, {
	  response_mode: patch.response_mode ?? agent.response_mode,
	  reply_delay_seconds: agent.reply_delay_seconds,
	  role_prompt: patch.role_prompt ?? agent.role_prompt,
	  max_auto_turns: agent.max_auto_turns,
	});
	setAgents(await api.roomAgents(room.id));
	}

	async function updateAgentPolicy(agentID: string, policy: { visibility: "private" | "workspace"; invocation_scope: "owner_only" | "room_members"; shared_skills: string[] }) {
		if (!workspace) return;
		await api.updateAgentPolicy(workspace.id, agentID, policy);
		const [nextWorkspaceAgents, nextCandidates] = await Promise.all([api.agents(workspace.id), api.agentCandidates(workspace.id)]);
		setWorkspaceAgents(nextWorkspaceAgents);
		setDiscoveredAgents(nextCandidates);
		if (room) setAgents(await api.roomAgents(room.id));
	}

	async function updateWorkspaceMemberRole(userID: string, role: "admin" | "member") {
		if (!workspace) return;
		await api.updateWorkspaceMember(workspace.id, userID, role);
		setWorkspaceMembers(await api.workspaceMembers(workspace.id));
		if (room) setMembers(await api.members(room.id));
	}

	async function addRoomMember(userID: string) {
		if (!room) return;
		await api.addRoomMember(room.id, userID);
		setMembers(await api.members(room.id));
	}

	async function updateRoomMemberRole(userID: string, role: "admin" | "member") {
		if (!room) return;
		await api.updateRoomMember(room.id, userID, role);
		setMembers(await api.members(room.id));
	}

	async function removeRoomMember(userID: string) {
		if (!room) return;
		await api.removeRoomMember(room.id, userID);
		const [nextRoomMembers, nextRoomAgents] = await Promise.all([api.members(room.id), api.roomAgents(room.id)]);
		setMembers(nextRoomMembers);
		setAgents(nextRoomAgents);
	}

	async function removeWorkspaceMember(userID: string) {
		if (!workspace) return;
		await api.removeWorkspaceMember(workspace.id, userID);
		const [nextMembers, nextAgents] = await Promise.all([api.workspaceMembers(workspace.id), api.agents(workspace.id)]);
		setWorkspaceMembers(nextMembers);
		setWorkspaceAgents(nextAgents);
		if (room) {
			const [nextRoomMembers, nextRoomAgents] = await Promise.all([api.members(room.id), api.roomAgents(room.id)]);
			setMembers(nextRoomMembers);
			setAgents(nextRoomAgents);
		}
	}

  async function scanAgents() {
    if (!workspace || scanning) return;
    setScanning(true);
    try {
      setDiscoveredAgents(await api.agentCandidates(workspace.id));
      setScanCompleted(true);
    } finally {
      setScanning(false);
    }
  }

	function openAgentDialog() {
		setDiscoveredAgents([]);
		setScanCompleted(false);
		setAddAgentOpen(true);
	}

  async function verifyLocalAgents() {
    if (!room || agents.length < 2 || verifying) return;
    setVerifying(true);
    setVerificationError("");
    try {
      const { created, failures } = await dispatchVerification(agents, (agent) => {
        return api.createTask(room.id, {
          target_agent_id: agent.id,
          title: `${agent.display_name} 本地身份验证`,
          prompt: `不要调用工具，只回复 LOCAL_${agent.type.toUpperCase()}_OK`,
          acceptance_criteria: `只返回 LOCAL_${agent.type.toUpperCase()}_OK`,
        });
      });
      acceptMessages(created.map((item) => item.message));
      setTasks((current) => created.reduce((next, item) => mergeTask(next, item.task), current));
      const dispatchErrors = failures.map(({ agentName, reason }) => `${agentName}: ${reason instanceof Error ? reason.message : t("验证任务派发失败")}`);
      if (dispatchErrors.length) setVerificationError(dispatchErrors.join("\n"));
      if (created.length) {
        try {
          setWorkItems(await api.workItems(room.id));
        } catch {
          setVerificationError([...dispatchErrors, t("任务已派发，但任务列表刷新失败；请刷新页面查看，勿重复提交。")].join("\n"));
        }
      }
    } catch (reason) {
      setVerificationError(reason instanceof Error ? reason.message : t("验证任务派发失败"));
    } finally {
      setVerifying(false);
    }
  }

  if (session === "loading") {
    return <main className="app-loading" aria-label="正在连接 Hub"><div className="brand-mark">AC</div><span /></main>;
  }
  if (session === "anonymous" || !user) return <LoginPage inviteCode={inviteCode} onLogin={login} onRegister={register} />;

  return (
    <div className="app-layout">
      {joinNotice && <div className="join-notice" role="status"><span>{t(joinNotice)}</span><button type="button" onClick={() => setJoinNotice("")} aria-label={t("关闭加入提示")}>×</button></div>}
	  {desktopOwnershipIssue && <div className={`desktop-account-warning${joinNotice ? " below-join-notice" : ""}`} role="status"><span><strong>{t(desktopOwnershipIssue === "other-account" ? "Client 与当前登录账号不一致" : "这台电脑还没有连接到当前账号")}</strong><small>{t(desktopOwnershipIssue === "other-account" ? "当前页面登录 @{username}，但本机 Agent 仍绑定在另一个账号。" : "当前页面登录 @{username}，连接后才能扫描本机 Agent。").replace("{username}", user.username)}</small></span><button type="button" onClick={openAgentDialog}>{t("处理")}</button></div>}
      <div className={`app-shell${leftPanelVisible ? "" : " left-panel-hidden"}${rightPanelVisible ? "" : " right-panel-hidden"}`}>
      <WorkspaceSidebar
        open={sidebarOpen}
        currentUser={user}
		surface={window.agentCollabDesktop ? "client" : "web"}
        workspaces={workspaces}
        selectedWorkspace={workspace}
        rooms={rooms}
        agents={workspaceAgents}
        selectedRoom={room}
        loading={roomsLoading}
        onSelectWorkspace={(next) => { setWorkspace(next); setSidebarOpen(false); }}
        onSelectRoom={(next) => { setRoom(next); setSidebarOpen(false); }}
        onCreateWorkspace={createWorkspace}
        onUpdateWorkspace={updateWorkspace}
        onDeleteWorkspace={deleteWorkspace}
        onCreateRoom={createRoom}
        onJoinHub={joinHub}
        onSetRoomPinned={setRoomPinned}
        theme={theme}
        onToggleTheme={() => setTheme((current) => current === "light" ? "dark" : "light")}
		  onOpenProfile={() => { setProfileSection("profile"); setProfileOpen(true); }}
        onManageHub={() => setHubAccountsOpen(true)}
        onLogout={logout}
        onClose={() => setSidebarOpen(false)}
      />
      <RoomView
        room={room}
        currentUser={user}
        members={members}
        workspaceMembers={workspaceMembers}
        agents={agents}
        workspaceAgents={workspaceAgents}
        registeredAgentCount={workspaceAgents.length}
        messages={messages}
        tasks={tasks}
        workItems={workItems}
        projectPlan={projectPlan}
        projectRepository={projectRepository}
        workspaceKnowledge={workspaceKnowledge}
        roomKnowledge={roomKnowledge}
        knowledgeLoading={knowledgeLoading}
        knowledgeError={knowledgeError}
        loading={messagesLoading}
        error={messageError}
        connection={connection}
        onSend={sendMessage}
        onCreateTask={createTask}
        onCancelTaskRoot={cancelTaskRoot}
        onUpdateWorkItem={updateWorkItem}
        onReviewWorkItem={reviewWorkItem}
        onMergeWorkItem={mergeReviewedWorkItem}
        onStartWorkItemAIReview={startWorkItemAIReview}
        onRejectWorkItem={rejectWorkItem}
        onSubmitWorkItemWorkspace={submitWorkItemWorkspace}
        onUpdateProjectPlan={updateProjectPlan}
        onCreateProjectStep={createProjectStep}
        onUpdateProjectStep={updateProjectStep}
        onDeleteProjectStep={deleteProjectStep}
		onUploadProjectStepWorkspace={uploadProjectStepWorkspace}
		onRemoveProjectStepWorkspace={removeProjectStepWorkspace}
		onBindProjectStepLocal={bindProjectStepLocal}
		onRemoveProjectStepLocal={removeProjectStepLocal}
		onOpenProjectStepLocal={openProjectStepLocal}
        onUploadKnowledge={uploadKnowledge}
        onDeleteKnowledge={deleteKnowledge}
		onManageAgents={openAgentDialog}
        onVerifyAgents={verifyLocalAgents}
        verifying={verifying}
        verificationError={verificationError}
        onShowSidebar={() => setSidebarOpen(true)}
        onShowDetails={() => setDetailsOpen(true)}
        hasWorkspaces={workspaces.length > 0}
        onJoinWorkspace={joinHub}
        leftPanelVisible={leftPanelVisible}
        rightPanelVisible={rightPanelVisible}
        onToggleLeftPanel={() => togglePanel("left")}
        onToggleRightPanel={() => togglePanel("right")}
      />
		  <RoomInspector open={detailsOpen} room={room} workspace={workspace} projectPlan={projectPlan} currentUser={user} members={members} workspaceMembers={workspaceMembers} agents={agents} workspaceAgents={workspaceAgents} verifying={verifying} onRegisterAgent={openAgentDialog} onJoinWorkspace={joinHub} onCreateHumanInvite={createHumanInvite} onAddRoomMember={addRoomMember} onUpdateRoomMemberRole={updateRoomMemberRole} onRemoveRoomMember={removeRoomMember} onAddRoomAgent={addRoomAgent} onRemoveRoomAgent={removeRoomAgent} onRemoveWorkspaceAgent={removeWorkspaceAgent} onUpdateAgentSettings={updateRoomAgentSettings} onUpdateAgentPolicy={updateAgentPolicy} onManageAgentSettings={() => { setProfileSection("agents"); setProfileOpen(true); }} onUpdateWorkspaceMemberRole={updateWorkspaceMemberRole} onRemoveWorkspaceMember={removeWorkspaceMember} onUpdateRoomProjectStep={updateRoomProjectStep} onVerifyAgents={verifyLocalAgents} onClose={() => setDetailsOpen(false)} />
      <AddAgentDialog open={addAgentOpen} discovered={discoveredAgents} registered={workspaceAgents} scanned={scanCompleted} scanning={scanning} onScan={scanAgents} onCreateRunnerInvite={createRunnerInvite} onClose={() => { setAddAgentOpen(false); void inspectDesktopOwnership(); }} onRegister={registerAgents} />
		<ProfileDialog open={profileOpen} user={user} agents={workspaceAgents} initialSection={profileSection} onClose={() => setProfileOpen(false)} onSave={updateProfile} onUpdateAgentPolicy={updateAgentPolicy} onManageAgents={() => { setProfileOpen(false); openAgentDialog(); }} canManageHub={user.role === "admin"} onManageHub={() => { setProfileOpen(false); setHubAccountsOpen(true); }} />
		<HubAccountsDialog open={hubAccountsOpen} currentUser={user} onClose={() => setHubAccountsOpen(false)} onLoad={loadHubUsers} onUpdateRole={updateHubUserRole} onArchiveAccount={archiveHubUser} />
      </div>
    </div>
  );
}
