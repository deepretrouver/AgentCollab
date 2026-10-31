import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { ArrowRight, BookOpenText, Buildings, CaretDown, ChatCircleText, Check, CheckCircle, ClockCountdown, DownloadSimple, FileText, FolderOpen, GitBranch, ListChecks, PencilSimple, Plus, Robot, SpinnerGap, Trash, UploadSimple, WarningCircle, X } from "@phosphor-icons/react";
import type { Agent, KnowledgeAsset, KnowledgeCollection, Message, ProjectPlan, ProjectPlanStatus, ProjectRepository, ProjectStep, ProjectStepStatus, Task, User, WorkItem, WorkItemReview, WorkItemStatus } from "../api/client";
import { WorkItemReviewPanel } from "./WorkItemReviewPanel";
import { formalDeliveryItems, formalProjectItems, formalProjectTasks } from "./workItemPresentation";
import { ProjectOffice, ProjectTimeline } from "./ProjectOffice";
import { useLocale } from "../i18n/locale";

interface ProjectCockpitProps {
  view: "overview" | "board" | "knowledge" | "artifacts";
  defaultBoardMode?: "office" | "board" | "timeline";
  currentUser: User;
  plan?: ProjectPlan;
  repository?: ProjectRepository;
  items: WorkItem[];
  tasks?: Task[];
  messages: Message[];
  members: User[];
  agents: Agent[];
  roomAgents?: Agent[];
  workspaceKnowledge?: KnowledgeCollection;
  roomKnowledge?: KnowledgeCollection;
  knowledgeLoading?: boolean;
  knowledgeError?: string;
  onUploadKnowledge?: (scope: "workspace" | "room", file: File) => Promise<void>;
  onDeleteKnowledge?: (scope: "workspace" | "room", assetID: string) => Promise<void>;
  onUpdateStatus: (itemID: string, status: WorkItemStatus) => Promise<void>;
  onReviewWorkItem?: (itemID: string) => Promise<WorkItemReview>;
  onMergeWorkItem?: (itemID: string) => Promise<void>;
  onStartAIReview?: (itemID: string, reviewAgentID: string, verificationAgentID: string) => Promise<void>;
  onRejectWorkItem?: (itemID: string, reason: string) => Promise<void>;
  onUpdatePlan?: (patch: Partial<{ objective: string; status: ProjectPlanStatus }>) => Promise<void>;
  onCreateStep?: (input: { parent_step_id?: string; title: string; description?: string; weight: number; status?: ProjectStepStatus; sort_order?: number; responsible_user_ids?: string[]; responsible_agent_ids?: string[] }) => Promise<void>;
  onUpdateStep?: (stepID: string, patch: Partial<{ parent_step_id: string; clear_parent: boolean; title: string; description: string; weight: number; status: ProjectStepStatus; sort_order: number; responsible_user_ids: string[]; responsible_agent_ids: string[] }>) => Promise<void>;
  onDeleteStep?: (stepID: string) => Promise<void>;
  onUploadStepWorkspace?: (stepID: string, files: File[]) => Promise<void>;
  onRemoveStepWorkspace?: (stepID: string) => Promise<void>;
  onBindStepLocal?: (stepID: string) => Promise<void>;
  onRemoveStepLocal?: (stepID: string) => Promise<void>;
  onOpenStepLocal?: (stepID: string) => Promise<void>;
  onSubmitWorkItemWorkspace?: (itemID: string, files: File[]) => Promise<void>;
  onOpenChat: (rootMessageID?: string) => void;
}

const statusLabels: Record<WorkItemStatus, string> = {
  backlog: "待整理",
  ready: "待开始",
  in_progress: "进行中",
  review: "待验收",
  done: "已完成",
  cancelled: "已取消",
};

const boardColumns: Array<{ status: WorkItemStatus; title: string; hint: string }> = [
  { status: "backlog", title: "待整理", hint: "还需要澄清或排期" },
  { status: "ready", title: "待开始", hint: "范围明确，尚未执行" },
  { status: "in_progress", title: "进行中", hint: "真人或 Agent 正在推进" },
  { status: "review", title: "待验收", hint: "已有交付，等待真人确认" },
  { status: "done", title: "已完成", hint: "已由真人验收" },
];

function EmptyProject({ onOpenChat }: { onOpenChat: () => void }) {
  return <div className="project-empty">
    <span><Robot size={24} weight="duotone" /></span>
    <h3>还没有工作事项</h3>
    <p>群主或工作区管理员先设置总目标、主要步骤和权重；普通聊天不会自动算作项目进度。</p>
    <button type="button" onClick={() => onOpenChat()}>返回聊天 <ArrowRight size={16} /></button>
  </div>;
}

function WorkItemRow({ item, agents, onOpenChat }: { item: WorkItem; agents: Agent[]; onOpenChat: (rootMessageID?: string) => void }) {
  const names = item.target_agent_ids.map((id) => agents.find((agent) => agent.id === id)?.display_name).filter(Boolean);
  return <button type="button" className="project-work-row" onClick={() => onOpenChat(item.root_message_id)}>
    <span className={`project-status-dot status-${item.status}`} />
    <span className="project-work-copy"><strong>{item.title}</strong><small>{names.length ? names.join("、") : "真人事项"} · {item.task_count} 次执行</small></span>
    <span className={`project-status-label status-${item.status}`}>{statusLabels[item.status]}</span>
  </button>;
}

function workItemStage(item: WorkItem) {
  if (item.verification_status === "passed") return "合并校验通过";
  if (item.verification_status === "pending") return "合并后校验中";
  if (item.merge_status === "merged") return "已合并主项目";
  if (item.ai_review_status === "passed") return "审查通过，待合并";
  if (item.ai_review_status === "pending") return "AI 审查中";
  if (item.status === "review") return "等待真人验收";
  if (item.active_task_count > 0) return `${item.active_task_count} 项执行中`;
  if (item.delivered_task_count > 0) return `${item.delivered_task_count} 项已交付`;
  return statusLabels[item.status];
}

function WorkLinkChain({ item, stepTitle, onOpenChat }: { item: WorkItem; stepTitle?: string; onOpenChat: (rootMessageID?: string) => void }) {
  return <div className="work-link-chain" aria-label={`${item.title} 协作关联链`}>
    <button type="button" onClick={() => onOpenChat(item.root_message_id)} disabled={!item.root_message_id} title="回到发起这项工作的原始话题"><ChatCircleText size={13} />来源话题</button>
    <ArrowRight size={12} aria-hidden="true" />
    <span title={stepTitle ?? "未归入项目步骤"}><GitBranch size={13} />{stepTitle ?? "未归入步骤"}</span>
    <ArrowRight size={12} aria-hidden="true" />
    <span><Robot size={13} />{item.task_count} 次执行</span>
    <ArrowRight size={12} aria-hidden="true" />
    <strong>{workItemStage(item)}</strong>
  </div>;
}

