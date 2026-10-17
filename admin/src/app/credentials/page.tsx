"use client";

import React, { useCallback, useState } from "react";
import { ChevronLeft, ChevronRight, Fingerprint, Pencil, Search, ShieldOff, Trash2 } from "lucide-react";
import { useI18n } from "@/i18n/I18nProvider";
import { useSession } from "@/lib/session";
import { AUTH_OAUTH_MANAGE, AUTH_USERS_MANAGE, can } from "@/lib/permissions";
import { CREDENTIAL_ERROR_KEYS, describeCodedError } from "@/lib/errors";
import { useResource } from "@/lib/useResource";
import { formatStamp } from "@/lib/format";
import {
  deleteAdminCredential, deleteAdminOAuthToken, fetchAdminCredentials, fetchAdminOAuthGrants,
  fetchAdminOAuthTokens, revokeAdminCredential, revokeAdminOAuthGrant, updateAdminCredential,
  type AdminCredential, type AdminCredentialQuery, type AdminOAuthGrant, type AdminOAuthQuery,
  type AdminOAuthToken, type AdminPage,
} from "@/lib/endpoints";
import { CancelButton, ConfirmDialog, Modal } from "@/components/ui/Modal";
import {
  Chip, EmptyBlock, ErrorNotice, ForbiddenBlock, inputClass, labelClass, LoadingBlock,
  primaryButtonClass, RefreshButton, SectionHeader, StatusMessage,
} from "@/components/ui/blocks";

const EMPTY_KEYS: AdminPage<AdminCredential> = { items: [], total: 0, limit: 50, offset: 0, has_more: false };
const EMPTY_GRANTS: AdminPage<AdminOAuthGrant> = { ...EMPTY_KEYS, items: [] };
const EMPTY_TOKENS: AdminPage<AdminOAuthToken> = { ...EMPTY_KEYS, items: [] };
const INITIAL_KEY_QUERY: AdminCredentialQuery = { owner_id: "", kind: "", status: "", limit: 50, offset: 0 };
const INITIAL_OAUTH_QUERY: AdminOAuthQuery = { owner_id: "", client_id: "", status: "", limit: 50, offset: 0 };
const actionClass = "inline-flex items-center gap-1 rounded-control border border-line px-2 py-1.5 text-[11px] text-text-body hover:bg-surfaceHover disabled:opacity-40";
const thClass = "px-3 py-2.5 text-left text-[11px] font-medium text-text-muted";
const tdClass = "px-3 py-3 align-top text-xs text-text-body";

type Message = { kind: "ok" | "err"; text: string } | null;
type Tab = "keys" | "grants" | "tokens";

function OwnerCell({ userId, username }: { userId: string; username: string }) {
  return <><div className="font-medium text-text-strong">{username}</div><div className="mt-1 break-all font-mono text-[10px] text-text-faint">{userId}</div></>;
}

function ScopeList({ scopes }: { scopes: string[] }) {
  return <div className="flex max-w-sm flex-wrap gap-1">{scopes.map((scope) => <span key={scope} className="whitespace-nowrap rounded-chip bg-surfaceSubtle px-1.5 py-0.5 font-mono text-[10px]">{scope}</span>)}</div>;
}

function CredentialStatus({ status }: { status: AdminCredential["status"] | AdminOAuthToken["status"] }) {
  const { t } = useI18n();
  return <Chip tone={status === "revoked" ? "danger" : status === "active" ? "ok" : "warn"}>{t(`credentials.status.${status}`)}</Chip>;
}

