"use client";

// 各面板共用的呈现原语。取数失败一律用 ErrorNotice 如实显示并给重试，
// 权限不足用它而不是"空列表"——空列表会被读成"这里真的没有数据"。

import React from "react";
import { AlertTriangle, Inbox, Loader2, RefreshCw, ShieldAlert } from "lucide-react";
import { useI18n } from "@/i18n/I18nProvider";

export function SectionHeader({
  icon,
  title,
  actions,
}: {
  icon: React.ReactNode;
  title: string;
  actions?: React.ReactNode;
}) {
  return (
    <div className="flex min-h-8 flex-wrap items-center justify-between gap-2">
      <div className="min-w-0">
        <h2 className="text-sm font-semibold text-text-strong flex items-center gap-2">
          {icon}
          <span>{title}</span>
        </h2>
      </div>
      {actions ? <div className="flex items-center gap-2 shrink-0">{actions}</div> : null}
    </div>
  );
}

export function RefreshButton({ onClick, loading }: { onClick: () => void; loading: boolean }) {
  const { t } = useI18n();
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={loading}
      title={t("action.refresh")}
      aria-label={t("action.refresh")}
      className="p-2 rounded-control bg-surfaceSubtle hover:bg-surfaceHover border border-line text-text-body transition-colors duration-fast ease-soft cursor-pointer disabled:opacity-50"
    >
      <RefreshCw className={`w-4 h-4 ${loading ? "animate-spin text-primary" : ""}`} />
    </button>
  );
}

export function ErrorNotice({
  message,
  onRetry,
  permissionHint,
}: {
  message: string;
  onRetry?: () => void;
  /** 中立的"本块需要什么权限"说明：无论 403 还是上游不可用都成立。 */
  permissionHint?: string;
}) {
  const { t } = useI18n();
  return (
    <div className="p-3 rounded-card bg-danger/10 border border-danger/30 space-y-2">
      <div className="flex items-start gap-2 text-danger text-xs leading-relaxed">
        <AlertTriangle className="w-3.5 h-3.5 shrink-0 mt-0.5" />
        <span>{message}</span>
      </div>
      {permissionHint ? <p className="text-[11px] text-danger leading-relaxed">{permissionHint}</p> : null}
      {onRetry ? (
        <button
          type="button"
          onClick={onRetry}
          className="px-2.5 py-1 rounded-control bg-danger/20 hover:bg-danger/30 text-danger text-[11px] font-medium transition-colors duration-fast ease-soft cursor-pointer"
        >
          {t("action.retry")}
        </button>
      ) : null}
    </div>
  );
}

/** 整块降级：当前身份没有进入这一块的权限码。页面不崩，只有本块换成这张卡。 */
export function ForbiddenBlock({ permission }: { permission: string }) {
  const { t } = useI18n();
  return (
    <div className="p-6 rounded-card bg-warn/[0.06] border border-warn/25 space-y-2">
      <div className="flex items-center gap-2 text-warn text-sm font-semibold">
        <ShieldAlert className="w-4 h-4 shrink-0" />
        <span>{t("permission.deniedTitle")}</span>
      </div>
      <p className="text-[11px] text-warn leading-relaxed">{t("permission.deniedDesc", { code: permission })}</p>
    </div>
  );
}

export function LoadingBlock() {
  const { t } = useI18n();
  return (
    <div className="py-6 text-center text-xs text-text-faint flex items-center justify-center gap-2">
      <Loader2 className="w-4 h-4 animate-spin text-primary" />
      <span>{t("state.loading")}</span>
    </div>
  );
}

export function EmptyBlock({ text }: { text?: string }) {
  const { t } = useI18n();
  return (
    <div className="py-6 border-y border-line-subtle text-center text-xs text-text-faint flex items-center justify-center gap-2">
      <Inbox className="w-4 h-4" />
      <span>{text ?? t("state.empty")}</span>
    </div>
  );
}

export function StatusMessage({ kind, text }: { kind: "ok" | "err"; text: string }) {
  return (
    <div
      role="status"
      className={`p-2.5 rounded-control text-[11px] font-mono leading-relaxed ${
        kind === "ok" ? "bg-success/10 text-success" : "bg-danger/10 text-danger"
      }`}
    >
      {text}
    </div>
  );
}

export function Chip({ children, tone = "neutral" }: { children: React.ReactNode; tone?: "neutral" | "ok" | "warn" | "danger" }) {
  const tones: Record<string, string> = {
    neutral: "bg-surfaceSubtle text-text-muted border-line",
    ok: "bg-success/10 text-success border-success/30",
    warn: "bg-warn/15 text-warn border-warn/30",
    danger: "bg-danger/15 text-danger border-danger/30",
  };
  return (
    <span className={`inline-flex items-center px-1.5 py-0.5 rounded-chip border text-[10px] font-mono ${tones[tone]}`}>
      {children}
    </span>
  );
}

export const inputClass =
  "w-full px-2.5 py-1.5 rounded-control bg-surfaceSubtle border border-line text-xs text-text-strong placeholder:text-text-faint focus:border-primary outline-none";
export const labelClass = "block text-[11px] font-mono text-text-body font-medium mb-1";
export const primaryButtonClass =
  "px-4 py-1.5 rounded-control bg-primary hover:bg-primary-hover text-primary-foreground text-xs font-semibold transition-colors duration-fast ease-soft disabled:opacity-50 cursor-pointer";
