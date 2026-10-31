import { useState, type FormEvent } from "react";
import { useLocale } from "../i18n/locale";
import { ArrowRight, CirclesFour } from "@phosphor-icons/react";

const LOGIN_HISTORY_KEY = "agentcollab.login-history";
const MAX_LOGIN_HISTORY = 5;

function readLoginHistory() {
  try {
    const raw = window.localStorage.getItem(`${LOGIN_HISTORY_KEY}:${window.location.origin}`);
    const parsed = raw ? JSON.parse(raw) : [];
    return Array.isArray(parsed)
      ? parsed.filter((value): value is string => typeof value === "string" && value.trim().length > 0).slice(0, MAX_LOGIN_HISTORY)
      : [];
  } catch {
    return [];
  }
}

function writeLoginHistory(username: string, history: string[]) {
  try {
    window.localStorage.setItem(`${LOGIN_HISTORY_KEY}:${window.location.origin}`, JSON.stringify([username, ...history.filter((item) => item !== username)].slice(0, MAX_LOGIN_HISTORY)));
  } catch {
    // Private browsing or a locked-down embedded client may deny localStorage.
  }
}

interface LoginPageProps {
  inviteCode?: string;
  onLogin: (username: string, password: string) => Promise<void>;
  onRegister?: (input: { username: string; display_name: string; password: string }) => Promise<void>;
}

