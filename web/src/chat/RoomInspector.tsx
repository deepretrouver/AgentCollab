import { type FormEvent, useEffect, useId, useState } from "react";
import { createPortal } from "react-dom";
import {
  CaretDown,
  ChatCircleDots,
  CopySimple,
  GitBranch,
  Plus,
  Question,
  Robot,
  ShieldCheck,
  SignIn,
  SlidersHorizontal,
  TestTube,
  Trash,
  UserCircle,
  UserPlus,
  X,
} from "@phosphor-icons/react";
import { ApiError, type Agent, type HumanInvite, type ProjectPlan, type Room, type RunnerInvite, type User, type Workspace } from "../api/client";
import { agentDisplayName, agentShortName, groupAgentsByOwner } from "../agents/presentation";
import { useLocale } from "../i18n/locale";

interface RoomInspectorProps {
  open: boolean;
	room?: Room;
	workspace?: Workspace;
  projectPlan?: ProjectPlan;
  currentUser?: User;
  members: User[];
	workspaceMembers?: User[];
  agents: Agent[];
  workspaceAgents: Agent[];
  verifying: boolean;
  onRegisterAgent: () => void;
  onJoinWorkspace?: (inviteLink: string) => Promise<void>;
  onCreateHumanInvite: () => Promise<HumanInvite>;
  // Deprecated compatibility hook for older embedded hosts. Device enrollment
  // is intentionally not rendered inside a room inspector anymore.
  onCreateRunnerInvite?: () => Promise<RunnerInvite>;
  onAddRoomAgent: (agentID: string) => Promise<void>;
  onRemoveRoomAgent: (agentID: string) => Promise<void>;
  onRemoveWorkspaceAgent: (agentID: string) => Promise<void>;
	onUpdateAgentSettings: (agentID: string, patch: Partial<Pick<Agent, "response_mode" | "role_prompt">>) => Promise<void>;
	onUpdateAgentPolicy?: (agentID: string, policy: { visibility: "private" | "workspace"; invocation_scope: "owner_only" | "room_members"; shared_skills: string[] }) => Promise<void>;
	onManageAgentSettings?: () => void;
	onAddRoomMember?: (userID: string) => Promise<void>;
	onUpdateRoomMemberRole?: (userID: string, role: "admin" | "member") => Promise<void>;
	onRemoveRoomMember?: (userID: string) => Promise<void>;
	onUpdateWorkspaceMemberRole?: (userID: string, role: "admin" | "member") => Promise<void>;
  onRemoveWorkspaceMember?: (userID: string) => Promise<void>;
  onUpdateRoomProjectStep?: (planStepID?: string) => Promise<void>;
  onVerifyAgents: () => Promise<void>;
  // Desktop panel visibility now lives in RoomView's single “视图” menu.
  // Kept optional so older embedded hosts do not break.
  onTogglePanel?: () => void;
  onClose: () => void;
}

type InspectorSection = "members" | "library" | "verification";

function initialCollapsed(): Record<InspectorSection, boolean> {
  try {
    const saved = JSON.parse(window.localStorage.getItem("agentcollab.inspector-collapsed") ?? "{}") as Partial<Record<InspectorSection, boolean>>;
    return {
      members: typeof saved.members === "boolean" ? saved.members : false,
      library: typeof saved.library === "boolean" ? saved.library : false,
      verification: typeof saved.verification === "boolean" ? saved.verification : true,
    };
  } catch {
    return { members: false, library: false, verification: true };
  }
}

function initialCollapsedMembers(): Record<string, boolean> {
  try {
    return JSON.parse(window.localStorage.getItem("agentcollab.member-tree-collapsed-v2") ?? "{}") as Record<string, boolean>;
  } catch {
    return {};
  }
}

function initialCollapsedLibraryOwners(): Record<string, boolean> {
  try {
    return JSON.parse(window.localStorage.getItem("agentcollab.agent-library-owner-collapsed-v2") ?? "{}") as Record<string, boolean>;
  } catch {
    return {};
  }
}

function actionError(reason: unknown, fallback: string) {
	if (reason instanceof ApiError && reason.status === 409) {
		if (fallback.includes("响应方式")) return "当前群已有主协调；请由那位 Agent 的主人先取消主协调设置。";
		if (fallback.includes("成员")) return "该成员的 Agent 仍有任务正在执行，任务结束后再移出。";
		if (fallback.includes("删除") || fallback.includes("移出")) return "Agent 还有排队或执行中的任务，任务结束后才能删除或移出。";
	}
	return reason instanceof Error ? reason.message : fallback;
}

function supportsScopedClient(version = "") {
	const normalized = version.trim().toLowerCase();
	if (normalized === "dev" || normalized === "development") return true;
	const match = normalized.match(/^v?(\d+)\.(\d+)/);
	return Boolean(match && (Number(match[1]) > 0 || Number(match[2]) >= 2));
}

function agentVisibilityLabel(agent: Agent) {
	return agent.visibility === "private" ? "仅自己可见" : "工作区可见";
}

function agentInvocationLabel(agent: Agent) {
	return agent.invocation_scope === "room_members" ? "群成员可调用" : "仅自己调用";
}

function agentSkillShareLabel(agent: Agent, ownerView: boolean, t: (key: string) => string) {
	const shared = agent.shared_skills?.length ?? 0;
	return ownerView ? t("开放 {0}/{1} Skills").replace("{0}", String(shared)).replace("{1}", String(agent.skills?.length ?? 0)) : t("可调用 {0} Skills").replace("{0}", String(shared));
}

function agentGroupSummary(agentList: Agent[], t: (key: string) => string) {
	if (agentList.length === 0) return t("未接入 Agent");
	const available = agentList.filter((agent) => agent.status === "online" || agent.status === "busy").length;
	return t(available > 0 ? "{0} 个 Agent，{1} 个在线" : "{0} 个 Agent，均离线").replace("{0}", String(agentList.length)).replace("{1}", String(available));
}