function Overview({ plan, repository, items, members, agents, onOpenChat }: Pick<ProjectCockpitProps, "plan" | "repository" | "items" | "members" | "agents" | "onOpenChat">) {
  if (!plan || plan.steps.length === 0) return <EmptyProject onOpenChat={onOpenChat} />;
  const done = plan.steps.filter((step) => step.status === "done").length;
  const review = plan.steps.filter((step) => step.status === "review").length;
  const active = plan.steps.filter((step) => step.status === "in_progress").length;
  const risks = items.filter((item) => item.status !== "done" && item.status !== "cancelled" && (item.status === "review" || item.failed_task_count > 0 || (item.due_at && new Date(item.due_at).getTime() < Date.now())));
  const completion = plan.progress;
  return <div className="project-overview">
    <section className="project-summary-card project-progress-card">
      <div><span>项目进度</span><strong>{completion}%</strong></div>
      <div className="project-progress-track"><span style={{ width: `${completion}%` }} /></div>
      <p>按计划步骤权重计算 · {done} 步已验收 · {active} 步进行中 · {review} 步待验收</p>
    </section>
    <section className="project-metric-grid">
      <article><ClockCountdown size={20} /><span>进行中</span><strong>{active}</strong></article>
      <article><CheckCircle size={20} /><span>待验收</span><strong>{review}</strong></article>
      <article className={risks.length ? "has-risk" : ""}><WarningCircle size={20} /><span>需关注</span><strong>{risks.length}</strong></article>
      <article><Robot size={20} /><span>协作成员</span><strong>{members.length + agents.length}</strong></article>
    </section>
    <div className="project-overview-columns">
      <section className="project-panel"><header><div><strong>项目目标与计划</strong><small>{plan.objective || "尚未填写项目总目标"}</small></div><button type="button" onClick={() => onOpenChat()}>发起讨论</button></header><div className="project-step-summary">{plan.steps.filter((step) => !step.parent_step_id).map((step) => <div key={step.id}><span><strong>{step.title}</strong><small>权重 {step.weight} · {step.linked_item_count} 个关联会话</small></span><em>{step.progress}%</em></div>)}</div></section>
      <section className="project-panel project-risk-panel"><header><div><strong>风险与等待</strong><small>失败执行、逾期或待真人验收</small></div></header>{risks.length ? risks.slice(0, 5).map((item) => <WorkItemRow key={item.id} item={item} agents={agents} onOpenChat={onOpenChat} />) : <p className="project-calm-state"><CheckCircle size={20} />当前没有需要特别关注的事项</p>}</section>
    </div>
    {repository && <section className="project-repository-card"><span><GitBranch size={19} /></span><div><strong>工作区 Git 主仓</strong><small><code>{repository.default_branch}</code> 为受保护主分支 · 成员在本地分支开发后提交审查</small><code>{repository.clone_url}</code></div><button type="button" onClick={() => void navigator.clipboard?.writeText(repository.clone_url)}>复制地址</button></section>}
  </div>;
}

function WeightLedger({ scope, currentTotal, proposedWeight, currentLabel = "当前已分配" }: { scope: string; currentTotal: number; proposedWeight: number; currentLabel?: string }) {
  const safeWeight = Number.isFinite(proposedWeight) && proposedWeight > 0 ? proposedWeight : 0;
  const nextTotal = currentTotal + safeWeight;
  const difference = Math.abs(100 - nextTotal);
  const balanceState = nextTotal === 100 ? "is-balanced" : nextTotal > 100 ? "is-over" : "is-under";
  const balanceLabel = nextTotal === 100 ? "已分配完成" : nextTotal > 100 ? `超出 ${difference}` : `还可分配 ${difference}`;
  return <div className={`plan-weight-ledger ${balanceState}`} aria-live="polite" aria-label={`${scope}权重实时合计`}>
    <span><small>{currentLabel}</small><strong>{currentTotal}</strong></span>
    <span><small>本次填写</small><strong>+ {safeWeight}</strong></span>
    <span className="plan-weight-next"><small>填写后总计</small><strong>{nextTotal} / 100</strong></span>
    <em>{balanceLabel}</em>
  </div>;
}

