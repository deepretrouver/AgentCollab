import { useMemo, useState } from "react";
import {
  Archive,
  ArrowRight,
  BookOpenText,
  CheckCircle,
  ClockCountdown,
  Desktop,
  FileText,
  FolderOpen,
  GitBranch,
  GitMerge,
  Pulse,
  Robot,
  User,
  UsersThree,
  WarningCircle,
} from "@phosphor-icons/react";
import type { Agent, ProjectPlan, ProjectStep, Task, User as Collaborator, WorkItem, WorkItemStatus } from "../api/client";
import { useLocale } from "../i18n/locale";

interface ProjectOfficeProps {
  plan?: ProjectPlan;
  items: WorkItem[];
  members: Collaborator[];
  agents: Agent[];
  tasks?: Task[];
  onOpenChat: (rootMessageID?: string) => void;
  onUpdateStatus: (itemID: string, status: WorkItemStatus) => Promise<void>;
  onOpenStructureBoard?: () => void;
}

const statusLabels: Record<WorkItemStatus, string> = {
  backlog: "待整理",
  ready: "待开始",
  in_progress: "执行中",
  review: "待验收",
  done: "已完成",
  cancelled: "已取消",
};

function stageFor(item: WorkItem, t: (text: string) => string = text => text) {
  if (item.verification_status === "failed") return t("合并后校验未通过");
  if (item.verification_status === "passed") return t("合并校验通过");
  if (item.verification_status === "pending") return t("合并后校验中");
  if (item.merge_status === "conflict") return t("合并冲突");
  if (item.merge_status === "merged") return t("已并入团队底稿");
  if (item.ai_review_status === "passed") return t("审查通过，待合并");
  if (item.ai_review_status === "pending") return t("AI 审查中");
  if (item.failed_task_count > 0) return t("{count} 项执行失败").replace("{count}", String(item.failed_task_count));
  if (item.status === "review") return t("等待真人验收");
  if (item.active_task_count > 0) return t("{count} 项执行中").replace("{count}", String(item.active_task_count));
  if (item.delivered_task_count > 0) return t("{count} 项已交付").replace("{count}", String(item.delivered_task_count));
  return t(statusLabels[item.status]);
}

function attentionReason(item: WorkItem, t: (text: string) => string = text => text) {
  if (item.merge_status === "conflict") return t("合并发生冲突");
  if (item.verification_status === "failed") return t("合并后校验未通过");
  if (item.ai_review_status === "changes_requested" || item.ai_review_status === "failed") return t("AI 建议退回处理");
  if (item.failed_task_count > 0) return t("{count} 项执行失败").replace("{count}", String(item.failed_task_count));
  if (item.status === "review") return t(item.git_branch ? "等待审查或合并" : "等待真人验收");
  if (item.due_at && new Date(item.due_at).getTime() < Date.now() && item.status !== "done") return t("已经超过截止时间");
  return "";
}

function taskTone(item: WorkItem) {
  if (attentionReason(item) && item.status !== "review") return "risk";
  if (item.status === "review") return "review";
  if (item.status === "in_progress" || item.active_task_count > 0) return "active";
  if (item.status === "done") return "done";
  return "idle";
}

function PixelPerson({ member, ownedAgents }: { member: Collaborator; ownedAgents: Agent[] }) {
  const { t } = useLocale();
  const busy = ownedAgents.some((agent) => agent.status === "busy" || (agent.running_task_count ?? 0) > 0);
  const online = ownedAgents.some((agent) => agent.status === "online" || agent.status === "busy");
  return <div className={`pixel-person ${busy ? "is-busy" : online ? "is-online" : "is-offline"}`} title={t("{0} · {1} 个 Agent").replaceAll("{0}", () => String(member.display_name)).replaceAll("{1}", () => String(ownedAgents.length))}>
    <span className="pixel-person-sprite" aria-hidden="true"><i /><b /></span>
    <span><strong>{member.display_name}</strong><small>{ownedAgents.length ? t("{0} 个 Agent").replaceAll("{0}", () => String(ownedAgents.length)) : t("真人成员")}</small></span>
  </div>;
}

