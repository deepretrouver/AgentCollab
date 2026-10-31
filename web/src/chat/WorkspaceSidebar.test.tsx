import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { Agent, User, Workspace } from "../api/client";
import { WorkspaceSidebar } from "./WorkspaceSidebar";
import { setLocale } from "../i18n/locale";

afterEach(() => setLocale("zh-CN"));

test("English invitations and workspace deletion retain the same callbacks and confirmation", async () => {
  setLocale("en");
  const user = userEvent.setup();
  const onJoinHub = vi.fn();
  const onDeleteWorkspace = vi.fn().mockResolvedValue(undefined);
  const ownedWorkspace: Workspace = { ...workspace, role: "owner" };
  render(<WorkspaceSidebar
    open={false} currentUser={currentUser} workspaces={[ownedWorkspace]} selectedWorkspace={ownedWorkspace} rooms={[]} agents={[]} loading={false}
    onSelectWorkspace={vi.fn()} onSelectRoom={vi.fn()} onCreateWorkspace={vi.fn()} onCreateRoom={vi.fn()} onJoinHub={onJoinHub}
    onDeleteWorkspace={onDeleteWorkspace} onSetRoomPinned={vi.fn()} theme="light" onToggleTheme={vi.fn()} onLogout={vi.fn()} onClose={vi.fn()}
  />);
  await user.click(screen.getByRole("button", { name: "Join by invitation" }));
  const link = "http://192.168.1.20:8080/?join=example-code";
  await user.type(screen.getByLabelText("Invitation link"), link);
  await user.click(screen.getByRole("button", { name: "Join team" }));
  expect(onJoinHub).toHaveBeenCalledWith(link);
  await user.click(screen.getByRole("button", { name: `Configure workspace ${workspace.name}` }));
  await user.click(screen.getByRole("button", { name: "Delete this workspace" }));
  expect(onDeleteWorkspace).not.toHaveBeenCalled();
  expect(screen.getByRole("alert")).toHaveTextContent("permanently deletes");
  await user.click(screen.getByRole("button", { name: "Confirm deletion" }));
  expect(onDeleteWorkspace).toHaveBeenCalledWith(workspace.id);
});

test("English room creation preserves user names and selected agent identity", async () => {
  setLocale("en");
  const user = userEvent.setup();
  const onCreateRoom = vi.fn().mockResolvedValue(undefined);
  render(<WorkspaceSidebar
    open={false} currentUser={currentUser} workspaces={[workspace]} selectedWorkspace={workspace} rooms={[]} agents={[agent]} loading={false}
    onSelectWorkspace={vi.fn()} onSelectRoom={vi.fn()} onCreateWorkspace={vi.fn()} onCreateRoom={onCreateRoom} onJoinHub={vi.fn()}
    onSetRoomPinned={vi.fn()} theme="light" onToggleTheme={vi.fn()} onLogout={vi.fn()} onClose={vi.fn()}
  />);
  expect(screen.getByText(workspace.name)).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Create room" }));
  await user.type(screen.getByLabelText("Room name"), "研究 Research");
  await user.click(screen.getByRole("checkbox", { name: /Codex Local/ }));
  await user.click(screen.getByRole("button", { name: /^Create$/ }));
  expect(onCreateRoom).toHaveBeenCalledWith("研究 Research", [agent.id]);
});

const currentUser: User = { id: "user-1", username: "owner", display_name: "Owner", role: "admin" };
const workspace: Workspace = { id: "workspace-1", name: "本地协作", owner_user_id: "user-1", created_at: "2026-07-16T03:00:00Z", updated_at: "2026-07-16T03:00:00Z" };
const agent: Agent = {
  id: "agent-1", display_name: "Codex Local", type: "codex", owner_user_id: "user-1", owner_name: "Owner",
  device_id: "device-1", device_name: "MacBook", os: "darwin", arch: "arm64", channel: "runner/darwin", permission_mode: "admin", status: "online",
  imported: true, available: true, version: "codex 0.132.0", executable: "/opt/homebrew/bin/codex", native_history: "Codex session",
  response_mode: "mention", reply_delay_seconds: 0, role_prompt: "", max_auto_turns: 4,
};

test("默认 owner 可从侧栏直接进入 Hub 账号管理", async () => {
  const user = userEvent.setup();
  const onManageHub = vi.fn();
  render(<WorkspaceSidebar
    open={false} currentUser={currentUser} workspaces={[workspace]} selectedWorkspace={workspace} rooms={[]} agents={[]} loading={false}
    onSelectWorkspace={vi.fn()} onSelectRoom={vi.fn()} onCreateWorkspace={vi.fn()} onCreateRoom={vi.fn()} onJoinHub={vi.fn()}
    onSetRoomPinned={vi.fn()} theme="light" onToggleTheme={vi.fn()} onOpenProfile={vi.fn()} onManageHub={onManageHub}
    onLogout={vi.fn()} onClose={vi.fn()}
  />);

  expect(screen.getByText(/@owner · 网页版 · Hub 管理员/)).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "打开 Hub 账号管理" }));
  expect(onManageHub).toHaveBeenCalledTimes(1);
});

