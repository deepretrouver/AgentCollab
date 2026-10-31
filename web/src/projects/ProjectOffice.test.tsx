import { act, fireEvent, render, screen } from "@testing-library/react";
import { ProjectOffice } from "./ProjectOffice";
import { setLocale } from "../i18n/locale";
import type { WorkItem, ProjectPlan } from "../api/client";

afterEach(() => act(() => setLocale("zh-CN")));
const item: WorkItem = { id: "i", workspace_id: "w", room_id: "r", root_message_id: "root", merge_status: "none", kind: "task", title: "Original task", description: "", status: "review", priority: "normal", task_count: 1, active_task_count: 0, delivered_task_count: 1, failed_task_count: 0, target_agent_ids: [], created_at: "2026-10-30T01:00:00Z", updated_at: "2026-10-30T01:00:00Z" };

test("English office has readable empty state and switches language", () => {
  setLocale("en");
  const { container } = render(<ProjectOffice items={[]} members={[]} agents={[]} onOpenChat={vi.fn()} onUpdateStatus={vi.fn()} />);
  expect(screen.getByText("Project office")).toBeInTheDocument();
  expect(container.textContent).not.toMatch(/[\u4e00-\u9fff]/);
  act(() => setLocale("zh-CN"));
  expect(screen.getByText("总办公室")).toBeInTheDocument();
});

test("English detail tabs preserve task actions", () => {
  setLocale("en");
  const update = vi.fn().mockResolvedValue(undefined);
  const open = vi.fn();
  const { container } = render(<ProjectOffice items={[item]} members={[]} agents={[]} onOpenChat={open} onUpdateStatus={update} />);
  fireEvent.click(screen.getAllByRole("button", { name: /Original task/ })[0]);
  for (const name of ["Business", "Execution trace", "Git and acceptance", "Shared context"]) {
    fireEvent.click(screen.getByRole("button", { name }));
    expect(container.textContent).not.toMatch(/[\u4e00-\u9fff]/);
  }
  fireEvent.click(screen.getByRole("button", { name: "Business" }));
  fireEvent.change(screen.getByLabelText("Update work item status"), { target: { value: "done" } });
  expect(update).toHaveBeenCalledWith("i", "done");
  fireEvent.click(screen.getByRole("button", { name: "View original topic and full delivery" }));
  expect(open).toHaveBeenCalledWith("root");
});

test("English step cards preserve project names and show counts without Chinese UI text", () => {
  setLocale("en");
  const plan: ProjectPlan = { workspace_id: "w", objective: "Original objective", status: "active", progress: 0, can_manage: true, steps: [{ id: "s", workspace_id: "w", title: "Original step", description: "", weight: 2, status: "in_progress", sort_order: 0, progress: 0, linked_item_count: 5, responsible_user_ids: [], responsible_agent_ids: [], created_at: item.created_at, updated_at: item.updated_at }] };
  const { container } = render(<ProjectOffice plan={plan} items={Array.from({ length: 5 }, (_, i) => ({ ...item, id: String(i), plan_step_id: "s" }))} members={[]} agents={[]} onOpenChat={vi.fn()} onUpdateStatus={vi.fn()} />);
  expect(screen.getByText("Original objective")).toBeInTheDocument();
  expect(screen.getByText("Original step")).toBeInTheDocument();
  expect(screen.getByText("No owner assigned")).toBeInTheDocument();
  expect(container.textContent).not.toMatch(/[\u4e00-\u9fff]/);
});