function PlanBoard({ plan, currentUser, members, agents, onUpdatePlan, onCreateStep, onUpdateStep, onDeleteStep, onUploadStepWorkspace, onRemoveStepWorkspace, onBindStepLocal, onRemoveStepLocal, onOpenStepLocal }: Pick<ProjectCockpitProps, "plan" | "currentUser" | "members" | "agents" | "onUpdatePlan" | "onCreateStep" | "onUpdateStep" | "onDeleteStep" | "onUploadStepWorkspace" | "onRemoveStepWorkspace" | "onBindStepLocal" | "onRemoveStepLocal" | "onOpenStepLocal">) {
  const [editing, setEditing] = useState(false);
  const [objective, setObjective] = useState(plan?.objective ?? "");
  const [title, setTitle] = useState("");
  const [weight, setWeight] = useState(10);
  const [parent, setParent] = useState("");
  const [editingStepID, setEditingStepID] = useState("");
  const [stepTitle, setStepTitle] = useState("");
  const [stepWeight, setStepWeight] = useState(1);
  const [stepParent, setStepParent] = useState("");
  const [stepUserIDs, setStepUserIDs] = useState<string[]>([]);
  const [stepAgentIDs, setStepAgentIDs] = useState<string[]>([]);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [confirmingWorkspaceID, setConfirmingWorkspaceID] = useState("");
  const [localRunnerID, setLocalRunnerID] = useState("");
  const workspaceInputs = useRef<Record<string, HTMLInputElement | null>>({});
  const steps = plan?.steps ?? [];
  const addPeers = steps.filter((step) => (step.parent_step_id ?? "") === parent && step.status !== "cancelled");
  const addCurrentWeight = addPeers.reduce((total, step) => total + step.weight, 0);
  const addScope = parent ? `${steps.find((step) => step.id === parent)?.title ?? "所选步骤"}的支线` : "主要步骤";
  useEffect(() => {
	let active = true;
	window.agentCollabDesktop?.getState().then((state) => { if (active) setLocalRunnerID(state.runnerId ?? ""); }).catch(() => {});
	return () => { active = false; };
  }, []);
  async function run(key: string, action: () => Promise<void>) { setBusy(key); setError(""); try { await action(); } catch (reason) { setError(reason instanceof Error ? reason.message : "项目计划更新失败"); } finally { setBusy(""); } }
  function startStepEdit(step: ProjectStep) {
    setEditingStepID(step.id);
    setStepTitle(step.title);
    setStepWeight(step.weight);
    setStepParent(step.parent_step_id ?? "");
    setStepUserIDs(step.responsible_user_ids ?? []);
    setStepAgentIDs(step.responsible_agent_ids ?? []);
    setError("");
  }
  function descendantIDs(stepID: string) {
    const found = new Set<string>();
    const visit = (id: string) => steps.filter((item) => item.parent_step_id === id).forEach((child) => { found.add(child.id); visit(child.id); });
    visit(stepID);
    return found;
  }
  async function saveStepEdit(step: ProjectStep) {
    const trimmedTitle = stepTitle.trim();
    if (!trimmedTitle) { setError("步骤名称不能为空"); return; }
    if (!Number.isFinite(stepWeight) || stepWeight < 1 || stepWeight > 1000) { setError("权重必须是 1–1000 之间的数字"); return; }
    await run(`edit-${step.id}`, async () => {
      await onUpdateStep?.(step.id, {
        title: trimmedTitle,
        weight: stepWeight,
        ...(stepParent ? { parent_step_id: stepParent } : { clear_parent: true }),
        responsible_user_ids: stepUserIDs,
        responsible_agent_ids: stepAgentIDs,
      });
      setEditingStepID("");
    });
  }
  async function uploadStepWorkspace(step: ProjectStep, files: FileList | null) {
    const selected = Array.from(files ?? []);
    if (!selected.length) return;
    await run(`workspace-${step.id}`, () => onUploadStepWorkspace?.(step.id, selected) ?? Promise.resolve());
    const input = workspaceInputs.current[step.id];
    if (input) input.value = "";
  }
  async function removeStepWorkspace(step: ProjectStep) {
    await run(`workspace-${step.id}`, () => onRemoveStepWorkspace?.(step.id) ?? Promise.resolve());
    setConfirmingWorkspaceID("");
  }
  function renderStep(step: ProjectStep, depth = 0): ReactNode {
    const children = steps.filter((item) => item.parent_step_id === step.id);
    const isEditing = editingStepID === step.id;
    const invalidParents = descendantIDs(step.id);
    const editPeers = steps.filter((candidate) => candidate.id !== step.id && (candidate.parent_step_id ?? "") === stepParent && candidate.status !== "cancelled");
    const editCurrentWeight = editPeers.reduce((total, candidate) => total + candidate.weight, 0);
    const editScope = stepParent ? `${steps.find((candidate) => candidate.id === stepParent)?.title ?? "所选步骤"}的支线` : "主要步骤";
    const responsibleNames = [...(step.responsible_user_ids ?? []).map((id) => members.find((member) => member.id === id)?.display_name), ...(step.responsible_agent_ids ?? []).map((id) => agents.find((agent) => agent.id === id)?.display_name)].filter(Boolean);
	const canUpload = Boolean(plan?.can_manage || step.responsible_user_ids?.includes(currentUser.id));
	const currentBinding = step.local_bindings?.find((binding) => binding.user_id === currentUser.id && binding.runner_id === localRunnerID);
	const responsibleBindings = step.local_bindings?.filter((binding) => step.responsible_user_ids.includes(binding.user_id)) ?? [];
	const canBindLocal = Boolean(window.agentCollabDesktop?.bindProjectStep && step.responsible_user_ids?.includes(currentUser.id));
	const sharedState = step.workspace ? `团队底稿 v${step.workspace.revision}` : "团队底稿未上传";
	const localState = currentBinding ? "本机目录已绑定" : responsibleBindings.length ? `${responsibleBindings.length} 人已绑定本机目录` : "本机目录未绑定";
    return <div key={step.id} className="plan-step-wrap">
      {isEditing ? <article className="plan-step-edit-card" style={{ marginLeft: `${depth * 22}px` }}>
        <label><span>步骤名称</span><input value={stepTitle} onChange={(event) => setStepTitle(event.target.value)} aria-label={`${step.title} 新名称`} /></label>
        <label><span>层级 / 归属</span><select value={stepParent} onChange={(event) => setStepParent(event.target.value)} aria-label={`${step.title} 新归属`}><option value="">主要步骤</option>{steps.filter((candidate) => candidate.id !== step.id && !invalidParents.has(candidate.id)).map((candidate) => <option key={candidate.id} value={candidate.id}>{candidate.title} 下的支线</option>)}</select></label>
        <label className="plan-weight-field"><span>权重值</span><input type="number" min="1" max="1000" value={stepWeight} onChange={(event) => setStepWeight(Number(event.target.value))} aria-label={`${step.title} 新权重`} /></label>
        <WeightLedger scope={editScope} currentTotal={editCurrentWeight} proposedWeight={stepWeight} currentLabel="同层其他步骤" />
        <fieldset className="plan-responsible-picker"><legend>负责人（可多选）</legend><div><strong>真人</strong>{members.length ? members.map((member) => <label key={member.id}><input type="checkbox" checked={stepUserIDs.includes(member.id)} onChange={() => setStepUserIDs((current) => current.includes(member.id) ? current.filter((id) => id !== member.id) : [...current, member.id])} /><span>{member.display_name}</span><small>@{member.username}</small></label>) : <p>工作区还没有真人成员</p>}</div><div><strong>Agent</strong>{agents.length ? agents.map((agent) => <label key={agent.id}><input type="checkbox" checked={stepAgentIDs.includes(agent.id)} onChange={() => setStepAgentIDs((current) => current.includes(agent.id) ? current.filter((id) => id !== agent.id) : [...current, agent.id])} /><span>{agent.display_name}</span><small>{agent.type}</small></label>) : <p>工作区还没有 Agent</p>}</div></fieldset>
        <div className="plan-step-edit-actions"><button type="button" className="is-primary" disabled={Boolean(busy)} onClick={() => void saveStepEdit(step)}><Check size={15} />保存</button><button type="button" disabled={Boolean(busy)} onClick={() => setEditingStepID("")}><X size={15} />取消</button></div>
      </article> : <details className="plan-step-card" style={{ marginLeft: `${depth * 22}px` }}>
        <summary className="plan-step-heading" aria-label={`查看步骤详情 ${step.title}`}>
          <span className="plan-step-icon"><GitBranch size={17} /></span>
          <span className="plan-step-copy">
            <strong>{step.title}</strong>
            <small className="plan-step-meta">权重 {step.weight} / 100 · {step.linked_item_count} 个任务会话 · 负责人：{responsibleNames.length ? responsibleNames.join("、") : "未指定"}</small>
            <span className="plan-step-readiness"><small className={step.workspace ? "is-ready" : ""}><FileText size={13} />{sharedState}</small><small className={currentBinding || responsibleBindings.length ? "is-ready" : ""}><FolderOpen size={13} />{localState}</small></span>
          </span>
          <span className="plan-step-progress"><small>进度</small><strong>{step.progress}%</strong></span>
          <CaretDown className="plan-step-caret" size={17} aria-hidden="true" />
        </summary>
        <div className="plan-step-detail">
          <p className="plan-step-workflow-hint">平时在本机目录工作；需要让全组依据同一版本讨论时，再更新团队底稿。两边不会自动互相覆盖。</p>
          <div className="plan-step-workspaces">
          <div className="step-shared-workspace">
            <span className="step-workspace-label">团队底稿 <small>Server 上的共享里程碑版本</small></span>
            {step.workspace ? <span className="step-workspace-state"><FolderOpen size={14} /><strong>{step.workspace.source_name || "步骤工作区"}</strong><small>{step.workspace.file_count} 个文件 · 版本 {step.workspace.revision} · {new Date(step.workspace.updated_at).toLocaleString()}</small></span> : <span className="step-workspace-state is-empty"><FolderOpen size={14} /><small>尚未上传团队底稿</small></span>}
			{canUpload && <input ref={(node) => { workspaceInputs.current[step.id] = node; }} className="sr-only" type="file" multiple {...({ webkitdirectory: "" } as Record<string, string>)} onChange={(event) => void uploadStepWorkspace(step, event.target.files)} />}
			<div className="step-workspace-actions">{canUpload && <button type="button" className="step-workspace-button" disabled={Boolean(busy) || !onUploadStepWorkspace} onClick={() => workspaceInputs.current[step.id]?.click()} title="管理员或步骤负责人可直接发布团队版本；会立即增加版本号，不经过任务审查">{busy === `workspace-${step.id}` ? <SpinnerGap className="is-spinning" size={15} /> : <UploadSimple size={15} />}<span>{step.workspace ? "更新底稿" : "上传底稿"}</span></button>}{step.workspace && <a className="step-workspace-download" href={`/api/v1/project-steps/${encodeURIComponent(step.id)}/workspace`} download title="所有工作区成员均可下载的只读团队底稿"><DownloadSimple size={15} /><span>下载只读副本</span></a>}{!canUpload && !step.workspace && <span className="step-workspace-waiting">等待负责人上传</span>}{step.workspace && canUpload && onRemoveStepWorkspace && <button type="button" className="step-workspace-unlink" disabled={Boolean(busy)} onClick={() => setConfirmingWorkspaceID(step.id)} title="取消当前绑定，Server 仍保留历史版本用于审计"><X size={15} /><span>解除</span></button>}</div>
          </div>
          <div className={`step-local-binding${currentBinding ? " is-bound" : ""}`}>
            <span><FolderOpen size={15} /><span><strong>我的本机目录 <small>仅留在这台电脑</small></strong><small>{currentBinding ? `${currentBinding.directory_label} · ${currentBinding.runner_status === "online" ? "可查询最新进展" : "Client 离线，任务会等待"}` : canBindLocal ? "绑定一次后，Agent 可在原目录汇报最新进展" : responsibleBindings.length ? `${responsibleBindings.length} 个负责人设备已绑定` : "尚无负责人绑定"}</small></span></span>
            {canBindLocal && (currentBinding ? <div><button type="button" disabled={Boolean(busy)} onClick={() => void run(`local-open-${step.id}`, () => onOpenStepLocal?.(step.id) ?? Promise.resolve())}>打开</button><button type="button" disabled={Boolean(busy)} onClick={() => void run(`local-${step.id}`, () => onBindStepLocal?.(step.id) ?? Promise.resolve())}>更换</button><button type="button" disabled={Boolean(busy)} onClick={() => void run(`local-${step.id}`, () => onRemoveStepLocal?.(step.id) ?? Promise.resolve())}>解除</button></div> : <button type="button" disabled={Boolean(busy)} onClick={() => void run(`local-${step.id}`, () => onBindStepLocal?.(step.id) ?? Promise.resolve())}>绑定目录</button>)}
          </div>
        </div>
		{confirmingWorkspaceID === step.id && <div className="step-workspace-confirm" role="alert"><span><strong>解除这个步骤的主工作区？</strong><small>看板将恢复为“还没有主工作区”。已经产生的历史版本和旧任务记录仍留在 Server，不会删除你电脑上的原目录。</small></span><button type="button" disabled={Boolean(busy)} onClick={() => void removeStepWorkspace(step)}>确认解除</button><button type="button" disabled={Boolean(busy)} onClick={() => setConfirmingWorkspaceID("")}>取消</button></div>}
        {plan?.can_manage && <div className="plan-step-management"><label><span>步骤状态</span><select aria-label={`${step.title} 步骤状态`} value={step.status} disabled={Boolean(busy)} onChange={(event) => void run(step.id, () => onUpdateStep?.(step.id, { status: event.target.value as ProjectStepStatus }) ?? Promise.resolve())}><option value="planned">待开始</option><option value="in_progress">进行中</option><option value="review">待验收</option><option value="done">已完成</option><option value="cancelled">已作废</option></select></label><span className="plan-step-management-actions"><button type="button" disabled={Boolean(busy)} onClick={() => startStepEdit(step)} aria-label={`编辑步骤 ${step.title}`} title="修改名称、层级、权重和负责人"><PencilSimple size={15} />编辑</button><button type="button" className="is-danger" disabled={Boolean(busy)} onClick={() => window.confirm(`删除步骤“${step.title}”？关联会话会保留但不再归属。`) && void run(step.id, () => onDeleteStep?.(step.id) ?? Promise.resolve())} aria-label={`删除步骤 ${step.title}`}><Trash size={15} />删除</button></span></div>}
        </div>
      </details>}
      {children.map((child) => renderStep(child, depth + 1))}
    </div>;
  }
	return <div className="plan-board">
	  <details className="project-plain-guide">
		<summary><span><GitBranch size={18} /><strong>第一次使用？点这里看整个项目怎么协作</strong></span><small>不需要学习 Git 命令</small></summary>
		<div className="project-plain-guide-body">
		  <ol>
			<li><span>1</span><div><strong>管理员先拆步骤、定负责人</strong><small>这里只是把大项目分成几块，真人和 Agent 都可以负责。</small></div></li>
			<li><span>2</span><div><strong>负责人绑定自己的本机工作目录</strong><small>目录仍留在负责人电脑；以后点名他的负责 Agent，可直接询问尚未提交的最新进展。Server 只知道目录名称，不保存绝对路径。</small></div></li>
			<li><span>3</span><div><strong>群内成员和 Agent 默认只读团队底稿</strong><small>所有人都能依据同一版本讨论；聊天任务使用临时副本，不能直接覆盖 Server 团队底稿。要问尚未上传的进展，再查询负责人本机。</small></div></li>
			<li><span>4</span><div><strong>做完后提交自己的目录</strong><small>在对应任务会话点“选择目录并提交审查”。平台把这次修改单独保存，不会马上覆盖团队底稿。</small></div></li>
			<li><span>5</span><div><strong>AI 帮忙审查，管理员决定合并</strong><small>通过后才把修改并入团队底稿并增加版本；不通过就退回继续修改。</small></div></li>
		  </ol>
		  <p><strong>Git 在这里是什么？</strong>就是 Server 后台的“版本保险箱”。你只需要上传、提交审查、确认合并，分支、版本记录和合并都由平台处理。</p>
		</div>
	  </details>
	  <section className="project-panel plan-definition"><header><div><strong>项目计划</strong><small>只有已验收步骤按权重计入总体进度；聊天和修 Bug 不会自动推进。</small></div>{plan?.can_manage && <button type="button" onClick={() => setEditing((value) => !value)}>{editing ? "收起编辑" : "编辑计划"}</button>}</header>
      <div className="plan-storage-legend"><span><FileText size={16} /><span><strong>团队底稿</strong><small>Server 保存的统一版本，全组可读</small></span></span><span><FolderOpen size={16} /><span><strong>本机目录</strong><small>负责人电脑上的实时工作内容，不会自动上传</small></span></span><em>展开某一步骤后管理文件与目录</em></div>
      {editing && <div className="plan-editor"><label>项目总目标<textarea value={objective} onChange={(event) => setObjective(event.target.value)} /></label><button type="button" disabled={Boolean(busy)} onClick={() => void run("plan", () => onUpdatePlan?.({ objective, status: "active" }) ?? Promise.resolve())}>保存目标</button><div className="plan-add-step"><label><span>步骤名称</span><input value={title} onChange={(event) => setTitle(event.target.value)} placeholder="新增主要步骤或支线任务" /></label><label><span>层级 / 归属</span><select value={parent} onChange={(event) => setParent(event.target.value)}><option value="">主要步骤</option>{steps.map((step) => <option key={step.id} value={step.id}>{step.title} 下的支线</option>)}</select></label><label className="plan-weight-field"><span>权重值</span><input type="number" min="1" max="1000" value={weight} onChange={(event) => setWeight(Number(event.target.value))} aria-label="步骤权重值" /></label><button type="button" disabled={!title.trim() || Boolean(busy)} onClick={() => void run("create", async () => { await onCreateStep?.({ parent_step_id: parent || undefined, title: title.trim(), weight }); setTitle(""); })}><Plus size={15} />新增</button></div><WeightLedger scope={addScope} currentTotal={addCurrentWeight} proposedWeight={weight} /><small className="plan-weight-hint">同一层的权重目标合计为 100。上方只显示现有权重、本次填写和保存后的总计。</small></div>}
      <div className="plan-step-list">{steps.filter((step) => !step.parent_step_id).map((step) => renderStep(step))}{steps.length === 0 && <p>先设置总目标，再添加主要步骤及各自权重。</p>}</div>
    </section>{error && <p className="project-board-error" role="alert">{error}</p>}
  </div>;
}

