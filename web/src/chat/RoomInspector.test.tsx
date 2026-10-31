import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type Agent, type ProjectPlan, type Room, type User, type Workspace } from "../api/client";
import { RoomInspector } from "./RoomInspector";
import { setLocale } from "../i18n/locale";

beforeEach(() => {
  setLocale("zh-CN");
  window.localStorage.clear();
});

afterEach(() => {
	act(() => setLocale("zh-CN"));
	document.getElementById("room-member-sidebar-slot")?.remove();
	vi.unstubAllGlobals();
	vi.restoreAllMocks();
});

const member: User = { id: "user-1", username: "owner", display_name: "Owner", role: "admin", workspace_role: "owner", room_role: "owner" };
const room: Room = { id: "room-1", workspace_id: "workspace-1", name: "方案讨论", created_at: "2026-07-16T03:00:00Z" };
const workspace: Workspace = { id: "workspace-1", name: "本地协作", owner_user_id: "user-1", role: "owner", created_at: "2026-07-16T02:00:00Z", updated_at: "2026-07-16T02:00:00Z" };
const agent: Agent = {
  id: "agent-1", display_name: "Codex Local", type: "codex", owner_user_id: "user-1", owner_name: "Owner",
  device_id: "device-1", device_name: "MacBook", os: "darwin", arch: "arm64", channel: "runner/darwin", permission_mode: "admin", status: "online",
  imported: true, available: true, version: "codex 0.132.0", executable: "/opt/homebrew/bin/codex", native_history: "Codex session",
  skills: [{ name: "stock-analysis", description: "分析本机股票数据" }], resources: [{ name: "research-db", description: "本地研究数据库", access: "read_write" }],
  response_mode: "mention", reply_delay_seconds: 0, role_prompt: "", max_auto_turns: 4,
  visibility: "workspace", invocation_scope: "owner_only", shared_skills: [], can_manage: true, can_invoke: true,
};

