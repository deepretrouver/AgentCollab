import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { Agent, KnowledgeAsset, Message, ProjectPlan, Task, User, WorkItem } from "../api/client";
import { ProjectCockpit } from "./ProjectCockpit";

const owner: User = { id: "user-1", username: "owner", display_name: "负责人", role: "admin" };
const agent: Agent = {
  id: "agent-1",
  owner_user_id: owner.id,
  owner_name: owner.display_name,
  device_id: "device-1",
  device_name: "验收电脑",
  os: "darwin",
  arch: "arm64",
  channel: "cli",
  type: "codex",
  display_name: "owner Codex",
  permission_mode: "admin",
  status: "online",
  imported: true,
  available: true,
  version: "test",
  executable: "codex",
  native_history: "",
  response_mode: "mention",
  reply_delay_seconds: 0,
  role_prompt: "",
  max_auto_turns: 1,
};
const workItem: WorkItem = {
  id: "work-1",
  workspace_id: "workspace-1",
  room_id: "room-1",
  root_message_id: "message-1",
  git_branch: "issue/message-1",
  merge_status: "open",
  kind: "task",
  title: "审查投资报告",
  description: "审查报告并给出风险结论",
  status: "review",
  priority: "high",
  owner_user_id: owner.id,
  task_count: 2,
  active_task_count: 0,
  delivered_task_count: 2,
  failed_task_count: 0,
  target_agent_ids: [agent.id],
  created_at: "2026-08-09T03:00:00Z",
  updated_at: "2026-08-09T03:10:00Z",
};

const plan: ProjectPlan = {
  workspace_id: "workspace-1",
  objective: "完成投资报告审查并形成可合并成果",
  status: "active",
  progress: 0,
  can_manage: true,
  steps: [{ id: "step-1", workspace_id: "workspace-1", title: "审查投资报告", description: "", weight: 100, status: "review", sort_order: 0, progress: 0, linked_item_count: 1, responsible_user_ids: [], responsible_agent_ids: [], created_at: "2026-08-09T02:00:00Z", updated_at: "2026-08-09T03:10:00Z" }],
};
workItem.plan_step_id = plan.steps[0].id;

afterEach(() => {
  cleanup();
  delete window.agentCollabDesktop;
});

test("总览把 Agent 交付显示为待验收而不是自动完成", () => {
  render(<ProjectCockpit view="overview" currentUser={owner} plan={plan} items={[workItem]} messages={[]} members={[owner]} agents={[agent]} onUpdateStatus={vi.fn()} onOpenChat={vi.fn()} />);

  expect(screen.getAllByText("0%").length).toBeGreaterThan(0);
  expect(screen.getAllByText("待验收").length).toBeGreaterThan(0);
  expect(screen.getByText("按计划步骤权重计算 · 0 步已验收 · 0 步进行中 · 1 步待验收")).toBeInTheDocument();
});

