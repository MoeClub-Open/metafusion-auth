"use client";

import { useTheme } from "@/lib/themeContext";
import { ThemePicker } from "./ThemePicker";

import React, { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import {
  ArrowLeft, FileClock, Fingerprint, KeyRound, LogOut, ServerCog,
  Shapes, ShieldCheck, Ticket, Users,
} from "lucide-react";
import { useI18n } from "@/i18n/I18nProvider";
import { useSession } from "@/lib/session";
import { canEnterAdmin, canEnterSection, type SectionKey } from "@/lib/permissions";
import { logout } from "@/lib/endpoints";
import { stripBasePath } from "@/lib/paths";
import { LocaleSwitcher } from "./LocaleSwitcher";
import { connectAdminWorkspace, prepareAdminNavigation, announceAdminNavigation, isAdminWorkspaceEmbedded } from "@/lib/adminWorkspace";
import { ThemeModeSwitcher } from "./ThemeModeSwitcher";
import { ErrorNotice, ForbiddenBlock, LoadingBlock } from "./ui/blocks";

const NAV_GROUPS: { labelKey: string; items: { key: SectionKey; href: string; icon: React.ReactNode }[] }[] = [
  { labelKey: "nav.groupIdentity", items: [
    { key: "users", href: "/", icon: <Users className="h-4 w-4" /> },
    { key: "credentials", href: "/credentials", icon: <Fingerprint className="h-4 w-4" /> },
    { key: "groups", href: "/groups", icon: <Shapes className="h-4 w-4" /> },
    { key: "invites", href: "/invites", icon: <Ticket className="h-4 w-4" /> },
  ] },
  { labelKey: "nav.groupSystem", items: [
    { key: "oauth", href: "/oauth-clients", icon: <KeyRound className="h-4 w-4" /> },
    { key: "audit", href: "/audit", icon: <FileClock className="h-4 w-4" /> },
    { key: "instance", href: "/instance", icon: <ServerCog className="h-4 w-4" /> },
  ] },
];

export function AppShell({ children }: { children: React.ReactNode }) {
  const { t, setLocale } = useI18n();
  const { setTheme } = useTheme();
  const [embedded, setEmbedded] = useState(false);
  useEffect(() => {
    const disconnect = connectAdminWorkspace(setLocale, setTheme);
    setEmbedded(disconnect !== null);
    return disconnect ?? undefined;
  }, [setLocale, setTheme]);
  const { status, user, error, reload } = useSession();
  const router = useRouter();
  const searchParams = useSearchParams();
  const pathname = stripBasePath(usePathname());
  const [signingOut, setSigningOut] = useState(false);

  const handleLogout = useCallback(async () => {
    setSigningOut(true);
    try { await logout(); } catch { /* 会话失效时仍返回主站。 */ }
    finally { (isAdminWorkspaceEmbedded() ? window.parent : window).location.href = "/"; }
  }, []);

  const visibleGroups = NAV_GROUPS.map((group) => ({
    ...group,
    items: group.items.filter((item) => canEnterSection(user, item.key)),
  })).filter((group) => group.items.length > 0);
  const visibleNav = visibleGroups.flatMap((group) => group.items);
  const activeHref = visibleNav.find((item) => item.href === "/" ? pathname === "/" : pathname.startsWith(item.href))?.href;
  const entryHref = visibleNav[0]?.href;
  useEffect(() => {
    if (status === "ready" && pathname === "/" && !activeHref && entryHref) router.replace(entryHref);
  }, [status, pathname, activeHref, entryHref, router]);

  useEffect(() => { announceAdminNavigation(); }, [pathname, searchParams]);

  return (
    <div data-mf-admin className="min-h-screen flex flex-col bg-background text-text-strong">
      <header hidden={embedded} className="sticky top-0 z-30 border-b border-line bg-surface/90 backdrop-blur">
        <div className="mx-auto flex h-12 w-full items-center justify-between gap-2 px-4 sm:px-6">
          <div className="flex min-w-0 flex-1 items-center gap-2">
            <a href="/admin" aria-label={t("app.backToHub")} title={t("app.backToHub")} className="inline-flex h-11 w-11 sm:w-auto shrink-0 items-center justify-center gap-1 text-xs text-text-muted hover:text-text-strong"><ArrowLeft className="h-4 w-4" /><span className="hidden sm:inline">{t("app.backToHub")}</span></a>
            <span className="hidden text-text-faint sm:inline">/</span>
            <div className="flex min-w-0 items-center gap-2 text-sm font-semibold text-text-strong"><ShieldCheck className="h-4 w-4 shrink-0 text-primary" /><span className="truncate">{t("app.title")}</span></div>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <LocaleSwitcher />
            <ThemeModeSwitcher />
            <ThemePicker />
            {user ? <span className="hidden max-w-[9rem] truncate rounded border border-primary/30 bg-primary/20 px-2 py-0.5 font-mono text-xs text-primary sm:inline-flex" title={user.username}>{user.username}</span> : null}
          </div>
        </div>
      </header>

      <div className="mx-auto flex w-full flex-1 flex-col gap-4 px-4 py-3 lg:flex-row">
        {status === "ready" && canEnterAdmin(user) && visibleNav.length > 0 ? (
          <aside className="w-full shrink-0 lg:w-[200px]">
            <div className="flex items-center gap-2 lg:hidden">
              <label htmlFor="account-admin-section" className="sr-only">{t("nav.section")}</label>
              <select id="account-admin-section" value={activeHref ?? ""} onChange={(e) => { prepareAdminNavigation("/admin/account" + e.target.value); if (embedded) router.replace(e.target.value); else router.push(e.target.value); }} className="min-w-0 flex-1 rounded-control border border-line bg-surface px-2.5 py-2 text-sm text-text-strong">
                {!activeHref ? <option value="" disabled>{t("nav.section")}</option> : null}
                {visibleGroups.map((group) => (
                  <optgroup key={group.labelKey} label={t(group.labelKey)}>
                    {group.items.map((item) => <option key={item.key} value={item.href}>{t(`nav.${item.key}`)}</option>)}
                  </optgroup>
                ))}
              </select>
              {user ? <button type="button" onClick={() => void handleLogout()} disabled={signingOut} className="inline-flex shrink-0 items-center gap-1 text-xs text-text-muted disabled:opacity-50"><LogOut className="h-4 w-4" />{t("session.logout")}</button> : null}
            </div>
            <nav aria-label={t("nav.section")} className={`hidden lg:block sticky ${embedded ? "top-3" : "top-16"} p-1`}>
              {visibleGroups.map((group) => (
                <div key={group.labelKey} className="py-1 first:pt-0 last:pb-0">
                  <div className="px-2 py-1 text-[11px] font-semibold tracking-wide text-text-faint">{t(group.labelKey)}</div>
                  {group.items.map((item) => {
                    const active = activeHref === item.href;
                    return <Link key={item.key} href={item.href} replace={embedded} onClick={(event) => prepareAdminNavigation(event.currentTarget.href, event)} aria-current={active ? "page" : undefined} className={`flex items-center gap-2 rounded-control px-2 py-1.5 text-[13px] transition-colors ${active ? "bg-primary/15 font-semibold text-primary" : "text-text-muted hover:bg-surfaceHover hover:text-text-strong"}`}>
                      {item.icon}<span>{t(`nav.${item.key}`)}</span>
                    </Link>;
                  })}
                </div>
              ))}
              {user ? <button type="button" onClick={() => void handleLogout()} disabled={signingOut} className="mt-2 flex w-full items-center gap-3 border-t border-line-subtle px-2 py-2 text-left text-xs text-text-muted hover:text-text-strong disabled:opacity-50"><LogOut className="h-4 w-4" />{t("session.logout")}</button> : null}
            </nav>
          </aside>
        ) : null}

        <main data-mf-admin-content className="min-w-0 flex-1 space-y-3">
          {status === "loading" ? <LoadingBlock /> : null}
          {status === "error" ? <ErrorNotice message={error ?? t("session.failed")} onRetry={reload} permissionHint={t("session.failedHint")} /> : null}
          {status === "ready" && !user ? (
            <div className="p-6 rounded-card bg-surfaceSubtle border border-line-subtle space-y-2">
              <div className="text-sm font-semibold text-text-strong">{t("session.loginRequiredTitle")}</div>
              <p className="text-xs text-text-muted leading-relaxed">{t("session.loginRequiredDesc")}</p>
            </div>
          ) : null}
          {status === "ready" && user && !canEnterAdmin(user) ? (
            <div className="space-y-3">
              <ForbiddenBlock permission={t("permission.anyAdminCode")} />
              <p className="text-xs text-text-faint leading-relaxed">{t("permission.adminCodes")}</p>
            </div>
          ) : null}
          {status === "ready" && user && canEnterAdmin(user) ? children : null}
        </main>
      </div>


    </div>
  );
}
