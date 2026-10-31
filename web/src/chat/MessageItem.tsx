import { ChatsCircle, DownloadSimple, FileText, Robot, UserCircle } from "@phosphor-icons/react";
import type { Message } from "../api/client";
import { parseBrainstormCommand } from "./conversation";
import { useLocale } from "../i18n/locale";

interface MessageItemProps {
  message: Message;
  senderName: string;
  senderType?: string;
  own: boolean;
  variant?: "standard" | "topic-root" | "topic-reply";
  taskTitle?: string;
}

export function MessageItem({ message, senderName, senderType, own, variant = "standard", taskTitle }: MessageItemProps) {
  const { t, locale } = useLocale();
  const isAgent = message.sender_kind === "agent";
  const time = new Intl.DateTimeFormat(locale, { hour: "2-digit", minute: "2-digit" }).format(new Date(message.created_at));
  const [content, receipt] = message.body.split("\n\n执行凭证\n", 2);
  const deliveryMatch = content.match(/^(交付|任务失败)：([^\n]+)\n\n([\s\S]*)$/);
  const visibleBody = deliveryMatch?.[3] ?? content;
  const deliveryTitle = deliveryMatch?.[2];
  const failed = deliveryMatch?.[1] === "任务失败";
  const brainstorm = message.sender_kind === "user" ? parseBrainstormCommand(visibleBody) : undefined;
  const structuredTaskTitle = typeof message.metadata?.task_title === "string" ? message.metadata.task_title : undefined;
  const topicReplyLabel = failed ? t("执行失败") : taskTitle ? t("任务交付：{0}").replace("{0}", () => taskTitle) : t("任务交付");
  return (
    <article className={`message-item message-${variant}${own ? " message-own" : ""}${isAgent ? " message-agent" : ""}`}>
      <div className="message-avatar" aria-hidden="true">
        {isAgent ? <Robot size={18} weight="fill" /> : <UserCircle size={19} weight="fill" />}
      </div>
      <div className="message-content">
        <header className="message-meta">
          <strong>{senderName}</strong>
          <span className={`identity-label ${isAgent ? "identity-agent" : "identity-human"}`}>
            {isAgent ? senderType ?? "Agent" : t("真人")}
          </span>
          <time dateTime={message.created_at}>{time}</time>
        </header>
        {structuredTaskTitle && <div className="message-task-label"><span>{t("任务")}</span><strong>{structuredTaskTitle}</strong></div>}
        {deliveryTitle && <div className={`delivery-label${failed ? " delivery-failed" : ""}`}>{variant === "topic-reply" ? topicReplyLabel : failed ? t("执行失败") : t("Agent 交付")}{variant !== "topic-reply" && <span>{deliveryTitle}</span>}</div>}
        {brainstorm ? <div className="brainstorm-message"><span><ChatsCircle size={17} weight="fill" />{t("专题讨论")}</span><strong>{brainstorm.topic}</strong><small>{t("主协调开题 · 全员独立分析 · 完成后统一汇总")}</small></div> : <p>{visibleBody}</p>}
        {(message.attachments?.length ?? 0) > 0 && <div className="message-attachments" aria-label={t("消息附件")}>{message.attachments?.map((attachment) => <a key={attachment.id} href={attachment.download_url} download={attachment.filename}><FileText size={18} /><span><strong>{attachment.filename}</strong><small>{formatAttachmentSize(attachment.size_bytes)} · {attachment.content_type}</small></span><DownloadSimple size={17} /></a>)}</div>}
        {receipt && <details className="execution-receipt"><summary>{t("查看执行凭证")}</summary><pre>{receipt}</pre></details>}
      </div>
    </article>
  );
}

function formatAttachmentSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
