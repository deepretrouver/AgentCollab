import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { Message, Room, User } from "../api/client";
import { RoomView } from "./RoomView";
import { setLocale } from "../i18n/locale";
import type { ConnectionState } from "../realtime/socket";

beforeEach(() => window.localStorage.clear());
afterEach(() => act(() => setLocale("zh-CN")));

const currentUser: User = { id: "user-1", username: "owner", display_name: "Owner", role: "admin" };
const room: Room = { id: "room-1", workspace_id: "workspace-1", name: "综合研究", created_at: "2026-07-16T03:00:00Z" };
const message: Message = {
  id: "message-1",
  workspace_id: "workspace-1",
  room_id: "room-1",
  sequence: 1,
  sender_kind: "user",
  sender_id: currentUser.id,
  kind: "chat",
  body: "请各位 Agent 开始分工",
  created_at: "2026-07-16T03:00:00Z",
};

test.each<[ConnectionState, string]>([["connecting", "Connecting messages"], ["live", "Messages live"], ["polling", "Messages auto-syncing"], ["retrying", "Reconnecting messages"], ["offline", "Messages unavailable"]])("English navigation and %s connection guidance", async (connection, label) => {
  setLocale("en");
  const user = userEvent.setup();
  render(<RoomView room={room} currentUser={currentUser} members={[currentUser]} agents={[]} registeredAgentCount={3} messages={[message]} tasks={[]} loading={false} error="" connection={connection} onSend={vi.fn()} onCreateTask={vi.fn()} onCancelTaskRoot={vi.fn()} onManageAgents={vi.fn()} onVerifyAgents={vi.fn()} verifying={false} verificationError="" onShowSidebar={vi.fn()} onShowDetails={vi.fn()} />);
  const state = screen.getByText(label);
  expect(state.getAttribute("title")).not.toMatch(/[\u4e00-\u9fff]/);
  expect(state.getAttribute("aria-label")).not.toMatch(/[\u4e00-\u9fff]/);
  await user.click(within(screen.getByRole("navigation", { name: "Room project views" })).getByRole("button", { name: "Collaboration" }));
  const modes = screen.getByRole("navigation", { name: "Collaboration views" });
  expect(within(modes).getByRole("button", { name: "Project office" })).toBeInTheDocument();
  await user.click(within(modes).getByRole("button", { name: "Execution timeline" }));
  expect(screen.getByText("No execution records yet")).toBeInTheDocument();
  expect(screen.getByRole("heading", { name: "# 综合研究" })).toBeInTheDocument();
});

test("verification failure stays visible after successful dispatch adds a message", () => {
  render(<RoomView room={room} currentUser={currentUser} members={[currentUser]} agents={[]} registeredAgentCount={3} messages={[message]} tasks={[]} loading={false} error="" connection="live" onSend={vi.fn()} onCreateTask={vi.fn()} onCancelTaskRoot={vi.fn()} onManageAgents={vi.fn()} onVerifyAgents={vi.fn()} verifying={false} verificationError="Reviewer: Permission denied" onShowSidebar={vi.fn()} onShowDetails={vi.fn()} />);
  expect(screen.getByRole("alert")).toHaveTextContent("Reviewer: Permission denied");
  expect(screen.getByText(message.body)).toBeInTheDocument();
});

test("显示群聊、连接状态和消息", () => {
  render(
    <RoomView
      room={room}
      currentUser={currentUser}
      members={[currentUser]}
      agents={[]}
      registeredAgentCount={3}
      messages={[message]}
      tasks={[]}
      loading={false}
      error=""
      connection="live"
      onSend={vi.fn()}
      onCreateTask={vi.fn()}
      onCancelTaskRoot={vi.fn()}
      onManageAgents={vi.fn()}
      onVerifyAgents={vi.fn()}
      verifying={false}
      verificationError=""
      onShowSidebar={vi.fn()}
      onShowDetails={vi.fn()}
    />,
  );

  expect(screen.getByRole("heading", { name: "# 综合研究" })).toBeInTheDocument();
  expect(screen.getByText("消息实时同步")).toBeInTheDocument();
  expect(screen.getByText("请各位 Agent 开始分工")).toBeInTheDocument();
  expect(screen.getByRole("textbox", { name: "群聊消息" })).not.toBeDisabled();
  expect(screen.getByRole("button", { name: "创建结构化任务" })).toBeDisabled();
});

test("群聊没有消息时显示可操作空状态", () => {
  render(
    <RoomView
      room={room}
      currentUser={currentUser}
      members={[currentUser]}
      agents={[]}
      registeredAgentCount={3}
      messages={[]}
      tasks={[]}
      loading={false}
      error=""
      connection="connecting"
      onSend={vi.fn()}
      onCreateTask={vi.fn()}
      onCancelTaskRoot={vi.fn()}
      onManageAgents={vi.fn()}
      onVerifyAgents={vi.fn()}
      verifying={false}
      verificationError=""
      onShowSidebar={vi.fn()}
      onShowDetails={vi.fn()}
    />,
  );

  expect(screen.getByText("这个群聊还没有 Agent 成员")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "打开成员列表" })).toBeInTheDocument();
});