function PixelAgent({ agent }: { agent: Agent }) {
  const { t } = useLocale();
  const working = agent.status === "busy" || (agent.running_task_count ?? 0) > 0;
  return <div className={`pixel-agent status-${agent.status} ${working ? "is-working" : ""}`} title={`${agent.display_name} · ${agent.device_name} · ${agent.status}`}>
    <span aria-hidden="true"><i /><b /></span>
    <strong>{agent.display_name}</strong>
    <small>{working ? t("工作中") : agent.status === "online" ? t("就绪") : agent.status === "offline" ? t("离线") : t("不可用")}</small>
  </div>;
}

function StepRoom({ step, items, members, agents, onSelect }: { step: ProjectStep; items: WorkItem[]; members: Collaborator[]; agents: Agent[]; onSelect: (item: WorkItem) => void }) {
  const { t } = useLocale();
  const responsibleMembers = members.filter((member) => step.responsible_user_ids.includes(member.id));
  const responsibleAgents = agents.filter((agent) => step.responsible_agent_ids.includes(agent.id));
  const people = responsibleMembers.length ? responsibleMembers : members.filter((member) => responsibleAgents.some((agent) => agent.owner_user_id === member.id));
  const reviewCount = items.filter((item) => item.status === "review").length;
  const activeCount = items.filter((item) => item.status === "in_progress" || item.active_task_count > 0).length;
  return <section className={`office-step-room step-${step.status}`} aria-label={t("项目步骤 {0}").replaceAll("{0}", () => String(step.title))}>
    <header>
      <span><GitBranch size={15} /><strong>{step.title}</strong></span>
      <em>{step.progress}%</em>
    </header>
    <div className="office-room-meta">
      <span>{activeCount} {t("执行中")}</span><span>{reviewCount} {t("待验收")}</span><span>{t("权重")} {step.weight}</span>
    </div>
    <div className="office-workstations">
      {people.map((member) => <div className="office-owner-station" key={member.id}>
        <PixelPerson member={member} ownedAgents={responsibleAgents.filter((agent) => agent.owner_user_id === member.id)} />
        <div className="office-agent-desks">{responsibleAgents.filter((agent) => agent.owner_user_id === member.id).map((agent) => <PixelAgent key={agent.id} agent={agent} />)}</div>
      </div>)}
      {people.length === 0 && <div className="office-unassigned-seat"><User size={18} /><span><strong>{t("尚未指定负责人")}</strong><small>{t("在结构看板编辑本步骤")}</small></span></div>}
    </div>
    <div className="office-task-tray">
      {items.slice(0, 4).map((item) => <button type="button" className={`office-task-file tone-${taskTone(item)}`} key={item.id} onClick={() => onSelect(item)}>
        <FileText size={14} weight="fill" /><span><strong>{item.title}</strong><small>{stageFor(item, t)}</small></span>
      </button>)}
      {items.length === 0 && <p>{t("本步骤还没有工作事项")}</p>}
      {items.length > 4 && <small>{t("另外")}{items.length - 4}{t("项在结构看板中")}</small>}
    </div>
    <footer>
      <span title={t("全组可读的 Server 正式版本")}><Archive size={14} />{step.workspace ? t("团队底稿 v{0}").replaceAll("{0}", String(step.workspace.revision)) : t("尚无团队底稿")}</span>
      <span title={t("负责人电脑上的实时工作内容")}><Desktop size={14} />{step.local_bindings?.length ? t("{0} 台已绑定").replaceAll("{0}", String(step.local_bindings.length)) : t("本机未绑定")}</span>
    </footer>
  </section>;
}

function TeamRoster({ members, agents }: { members: Collaborator[]; agents: Agent[] }) {
  const { t } = useLocale();
  return <section className="office-team-roster" id="office-team-roster" aria-label={t("总办公室团队席位")}>
    <header><UsersThree size={16} /><span><strong>{t("全员席位")}</strong><small>{t("真人在上，名下 Agent 在对应工位；步骤负责人会同时出现在相关工作区。")}</small></span><em>{members.length}{t("真人 ·")}{agents.length} Agent</em></header>
    <div>{members.map((member) => {
      const ownedAgents = agents.filter((agent) => agent.owner_user_id === member.id);
      return <article key={member.id}>
        <PixelPerson member={member} ownedAgents={ownedAgents} />
        <div>{ownedAgents.length ? ownedAgents.map((agent) => <PixelAgent key={agent.id} agent={agent} />) : <small>{t("尚未接入 Agent")}</small>}</div>
      </article>;
    })}</div>
  </section>;
}