test("Client 侧栏同时标明当前账号和使用入口", () => {
  render(<WorkspaceSidebar
    open={false} currentUser={currentUser} surface="client" workspaces={[workspace]} selectedWorkspace={workspace} rooms={[]} agents={[]} loading={false}
    onSelectWorkspace={vi.fn()} onSelectRoom={vi.fn()} onCreateWorkspace={vi.fn()} onCreateRoom={vi.fn()} onJoinHub={vi.fn()}
    onSetRoomPinned={vi.fn()} theme="light" onToggleTheme={vi.fn()} onLogout={vi.fn()} onClose={vi.fn()}
  />);

  expect(screen.getByText(/@owner · Client · Hub 管理员/)).toBeInTheDocument();
  expect(screen.getByText("还没有群聊。点击右上角 + 手动创建。")).toBeInTheDocument();
});

test("创建群聊时选择已注册 Agent 成员", async () => {
  const user = userEvent.setup();
  const onCreateRoom = vi.fn().mockResolvedValue(undefined);
  render(<WorkspaceSidebar
    open={false}
    currentUser={currentUser}
    workspaces={[workspace]}
    selectedWorkspace={workspace}
    rooms={[]}
    agents={[agent]}
    loading={false}
    onSelectWorkspace={vi.fn()}
    onSelectRoom={vi.fn()}
    onCreateWorkspace={vi.fn().mockResolvedValue(undefined)}
    onCreateRoom={onCreateRoom}
    onJoinHub={vi.fn()}
    onSetRoomPinned={vi.fn().mockResolvedValue(undefined)}
    onTogglePanel={vi.fn()}
    theme="light"
    onToggleTheme={vi.fn()}
    onLogout={vi.fn().mockResolvedValue(undefined)}
    onClose={vi.fn()}
  />);

  await user.click(screen.getByRole("button", { name: "创建群聊" }));
  await user.type(screen.getByLabelText("群名称"), "方案讨论");
  await user.click(screen.getByRole("checkbox", { name: /Codex Local/ }));
  await user.click(screen.getByRole("button", { name: "创建" }));

  expect(onCreateRoom).toHaveBeenCalledWith("方案讨论", ["agent-1"]);
});

test("置顶群聊固定在顶部，其余群聊按所选方式排序", async () => {
  window.localStorage.clear();
  const user = userEvent.setup();
  const onSetRoomPinned = vi.fn().mockResolvedValue(undefined);
  const rooms = [
    { id: "room-a", workspace_id: workspace.id, name: "Alpha", created_at: "2026-07-15T03:00:00Z" },
    { id: "room-z", workspace_id: workspace.id, name: "Zulu", created_at: "2026-07-17T03:00:00Z" },
    { id: "room-p", workspace_id: workspace.id, name: "Pinned", created_at: "2026-07-14T03:00:00Z", pinned: true, pinned_at: "2026-07-18T03:00:00Z" },
  ];
  render(<WorkspaceSidebar
    open={false} currentUser={currentUser} workspaces={[workspace]} selectedWorkspace={workspace} rooms={rooms} agents={[agent]} loading={false}
    onSelectWorkspace={vi.fn()} onSelectRoom={vi.fn()} onCreateWorkspace={vi.fn()} onCreateRoom={vi.fn()} onJoinHub={vi.fn()}
    onSetRoomPinned={onSetRoomPinned} onTogglePanel={vi.fn()} theme="light" onToggleTheme={vi.fn()} onLogout={vi.fn()} onClose={vi.fn()}
  />);

  const nav = screen.getByRole("navigation", { name: "群聊列表" });
  expect([...nav.querySelectorAll(".room-select-button span")].map((node) => node.textContent)).toEqual(["Pinned", "Zulu", "Alpha"]);
  await user.selectOptions(screen.getByRole("combobox", { name: "群聊排序" }), "name_asc");
  expect([...nav.querySelectorAll(".room-select-button span")].map((node) => node.textContent)).toEqual(["Pinned", "Alpha", "Zulu"]);
  await user.click(screen.getByRole("button", { name: "取消置顶 Pinned" }));
  expect(onSetRoomPinned).toHaveBeenCalledWith(rooms[2], false);
});