function Pager({ total, limit, offset, hasMore, loading, onPage, onLimit }: {
  total: number; limit: number; offset: number; hasMore: boolean; loading: boolean;
  onPage: (offset: number) => void; onLimit: (limit: number) => void;
}) {
  const { t } = useI18n();
  return <div className="flex flex-wrap items-center justify-between gap-3 border-t border-line-subtle px-3 py-3 text-[11px] text-text-muted">
    <span>{t("credentials.pagination", { total, page: Math.floor(offset / limit) + 1 })}</span>
    <div className="flex flex-wrap items-center gap-2">
      <label className="flex items-center gap-2 whitespace-nowrap">{t("credentials.pageSize")}<select className={`${inputClass} w-auto`} value={limit} disabled={loading} onChange={(event) => onLimit(Number(event.target.value))}>{[25, 50, 100].map((size) => <option key={size} value={size}>{size}</option>)}</select></label>
      <button type="button" className={actionClass} disabled={loading || offset === 0} onClick={() => onPage(Math.max(0, offset - limit))}><ChevronLeft className="h-3.5 w-3.5" />{t("action.previous")}</button>
      <button type="button" className={actionClass} disabled={loading || !hasMore} onClick={() => onPage(offset + limit)}>{t("action.next")}<ChevronRight className="h-3.5 w-3.5" /></button>
    </div>
  </div>;
}

function FilterActions({ onReset, onReload, loading }: { onReset: () => void; onReload: () => void; loading: boolean }) {
  const { t } = useI18n();
  return <div className="flex items-center justify-between gap-2"><div className="flex items-center gap-2"><button type="submit" className={`${primaryButtonClass} inline-flex items-center gap-1.5`}><Search className="h-3.5 w-3.5" />{t("action.filter")}</button><CancelButton label={t("action.reset")} onClick={onReset} /></div><RefreshButton loading={loading} onClick={onReload} /></div>;
}

