import { ShieldCheck, UserCircle, X } from "@phosphor-icons/react";
import { useEffect, useState } from "react";
import type { User } from "../api/client";

interface HubAccountsDialogProps {
  open: boolean;
  currentUser: User;
  onClose: () => void;
  onLoad: () => Promise<User[]>;
  onUpdateRole: (userID: string, role: User["role"]) => Promise<User>;
  onArchiveAccount: (userID: string) => Promise<void>;
}

function initial(name: string) {
  return name.trim().slice(0, 1).toUpperCase() || "?";
}

export function HubAccountsDialog({ open, currentUser, onClose, onLoad, onUpdateRole, onArchiveAccount }: HubAccountsDialogProps) {
  const [accounts, setAccounts] = useState<User[]>([]);
  const [loading, setLoading] = useState(false);
  const [updatingID, setUpdatingID] = useState("");
  const [archiveConfirmationID, setArchiveConfirmationID] = useState("");
  const [archivingID, setArchivingID] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return;
    let active = true;
    setLoading(true);
    setError("");
    setArchiveConfirmationID("");
    void onLoad().then((next) => { if (active) setAccounts(next); }).catch((reason) => {
      if (active) setError(reason instanceof Error ? reason.message : "无法读取 Hub 账号。");
    }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [open, onLoad]);

  if (!open) return null;

  async function updateRole(account: User, role: User["role"]) {
    if (account.id === currentUser.id || account.role === role) return;
    setUpdatingID(account.id);
    setError("");
    try {
      const updated = await onUpdateRole(account.id, role);
      setAccounts((current) => current.map((item) => item.id === updated.id ? updated : item));
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "无法更新账号权限。");
    } finally {
      setUpdatingID("");
    }
  }

  async function archiveAccount(account: User) {
    if (account.id === currentUser.id) return;
    setArchivingID(account.id);
    setError("");
    try {
      await onArchiveAccount(account.id);
      setAccounts((current) => current.filter((item) => item.id !== account.id));
      setArchiveConfirmationID("");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "无法删除账号。");
    } finally {
      setArchivingID("");
    }
  }

  return <div className="hub-accounts-backdrop" role="presentation" onMouseDown={(event) => { if (event.currentTarget === event.target) onClose(); }}>
    <section className="hub-accounts-dialog" role="dialog" aria-modal="true" aria-labelledby="hub-accounts-title">
      <header><div><span>Hub 管理</span><strong id="hub-accounts-title">账号管理员</strong><p>这是平台级权限：可管理账号和 Hub 管理员，但不会自动加入、查看或修改任何工作区、群聊、Agent、Skill 或本地电脑。</p></div><button type="button" onClick={onClose} aria-label="关闭账号管理员"><X size={20} /></button></header>
      <div className="hub-role-note"><ShieldCheck size={18} /><span><strong>@owner</strong> 是默认 Hub 管理员，不可降级或删除。工作区 Owner、工作区管理员、群主和群管理员仍在各自工作区/群详情中单独管理。</span></div>
      <div className="hub-account-list" aria-busy={loading}>
        {loading && <p className="hub-account-empty">正在读取账号…</p>}
        {!loading && accounts.map((account) => <article className="hub-account-row" key={account.id}>
          <span className="hub-account-avatar">{account.avatar_url ? <img src={account.avatar_url} alt="" /> : initial(account.display_name)}</span>
          <div className="hub-account-info"><strong>{account.display_name}</strong><small>@{account.username}</small></div>
          {account.username === "owner" ? <span className="hub-admin-badge">默认 Hub 管理员</span> : account.role === "admin" ? <span className="hub-admin-badge">Hub 管理员</span> : <span className="hub-member-badge">普通账号</span>}
          {account.id === currentUser.id ? <span className="hub-account-self">当前账号</span> : account.username === "owner" ? <span className="hub-account-self">受保护</span> : <div className="hub-account-actions">
            <button type="button" disabled={updatingID === account.id || archivingID === account.id} onClick={() => void updateRole(account, account.role === "admin" ? "member" : "admin")}>{updatingID === account.id ? "保存中" : account.role === "admin" ? "移除管理员" : "设为管理员"}</button>
            <button className="hub-account-delete-button" type="button" disabled={updatingID === account.id || archivingID === account.id} onClick={() => { setArchiveConfirmationID(account.id); setError(""); }} aria-label={`删除账号 ${account.display_name}`}>{archivingID === account.id ? "删除中" : "删除账号"}</button>
          </div>}
          {archiveConfirmationID === account.id && <div className="hub-account-delete-confirm" role="alert"><p>删除后，该账号将无法登录并从 Hub 名单隐藏；历史消息会保留原署名。</p><div><button className="hub-account-delete-button" type="button" disabled={archivingID === account.id} onClick={() => void archiveAccount(account)} aria-label={`确认删除账号 ${account.display_name}`}>{archivingID === account.id ? "正在删除" : "确认删除"}</button><button type="button" disabled={archivingID === account.id} onClick={() => setArchiveConfirmationID("")}>取消</button></div></div>}
        </article>)}
        {!loading && accounts.length === 0 && <p className="hub-account-empty">尚未找到账号。</p>}
      </div>
      {error && <p className="form-error hub-account-error" role="alert">{error}</p>}
    </section>
  </div>;
}
