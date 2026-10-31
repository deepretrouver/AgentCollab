import { Fragment, useMemo, useState, type FormEvent } from "react";
import {
  CaretDown,
  ChatCircleDots,
  CirclesFour,
  GearSix,
  LinkSimple,
  Moon,
  PushPin,
  PushPinSlash,
  Plus,
  SignOut,
  Sun,
  ShieldCheck,
  X,
} from "@phosphor-icons/react";
import type { Agent, Room, User, Workspace } from "../api/client";
import { useLocale } from "../i18n/locale";

interface WorkspaceSidebarProps {
  open: boolean;
  currentUser: User;
  workspaces: Workspace[];
  selectedWorkspace?: Workspace;
  rooms: Room[];
  agents: Agent[];
  selectedRoom?: Room;
  loading: boolean;
  surface?: "client" | "web";
  onSelectWorkspace: (workspace: Workspace) => void;
  onSelectRoom: (room: Room) => void;
  onCreateWorkspace: (name: string) => Promise<void>;
  onUpdateWorkspace?: (workspaceID: string, name: string) => Promise<void>;
  onDeleteWorkspace?: (workspaceID: string) => Promise<void>;
  onCreateRoom: (name: string, agentIDs: string[]) => Promise<void>;
  onJoinHub: (inviteLink: string) => Promise<void> | void;
  onSetRoomPinned: (room: Room, pinned: boolean) => Promise<void>;
  // Desktop panel visibility now lives in RoomView's single “视图” menu.
  // Kept optional so older embedded hosts do not break.
  onTogglePanel?: () => void;
  theme: "light" | "dark";
  onToggleTheme: () => void;
	  onOpenProfile?: () => void;
  onManageHub?: () => void;
  onLogout: () => Promise<void>;
  onClose: () => void;
}

