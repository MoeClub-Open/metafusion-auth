// 会话 Cookie mf_session 是 HttpOnly，脚本读不到。账号服务在写入/清除会话时同步
// 一个可读的提示 Cookie mf_has_session（主站 proxy 也会补种，名称与语义一致）。
// 没有提示 = 浏览器里没有会话，直接按未登录处理，不发注定 401 的 /api/auth/me。
// 提示只用来省请求，不代表身份；真正的身份仍以 /api/auth/me 为准。
export const SESSION_HINT_COOKIE_NAME = "mf_has_session";

export function hasSessionHintIn(cookie: string): boolean {
  return new RegExp(`(?:^|;\\s*)${SESSION_HINT_COOKIE_NAME}=1(?:;|$)`).test(cookie);
}

export function hasSessionHint(): boolean {
  if (typeof document === "undefined") return false;
  return hasSessionHintIn(document.cookie);
}

export function clearSessionHint(): void {
  if (typeof document === "undefined") return;
  document.cookie = `${SESSION_HINT_COOKIE_NAME}=; path=/; max-age=0; samesite=lax`;
}
