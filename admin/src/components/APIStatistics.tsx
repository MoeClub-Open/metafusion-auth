"use client";

import { useCallback, useState } from "react";
import { Activity } from "lucide-react";
import { useI18n } from "@/i18n/I18nProvider";
import { getJson } from "@/lib/api";
import { useResource } from "@/lib/useResource";
import { AUTH_AUDIT_READ } from "@/lib/permissions";
import { AUDIT_SERVICES } from "@/lib/endpoints";
import { LogCleanup } from "./LogCleanup";
import { EmptyBlock, ErrorNotice, inputClass, labelClass, LoadingBlock, primaryButtonClass, RefreshButton, SectionHeader } from "@/components/ui/blocks";

interface Statistic { day: string; service: string; method: string; route: string; status: number; requests: number; total_ms: number; max_ms: number }
const EMPTY = { items: [] as Statistic[] };
const buttonClass = "rounded-control border border-line px-3 py-2 text-xs text-text-body hover:bg-surfaceSubtle disabled:opacity-40";
const date = (offset: number) => new Date(Date.now() + offset * 86400000).toISOString().slice(0, 10);

export function APIStatistics() {
  const { t, locale } = useI18n();
  const [draft, setDraft] = useState(() => ({ from: date(-6), to: date(0), service: "" }));
  const [query, setQuery] = useState(draft);
  const [page, setPage] = useState(1);
  const load = useCallback(() => getJson<typeof EMPTY>(`/api/admin/api-statistics?${new URLSearchParams(query)}`), [query]);
  const stats = useResource(load, EMPTY, AUTH_AUDIT_READ);
  const total = stats.data.items.reduce((out, row) => ({ requests: out.requests + row.requests, errors: out.errors + (row.status >= 400 ? row.requests : 0), ms: out.ms + row.total_ms }), { requests: 0, errors: 0, ms: 0 });
  const pages = Math.max(1, Math.ceil(stats.data.items.length / 50));
  const format = (value: number) => value.toLocaleString(locale, { maximumFractionDigits: 1 });
  return <div className="space-y-4">
    <SectionHeader icon={<Activity className="size-4 text-primary" />} title={t("logs.statistics")} desc={t("logs.statisticsHint")} actions={<RefreshButton onClick={stats.reload} loading={stats.loading} />} />
    <form className="flex flex-wrap items-end gap-3" onSubmit={(event) => { event.preventDefault(); setQuery(draft); setPage(1); }}>
      <label className={labelClass}>{t("logs.from")}<input type="date" required value={draft.from} max={draft.to} onChange={(event) => setDraft({ ...draft, from: event.target.value })} className={inputClass + " mt-1 block"} /></label>
      <label className={labelClass}>{t("logs.to")}<input type="date" required value={draft.to} min={draft.from} max={date(0)} onChange={(event) => setDraft({ ...draft, to: event.target.value })} className={inputClass + " mt-1 block"} /></label>
      <label className={labelClass}>{t("audit.filter.service")}<select value={draft.service} onChange={(event) => setDraft({ ...draft, service: event.target.value })} className={inputClass + " mt-1 block"}>
        <option value="">{t("logs.allServices")}</option>{AUDIT_SERVICES.map((value) => <option key={value} value={value}>{value}</option>)}
      </select></label>
      <button className={primaryButtonClass} type="submit">{t("audit.apply")}</button>
    </form>
    {stats.error && <ErrorNotice message={stats.error} onRetry={stats.reload} />}
    {stats.loading ? <LoadingBlock /> : !stats.error && <>
      <dl className="flex flex-wrap gap-x-8 gap-y-3 border-y border-line-subtle py-3 text-xs">
        <div><dt className="text-text-muted">{t("logs.requests")}</dt><dd className="mt-1 text-lg font-semibold text-text-strong">{format(total.requests)}</dd></div>
        <div><dt className="text-text-muted">{t("logs.errors")}</dt><dd className="mt-1 text-lg font-semibold text-text-strong">{format(total.errors)}</dd></div>
        <div><dt className="text-text-muted">{t("logs.average")}</dt><dd className="mt-1 text-lg font-semibold text-text-strong">{format(total.requests ? total.ms / total.requests : 0)} ms</dd></div>
      </dl>
      {stats.data.items.length === 0 ? <EmptyBlock text={t("audit.empty")} /> : <div className="overflow-x-auto rounded-card border border-line-subtle">
        <table className="w-full text-left text-xs"><thead className="bg-surfaceSubtle text-text-muted"><tr>
          {["logs.day", "audit.col.service", "logs.route", "logs.status", "logs.requests", "logs.average", "logs.maximum"].map((key) => <th key={key} className="whitespace-nowrap px-3 py-2.5 font-medium">{t(key)}</th>)}
        </tr></thead><tbody className="divide-y divide-line-subtle">{stats.data.items.slice((page - 1) * 50, page * 50).map((row) => <tr key={`${row.day}:${row.service}:${row.method}:${row.route}:${row.status}`}>
          <td className="whitespace-nowrap px-3 py-2.5">{row.day}</td><td className="px-3 py-2.5">{row.service}</td><td className="min-w-48 break-all px-3 py-2.5 font-mono">{row.method} {row.route}</td><td className="px-3 py-2.5">{row.status}</td><td className="px-3 py-2.5">{format(row.requests)}</td><td className="whitespace-nowrap px-3 py-2.5">{format(row.total_ms / row.requests)} ms</td><td className="whitespace-nowrap px-3 py-2.5">{format(row.max_ms)} ms</td>
        </tr>)}</tbody></table>
      </div>}
      {pages > 1 && <div className="flex items-center justify-end gap-3 text-xs"><button className={buttonClass} disabled={page === 1} onClick={() => setPage(page - 1)}>{t("audit.prev")}</button><span>{page} / {pages}</span><button className={buttonClass} disabled={page >= pages} onClick={() => setPage(page + 1)}>{t("audit.next")}</button></div>}
    </>}
    <LogCleanup kind="statistics" onCleaned={() => { setPage(1); stats.reload(); }} />
  </div>;
}