test("粘贴一个完整邀请链接即可加入", async () => {
  const user = userEvent.setup();
  const onJoinHub = vi.fn();
  render(<WorkspaceSidebar
    open={false} currentUser={currentUser} workspaces={[workspace]} selectedWorkspace={workspace} rooms={[]} agents={[]} loading={false}
    onSelectWorkspace={vi.fn()} onSelectRoom={vi.fn()} onCreateWorkspace={vi.fn()} onCreateRoom={vi.fn()} onJoinHub={onJoinHub}
    onSetRoomPinned={vi.fn()} onTogglePanel={vi.fn()} theme="light" onToggleTheme={vi.fn()} onLogout={vi.fn()} onClose={vi.fn()}
  />);

  await user.click(screen.getByRole("button", { name: "通过邀请加入" }));
  await user.type(screen.getByLabelText("邀请链接"), "http://192.168.1.20:8080/?join=human-code");
  await user.click(screen.getByRole("button", { name: "加入团队" }));
  expect(onJoinHub).toHaveBeenCalledWith("http://192.168.1.20:8080/?join=human-code");
});

test("没有任何工作区时加入团队入口仍默认收起", () => {
  render(<WorkspaceSidebar
    open={false} currentUser={currentUser} workspaces={[]} rooms={[]} agents={[]} loading={false}
    onSelectWorkspace={vi.fn()} onSelectRoom={vi.fn()} onCreateWorkspace={vi.fn()} onCreateRoom={vi.fn()} onJoinHub={vi.fn()}
    onSetRoomPinned={vi.fn()} theme="light" onToggleTheme={vi.fn()} onLogout={vi.fn()} onClose={vi.fn()}
  />);

  expect(screen.queryByText("粘贴对方发来的完整邀请链接即可。地址、邀请码和目标群聊都已经包含在链接中。")).not.toBeInTheDocument();
  expect(screen.queryByLabelText("邀请链接")).not.toBeInTheDocument();
});

test("工作区 Owner 可以从当前工作区的设置入口改名或删除", async () => {
  const user = userEvent.setup();
  const onUpdateWorkspace = vi.fn().mockResolvedValue(undefined);
  const onDeleteWorkspace = vi.fn().mockResolvedValue(undefined);
  const ownedWorkspace = { ...workspace, role: "owner" as const };
  render(<WorkspaceSidebar
    open={false} currentUser={currentUser} workspaces={[ownedWorkspace]} selectedWorkspace={ownedWorkspace} rooms={[]} agents={[]} loading={false}
    onSelectWorkspace={vi.fn()} onSelectRoom={vi.fn()} onCreateWorkspace={vi.fn()} onCreateRoom={vi.fn()} onJoinHub={vi.fn()}
    onUpdateWorkspace={onUpdateWorkspace} onDeleteWorkspace={onDeleteWorkspace} onSetRoomPinned={vi.fn()} theme="light" onToggleTheme={vi.fn()} onLogout={vi.fn()} onClose={vi.fn()}
  />);

  await user.click(screen.getByRole("button", { name: "设置工作区 本地协作" }));
  const name = screen.getByLabelText("工作区名称");
  await user.clear(name);
  await user.type(name, "本地小团队");
  await user.click(screen.getByRole("button", { name: "保存名称" }));
  expect(onUpdateWorkspace).toHaveBeenCalledWith("workspace-1", "本地小团队");

  await user.click(screen.getByRole("button", { name: "设置工作区 本地协作" }));
  await user.click(screen.getByRole("button", { name: "删除此工作区" }));
  await user.click(screen.getByRole("button", { name: "确认删除" }));
  expect(onDeleteWorkspace).toHaveBeenCalledWith("workspace-1");
});

test("确认删除不会将同名设置再次提交为改名", async () => {
  const user = userEvent.setup();
  const onUpdateWorkspace = vi.fn().mockResolvedValue(undefined);
  const onDeleteWorkspace = vi.fn().mockResolvedValue(undefined);
  const ownedWorkspace = { ...workspace, role: "owner" as const };
  render(<WorkspaceSidebar
    open={false} currentUser={currentUser} workspaces={[ownedWorkspace]} selectedWorkspace={ownedWorkspace} rooms={[]} agents={[]} loading={false}
    onSelectWorkspace={vi.fn()} onSelectRoom={vi.fn()} onCreateWorkspace={vi.fn()} onCreateRoom={vi.fn()} onJoinHub={vi.fn()}
    onUpdateWorkspace={onUpdateWorkspace} onDeleteWorkspace={onDeleteWorkspace} onSetRoomPinned={vi.fn()} theme="light" onToggleTheme={vi.fn()} onLogout={vi.fn()} onClose={vi.fn()}
  />);

  await user.click(screen.getByRole("button", { name: "设置工作区 本地协作" }));
  await user.click(screen.getByRole("button", { name: "删除此工作区" }));
  await user.click(screen.getByRole("button", { name: "确认删除" }));
  expect(onDeleteWorkspace).toHaveBeenCalledWith("workspace-1");
  expect(onUpdateWorkspace).not.toHaveBeenCalled();
});