test("协作台默认用办公室解释步骤、人员、Agent、目录和待验收任务", async () => {
  const user = userEvent.setup();
  const officePlan: ProjectPlan = { ...plan, steps: [{ ...plan.steps[0], responsible_user_ids: [owner.id], responsible_agent_ids: [agent.id], workspace: { step_id: "step-1", source_name: "红利项目", file_count: 8, total_bytes: 2048, revision: 3, last_commit: "abc", created_at: plan.steps[0].created_at, updated_at: plan.steps[0].updated_at } }] };
  const runningTask: Task = { id: "task-running", work_item_id: workItem.id, plan_step_id: "step-1", workspace_id: "workspace-1", room_id: "room-1", collaboration_depth: 0, creator_kind: "user", creator_id: owner.id, target_agent_id: agent.id, title: "执行仓位复核", prompt: "复核", acceptance_criteria: "", state: "running", current_attempt: 1, created_at: workItem.created_at, updated_at: workItem.updated_at };
  const cancelledItem: WorkItem = { ...workItem, id: "work-cancelled", status: "cancelled", title: "已经取消的旧任务" };
  const archivedFailure: Task = { ...runningTask, id: "task-old-failure", work_item_id: cancelledItem.id, state: "failed", title: cancelledItem.title };
  render(<ProjectCockpit view="board" defaultBoardMode="office" currentUser={owner} plan={officePlan} items={[workItem, cancelledItem]} tasks={[runningTask, archivedFailure]} messages={[]} members={[owner]} agents={[agent]} onUpdateStatus={vi.fn()} onOpenChat={vi.fn()} />);

  expect(screen.getAllByText("总办公室").length).toBeGreaterThan(1);
  expect(screen.getAllByText("负责人").length).toBeGreaterThan(0);
  expect(screen.getAllByText("owner Codex").length).toBeGreaterThan(0);
  expect(screen.getByText("团队底稿 v3")).toBeInTheDocument();
  expect(screen.getByText("总控台")).toBeInTheDocument();
  expect(screen.getByText("全员席位")).toBeInTheDocument();
  expect(screen.getByText("正在执行").parentElement).toHaveTextContent("1正在执行");
  expect(screen.getByText("待我处理").parentElement).toHaveTextContent("1待我处理");
  await user.click(screen.getAllByRole("button", { name: /审查投资报告/ })[0]);
  const detail = screen.getByRole("complementary", { name: "审查投资报告 详情" });
  expect(detail).toHaveTextContent("业务执行链路Git 与验收共享上下文");
  await user.click(screen.getByRole("button", { name: "Git 与验收" }));
  expect(detail).toHaveTextContent("issue/message-1");
  await user.click(screen.getByRole("button", { name: "共享上下文" }));
  expect(detail).toHaveTextContent("完成投资报告审查并形成可合并成果");

  await user.click(screen.getByRole("button", { name: "结构看板" }));
  expect(screen.getByText("项目计划")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "执行时间线" }));
  expect(screen.getByText("原生 Agent 执行中")).toBeInTheDocument();
});

test("总办公室和总览不把普通点名、本地验证计入待处理", () => {
  const structuredRoot: Message = { id: "message-formal", workspace_id: "workspace-1", room_id: "room-1", sequence: 1, sender_kind: "user", sender_id: owner.id, kind: "chat", body: "正式复核", metadata: { task_source: "structured", task_title: "正式复核" }, created_at: workItem.created_at };
  const ordinaryRoot: Message = { ...structuredRoot, id: "message-chat", sequence: 2, body: "@Codex 介绍下自己", metadata: undefined };
  const verificationRoot: Message = { ...structuredRoot, id: "message-verify", sequence: 3, body: "回复 LOCAL_OK", metadata: { task_source: "structured", task_title: "owner Codex 本地身份验证" } };
  const formal: WorkItem = { ...workItem, id: "formal", root_message_id: structuredRoot.id, plan_step_id: undefined, git_branch: undefined, merge_status: "none", title: "正式复核", failed_task_count: 1, delivered_task_count: 0 };
  const ordinary: WorkItem = { ...formal, id: "ordinary", root_message_id: ordinaryRoot.id, title: "@Codex 介绍下自己" };
  const verification: WorkItem = { ...formal, id: "verification", root_message_id: verificationRoot.id, title: "owner Codex 本地身份验证" };

  render(<ProjectCockpit view="board" defaultBoardMode="office" currentUser={owner} plan={plan} items={[formal, ordinary, verification]} messages={[structuredRoot, ordinaryRoot, verificationRoot]} members={[owner]} agents={[agent]} onUpdateStatus={vi.fn()} onOpenChat={vi.fn()} />);

  expect(screen.getByText("待我处理").parentElement).toHaveTextContent("1待我处理");
  expect(screen.getAllByText("正式复核").length).toBeGreaterThan(0);
  expect(screen.queryByText("@Codex 介绍下自己")).not.toBeInTheDocument();
  expect(screen.queryByText("owner Codex 本地身份验证")).not.toBeInTheDocument();
});

