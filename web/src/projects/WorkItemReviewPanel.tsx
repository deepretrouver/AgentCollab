import { Check, GitBranch, WarningCircle, X } from "@phosphor-icons/react";
import type { WorkItemReview } from "../api/client";
import { useLocale } from "../i18n/locale";

interface WorkItemReviewPanelProps {
  review: WorkItemReview;
  merging?: boolean;
  onMerge?: () => Promise<void> | void;
  onClose: () => void;
}

const fileStatus: Record<string, string> = {
  A: "已新增",
  M: "已修改",
  D: "已删除",
  R: "已重命名",
};

export function WorkItemReviewPanel({ review, merging, onMerge, onClose }: WorkItemReviewPanelProps) {
  const { t } = useLocale();
  return <section className="work-review-panel" aria-label={`${t("审查分支")} ${review.branch}`}>
    <header>
      <span><GitBranch size={17} /></span>
      <div><strong>{t("合并前审查")}</strong><code>{review.branch}</code></div>
      <button type="button" onClick={onClose} aria-label={t("关闭改动审查")}><X size={15} /></button>
    </header>
    <div className="work-review-metrics">
      <span><strong>{review.file_count}</strong> {t("个文件")}</span>
      <span className="is-addition">+{review.additions}</span>
      <span className="is-deletion">−{review.deletions}</span>
    </div>
    <div className="work-review-files">
      {review.changed_files.map((file) => <div key={`${file.status}:${file.path}`}>
        <em className={`status-${file.status.toLocaleLowerCase()}`}>{t(fileStatus[file.status] ?? file.status)}</em>
        <code title={file.path}>{file.path}</code>
        <span><i>+{file.additions}</i><b>−{file.deletions}</b></span>
      </div>)}
    </div>
    {review.mergeable
      ? <footer><span><Check size={15} />{t("与主工作区没有检测到冲突")}</span>{onMerge && <button type="button" disabled={merging} onClick={() => void onMerge()}>{t(merging ? "正在合并" : "确认合并到主工作区")}</button>}</footer>
      : <p className="work-review-conflict"><WarningCircle size={16} weight="fill" />{t("分支与主工作区存在冲突。请更新任务工作区后重新提交审查。")}</p>}
  </section>;
}