type DetailTab = "overview" | "trace" | "git" | "context";

function TaskDetail({ item, step, plan, agents, tasks, working, onClose, onOpenChat, onUpdateStatus, onOpenStructureBoard }: { item: WorkItem; step?: ProjectStep; plan?: ProjectPlan; agents: Agent[]; tasks: Task[]; working: boolean; onClose: () => void; onOpenChat: (rootMessageID?: string) => void; onUpdateStatus: (itemID: string, status: WorkItemStatus) => Promise<void>; onOpenStructureBoard?: () => void }) {
  const { t, locale } = useLocale();
  const [tab, setTab] = useState<DetailTab>("overview");
  const names = item.target_agent_ids.map((id) => agents.find((agent) => agent.id === id)?.display_name).filter(Boolean);
  const itemTasks = tasks.filter((task) => task.work_item_id === item.id).sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime());
  return <aside className="office-detail-drawer" aria-label={t("{0} 详情").replaceAll("{0}", () => String(item.title))}>
    <header><span><small>{t("当前工作事项")}</small><strong>{item.title}</strong></span><button type="button" onClick={onClose} aria-label={t("关闭事项详情")}>×</button></header>
    <nav className="office-detail-tabs" aria-label={t("事项详情分类")}>
      <button type="button" className={tab === "overview" ? "is-active" : ""} onClick={() => setTab("overview")}>{t("业务")}</button>
      <button type="button" className={tab === "trace" ? "is-active" : ""} onClick={() => setTab("trace")}>{t("执行链路")}</button>
      <button type="button" className={tab === "git" ? "is-active" : ""} onClick={() => setTab("git")}>{t("Git 与验收")}</button>
      <button type="button" className={tab === "context" ? "is-active" : ""} onClick={() => setTab("context")}>{t("共享上下文")}</button>
    </nav>
    {tab === "overview" && <>
      <dl>
        <div><dt>{t("所属步骤")}</dt><dd>{step?.title ?? t("尚未归入步骤")}</dd></div>
        <div><dt>{t("当前阶段")}</dt><dd>{stageFor(item, t)}</dd></div>
        <div><dt>{t("执行 Agent")}</dt><dd>{names.length ? names.join("、") : t("真人事项")}</dd></div>
        <div><dt>{t("执行情况")}</dt><dd>{item.delivered_task_count}/{item.task_count}{t("已交付")}{item.failed_task_count ? t(" · {0} 失败").replaceAll("{0}", () => String(item.failed_task_count)) : ""}</dd></div>
      </dl>
      {attentionReason(item) && <p className="office-detail-alert"><WarningCircle size={16} />{attentionReason(item, t)}</p>}
      <label>{t("更新事项状态")}<select value={item.status} disabled={working} onChange={(event) => void onUpdateStatus(item.id, event.target.value as WorkItemStatus)}>{Object.entries(statusLabels).map(([value, label]) => <option key={value} value={value}>{t(label)}</option>)}</select></label>
    </>}
    {tab === "trace" && <section className="office-detail-section">
      <header><Pulse size={16} /><span><strong>{t("从发起到验收")}</strong><small>{t("这里展示 Hub 记录到的业务节点，不展示内部提示词。")}</small></span></header>
      <div className="office-flow-line" aria-label={t("事项流转路径")}><span className="is-complete">{t("发起")}</span><ArrowRight size={12} /><span className={item.task_count ? "is-complete" : ""}>{t("执行")}</span><ArrowRight size={12} /><span className={item.delivered_task_count ? "is-complete" : ""}>{t("交付")}</span><ArrowRight size={12} /><span className={item.status === "done" ? "is-complete" : item.status === "review" ? "is-current" : ""}>{t("验收")}</span></div>
      <ol>{itemTasks.length ? itemTasks.map((task) => <li key={task.id}><span className={`timeline-node tone-${task.state === "failed" || task.state === "blocked" ? "risk" : "active"}`} /><span><strong>{agents.find((agent) => agent.id === task.target_agent_id)?.display_name ?? "Agent"}</strong><small>{task.state}{t("· 第")}{task.current_attempt}{t("次执行 ·")}{new Date(task.updated_at).toLocaleString(locale)}</small></span></li>) : <li><span className="timeline-node" /><span><strong>{t("尚无 Agent 执行记录")}</strong><small>{t("真人事项也可以直接进入交付与验收。")}</small></span></li>}</ol>
    </section>}
    {tab === "git" && <section className="office-detail-section">
      <header><GitMerge size={16} /><span><strong>{t("文件版本与验收")}</strong><small>{t("Agent 不能直接覆盖团队正式版本。")}</small></span></header>
      <dl><div><dt>{t("修改分支")}</dt><dd>{item.git_branch || t("没有文件修改")}</dd></div><div><dt>{t("AI 审查")}</dt><dd>{item.ai_review_status || t("尚未开始")}</dd></div><div><dt>{t("合并状态")}</dt><dd>{item.merge_status || t("不适用")}</dd></div><div><dt>{t("合并后校验")}</dt><dd>{item.verification_status || t("尚未开始")}</dd></div></dl>
      {item.git_branch && <button type="button" className="office-secondary-action" onClick={onOpenStructureBoard}>{t("到结构看板审查、合并和校验")}</button>}
      {!item.git_branch && item.status === "review" && <div className="office-accept-actions"><button type="button" disabled={working} onClick={() => void onUpdateStatus(item.id, "done")}>{t("验收通过")}</button><button type="button" disabled={working} onClick={() => void onUpdateStatus(item.id, "in_progress")}>{t("退回修改")}</button></div>}
    </section>}
    {tab === "context" && <section className="office-detail-section">
      <header><BookOpenText size={16} /><span><strong>{t("本次任务共享给 Agent 的依据")}</strong><small>{t("显示来源范围，不暴露或改写用户原话。")}</small></span></header>
      <dl><div><dt>{t("项目目标")}</dt><dd>{plan?.objective || t("尚未设置")}</dd></div><div><dt>{t("步骤说明")}</dt><dd>{step?.description || step?.title || t("尚未归入步骤")}</dd></div><div><dt>{t("正式底稿")}</dt><dd>{step?.workspace ? `${step.workspace.source_name} · v${step.workspace.revision}` : t("尚无团队正式版本")}</dd></div><div><dt>{t("本机进展")}</dt><dd>{step?.local_bindings?.length ? t("{0} 台负责人电脑已绑定").replaceAll("{0}", String(step.local_bindings.length)) : t("没有负责人本机目录绑定")}</dd></div><div><dt>{t("知识资料")}</dt><dd>{t("工作区知识库与本群知识库按权限自动附带；具体文件请到知识库查看。")}</dd></div></dl>
    </section>}
    <button type="button" className="office-primary-action" onClick={() => onOpenChat(item.root_message_id)}>{t("查看原话题与完整交付")}</button>
  </aside>;
}

