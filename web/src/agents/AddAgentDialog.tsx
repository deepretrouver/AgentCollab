import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { Check, Cpu, DesktopTower, MagnifyingGlass, Plus, WarningCircle, X } from "@phosphor-icons/react";
import { api, type Agent, type RunnerInvite } from "../api/client";
import { useLocale } from "../i18n/locale";

interface AddAgentDialogProps {
  open: boolean;
  discovered: Agent[];
  registered: Agent[];
  scanned: boolean;
  scanning: boolean;
  onScan: () => Promise<void>;
  onCreateRunnerInvite: () => Promise<RunnerInvite>;
  onClose: () => void;
  onRegister: (agentIDs: string[]) => Promise<void>;
}

type DesktopConnectionIssue = "unconnected" | "other-account" | "offline" | "web";

function runtimeLabel(agent: Agent, t: (text: string) => string) {
  if (agent.transport !== "acp") return t("原生直连");
  return `${t(agent.catalog_tier === "preset" ? "ACP 预设" : "自定义 ACP")} v${agent.protocol_version ?? 1}`;
}

export function AddAgentDialog({ open, discovered, registered, scanned, scanning, onScan, onCreateRunnerInvite, onClose, onRegister }: AddAgentDialogProps) {
  const { locale, t } = useLocale();
  const registeredIDs = useMemo(() => new Set(registered.map((agent) => agent.id)), [registered]);
  const available = discovered.filter((agent) => !registeredIDs.has(agent.id));
  const [selected, setSelected] = useState<string[]>([]);
  const validSelected = selected.filter((id) => available.some((agent) => agent.id === id && agent.available));
  const [saving, setSaving] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [connectionIssue, setConnectionIssue] = useState<DesktopConnectionIssue>();
  const [error, setError] = useState("");
  const autoScanStarted = useRef(false);

  useEffect(() => {
    if (!open) {
      autoScanStarted.current = false;
      setSelected([]);
      setConnectionIssue(undefined);
      setError("");
      return;
    }
    let active = true;
    void inspectDesktopConnection().then(async (issue) => {
      if (!active) return;
      setConnectionIssue(issue);
      if (!issue && !scanned && !scanning && !autoScanStarted.current) {
        // Mark only an actual scan, not a connection check that can be cancelled.
        // Keep the mark after failures so retries remain an explicit user action.
        autoScanStarted.current = true;
        await onScan();
      }
    }).catch((reason) => {
      if (active) setError(reason instanceof Error ? reason.message : t("无法检查本机连接"));
    });
    return () => { active = false; };
  // Opening this dialog means "find agents": check the Client and start the scan without a second click.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, scanned, scanning]);

  if (!open) return null;

  function toggle(agentID: string) {
    setSelected((current) => current.includes(agentID) ? current.filter((id) => id !== agentID) : [...current, agentID]);
  }

  async function inspectDesktopConnection(): Promise<DesktopConnectionIssue | undefined> {
    const desktop = window.agentCollabDesktop;
    if (!desktop) return "web";
    const state = await desktop.getState();
    if (!state.runnerRegistered) return "unconnected";
    const ownership = state.runnerId ? await api.runnerOwnership(state.runnerId) : { owned_by_current_user: false };
    if (!ownership.owned_by_current_user) return "other-account";
    return state.runnerRunning ? undefined : "offline";
  }

  async function scanWithDesktopCheck() {
    if (scanning || connecting) return;
    setError("");
    try {
      const issue = await inspectDesktopConnection();
      setConnectionIssue(issue);
      if (issue) return;
      await onScan();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("无法检查本机连接"));
    }
  }

  async function connectAndScan() {
    const desktop = window.agentCollabDesktop;
    if (!desktop || connecting) return;
    setConnecting(true);
    setError("");
    try {
      if (connectionIssue === "offline") {
        await desktop.reconnect();
      } else {
        const invite = await onCreateRunnerInvite();
        await desktop.connectCurrentComputer({ hubUrl: window.location.origin, inviteCode: invite.code });
      }
      await new Promise((resolve) => window.setTimeout(resolve, 700));
      setConnectionIssue(undefined);
      await onScan();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("无法连接本机 Agent 服务"));
    } finally {
      setConnecting(false);
    }
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (validSelected.length === 0 || saving) return;
    setSaving(true);
    setError("");
    try {
      await onRegister(validSelected);
      setSelected([]);
      onClose();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("无法注册 Agent"));
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="agent-dialog-backdrop" role="presentation">
      <form className="agent-dialog" onSubmit={submit} aria-label={t("添加我的 Agent")}>
        <header>
          <div><strong>{t("添加我的 Agent")}</strong><p>{t("选择这台电脑上要带入团队的 Agent；已添加过的 Agent 可以直接复用。")}</p></div>
          <button type="button" onClick={onClose} aria-label={t("关闭注册 Agent")}><X size={19} /></button>
        </header>
        <div className="agent-picker" role="region" aria-label={t("扫描到的 Agent")}>
          {connectionIssue ? <div className="agent-picker-empty needs-connection">
            <DesktopTower size={23} />
            <strong>{t(connectionIssue === "web" ? "网页版不能扫描这台电脑" : connectionIssue === "other-account" ? "这台电脑属于另一个账号" : connectionIssue === "unconnected" ? "先把这台电脑连接到当前账号" : "本机 Agent 服务未连接")}</strong>
            <p>{t(connectionIssue === "web" ? "请在 AgentCollab Client 中打开当前团队，再点击“添加我的 Agent”。网页版可以参与协作，但不能直接扫描本机；已通过 Client 接入并授权的 Agent 仍可在网页中调用。" : connectionIssue === "other-account" ? "确认改绑后，本机 Agent 将由当前登录账号管理；旧账号的历史消息和交付记录不会删除。" : connectionIssue === "unconnected" ? "Client 会自动连接这台电脑，不需要填写地址或配对码。" : "点击下方按钮恢复连接，成功后会自动继续查找 Agent。")}</p>
            {connectionIssue !== "web" && <button type="button" className="agent-connect-action" disabled={connecting || scanning} onClick={() => void connectAndScan()}>{t(connecting ? "正在连接" : connectionIssue === "other-account" ? "改绑到当前账号并查找" : connectionIssue === "offline" ? "重新连接并查找" : "连接本机并查找")}</button>}
          </div> : !scanned ? <div className="agent-scan-start">
            <span><MagnifyingGlass size={24} /></span>
            <strong>{t("查找可加入的本机 Agent")}</strong>
            <p>{t("这里只读取本机可用 Agent，不会执行任务。")}</p>
            <button type="button" disabled={scanning || connecting} onClick={() => void scanWithDesktopCheck()}>{t(scanning ? "正在查找" : "查找本机 Agent")}</button>
          </div> : <>
          {available.map((agent) => {
            const checked = validSelected.includes(agent.id);
            return (
              <label className={`agent-choice${checked ? " is-selected" : ""}${!agent.available ? " is-unavailable" : ""}`} key={agent.id}>
                <input type="checkbox" checked={checked} disabled={!agent.available} onChange={() => toggle(agent.id)} />
                <span className="agent-choice-icon"><Cpu size={20} weight="fill" /></span>
                <span className="agent-choice-copy">
                  <strong>{agent.display_name}<small>{agent.type} · {runtimeLabel(agent, t)}</small></strong>
                  <span>{agent.available ? `${t("可加入")} · ${agent.version || t("版本未知")}` : t("未找到可执行文件")}</span>
                  <span>{agent.executable || `${agent.type} ${t("CLI 未安装")}`} · {agent.device_name}</span>
                </span>
                <span className="agent-choice-check">{!agent.available ? <WarningCircle size={16} /> : checked ? <Check size={16} weight="bold" /> : <Plus size={16} />}</span>
              </label>
            );
          })}
          {available.length === 0 && <div className="agent-picker-empty"><Check size={22} /><strong>{t(discovered.length > 0 ? "这台电脑上的 Agent 已全部添加" : "没有找到可用 Agent")}</strong><p>{t(discovered.length > 0 ? "它们已经在工作区 Agent 库中，可以直接加入群聊。" : "请确认本机已经安装并登录相应 Agent CLI，然后重新查找。")}</p></div>}
          </>}
        </div>
        {error && <p className="task-error" role="alert">{error}</p>}
        <footer aria-label={t("加入操作")}>
          <span>{t(scanned ? "只显示当前可用的 Agent；安装或登录新的 Agent 后可再次查找。" : "查找完成后，选择要带入团队的 Agent。")}</span>
          {scanned && <button className="agent-add-submit" type="submit" disabled={saving || validSelected.length === 0}>{saving ? t("正在加入") : locale === "en" ? `Add ${validSelected.length} agent${validSelected.length === 1 ? "" : "s"}` : `加入 ${validSelected.length || ""} 个 Agent`}</button>}
        </footer>
      </form>
    </div>
  );
}