test("English member section counts and collapse controls retain their behavior", async () => {
  setLocale("en");
  const user = userEvent.setup();
  render(<RoomInspector open room={room} workspace={workspace} currentUser={member} members={[member]} agents={[]} workspaceAgents={[]} verifying={false}
    onRegisterAgent={vi.fn()} onCreateHumanInvite={vi.fn()} onAddRoomAgent={vi.fn()} onRemoveRoomAgent={vi.fn()}
    onRemoveWorkspaceAgent={vi.fn()} onUpdateAgentSettings={vi.fn()} onVerifyAgents={vi.fn()} onClose={vi.fn()} />);
  expect(screen.getByRole("heading", { name: "Room members People: 1 / Agents: 0" })).toBeInTheDocument();
  expect(screen.getByText("Participants")).toBeInTheDocument();
  expect(screen.getByText("Room owner")).toBeInTheDocument();
  expect(screen.getByText(/No agents connected/)).toBeInTheDocument();
  expect(screen.getByText("This member has not connected an agent")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Collapse room members" }));
  expect(screen.queryByText("Participants")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Expand room members" })).toHaveAttribute("aria-expanded", "false");
  await user.click(screen.getByRole("button", { name: "Expand room members" }));
  expect(screen.getByText("Participants")).toBeInTheDocument();
});

test("English invitation entry preserves the link and invokes the same join callback", async () => {
  setLocale("en");
  const user = userEvent.setup();
  const join = vi.fn().mockResolvedValue(undefined);
  const register = vi.fn();
  render(<RoomInspector open room={room} workspace={workspace} currentUser={member} members={[member]} agents={[]} workspaceAgents={[]} verifying={false}
    onJoinWorkspace={join} onRegisterAgent={register} onCreateHumanInvite={vi.fn()} onAddRoomAgent={vi.fn()} onRemoveRoomAgent={vi.fn()}
    onRemoveWorkspaceAgent={vi.fn()} onUpdateAgentSettings={vi.fn()} onVerifyAgents={vi.fn()} onClose={vi.fn()} />);
  await user.click(screen.getByRole("button", { name: "Invitations and connections" }));
  await user.click(screen.getByRole("button", { name: "Enter invitation link" }));
  const link = "http://team.example/?join=demo-code";
  await user.type(screen.getByLabelText("Team invitation link"), link);
  act(() => setLocale("zh-CN"));
  expect(screen.getByLabelText("团队邀请链接")).toHaveValue(link);
  act(() => setLocale("en"));
  await user.click(screen.getByRole("button", { name: "Join team" }));
  expect(join).toHaveBeenCalledWith(link);
  await user.click(screen.getByRole("button", { name: "Open local agents" }));
  expect(register).toHaveBeenCalledTimes(1);
  expect(screen.getByLabelText("Team invitation link")).toHaveValue(link);
});

test("English room tools expose settings and preserve room identity", async () => {
  setLocale("en");
  const user = userEvent.setup();
  const settings = vi.fn();
  render(<RoomInspector open room={room} workspace={workspace} currentUser={member} members={[member]} agents={[]} workspaceAgents={[]} verifying={false}
    onManageAgentSettings={settings} onRegisterAgent={vi.fn()} onCreateHumanInvite={vi.fn()} onAddRoomAgent={vi.fn()} onRemoveRoomAgent={vi.fn()}
    onRemoveWorkspaceAgent={vi.fn()} onUpdateAgentSettings={vi.fn()} onVerifyAgents={vi.fn()} onClose={vi.fn()} />);
  expect(screen.getByText("Room tools")).toBeInTheDocument();
  expect(screen.getByText("# 方案讨论")).toBeInTheDocument();
  expect(screen.getByText("No agents waiting to join. Find joined agents under Room members on the left.")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Run checks" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "My agent settings" }));
  expect(settings).toHaveBeenCalledOnce();
  await user.click(screen.getByRole("button", { name: "Open invitations and connections" }));
  expect(screen.getByRole("button", { name: "Open local agents" })).toBeInTheDocument();
});

test.each([true, false])("English default step preserves permissions: can_manage=%s", async (canManage) => {
  setLocale("en");
  const user = userEvent.setup();
  const update = vi.fn().mockResolvedValue(undefined);
  const plan: ProjectPlan = {
    workspace_id: workspace.id, objective: "团队目标", status: "active", progress: 0, can_manage: canManage,
    steps: [{ id: "step-1", workspace_id: workspace.id, title: "审查报告", description: "", weight: 1, status: "planned", sort_order: 0, progress: 0, linked_item_count: 0, responsible_user_ids: [], responsible_agent_ids: [], created_at: room.created_at, updated_at: room.created_at }],
  };
  render(<RoomInspector open room={room} workspace={workspace} currentUser={member} members={[member]} agents={[]} workspaceAgents={[]} verifying={false}
    projectPlan={plan} onUpdateRoomProjectStep={update} onRegisterAgent={vi.fn()} onCreateHumanInvite={vi.fn()} onAddRoomAgent={vi.fn()} onRemoveRoomAgent={vi.fn()}
    onRemoveWorkspaceAgent={vi.fn()} onUpdateAgentSettings={vi.fn()} onVerifyAgents={vi.fn()} onClose={vi.fn()} />);
  const select = screen.getByRole("combobox", { name: "Default project step for this room" });
  expect(within(select).getByRole("option", { name: "审查报告" })).toHaveValue("step-1");
  if (canManage) {
    await user.selectOptions(select, "step-1");
    expect(update).toHaveBeenLastCalledWith("step-1");
    await user.selectOptions(select, "");
    expect(update).toHaveBeenLastCalledWith(undefined);
  } else {
    expect(select).toBeDisabled();
    expect(update).not.toHaveBeenCalled();
  }
});

test("显示群聊成员和可添加 Agent，并可加入当前群", async () => {
  const user = userEvent.setup();
  const onAddRoomAgent = vi.fn().mockResolvedValue(undefined);
  render(<RoomInspector
    open
    room={room}
    members={[member]}
    agents={[]}
    workspaceAgents={[agent]}
    verifying={false}
    onRegisterAgent={vi.fn()}
    onCreateHumanInvite={vi.fn()}
    onCreateRunnerInvite={vi.fn()}
    onAddRoomAgent={onAddRoomAgent}
    onRemoveRoomAgent={vi.fn()}
    onRemoveWorkspaceAgent={vi.fn()}
    onUpdateAgentSettings={vi.fn()}
    onVerifyAgents={vi.fn()}
    onTogglePanel={vi.fn()}
    onClose={vi.fn()}
  />);

  expect(screen.getByRole("heading", { name: "群聊成员 1 真人 / 0 Agent" })).toBeInTheDocument();
  expect(screen.getByRole("heading", { name: "可添加 Agent 1" })).toBeInTheDocument();
	expect(screen.getByText("协作席位")).toBeInTheDocument();
	expect(screen.getAllByText("Owner")).toHaveLength(2);
	expect(screen.getByText("@owner，未接入 Agent")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "将 Codex Local 加入群聊" }));
  expect(onAddRoomAgent).toHaveBeenCalledWith("agent-1");
	 expect(await screen.findByText("Codex Local 已加入当前群聊，现在可在输入框 @ 它。")).toBeInTheDocument();
});

test("问号说明使用即时浮层而不是浏览器延迟 title", async () => {
  const user = userEvent.setup();
  render(<RoomInspector
    open room={room} members={[member]} agents={[]} workspaceAgents={[agent]} verifying={false}
    onRegisterAgent={vi.fn()} onCreateHumanInvite={vi.fn()} onAddRoomAgent={vi.fn()} onRemoveRoomAgent={vi.fn()} onRemoveWorkspaceAgent={vi.fn()}
    onUpdateAgentSettings={vi.fn()} onVerifyAgents={vi.fn()} onClose={vi.fn()}
  />);

  const help = screen.getByLabelText(/可添加 Agent 说明/);
  expect(help).not.toHaveAttribute("title");
  expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
  await user.hover(help);
  expect(screen.getByRole("tooltip")).toHaveTextContent("这里只列尚未加入当前群聊的 Agent");
  await user.unhover(help);
  expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
  fireEvent.focus(help);
  expect(screen.getByRole("tooltip")).toBeInTheDocument();
});

test("完整应用中把群聊成员迁入左侧当前群聊的二级区域", async () => {
  const slot = document.createElement("div");
  slot.id = "room-member-sidebar-slot";
  document.body.appendChild(slot);
  const { container } = render(<RoomInspector
    open room={room} workspace={workspace} currentUser={member} members={[member]} agents={[agent]} workspaceAgents={[agent]} verifying={false}
    onRegisterAgent={vi.fn()} onCreateHumanInvite={vi.fn()} onAddRoomAgent={vi.fn()} onRemoveRoomAgent={vi.fn()} onRemoveWorkspaceAgent={vi.fn()}
    onUpdateAgentSettings={vi.fn()} onVerifyAgents={vi.fn()} onClose={vi.fn()}
  />);

  await waitFor(() => expect(within(slot).getByRole("heading", { name: "群聊成员 1 真人 / 1 Agent" })).toBeInTheDocument());
  expect(within(slot).getByText("Owner")).toBeInTheDocument();
  expect(within(slot).getByText("Codex Local")).toBeInTheDocument();
  expect(within(container).queryByRole("heading", { name: "群聊成员 1 真人 / 1 Agent" })).not.toBeInTheDocument();
  expect(within(container).getByRole("button", { name: "邀请与接入" })).toBeInTheDocument();
});

test("单 Agent 和多 Agent 都可按真人独立展开和折叠", async () => {
  const user = userEvent.setup();
  const slot = document.createElement("div");
  slot.id = "room-member-sidebar-slot";
  document.body.appendChild(slot);
  const teammate: User = { id: "user-2", username: "alice", display_name: "Alice", role: "member", workspace_role: "member", room_role: "member" };
  const teammateAgent: Agent = { ...agent, id: "agent-2", display_name: "Alice Hermes", owner_user_id: teammate.id, owner_name: teammate.display_name };
  const ownerSecondAgent: Agent = { ...agent, id: "agent-3", display_name: "Owner WorkBuddy", type: "workbuddy" };
  render(<RoomInspector
    open room={room} workspace={workspace} currentUser={member} members={[member, teammate]} agents={[agent, ownerSecondAgent, teammateAgent]} workspaceAgents={[agent, ownerSecondAgent, teammateAgent]} verifying={false}
    onRegisterAgent={vi.fn()} onCreateHumanInvite={vi.fn()} onAddRoomAgent={vi.fn()} onRemoveRoomAgent={vi.fn()} onRemoveWorkspaceAgent={vi.fn()}
    onUpdateAgentSettings={vi.fn()} onVerifyAgents={vi.fn()} onClose={vi.fn()}
  />);

  await waitFor(() => expect(within(slot).getByRole("button", { name: "展开 Owner 名下 Agent" })).toBeInTheDocument());
  expect(within(slot).getByRole("button", { name: "折叠 Alice 名下 Agent" })).toBeInTheDocument();
  expect(within(slot).getByText("Hermes")).toBeInTheDocument();
  expect(within(slot).queryByText("Codex Local")).not.toBeInTheDocument();
  await user.click(within(slot).getByRole("button", { name: "折叠 Alice 名下 Agent" }));
  expect(within(slot).queryByText("Hermes")).not.toBeInTheDocument();
  await user.click(within(slot).getByRole("button", { name: "展开 Alice 名下 Agent" }));
  await user.click(within(slot).getByRole("button", { name: "展开 Owner 名下 Agent" }));
  expect(within(slot).getByText("Codex Local")).toBeInTheDocument();
  expect(within(slot).getByText("WorkBuddy")).toBeInTheDocument();
  await user.click(within(slot).getByRole("button", { name: "折叠 Owner 名下 Agent" }));
  expect(within(slot).queryByText("Codex Local")).not.toBeInTheDocument();
  expect(within(slot).getByText("Hermes")).toBeInTheDocument();
});

test("新版布局首次打开时忽略旧版展开偏好，多 Agent 默认折叠", async () => {
  window.localStorage.setItem("agentcollab.member-tree-collapsed", JSON.stringify({ "user-1": false }));
  window.localStorage.setItem("agentcollab.agent-library-owner-collapsed", JSON.stringify({ "user-1": false }));
  const slot = document.createElement("div");
  slot.id = "room-member-sidebar-slot";
  document.body.appendChild(slot);
  const ownerSecondAgent: Agent = { ...agent, id: "agent-legacy-layout", display_name: "Owner WorkBuddy", type: "workbuddy" };
  render(<RoomInspector
    open room={room} workspace={workspace} currentUser={member} members={[member]} workspaceMembers={[member]}
    agents={[agent, ownerSecondAgent]} workspaceAgents={[agent, ownerSecondAgent]} verifying={false}
    onRegisterAgent={vi.fn()} onCreateHumanInvite={vi.fn()} onAddRoomAgent={vi.fn()} onRemoveRoomAgent={vi.fn()} onRemoveWorkspaceAgent={vi.fn()}
    onUpdateAgentSettings={vi.fn()} onVerifyAgents={vi.fn()} onClose={vi.fn()}
  />);

  await waitFor(() => expect(within(slot).getByRole("button", { name: "展开 Owner 名下 Agent" })).toBeInTheDocument());
  expect(within(slot).queryByText("Codex Local")).not.toBeInTheDocument();
  expect(window.localStorage.getItem("agentcollab.member-tree-collapsed-v2")).toBeNull();
});

test("可添加 Agent 即使每人只有一个也可按所有者独立折叠", async () => {
  const user = userEvent.setup();
  const teammate: User = { id: "user-2", username: "alice", display_name: "Alice", role: "member", workspace_role: "member", room_role: "member" };
  const teammateAgent: Agent = { ...agent, id: "agent-2", display_name: "Alice Hermes", owner_user_id: teammate.id, owner_name: teammate.display_name };
  const ownerSecondAgent: Agent = { ...agent, id: "agent-3", display_name: "Owner WorkBuddy", type: "workbuddy" };
  render(<RoomInspector
    open room={room} workspace={workspace} currentUser={member} members={[member, teammate]} workspaceMembers={[member, teammate]} agents={[]} workspaceAgents={[agent, ownerSecondAgent, teammateAgent]} verifying={false}
    onRegisterAgent={vi.fn()} onCreateHumanInvite={vi.fn()} onAddRoomAgent={vi.fn()} onRemoveRoomAgent={vi.fn()} onRemoveWorkspaceAgent={vi.fn()}
    onUpdateAgentSettings={vi.fn()} onVerifyAgents={vi.fn()} onClose={vi.fn()}
  />);

  expect(screen.queryByText("Codex Local")).not.toBeInTheDocument();
  expect(screen.getByText("Hermes")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "折叠可添加 Agent 中 Alice 名下 Agent" }));
  expect(screen.queryByText("Hermes")).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "展开可添加 Agent 中 Owner 名下 Agent" }));
  expect(screen.getByText("Codex Local")).toBeInTheDocument();
  expect(screen.getByText("WorkBuddy")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "展开可添加 Agent 中 Alice 名下 Agent" }));
  expect(screen.getByText("Hermes")).toBeInTheDocument();
});

