import type { Metadata, Viewport } from "next";
import { cookies } from "next/headers";
import Link from "next/link";
import "@/shared/theme/globals.css";
import { getMessages } from "@/i18n/getMessages";
import { I18nProvider } from "@/i18n/I18nProvider";
import { SessionProvider } from "@/lib/session";
import { localeCookieName, normalizeLocale } from "@/i18n/routing";
import { ThemeProvider } from "@/lib/themeContext";
import { DEFAULT_ACCENT, DEFAULT_TONE, DEFAULT_PATTERN, DEFAULT_BG_DARK } from "@/lib/theme.generated";
import { themeBootScript } from "@/shared/theme/boot.generated";
import { BrandMark } from "@/components/Logo";
import { PageContainer } from "@/components/ui/PageShell";
import { LocaleSwitcher } from "@/components/LocaleSwitcher";
import { ThemeModeSwitcher } from "@/components/ThemeModeSwitcher";
import { ThemePicker } from "@/components/ThemePicker";

// 语言按请求决定（NEXT_LOCALE cookie），因此不做静态预渲染。
export const dynamic = "force-dynamic";

export const viewport: Viewport = { themeColor: DEFAULT_BG_DARK };

export async function generateMetadata(): Promise<Metadata> {
  const messages = getMessages((await cookies()).get(localeCookieName)?.value);
  return {
    title: messages["app.title"] ?? "MetaFusion Account",
    description: messages["app.description"] ?? "",
    icons: { icon: "/favicon.svg", shortcut: "/favicon.svg" },
    robots: { index: false, follow: false },
  };
}

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  const locale = normalizeLocale((await cookies()).get(localeCookieName)?.value);
  return (
    <html lang={locale} className="dark" data-theme-mode="dark" data-theme-accent={DEFAULT_ACCENT} data-theme-tone={DEFAULT_TONE} data-theme-pattern={DEFAULT_PATTERN} suppressHydrationWarning>
      <head><script dangerouslySetInnerHTML={{ __html: themeBootScript }} /></head>
      <body className="min-h-dvh flex flex-col font-sans antialiased">
        <ThemeProvider>
          <I18nProvider initialLocale={locale}>
            <PageContainer as="header" width="narrow" className="relative z-20 shrink-0 p-4 sm:p-5">
              <div className="flex items-center justify-between gap-3">
                <Link href="/" aria-label="MetaFusion" className="mf-focus flex shrink-0 items-center gap-2.5 rounded-control text-text-strong">
                  <BrandMark size={28} idSuffix="account" />
                  <span className="hidden sm:inline font-display text-xl tracking-[-0.03em]">MetaFusion</span>
                </Link>
                <div className="flex items-center gap-2">
                  <LocaleSwitcher />
                  <ThemeModeSwitcher />
                  <ThemePicker />
                </div>
              </div>
            </PageContainer>
            <SessionProvider>{children}</SessionProvider>
          </I18nProvider>
        </ThemeProvider>
      </body>
    </html>
  );
}