test("产物只汇总正式任务并可直接验收，不混入普通点名和本地验证", async () => {
  const user = userEvent.setup();
  const onUpdateStatus = vi.fn().mockResolvedValue(undefined);
  const structuredRoot: Message = { id: "message-structured", workspace_id: "workspace-1", room_id: "room-1", sequence: 2, sender_kind: "user", sender_id: owner.id, kind: "chat", body: "形成正式结论", metadata: { task_source: "structured", task_title: "形成正式结论" }, created_at: workItem.created_at };
  const ordinaryRoot: Message = { ...structuredRoot, id: "message-ordinary", sequence: 3, body: "@Codex 随便聊聊", metadata: undefined };
  const verificationRoot: Message = { ...structuredRoot, id: "message-verification", sequence: 4, body: "回复 LOCAL_OK", metadata: { task_source: "structured", task_title: "owner Codex 本地身份验证" } };
  const explicit = { ...workItem, id: "work-structured", root_message_id: structuredRoot.id, plan_step_id: undefined, git_branch: undefined, merge_status: "none" as const, title: "形成正式结论" };
  const ordinary = { ...explicit, id: "work-ordinary", root_message_id: ordinaryRoot.id, title: "普通点名对话" };
  const verification = { ...explicit, id: "work-verification", root_message_id: verificationRoot.id, title: "owner Codex 本地身份验证" };

  render(<ProjectCockpit view="artifacts" currentUser={owner} plan={plan} items={[workItem, explicit, ordinary, verification]} messages={[structuredRoot, ordinaryRoot, verificationRoot]} members={[owner]} agents={[agent]} onUpdateStatus={onUpdateStatus} onOpenChat={vi.fn()} />);

  expect(screen.getByText("正式交付")).toBeInTheDocument();
  expect(screen.getByText("形成正式结论")).toBeInTheDocument();
  expect(screen.getByText("审查投资报告")).toBeInTheDocument();
  expect(screen.queryByText("普通点名对话")).not.toBeInTheDocument();
  expect(screen.queryByText("owner Codex 本地身份验证")).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "验收通过" }));
  expect(onUpdateStatus).toHaveBeenCalledWith(explicit.id, "done");
});

test("看板先由 AI 审查，再允许管理员合并", async () => {
  const user = userEvent.setup();
  const onUpdateStatus = vi.fn().mockResolvedValue(undefined);
  const onReviewWorkItem = vi.fn().mockResolvedValue({
    branch: workItem.git_branch,
    base_commit: "base",
    head_commit: "head",
    mergeable: true,
    file_count: 1,
    additions: 12,
    deletions: 3,
    changed_files: [{ path: "reports/final.md", status: "M", additions: 12, deletions: 3 }],
  });
  const onMergeWorkItem = vi.fn().mockResolvedValue(undefined);
  const onStartAIReview = vi.fn().mockResolvedValue(undefined);
  const onOpenChat = vi.fn();
  const { rerender } = render(<ProjectCockpit view="board" currentUser={owner} plan={plan} items={[workItem]} messages={[]} members={[owner]} agents={[agent]} roomAgents={[agent]} onUpdateStatus={onUpdateStatus} onReviewWorkItem={onReviewWorkItem} onMergeWorkItem={onMergeWorkItem} onStartAIReview={onStartAIReview} onOpenChat={onOpenChat} />);

  expect(screen.getByText("2/2 已交付")).toBeInTheDocument();
  expect(screen.getByLabelText("审查投资报告 协作关联链")).toHaveTextContent("来源话题审查投资报告2 次执行等待真人验收");
  await user.click(screen.getByRole("button", { name: "来源话题" }));
  expect(onOpenChat).toHaveBeenCalledWith(workItem.root_message_id);
  expect(screen.getByRole("option", { name: "合并后完成" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "发起 AI 审查" }));
  expect(onStartAIReview).toHaveBeenCalledWith(workItem.id, agent.id, agent.id);
  const reviewed = { ...workItem, ai_review_status: "passed" as const, ai_review_summary: "审查结论：建议合并\n主要风险：未发现明显风险" };
  rerender(<ProjectCockpit view="board" currentUser={owner} plan={plan} items={[reviewed]} messages={[]} members={[owner]} agents={[agent]} roomAgents={[agent]} onUpdateStatus={onUpdateStatus} onReviewWorkItem={onReviewWorkItem} onMergeWorkItem={onMergeWorkItem} onStartAIReview={onStartAIReview} onOpenChat={onOpenChat} />);
  expect(screen.getByText("AI 建议合并")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "技术详情" }));
  expect(onReviewWorkItem).toHaveBeenCalledWith(workItem.id);
  expect(screen.getByText("reports/final.md")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "确认合并到主工作区" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "按 AI 建议合并" }));
  expect(onMergeWorkItem).toHaveBeenCalledWith(workItem.id);
});