function ownerGroupCollapsed(saved: Record<string, boolean>, ownerID: string, agentCount: number) {
	if (agentCount === 0) return false;
	return saved[ownerID] ?? agentCount > 1;
}

function agentResponseLabel(agent: Agent) {
  switch (agent.response_mode) {
    case "coordinator": return "主协调";
    case "human_all": return "回复真人";
    case "muted": return "静默";
    default: return "仅 @";
  }
}

function CompactHelp({ label, details }: { label: string; details: string }) {
  const tooltipID = useId();
  const [position, setPosition] = useState<{ left: number; top: number; width: number; above: boolean }>();
  function show(anchor: HTMLElement) {
    const rect = anchor.getBoundingClientRect();
    const width = Math.min(320, window.innerWidth - 24);
    const left = Math.max(12, Math.min(rect.left + rect.width / 2 - width / 2, window.innerWidth - width - 12));
    const above = rect.top > window.innerHeight / 2;
    setPosition({ left, top: above ? rect.top - 8 : rect.bottom + 8, width, above });
  }
  return <>
    <span
      className="compact-help"
      aria-label={`${label}：${details.replaceAll("\n", "；")}`}
      aria-describedby={position ? tooltipID : undefined}
      role="note"
      tabIndex={0}
      onMouseEnter={(event) => show(event.currentTarget)}
      onMouseLeave={() => setPosition(undefined)}
      onFocus={(event) => show(event.currentTarget)}
      onBlur={() => setPosition(undefined)}
      onKeyDown={(event) => { if (event.key === "Escape") setPosition(undefined); }}
    ><Question size={13} weight="bold" /></span>
    {position && createPortal(<span id={tooltipID} className={`compact-help-tooltip${position.above ? " is-above" : ""}`} role="tooltip" style={{ left: position.left, top: position.top, width: position.width }}>{details}</span>, document.body)}
  </>;
}

function agentRuntimeLabel(agent: Agent, t: (key: string) => string) {
	if (agent.transport !== "acp") return t("原生直连");
	return `${t(agent.catalog_tier === "preset" ? "ACP 预设" : "自定义 ACP")} v${agent.protocol_version ?? 1}`;
}

function agentConnectionState(agent: Agent, t: (text: string) => string, locale: string) {
	const queued = agent.queued_task_count ?? 0;
	const running = agent.running_task_count ?? 0;
	const session = agent.native_session_ready ? t(" 已保存原生会话，下次任务可继续同一上下文。") : "";
	const lastSeen = agent.runner_last_seen_at ? t(" 最后心跳：{time}。").replace("{time}", new Date(agent.runner_last_seen_at).toLocaleString(locale)) : "";
	switch (agent.status) {
		case "online":
			return queued > 0
				? { label: t("待执行"), state: "queued", description: t("Client 已连接，{count} 项任务正在排队。").replace("{count}", String(queued)) + session + lastSeen }
				: { label: t("就绪"), state: "ready", description: t("Client 已连接，可以接收任务。") + session + lastSeen };
		case "busy":
			return { label: t("执行中"), state: "working", description: t("Client 已连接，正在执行 {running} 项任务{queued}。").replace("{running}", String(Math.max(1, running))).replace("{queued}", queued > 0 ? t("，另有 {count} 项排队").replace("{count}", String(queued)) : "") + session + lastSeen };
		case "unavailable":
			return { label: t("不可用"), state: "unavailable", description: t("Client 已连接，但这台电脑未检测到可执行的 Agent。") + lastSeen };
		default:
			return queued > 0
				? { label: t("等待上线"), state: "waiting", description: t("已有 {count} 项任务排队；目标电脑的 Client 自动重连后会继续执行。").replace("{count}", String(queued)) + session + lastSeen }
				: { label: t("离线"), state: "offline", description: t("等待目标电脑的 Client 自动重连。") + session + lastSeen };
	}
}

function AgentConnectionStatus({ agent, label = agent.display_name }: { agent: Agent; label?: string }) {
	const { t, locale } = useLocale();
	const state = agentConnectionState(agent, t, locale);
	return <span className={`agent-connection agent-connection-${state.state}`} aria-label={t("{name} 运行状态：{state}，{description}").replace("{name}", label).replace("{state}", state.label).replace("{description}", state.description)} title={state.description}>
		<span className="agent-connection-dot" aria-hidden="true" />
		<span>{state.label}</span>
	</span>;
}

