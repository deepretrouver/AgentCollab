import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { Agent } from "../api/client";
import { ProfileDialog } from "./ProfileDialog";
import { setLocale } from "../i18n/locale";

afterEach(() => setLocale("zh-CN"));

test("切换语言保留尚未保存的个人信息并准确提交", async () => {
  const user = userEvent.setup();
  const onSave = vi.fn().mockResolvedValue(undefined);
  render(<ProfileDialog open user={profile} onClose={vi.fn()} onSave={onSave} />);
  await user.type(screen.getByLabelText("个人简介"), "研究 Research");
  act(() => setLocale("en"));
  expect(screen.getByRole("dialog", { name: "Profile" })).toBeInTheDocument();
  expect(screen.getByLabelText("Bio")).toHaveValue("研究 Research");
  await user.type(screen.getByLabelText("Current focus"), "跨机协作");
  await user.click(screen.getByRole("button", { name: "Save profile" }));
  expect(onSave).toHaveBeenCalledWith({ display_name: "Owner", avatar_url: "", bio: "研究 Research", current_focus: "跨机协作" });
});

const profile = { id: "user-1", username: "owner", display_name: "Owner", role: "admin" as const, bio: "", current_focus: "" };
const agent: Agent = {
  id: "agent-1", display_name: "Owner Codex", type: "codex", owner_user_id: "user-1", owner_name: "Owner", device_id: "device-1", device_name: "MacBook", os: "darwin", arch: "arm64", channel: "runner/darwin", permission_mode: "admin", status: "online", imported: true, available: true, version: "dev", executable: "codex", native_history: "", response_mode: "mention", reply_delay_seconds: 0, role_prompt: "", max_auto_turns: 4, visibility: "workspace", invocation_scope: "owner_only", shared_skills: [], can_manage: true, skills: [{ name: "stock-analysis", description: "股票分析", source: "Hermes" }, { name: "report-review", description: "报告审查", source: "通用本机" }], resources: [{ name: "research-db", access: "read_write" }],
};

test("编辑并保存自己的个人资料", async () => {
  const user = userEvent.setup();
  const onSave = vi.fn().mockResolvedValue(undefined);
  const onClose = vi.fn();
  render(<ProfileDialog open user={profile} onClose={onClose} onSave={onSave} />);

  await user.clear(screen.getByLabelText("显示名称"));
  await user.type(screen.getByLabelText("显示名称"), "Owner Research");
  await user.type(screen.getByLabelText("个人简介"), "负责研究");
  await user.type(screen.getByLabelText("当前事项"), "验证局域网协作");
  await user.click(screen.getByRole("button", { name: "保存资料" }));

  expect(onSave).toHaveBeenCalledWith({ display_name: "Owner Research", avatar_url: "", bio: "负责研究", current_focus: "验证局域网协作" });
  expect(onClose).toHaveBeenCalled();
});

test("我的 Agent 集中显示全部 Skills 并可逐项授权", async () => {
  const user = userEvent.setup();
  const onUpdateAgentPolicy = vi.fn().mockResolvedValue(undefined);
  render(<ProfileDialog open user={profile} agents={[agent]} initialSection="agents" onClose={vi.fn()} onSave={vi.fn()} onUpdateAgentPolicy={onUpdateAgentPolicy} />);

  expect(screen.getByRole("button", { name: /我的 Agent 1/ })).toHaveClass("is-active");
  expect(screen.getByText("stock-analysis")).toBeInTheDocument();
  expect(screen.getByText("report-review")).toBeInTheDocument();
  expect(screen.getByText("research-db")).toBeInTheDocument();
  expect(screen.getByText("0 / 2")).toBeInTheDocument();

  await user.click(screen.getByRole("checkbox", { name: /stock-analysis/ }));
  expect(onUpdateAgentPolicy).toHaveBeenCalledWith("agent-1", { visibility: "workspace", invocation_scope: "owner_only", shared_skills: ["stock-analysis"] });
});

