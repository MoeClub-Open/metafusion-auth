"use client";

import { useState } from "react";
import { useI18n } from "@/i18n/I18nProvider";
import { can } from "@/lib/permissions";
import { useSession } from "@/lib/session";
import { getJson, sendJson } from "@/lib/api";
import { describeApiError } from "@/lib/errors";
import { AUDIT_SERVICES } from "@/lib/endpoints";
import { ConfirmDialog } from "@/components/ui/Modal";
import { inputClass, labelClass, primaryButtonClass, StatusMessage } from "@/components/ui/blocks";

interface CleanupScope { kind: string; before: string; service: string }

export function LogCleanup({ kind, onCleaned }: { kind: "audit" | "statistics"; onCleaned: () => void }) {
  const { t } = useI18n();
  const { user } = useSession();
  const today = new Date().toISOString().slice(0, 10);
  const [before, setBefore] = useState(today);
  const [service, setService] = useState("");
  const [pending, setPending] = useState<(CleanupScope & { count: number }) | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [removed, setRemoved] = useState<number | null>(null);
  if (!can(user, "auth.audit.manage")) return null;

  const preview = async (event: React.FormEvent) => {
    event.preventDefault();
    const scope = { kind, before, service };
    setBusy(true); setError(null); setRemoved(null);
    try {
      const result = await getJson<{ count: number }>(`/api/admin/log-cleanup?${new URLSearchParams(scope)}`);
      setPending({ ...scope, count: result.count });
    } catch (error) { setError(describeApiError(error, t, "auth.audit.manage")); }
    finally { setBusy(false); }
  };
  const cleanup = async () => {
    if (!pending) return;
    setBusy(true); setError(null);
    try {
      const result = await sendJson<{ removed: number }>("/api/admin/log-cleanup", "POST", { kind: pending.kind, before: pending.before, service: pending.service });
      setRemoved(result.removed); setPending(null); onCleaned();
    } catch (error) { setPending(null); setError(describeApiError(error, t, "auth.audit.manage")); }
    finally { setBusy(false); }
  };
  return <details className="border-t border-line-subtle pt-4">
    <summary className="w-fit cursor-pointer text-xs font-medium text-text-muted">{t("logs.cleanup")}</summary>
    <form onSubmit={(event) => void preview(event)} className="mt-3 space-y-3">
      <p className="text-xs text-text-muted">{t("logs.cleanupHint")}</p>
      <div className="flex flex-wrap items-end gap-3">
        <label className={labelClass}>{t("logs.before")}<input type="date" required max={today} value={before} onChange={(event) => setBefore(event.target.value)} className={inputClass + " mt-1 block"} /></label>
        <label className={labelClass}>{t("audit.filter.service")}<select value={service} onChange={(event) => setService(event.target.value)} className={inputClass + " mt-1 block"}>
          <option value="">{t("logs.allServices")}</option>{AUDIT_SERVICES.map((value) => <option key={value} value={value}>{value}</option>)}
        </select></label>
        <button type="submit" disabled={busy || !before} className={primaryButtonClass + " disabled:opacity-50"}>{t("logs.preview")}</button>
      </div>
      {error && <StatusMessage kind="err" text={error} />}
      {removed !== null && <p role="status" className="text-xs text-success">{t("logs.cleaned", { count: removed })}</p>}
    </form>
    <ConfirmDialog open={pending !== null} busy={busy} title={t("logs.cleanup")} confirmLabel={t("logs.confirm")} onClose={() => setPending(null)} onConfirm={() => void cleanup()}
      message={pending ? t("logs.confirmMessage", { count: pending.count, kind: t(`logs.kind.${pending.kind}`), before: pending.before, service: pending.service || t("logs.allServices") }) : ""} />
  </details>;
}