test("已有步骤可以修改名称层级权重和真人 Agent 负责人", async () => {
  const user = userEvent.setup();
  const onUpdateStep = vi.fn().mockResolvedValue(undefined);
  render(<ProjectCockpit view="board" currentUser={owner} plan={plan} items={[]} messages={[]} members={[owner]} agents={[agent]} onUpdateStatus={vi.fn()} onUpdateStep={onUpdateStep} onOpenChat={vi.fn()} />);

  expect(screen.getByText(/权重 100 \/ 100/)).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "编辑步骤 审查投资报告" }));
  const name = screen.getByRole("textbox", { name: "审查投资报告 新名称" });
  await user.clear(name);
  await user.type(name, "形成终稿");
  const weight = screen.getByRole("spinbutton", { name: "审查投资报告 新权重" });
  await user.clear(weight);
  await user.type(weight, "40");
  const checkboxes = screen.getAllByRole("checkbox");
  await user.click(checkboxes[0]);
  await user.click(checkboxes[1]);
  await user.click(screen.getByRole("button", { name: "保存" }));
  expect(onUpdateStep).toHaveBeenCalledWith("step-1", expect.objectContaining({ title: "形成终稿", weight: 40, responsible_user_ids: [owner.id], responsible_agent_ids: [agent.id] }));
});

test("新增步骤时实时显示同层权重合计和剩余额度", async () => {
  const user = userEvent.setup();
  const weightedPlan: ProjectPlan = {
    ...plan,
    steps: [
      { ...plan.steps[0], id: "step-1", title: "市场研究", weight: 5 },
      { ...plan.steps[0], id: "step-2", title: "标的选择", weight: 5 },
      { ...plan.steps[0], id: "step-3", title: "仓位管理", weight: 40 },
    ],
  };
  render(<ProjectCockpit view="board" currentUser={owner} plan={weightedPlan} items={[]} messages={[]} members={[owner]} agents={[agent]} onUpdateStatus={vi.fn()} onOpenChat={vi.fn()} />);

  await user.click(screen.getByRole("button", { name: "编辑计划" }));
  const ledger = screen.getByLabelText("主要步骤权重实时合计");
  expect(ledger).toHaveTextContent("当前已分配50");
  expect(ledger).toHaveTextContent("本次填写+ 10");
  expect(ledger).toHaveTextContent("填写后总计60 / 100");
  expect(ledger).toHaveTextContent("还可分配 40");

  const input = screen.getByRole("spinbutton", { name: "步骤权重值" });
  await user.clear(input);
  await user.type(input, "50");
  expect(ledger).toHaveTextContent("填写后总计100 / 100");
  expect(ledger).toHaveTextContent("已分配完成");
});