export function WorkspaceSidebar(props: WorkspaceSidebarProps) {
  const { locale, t } = useLocale();
  const [workspaceForm, setWorkspaceForm] = useState(false);
  const [workspaceSettingsForm, setWorkspaceSettingsForm] = useState(false);
  const [roomForm, setRoomForm] = useState(false);
  const [joinHubForm, setJoinHubForm] = useState(false);
  const [name, setName] = useState("");
  const [workspaceSettingsName, setWorkspaceSettingsName] = useState("");
  const [confirmWorkspaceDelete, setConfirmWorkspaceDelete] = useState(false);
  const [savingWorkspaceSettings, setSavingWorkspaceSettings] = useState(false);
  const [inviteLink, setInviteLink] = useState("");
  const [joiningHub, setJoiningHub] = useState(false);
  const [selectedAgentIDs, setSelectedAgentIDs] = useState<string[]>([]);
  const [error, setError] = useState("");
  const [sortMode, setSortMode] = useState(() => window.localStorage.getItem("agentcollab.room-sort") ?? "time_desc");
  const orderedRooms = useMemo(() => {
    const pinned = props.rooms.filter((room) => room.pinned).sort((left, right) =>
      new Date(left.pinned_at ?? left.created_at).getTime() - new Date(right.pinned_at ?? right.created_at).getTime());
    const remaining = props.rooms.filter((room) => !room.pinned).sort((left, right) => {
      if (sortMode === "name_asc" || sortMode === "name_desc") {
        const compared = left.name.localeCompare(right.name, locale, { numeric: true, sensitivity: "base" });
        return sortMode === "name_asc" ? compared : -compared;
      }
      const compared = new Date(left.created_at).getTime() - new Date(right.created_at).getTime();
      return sortMode === "time_asc" ? compared : -compared;
    });
    return [...pinned, ...remaining];
  }, [props.rooms, sortMode, locale]);

  async function create(event: FormEvent, kind: "workspace" | "room") {
    event.preventDefault();
    const value = name.trim();
    if (!value) return;
    setError("");
    try {
      if (kind === "workspace") await props.onCreateWorkspace(value);
      else await props.onCreateRoom(value, selectedAgentIDs);
      setName("");
      setWorkspaceForm(false);
      setRoomForm(false);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("创建失败"));
    }
  }

  function begin(kind: "workspace" | "room") {
    setName("");
    setSelectedAgentIDs([]);
    setError("");
    setWorkspaceForm(kind === "workspace");
    setWorkspaceSettingsForm(false);
    setRoomForm(kind === "room");
  }

  function beginWorkspaceSettings() {
    if (!props.selectedWorkspace) return;
    setWorkspaceSettingsName(props.selectedWorkspace.name);
    setConfirmWorkspaceDelete(false);
    setError("");
    setWorkspaceForm(false);
    setRoomForm(false);
    setJoinHubForm(false);
    setWorkspaceSettingsForm(true);
  }

  async function saveWorkspaceSettings(event: FormEvent) {
    event.preventDefault();
    if (!props.selectedWorkspace) return;
    const value = workspaceSettingsName.trim();
    if (!value) return;
	if (value === props.selectedWorkspace.name) {
		setWorkspaceSettingsForm(false);
		return;
	}
    setSavingWorkspaceSettings(true);
    setError("");
    try {
      if (!props.onUpdateWorkspace) throw new Error(t("当前页面不支持工作区设置"));
      await props.onUpdateWorkspace(props.selectedWorkspace.id, value);
      setWorkspaceSettingsForm(false);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("保存失败"));
    } finally {
      setSavingWorkspaceSettings(false);
    }
  }

  async function deleteWorkspace() {
    if (!props.selectedWorkspace) return;
    setSavingWorkspaceSettings(true);
    setError("");
    try {
      if (!props.onDeleteWorkspace) throw new Error(t("当前页面不支持删除工作区"));
      await props.onDeleteWorkspace(props.selectedWorkspace.id);
      setWorkspaceSettingsForm(false);
      setConfirmWorkspaceDelete(false);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("删除失败"));
    } finally {
      setSavingWorkspaceSettings(false);
    }
  }

  function toggleAgent(agentID: string) {
    setSelectedAgentIDs((current) => current.includes(agentID) ? current.filter((id) => id !== agentID) : [...current, agentID]);
  }

  async function joinHub(event: FormEvent) {
    event.preventDefault();
    if (!inviteLink.trim()) return;
    setError("");
    setJoiningHub(true);
    try {
      await props.onJoinHub(inviteLink.trim());
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("无法打开 Hub"));
    } finally {
      setJoiningHub(false);
    }
  }

  return (
    <>
      <button className={`mobile-scrim${props.open ? " is-visible" : ""}`} onClick={props.onClose} aria-label={t("关闭工作区")} />
      <aside className={`workspace-sidebar${props.open ? " is-open" : ""}`}>
        <header className="sidebar-brand">
          <div className="brand-mark"><CirclesFour weight="fill" size={20} /></div>
          <span>AgentCollab</span>
          <button className="theme-toggle" onClick={props.onToggleTheme} aria-label={props.theme === "light" ? t("切换为深色模式") : t("切换为明亮模式")} title={props.theme === "light" ? t("切换为深色模式") : t("切换为明亮模式")}>
            {props.theme === "light" ? <Moon size={17} /> : <Sun size={17} />}
          </button>
          <button className="mobile-icon-button" onClick={props.onClose} aria-label={t("关闭工作区")}><X size={19} /></button>
        </header>

        <section className="workspace-switcher">
          <div className="sidebar-section-heading">
            <span>{t("工作区")}</span>
            <div className="sidebar-heading-actions">
              <button onClick={() => { setJoinHubForm(true); setWorkspaceForm(false); setWorkspaceSettingsForm(false); setRoomForm(false); setError(""); }} aria-label={t("通过邀请加入")}><LinkSimple size={16} /></button>
              <button onClick={() => begin("workspace")} aria-label={t("创建工作区")}><Plus size={16} /></button>
            </div>
          </div>
          {joinHubForm && (
            <form className="compact-form hub-join-form" onSubmit={joinHub}>
              <strong>{t("通过邀请加入")}</strong>
              <p>{t("粘贴对方发来的完整邀请链接即可。地址、邀请码和目标群聊都已经包含在链接中。")}</p>
              <label htmlFor="join-invite-link">{t("邀请链接")}</label>
              <input id="join-invite-link" value={inviteLink} onChange={(event) => setInviteLink(event.target.value)} autoFocus placeholder={t("http://团队地址/?join=…")} />
              {error && <p className="hub-join-error" role="alert">{error}</p>}
              <div><button type="submit" disabled={joiningHub}>{joiningHub ? t("正在加入") : t("加入团队")}</button><button type="button" onClick={() => setJoinHubForm(false)}>{t("取消")}</button></div>
            </form>
          )}
          {workspaceForm && (
            <form className="compact-form" onSubmit={(event) => void create(event, "workspace")}>
              <label htmlFor="new-workspace">{t("工作区名称")}</label>
              <input id="new-workspace" value={name} onChange={(event) => setName(event.target.value)} autoFocus maxLength={80} />
              {error && <p role="alert">{error}</p>}
              <div><button type="submit">{t("创建")}</button><button type="button" onClick={() => setWorkspaceForm(false)}>{t("取消")}</button></div>
            </form>
          )}
          {workspaceSettingsForm && props.selectedWorkspace && (
            <form className="compact-form workspace-settings-form" onSubmit={(event) => void saveWorkspaceSettings(event)}>
              <strong>{t("工作区设置")}</strong>
              <p>{t("仅工作区 Owner 可以修改名称或删除工作区。")}</p>
              <label htmlFor="workspace-settings-name">{t("工作区名称")}</label>
              <input id="workspace-settings-name" value={workspaceSettingsName} onChange={(event) => setWorkspaceSettingsName(event.target.value)} autoFocus maxLength={80} />
              {error && <p role="alert">{error}</p>}
              <div><button type="submit" disabled={savingWorkspaceSettings}>{savingWorkspaceSettings ? t("正在保存") : t("保存名称")}</button><button type="button" onClick={() => setWorkspaceSettingsForm(false)}>{t("取消")}</button></div>
              {!confirmWorkspaceDelete ? <button className="workspace-delete-button" type="button" onClick={() => setConfirmWorkspaceDelete(true)}>{t("删除此工作区")}</button> : <div className="workspace-delete-confirm" role="alert"><p>{t("会删除此工作区的群聊、消息与仅属于它的 Agent 记录，无法恢复。已在其他工作区使用的本机 Agent 会自动保留，协作不会中断。")}</p><button type="button" disabled={savingWorkspaceSettings} onClick={() => void deleteWorkspace()}>{savingWorkspaceSettings ? t("正在删除") : t("确认删除")}</button><button type="button" onClick={() => setConfirmWorkspaceDelete(false)}>{t("保留工作区")}</button></div>}
            </form>
          )}
          <div className="workspace-list">
            {props.workspaces.map((workspace) => (
              <div className={`workspace-nav-item${workspace.id === props.selectedWorkspace?.id ? " is-selected" : ""}`} key={workspace.id}>
                <button className="workspace-select-button" onClick={() => props.onSelectWorkspace(workspace)}>
                  <span className="workspace-monogram">{workspace.name.slice(0, 1).toUpperCase()}</span>
                  <span>{workspace.name}</span>
                  {workspace.id === props.selectedWorkspace?.id && <CaretDown size={14} />}
                </button>
                {workspace.id === props.selectedWorkspace?.id && workspace.role === "owner" && <button className="workspace-settings-button" type="button" onClick={beginWorkspaceSettings} aria-label={`${t("设置工作区")} ${workspace.name}`} title={t("工作区设置")}><GearSix size={15} /></button>}
              </div>
            ))}
          </div>
        </section>

        <section className="room-navigation">
          <div className="sidebar-section-heading">
            <span>{t("群聊")}</span>
            <div className="sidebar-heading-actions">
              <label className="room-sort-control">
                <span className="sr-only">{t("群聊排序")}</span>
                <select aria-label={t("群聊排序")} value={sortMode} onChange={(event) => {
                  setSortMode(event.target.value);
                  window.localStorage.setItem("agentcollab.room-sort", event.target.value);
                }}>
                  <option value="time_desc">{t("时间：新到旧")}</option>
                  <option value="time_asc">{t("时间：旧到新")}</option>
                  <option value="name_asc">{t("名称：A-Z")}</option>
                  <option value="name_desc">{t("名称：Z-A")}</option>
                </select>
              </label>
              <button disabled={!props.selectedWorkspace} onClick={() => begin("room")} aria-label={t("创建群聊")}><Plus size={16} /></button>
            </div>
          </div>
          {roomForm && (
            <form className="compact-form" onSubmit={(event) => void create(event, "room")}>
              <label htmlFor="new-room">{t("群名称")}</label>
              <input id="new-room" value={name} onChange={(event) => setName(event.target.value)} autoFocus maxLength={80} />
              <fieldset className="room-agent-options">
                <legend>{t("选择 Agent 成员")} <span>{t("可多选")}</span></legend>
                {props.agents.length > 0 ? props.agents.map((agent) => (
                  <label key={agent.id}>
                    <input type="checkbox" checked={selectedAgentIDs.includes(agent.id)} disabled={agent.status !== "online"} onChange={() => toggleAgent(agent.id)} />
                    <span><strong>{agent.display_name}</strong><small>{agent.type}</small></span>
                  </label>
                )) : <p>{t("Agent 库为空，请先在右侧注册 Agent。")}</p>}
              </fieldset>
              {error && <p role="alert">{error}</p>}
              <div><button type="submit">{t("创建")}</button><button type="button" onClick={() => setRoomForm(false)}>{t("取消")}</button></div>
            </form>
          )}
          {props.loading ? (
            <div className="sidebar-skeleton"><span /><span /><span /></div>
          ) : props.rooms.length ? (
            <nav aria-label={t("群聊列表")}>
              {orderedRooms.map((room) => {
                const selected = room.id === props.selectedRoom?.id;
                return <Fragment key={room.id}>
                  <div className={`room-nav-item${selected ? " is-selected" : ""}`}>
                    <button className="room-select-button" onClick={() => props.onSelectRoom(room)}>
                      <ChatCircleDots size={17} weight={selected ? "fill" : "regular"} />
                      <span>{room.name}</span>
                    </button>
                    <button
                      className={`room-pin-button${room.pinned ? " is-pinned" : ""}`}
                      onClick={() => void props.onSetRoomPinned(room, !room.pinned)}
                      aria-label={`${room.pinned ? t("取消置顶") : t("置顶")} ${room.name}`}
                      title={room.pinned ? t("取消置顶") : t("置顶")}
                    >
                      {room.pinned ? <PushPinSlash size={15} /> : <PushPin size={15} />}
                    </button>
                  </div>
                  {selected && <div id="room-member-sidebar-slot" className="room-nav-member-slot" aria-label={`${room.name} ${t("群聊成员")}`} />}
                </Fragment>;
              })}
            </nav>
          ) : (
            <p className="sidebar-empty">{props.selectedWorkspace ? t("还没有群聊。点击右上角 + 手动创建。") : t("先选择工作区")}</p>
          )}
        </section>

        <footer className="user-footer">
          <button className="user-profile-trigger" type="button" onClick={props.onOpenProfile} aria-label={t("打开个人资料")}>
            <span className="user-avatar">{props.currentUser.avatar_url ? <img src={props.currentUser.avatar_url} alt="" /> : props.currentUser.display_name.slice(0, 1).toUpperCase()}</span>
            <span><strong>{props.currentUser.display_name}</strong><small>@{props.currentUser.username} · {props.surface === "client" ? "Client" : t("网页版")} · {props.currentUser.role === "admin" ? t("Hub 管理员") : t("成员")}</small></span>
          </button>
          {props.currentUser.role === "admin" && <button type="button" onClick={props.onManageHub} aria-label={t("打开 Hub 账号管理")} title={t("Hub 账号管理")}><ShieldCheck size={18} /></button>}
          <button onClick={() => void props.onLogout()} aria-label={t("退出登录")}><SignOut size={18} /></button>
        </footer>
      </aside>
    </>
  );
}