function Board({ plan, items, agents, roomAgents, onUpdateStatus, onReviewWorkItem, onMergeWorkItem, onStartAIReview, onRejectWorkItem, onSubmitWorkItemWorkspace, onOpenChat }: Pick<ProjectCockpitProps, "plan" | "items" | "agents" | "roomAgents" | "onUpdateStatus" | "onReviewWorkItem" | "onMergeWorkItem" | "onStartAIReview" | "onRejectWorkItem" | "onSubmitWorkItemWorkspace" | "onOpenChat">) {
  const [updating, setUpdating] = useState("");
  const [error, setError] = useState("");
  const [reviewAgentByItem, setReviewAgentByItem] = useState<Record<string, string>>({});
  const [verificationAgentByItem, setVerificationAgentByItem] = useState<Record<string, string>>({});
  const [reviewingItemID, setReviewingItemID] = useState("");
  const [review, setReview] = useState<WorkItemReview>();
  const submitInputs = useRef<Record<string, HTMLInputElement | null>>({});
  if (items.length === 0) return <div className="project-board-empty"><GitBranch size={21} /><div><strong>还没有纳入计划的 Issue</strong><small>发送会话或创建任务时选择项目步骤，它才会出现在这里；普通聊天保持在聊天时间线中。</small></div><button type="button" onClick={() => onOpenChat()}>发起会话</button></div>;
  async function update(itemID: string, status: WorkItemStatus) {
    setUpdating(itemID);
    setError("");
    try { await onUpdateStatus(itemID, status); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "无法更新事项状态"); }
    finally { setUpdating(""); }
  }
  async function merge(itemID: string) {
    setUpdating(itemID); setError("");
    try { await onMergeWorkItem?.(itemID); setReview(undefined); setReviewingItemID(""); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "无法合并功能分支"); }
    finally { setUpdating(""); }
  }
  async function inspect(itemID: string) {
    setUpdating(itemID); setError("");
    try { setReview(await onReviewWorkItem?.(itemID)); setReviewingItemID(itemID); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "无法读取功能分支改动"); }
    finally { setUpdating(""); }
  }
  async function startAIReview(itemID: string) {
    const available = (roomAgents ?? agents).filter((agent) => agent.status !== "unavailable" && agent.can_invoke !== false);
    const reviewer = reviewAgentByItem[itemID] || available[0]?.id;
    const verifier = verificationAgentByItem[itemID] || available[1]?.id || reviewer;
    if (!reviewer || !verifier) { setError("请先在本群加入至少一个可调用 Agent"); return; }
    setUpdating(itemID); setError("");
    try { await onStartAIReview?.(itemID, reviewer, verifier); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "无法发起 AI 审查"); }
    finally { setUpdating(""); }
  }
  async function reject(itemID: string, summary?: string) {
    setUpdating(itemID); setError("");
    try { await onRejectWorkItem?.(itemID, summary || "根据 AI 审查结论退回修改"); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "无法退回修改"); }
    finally { setUpdating(""); }
  }
  async function submit(itemID: string, files: FileList | null) {
    const selected = Array.from(files ?? []);
    if (!selected.length) return;
    setUpdating(itemID); setError("");
    try { await onSubmitWorkItemWorkspace?.(itemID, selected); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "无法提交任务工作区"); }
    finally { setUpdating(""); const input = submitInputs.current[itemID]; if (input) input.value = ""; }
  }
  return <><div className="project-board" aria-label="工作事项看板">
    {boardColumns.map((column) => {
      const columnItems = items.filter((item) => item.status === column.status);
      return <section className="project-board-column" key={column.status}>
        <header><div><strong>{column.title}</strong><span>{columnItems.length}</span></div><small>{column.hint}</small></header>
        <div className="project-board-stack">
          {columnItems.map((item) => {
            const targetNames = item.target_agent_ids.map((id) => agents.find((agent) => agent.id === id)?.display_name).filter(Boolean);
            const stepTitle = plan?.steps.find((step) => step.id === item.plan_step_id)?.title;
            return <article className={`project-board-card priority-${item.priority}`} key={item.id}>
              <button type="button" className="project-board-card-title" onClick={() => onOpenChat(item.root_message_id)}>{item.title}</button>
              <p>{targetNames.length ? targetNames.join("、") : "真人事项"}</p>
              <div><span>{item.delivered_task_count}/{item.task_count} 已交付</span>{item.failed_task_count > 0 && <em><WarningCircle size={14} /> {item.failed_task_count} 失败</em>}</div>
              <WorkLinkChain item={item} stepTitle={stepTitle} onOpenChat={onOpenChat} />
              {item.plan_step_id && item.merge_status !== "merged" && <div className="project-git-branch"><GitBranch size={14} /><span><strong>{item.git_branch ? "任务修改" : "文件工作区"}</strong>{item.git_branch ? <small>由系统管理版本</small> : <small>需要修改文件时再创建</small>}</span>{item.status !== "review" && <button type="button" disabled={updating === item.id || !onSubmitWorkItemWorkspace} onClick={() => submitInputs.current[item.id]?.click()}>{updating === item.id ? "提交中" : item.git_branch ? "更新提交" : "选择目录并提交"}</button>}<input ref={(node) => { submitInputs.current[item.id] = node; }} className="sr-only" type="file" multiple {...({ webkitdirectory: "" } as Record<string, string>)} onChange={(event) => void submit(item.id, event.target.files)} /></div>}
              {item.status === "review" && item.merge_status === "open" && (!item.ai_review_status || item.ai_review_status === "none") && <section className="ai-review-card"><header><Robot size={18} /><span><strong>请 AI 先审查</strong><small>Agent 解释改动、运行验证，管理员只做最终决定</small></span></header><label>主审 Agent<select value={reviewAgentByItem[item.id] ?? ""} onChange={(event) => setReviewAgentByItem((current) => ({ ...current, [item.id]: event.target.value }))}><option value="">自动选择</option>{(roomAgents ?? agents).filter((candidate) => candidate.status !== "unavailable").map((candidate) => <option key={candidate.id} value={candidate.id}>{candidate.display_name} · {candidate.status === "online" ? "在线" : "离线等待"}</option>)}</select></label><label>合并后校验<select value={verificationAgentByItem[item.id] ?? ""} onChange={(event) => setVerificationAgentByItem((current) => ({ ...current, [item.id]: event.target.value }))}><option value="">默认由另一 Agent 校验</option>{(roomAgents ?? agents).filter((candidate) => candidate.status !== "unavailable").map((candidate) => <option key={candidate.id} value={candidate.id}>{candidate.display_name}</option>)}</select></label><button type="button" disabled={updating === item.id || !onStartAIReview} onClick={() => void startAIReview(item.id)}>{updating === item.id ? "正在委托" : "发起 AI 审查"}</button></section>}
              {item.ai_review_status === "pending" && <section className="ai-review-card is-pending"><header><SpinnerGap className="is-spinning" size={18} /><span><strong>AI 正在审查</strong><small>正在检查待合并项目并运行适用验证</small></span></header></section>}
              {(item.ai_review_status === "passed" || item.ai_review_status === "changes_requested" || item.ai_review_status === "failed") && <section className={`ai-review-card status-${item.ai_review_status}`}><header>{item.ai_review_status === "passed" ? <CheckCircle size={19} weight="fill" /> : <WarningCircle size={19} weight="fill" />}<span><strong>{item.ai_review_status === "passed" ? "AI 建议合并" : item.ai_review_status === "changes_requested" ? "AI 建议修改后再合并" : "AI 审查未完成"}</strong><small>以下是面向管理员的通俗结论</small></span></header><pre>{item.ai_review_summary || "Agent 没有返回审查说明"}</pre><div className="ai-review-actions">{item.ai_review_status === "passed" && <button type="button" disabled={updating === item.id} onClick={() => void merge(item.id)}>按 AI 建议合并</button>}<button type="button" className="is-secondary" disabled={updating === item.id || !onRejectWorkItem} onClick={() => void reject(item.id, item.ai_review_summary)}>退回修改</button><button type="button" className="is-quiet" disabled={updating === item.id || !onReviewWorkItem} onClick={() => void inspect(item.id)}>技术详情</button></div></section>}
              {item.verification_status === "pending" && <section className="ai-review-card is-pending"><header><SpinnerGap className="is-spinning" size={18} /><span><strong>已合并，正在独立校验</strong><small>校验通过后才会正式完成</small></span></header></section>}
              {(item.verification_status === "passed" || item.verification_status === "failed") && <section className={`ai-review-card status-${item.verification_status}`}><header>{item.verification_status === "passed" ? <CheckCircle size={19} weight="fill" /> : <WarningCircle size={19} weight="fill" />}<span><strong>{item.verification_status === "passed" ? "合并后校验通过" : "合并后校验异常"}</strong><small>{item.verification_status === "passed" ? "事项已经正式完成" : "主项目已合并，但需要管理员安排修复"}</small></span></header><pre>{item.verification_summary}</pre></section>}
              {reviewingItemID === item.id && review && <WorkItemReviewPanel review={review} merging={updating === item.id} onClose={() => { setReview(undefined); setReviewingItemID(""); }} />}
              {item.status === "review" && !item.git_branch && item.delivered_task_count > 0 && <section className="ai-review-card status-passed"><header><CheckCircle size={19} weight="duotone" /><span><strong>文字与文件交付待验收</strong><small>查看原话题中的完整答复和附件后，由管理员确认</small></span></header><div className="ai-review-actions"><button type="button" disabled={updating === item.id} onClick={() => void update(item.id, "done")}>验收通过</button><button type="button" className="is-secondary" disabled={updating === item.id} onClick={() => void update(item.id, "in_progress")}>退回修改</button><button type="button" className="is-quiet" onClick={() => onOpenChat(item.root_message_id)}>查看交付</button></div></section>}
              {item.merge_status === "merged" && <div className="project-git-branch"><Check size={14} /><span><strong>已合并</strong><code>{item.git_branch}</code></span></div>}
              <label><span className="sr-only">更新事项状态</span><select aria-label={`${item.title} 状态`} value={item.status} disabled={updating === item.id} onChange={(event) => void update(item.id, event.target.value as WorkItemStatus)}>{boardColumns.map((option) => <option key={option.status} value={option.status} disabled={option.status === "done" && item.merge_status === "open"}>{option.status === "done" && item.merge_status === "open" ? "合并后完成" : option.title}</option>)}<option value="cancelled">已取消</option></select></label>
            </article>;
          })}
          {columnItems.length === 0 && <p className="project-column-empty">暂无事项</p>}
        </div>
      </section>;
    })}
  </div>{error && <p className="project-board-error" role="alert">{error}</p>}</>;
}