test("步骤主工作区可以二次确认后解除且保留历史说明", async () => {
  const user = userEvent.setup();
  const onRemoveStepWorkspace = vi.fn().mockResolvedValue(undefined);
  const planWithWorkspace: ProjectPlan = { ...plan, steps: [{ ...plan.steps[0], workspace: { step_id: "step-1", source_name: "红利项目", file_count: 339, total_bytes: 1024, revision: 1, last_commit: "abc123", created_at: "2026-08-13T08:00:00Z", updated_at: "2026-08-13T08:00:00Z" } }] };
  render(<ProjectCockpit view="board" currentUser={owner} plan={planWithWorkspace} items={[]} messages={[]} members={[owner]} agents={[agent]} onUpdateStatus={vi.fn()} onRemoveStepWorkspace={onRemoveStepWorkspace} onOpenChat={vi.fn()} />);

  expect(screen.getByText("红利项目")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "解除" }));
  expect(screen.getByText(/历史版本和旧任务记录仍留在 Server/)).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "确认解除" }));
  expect(onRemoveStepWorkspace).toHaveBeenCalledWith("step-1");
});

test("看板步骤默认收起文件操作并在摘要显示两类目录状态", async () => {
  const user = userEvent.setup();
  const planWithWorkspace: ProjectPlan = { ...plan, steps: [{ ...plan.steps[0], workspace: { step_id: "step-1", source_name: "红利项目", file_count: 339, total_bytes: 1024, revision: 3, last_commit: "abc123", created_at: "2026-08-13T08:00:00Z", updated_at: "2026-08-13T08:00:00Z" } }] };
  render(<ProjectCockpit view="board" currentUser={owner} plan={planWithWorkspace} items={[]} messages={[]} members={[owner]} agents={[agent]} onUpdateStatus={vi.fn()} onOpenChat={vi.fn()} />);

  expect(screen.getByText("团队底稿 v3")).toBeInTheDocument();
  expect(screen.getByText("本机目录未绑定")).toBeInTheDocument();
  const summary = screen.getByLabelText("查看步骤详情 审查投资报告");
  expect(summary.closest("details")).not.toHaveAttribute("open");
  await user.click(summary);
  expect(summary.closest("details")).toHaveAttribute("open");
  expect(screen.getByText(/平时在本机目录工作/)).toBeInTheDocument();
  expect(screen.getByText("Server 上的共享里程碑版本")).toBeInTheDocument();
});

test("普通成员可下载团队底稿但不能更新或解除", () => {
  const member: User = { id: "user-2", username: "member", display_name: "参与成员", role: "member" };
  const sharedPlan: ProjectPlan = {
    ...plan,
    can_manage: false,
    steps: [{ ...plan.steps[0], responsible_user_ids: [], workspace: {
      step_id: "step-1", source_name: "共享研究", file_count: 8, total_bytes: 4096,
      revision: 2, last_commit: "shared123", created_at: "2026-08-15T08:00:00Z", updated_at: "2026-08-15T08:00:00Z",
    } }],
  };
  const { container } = render(<ProjectCockpit view="board" currentUser={member} plan={sharedPlan} items={[]} messages={[]} members={[owner, member]} agents={[agent]} onUpdateStatus={vi.fn()} onUploadStepWorkspace={vi.fn()} onRemoveStepWorkspace={vi.fn()} onOpenChat={vi.fn()} />);

  const download = screen.getByRole("link", { name: "下载只读副本" });
  expect(download).toHaveAttribute("href", "/api/v1/project-steps/step-1/workspace");
  expect(screen.queryByRole("button", { name: "更新底稿" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "解除" })).not.toBeInTheDocument();
  expect(container.querySelector('input[type="file"][webkitdirectory]')).not.toBeInTheDocument();
});

test("看板内用大白话解释本地目录和后台 Git 的协作关系", async () => {
  const user = userEvent.setup();
  render(<ProjectCockpit view="board" currentUser={owner} plan={plan} items={[]} messages={[]} members={[owner]} agents={[agent]} onUpdateStatus={vi.fn()} onOpenChat={vi.fn()} />);

  await user.click(screen.getByText("第一次使用？点这里看整个项目怎么协作"));
  expect(screen.getByText("不需要学习 Git 命令")).toBeInTheDocument();
  expect(screen.getByText(/目录仍留在负责人电脑/)).toBeInTheDocument();
  expect(screen.getByText(/默认只读团队底稿/)).toBeInTheDocument();
  expect(screen.getByText(/版本保险箱/)).toBeInTheDocument();
});

