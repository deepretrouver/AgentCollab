import { useState, type FormEvent } from "react";
import { useLocale } from "../i18n/locale";
import { CheckCircle, X } from "@phosphor-icons/react";
import type { Agent, ProjectPlan, Task, TaskExecutionSource, User } from "../api/client";
import { agentDisplayName, agentShortName, groupAgentsByOwner } from "../agents/presentation";

interface TaskComposerProps {
  agents: Agent[];
  members?: User[];
  initialPrompt: string;
  projectPlan?: ProjectPlan;
  initialStepID?: string;
  onClose: () => void;
  onCreate: (input: { target_agent_id: string; title: string; prompt: string; acceptance_criteria: string; plan_step_id?: string; execution_source?: TaskExecutionSource }) => Promise<Task>;
}

export function TaskComposer({ agents, members = [], projectPlan, initialStepID = "", initialPrompt, onClose, onCreate }: TaskComposerProps) {
  const { t } = useLocale();
	const availableAgents = agents.filter((agent) => agent.can_invoke !== false && agent.status !== "unavailable");
  const agentGroups = groupAgentsByOwner(availableAgents, members);
  const [target, setTarget] = useState(availableAgents.find((agent) => agent.status === "online")?.id ?? availableAgents[0]?.id ?? "");
  const [title, setTitle] = useState("");
  const [prompt, setPrompt] = useState(initialPrompt);
  const [acceptance, setAcceptance] = useState("");
  const [planStepID, setPlanStepID] = useState(initialStepID);
  const [executionSource, setExecutionSource] = useState<TaskExecutionSource>("official");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!target || !title.trim() || !prompt.trim() || submitting) return;
    setSubmitting(true);
    setError("");
    try {
      await onCreate({ target_agent_id: target, title: title.trim(), prompt: prompt.trim(), acceptance_criteria: acceptance.trim(), plan_step_id: planStepID || undefined, execution_source: planStepID ? executionSource : undefined });
      onClose();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "任务创建失败");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form className="task-composer" onSubmit={submit}>
      <header><div><span>{t("结构化任务")}</span><strong>{t("交给指定 Agent 执行并回传群聊")}</strong></div><button type="button" onClick={onClose} aria-label={t("关闭任务面板")}><X size={18} /></button></header>
      <label>{t("目标 Agent")}<select value={target} onChange={(event) => setTarget(event.target.value)} required>
		{agentGroups.map((group) => <optgroup key={group.ownerID} label={group.ownerName}>
          {group.agents.map((agent) => <option key={agent.id} value={agent.id}>{group.agents.length === 1 ? agentDisplayName(agent, group.owner) : agentShortName(agent, group.owner)} · {agent.type} · {agent.device_name}{agent.status === "offline" ? ` · ${t("离线，等待上线")}` : ""}</option>)}
        </optgroup>)}
      </select></label>
      <label>{t("任务标题")}<input value={title} onChange={(event) => setTitle(event.target.value)} maxLength={160} placeholder={t("例如：检查本地项目状态")} required /></label>
      {projectPlan && projectPlan.steps.length > 0 && <label>{t("归属项目步骤")}<select value={planStepID} onChange={(event) => setPlanStepID(event.target.value)}><option value="">{t("独立任务 · 不计项目进度")}</option>{projectPlan.steps.filter((step) => step.status !== "cancelled").map((step) => <option key={step.id} value={step.id}>{step.parent_step_id ? "↳ " : ""}{step.title}</option>)}</select></label>}
      {planStepID && <label>{t("项目资料")}<select aria-label={t("任务项目资料")} value={executionSource} onChange={(event) => setExecutionSource(event.target.value as TaskExecutionSource)}><option value="official">{t("公共底稿 · 群内共享只读")}</option><option value="responsible_local">{t("查询负责人本机最新进展")}</option><option value="none">{t("不读取文件 · 仅会话上下文")}</option></select><small>{t(executionSource === "responsible_local" ? "目标 Agent 必须是该步骤负责人，并已在自己的 Client 绑定本机目录；仅汇报，不自动提交。" : executionSource === "official" ? "Agent 获得任务临时副本，不能直接覆盖公共底稿。" : "")}</small></label>}
      <label>{t("任务要求")}<textarea value={prompt} onChange={(event) => setPrompt(event.target.value)} maxLength={50_000} required /></label>
      <label>{t("验收标准（可选）")}<input value={acceptance} onChange={(event) => setAcceptance(event.target.value)} placeholder={t("例如：返回一句确认文本")} /></label>
      {error && <p className="task-error" role="alert">{t(error)}</p>}
	  <button className="task-submit" type="submit" disabled={submitting || !target || !title.trim() || !prompt.trim()}><CheckCircle size={18} weight="fill" />{t(submitting ? "正在派发" : availableAgents.find((agent) => agent.id === target)?.status === "offline" ? "创建并等待上线" : "创建并派发")}</button>
    </form>
  );
}