export function ProjectOffice(props: ProjectOfficeProps) {
  const { t, locale } = useLocale();
  const [selectedID, setSelectedID] = useState("");
  const [workingID, setWorkingID] = useState("");
  const [railMode, setRailMode] = useState<"attention" | "activity">("attention");
  const steps = props.plan?.steps.filter((step) => !step.parent_step_id && step.status !== "cancelled") ?? [];
  const activeItems = props.items.filter((item) => item.status !== "done" && item.status !== "cancelled");
  const attention = activeItems.filter((item) => Boolean(attentionReason(item)));
  const unassigned = activeItems.filter((item) => !item.plan_step_id);
  const selected = props.items.find((item) => item.id === selectedID);
  const liveTasks = (props.tasks ?? []).filter((task) => task.state === "queued" || task.state === "dispatched" || task.state === "running" || task.state === "blocked");
  const failedTasks = (props.tasks ?? []).filter((task) => task.state === "failed");
  const relevantFailedTasks = failedTasks.filter((task) => {
    if (!task.work_item_id) return true;
    return activeItems.some((item) => item.id === task.work_item_id);
  });
  const executingCount = new Set([
    ...activeItems.filter((item) => item.status === "in_progress" || item.active_task_count > 0).map((item) => `item:${item.id}`),
    ...liveTasks.map((task) => task.work_item_id ? `item:${task.work_item_id}` : `task:${task.id}`),
  ]).size;
  const pendingCount = new Set([
    ...attention.map((item) => `item:${item.id}`),
    ...relevantFailedTasks.map((task) => task.work_item_id ? `item:${task.work_item_id}` : `task:${task.id}`),
  ]).size;
  const sortedActivity = useMemo(() => [...props.items].sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime()).slice(0, 8), [props.items]);
  async function updateStatus(itemID: string, status: WorkItemStatus) {
    setWorkingID(itemID);
    try { await props.onUpdateStatus(itemID, status); } finally { setWorkingID(""); }
  }
  return <div className="project-office">
    <section className="office-command-strip">
      <div><button type="button" onClick={() => setRailMode("activity")}><strong>{executingCount}</strong><small>{t("正在执行")}</small></button><button type="button" className={pendingCount ? "has-attention" : ""} onClick={() => setRailMode("attention")}><strong>{pendingCount}</strong><small>{t("待我处理")}</small></button><button type="button" onClick={() => document.getElementById("office-team-roster")?.scrollIntoView({ behavior: "smooth", block: "center" })}><strong>{props.agents.filter((agent) => agent.status === "online" || agent.status === "busy").length}</strong><small>{t("Agent 在线")}</small></button></div>
      <p><span className="office-live-dot" />{t("一张总办公室展示全员、步骤、任务和异常；点击工单查看业务、链路、Git 与共享上下文。")}</p>
    </section>
    <div className="office-layout">
      <main className="office-floor" aria-label={t("项目总办公室")}>
        <div className="office-floor-title"><span><strong>{t("总办公室")}</strong><small>{props.plan?.objective || t("所有真人、Agent、步骤与事项集中在同一张协作楼层图")}</small></span><em>{props.plan?.progress ?? 0}%</em></div>
        <div className="office-floor-legend"><span><i className="tone-active" />{t("工作区")}</span><span><i className="tone-review" />{t("待验收")}</span><span><i className="tone-risk" />{t("有异常")}</span><small>{t("步骤是办公室区域，不是互相隔离的项目副本")}</small></div>
        <div className="office-rooms">{steps.map((step) => <StepRoom key={step.id} step={step} items={activeItems.filter((item) => item.plan_step_id === step.id)} members={props.members} agents={props.agents} onSelect={(item) => setSelectedID(item.id)} />)}</div>
        {steps.length === 0 && <div className="office-empty-floor"><GitBranch size={24} /><strong>{t("先在结构看板建立项目步骤")}</strong><small>{t("步骤会变成房间，负责人和 Agent 会出现在相应工作台。")}</small></div>}
        <section className="office-common-area">
          <div><header><FolderOpen size={16} /><strong>{t("中央收件台")}</strong><span>{unassigned.length}</span></header>{unassigned.length ? unassigned.slice(0, 5).map((item) => <button type="button" key={item.id} onClick={() => setSelectedID(item.id)}>{item.title}</button>) : <small>{t("所有工作事项都已归入步骤")}</small>}</div>
          <div className="office-records-cabinet"><header><Archive size={16} /><strong>{t("公共档案柜")}</strong><span>{steps.filter((step) => Boolean(step.workspace)).length}</span></header><small>{t("公共底稿是团队里程碑版本；负责人平时仍在自己绑定的本机目录工作。")}</small></div>
        </section>
        <TeamRoster members={props.members} agents={props.agents} />
      </main>
      <aside className="office-control-rail" aria-label={t("总办公室控制台")}>
        <header><span><strong>{t("总控台")}</strong><small>{t("处理异常或追踪最近流转")}</small></span><nav><button type="button" className={railMode === "attention" ? "is-active" : ""} onClick={() => setRailMode("attention")}><CheckCircle size={14} />{t("待我处理")}{pendingCount}</button><button type="button" className={railMode === "activity" ? "is-active" : ""} onClick={() => setRailMode("activity")}><ClockCountdown size={14} />{t("最近流转")}</button></nav></header>
        {railMode === "attention" ? <div className="office-attention-list">{pendingCount ? <>{attention.map((item) => <button type="button" key={item.id} onClick={() => setSelectedID(item.id)}><span className="project-status-dot status-review" /><span><strong>{item.title}</strong><small>{attentionReason(item, t)}</small></span></button>)}{relevantFailedTasks.filter((task) => !task.work_item_id || !attention.some((item) => item.id === task.work_item_id)).map((task) => <button type="button" key={task.id} onClick={() => props.onOpenChat(task.root_message_id)}><WarningCircle size={14} /><span><strong>{task.title}</strong><small>{t("Agent 执行失败，点击查看凭证")}</small></span></button>)}</> : <p><CheckCircle size={18} />{t("当前没有需要真人处理的事项")}</p>}</div> : <div className="office-activity-list">{sortedActivity.map((item) => <button type="button" key={item.id} onClick={() => setSelectedID(item.id)}><span className={`project-status-dot status-${item.status}`} /><span><strong>{item.title}</strong><small>{stageFor(item, t)} · {new Date(item.updated_at).toLocaleString(locale)}</small></span></button>)}{sortedActivity.length === 0 && <p>{t("还没有任务活动")}</p>}</div>}
      </aside>
    </div>
    {selected && <TaskDetail key={selected.id} item={selected} step={props.plan?.steps.find((step) => step.id === selected.plan_step_id)} plan={props.plan} agents={props.agents} tasks={props.tasks ?? []} working={workingID === selected.id} onClose={() => setSelectedID("")} onOpenChat={props.onOpenChat} onUpdateStatus={updateStatus} onOpenStructureBoard={props.onOpenStructureBoard} />}
  </div>;
}

