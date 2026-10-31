import { act, fireEvent, render, screen } from "@testing-library/react";
import { WorkItemReviewPanel } from "./WorkItemReviewPanel";
import { setLocale } from "../i18n/locale";
import type { WorkItemReview } from "../api/client";

const review: WorkItemReview = {
  branch: "tasks/测试-1", base_commit: "base", head_commit: "head", mergeable: true,
  file_count: 1, additions: 2, deletions: 1,
  changed_files: [{ path: "src/原文.txt", status: "M", additions: 2, deletions: 1 }],
};
afterEach(() => act(() => setLocale("zh-CN")));

test.each([["A", "Added"], ["M", "Modified"], ["D", "Deleted"], ["R", "Renamed"], ["?", "?"]])("English file status %s is readable without changing file names", (status, label) => {
  setLocale("en");
  render(<WorkItemReviewPanel review={{ ...review, changed_files: [{ ...review.changed_files[0], status }] }} onClose={vi.fn()} />);
  expect(screen.getByText(label)).toBeInTheDocument();
  expect(screen.getByText("src/原文.txt")).toBeInTheDocument();
});

test("English review preserves paths and delegates explicit merge and close", () => {
  setLocale("en");
  const merge = vi.fn();
  const close = vi.fn();
  render(<WorkItemReviewPanel review={review} onMerge={merge} onClose={close} />);
  expect(screen.getByText("Review before merging")).toBeInTheDocument();
  expect(screen.getByText("Modified")).toBeInTheDocument();
  expect(screen.getByText("src/原文.txt")).toBeInTheDocument();
  expect(screen.getByText("tasks/测试-1")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Confirm merge into main workspace" }));
  expect(merge).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "Close change review" }));
  expect(close).toHaveBeenCalledTimes(1);
  act(() => setLocale("zh-CN"));
  expect(screen.getByText("合并前审查")).toBeInTheDocument();
});

test("English conflict never offers merge", () => {
  setLocale("en");
  render(<WorkItemReviewPanel review={{ ...review, mergeable: false }} onMerge={vi.fn()} onClose={vi.fn()} />);
  expect(screen.getByText(/The branch conflicts with the main workspace/)).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: /Confirm merge/ })).not.toBeInTheDocument();
});

test("English merge is disabled while pending and absent without permission", () => {
  setLocale("en");
  const { rerender } = render(<WorkItemReviewPanel review={review} merging onMerge={vi.fn()} onClose={vi.fn()} />);
  expect(screen.getByRole("button", { name: "Merging" })).toBeDisabled();
  rerender(<WorkItemReviewPanel review={review} onClose={vi.fn()} />);
  expect(screen.queryByRole("button", { name: /Confirm merge|Merging/ })).not.toBeInTheDocument();
});