function KeysPanel() {
  const { t, locale } = useI18n();
  const [draft, setDraft] = useState(INITIAL_KEY_QUERY);
  const [query, setQuery] = useState(INITIAL_KEY_QUERY);
  const load = useCallback(() => fetchAdminCredentials(query), [query]);
  const result = useResource(load, EMPTY_KEYS, AUTH_USERS_MANAGE, CREDENTIAL_ERROR_KEYS);
  const [message, setMessage] = useState<Message>(null);
  const [editing, setEditing] = useState<AdminCredential | null>(null);
  const [form, setForm] = useState({ name: "", scopes: "" });
  const [editError, setEditError] = useState<string | null>(null);
  const [confirm, setConfirm] = useState<{ item: AdminCredential; action: "revoke" | "delete" } | null>(null);
  const [busy, setBusy] = useState(false);

  const describe = (error: unknown) => describeCodedError(error, t, AUTH_USERS_MANAGE, CREDENTIAL_ERROR_KEYS);
  const refresh = (mayRemove: boolean) => {
    if (mayRemove && result.data.items.length === 1 && (query.offset ?? 0) > 0) setQuery((prev) => ({ ...prev, offset: Math.max(0, (prev.offset ?? 0) - (prev.limit ?? 50)) }));
    else result.reload();
  };
  const save = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!editing) return;
    const name = form.name.trim();
    if (!name) { setEditError(t("credentials.nameRequired")); return; }
    const scopes = [...new Set(form.scopes.split(/[\s,]+/).filter(Boolean))];
    const patch: { name?: string; scopes?: string[] } = {};
    if (name !== editing.name) patch.name = name;
    if (JSON.stringify(scopes) !== JSON.stringify(editing.scopes)) patch.scopes = scopes;
    if (Object.keys(patch).length === 0) { setEditing(null); return; }
    setBusy(true); setEditError(null); setMessage(null);
    try { await updateAdminCredential(editing.id, patch); setEditing(null); setMessage({ kind: "ok", text: t("credentials.saved") }); refresh(false); }
    catch (error) { setEditError(describe(error)); }
    finally { setBusy(false); }
  };
  const confirmAction = async () => {
    if (!confirm) return;
    setBusy(true); setMessage(null);
    try {
      if (confirm.action === "delete") await deleteAdminCredential(confirm.item.id);
      else await revokeAdminCredential(confirm.item.id);
      setMessage({ kind: "ok", text: t(confirm.action === "delete" ? "credentials.deleted" : "credentials.revoked") });
      refresh(confirm.action === "delete" || Boolean(query.status)); setConfirm(null);
    } catch (error) { setMessage({ kind: "err", text: describe(error) }); setConfirm(null); }
    finally { setBusy(false); }
  };

  return <div className="space-y-4">
    <form onSubmit={(event) => { event.preventDefault(); setQuery({ ...draft, owner_id: draft.owner_id?.trim(), limit: query.limit, offset: 0 }); }} className="space-y-3 rounded-card border border-line-subtle bg-surfaceSubtle p-4">
      <div className="grid gap-3 sm:grid-cols-3">
        <label className={labelClass}>{t("credentials.filter.owner")}<input className={`${inputClass} mt-1`} value={draft.owner_id} onChange={(event) => setDraft({ ...draft, owner_id: event.target.value })} /></label>
        <label className={labelClass}>{t("credentials.filter.kind")}<select className={`${inputClass} mt-1`} value={draft.kind} onChange={(event) => setDraft({ ...draft, kind: event.target.value as AdminCredentialQuery["kind"] })}><option value="">{t("credentials.filter.all")}</option>{["pat", "mcp"].map((kind) => <option key={kind} value={kind}>{t(`credentials.kind.${kind}`)}</option>)}</select></label>
        <label className={labelClass}>{t("credentials.filter.status")}<select className={`${inputClass} mt-1`} value={draft.status} onChange={(event) => setDraft({ ...draft, status: event.target.value as AdminCredentialQuery["status"] })}><option value="">{t("credentials.filter.all")}</option>{["active", "revoked", "expired", "disabled"].map((status) => <option key={status} value={status}>{t(`credentials.status.${status}`)}</option>)}</select></label>
      </div>
      <FilterActions loading={result.loading} onReload={result.reload} onReset={() => { setDraft(INITIAL_KEY_QUERY); setQuery(INITIAL_KEY_QUERY); }} />
    </form>
    {message ? <StatusMessage {...message} /> : null}
    {result.error ? <ErrorNotice message={result.error} onRetry={result.reload} /> : null}
    {result.loading ? <LoadingBlock /> : !result.error ? <div className="overflow-hidden rounded-card border border-line-subtle bg-surface">
      {result.data.items.length ? <div className="overflow-x-auto"><table className="w-full min-w-[800px]"><thead className="bg-surfaceSubtle"><tr>{["owner", "credential", "scopes", "activity", "actions"].map((column) => <th key={column} scope="col" className={thClass}>{t(`credentials.col.${column}`)}</th>)}</tr></thead><tbody className="divide-y divide-line-subtle">{result.data.items.map((item) => <tr key={item.id}>
        <td className={tdClass}><OwnerCell userId={item.user_id} username={item.username} /></td>
        <td className={tdClass}><div className="font-medium text-text-strong">{item.name}</div><div className="mt-1 font-mono text-[10px] text-text-faint">{item.token_prefix}…</div><div className="mt-2 flex flex-wrap gap-1"><Chip>{t(`credentials.kind.${item.kind}`)}</Chip><CredentialStatus status={item.status} /></div></td>
        <td className={tdClass}><ScopeList scopes={item.scopes} /></td>
        <td className={`${tdClass} min-w-44 space-y-1 text-[11px]`}><div>{t("credentials.created", { stamp: formatStamp(item.created_at, locale) })}</div><div>{item.expires_at ? t("credentials.expires", { stamp: formatStamp(item.expires_at, locale) }) : t("credentials.noExpiry")}</div><div>{item.last_used_at ? t("credentials.lastUsed", { stamp: formatStamp(item.last_used_at, locale) }) : t("credentials.neverUsed")}</div>{item.revoked_at ? <div>{t("credentials.revokedAt", { stamp: formatStamp(item.revoked_at, locale) })}</div> : null}</td>
        <td className={tdClass}><div className="flex flex-wrap gap-1.5"><button type="button" className={actionClass} disabled={busy} onClick={() => { setEditing(item); setForm({ name: item.name, scopes: item.scopes.join("\n") }); setEditError(null); }}><Pencil className="h-3 w-3" />{t("action.edit")}</button><button type="button" className={actionClass} disabled={busy || Boolean(item.revoked_at)} onClick={() => setConfirm({ item, action: "revoke" })}><ShieldOff className="h-3 w-3" />{t("credentials.revoke")}</button><button type="button" className={`${actionClass} text-rose-400`} disabled={busy} onClick={() => setConfirm({ item, action: "delete" })}><Trash2 className="h-3 w-3" />{t("action.delete")}</button></div></td>
      </tr>)}</tbody></table></div> : <EmptyBlock />}
      <Pager total={result.data.total} limit={query.limit ?? 50} offset={query.offset ?? 0} hasMore={result.data.has_more} loading={result.loading} onPage={(offset) => setQuery({ ...query, offset })} onLimit={(limit) => setQuery({ ...query, limit, offset: 0 })} />
    </div> : null}
    <Modal open={editing !== null} title={t("credentials.editTitle")} onClose={() => setEditing(null)} busy={busy}>
      <form onSubmit={save} className="space-y-4">
        {editing ? <OwnerCell userId={editing.user_id} username={editing.username} /> : null}
        <label className={labelClass}>{t("credentials.field.name")}<input required maxLength={64} value={form.name} disabled={busy} onChange={(event) => setForm({ ...form, name: event.target.value })} className={`${inputClass} mt-1`} /></label>
        <label className={labelClass}>{t("credentials.field.scopes")}<textarea rows={7} value={form.scopes} disabled={busy} onChange={(event) => setForm({ ...form, scopes: event.target.value })} className={`${inputClass} mt-1 font-mono`} /><span className="mt-1 block text-text-faint">{t("credentials.scopesHint")}</span></label>
        {editError ? <StatusMessage kind="err" text={editError} /> : null}
        <div className="flex justify-end gap-2"><CancelButton disabled={busy} onClick={() => setEditing(null)} label={t("action.cancel")} /><button type="submit" className={primaryButtonClass} disabled={busy}>{t(busy ? "action.saving" : "action.save")}</button></div>
      </form>
    </Modal>
    <ConfirmDialog open={confirm !== null} title={t(confirm?.action === "delete" ? "credentials.deleteTitle" : "credentials.revokeTitle")} message={t(confirm?.action === "delete" ? "credentials.deleteConfirm" : "credentials.revokeConfirm", { name: confirm?.item.name ?? "" })} confirmLabel={t(confirm?.action === "delete" ? "action.delete" : "credentials.revoke")} busy={busy} onClose={() => setConfirm(null)} onConfirm={() => void confirmAction()} />
  </div>;
}