test("群主可将他人已共享但暂时离线的 Agent 加入当前群", async () => {
  const user = userEvent.setup();
  const onAddRoomAgent = vi.fn().mockResolvedValue(undefined);
  const teammateAgent: Agent = { ...agent, id: "agent-remote", display_name: "Alice Hermes", type: "hermes", owner_user_id: "user-2", owner_name: "Alice", device_name: "Alice PC", status: "offline", can_manage: false, can_moderate: true, can_invoke: true, invocation_scope: "room_members", runner_version: "dev" };
  render(<RoomInspector
    open room={room} workspace={workspace} currentUser={member} members={[member]} workspaceAgents={[teammateAgent]} agents={[]} verifying={false}
    onRegisterAgent={vi.fn()} onCreateHumanInvite={vi.fn()} onCreateRunnerInvite={vi.fn()} onAddRoomAgent={onAddRoomAgent}
    onRemoveRoomAgent={vi.fn()} onRemoveWorkspaceAgent={vi.fn()} onUpdateAgentSettings={vi.fn()} onVerifyAgents={vi.fn()} onTogglePanel={vi.fn()} onClose={vi.fn()}
  />);

  await user.click(screen.getByRole("button", { name: "将 Alice Hermes 加入群聊" }));
  expect(onAddRoomAgent).toHaveBeenCalledWith("agent-remote");
  expect(await screen.findByText("Alice Hermes 已加入当前群聊；目标电脑 Client 当前离线，恢复在线后即可在输入框 @ 它。")).toBeInTheDocument();
});

