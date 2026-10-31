import { Check, Desktop, Robot, ShieldCheck } from "@phosphor-icons/react";
import { useEffect, useMemo, useState } from "react";
import type { Agent, User } from "../api/client";
import { useLocale } from "../i18n/locale";

interface MyAgentsSettingsProps {
  user: User;
  agents: Agent[];
  onUpdatePolicy: (agentID: string, policy: { visibility: "private" | "workspace"; invocation_scope: "owner_only" | "room_members"; shared_skills: string[] }) => Promise<void>;
  onManageAgents?: () => void;
}

function connectionLabel(agent: Agent) {
  if (agent.status === "busy") return "执行中";
  if (agent.status === "online") return "就绪";
  if (agent.status === "unavailable") return "不可用";
  return "离线";
}

function skillSource(agent: Agent, source?: string) {
  if (source?.trim()) return source;
  return agent.type === "opencode" ? "通用本机" : "本机已扫描";
}

export function MyAgentsSettings({ user, agents, onUpdatePolicy, onManageAgents }: MyAgentsSettingsProps) {
  const { t } = useLocale();
  const ownedAgents = useMemo(() => agents.filter((agent) => agent.can_manage ?? agent.owner_user_id === user.id), [agents, user.id]);
  const [selectedID, setSelectedID] = useState(ownedAgents[0]?.id ?? "");
  const [updating, setUpdating] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!ownedAgents.some((agent) => agent.id === selectedID)) setSelectedID(ownedAgents[0]?.id ?? "");
  }, [ownedAgents, selectedID]);

  const selected = ownedAgents.find((agent) => agent.id === selectedID);

  async function update(patch: Partial<{ visibility: "private" | "workspace"; invocation_scope: "owner_only" | "room_members"; shared_skills: string[] }>) {
    if (!selected || updating) return;
    setUpdating(true);
    setError("");
    try {
      await onUpdatePolicy(selected.id, {
        visibility: patch.visibility ?? selected.visibility ?? "workspace",
        invocation_scope: patch.invocation_scope ?? selected.invocation_scope ?? "owner_only",
        shared_skills: patch.shared_skills ?? selected.shared_skills ?? [],
      });
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("Agent 权限保存失败，请稍后重试。"));
    } finally {
      setUpdating(false);
    }
  }

  if (ownedAgents.length === 0) return <div className="my-agents-empty">
    <Robot size={32} weight="duotone" />
    <strong>{t("还没有属于你的 Agent")}</strong>
    <p>{t("先连接本机 Client，并扫描加入 Codex、Hermes、WorkBuddy 等本机 Agent。")}</p>
    {onManageAgents && <button type="button" className="primary-button" onClick={onManageAgents}>{t("添加我的 Agent")}</button>}
  </div>;

  const skills = selected?.skills ?? [];
  const resources = selected?.resources ?? [];
  const sharedSkills = selected?.shared_skills ?? [];
  // Saved grants may refer to skills absent from the latest discovery.
  // Count the intersection, not two equally sized but different lists.
  const currentSharedCount = skills.filter((skill) => sharedSkills.includes(skill.name)).length;

  return <div className="my-agents-console">
    <aside className="my-agents-list" aria-label={t("我的 Agent 列表")}>
      <div className="my-agents-list-heading"><span>{t("我的 Agent")}</span><strong>{ownedAgents.length}</strong></div>
      {ownedAgents.map((agent) => <button type="button" key={agent.id} className={agent.id === selectedID ? "is-selected" : ""} onClick={() => setSelectedID(agent.id)}>
        <Robot size={18} weight="fill" />
        <span><strong>{agent.display_name}</strong><small>{agent.type} · {agent.device_name}</small></span>
        <em className={`agent-console-status is-${agent.status}`}>{t(connectionLabel(agent))}</em>
      </button>)}
      {onManageAgents && <button type="button" className="my-agents-add" onClick={onManageAgents}>{t("＋ 添加或扫描 Agent")}</button>}
    </aside>

    {selected && <section className="my-agent-detail" aria-label={`${selected.display_name} ${t("权限设置")}`}>
      <header>
        <div className="my-agent-detail-icon"><Robot size={23} weight="fill" /></div>
        <div><strong>{selected.display_name}</strong><span>{selected.type} · {selected.device_name} · {selected.os}/{selected.arch}</span></div>
        <em className={`agent-console-status is-${selected.status}`}>{t(connectionLabel(selected))}</em>
      </header>

      <section className="my-agent-policy-grid">
        <label><span>{t("可见范围")}</span><select aria-label={`${selected.display_name} ${t("可见范围")}`} value={selected.visibility ?? "workspace"} disabled={updating} onChange={(event) => void update({ visibility: event.target.value as "private" | "workspace" })}><option value="private">{t("仅自己可见")}</option><option value="workspace">{t("工作区可见")}</option></select><small>{t("决定其他成员是否能在 Agent 库看到它。")}</small></label>
        <label><span>{t("调用范围")}</span><select aria-label={`${selected.display_name} ${t("调用范围")}`} value={selected.invocation_scope ?? "owner_only"} disabled={updating} onChange={(event) => void update({ invocation_scope: event.target.value as "owner_only" | "room_members" })}><option value="owner_only">{t("仅自己调用")}</option><option value="room_members">{t("所在群成员可调用")}</option></select><small>{t("仍需先把 Agent 加入对应群聊。")}</small></label>
      </section>

      <section className="my-agent-skill-policy">
        <header><div><strong>{t("允许他人调用的 Skills")}</strong><span>{t("全部扫描结果都显示在这里；逐项勾选才会共享。")}</span></div><div><b>{currentSharedCount} / {skills.length}</b><button type="button" disabled={updating || skills.length === 0 || currentSharedCount === skills.length} onClick={() => void update({ shared_skills: skills.map((skill) => skill.name) })}>{t("全部授权")}</button><button type="button" disabled={updating || sharedSkills.length === 0} onClick={() => void update({ shared_skills: [] })}>{t("全部取消")}</button></div></header>
        {skills.length > 0 ? <div className="my-agent-skill-list">{skills.map((skill) => {
          const checked = sharedSkills.includes(skill.name);
          return <label key={skill.name} title={skill.description}>
            <input type="checkbox" checked={checked} disabled={updating} onChange={() => void update({ shared_skills: checked ? sharedSkills.filter((name) => name !== skill.name) : [...sharedSkills, skill.name] })} />
            <span><strong>{skill.name}</strong><small>{skill.description || t("本机 Skill")}</small></span>
            <em>{skill.source?.trim() ? skill.source : t(skillSource(selected))}</em>
            {checked && <Check size={16} weight="bold" />}
          </label>;
        })}</div> : <p className="my-agent-capability-empty">{t("这台 Client 尚未同步 Skills；重新扫描或重连后会自动显示。")}</p>}
      </section>

      <section className="my-agent-resources">
        <header><div><strong>{t("本机 Resources")}</strong><span>{t("仅供你自己的 Agent 在本机执行时使用，不向其他成员开放。")}</span></div><ShieldCheck size={20} /></header>
        {resources.length > 0 ? <div>{resources.map((resource) => <span key={resource.name}><Desktop size={15} /><strong>{resource.name}</strong><em>{resource.access}</em></span>)}</div> : <p>{t("未同步本机资源。")}</p>}
      </section>
      {error && <p className="form-error" role="alert">{error}</p>}
    </section>}
  </div>;
}
