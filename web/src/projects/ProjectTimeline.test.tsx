import { act, fireEvent, render, screen } from "@testing-library/react";
import { ProjectTimeline } from "./ProjectOffice";
import { setLocale } from "../i18n/locale";
import type { Task, WorkItem } from "../api/client";

afterEach(() => act(() => setLocale("zh-CN")));
const task: Task = { id: "t", workspace_id: "w", room_id: "r", root_message_id: "root", collaboration_depth: 0, creator_kind: "user", creator_id: "u", target_agent_id: "a", title: "原始中文任务", prompt: "", acceptance_criteria: "", state: "queued", current_attempt: 2, created_at: "2026-10-30T01:00:00Z", updated_at: "2026-10-30T01:00:00Z" };
const item: WorkItem = { id: "i", workspace_id: "w", room_id: "r", root_message_id: "root", merge_status: "none", kind: "task", title: "原始事项", description: "", status: "in_progress", priority: "normal", task_count: 3, active_task_count: 2, delivered_task_count: 1, failed_task_count: 0, target_agent_ids: [], created_at: task.created_at, updated_at: task.updated_at };

test.each<Task["state"]>(["queued", "dispatched", "running", "blocked", "failed"])("English timeline shows %s and opens the original topic", state => {
  setLocale("en");
  const open = vi.fn();
  const { container } = render(<ProjectTimeline items={[]} agents={[]} tasks={[{ ...task, state }]} onOpenChat={open} />);
  expect(screen.getByText("Execution timeline")).toBeInTheDocument();
  expect(container.textContent).toContain("Attempt 2");
  fireEvent.click(screen.getByRole("button", { name: /原始中文任务/ }));
  expect(open).toHaveBeenCalledWith("root");
  expect(container.textContent?.replace("原始中文任务", "")).not.toMatch(/[\u4e00-\u9fff]/);
});

test("English item counts and language switching preserve business content", () => {
  setLocale("en");
  render(<ProjectTimeline items={[item]} agents={[]} onOpenChat={vi.fn()} />);
  expect(screen.getByText("2 running")).toBeInTheDocument();
  expect(screen.getByText("原始事项")).toBeInTheDocument();
  act(() => setLocale("zh-CN"));
  expect(screen.getByText("2 项执行中")).toBeInTheDocument();
});

test("failed post-merge verification takes priority over merged status", () => {
  setLocale("en");
  render(<ProjectTimeline items={[{ ...item, merge_status: "merged", verification_status: "failed" }]} agents={[]} onOpenChat={vi.fn()} />);
  expect(screen.getByText("Merge verification failed")).toBeInTheDocument();
  expect(screen.queryByText("Merged into team baseline")).not.toBeInTheDocument();
});
