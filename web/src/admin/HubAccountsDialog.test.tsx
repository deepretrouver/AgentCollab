import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HubAccountsDialog } from "./HubAccountsDialog";

const owner = { id: "owner", username: "owner", display_name: "Owner", role: "admin" as const };
const member = { id: "member", username: "member", display_name: "Member", role: "member" as const };

test("Hub 管理员可以授予既有账号管理员角色", async () => {
  const user = userEvent.setup();
  const onUpdateRole = vi.fn().mockResolvedValue({ ...member, role: "admin" as const });
  render(<HubAccountsDialog open currentUser={owner} onClose={vi.fn()} onLoad={vi.fn().mockResolvedValue([owner, member])} onUpdateRole={onUpdateRole} onArchiveAccount={vi.fn()} />);

  expect(await screen.findByText("Member")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "设为管理员" }));
  expect(onUpdateRole).toHaveBeenCalledWith("member", "admin");
  expect(await screen.findByText("默认 Hub 管理员")).toBeInTheDocument();
  expect(await screen.findByText("Hub 管理员")).toBeInTheDocument();
  expect(screen.getByText("当前账号")).toBeInTheDocument();
});

test("删除账号需要二次确认，成功后从当前名单移除", async () => {
  const user = userEvent.setup();
  const onArchiveAccount = vi.fn().mockResolvedValue(undefined);
  render(<HubAccountsDialog open currentUser={owner} onClose={vi.fn()} onLoad={vi.fn().mockResolvedValue([owner, member])} onUpdateRole={vi.fn()} onArchiveAccount={onArchiveAccount} />);

  expect(await screen.findByText("Member")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "删除账号 Member" }));
  expect(onArchiveAccount).not.toHaveBeenCalled();
  expect(screen.getByText("删除后，该账号将无法登录并从 Hub 名单隐藏；历史消息会保留原署名。")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "确认删除账号 Member" }));
  expect(onArchiveAccount).toHaveBeenCalledWith("member");
  expect(screen.queryByText("Member")).not.toBeInTheDocument();
});