test("Agent 名称旁显示真实运行状态，并为离线设备给出简化恢复入口", () => {
  const offlineAgent: Agent = { ...agent, id: "agent-offline", display_name: "Alice Hermes", owner_user_id: "user-2", owner_name: "Alice", status: "offline", can_manage: false, can_moderate: true };
  render(<RoomInspector
    open room={room} workspace={workspace} currentUser={member} members={[member]} agents={[offlineAgent]} workspaceAgents={[offlineAgent]} verifying={false}
    onRegisterAgent={vi.fn()} onCreateHumanInvite={vi.fn()} onCreateRunnerInvite={vi.fn()} onAddRoomAgent={vi.fn()} onRemoveRoomAgent={vi.fn()} onRemoveWorkspaceAgent={vi.fn()}
    onUpdateAgentSettings={vi.fn()} onVerifyAgents={vi.fn()} onTogglePanel={vi.fn()} onClose={vi.fn()}
  />);

	expect(screen.getAllByLabelText("Alice Hermes 运行状态：离线，等待目标电脑的 Client 自动重连。")).toHaveLength(1);
});

test.each([
  ["online", 0, "Ready", "Client connected; ready to receive tasks."],
  ["online", 2, "Queued", "Client connected; 2 tasks queued."],
  ["busy", 2, "Running", "Client connected; active tasks: 1, with 2 more queued."],
  ["unavailable", 0, "Unavailable", "Client connected, but no executable Agent was found on this computer."],
  ["offline", 2, "Waiting for connection", "2 tasks queued; execution resumes after the target Client reconnects."],
  ["offline", 0, "Offline", "Waiting for the target computer's Client to reconnect."],
] as const)("English runtime status %s with %s queued tasks", (status, queued, label, description) => {
  setLocale("en");
  const runtimeAgent: Agent = { ...agent, status, queued_task_count: queued, running_task_count: status === "busy" ? 1 : 0 };
  render(<RoomInspector open room={room} workspace={workspace} currentUser={member} members={[member]} agents={[runtimeAgent]} workspaceAgents={[runtimeAgent]} verifying={false}
    onRegisterAgent={vi.fn()} onCreateHumanInvite={vi.fn()} onAddRoomAgent={vi.fn()} onRemoveRoomAgent={vi.fn()} onRemoveWorkspaceAgent={vi.fn()}
    onUpdateAgentSettings={vi.fn()} onVerifyAgents={vi.fn()} onClose={vi.fn()} />);
  const badge = screen.getByTitle(description);
  expect(badge).toHaveTextContent(label);
  expect(badge.getAttribute("aria-label")).toContain(`Status: ${label}`);
  act(() => setLocale("zh-CN"));
  expect(screen.queryByTitle(description)).not.toBeInTheDocument();
});

test("运行状态区分排队、执行与可续接原生会话", () => {
  const runningAgent: Agent = { ...agent, status: "busy", queued_task_count: 2, running_task_count: 1, native_session_ready: true };
  render(<RoomInspector
    open room={room} workspace={workspace} currentUser={member} members={[member]} agents={[runningAgent]} workspaceAgents={[runningAgent]} verifying={false}
    onRegisterAgent={vi.fn()} onCreateHumanInvite={vi.fn()} onAddRoomAgent={vi.fn()} onRemoveRoomAgent={vi.fn()} onRemoveWorkspaceAgent={vi.fn()}
    onUpdateAgentSettings={vi.fn()} onVerifyAgents={vi.fn()} onClose={vi.fn()}
  />);

  expect(screen.getAllByText("执行中")).toHaveLength(1);
  expect(screen.getAllByLabelText(/正在执行 1 项任务，另有 2 项排队.*已保存原生会话/)).toHaveLength(1);
});

