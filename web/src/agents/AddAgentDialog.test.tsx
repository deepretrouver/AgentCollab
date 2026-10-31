import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach } from "vitest";
import { api, type Agent } from "../api/client";
import { AddAgentDialog } from "./AddAgentDialog";
import { setLocale } from "../i18n/locale";
import { StrictMode } from "react";

afterEach(() => {
  setLocale("zh-CN");
  vi.restoreAllMocks();
  delete window.agentCollabDesktop;
});

const createInvite = vi.fn().mockResolvedValue({ code: "pair-code" });

function mockConnectedDesktop() {
  window.agentCollabDesktop = {
    isDesktop: true, platform: "darwin",
    getState: vi.fn().mockResolvedValue({ configured: true, runnerRegistered: true, runnerRunning: true, runnerId: "runner-1", platform: "darwin", version: "1.3.3" }),
    configure: vi.fn(), connectCurrentComputer: vi.fn(), reconnect: vi.fn(), openSettings: vi.fn(), onStatus: vi.fn(() => () => undefined),
  };
  vi.spyOn(api, "runnerOwnership").mockResolvedValue({ owned_by_current_user: true });
}

test("StrictMode 不会取消首次自动查找且不重复扫描", async () => {
  mockConnectedDesktop();
  const onScan = vi.fn().mockResolvedValue(undefined);
  render(<StrictMode><AddAgentDialog open discovered={[]} registered={[]} scanned={false} scanning={false} onScan={onScan} onCreateRunnerInvite={createInvite} onClose={vi.fn()} onRegister={vi.fn()} /></StrictMode>);
  await waitFor(() => expect(onScan).toHaveBeenCalledTimes(1));
});

test("扫描状态更新不会吞掉尚未结束的连接检查", async () => {
  mockConnectedDesktop();
  let resolveState!: (value: Awaited<ReturnType<NonNullable<typeof window.agentCollabDesktop>["getState"]>>) => void;
  vi.mocked(window.agentCollabDesktop!.getState).mockReturnValue(new Promise((resolve) => { resolveState = resolve; }));
  const props = { open: true, discovered: [], registered: [], onScan: vi.fn(), onCreateRunnerInvite: createInvite, onClose: vi.fn(), onRegister: vi.fn() };
  const { rerender } = render(<AddAgentDialog {...props} scanned={false} scanning />);
  rerender(<AddAgentDialog {...props} scanned scanning={false} />);
  await act(async () => resolveState({ configured: false, runnerRegistered: false, runnerRunning: false, hubManaged: false, platform: "darwin", version: "1.3.3" }));
  expect(await screen.findByText("先把这台电脑连接到当前账号")).toBeInTheDocument();
  expect(props.onScan).not.toHaveBeenCalled();
});

test("自动扫描失败不会因状态变化无限重试，仍可手动重试", async () => {
  mockConnectedDesktop();
  const onScan = vi.fn().mockRejectedValue(new Error("Scan unavailable"));
  const props = { open: true, discovered: [], registered: [], scanned: false, onScan, onCreateRunnerInvite: createInvite, onClose: vi.fn(), onRegister: vi.fn() };
  const { rerender } = render(<AddAgentDialog {...props} scanning={false} />);
  expect(await screen.findByRole("alert")).toHaveTextContent("Scan unavailable");
  rerender(<AddAgentDialog {...props} scanning />);
  rerender(<AddAgentDialog {...props} scanning={false} />);
  await act(async () => {});
  expect(onScan).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "查找本机 Agent" }));
  await waitFor(() => expect(onScan).toHaveBeenCalledTimes(2));
});

const agent: Agent = {
  id: "agent-1", display_name: "Codex Local", type: "codex", owner_user_id: "user-1", owner_name: "Owner",
  device_id: "device-1", device_name: "Yuke MacBook Air", os: "darwin", arch: "arm64", channel: "runner/darwin", permission_mode: "admin", status: "online",
	imported: false, available: true, version: "codex 0.132.0", executable: "/opt/homebrew/bin/codex", native_history: "Codex session",
	response_mode: "mention", reply_delay_seconds: 0, role_prompt: "", max_auto_turns: 4,
};