function Artifacts({ messages, items, agents, currentUser, plan, onUpdateStatus, onOpenChat }: Pick<ProjectCockpitProps, "messages" | "items" | "agents" | "currentUser" | "plan" | "onUpdateStatus" | "onOpenChat">) {
  const attachments = useMemo(() => messages.flatMap((message) => (message.attachments ?? []).map((attachment) => ({ attachment, message }))), [messages]);
  const deliveries = formalDeliveryItems(items, messages);
  const [updating, setUpdating] = useState("");
  const [error, setError] = useState("");
  const legacyHubAdmin = !currentUser.workspace_role && currentUser.role === "admin";
  const canAccept = Boolean(plan?.can_manage || currentUser.workspace_role === "owner" || currentUser.workspace_role === "admin" || legacyHubAdmin);

  async function update(item: WorkItem, status: "in_progress" | "done") {
    setUpdating(item.id);
    setError("");
    try { await onUpdateStatus(item.id, status); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "无法更新验收状态"); }
    finally { setUpdating(""); }
  }

  if (attachments.length === 0 && deliveries.length === 0) return <div className="project-empty"><span><FileText size={24} weight="duotone" /></span><h3>还没有汇总产物</h3><p>Agent 的可验收交付与会话中的临时附件会集中显示在这里；长期共同参考资料请放入知识库。</p><button type="button" onClick={() => onOpenChat()}>返回聊天</button></div>;
  return <div className="project-artifacts">
    {attachments.length > 0 && <section className="project-panel"><header><div><strong>会话临时附件</strong><small>{attachments.length} 个文件 · 不等于正式交付</small></div></header><div className="artifact-grid">{attachments.map(({ attachment }) => <a key={attachment.id} href={attachment.download_url ?? `/api/v1/rooms/${encodeURIComponent(attachment.room_id)}/attachments/${encodeURIComponent(attachment.id)}`}><FileText size={20} /><span><strong>{attachment.filename}</strong><small>{Math.max(1, Math.round(attachment.size_bytes / 1024))} KB</small></span></a>)}</div></section>}
    <section className="project-panel formal-delivery-panel"><header><div><strong>正式交付</strong><small>只显示项目步骤任务和明确创建的任务；普通 @ 对话仍留在聊天中</small></div></header>
      {deliveries.length > 0 ? deliveries.map((item) => {
        const names = item.target_agent_ids.map((id) => agents.find((agent) => agent.id === id)?.display_name).filter(Boolean);
        const stepTitle = plan?.steps.find((step) => step.id === item.plan_step_id)?.title;
        const textDeliveryReady = item.status === "review" && !item.git_branch && item.delivered_task_count > 0;
        return <article className="formal-delivery-row" key={item.id}>
          <div className="formal-delivery-main">
            <span className={`project-status-dot status-${item.status}`} />
            <span><strong>{item.title}</strong><small>{stepTitle ? `归属：${stepTitle} · ` : ""}{names.length ? names.join("、") : "真人事项"} · {item.delivered_task_count}/{item.task_count} 已交付</small></span>
            <em className={`project-status-label status-${item.status}`}>{statusLabels[item.status]}</em>
          </div>
          <div className="formal-delivery-actions">
            {textDeliveryReady && canAccept && <><button type="button" className="is-primary" disabled={updating === item.id} onClick={() => void update(item, "done")}><CheckCircle size={16} />验收通过</button><button type="button" disabled={updating === item.id} onClick={() => void update(item, "in_progress")}><X size={16} />退回修改</button></>}
            {textDeliveryReady && !canAccept && <span>等待工作区管理员验收</span>}
            {item.status === "review" && Boolean(item.git_branch) && <span>文件修改任务请到看板完成 AI 审查、合并与校验</span>}
            {item.status === "done" && <span className="is-accepted"><CheckCircle size={14} />已验收</span>}
            {item.status === "cancelled" && <span>事项已取消，交付记录仅供审计</span>}
            {item.status !== "review" && item.status !== "done" && item.status !== "cancelled" && <span>{item.active_task_count > 0 ? "Agent 仍在执行" : "已有部分交付，等待其余执行结束"}</span>}
            <button type="button" className="is-quiet" onClick={() => onOpenChat(item.root_message_id)}><ChatCircleText size={16} />查看原话题</button>
          </div>
        </article>;
      }) : <p className="project-calm-state"><CheckCircle size={20} />当前没有正式交付；普通点名回复不会出现在这里</p>}
      {error && <p className="project-board-error" role="alert">{error}</p>}
    </section>
  </div>;
}

