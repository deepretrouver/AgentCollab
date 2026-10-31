import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { Agent } from "../api/client";
import { setLocale } from "../i18n/locale";
import { RoomOnboarding } from "./RoomOnboarding";

beforeEach(() => setLocale("en"));
afterEach(() => act(() => setLocale("zh-CN")));

const props = () => ({ registeredCount: 0, agents: [] as Agent[], verifying: false, verificationError: "", onManageAgents: vi.fn(), onShowMembers: vi.fn(), onVerify: vi.fn().mockResolvedValue(undefined) });

test("empty workspace explains Client requirement and opens onboarding", async () => {
  const p = props();
  const { container } = render(<RoomOnboarding {...p} />);
  expect(screen.getByText(/people can chat without installing it/)).toBeInTheDocument();
  expect(container.textContent).not.toMatch(/[\u4e00-\u9fff]/);
  await userEvent.click(screen.getByRole("button", { name: "Add my agents" }));
  expect(p.onManageAgents).toHaveBeenCalledOnce();
  expect(p.onShowMembers).not.toHaveBeenCalled();
});

test("registered agents route to member selection and language switching preserves count", async () => {
  const p = props();
  render(<RoomOnboarding {...p} registeredCount={3} />);
  expect(screen.getByText(/3 registered agents/)).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Open member list" }));
  expect(p.onShowMembers).toHaveBeenCalledOnce();
  act(() => setLocale("zh-CN"));
  expect(screen.getByText(/工作区已注册 3 个 Agent/)).toBeInTheDocument();
});

test("verification is explicit, prevents repeat submission, and preserves agent and error text", async () => {
  const p = props();
  // This presentation-only fixture needs only the fields rendered by the component.
  const agents = [{ id: "a", display_name: "研究助手", device_name: "Office", channel: "runner" }, { id: "b", display_name: "Reviewer", device_name: "Laptop", channel: "runner" }] as Agent[];
  const { rerender } = render(<RoomOnboarding {...p} agents={agents} />);
  expect(screen.getByText(/Membership does not guarantee availability/)).toBeInTheDocument();
  expect(screen.getByText("研究助手")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Check agent connections" }));
  expect(p.onVerify).toHaveBeenCalledOnce();
  rerender(<RoomOnboarding {...p} agents={agents} verifying verificationError="Provider unavailable" />);
  expect(screen.getByRole("button", { name: "Checking" })).toBeDisabled();
  expect(screen.getByRole("alert")).toHaveTextContent("Provider unavailable");
  rerender(<RoomOnboarding {...p} agents={agents.slice(0, 1)} />);
  expect(screen.getByRole("button", { name: "Check agent connections" })).toBeDisabled();
});