test("群详情只管理协作成员，不再混入 Client 配对和设备运维", () => {
  render(<RoomInspector
    open room={room} workspace={workspace} currentUser={member} members={[member]} agents={[agent]} workspaceAgents={[agent]} verifying={false}
    onRegisterAgent={vi.fn()} onCreateHumanInvite={vi.fn()} onAddRoomAgent={vi.fn()} onRemoveRoomAgent={vi.fn()} onRemoveWorkspaceAgent={vi.fn()}
    onUpdateAgentSettings={vi.fn()} onVerifyAgents={vi.fn()} onClose={vi.fn()}
  />);

  expect(screen.queryByText("我的设备与 Agent")).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "添加电脑" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "连接本机" })).not.toBeInTheDocument();
  expect(screen.queryByText(/配对码/)).not.toBeInTheDocument();
  expect(screen.getByRole("heading", { name: "群聊成员 1 真人 / 1 Agent" })).toBeInTheDocument();
  expect(screen.getByRole("heading", { name: "可添加 Agent 0" })).toBeInTheDocument();
});

test("未选择群聊时解释下一步，不显示伪成员和可误点操作", () => {
  const other: User = { id: "user-2", username: "alice", display_name: "Alice", role: "member", workspace_role: "member" };
  render(<RoomInspector
    open workspace={workspace} currentUser={member} members={[]} workspaceMembers={[member, other]} agents={[]} workspaceAgents={[agent]} verifying={false}
    onRegisterAgent={vi.fn()} onCreateHumanInvite={vi.fn()} onAddRoomAgent={vi.fn()} onRemoveRoomAgent={vi.fn()} onRemoveWorkspaceAgent={vi.fn()}
    onUpdateAgentSettings={vi.fn()} onVerifyAgents={vi.fn()} onClose={vi.fn()}
  />);

  expect(screen.getByText("先创建或选择群聊")).toBeInTheDocument();
  expect(screen.getByText(/工作区不会自动创建群聊/)).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "邀请与接入" })).toBeEnabled();
  expect(screen.queryByText("Alice")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "将 Codex Local 加入群聊" })).toBeDisabled();
});

test("群主可设置群管理员，移出群不会调用工作区删除", async () => {
  const user = userEvent.setup();
  const teammate: User = { id: "user-2", username: "alice", display_name: "Alice", role: "member", workspace_role: "member", room_role: "member" };
  const onUpdateRoomMemberRole = vi.fn().mockResolvedValue(undefined);
  const onRemoveRoomMember = vi.fn().mockResolvedValue(undefined);
  const onRemoveWorkspaceMember = vi.fn().mockResolvedValue(undefined);
  render(<RoomInspector
    open room={room} workspace={workspace} currentUser={member} members={[member, teammate]} workspaceMembers={[member, teammate]} agents={[]} workspaceAgents={[]} verifying={false}
    onRegisterAgent={vi.fn()} onCreateHumanInvite={vi.fn()} onCreateRunnerInvite={vi.fn()} onAddRoomAgent={vi.fn()} onRemoveRoomAgent={vi.fn()} onRemoveWorkspaceAgent={vi.fn()}
    onUpdateAgentSettings={vi.fn()} onUpdateRoomMemberRole={onUpdateRoomMemberRole} onRemoveRoomMember={onRemoveRoomMember} onRemoveWorkspaceMember={onRemoveWorkspaceMember}
    onVerifyAgents={vi.fn()} onTogglePanel={vi.fn()} onClose={vi.fn()}
  />);

  expect(screen.getByText("群主")).toBeInTheDocument();
  await user.selectOptions(screen.getByRole("combobox", { name: "Alice 群内角色" }), "admin");
  expect(onUpdateRoomMemberRole).toHaveBeenCalledWith("user-2", "admin");
  await user.click(screen.getByRole("button", { name: "将 Alice 移出当前群" }));
  expect(screen.getByText(/工作区账号、设备和 Agent 库不受影响/)).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "确认移出" }));
  expect(onRemoveRoomMember).toHaveBeenCalledWith("user-2");
  expect(onRemoveWorkspaceMember).not.toHaveBeenCalled();
});

test("未入群账号只显示一次，并在同一区域提供加入与工作区管理", async () => {
  const user = userEvent.setup();
  const other: User = { id: "user-3", username: "bob", display_name: "Bob", role: "member", workspace_role: "member" };
  const onAddRoomMember = vi.fn().mockResolvedValue(undefined);
  render(<RoomInspector
    open room={room} workspace={workspace} currentUser={member} members={[member]} workspaceMembers={[member, other]} agents={[]} workspaceAgents={[]} verifying={false}
    onRegisterAgent={vi.fn()} onCreateHumanInvite={vi.fn()} onCreateRunnerInvite={vi.fn()} onAddRoomMember={onAddRoomMember}
    onAddRoomAgent={vi.fn()} onRemoveRoomAgent={vi.fn()} onRemoveWorkspaceAgent={vi.fn()} onUpdateAgentSettings={vi.fn()}
    onUpdateWorkspaceMemberRole={vi.fn()} onRemoveWorkspaceMember={vi.fn()} onVerifyAgents={vi.fn()} onTogglePanel={vi.fn()} onClose={vi.fn()}
  />);

  await user.click(screen.getByText("其他工作区账号 · 1"));
  expect(screen.getAllByText("Bob")).toHaveLength(1);
  expect(screen.getByRole("combobox", { name: "Bob 工作区角色" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "从工作区删除 Bob" })).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "将 Bob 加入当前群" }));
  expect(onAddRoomMember).toHaveBeenCalledWith("user-3");
});

