import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { LoginPage } from "./LoginPage";
import { setLocale } from "../i18n/locale";

beforeEach(() => { window.localStorage.clear(); setLocale("zh-CN"); });
afterEach(() => setLocale("zh-CN"));

test("English invitation flow preserves form input and language preference", async () => {
  const user = userEvent.setup();
  const onRegister = vi.fn().mockResolvedValue(undefined);
  render(<LoginPage inviteCode="demo" onLogin={vi.fn()} onRegister={onRegister} />);
  await user.type(screen.getByLabelText("显示名称"), "Alex");
  await user.selectOptions(screen.getByLabelText("Language / 语言"), "en");
  expect(screen.getByLabelText("Display name")).toHaveValue("Alex");
  expect(window.localStorage.getItem("agentcollab.locale")).toBe("en");
  expect(document.documentElement.lang).toBe("en");
  await user.type(screen.getByLabelText("New username"), "alex");
  await user.type(screen.getByLabelText("Set password"), "demo-password");
  await user.click(screen.getByRole("button", { name: "Create account and join" }));
  expect(onRegister).toHaveBeenCalledWith({ username: "alex", display_name: "Alex", password: "demo-password" });
});

test("提交用户名和密码", async () => {
  const user = userEvent.setup();
  const onLogin = vi.fn().mockResolvedValue(undefined);
  render(<LoginPage onLogin={onLogin} />);

  await user.type(screen.getByLabelText("用户名"), "owner");
  await user.type(screen.getByLabelText("密码"), "password-1234");
  await user.click(screen.getByRole("button", { name: /AgentCollab/ }));

  expect(onLogin).toHaveBeenCalledWith("owner", "password-1234");
});

test("就地显示登录错误", async () => {
  const user = userEvent.setup();
  render(<LoginPage onLogin={vi.fn().mockRejectedValue(new Error("用户名或密码不正确"))} />);

  await user.type(screen.getByLabelText("用户名"), "owner");
  await user.type(screen.getByLabelText("密码"), "wrong-password");
  await user.click(screen.getByRole("button", { name: /AgentCollab/ }));

  expect(await screen.findByRole("alert")).toHaveTextContent("用户名或密码不正确");
});

test("邀请链接允许创建自己的真人账号并加入群聊", async () => {
  const user = userEvent.setup();
  const onRegister = vi.fn().mockResolvedValue(undefined);
  render(<LoginPage inviteCode="invite-code" onLogin={vi.fn()} onRegister={onRegister} />);

  expect(screen.getByRole("heading", { name: "加入群聊" })).toBeInTheDocument();
  expect(screen.getByRole("link", { name: "在 Client 中打开" })).toHaveAttribute("href", expect.stringContaining("agentcollab://join?"));
  expect(screen.getByText(/共享时间线/)).toBeInTheDocument();
  await user.type(screen.getByLabelText("显示名称"), "李雷");
  await user.type(screen.getByLabelText("新账号用户名"), "lilei");
  await user.type(screen.getByLabelText("设置密码"), "password-1234");
  await user.click(screen.getByRole("button", { name: "创建账号并加入" }));

  expect(onRegister).toHaveBeenCalledWith({ username: "lilei", display_name: "李雷", password: "password-1234" });
});

test("没有邀请链接时也能创建普通账号", async () => {
  const user = userEvent.setup();
  const onRegister = vi.fn().mockResolvedValue(undefined);
  render(<LoginPage onLogin={vi.fn()} onRegister={onRegister} />);

  await user.click(screen.getByRole("tab", { name: "创建新账号" }));
  expect(screen.getByRole("heading", { name: "创建账号" })).toBeInTheDocument();
  await user.type(screen.getByLabelText("显示名称"), "王小明");
  await user.type(screen.getByLabelText("新账号用户名"), "wangxiaoming");
  await user.type(screen.getByLabelText("设置密码"), "password-1234");
  await user.click(screen.getByRole("button", { name: "创建账号" }));

  expect(onRegister).toHaveBeenCalledWith({ username: "wangxiaoming", display_name: "王小明", password: "password-1234" });
});

test("已有账号可从邀请页登录后加入", async () => {
  const user = userEvent.setup();
  const onLogin = vi.fn().mockResolvedValue(undefined);
  render(<LoginPage inviteCode="invite-code" onLogin={onLogin} onRegister={vi.fn()} />);

  await user.click(screen.getByRole("tab", { name: "已有账号登录" }));
  await user.type(screen.getByLabelText("用户名"), "member");
  await user.type(screen.getByLabelText("密码"), "password-5678");
  await user.click(screen.getByRole("button", { name: "进入 AgentCollab" }));
  expect(onLogin).toHaveBeenCalledWith("member", "password-5678");
});

test("登录成功后记住用户名，但不把密码写入本地历史", async () => {
  const user = userEvent.setup();
  const first = render(<LoginPage onLogin={vi.fn().mockResolvedValue(undefined)} />);

  await user.type(screen.getByLabelText("用户名"), "owner");
  await user.type(screen.getByLabelText("密码"), "password-1234");
  await user.click(screen.getByRole("button", { name: /AgentCollab/ }));
  first.unmount();

  const stored = window.localStorage.getItem(`agentcollab.login-history:${window.location.origin}`) ?? "";
  expect(stored).toContain("owner");
  expect(stored).not.toContain("password-1234");

  render(<LoginPage onLogin={vi.fn().mockResolvedValue(undefined)} />);
  expect(screen.getByLabelText("用户名")).toHaveValue("owner");
  expect(screen.getByLabelText("密码")).toHaveValue("");
  await user.click(screen.getByRole("button", { name: "owner" }));
  expect(screen.getByLabelText("用户名")).toHaveValue("owner");
});