export function RoomInspector({
	open, room, workspace, projectPlan, currentUser, members, workspaceMembers, agents, workspaceAgents, verifying, onRegisterAgent, onAddRoomAgent,
	onJoinWorkspace, onCreateHumanInvite, onRemoveRoomAgent, onRemoveWorkspaceAgent, onUpdateAgentSettings, onManageAgentSettings,
	onAddRoomMember, onUpdateRoomMemberRole, onRemoveRoomMember, onUpdateWorkspaceMemberRole, onRemoveWorkspaceMember,
	onUpdateRoomProjectStep, onVerifyAgents, onClose,
}: RoomInspectorProps) {
  const [addingAgentID, setAddingAgentID] = useState("");
  const [updatingAgentID, setUpdatingAgentID] = useState("");
  const [removingKey, setRemovingKey] = useState("");
  const [confirmingKey, setConfirmingKey] = useState("");
  const [collapsed, setCollapsed] = useState(initialCollapsed);
  const [collapsedMembers, setCollapsedMembers] = useState(initialCollapsedMembers);
  const [collapsedLibraryOwners, setCollapsedLibraryOwners] = useState(initialCollapsedLibraryOwners);
  const [error, setError] = useState("");
  const [libraryFeedback, setLibraryFeedback] = useState("");
  const [humanInvite, setHumanInvite] = useState<HumanInvite>();
  const [creatingHumanInvite, setCreatingHumanInvite] = useState(false);
  const [humanLinkCopied, setHumanLinkCopied] = useState(false);
  const { t } = useLocale();
  const [collaborationEntryOpen, setCollaborationEntryOpen] = useState(false);
  const [workspaceJoinOpen, setWorkspaceJoinOpen] = useState(false);
  const [workspaceInviteLink, setWorkspaceInviteLink] = useState("");
  const [workspaceJoinError, setWorkspaceJoinError] = useState("");
  const [joiningWorkspace, setJoiningWorkspace] = useState(false);
	const [memberPortalTarget, setMemberPortalTarget] = useState<HTMLElement | null>(null);
	const [selectedRoomAgentID, setSelectedRoomAgentID] = useState("");
	const roomAgentIDs = new Set(agents.map((agent) => agent.id));
	const effectiveCurrentUser = currentUser ?? members[0];
	const currentRoomRole = members.find((member) => member.id === effectiveCurrentUser?.id)?.room_role;
	const canManageRoomRoles = currentRoomRole === "owner" || workspace?.role === "owner" || (!workspace && effectiveCurrentUser?.role === "admin");
	const canModerate = currentRoomRole === "owner" || currentRoomRole === "admin" || workspace?.role === "owner" || workspace?.role === "admin" || (!workspace && effectiveCurrentUser?.role === "admin");
	const canAdministerWorkspace = workspace?.role === "owner" || (!workspace && effectiveCurrentUser?.role === "admin");
	const canManageAgent = (agent: Agent) => agent.can_manage ?? agent.owner_user_id === effectiveCurrentUser?.id;
	const canModerateAgent = (agent: Agent) => Boolean(agent.can_moderate) || canModerate;
	const canModerateWorkspaceAgent = (agent: Agent) => Boolean(agent.can_moderate) || workspace?.role === "owner" || workspace?.role === "admin";
	const roomMemberIDs = new Set(members.map((member) => member.id));
	const otherWorkspaceMembers = room ? (workspaceMembers ?? members).filter((member) => !roomMemberIDs.has(member.id)) : [];
	const roomAgentGroups = groupAgentsByOwner(agents, members);
	const addableWorkspaceAgents = workspaceAgents.filter((agent) => !roomAgentIDs.has(agent.id));
	const workspaceAgentGroups = groupAgentsByOwner(addableWorkspaceAgents, workspaceMembers ?? members);
	const selectedRoomAgent = agents.find((agent) => agent.id === selectedRoomAgentID);
	const knownPeople = workspaceMembers ?? members;
	const selectedRoomAgentOwner = selectedRoomAgent ? knownPeople.find((person) => person.id === selectedRoomAgent.owner_user_id) : undefined;
	const selectedRoomAgentName = selectedRoomAgent ? agentDisplayName(selectedRoomAgent, selectedRoomAgentOwner) : "";

	useEffect(() => {
		setMemberPortalTarget(document.getElementById("room-member-sidebar-slot"));
	}, [room?.id]);

  function toggleSection(section: InspectorSection) {
    setCollapsed((current) => {
      const next = { ...current, [section]: !current[section] };
      window.localStorage.setItem("agentcollab.inspector-collapsed", JSON.stringify(next));
      return next;
    });
  }

  function toggleMember(memberID: string, agentCount: number) {
    setCollapsedMembers((current) => {
      const next = { ...current, [memberID]: !ownerGroupCollapsed(current, memberID, agentCount) };
      window.localStorage.setItem("agentcollab.member-tree-collapsed-v2", JSON.stringify(next));
      return next;
    });
  }

  function toggleLibraryOwner(ownerID: string, agentCount: number) {
    setCollapsedLibraryOwners((current) => {
      const next = { ...current, [ownerID]: !ownerGroupCollapsed(current, ownerID, agentCount) };
      window.localStorage.setItem("agentcollab.agent-library-owner-collapsed-v2", JSON.stringify(next));
      return next;
    });
  }

  async function addAgent(agentID: string) {
    if (addingAgentID) return;
    setAddingAgentID(agentID);
    setError("");
	setLibraryFeedback("");
    try {
      await onAddRoomAgent(agentID);
		const agent = workspaceAgents.find((candidate) => candidate.id === agentID);
		setLibraryFeedback(agent?.status === "online"
			? `${agent.display_name} 已加入当前群聊，现在可在输入框 @ 它。`
			: `${agent?.display_name ?? "该 Agent"} 已加入当前群聊；目标电脑 Client 当前离线，恢复在线后即可在输入框 @ 它。`);
    } catch (reason) {
		const message = actionError(reason, "无法加入群聊");
		setError(message);
		setLibraryFeedback(message);
    } finally {
      setAddingAgentID("");
    }
  }

  async function removeAgent(scope: "room" | "workspace", agentID: string) {
    const key = `${scope}:${agentID}`;
    if (removingKey) return;
    setRemovingKey(key);
    setError("");
    try {
      if (scope === "room") await onRemoveRoomAgent(agentID);
      else await onRemoveWorkspaceAgent(agentID);
      setConfirmingKey("");
    } catch (reason) {
      setError(actionError(reason, scope === "room" ? "无法从群聊移出 Agent" : "无法从可用 Agent 删除"));
    } finally {
      setRemovingKey("");
    }
  }

	async function updateSettings(agentID: string, patch: Partial<Pick<Agent, "response_mode" | "role_prompt">>) {
    if (updatingAgentID) return;
    setUpdatingAgentID(agentID);
    setError("");
    try {
      await onUpdateAgentSettings(agentID, patch);
    } catch (reason) {
      setError(actionError(reason, "无法更新 Agent 响应方式"));
	    } finally {
	      setUpdatingAgentID("");
		}
	}

	async function addMember(userID: string) {
		if (addingAgentID) return;
		setAddingAgentID(`member:${userID}`); setError("");
		try { if (onAddRoomMember) await onAddRoomMember(userID); }
		catch (reason) { setError(actionError(reason, "无法添加群成员")); }
		finally { setAddingAgentID(""); }
	}

	async function removeMember(scope: "room" | "workspace", userID: string) {
		const key = `${scope}-member:${userID}`;
		if (removingKey) return;
		setRemovingKey(key); setError("");
		try {
			if (scope === "room") { if (onRemoveRoomMember) await onRemoveRoomMember(userID); }
			else if (onRemoveWorkspaceMember) await onRemoveWorkspaceMember(userID);
			setConfirmingKey("");
		}
		catch (reason) { setError(actionError(reason, scope === "room" ? "无法移出群成员" : "无法删除工作区成员")); }
		finally { setRemovingKey(""); }
	}
  async function createHumanInvite() {
    if (creatingHumanInvite) return;
    setCreatingHumanInvite(true);
    setError("");
    try {
      const invite = await onCreateHumanInvite();
      setHumanInvite(invite);
      setHumanLinkCopied(false);
    } catch (reason) {
      setError(actionError(reason, "无法生成真人群聊邀请"));
    } finally {
      setCreatingHumanInvite(false);
    }
  }

  async function joinWorkspace(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const inviteLink = workspaceInviteLink.trim();
    if (!inviteLink || !onJoinWorkspace || joiningWorkspace) return;
    setJoiningWorkspace(true);
    setWorkspaceJoinError("");
    try {
      await onJoinWorkspace(inviteLink);
    } catch (reason) {
      setWorkspaceJoinError(reason instanceof Error ? reason.message : "无法加入团队，请检查邀请链接");
    } finally {
      setJoiningWorkspace(false);
    }
  }

  async function copyHumanJoinLink(link: string) {
    setError("");
    try {
      // Clipboard API is unavailable on many LAN HTTP pages. Prefer it when
      // permitted, then fall back to the legacy selection command so the
      // Server's local-network invitation flow still works without HTTPS.
      if (navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(link);
      } else {
        const textarea = document.createElement("textarea");
        textarea.value = link;
        textarea.setAttribute("readonly", "");
        textarea.style.position = "fixed";
        textarea.style.top = "0";
        textarea.style.left = "-9999px";
        document.body.appendChild(textarea);
        textarea.focus();
        textarea.select();
        const copied = document.execCommand("copy");
        textarea.remove();
        if (!copied) throw new Error("clipboard copy was rejected");
      }
      setHumanLinkCopied(true);
      window.setTimeout(() => setHumanLinkCopied(false), 1600);
    } catch {
      setHumanLinkCopied(false);
      setError("无法自动复制，请选中上方链接后手动复制");
    }
  }

  // Prefer the address the operator is currently using when it is reachable
  // from the LAN. A Hub can be configured with an old/stale public_url (for
  // example after the server changes Wi-Fi or Ethernet), while the page that
  // generated the invite is already known to be reachable. Only fall back to
  // the configured Hub URLs for loopback pages, which must never be shared
  // with another computer.
  const currentOrigin = window.location.origin;
  const currentHostname = window.location.hostname.toLowerCase();
  const currentOriginIsLoopback = currentHostname === "localhost" || currentHostname === "127.0.0.1" || currentHostname === "0.0.0.0" || currentHostname === "[::1]" || currentHostname === "::1";
  const humanHubURLs = humanInvite
    ? Array.from(new Set(currentOriginIsLoopback
      ? [...(humanInvite.hub_urls ?? []), currentOrigin]
      : [currentOrigin, ...(humanInvite.hub_urls ?? [])]))
    : [];
  const humanHubURL = humanHubURLs[0] || window.location.origin;
  const humanJoinURL = humanInvite ? `${humanHubURL.replace(/\/$/, "")}/?join=${encodeURIComponent(humanInvite.code)}` : "";

	function renderRoomAgent(agent: Agent) {
		const key = `room:${agent.id}`;
		const owner = knownPeople.find((person) => person.id === agent.owner_user_id);
		const displayName = agentDisplayName(agent, owner);
		const shortName = agentShortName(agent, owner);
		return <div className="agent-row agent-owner-child" key={agent.id}>
			<Robot size={18} weight="fill" />
			<span>
				<span className="agent-title-line"><strong title={displayName}>{shortName} {canManageAgent(agent) && <em className="owner-badge">我的</em>}</strong>{(canManageAgent(agent) || canModerateAgent(agent)) && <button type="button" className="danger-icon-button" onClick={() => setConfirmingKey(key)} aria-label={`将 ${displayName} 移出当前群聊`}><Trash size={15} /></button>}</span>
				<span className="agent-compact-meta-line"><small className="agent-compact-meta">{agent.type}，{agent.device_name}</small><AgentConnectionStatus agent={agent} label={displayName} /></span>
				{confirmingKey === key && <span className="inline-confirm" role="group" aria-label={`确认将 ${displayName} 移出当前群聊`}><span>移出当前群？历史消息会保留。</span><button type="button" disabled={removingKey === key} onClick={() => void removeAgent("room", agent.id)}>确认移出</button><button type="button" onClick={() => setConfirmingKey("")}>取消</button></span>}
				<span className="agent-compact-controls"><button type="button" className="agent-detail-trigger" onClick={() => setSelectedRoomAgentID(agent.id)} aria-label={t("查看 {0} 详情与群内设置").replace("{0}", () => displayName)}><SlidersHorizontal size={13} /><span>{t("详情 / 群内设置")}</span></button><span className="agent-skill-share-chip">{agentSkillShareLabel(agent, canManageAgent(agent), t)}</span><span className="agent-response-chip">{t(agentResponseLabel(agent))}</span></span>
				{agent.can_invoke === false && <small className="capability-unsynced">{agent.invocation_scope === "room_members" && !supportsScopedClient(agent.runner_version) ? "目标电脑 Client 需要升级后才能安全开放调用。" : "所有者尚未授权你调用此 Agent。"}</small>}
			</span>
		</div>;
	}

	function renderLibraryAgent(agent: Agent) {
		const key = `workspace:${agent.id}`;
		const owner = knownPeople.find((person) => person.id === agent.owner_user_id);
		const displayName = agentDisplayName(agent, owner);
		const shortName = agentShortName(agent, owner);
		return <div className="library-agent-row" key={agent.id}>
			<Robot size={18} weight="fill" />
			<span><span className="library-agent-name"><strong title={displayName}>{shortName} {canManageAgent(agent) && <em className="owner-badge">我的</em>}</strong></span><span className="agent-compact-meta-line"><small>{agent.type}，{agent.device_name}</small><AgentConnectionStatus agent={agent} label={displayName} /></span></span>
			<span className="library-agent-actions">
				{(canManageAgent(agent) || canModerateAgent(agent)) && <button type="button" className={addingAgentID === agent.id ? "is-loading" : ""} disabled={!room || agent.status === "unavailable" || Boolean(addingAgentID)} onClick={() => void addAgent(agent.id)} aria-label={addingAgentID === agent.id ? `正在将 ${displayName} 加入群聊` : `将 ${displayName} 加入群聊`} title={addingAgentID === agent.id ? "正在加入群聊" : agent.status === "offline" ? "先加入群；目标电脑恢复在线后即可调用" : "加入当前群聊"}><Plus size={15} /></button>}
				{(canManageAgent(agent) || canModerateWorkspaceAgent(agent)) && <button type="button" className="danger-icon-button" onClick={() => setConfirmingKey(key)} aria-label={`从 Agent 库删除 ${displayName}`}><Trash size={15} /></button>}
			</span>
			{confirmingKey === key && <span className="inline-confirm library-confirm" role="group" aria-label={`确认从 Agent 库删除 ${displayName}`}><span>将从所有群移出；历史记录保留。</span><button type="button" disabled={removingKey === key} onClick={() => void removeAgent("workspace", agent.id)}>确认删除</button><button type="button" onClick={() => setConfirmingKey("")}>取消</button></span>}
		</div>;
	}

	const memberSection = <section className={`room-member-section${collapsed.members ? " is-collapsed" : ""}`}>
		<div className="inspector-section-heading">
			<button type="button" onClick={() => toggleSection("members")} aria-expanded={!collapsed.members} aria-label={t(collapsed.members ? "展开群聊成员" : "折叠群聊成员")}>
				<h2>{t("群聊成员")} <span>{t("{0} 真人 / {1} Agent").replaceAll("{0}", String(members.length)).replaceAll("{1}", String(agents.length))}</span></h2><CaretDown size={16} />
			</button>
		</div>
		{!collapsed.members && <div className="inspector-section-body">
			{!room && <div className="inspector-no-room"><ChatCircleDots size={24} /><strong>先创建或选择群聊</strong><p>工作区不会自动创建群聊。请在左侧“群聊”标题旁点击 + 手动创建；选中群聊后才能邀请真人或添加 Agent。</p></div>}
			{room && <>
			<p className="member-group-label">{t("协作席位")}</p>
			<div className="member-tree">
				{members.map((member) => {
					const key = `room-member:${member.id}`;
					const canRemove = member.id !== effectiveCurrentUser?.id && member.room_role !== "owner" && (canManageRoomRoles || workspace?.role === "admin" || (currentRoomRole === "admin" && member.room_role === "member"));
					const ownedAgents = agents.filter((agent) => agent.owner_user_id === member.id);
					const canCollapseAgents = ownedAgents.length > 0;
					const memberCollapsed = ownerGroupCollapsed(collapsedMembers, member.id, ownedAgents.length);
					return <section className={`member-tree-group${memberCollapsed ? " is-collapsed" : ""}${ownedAgents.length === 1 ? " is-single-agent" : ""}`} key={member.id}>
						<div className="member-row member-tree-owner">
							{canCollapseAgents ? <button type="button" className="member-owner-toggle" onClick={() => toggleMember(member.id, ownedAgents.length)} aria-expanded={!memberCollapsed} aria-controls={`member-agents-${member.id}`} aria-label={`${memberCollapsed ? "展开" : "折叠"} ${member.display_name} 名下 Agent`}>
								<span className="member-avatar"><UserCircle size={19} weight="fill" /></span>
								<span><strong>{member.display_name}</strong><small>@{member.username}，{agentGroupSummary(ownedAgents, t)}</small></span>
								<CaretDown size={14} />
							</button> : <div className="member-owner-toggle member-owner-static">
								<span className="member-avatar"><UserCircle size={19} weight="fill" /></span>
								<span><strong>{member.display_name}</strong><small>@{member.username}，{agentGroupSummary(ownedAgents, t)}</small></span>
							</div>}
							{member.room_role === "owner" ? <span className="member-role-badge"><ShieldCheck size={15} />{t("群主")}</span> : canManageRoomRoles && member.id !== effectiveCurrentUser?.id ? <select className="member-role-select" aria-label={`${member.display_name} 群内角色`} value={member.room_role ?? "member"} onChange={(event) => void onUpdateRoomMemberRole?.(member.id, event.target.value as "admin" | "member")}><option value="member">{t("成员")}</option><option value="admin">{t("管理员")}</option></select> : member.room_role === "admin" ? <span className="member-role-badge"><ShieldCheck size={15} />{t("管理员")}</span> : null}
							{canRemove && <button type="button" className="danger-icon-button" onClick={() => setConfirmingKey(key)} aria-label={`将 ${member.display_name} 移出当前群`}><Trash size={15} /></button>}
							{confirmingKey === key && <span className="inline-confirm member-confirm" role="group"><span>移出当前群？工作区账号、设备和 Agent 库不受影响。</span><button type="button" disabled={removingKey === key} onClick={() => void removeMember("room", member.id)}>确认移出</button><button type="button" onClick={() => setConfirmingKey("")}>取消</button></span>}
						</div>
						{!memberCollapsed && (ownedAgents.length > 0 ? <div className="member-agent-children" id={`member-agents-${member.id}`}>{ownedAgents.map(renderRoomAgent)}</div> : <p className="member-agent-empty" id={`member-agents-${member.id}`}>{t("此成员尚未接入 Agent")}</p>)}
					</section>;
				})}
				{roomAgentGroups.filter((group) => !group.owner).map((group) => { const memberCollapsed = ownerGroupCollapsed(collapsedMembers, group.ownerID, group.agents.length); const canCollapseAgents = group.agents.length > 0; return <section className={`member-tree-group member-tree-unlinked${memberCollapsed ? " is-collapsed" : ""}${group.agents.length === 1 ? " is-single-agent" : ""}`} key={group.ownerID}>
					<div className="member-row member-tree-owner">{canCollapseAgents ? <button type="button" className="member-owner-toggle" onClick={() => toggleMember(group.ownerID, group.agents.length)} aria-expanded={!memberCollapsed} aria-controls={`member-agents-${group.ownerID}`} aria-label={`${memberCollapsed ? "展开" : "折叠"} ${group.ownerName} 名下 Agent`}><span className="member-avatar"><UserCircle size={19} weight="fill" /></span><span><strong>{group.ownerName}</strong><small>未加入本群，{agentGroupSummary(group.agents, t)}</small></span><CaretDown size={14} /></button> : <div className="member-owner-toggle member-owner-static"><span className="member-avatar"><UserCircle size={19} weight="fill" /></span><span><strong>{group.ownerName}</strong><small>未加入本群，{agentGroupSummary(group.agents, t)}</small></span></div>}</div>
					{!memberCollapsed && <div className="member-agent-children" id={`member-agents-${group.ownerID}`}>{group.agents.map(renderRoomAgent)}</div>}
				</section>; })}
				{members.length === 0 && <p className="inspector-empty">{t("正在同步成员")}</p>}
			</div>
			{(canModerate || canAdministerWorkspace) && otherWorkspaceMembers.length > 0 && <details className="workspace-member-admin"><summary>其他工作区账号 · {otherWorkspaceMembers.length}</summary><p>这里只显示未加入当前群的账号。可将其加入群聊；工作区 Owner 还可在这里管理或删除账号。</p><div className="member-list">{otherWorkspaceMembers.map((member) => { const key = `workspace-member:${member.id}`; return <div className="member-row" key={member.id}><span className="member-avatar"><UserCircle size={19} weight="fill" /></span><span><strong>{member.display_name}</strong><small>@{member.username}</small></span>{canAdministerWorkspace && member.workspace_role !== "owner" && <select className="member-role-select" aria-label={`${member.display_name} 工作区角色`} value={member.workspace_role ?? "member"} onChange={(event) => void onUpdateWorkspaceMemberRole?.(member.id, event.target.value as "admin" | "member")}><option value="member">{t("成员")}</option><option value="admin">{t("管理员")}</option></select>}{canModerate && <button type="button" className="section-quick-action compact-icon-action" disabled={Boolean(addingAgentID)} onClick={() => void addMember(member.id)} aria-label={`将 ${member.display_name} 加入当前群`} title="加入当前群"><Plus size={14} /></button>}{canAdministerWorkspace && member.workspace_role !== "owner" && <button type="button" className="danger-icon-button" onClick={() => setConfirmingKey(key)} aria-label={`从工作区删除 ${member.display_name}`}><Trash size={15} /></button>}{confirmingKey === key && <span className="inline-confirm member-confirm" role="group"><span>将从整个工作区删除账号、设备和 Agent；历史记录保留。</span><button type="button" disabled={removingKey === key} onClick={() => void removeMember("workspace", member.id)}>确认删除</button><button type="button" onClick={() => setConfirmingKey("")}>取消</button></span>}</div>; })}</div></details>}
			</>}
		</div>}
	</section>;

  return (
    <aside className={`room-inspector${open ? " is-open" : ""}`}>
      <header>
        <div><span>{t("群工具")}</span><strong>{room ? `# ${room.name}` : t("未选择")}</strong></div>
        <button className="mobile-icon-button" onClick={onClose} aria-label={t("关闭群详情")}><X size={19} /></button>
      </header>

      {room && projectPlan && projectPlan.steps.length > 0 && <section className="room-project-link">
        <label>
          <span><GitBranch size={16} />{t("群聊默认步骤")} <CompactHelp label={t("群聊默认步骤说明")} details={t("设定后，新会话会默认归入该步骤。发送前仍可改选其他步骤或普通会话。")} /></span>
          <select aria-label={t("群聊默认项目步骤")} value={room.plan_step_id ?? ""} disabled={!projectPlan.can_manage || !onUpdateRoomProjectStep} onChange={(event) => void onUpdateRoomProjectStep?.(event.target.value || undefined)}>
            <option value="">{t("不固定 · 每次会话选择")}</option>
            {projectPlan.steps.filter((step) => step.status !== "cancelled").map((step) => <option key={step.id} value={step.id}>{step.parent_step_id ? "↳ " : ""}{step.title}</option>)}
          </select>
        </label>
      </section>}

	  <section className="inspector-primary-action">
		<button type="button" onClick={() => setCollaborationEntryOpen((current) => !current)} aria-expanded={collaborationEntryOpen} aria-label={t("邀请与接入")}><UserPlus size={16} /><span><strong>{t("邀请与接入")}</strong><small>{t("加入 / 邀请 / Agent")}</small></span><CompactHelp label={t("邀请与接入说明")} details={t("收到链接可加入其他团队；群管理员可邀请协作者；需要贡献本机 Agent 时再连接 Client。")} /><CaretDown size={16} /></button>
	  </section>

      {collaborationEntryOpen && <section className="collaboration-entry-panel" aria-label={t("邀请与接入")}>
        <header><div><strong>{t("邀请与接入")}</strong><small>{t("加入已有团队、邀请协作者，或接入自己的 Agent。")}</small></div><button type="button" onClick={() => setCollaborationEntryOpen(false)} aria-label={t("关闭邀请与接入")}><X size={16} /></button></header>
        <div className="collaboration-entry-options">
          <article className="is-join"><span className="entry-option-icon"><SignIn size={18} weight="bold" /></span><div><strong>{t("加入团队")}</strong><small>{t("粘贴别人发来的完整邀请链接，进入对方的工作区和群聊。")}</small></div><button type="button" onClick={() => { setWorkspaceJoinOpen((current) => !current); setWorkspaceJoinError(""); }}>{workspaceJoinOpen ? t("收起") : t("输入邀请链接")}</button></article>
          <article><span className="entry-option-icon"><UserPlus size={18} weight="bold" /></span><div><strong>{t("邀请协作者")}</strong><small>{canModerate ? t("生成一个链接，对方登录或注册后直接进入当前工作区和群聊。") : t("群主或群管理员可以生成链接；普通成员仍可加入其他团队或添加自己的 Agent。")}</small></div><button type="button" disabled={!room || creatingHumanInvite || !canModerate} onClick={() => void createHumanInvite()}>{creatingHumanInvite ? t("生成中") : !canModerate ? t("需要群管理员") : humanInvite ? t("重新生成") : t("生成邀请链接")}</button></article>
          <article><span className="entry-option-icon is-muted"><Robot size={18} weight="fill" /></span><div><strong>{t("添加我的 Agent")}</strong><small>{t("只管理自己的电脑和 Agent；普通真人协作不需要安装 Client。")}</small></div><button type="button" onClick={onRegisterAgent}>{t("打开本机 Agent")}</button></article>
        </div>
        {workspaceJoinOpen && <form className="collaboration-join-form" onSubmit={(event) => void joinWorkspace(event)}>
          <label htmlFor="collaboration-team-invite">{t("团队邀请链接")}</label>
          <div><input id="collaboration-team-invite" value={workspaceInviteLink} onChange={(event) => setWorkspaceInviteLink(event.target.value)} placeholder={t("http://团队地址/?join=…")} autoFocus /><button type="submit" disabled={!workspaceInviteLink.trim() || joiningWorkspace || !onJoinWorkspace}>{joiningWorkspace ? t("正在加入") : t("加入团队")}</button></div>
          {workspaceJoinError && <p className="form-error" role="alert">{t(workspaceJoinError)}</p>}
          <small>{t("请粘贴完整邀请链接，不需要另填 Hub 地址或配对码。")}</small>
        </form>}
        {humanInvite && <div className="collaboration-invite-result">
          <div><strong>{t("发送这一个链接即可")}</strong><small>{t("链接已包含团队地址和目标群聊；对方不需要填写 Hub 地址、邀请码或设备配对码。")}</small></div>
          <div className="collaboration-invite-link"><code>{humanJoinURL}</code><button type="button" onClick={() => void copyHumanJoinLink(humanJoinURL)} aria-label={t("复制真人群聊加入链接")}><CopySimple size={14} />{humanLinkCopied ? t("已复制") : t("复制链接")}</button></div>
          <small>{t("有效至")} {new Date(humanInvite.expires_at).toLocaleString()} {t("，且只能使用一次。")}</small>
        </div>}
      </section>}

	  {memberPortalTarget ? createPortal(memberSection, memberPortalTarget) : memberSection}

      <section className={`agent-library-section${collapsed.library ? " is-collapsed" : ""}`}>
        <div className="inspector-section-heading">
		  <button type="button" onClick={() => toggleSection("library")} aria-expanded={!collapsed.library} aria-label={t(collapsed.library ? "展开可添加 Agent" : "折叠可添加 Agent")}><h2>{t("可添加 Agent")} <span>{addableWorkspaceAgents.length}</span></h2><CompactHelp label={t("可添加 Agent 说明")} details={t("这里只列尚未加入当前群聊的 Agent。已经入群的 Agent 统一在左侧群聊成员中查看和设置。")} /><CaretDown size={16} /></button>
		  <span className="inspector-heading-actions">{onManageAgentSettings && <button type="button" className="section-quick-action" onClick={onManageAgentSettings}><SlidersHorizontal size={14} />{t("我的 Agent 设置")}</button>}<button type="button" className="section-quick-action compact-icon-action" onClick={() => setCollaborationEntryOpen(true)} aria-label={t("打开邀请与接入")} title={t("邀请真人或添加我的 Agent")}><Plus size={14} /></button></span>
        </div>
        {!collapsed.library && <div className="inspector-section-body">
		  {libraryFeedback && <p className={`library-agent-feedback${error ? " is-error" : ""}`} role={error ? "alert" : "status"}>{libraryFeedback}</p>}
		  {addableWorkspaceAgents.length > 0 ? <div className="library-owner-groups">
			{workspaceAgentGroups.map((group) => { const canCollapseAgents = group.agents.length > 0; const ownerCollapsed = ownerGroupCollapsed(collapsedLibraryOwners, group.ownerID, group.agents.length); return <section className={`library-owner-group${ownerCollapsed ? " is-collapsed" : ""}${group.agents.length === 1 ? " is-single-agent" : ""}`} key={group.ownerID}>
			  <header>{canCollapseAgents ? <button type="button" className="library-owner-toggle" onClick={() => toggleLibraryOwner(group.ownerID, group.agents.length)} aria-expanded={!ownerCollapsed} aria-controls={`library-owner-agents-${group.ownerID}`} aria-label={`${ownerCollapsed ? "展开" : "折叠"}可添加 Agent 中 ${group.ownerName} 名下 Agent`}><span className="member-avatar"><UserCircle size={17} weight="fill" /></span><span><strong>{group.ownerName}</strong><small>{group.owner ? `@${group.owner.username}，` : ""}{agentGroupSummary(group.agents, t)}</small></span><CaretDown size={14} /></button> : <div className="library-owner-toggle library-owner-static"><span className="member-avatar"><UserCircle size={17} weight="fill" /></span><span><strong>{group.ownerName}</strong><small>{group.owner ? `@${group.owner.username}，` : ""}{agentGroupSummary(group.agents, t)}</small></span></div>}</header>
			  {!ownerCollapsed && <div className="library-agent-list" id={`library-owner-agents-${group.ownerID}`}>{group.agents.map(renderLibraryAgent)}</div>}
			</section>; })}
		  </div> : <p className="inspector-empty">{t("当前没有待加入的 Agent。已入群 Agent 请在左侧“群聊成员”中查看。")}</p>}
        </div>}
      </section>

      <section className={`verification-section${collapsed.verification ? " is-collapsed" : ""}`}>
        <div className="inspector-section-heading">
          <button type="button" onClick={() => toggleSection("verification")} aria-expanded={!collapsed.verification} aria-label={t(collapsed.verification ? "展开高级连接诊断" : "折叠高级连接诊断")}><h2>{t("高级 · 连接诊断")} <CompactHelp label={t("连接诊断说明")} details={t("仅在排查问题时使用：检查当前群中的 Agent 能否独立接收任务并返回结果。")} /></h2><CaretDown size={16} /></button>
          <button type="button" className="section-quick-action" disabled={agents.length < 2 || verifying} onClick={() => void onVerifyAgents()}><TestTube size={14} />{t(verifying ? "检查中" : "运行检查")}</button>
        </div>
      </section>
      {error && <p className="inspector-error inspector-global-error" role="alert">{t(error)}</p>}

	  {selectedRoomAgent && <div className="agent-detail-drawer-backdrop" role="presentation" onMouseDown={(event) => { if (event.currentTarget === event.target) setSelectedRoomAgentID(""); }}>
		<section className="agent-detail-drawer" role="dialog" aria-modal="true" aria-label={t("{0} 群内设置").replace("{0}", () => selectedRoomAgentName)}>
		  <header><div><span>{t("本群 Agent 设置")}</span><strong>{selectedRoomAgentName}</strong><small>{selectedRoomAgent.type} · {agentRuntimeLabel(selectedRoomAgent, t)} · {selectedRoomAgent.device_name}</small></div><button type="button" onClick={() => setSelectedRoomAgentID("")} aria-label={t("关闭 Agent 群内设置")}><X size={18} /></button></header>
		  <div className="agent-detail-drawer-body">
			<section className="agent-detail-summary"><AgentConnectionStatus agent={selectedRoomAgent} label={selectedRoomAgentName} /><span>{t(agentVisibilityLabel(selectedRoomAgent))}</span><span>{t(agentInvocationLabel(selectedRoomAgent))}</span><span>{agentSkillShareLabel(selectedRoomAgent, canManageAgent(selectedRoomAgent), t)}</span></section>
			<section className="agent-detail-section"><header><strong>{t("本群行为")}</strong><span>{t(agentResponseLabel(selectedRoomAgent))}</span></header>{canManageAgent(selectedRoomAgent) ? <div className="agent-detail-room-form"><label><span>{t("响应方式")}</span><select aria-label={t("{0} 响应方式").replace("{0}", () => selectedRoomAgent.display_name)} value={selectedRoomAgent.response_mode} disabled={updatingAgentID === selectedRoomAgent.id} onChange={(event) => void updateSettings(selectedRoomAgent.id, { response_mode: event.target.value as Agent["response_mode"] })}><option value="mention">{t("仅被 @ 时")}</option><option value="coordinator">{t("主协调 / 自动分诊")}</option><option value="human_all">{t("回复所有真人消息")}</option><option value="muted">{t("静默")}</option></select></label><label><span>{t("群内角色")}</span><textarea aria-label={t("{0} 群内角色").replace("{0}", () => selectedRoomAgent.display_name)} defaultValue={selectedRoomAgent.role_prompt} maxLength={2000} rows={4} placeholder={t("例如：负责架构拆解与任务分诊")} disabled={updatingAgentID === selectedRoomAgent.id} onBlur={(event) => { if (event.target.value.trim() !== selectedRoomAgent.role_prompt) void updateSettings(selectedRoomAgent.id, { role_prompt: event.target.value.trim() }); }} /></label></div> : <p>{t("本群响应：")}{t(agentResponseLabel(selectedRoomAgent))}{selectedRoomAgent.role_prompt ? ` · ${selectedRoomAgent.role_prompt}` : ""}</p>}</section>
		  </div>
		  <footer><span>{canManageAgent(selectedRoomAgent) ? t("本机发现 {0} 个 Skill，当前允许他人调用 {1} 个。").replace("{0}", String(selectedRoomAgent.skills?.length ?? 0)).replace("{1}", String(selectedRoomAgent.shared_skills?.length ?? 0)) : t("只有 Agent 主人可以修改共享 Skill；本群管理员只管理群成员。")}</span>{canManageAgent(selectedRoomAgent) && onManageAgentSettings && <button type="button" className="primary-button" onClick={() => { setSelectedRoomAgentID(""); onManageAgentSettings(); }}>{t("管理 Skill 授权")}</button>}</footer>
		</section>
	  </div>}
    </aside>
  );
}