function OAuthPanel({ view }: { view: "grants" | "tokens" }) {
  const { t, locale } = useI18n();
  const [draft, setDraft] = useState(INITIAL_OAUTH_QUERY);
  const [query, setQuery] = useState(INITIAL_OAUTH_QUERY);
  const load = useCallback<() => Promise<AdminPage<AdminOAuthGrant | AdminOAuthToken>>>(() => view === "grants" ? fetchAdminOAuthGrants(query) : fetchAdminOAuthTokens(query), [query, view]);
  const result = useResource(load, view === "grants" ? EMPTY_GRANTS : EMPTY_TOKENS, AUTH_OAUTH_MANAGE, CREDENTIAL_ERROR_KEYS);
  const [message, setMessage] = useState<Message>(null);
  const [confirm, setConfirm] = useState<AdminOAuthGrant | AdminOAuthToken | null>(null);
  const [busy, setBusy] = useState(false);
  const isGrants = view === "grants";

  const revoke = async () => {
    if (!confirm) return;
    setBusy(true); setMessage(null);
    try {
      if ("id" in confirm) await deleteAdminOAuthToken(confirm.id);
      else await revokeAdminOAuthGrant(confirm.user_id, confirm.client_id);
      setMessage({ kind: "ok", text: t(isGrants ? "credentials.grantRevoked" : "credentials.tokenDeleted") });
      setConfirm(null);
      if (!isGrants && result.data.items.length === 1 && (query.offset ?? 0) > 0) setQuery((prev) => ({ ...prev, offset: Math.max(0, (prev.offset ?? 0) - (prev.limit ?? 50)) }));
      else result.reload();
    } catch (error) { setMessage({ kind: "err", text: describeCodedError(error, t, AUTH_OAUTH_MANAGE, CREDENTIAL_ERROR_KEYS) }); setConfirm(null); }
    finally { setBusy(false); }
  };

  return <div className="space-y-4">
    <form onSubmit={(event) => { event.preventDefault(); setQuery({ ...draft, owner_id: draft.owner_id?.trim(), client_id: draft.client_id?.trim(), limit: query.limit, offset: 0 }); }} className="space-y-3 rounded-card border border-line-subtle bg-surfaceSubtle p-4">
      <div className={`grid gap-3 ${isGrants ? "sm:grid-cols-2" : "sm:grid-cols-3"}`}>
        <label className={labelClass}>{t("credentials.filter.owner")}<input className={`${inputClass} mt-1`} value={draft.owner_id} onChange={(event) => setDraft({ ...draft, owner_id: event.target.value })} /></label>
        <label className={labelClass}>{t("credentials.filter.client")}<input className={`${inputClass} mt-1`} value={draft.client_id} onChange={(event) => setDraft({ ...draft, client_id: event.target.value })} /></label>
        {!isGrants ? <label className={labelClass}>{t("credentials.filter.status")}<select className={`${inputClass} mt-1`} value={draft.status} onChange={(event) => setDraft({ ...draft, status: event.target.value as AdminOAuthQuery["status"] })}><option value="">{t("credentials.filter.all")}</option>{["active", "expired", "disabled"].map((status) => <option key={status} value={status}>{t(`credentials.status.${status}`)}</option>)}</select></label> : null}
      </div>
      <FilterActions loading={result.loading} onReload={result.reload} onReset={() => { setDraft(INITIAL_OAUTH_QUERY); setQuery(INITIAL_OAUTH_QUERY); }} />
    </form>
    {message ? <StatusMessage {...message} /> : null}
    {result.error ? <ErrorNotice message={result.error} onRetry={result.reload} /> : null}
    {result.loading ? <LoadingBlock /> : !result.error ? <div className="overflow-hidden rounded-card border border-line-subtle bg-surface">
      {result.data.items.length ? <div className="overflow-x-auto"><table className="w-full min-w-[740px]"><thead className="bg-surfaceSubtle"><tr>{["owner", "client", "scopes", ...(isGrants ? ["tokens"] : []), "activity", "actions"].map((column) => <th key={column} scope="col" className={thClass}>{t(`credentials.col.${column}`)}</th>)}</tr></thead><tbody className="divide-y divide-line-subtle">{result.data.items.map((item) => <tr key={"id" in item ? item.id : `${item.user_id}:${item.client_id}`}>
        <td className={tdClass}><OwnerCell userId={item.user_id} username={item.username} /></td>
        <td className={tdClass}><div className="font-medium text-text-strong">{item.client_name}</div><div className="mt-1 break-all font-mono text-[10px] text-text-faint">{item.client_id}</div><div className="mt-2">{"id" in item ? <CredentialStatus status={item.status} /> : <Chip tone={item.active ? "ok" : "neutral"}>{t(item.active ? "credentials.status.active" : "credentials.status.inactive")}</Chip>}</div></td>
        <td className={tdClass}><ScopeList scopes={"scope" in item ? item.scope.split(/\s+/).filter(Boolean) : item.scopes} /></td>
        {"token_count" in item ? <td className={`${tdClass} font-mono`}>{item.token_count}</td> : null}
        <td className={`${tdClass} min-w-44 space-y-1 text-[11px]`}>{"granted_at" in item ? <><div>{t("credentials.granted", { stamp: formatStamp(item.granted_at ?? undefined, locale) })}</div><div>{t("credentials.updated", { stamp: formatStamp(item.updated_at ?? undefined, locale) })}</div>{item.expires_at ? <div>{t("credentials.lastExpiry", { stamp: formatStamp(item.expires_at, locale) })}</div> : null}</> : <><div>{t("credentials.created", { stamp: formatStamp(item.created_at ?? undefined, locale) })}</div><div>{t("credentials.expires", { stamp: formatStamp(item.expires_at, locale) })}</div></>}</td>
        <td className={tdClass}><button type="button" className={`${actionClass} text-rose-400`} disabled={busy} onClick={() => setConfirm(item)}><ShieldOff className="h-3 w-3" />{t(isGrants ? "credentials.revoke" : "credentials.tokenRevoke")}</button></td>
      </tr>)}</tbody></table></div> : <EmptyBlock />}
      <Pager total={result.data.total} limit={query.limit ?? 50} offset={query.offset ?? 0} hasMore={result.data.has_more} loading={result.loading} onPage={(offset) => setQuery({ ...query, offset })} onLimit={(limit) => setQuery({ ...query, limit, offset: 0 })} />
    </div> : null}
    <ConfirmDialog open={confirm !== null} title={t(isGrants ? "credentials.grantRevokeTitle" : "credentials.tokenDeleteTitle")} message={t(isGrants ? "credentials.grantRevokeConfirm" : "credentials.tokenDeleteConfirm", { user: confirm?.username ?? "", client: confirm?.client_name ?? "" })} confirmLabel={t(isGrants ? "credentials.revoke" : "credentials.tokenRevoke")} busy={busy} onClose={() => setConfirm(null)} onConfirm={() => void revoke()} />
  </div>;
}