test("我的 Agent 支持一次授权所有扫描到的 Skills", async () => {
  const user = userEvent.setup();
  const onUpdateAgentPolicy = vi.fn().mockResolvedValue(undefined);
  render(<ProfileDialog open user={profile} agents={[agent]} initialSection="agents" onClose={vi.fn()} onSave={vi.fn()} onUpdateAgentPolicy={onUpdateAgentPolicy} />);
  await user.click(screen.getByRole("button", { name: "全部授权" }));
  expect(onUpdateAgentPolicy).toHaveBeenCalledWith("agent-1", { visibility: "workspace", invocation_scope: "owner_only", shared_skills: ["stock-analysis", "report-review"] });
});

test("过期 Skill 授权不能冒充当前全部授权", async () => {
  const user = userEvent.setup();
  const onUpdateAgentPolicy = vi.fn().mockResolvedValue(undefined);
  const changed = { ...agent, shared_skills: ["old-skill", "stock-analysis"] };
  render(<ProfileDialog open user={profile} agents={[changed]} initialSection="agents" onClose={vi.fn()} onSave={vi.fn()} onUpdateAgentPolicy={onUpdateAgentPolicy} />);
  expect(screen.getByText("1 / 2")).toBeInTheDocument();
  expect(screen.getByRole("checkbox", { name: /report-review/ })).not.toBeChecked();
  expect(onUpdateAgentPolicy).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "全部授权" }));
  expect(onUpdateAgentPolicy).toHaveBeenCalledWith(agent.id, { visibility: "workspace", invocation_scope: "owner_only", shared_skills: ["stock-analysis", "report-review"] });
});

test("扫描为空时仍能主动撤销保留的旧授权", async () => {
  const user = userEvent.setup();
  const onUpdateAgentPolicy = vi.fn().mockResolvedValue(undefined);
  render(<ProfileDialog open user={profile} agents={[{ ...agent, skills: [], shared_skills: ["old-skill"] }]} initialSection="agents" onClose={vi.fn()} onSave={vi.fn()} onUpdateAgentPolicy={onUpdateAgentPolicy} />);
  expect(screen.getByText("0 / 0")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "全部授权" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "全部取消" })).toBeEnabled();
  await user.click(screen.getByRole("button", { name: "全部取消" }));
  expect(onUpdateAgentPolicy).toHaveBeenCalledWith(agent.id, { visibility: "workspace", invocation_scope: "owner_only", shared_skills: [] });
});

test("英文权限表单保持枚举和 Skill 标识，不因语言切换自动授权", async () => {
  setLocale("en");
  const user = userEvent.setup();
  const onUpdateAgentPolicy = vi.fn().mockResolvedValue(undefined);
  render(<ProfileDialog open user={profile} agents={[agent]} initialSection="agents" onClose={vi.fn()} onSave={vi.fn()} onUpdateAgentPolicy={onUpdateAgentPolicy} />);
  expect(screen.getByRole("combobox", { name: "Owner Codex Visibility" })).toHaveValue("workspace");
  expect(screen.getByRole("combobox", { name: "Owner Codex Invocation access" })).toHaveValue("owner_only");
  expect(screen.getByText("股票分析")).toBeInTheDocument();
  expect(onUpdateAgentPolicy).not.toHaveBeenCalled();
  await user.selectOptions(screen.getByRole("combobox", { name: "Owner Codex Visibility" }), "private");
  expect(onUpdateAgentPolicy).toHaveBeenLastCalledWith(agent.id, { visibility: "private", invocation_scope: "owner_only", shared_skills: [] });
  await user.click(screen.getByRole("checkbox", { name: /report-review/ }));
  expect(onUpdateAgentPolicy).toHaveBeenLastCalledWith(agent.id, { visibility: "workspace", invocation_scope: "owner_only", shared_skills: ["report-review"] });
  const calls = onUpdateAgentPolicy.mock.calls.length;
  act(() => setLocale("zh-CN"));
  expect(onUpdateAgentPolicy).toHaveBeenCalledTimes(calls);
});
