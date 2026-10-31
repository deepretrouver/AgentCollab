import { act, render, screen } from "@testing-library/react";
import type { Message } from "../api/client";
import { MessageItem } from "./MessageItem";
import { setLocale } from "../i18n/locale";

beforeEach(() => setLocale("zh-CN"));
afterEach(() => act(() => setLocale("zh-CN")));

const message: Message = {
  id: "message-1",
  workspace_id: "workspace-1",
  room_id: "room-1",
  sequence: 1,
  sender_kind: "agent",
  sender_id: "agent-1",
  kind: "chat",
  body: "已完成第一轮分析",
  created_at: "2026-07-16T03:00:00Z",
};

test("明确区分 Agent 消息", () => {
  render(<MessageItem message={message} senderName="Hermes Research" own={false} />);

  expect(screen.getByText("Hermes Research")).toBeInTheDocument();
  expect(screen.getByText("Agent")).toBeInTheDocument();
  expect(screen.getByText("已完成第一轮分析")).toBeInTheDocument();
});

test("English labels preserve original names, body, task title and execution receipt", () => {
  setLocale("en");
  const { rerender } = render(<MessageItem message={{ ...message, sender_kind: "user", metadata: { task_title: "研究任务" } }} senderName="研究员" own />);
  expect(screen.getByText("Human")).toBeInTheDocument();
  expect(screen.getByText("Task")).toBeInTheDocument();
  expect(screen.getByText("研究任务")).toBeInTheDocument();
  expect(screen.getByText("研究员")).toBeInTheDocument();
  expect(screen.getByText(message.body)).toBeInTheDocument();
  rerender(<MessageItem message={{ ...message, body: "交付：内部标题\n\n结果原文\n\n执行凭证\n原始凭证" }} senderName="研究员" own={false} variant="topic-reply" taskTitle="研究$&任务" />);
  expect(screen.getByText("Task delivery: 研究$&任务")).toBeInTheDocument();
  expect(screen.getByText("结果原文")).toBeInTheDocument();
  const receipt = screen.getByText("View execution receipt").closest("details");
  expect(receipt).not.toHaveAttribute("open");
  expect(screen.getByText("原始凭证")).toBeInTheDocument();
  rerender(<MessageItem message={{ ...message, body: "任务失败：内部标题\n\n未连接" }} senderName="研究员" own={false} />);
  expect(screen.getByText("Execution failed")).toBeInTheDocument();
  expect(screen.getByText("未连接")).toBeInTheDocument();
});

test("交付内容优先显示，执行凭证默认折叠", () => {
  render(<MessageItem message={{ ...message, kind: "delivery", body: "交付：群聊点名：Hermes\n\n这是正式回复\n\n执行凭证\nCLI：/opt/homebrew/bin/hermes\n进程 PID：42" }} senderName="Hermes" own={false} />);

  expect(screen.getByText("这是正式回复")).toBeInTheDocument();
  expect(screen.getByText("群聊点名：Hermes")).toBeInTheDocument();
  expect(screen.getByText("查看执行凭证")).toBeInTheDocument();
  expect(screen.queryByText(/交付：群聊点名/)).not.toBeInTheDocument();
});

test("话题中的交付不展示系统生成的群聊点名标题", () => {
  render(<MessageItem message={{ ...message, kind: "delivery", body: "交付：群聊点名：Hermes\n\n这是正式回复" }} senderName="Hermes" own={false} variant="topic-reply" />);

  expect(screen.getByText("任务交付")).toBeInTheDocument();
  expect(screen.queryByText("群聊点名：Hermes")).not.toBeInTheDocument();
});

test("群聊级 brainstorm 显示为专题讨论卡片", () => {
  render(<MessageItem message={{ ...message, sender_kind: "user", sender_id: "user-1", body: "/brainstorm 评估股票技能的实盘效果" }} senderName="Owner" own />);

  expect(screen.getByText("专题讨论")).toBeInTheDocument();
  expect(screen.getByText("评估股票技能的实盘效果")).toBeInTheDocument();
  expect(screen.queryByText(/\/brainstorm/)).not.toBeInTheDocument();
});

test("群消息显示可下载的报告附件", () => {
  render(<MessageItem message={{ ...message, sender_kind: "user", sender_id: "user-1", attachments: [{
    id: "attachment-1", workspace_id: "workspace-1", room_id: "room-1", uploader_user_id: "user-1",
    message_id: "message-1", filename: "授信报告.pdf", content_type: "application/pdf", size_bytes: 2048,
    sha256: "abc", download_url: "/api/v1/rooms/room-1/attachments/attachment-1", created_at: message.created_at,
  }] }} senderName="Owner" own />);

  const link = screen.getByRole("link", { name: /授信报告\.pdf/ });
  expect(link).toHaveAttribute("href", "/api/v1/rooms/room-1/attachments/attachment-1");
  expect(link).toHaveTextContent("2.0 KB");
});