test("关闭后重新打开不保留上一次勾选", async () => {
  const props = { discovered: [agent], registered: [], scanned: true, scanning: false, onScan: vi.fn(), onCreateRunnerInvite: createInvite, onClose: vi.fn(), onRegister: vi.fn() };
  const { rerender } = render(<AddAgentDialog {...props} open />);
  fireEvent.click(screen.getByText("Codex Local"));
  rerender(<AddAgentDialog {...props} open={false} />);
  rerender(<AddAgentDialog {...props} open />);
  expect(screen.getByRole("checkbox")).not.toBeChecked();
  expect(screen.getByRole("button", { name: /加入.*个 Agent/ })).toBeDisabled();
  await act(async () => {});
});

test("扫描结果变化后不提交消失或已不可用的 Agent", async () => {
  const onRegister = vi.fn();
  const props = { registered: [], scanned: true, scanning: false, onScan: vi.fn(), onCreateRunnerInvite: createInvite, onClose: vi.fn(), onRegister };
  const { rerender } = render(<AddAgentDialog {...props} discovered={[agent]} open />);
  fireEvent.click(screen.getByText("Codex Local"));
  rerender(<AddAgentDialog {...props} discovered={[{ ...agent, available: false }]} open />);
  fireEvent.submit(screen.getByRole("form", { name: "添加我的 Agent" }));
  expect(onRegister).not.toHaveBeenCalled();
  await act(async () => {});
});

test("英文 Agent 接入保留原生身份并正确提交", async () => {
  setLocale("en");
  const onRegister = vi.fn().mockResolvedValue(undefined);
  render(<AddAgentDialog open discovered={[agent]} registered={[]} scanned scanning={false} onScan={vi.fn()} onCreateRunnerInvite={createInvite} onClose={vi.fn()} onRegister={onRegister} />);
  expect(screen.getByText(/codex · Native CLI/)).toBeInTheDocument();
  fireEvent.click(screen.getByText("Codex Local"));
  fireEvent.click(screen.getByRole("button", { name: "Add 1 agent" }));
  await waitFor(() => expect(onRegister).toHaveBeenCalledWith([agent.id]));
});

test("英文浏览器说明区分本机扫描与调用已授权 Agent", async () => {
  setLocale("en");
  const onScan = vi.fn();
  render(<AddAgentDialog open discovered={[]} registered={[]} scanned={false} scanning={false} onScan={onScan} onCreateRunnerInvite={createInvite} onClose={vi.fn()} onRegister={vi.fn()} />);
  expect(await screen.findByText("Use the Client to scan this computer")).toBeInTheDocument();
  expect(screen.getByText(/call authorized agents/)).toBeInTheDocument();
  expect(onScan).not.toHaveBeenCalled();
});

test("其他设备已注册不代表本机扫描成功", async () => {
  render(<AddAgentDialog open discovered={[]} registered={[agent]} scanned scanning={false} onScan={vi.fn()} onCreateRunnerInvite={createInvite} onClose={vi.fn()} onRegister={vi.fn()} />);
  expect(screen.getByText("没有找到可用 Agent")).toBeInTheDocument();
  expect(screen.queryByText("这台电脑上的 Agent 已全部添加")).not.toBeInTheDocument();
  await act(async () => {});
});

test("选择后把本机 Agent 加入工作区 Agent 库", async () => {
  const onRegister = vi.fn().mockResolvedValue(undefined);
  render(<AddAgentDialog open discovered={[agent]} registered={[]} scanned scanning={false} onScan={vi.fn()} onCreateRunnerInvite={createInvite} onClose={vi.fn()} onRegister={onRegister} />);
  fireEvent.click(screen.getByText("Codex Local"));
  fireEvent.click(screen.getByRole("button", { name: "加入 1 个 Agent" }));
  expect(onRegister).toHaveBeenCalledWith(["agent-1"]);
});

test("候选 Agent 很多时仍可选择列表末尾并确认加入", async () => {
  const discovered = Array.from({ length: 18 }, (_, index) => ({
    ...agent,
    id: `agent-${index + 1}`,
    display_name: `Agent ${index + 1}`,
    type: `type-${index + 1}`,
  }));
  const onRegister = vi.fn().mockResolvedValue(undefined);
  render(<AddAgentDialog open discovered={discovered} registered={[]} scanned scanning={false} onScan={vi.fn()} onCreateRunnerInvite={createInvite} onClose={vi.fn()} onRegister={onRegister} />);

  expect(screen.getByRole("region", { name: "扫描到的 Agent" })).toBeInTheDocument();
  expect(screen.getByLabelText("加入操作")).toBeInTheDocument();
  fireEvent.click(screen.getByText("Agent 18"));
  fireEvent.click(screen.getByRole("button", { name: "加入 1 个 Agent" }));
  expect(onRegister).toHaveBeenCalledWith(["agent-18"]);
});