function formatBytes(value: number) {
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${Math.max(1, Math.round(value / 1024))} KB`;
  return `${(value / 1024 / 1024).toFixed(1)} MB`;
}

function KnowledgeSection({
  scope, title, hint, collection, busy, onUpload, onDelete,
}: {
  scope: "workspace" | "room";
  title: string;
  hint: string;
  collection: KnowledgeCollection;
  busy: boolean;
  onUpload?: (scope: "workspace" | "room", file: File) => Promise<void>;
  onDelete?: (scope: "workspace" | "room", assetID: string) => Promise<void>;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [working, setWorking] = useState("");
  const [error, setError] = useState("");

  async function upload(file?: File) {
    if (!file || !onUpload) return;
    setWorking("upload");
    setError("");
    try { await onUpload(scope, file); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "上传失败"); }
    finally {
      setWorking("");
      if (inputRef.current) inputRef.current.value = "";
    }
  }

  async function remove(asset: KnowledgeAsset) {
    if (!onDelete || !window.confirm(`从知识库删除“${asset.filename}”？`)) return;
    setWorking(asset.id);
    setError("");
    try { await onDelete(scope, asset.id); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "删除失败"); }
    finally { setWorking(""); }
  }

  return <section className="knowledge-scope project-panel">
    <header>
      <div><strong>{title}</strong><small>{hint} · {collection.assets.length} 个文件</small></div>
      <button type="button" className="knowledge-upload-button" disabled={busy || Boolean(working) || !onUpload} onClick={() => inputRef.current?.click()}>{working === "upload" ? <SpinnerGap className="is-spinning" size={16} /> : <UploadSimple size={16} />}上传资料</button>
      <input ref={inputRef} className="sr-only" type="file" accept=".pdf,.doc,.docx,.xls,.xlsx,.csv,.txt,.md,.ppt,.pptx,.png,.jpg,.jpeg,.webp" onChange={(event) => void upload(event.target.files?.[0])} />
    </header>
    {collection.assets.length ? <div className="knowledge-file-list">{collection.assets.map((asset) => <article key={asset.id}>
      <a href={asset.download_url}><span className="knowledge-file-icon"><FileText size={19} /></span><span><strong>{asset.filename}</strong><small>{formatBytes(asset.size_bytes)} · {asset.uploader_name || "群成员"} · {new Date(asset.created_at).toLocaleDateString()}</small></span></a>
      {asset.can_delete && <button type="button" disabled={working === asset.id} onClick={() => void remove(asset)} aria-label={`删除 ${asset.filename}`} title="删除资料">{working === asset.id ? <SpinnerGap className="is-spinning" size={16} /> : <Trash size={16} />}</button>}
    </article>)}</div> : <div className="knowledge-empty"><BookOpenText size={22} /><div><strong>还没有{title}</strong><small>上传后，群成员和本群 Agent 可以在任务中共同引用。</small></div></div>}
    {error && <p className="knowledge-error" role="alert">{error}</p>}
    {collection.directory && <details className="knowledge-directory"><summary><FolderOpen size={15} />本地存储位置</summary><code>{collection.directory}</code><small>这是 Server 存储根目录下的相对路径，工作区或群聊改名不会使资料失联。</small></details>}
  </section>;
}

function Knowledge(props: Pick<ProjectCockpitProps, "plan" | "items" | "workspaceKnowledge" | "roomKnowledge" | "knowledgeLoading" | "knowledgeError" | "onUploadKnowledge" | "onDeleteKnowledge">) {
  const workspaceCollection = props.workspaceKnowledge ?? { assets: [], directory: "" };
  const roomCollection = props.roomKnowledge ?? { assets: [], directory: "" };
  const activeSteps = props.plan?.steps.filter((step) => step.status === "in_progress").length ?? 0;
  const reviewItems = props.items.filter((item) => item.status === "review").length;
  const mergedItems = props.items.filter((item) => item.merge_status === "merged").length;
  return <div className="project-knowledge">
    <header className="knowledge-intro"><span><BookOpenText size={22} weight="duotone" /></span><div><strong>项目知识库</strong><small>规范、历史材料与参考样例存放在真实目录中；发起任务时，本群 Agent 会自动收到工作区共享和本群资料。</small></div></header>
    <section className="project-memory-snapshot" aria-label="项目共享记忆快照">
      <header><span><Robot size={18} weight="duotone" /><span><strong>共享记忆快照</strong><small>Hub 从正式计划和事项状态生成，Agent 接手任务时自动取得，不需要重新讲述项目背景。</small></span></span><em>自动随任务更新</em></header>
      <div><article><small>项目目标</small><strong>{props.plan?.objective || "尚未设置"}</strong></article><article><small>当前推进</small><strong>{activeSteps} 个步骤进行中</strong></article><article><small>等待决定</small><strong>{reviewItems} 项待审查或验收</strong></article><article><small>正式沉淀</small><strong>{mergedItems} 项已并入团队版本</strong></article></div>
      <footer><span>工作区共享：{workspaceCollection.assets.length} 份</span><span>本群资料：{roomCollection.assets.length} 份</span><span>步骤记忆：目标、说明、状态与正式版本</span></footer>
    </section>
    {props.knowledgeError && <p className="knowledge-page-error" role="alert">{props.knowledgeError}</p>}
    <div className="knowledge-columns">
      <KnowledgeSection scope="room" title="本群资料" hint="只服务当前群聊" collection={roomCollection} busy={Boolean(props.knowledgeLoading)} onUpload={props.onUploadKnowledge} onDelete={props.onDeleteKnowledge} />
      <KnowledgeSection scope="workspace" title="工作区共享" hint="同一工作区的群聊都可引用" collection={workspaceCollection} busy={Boolean(props.knowledgeLoading)} onUpload={props.onUploadKnowledge} onDelete={props.onDeleteKnowledge} />
    </div>
  </div>;
}

export function ProjectCockpit(props: ProjectCockpitProps) {
  const { t } = useLocale();
  const [boardMode, setBoardMode] = useState<"office" | "board" | "timeline">(props.defaultBoardMode ?? "board");
  const projectItems = formalProjectItems(props.items, props.messages);
  const projectTasks = formalProjectTasks(props.tasks ?? [], props.items, props.messages);
  if (props.view === "overview") return <Overview plan={props.plan} repository={props.repository} items={projectItems} members={props.members} agents={props.agents} onOpenChat={props.onOpenChat} />;
  if (props.view === "board") return <div className="project-collaboration-console">
    <header className="collaboration-console-header">
      <div><strong>{t("协作台")}</strong><small>{t("总办公室集中理解和调度，结构看板精确管理，执行时间线负责追踪与审计。")}</small></div>
      <nav aria-label={t("协作台视图")}>
        <button type="button" className={boardMode === "office" ? "is-active" : ""} onClick={() => setBoardMode("office")}><Buildings size={16} />{t("总办公室")}</button>
        <button type="button" className={boardMode === "board" ? "is-active" : ""} onClick={() => setBoardMode("board")}><ListChecks size={16} />{t("结构看板")}</button>
        <button type="button" className={boardMode === "timeline" ? "is-active" : ""} onClick={() => setBoardMode("timeline")}><ClockCountdown size={16} />{t("执行时间线")}</button>
      </nav>
    </header>
    {boardMode === "office" && <ProjectOffice plan={props.plan} items={projectItems} tasks={projectTasks} members={props.members} agents={props.agents} onUpdateStatus={props.onUpdateStatus} onOpenChat={props.onOpenChat} onOpenStructureBoard={() => setBoardMode("board")} />}
    {boardMode === "board" && <><PlanBoard plan={props.plan} currentUser={props.currentUser} members={props.members} agents={props.agents} onUpdatePlan={props.onUpdatePlan} onCreateStep={props.onCreateStep} onUpdateStep={props.onUpdateStep} onDeleteStep={props.onDeleteStep} onUploadStepWorkspace={props.onUploadStepWorkspace} onRemoveStepWorkspace={props.onRemoveStepWorkspace} onBindStepLocal={props.onBindStepLocal} onRemoveStepLocal={props.onRemoveStepLocal} onOpenStepLocal={props.onOpenStepLocal} /><Board plan={props.plan} items={projectItems.filter((item) => Boolean(item.plan_step_id))} agents={props.agents} roomAgents={props.roomAgents} onUpdateStatus={props.onUpdateStatus} onReviewWorkItem={props.onReviewWorkItem} onMergeWorkItem={props.onMergeWorkItem} onStartAIReview={props.onStartAIReview} onRejectWorkItem={props.onRejectWorkItem} onSubmitWorkItemWorkspace={props.onSubmitWorkItemWorkspace} onOpenChat={props.onOpenChat} /></>}
    {boardMode === "timeline" && <ProjectTimeline plan={props.plan} items={projectItems} tasks={projectTasks} agents={props.agents} onOpenChat={props.onOpenChat} />}
  </div>;
  if (props.view === "knowledge") return <Knowledge plan={props.plan} items={projectItems} workspaceKnowledge={props.workspaceKnowledge} roomKnowledge={props.roomKnowledge} knowledgeLoading={props.knowledgeLoading} knowledgeError={props.knowledgeError} onUploadKnowledge={props.onUploadKnowledge} onDeleteKnowledge={props.onDeleteKnowledge} />;
  return <Artifacts messages={props.messages} items={props.items} agents={props.agents} currentUser={props.currentUser} plan={props.plan} onUpdateStatus={props.onUpdateStatus} onOpenChat={props.onOpenChat} />;
}