test("空账号明确显示加入已有团队入口", async () => {
  const user = userEvent.setup();
  const onJoinWorkspace = vi.fn().mockResolvedValue(undefined);
  render(
    <RoomView
      currentUser={currentUser}
      members={[currentUser]}
      agents={[]}
      registeredAgentCount={0}
      messages={[]}
      tasks={[]}
      loading={false}
      error=""
      connection="offline"
      onSend={vi.fn()}
      onCreateTask={vi.fn()}
      onCancelTaskRoot={vi.fn()}
      onManageAgents={vi.fn()}
      onVerifyAgents={vi.fn()}
      verifying={false}
      verificationError=""
      onShowSidebar={vi.fn()}
      onShowDetails={vi.fn()}
      hasWorkspaces={false}
      onJoinWorkspace={onJoinWorkspace}
    />,
  );

  expect(screen.getByRole("heading", { name: "加入已有团队" })).toBeInTheDocument();
  await user.type(screen.getByLabelText("团队邀请链接"), "http://192.168.1.20:8080/?join=human-code");
  await user.click(screen.getByRole("button", { name: "加入团队" }));
  expect(onJoinWorkspace).toHaveBeenCalledWith("http://192.168.1.20:8080/?join=human-code");
});

test("实时通道恢复期间说明消息自动同步且不影响 Agent 状态", () => {
  render(
    <RoomView
      room={room}
      currentUser={currentUser}
      members={[currentUser]}
      agents={[]}
      registeredAgentCount={3}
      messages={[message]}
      tasks={[]}
      loading={false}
      error=""
      connection="polling"
      onSend={vi.fn()}
      onCreateTask={vi.fn()}
      onCancelTaskRoot={vi.fn()}
      onManageAgents={vi.fn()}
      onVerifyAgents={vi.fn()}
      verifying={false}
      verificationError=""
      onShowSidebar={vi.fn()}
      onShowDetails={vi.fn()}
    />,
  );

  expect(screen.getByText("消息自动同步")).toBeInTheDocument();
  expect(screen.getByLabelText("群聊消息状态：消息自动同步。实时通道暂时不可用，群消息仍会自动同步，不影响 Agent 在线状态。")).toBeInTheDocument();
  expect(screen.queryByText("消息连接恢复中")).not.toBeInTheDocument();
});

test("桌面端通过一个视图菜单控制两侧面板", async () => {
  const user = userEvent.setup();
  const onToggleLeftPanel = vi.fn();
  render(<RoomView room={room} currentUser={currentUser} members={[currentUser]} agents={[]} registeredAgentCount={3} messages={[message]} tasks={[]} loading={false} error="" connection="live" onSend={vi.fn()} onCreateTask={vi.fn()} onCancelTaskRoot={vi.fn()} onManageAgents={vi.fn()} onVerifyAgents={vi.fn()} verifying={false} verificationError="" onShowSidebar={vi.fn()} onShowDetails={vi.fn()} leftPanelVisible rightPanelVisible onToggleLeftPanel={onToggleLeftPanel} onToggleRightPanel={vi.fn()} />);

  await user.click(screen.getByRole("button", { name: "视图布局" }));
  expect(screen.getByRole("menuitemcheckbox", { name: /工作区/ })).toHaveAttribute("aria-checked", "true");
  expect(screen.getByRole("menuitemcheckbox", { name: /群详情/ })).toHaveAttribute("aria-checked", "true");
  await user.click(screen.getByRole("menuitemcheckbox", { name: /工作区/ }));
  expect(onToggleLeftPanel).toHaveBeenCalledOnce();
});

test("显示用户原文并隐藏内部任务 prompt", () => {
  const internalTask: Message = { ...message, id: "message-2", sequence: 2, kind: "task", body: "你在 AgentCollab 协作群中被点名。请先理解以下最近群聊上下文。" };
  render(
    <RoomView
      room={room}
      currentUser={currentUser}
      members={[currentUser]}
      agents={[]}
      registeredAgentCount={3}
      messages={[message, internalTask]}
      tasks={[]}
      loading={false}
      error=""
      connection="live"
      onSend={vi.fn()}
      onCreateTask={vi.fn()}
      onCancelTaskRoot={vi.fn()}
      onManageAgents={vi.fn()}
      onVerifyAgents={vi.fn()}
      verifying={false}
      verificationError=""
      onShowSidebar={vi.fn()}
      onShowDetails={vi.fn()}
    />,
  );

  expect(screen.getByText("请各位 Agent 开始分工")).toBeInTheDocument();
  expect(screen.queryByText(/你在 AgentCollab 协作群中被点名/)).not.toBeInTheDocument();
});