test("候选 Agent 明确显示原生与 ACP 运行时", () => {
  render(<AddAgentDialog
    open
    discovered={[
      { ...agent, id: "native", transport: "native", catalog_tier: "native" },
      { ...agent, id: "acp", display_name: "Codex ACP", transport: "acp", catalog_tier: "preset", protocol_version: 1 },
    ]}
    registered={[]}
    scanned
    scanning={false}
    onScan={vi.fn()}
    onCreateRunnerInvite={createInvite}
    onClose={vi.fn()}
    onRegister={vi.fn()}
  />);

  expect(screen.getByText(/codex · 原生直连/)).toBeInTheDocument();
  expect(screen.getByText(/codex · ACP 预设 v1/)).toBeInTheDocument();
});

test("空结果时识别其他账号的 Runner 并在弹窗内完成改绑", async () => {
  const onScan = vi.fn().mockResolvedValue(undefined);
  const connectCurrentComputer = vi.fn().mockResolvedValue(undefined);
  window.agentCollabDesktop = {
    isDesktop: true,
    platform: "darwin",
    getState: vi.fn().mockResolvedValue({ configured: true, runnerRegistered: true, runnerRunning: true, runnerId: "runner-owner", platform: "darwin", version: "0.3.1" }),
    configure: vi.fn(),
    connectCurrentComputer,
    reconnect: vi.fn(),
    openSettings: vi.fn(),
    onStatus: vi.fn(() => () => undefined),
  };
  vi.spyOn(api, "runnerOwnership").mockResolvedValue({ owned_by_current_user: false });

  render(<AddAgentDialog open discovered={[]} registered={[]} scanned scanning={false} onScan={onScan} onCreateRunnerInvite={createInvite} onClose={vi.fn()} onRegister={vi.fn()} />);

  expect(await screen.findByText("这台电脑属于另一个账号")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "改绑到当前账号并查找" }));

  await waitFor(() => expect(connectCurrentComputer).toHaveBeenCalledWith({ hubUrl: window.location.origin, inviteCode: "pair-code" }));
  await waitFor(() => expect(onScan).toHaveBeenCalled());
});

test("Client 打开扫描窗口后自动检查连接并开始查找", async () => {
  const onScan = vi.fn().mockResolvedValue(undefined);
  window.agentCollabDesktop = {
    isDesktop: true,
    platform: "darwin",
    getState: vi.fn().mockResolvedValue({ configured: true, runnerRegistered: true, runnerRunning: true, runnerId: "runner-owner", platform: "darwin", version: "0.4.0" }),
    configure: vi.fn(),
    connectCurrentComputer: vi.fn(),
    reconnect: vi.fn(),
    openSettings: vi.fn(),
    onStatus: vi.fn(() => () => undefined),
  };
  vi.spyOn(api, "runnerOwnership").mockResolvedValue({ owned_by_current_user: true });

  render(<AddAgentDialog open discovered={[]} registered={[]} scanned={false} scanning={false} onScan={onScan} onCreateRunnerInvite={createInvite} onClose={vi.fn()} onRegister={vi.fn()} />);

  await waitFor(() => expect(onScan).toHaveBeenCalledTimes(1));
});

test("网页版明确要求回到 Client，不伪装成本机扫描入口", async () => {
  const onScan = vi.fn().mockResolvedValue(undefined);
  render(<AddAgentDialog open discovered={[]} registered={[]} scanned={false} scanning={false} onScan={onScan} onCreateRunnerInvite={createInvite} onClose={vi.fn()} onRegister={vi.fn()} />);

  expect(await screen.findByText("网页版不能扫描这台电脑")).toBeInTheDocument();
  expect(screen.getByText(/请在 AgentCollab Client/)).toBeInTheDocument();
  expect(onScan).not.toHaveBeenCalled();
});
