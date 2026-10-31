import { Camera, X } from "@phosphor-icons/react";
import { useEffect, useRef, useState, type ChangeEvent, type FormEvent } from "react";
import type { Agent, User } from "../api/client";
import { MyAgentsSettings } from "./MyAgentsSettings";
import { useLocale } from "../i18n/locale";

export type ProfileSection = "profile" | "agents";

interface ProfileDialogProps {
  open: boolean;
  user: User;
  onClose: () => void;
  onSave: (input: Pick<User, "display_name" | "avatar_url" | "bio" | "current_focus">) => Promise<void>;
	canManageHub?: boolean;
	onManageHub?: () => void;
  agents?: Agent[];
  initialSection?: ProfileSection;
  onUpdateAgentPolicy?: (agentID: string, policy: { visibility: "private" | "workspace"; invocation_scope: "owner_only" | "room_members"; shared_skills: string[] }) => Promise<void>;
  onManageAgents?: () => void;
}

const acceptedImageTypes = new Set(["image/png", "image/jpeg", "image/webp"]);
const maxAvatarBytes = 1_000_000;

function initial(name: string) {
  return name.trim().slice(0, 1).toUpperCase() || "?";
}

export function ProfileDialog({ open, user, onClose, onSave, canManageHub = false, onManageHub, agents = [], initialSection = "profile", onUpdateAgentPolicy, onManageAgents }: ProfileDialogProps) {
  const { t } = useLocale();
  const [displayName, setDisplayName] = useState(user.display_name);
  const [avatarURL, setAvatarURL] = useState(user.avatar_url ?? "");
  const [bio, setBio] = useState(user.bio ?? "");
  const [currentFocus, setCurrentFocus] = useState(user.current_focus ?? "");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [section, setSection] = useState<ProfileSection>(initialSection);
  const fileInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!open) return;
    setDisplayName(user.display_name);
    setAvatarURL(user.avatar_url ?? "");
    setBio(user.bio ?? "");
    setCurrentFocus(user.current_focus ?? "");
    setError("");
    setSection(initialSection);
  }, [open, user, initialSection]);

  if (!open) return null;

  function selectAvatar(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    if (!acceptedImageTypes.has(file.type) || file.size > maxAvatarBytes) {
      setError(t("头像请使用小于 1MB 的 PNG、JPG 或 WebP 图片。"));
      return;
    }
    const reader = new FileReader();
    reader.onload = () => setAvatarURL(typeof reader.result === "string" ? reader.result : "");
    reader.onerror = () => setError(t("头像读取失败，请重新选择图片。"));
    reader.readAsDataURL(file);
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setError("");
    setSaving(true);
    try {
      await onSave({ display_name: displayName.trim(), avatar_url: avatarURL, bio: bio.trim(), current_focus: currentFocus.trim() });
      onClose();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("个人资料保存失败，请稍后重试。"));
    } finally {
      setSaving(false);
    }
  }

  return <div className="profile-dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.currentTarget === event.target) onClose(); }}>
    <section className={`profile-dialog${section === "agents" ? " is-agent-console" : ""}`} role="dialog" aria-modal="true" aria-labelledby="profile-dialog-title">
      <header>
        <div><span>{t("个人设置")}</span><strong id="profile-dialog-title">{t(section === "profile" ? "个人资料" : "我的 Agent")}</strong><p>{t(section === "profile" ? "管理你的名片信息。" : "集中管理自己的 Agent、可见范围和可共享 Skills。")}</p>{canManageHub && section === "profile" && <button className="profile-hub-admin-link" type="button" onClick={onManageHub}>{t("管理 Hub 账号权限")}</button>}</div>
        <button type="button" onClick={onClose} aria-label={t("关闭个人资料")}><X size={20} /></button>
      </header>
      <nav className="profile-dialog-tabs" aria-label={t("个人设置页面")}><button type="button" className={section === "profile" ? "is-active" : ""} onClick={() => setSection("profile")}>{t("个人资料")}</button><button type="button" className={section === "agents" ? "is-active" : ""} onClick={() => setSection("agents")}>{t("我的 Agent")} <span>{agents.filter((agent) => agent.can_manage ?? agent.owner_user_id === user.id).length}</span></button></nav>
      {section === "profile" ? <form className="profile-dialog-form" onSubmit={submit}><div className="profile-dialog-body">
        <section className="profile-identity">
          <button className="profile-avatar-editor" type="button" onClick={() => fileInputRef.current?.click()} aria-label={t("选择头像")}>
            {avatarURL ? <img src={avatarURL} alt={t("个人头像预览")} /> : <span>{initial(displayName)}</span>}
            <em><Camera size={16} /></em>
          </button>
          <div><strong>{displayName || t("未命名用户")}</strong><span>@{user.username}</span><button type="button" onClick={() => fileInputRef.current?.click()}>{t("更换头像")}</button></div>
          <input ref={fileInputRef} type="file" accept="image/png,image/jpeg,image/webp" onChange={selectAvatar} hidden />
        </section>
        <label><span>{t("显示名称")}</span><input aria-label={t("显示名称")} value={displayName} maxLength={80} onChange={(event) => setDisplayName(event.target.value)} required /></label>
        <label><span>{t("个人简介")} <em>{t("选填，最多 500 字")}</em></span><textarea aria-label={t("个人简介")} value={bio} maxLength={500} rows={4} placeholder={t("例如：负责投资研究与工作流设计。")} onChange={(event) => setBio(event.target.value)} /></label>
        <label><span>{t("当前事项")} <em>{t("选填，最多 240 字")}</em></span><textarea aria-label={t("当前事项")} value={currentFocus} maxLength={240} rows={3} placeholder={t("例如：本周在推进 AgentCollab 局域网协作试运行。")} onChange={(event) => setCurrentFocus(event.target.value)} /></label>
        {error && <p className="form-error" role="alert">{error}</p>}
      </div><footer><span>{t("头像保存在此 Hub 的本地数据库，不会上传至第三方。")}</span><div><button type="button" onClick={onClose}>{t("取消")}</button><button className="primary-button" type="submit" disabled={saving}>{t(saving ? "保存中" : "保存资料")}</button></div></footer></form> : onUpdateAgentPolicy ? <MyAgentsSettings user={user} agents={agents} onUpdatePolicy={onUpdateAgentPolicy} onManageAgents={onManageAgents} /> : <div className="my-agents-empty"><strong>{t("Agent 管理暂不可用")}</strong></div>}
    </section>
  </div>;
}