export function LoginPage({ inviteCode = "", onLogin, onRegister }: LoginPageProps) {
  const { t, locale, setLocale } = useLocale();
  const [mode, setMode] = useState<"login" | "register">(inviteCode && onRegister ? "register" : "login");
  const [loginHistory, setLoginHistory] = useState<string[]>(readLoginHistory);
  const [username, setUsername] = useState(() => (inviteCode && onRegister ? "" : readLoginHistory()[0] ?? ""));
  const [password, setPassword] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [rememberUsername, setRememberUsername] = useState(true);
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const clientJoinURL = inviteCode ? `agentcollab://join?hub=${encodeURIComponent(window.location.origin)}&invite=${encodeURIComponent(inviteCode)}` : "";

  async function submit(event: FormEvent) {
    event.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      await onLogin(username, password);
      if (rememberUsername && username.trim()) {
        const nextHistory = [username, ...loginHistory.filter((item) => item !== username)].slice(0, MAX_LOGIN_HISTORY);
        setLoginHistory(nextHistory);
        writeLoginHistory(username, loginHistory);
      }
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("无法登录，请稍后重试"));
    } finally {
      setSubmitting(false);
    }
  }

  async function register(event: FormEvent) {
    event.preventDefault();
    if (!onRegister) return;
    setError("");
    setSubmitting(true);
    try {
      await onRegister({ username, display_name: displayName, password });
      if (rememberUsername && username.trim()) {
        const nextHistory = [username, ...loginHistory.filter((item) => item !== username)].slice(0, MAX_LOGIN_HISTORY);
        setLoginHistory(nextHistory);
        writeLoginHistory(username, loginHistory);
      }
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("无法加入群聊，请稍后重试"));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="login-page">
      <section className="login-context" aria-label={t("AgentCollab 介绍")}>
        <div className="brand-mark"><CirclesFour weight="fill" size={24} /></div>
        <div>
          <p className="login-brand">AgentCollab</p>
          <h1>{t("人和 Agent，")}<br />{t("在一个群里协作。")}</h1>
          <p className="login-description">{t("讨论、派发、执行和交付回到一条可追溯的时间线。")}</p>
        </div>
        <p className="login-storage">{t("自托管 Hub，数据保留在你的存储中。")}</p>
      </section>
      <section className="login-panel" aria-labelledby="login-title">
        <label className="language-picker">Language / 语言 <select aria-label="Language / 语言" value={locale} onChange={event => setLocale(event.target.value === "en" ? "en" : "zh-CN")}><option value="zh-CN">简体中文</option><option value="en">English</option></select></label>
        {inviteCode && !window.agentCollabDesktop && <div className="open-client-invite">
          <strong>{t("已安装 AgentCollab Client？")}</strong>
          <span>{t("在 Client 中打开后会自动记住团队地址，并可添加这台电脑上的 Agent。")}</span>
          <a href={clientJoinURL}>{t("在 Client 中打开")}</a>
          <a className="client-download-link" href="/downloads/client">{t("下载 Client")}</a>
        </div>}
        {onRegister && <div className="invite-login-switch" role="tablist" aria-label={inviteCode ? t("加入群聊方式") : t("账号操作")}>
          <button type="button" role="tab" aria-selected={mode === "register"} onClick={() => { setMode("register"); setError(""); }}>{t("创建新账号")}</button>
          <button type="button" role="tab" aria-selected={mode === "login"} onClick={() => { setMode("login"); setError(""); }}>{t("已有账号登录")}</button>
        </div>}
        <form className="login-form" onSubmit={mode === "register" ? register : submit}>
          <div>
            <p className="section-kicker">{inviteCode ? t("群聊邀请") : t("访问工作区")}</p>
            <h2 id="login-title">{mode === "register" ? (inviteCode ? t("加入群聊") : t("创建账号")) : t("登录")}</h2>
            <p className="form-intro">{mode === "register" ? (inviteCode ? t("创建你自己的真人账号。加入后，你能看到共享时间线、群成员和 Agent 的任务交付。") : t("创建你的个人账号。之后可通过邀请链接加入他人的工作区，或创建自己的工作区。")) : inviteCode ? t("登录你自己的账号，随后自动加入这个群聊。") : t("使用已有账号登录这个团队。")}</p>
          </div>
          {mode === "register" && <label>
            <span>{t("显示名称")}</span>
            <input
              autoComplete="name"
              autoFocus
              value={displayName}
              onChange={(event) => setDisplayName(event.target.value)}
              required
            />
          </label>}
          <label>
            <span>{mode === "register" ? t("新账号用户名") : t("用户名")}</span>
            <input
              name="username"
              autoComplete="username"
              autoFocus={mode === "login"}
              aria-describedby={mode === "register" ? "username-hint" : undefined}
              placeholder={mode === "register" ? t("例如：郭昱科") : undefined}
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              required
            />
          </label>
          {mode === "login" && loginHistory.length > 0 && <div className="login-account-history" aria-label={t("最近登录账号")}>
            <span>{t("最近登录")}</span>
            <div>
              {loginHistory.map((account) => <button key={account} type="button" onClick={() => setUsername(account)}>{account}</button>)}
              <button type="button" className="history-clear" onClick={() => {
                setLoginHistory([]);
                try { window.localStorage.removeItem(`${LOGIN_HISTORY_KEY}:${window.location.origin}`); } catch { /* ignore */ }
              }}>{t("清除")}</button>
            </div>
          </div>}
          {mode === "register" && <small id="username-hint" className="input-hint">{t("可使用中文、英文、数字和常用符号。")}</small>}
          <label>
            <span>{mode === "register" ? t("设置密码") : t("密码")}</span>
            <input
              name="password"
              type="password"
              autoComplete={mode === "register" ? "new-password" : "current-password"}
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              required
            />
          </label>
          {mode === "login" && <div className="login-options">
            <label className="checkbox-option">
              <input type="checkbox" checked={rememberUsername} onChange={(event) => setRememberUsername(event.target.checked)} />
              <span>{t("记住用户名")}</span>
            </label>
            <span className="password-manager-hint">{t("密码由浏览器或系统密码管理器保存")}</span>
          </div>}
          {error && <p className="form-error" role="alert">{error}</p>}
          <button className="primary-button login-submit" type="submit" disabled={submitting}>
            <span>{submitting ? (mode === "register" ? t("正在创建") : t("正在登录")) : (mode === "register" ? (inviteCode ? t("创建账号并加入") : t("创建账号")) : t("进入 AgentCollab"))}</span>
            <ArrowRight size={18} weight="bold" />
          </button>
        </form>
      </section>
    </main>
  );
}