test("步骤负责人可在 Client 绑定、打开和解除自己的本机工作目录", async () => {
  const user = userEvent.setup();
  const onBindStepLocal = vi.fn().mockResolvedValue(undefined);
  const onOpenStepLocal = vi.fn().mockResolvedValue(undefined);
  const onRemoveStepLocal = vi.fn().mockResolvedValue(undefined);
  window.agentCollabDesktop = {
    isDesktop: true,
    platform: "darwin",
    getState: vi.fn().mockResolvedValue({ configured: true, runnerRegistered: true, runnerRunning: true, runnerId: "runner-1", deviceId: "device-1", platform: "darwin", version: "1.3.1" }),
    configure: vi.fn(), connectCurrentComputer: vi.fn(), reconnect: vi.fn(), openSettings: vi.fn(), onStatus: vi.fn(() => vi.fn()),
    bindProjectStep: vi.fn(), unbindProjectStep: vi.fn(), openProjectStep: vi.fn(),
  };
  const responsiblePlan: ProjectPlan = {
    ...plan,
    steps: [{ ...plan.steps[0], responsible_user_ids: [owner.id], responsible_agent_ids: [agent.id], local_bindings: [{
      step_id: "step-1", user_id: owner.id, runner_id: "runner-1", device_id: "device-1", device_name: "验收电脑", directory_label: "红利研究", runner_status: "online", created_at: "2026-08-13T08:00:00Z", updated_at: "2026-08-13T08:00:00Z",
    }] }],
  };
  render(<ProjectCockpit view="board" currentUser={owner} plan={responsiblePlan} items={[]} messages={[]} members={[owner]} agents={[agent]} onUpdateStatus={vi.fn()} onBindStepLocal={onBindStepLocal} onOpenStepLocal={onOpenStepLocal} onRemoveStepLocal={onRemoveStepLocal} onOpenChat={vi.fn()} />);

  expect(await screen.findByText(/红利研究 · 可查询最新进展/)).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "打开" }));
  await user.click(screen.getByRole("button", { name: "更换" }));
  await user.click(screen.getByRole("button", { name: "解除" }));
  expect(onOpenStepLocal).toHaveBeenCalledWith("step-1");
  expect(onBindStepLocal).toHaveBeenCalledWith("step-1");
  expect(onRemoveStepLocal).toHaveBeenCalledWith("step-1");
});

test("知识库区分本群资料与工作区共享并可上传", async () => {
  const user = userEvent.setup();
  const onUpload = vi.fn().mockResolvedValue(undefined);
  const asset: KnowledgeAsset = {
    id: "knowledge-1", workspace_id: "workspace-1", room_id: "room-1", scope: "room",
    uploader_user_id: owner.id, uploader_name: owner.display_name, filename: "投委会规范.md",
    content_type: "text/markdown", size_bytes: 1280, sha256: "abc", download_url: "/knowledge/1",
    can_delete: true, created_at: "2026-08-11T02:00:00Z",
  };
  const { container } = render(<ProjectCockpit view="knowledge" currentUser={owner} items={[]} messages={[]} members={[owner]} agents={[agent]}
    roomKnowledge={{ assets: [asset], directory: "workspaces/workspace-1/rooms/room-1/knowledge" }}
    workspaceKnowledge={{ assets: [], directory: "workspaces/workspace-1/knowledge" }}
    onUploadKnowledge={onUpload} onDeleteKnowledge={vi.fn()} onUpdateStatus={vi.fn()} onOpenChat={vi.fn()} />);

  expect(screen.getByText("投委会规范.md")).toBeInTheDocument();
  expect(screen.getByText("工作区共享")).toBeInTheDocument();
  const inputs = container.querySelectorAll<HTMLInputElement>('input[type="file"]');
  await user.upload(inputs[0], new File(["reference"], "参考.txt", { type: "text/plain" }));
  expect(onUpload).toHaveBeenCalledWith("room", expect.objectContaining({ name: "参考.txt" }));
});
