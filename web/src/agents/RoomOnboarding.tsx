import { ArrowRight, Cpu, PlugsConnected, UsersThree } from "@phosphor-icons/react";
import type { Agent } from "../api/client";
import { useLocale } from "../i18n/locale";

interface RoomOnboardingProps {
  registeredCount: number;
  agents: Agent[];
  verifying: boolean;
  verificationError: string;
  onManageAgents: () => void;
  onShowMembers: () => void;
  onVerify: () => Promise<void>;
}

export function RoomOnboarding({ registeredCount, agents, verifying, verificationError, onManageAgents, onShowMembers, onVerify }: RoomOnboardingProps) {
  const { t } = useLocale();
  if (agents.length === 0) {
    const hasRegisteredAgents = registeredCount > 0;
    return (
      <div className="room-onboarding">
        <div className="onboarding-mark"><PlugsConnected size={30} weight="duotone" /></div>
        <h2>{t(hasRegisteredAgents ? "这个群聊还没有 Agent 成员" : "先加入工作区 Agent 库")}</h2>
        <p>{hasRegisteredAgents ? t("工作区已注册 {0} 个 Agent。打开成员列表，选择加入当前群聊。").replaceAll("{0}", String(registeredCount)) : t("选择要带入团队的本机 Agent；需要安装并连接 Client，普通真人聊天无需安装。")}</p>
        <button className="onboarding-primary" type="button" onClick={hasRegisteredAgents ? onShowMembers : onManageAgents}>{t(hasRegisteredAgents ? "打开成员列表" : "添加我的 Agent")}<ArrowRight size={18} /></button>
        <div className="onboarding-evidence">
          <span><Cpu size={16} />{t("Agent 库与群成员分离")}</span>
          <span><UsersThree size={16} />{t("每个群聊独立选择成员")}</span>
        </div>
      </div>
    );
  }

  return (
    <div className="room-onboarding ready-onboarding">
      <div className="onboarding-mark"><UsersThree size={30} weight="duotone" /></div>
      <h2>{t(agents.length >= 2 ? "检查多 Agent 协作" : "再加入一个 Agent")}</h2>
      <p>{t(agents.length >= 2 ? "已加入不代表可执行。检查会向 Agent 提交真实验证任务，请查看各自的成功或失败结果。" : "加入至少两个 Agent 后可运行协作检查；也可以先点名单个 Agent 发送消息。")}</p>
      <div className="onboarding-actions">
        <button className="onboarding-primary" type="button" disabled={agents.length < 2 || verifying} onClick={() => void onVerify()}>{t(verifying ? "正在检查" : "检查 Agent 连接")}</button>
        <button className="onboarding-secondary" type="button" onClick={onShowMembers}>{t("查看成员列表")}</button>
      </div>
      {verificationError && <p className="onboarding-error" role="alert">{verificationError}</p>}
      <div className="onboarding-agent-strip">
        {agents.map((agent) => <span key={agent.id}><strong>{agent.display_name}</strong><small>{agent.device_name} · {agent.channel}</small></span>)}
      </div>
    </div>
  );
}
