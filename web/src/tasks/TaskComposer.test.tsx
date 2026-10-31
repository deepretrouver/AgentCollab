import { act, render, screen } from "@testing-library/react";
import { setLocale } from "../i18n/locale";
import userEvent from "@testing-library/user-event";
import type { Agent } from "../api/client";
import { TaskComposer } from "./TaskComposer";

afterEach(() => act(() => setLocale("zh-CN")));

test("English task form preserves entered content across language switches", async () => {
  setLocale("en");
  const user = userEvent.setup();
  const onCreate = vi.fn().mockResolvedValue(undefined);
  const onClose = vi.fn();
  render(<TaskComposer agents={[{ ...agent, status: "offline" }]} initialPrompt="审查 report" onClose={onClose} onCreate={onCreate} />);
  await user.type(screen.getByLabelText("Task title"), "Review 原稿");
  await user.type(screen.getByLabelText("Acceptance criteria (optional)"), "列出 gaps");
  act(() => setLocale("zh-CN"));
  expect(screen.getByLabelText("任务标题")).toHaveValue("Review 原稿");
  expect(onCreate).not.toHaveBeenCalled();
  act(() => setLocale("en"));
  await user.click(screen.getByRole("button", { name: "Create and wait for connection" }));
  expect(onCreate).toHaveBeenCalledWith({ target_agent_id: "agent-1", title: "Review 原稿", prompt: "审查 report", acceptance_criteria: "列出 gaps", plan_step_id: undefined, execution_source: undefined });
  expect(onClose).toHaveBeenCalledOnce();
});

const agent: Agent = {
  id: "agent-1", display_name: "Codex Local", type: "codex", owner_user_id: "user-1", owner_name: "Owner",
  device_id: "device-1", device_name: "MacBook", os: "darwin", arch: "arm64", channel: "runner/darwin", permission_mode: "admin", status: "online",
	imported: true, available: true, version: "codex 0.132.0", executable: "/opt/homebrew/bin/codex", native_history: "Codex session",
	response_mode: "mention", reply_delay_seconds: 0, role_prompt: "", max_auto_turns: 4,
};

test("创建结构化 Agent 任务", async () => {
  const user = userEvent.setup();
  const onCreate = vi.fn().mockResolvedValue(undefined);
  render(<TaskComposer agents={[agent]} initialPrompt="检查项目" onClose={vi.fn()} onCreate={onCreate} />);
  await user.type(screen.getByPlaceholderText("例如：检查本地项目状态"), "本地冒烟测试");
  await user.click(screen.getByRole("button", { name: "创建并派发" }));
  expect(onCreate).toHaveBeenCalledWith(expect.objectContaining({ target_agent_id: "agent-1", title: "本地冒烟测试", prompt: "检查项目" }));
});