export default function CredentialsPage() {
  const { t } = useI18n();
  const { user } = useSession();
  const keysAllowed = can(user, AUTH_USERS_MANAGE);
  const oauthAllowed = can(user, AUTH_OAUTH_MANAGE);
  const tabs: Tab[] = [...(keysAllowed ? ["keys" as const] : []), ...(oauthAllowed ? ["grants" as const, "tokens" as const] : [])];
  const [selected, setSelected] = useState<Tab>(keysAllowed ? "keys" : "grants");
  const current = tabs.includes(selected) ? selected : tabs[0];
  if (!current) return <ForbiddenBlock permission={`${AUTH_USERS_MANAGE} / ${AUTH_OAUTH_MANAGE}`} />;
  const changeTabByKey = (event: React.KeyboardEvent, index: number) => {
    let next = index;
    if (event.key === "ArrowRight") next = (index + 1) % tabs.length;
    else if (event.key === "ArrowLeft") next = (index + tabs.length - 1) % tabs.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = tabs.length - 1;
    else return;
    event.preventDefault();
    setSelected(tabs[next]!);
    document.getElementById(`credential-tab-${tabs[next]}`)?.focus();
  };
  return <div className="space-y-4">
    <SectionHeader icon={<Fingerprint className="h-4 w-4 text-primary" />} title={t("credentials.title")} desc={t("credentials.desc")} />
    <div className="flex flex-wrap gap-1 rounded-card border border-line-subtle bg-surfaceSubtle p-1" role="tablist" aria-label={t("credentials.title")}>{tabs.map((tab, index) => <button key={tab} id={`credential-tab-${tab}`} type="button" role="tab" tabIndex={current === tab ? 0 : -1} aria-selected={current === tab} aria-controls={`credential-panel-${tab}`} onKeyDown={(event) => changeTabByKey(event, index)} onClick={() => setSelected(tab)} className={`rounded-control px-3 py-2 text-xs ${current === tab ? "bg-primary/15 font-semibold text-primary" : "text-text-muted hover:bg-surfaceHover"}`}>{t(`credentials.tab.${tab}`)}</button>)}</div>
    <div role="tabpanel" id={`credential-panel-${current}`} aria-labelledby={`credential-tab-${current}`} key={current}>{current === "keys" ? <KeysPanel /> : <OAuthPanel view={current} />}</div>
  </div>;
}