test.each([true, false])("English agent drawer respects ownership: can_manage=%s", async (canManage) => {
  setLocale("en");
  const user = userEvent.setup();
  const update = vi.fn().mockResolvedValue(undefined);
  const settings = vi.fn();
  const selected = { ...agent, can_manage: canManage, role_prompt: "保留原始职责" };
  render(<RoomInspector open room={room} workspace={workspace} currentUser={member} members={[member]} agents={[selected]} workspaceAgents={[selected]} verifying={false}
    onRegisterAgent={vi.fn()} onCreateHumanInvite={vi.fn()} onAddRoomAgent={vi.fn()} onRemoveRoomAgent={vi.fn()} onRemoveWorkspaceAgent={vi.fn()}
    onUpdateAgentSettings={update} onManageAgentSettings={settings} onVerifyAgents={vi.fn()} onClose={vi.fn()} />);
  expect(screen.getByText(/Agents: 1, online: 1/)).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "View Codex Local details and room settings" }));
  const drawer = screen.getByRole("dialog", { name: "Codex Local room settings" });
  expect(within(drawer).getByText("Room behavior")).toBeInTheDocument();
  expect(drawer.textContent?.replaceAll("保留原始职责", "")).not.toMatch(/[\u4e00-\u9fff]/);
  if (canManage) {
    const mode = within(drawer).getByRole("combobox", { name: "Codex Local response mode" });
    expect(within(mode).getAllByRole("option").map(option => option.textContent)).toEqual(["Only when @mentioned", "Coordinator / automatic routing", "Reply to all human messages", "Muted"]);
    await user.selectOptions(mode, "human_all");
    expect(update).toHaveBeenCalledWith(agent.id, { response_mode: "human_all" });
    expect(within(drawer).getByRole("textbox", { name: "Codex Local room role" })).toHaveValue("保留原始职责");
    expect(within(drawer).getByText("1 local skills discovered; 0 shared with others.")).toBeInTheDocument();
    await user.click(within(drawer).getByRole("button", { name: "Manage skill permissions" }));
    expect(settings).toHaveBeenCalledOnce();
  } else {
    expect(within(drawer).queryByRole("combobox")).not.toBeInTheDocument();
    expect(within(drawer).queryByRole("button", { name: "Manage skill permissions" })).not.toBeInTheDocument();
    expect(within(drawer).getByText("Only the agent owner can change shared skills. Room administrators manage membership only.")).toBeInTheDocument();
    expect(update).not.toHaveBeenCalled();
    await user.click(within(drawer).getByRole("button", { name: "Close agent room settings" }));
  }
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

test("可为群内 Agent 选择自动响应方式", async () => {
  const user = userEvent.setup();
  const onUpdateAgentSettings = vi.fn().mockResolvedValue(undefined);
  render(<RoomInspector
    open room={room} members={[member]} agents={[agent]} workspaceAgents={[agent]} verifying={false}
    onRegisterAgent={vi.fn()} onCreateHumanInvite={vi.fn()} onCreateRunnerInvite={vi.fn()} onAddRoomAgent={vi.fn()} onUpdateAgentSettings={onUpdateAgentSettings}
    onRemoveRoomAgent={vi.fn()} onRemoveWorkspaceAgent={vi.fn()} onVerifyAgents={vi.fn()} onTogglePanel={vi.fn()} onClose={vi.fn()}
  />);

  await user.click(screen.getByRole("button", { name: "查看 Codex Local 详情与群内设置" }));
  const responseMode = screen.getByRole("combobox", { name: "Codex Local 响应方式" });
  expect(screen.queryByRole("option", { name: "头脑风暴成员" })).not.toBeInTheDocument();
  await user.selectOptions(responseMode, "human_all");
  expect(onUpdateAgentSettings).toHaveBeenCalledWith("agent-1", { response_mode: "human_all" });
});

test("Agent 权限集中入口跳转到个人设置，群成员行只保留摘要", async () => {
  const user = userEvent.setup();
  const onManageAgentSettings = vi.fn();
  render(<RoomInspector
    open room={room} workspace={workspace} currentUser={member} members={[member]} agents={[agent]} workspaceAgents={[agent]} verifying={false}
    onRegisterAgent={vi.fn()} onCreateHumanInvite={vi.fn()} onCreateRunnerInvite={vi.fn()} onAddRoomAgent={vi.fn()} onRemoveRoomAgent={vi.fn()} onRemoveWorkspaceAgent={vi.fn()}
    onUpdateAgentSettings={vi.fn()} onUpdateAgentPolicy={vi.fn()} onManageAgentSettings={onManageAgentSettings} onVerifyAgents={vi.fn()} onTogglePanel={vi.fn()} onClose={vi.fn()}
  />);

  expect(screen.getByText("开放 0/1 Skills")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "查看 Codex Local 详情与群内设置" })).toBeInTheDocument();
  expect(screen.queryByText("权限设置")).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "我的 Agent 设置" }));
  expect(onManageAgentSettings).toHaveBeenCalledTimes(1);
});

