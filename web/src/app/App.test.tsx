import { act, render, screen } from "@testing-library/react";
import { setLocale } from "../i18n/locale";
import userEvent from "@testing-library/user-event";
import { api, ApiError, type User } from "../api/client";
import { App } from "./App";

const existingUser: User = {
  id: "user-member",
  username: "member",
  display_name: "群成员",
  role: "member",
};

afterEach(() => {
  act(() => setLocale("zh-CN"));
  vi.restoreAllMocks();
  window.history.replaceState({}, "", "/");
});

test("已使用的群聊邀请不阻止已有账号正常登录", async () => {
  window.history.replaceState({}, "", "/?join=used-invite");
  vi.spyOn(api, "me")
    .mockRejectedValueOnce(new ApiError(401, "unauthorized", "未登录"))
    .mockResolvedValue(existingUser);
  vi.spyOn(api, "login").mockResolvedValue(existingUser);
  vi.spyOn(api, "acceptInvite").mockRejectedValue(new ApiError(409, "invite_used", "邀请已使用"));
  vi.spyOn(api, "logout").mockResolvedValue(undefined);
  vi.spyOn(api, "workspaces").mockResolvedValue([]);

  const user = userEvent.setup();
  render(<App />);

  await user.click(await screen.findByRole("tab", { name: "已有账号登录" }));
  await user.type(screen.getByLabelText("用户名"), "member");
  await user.type(screen.getByLabelText("密码"), "password");
  await user.click(screen.getByRole("button", { name: "进入 AgentCollab" }));

  expect(await screen.findByRole("status")).toHaveTextContent("已按普通账号登录");
  expect(window.location.search).toBe("");
  expect(api.logout).not.toHaveBeenCalled();
  act(() => setLocale("en"));
  expect(screen.getByRole("status")).toHaveTextContent("signed in normally");
  expect(screen.getByRole("button", { name: "Dismiss invitation notice" })).toBeInTheDocument();
});

test("Server 管理入口会在 owner 登录后直接打开账号管理", async () => {
  window.history.replaceState({}, "", "/?admin=1");
  const owner: User = { id: "owner-id", username: "owner", display_name: "Owner", role: "admin" };
  vi.spyOn(api, "me").mockResolvedValue(owner);
  vi.spyOn(api, "workspaces").mockResolvedValue([]);
  vi.spyOn(api, "hubUsers").mockResolvedValue([owner]);

  render(<App />);

  expect(await screen.findByRole("dialog", { name: "账号管理员" })).toBeInTheDocument();
  expect(await screen.findByText("默认 Hub 管理员")).toBeInTheDocument();
  expect(api.hubUsers).toHaveBeenCalledTimes(1);
});