export function ProjectTimeline({ plan, items, agents, tasks = [], onOpenChat }: Pick<ProjectOfficeProps, "plan" | "items" | "agents" | "tasks" | "onOpenChat">) {
  const { t, locale } = useLocale();
  const sorted = [...items].sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime());
  return <section className="project-execution-timeline">
    <header><div><strong>{t("执行时间线")}</strong><small>{t("从发起、执行、交付、审查到验收，按最后变化时间排列")}</small></div><span>{t("{count} 项记录").replace("{count}", String(sorted.length))}</span></header>
    <div>{tasks.filter((task) => task.state !== "delivered" && task.state !== "cancelled").sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime()).map((task) => {
      const agent = agents.find((candidate) => candidate.id === task.target_agent_id);
      const label = task.state === "queued" ? "等待 Runner 领取" : task.state === "dispatched" ? "Runner 已领取" : task.state === "running" ? "原生 Agent 执行中" : task.state === "blocked" ? "等待恢复" : "执行失败";
      return <button type="button" key={`task-${task.id}`} onClick={() => onOpenChat(task.root_message_id)}><span className={`timeline-node tone-${task.state === "failed" || task.state === "blocked" ? "risk" : "active"}`} /><span><small>{new Date(task.updated_at).toLocaleString(locale)} · {t("第 {count} 次尝试").replace("{count}", String(task.current_attempt))}</small><strong>{task.title}</strong><em>{agent?.display_name ?? "Agent"} · {t("原生任务状态")}</em></span><b>{t(label)}</b></button>;
    })}{sorted.map((item) => {
      const agentNames = item.target_agent_ids.map((id) => agents.find((agent) => agent.id === id)?.display_name).filter(Boolean);
      const step = plan?.steps.find((candidate) => candidate.id === item.plan_step_id);
      return <button type="button" key={item.id} onClick={() => onOpenChat(item.root_message_id)}><span className={`timeline-node tone-${taskTone(item)}`} /><span><small>{new Date(item.updated_at).toLocaleString(locale)}</small><strong>{item.title}</strong><em>{step?.title ?? t("未归入步骤")} · {agentNames.join(locale === "en" ? ", " : "、") || t("真人事项")}</em></span><b>{stageFor(item, t)}</b></button>;
    })}{sorted.length === 0 && tasks.length === 0 && <p>{t("还没有可显示的执行记录")}</p>}</div>
  </section>;
}