test("非主人只能看到授权摘要，不能修改其他人的 Agent", () => {
  const teammate: User = { id: "user-2", username: "alice", display_name: "Alice", role: "member", workspace_role: "member", room_role: "member" };
  const protectedAgent: Agent = { ...agent, can_manage: false, can_invoke: false, skills: [], resources: [], shared_skills: [] };
  render(<RoomInspector
    open room={room} workspace={{ ...workspace, role: "member" }} currentUser={teammate} members={[member, teammate]} agents={[protectedAgent]} workspaceAgents={[protectedAgent]} verifying={false}
    onRegisterAgent={vi.fn()} onCreateHumanInvite={vi.fn()} onCreateRunnerInvite={vi.fn()} onAddRoomAgent={vi.fn()} onRemoveRoomAgent={vi.fn()} onRemoveWorkspaceAgent={vi.fn()}
    onUpdateAgentSettings={vi.fn()} onUpdateAgentPolicy={vi.fn()} onVerifyAgents={vi.fn()} onTogglePanel={vi.fn()} onClose={vi.fn()}
  />);

  expect(screen.queryByText("权限设置")).not.toBeInTheDocument();
  expect(screen.queryByRole("combobox", { name: "Codex Local 响应方式" })).not.toBeInTheDocument();
  expect(screen.getByText("仅 @")).toBeInTheDocument();
  expect(screen.getByText("可调用 0 Skills")).toBeInTheDocument();
  expect(screen.queryByRole("checkbox", { name: "stock-analysis" })).not.toBeInTheDocument();
});

test("群聊成员整体折叠时同时收起真人名下的 Agent", async () => {
  window.localStorage.clear();
  const user = userEvent.setup();
  render(<RoomInspector
    open room={room} members={[member]} agents={[agent]} workspaceAgents={[]} verifying={false}
    onRegisterAgent={vi.fn()} onCreateHumanInvite={vi.fn()} onCreateRunnerInvite={vi.fn()} onAddRoomAgent={vi.fn()} onUpdateAgentSettings={vi.fn()}
    onRemoveRoomAgent={vi.fn()} onRemoveWorkspaceAgent={vi.fn()} onVerifyAgents={vi.fn()} onTogglePanel={vi.fn()} onClose={vi.fn()}
  />);

  expect(screen.getByText("@owner，1 个 Agent，1 个在线")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "折叠群聊成员" }));
  expect(screen.getByRole("button", { name: "展开群聊成员" })).toBeInTheDocument();
  expect(screen.queryByRole("combobox", { name: "Codex Local 响应方式" })).not.toBeInTheDocument();
  expect(window.localStorage.getItem("agentcollab.inspector-collapsed")).toContain('"members":true');
});

test("多人多 Agent 按所有者形成二级组织树", () => {
	const alice: User = { id: "user-2", username: "alice", display_name: "Alice", role: "member", workspace_role: "member", room_role: "member" };
	const aliceAgent: Agent = { ...agent, id: "agent-2", display_name: "Alice Hermes", type: "hermes", owner_user_id: "user-2", owner_name: "Alice", can_manage: false, invocation_scope: "room_members" };
	render(<RoomInspector
		open room={room} workspace={workspace} currentUser={member} members={[member, alice]} workspaceMembers={[member, alice]} agents={[agent, aliceAgent]} workspaceAgents={[agent, aliceAgent]} verifying={false}
		onRegisterAgent={vi.fn()} onCreateHumanInvite={vi.fn()} onAddRoomAgent={vi.fn()} onRemoveRoomAgent={vi.fn()} onRemoveWorkspaceAgent={vi.fn()}
		onUpdateAgentSettings={vi.fn()} onVerifyAgents={vi.fn()} onClose={vi.fn()}
	/>);

	expect(screen.getByText("@owner，1 个 Agent，1 个在线")).toBeInTheDocument();
	expect(screen.getByText("@alice，1 个 Agent，1 个在线")).toBeInTheDocument();
	expect(screen.getAllByText("Codex Local")).toHaveLength(1);
	expect(screen.getAllByText("Hermes")).toHaveLength(1);
});

test("群成员卡片联动显示已授权与本机发现 Skill 数量，详细权限留在个人设置", async () => {
  const user = userEvent.setup();
  const skills = Array.from({ length: 39 }, (_, index) => ({ name: index === 0 ? "stock-analysis" : `skill-${index + 1}`, description: `能力 ${index + 1}` }));
  const scopedAgent: Agent = { ...agent, skills, shared_skills: ["stock-analysis"] };
  render(<RoomInspector
    open room={room} members={[member]} agents={[scopedAgent]} workspaceAgents={[scopedAgent]} verifying={false}
    onRegisterAgent={vi.fn()} onCreateHumanInvite={vi.fn()} onCreateRunnerInvite={vi.fn()} onAddRoomAgent={vi.fn()} onUpdateAgentSettings={vi.fn()}
    onRemoveRoomAgent={vi.fn()} onRemoveWorkspaceAgent={vi.fn()} onVerifyAgents={vi.fn()} onTogglePanel={vi.fn()} onClose={vi.fn()}
  />);

  expect(screen.getByText("开放 1/39 Skills")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "查看 Codex Local 详情与群内设置" }));
  expect(screen.getByText("本机发现 39 个 Skill，当前允许他人调用 1 个。")).toBeInTheDocument();
  expect(screen.queryByText("stock-analysis")).not.toBeInTheDocument();
  expect(screen.queryByText("research-db")).not.toBeInTheDocument();
});

test("折叠可添加 Agent 后仍可从统一入口添加我的 Agent，并用二次确认删除", async () => {
  window.localStorage.clear();
  const user = userEvent.setup();
  const onRegisterAgent = vi.fn();
  const onRemoveWorkspaceAgent = vi.fn().mockResolvedValue(undefined);
  render(<RoomInspector
    open room={room} members={[member]} agents={[]} workspaceAgents={[agent]} verifying={false}
    onRegisterAgent={onRegisterAgent} onCreateHumanInvite={vi.fn()} onCreateRunnerInvite={vi.fn()} onAddRoomAgent={vi.fn()} onRemoveRoomAgent={vi.fn()} onRemoveWorkspaceAgent={onRemoveWorkspaceAgent}
    onUpdateAgentSettings={vi.fn()} onVerifyAgents={vi.fn()} onTogglePanel={vi.fn()} onClose={vi.fn()}
  />);

  await user.click(screen.getByRole("button", { name: "折叠可添加 Agent" }));
  expect(screen.queryByText("Codex Local")).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "邀请与接入" }));
  expect(screen.getByText(/普通真人协作不需要安装 Client/)).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "打开本机 Agent" }));
  expect(onRegisterAgent).toHaveBeenCalledTimes(1);
  await user.click(screen.getByRole("button", { name: "展开可添加 Agent" }));
  await user.click(screen.getByRole("button", { name: "从 Agent 库删除 Codex Local" }));
  expect(onRemoveWorkspaceAgent).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "确认删除" }));
  expect(onRemoveWorkspaceAgent).toHaveBeenCalledWith("agent-1");
});

test("从当前群移出 Agent 需要二次确认", async () => {
  window.localStorage.clear();
  const user = userEvent.setup();
  const onRemoveRoomAgent = vi.fn().mockResolvedValue(undefined);
  render(<RoomInspector
    open room={room} members={[member]} agents={[agent]} workspaceAgents={[agent]} verifying={false}
    onRegisterAgent={vi.fn()} onCreateHumanInvite={vi.fn()} onCreateRunnerInvite={vi.fn()} onAddRoomAgent={vi.fn()} onRemoveRoomAgent={onRemoveRoomAgent} onRemoveWorkspaceAgent={vi.fn()}
    onUpdateAgentSettings={vi.fn()} onVerifyAgents={vi.fn()} onTogglePanel={vi.fn()} onClose={vi.fn()}
  />);
  await user.click(screen.getByRole("button", { name: "将 Codex Local 移出当前群聊" }));
  expect(onRemoveRoomAgent).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "确认移出" }));
  expect(onRemoveRoomAgent).toHaveBeenCalledWith("agent-1");
});

test("为当前群生成一个统一加入链接，不再要求手填 Hub 和设备配对码", async () => {
  window.localStorage.clear();
  const user = userEvent.setup();
  const onCreateHumanInvite = vi.fn().mockResolvedValue({
    id: "human-invite-1",
    code: "human-one-time-code",
    workspace_id: "workspace-1",
    room_id: "room-1",
    role: "member",
    expires_at: "2026-07-20T12:30:00Z",
    hub_urls: ["http://192.168.1.20:8080"],
  });
  render(<RoomInspector
    open room={room} members={[member]} agents={[]} workspaceAgents={[agent]} verifying={false}
    onRegisterAgent={vi.fn()} onCreateHumanInvite={onCreateHumanInvite} onCreateRunnerInvite={vi.fn()} onAddRoomAgent={vi.fn()} onRemoveRoomAgent={vi.fn()} onRemoveWorkspaceAgent={vi.fn()}
    onUpdateAgentSettings={vi.fn()} onVerifyAgents={vi.fn()} onTogglePanel={vi.fn()} onClose={vi.fn()}
  />);

  await user.click(screen.getByRole("button", { name: "邀请与接入" }));
  await user.click(screen.getByRole("button", { name: "生成邀请链接" }));
  expect(onCreateHumanInvite).toHaveBeenCalledTimes(1);
  expect(screen.getByText("发送这一个链接即可")).toBeInTheDocument();
  expect(screen.queryByRole("combobox", { name: "真人加入使用的 Hub 地址" })).not.toBeInTheDocument();
  expect(screen.getByText("http://192.168.1.20:8080/?join=human-one-time-code")).toBeInTheDocument();
  expect(screen.getByText(/不需要填写 Hub 地址、邀请码或设备配对码/)).toBeInTheDocument();
});

test("邀请与接入中可直接粘贴链接加入其他团队", async () => {
  const user = userEvent.setup();
  const onJoinWorkspace = vi.fn().mockResolvedValue(undefined);
  render(<RoomInspector
    open room={room} workspace={workspace} currentUser={member} members={[member]} agents={[]} workspaceAgents={[]} verifying={false}
    onRegisterAgent={vi.fn()} onJoinWorkspace={onJoinWorkspace} onCreateHumanInvite={vi.fn()} onAddRoomAgent={vi.fn()} onRemoveRoomAgent={vi.fn()} onRemoveWorkspaceAgent={vi.fn()}
    onUpdateAgentSettings={vi.fn()} onVerifyAgents={vi.fn()} onClose={vi.fn()}
  />);

  await user.click(screen.getByRole("button", { name: "邀请与接入" }));
  expect(screen.getByText("加入已有团队、邀请协作者，或接入自己的 Agent。")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "输入邀请链接" }));
  const input = screen.getByRole("textbox", { name: "团队邀请链接" });
  await user.type(input, "http://192.168.1.20:8080/?join=team-code");
  await user.click(screen.getByRole("button", { name: "加入团队" }));
  expect(onJoinWorkspace).toHaveBeenCalledWith("http://192.168.1.20:8080/?join=team-code");
});
