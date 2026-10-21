package handler

import (
	"context"
	"database/sql"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/MoeclubM/metafusion-auth/internal/store"

	_ "github.com/lib/pq"
)

// 登录失败保护（login_guard.go）的行为口径，全部走 HTTP 中间件本身：
//
//	账号维度：只退避，不因其他来源的失败而硬锁账号；
//	IP 维度：同窗口内失败满 20 次起拦（兜住"每账号只试几次"的账号喷洒）；
//	递增延迟：失败 3 次起 200ms/400ms…封顶 2s，只在已有失败史时产生。
//
// 不需要数据库：Store 的 DB 指向一个打不通的地址，登录查询稳定失败 → 401
// （口令之外还有 bcrypt 比较，失败路径本身有真实耗时，所以延迟断言留了余量）。
// 空体与无标识请求按设计不计数，因此这里一律构造合法 JSON 体。
func newLoginGuardTestServer(t *testing.T, now *time.Time) (*gin.Engine, *loginGuard) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := sql.Open("postgres", "postgres://user:pw@127.0.0.1:1/auth_guard_test?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatalf("open placeholder db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s := &store.Store{DB: db, Tokens: newTestIssuer(t)}
	h := New(s)
	// 关掉按 IP 的请求速率层：本文件只验失败凭据层，否则第 16 次请求先被
	// rate_limited 拦下，量到的是另一层的阈值（两层的关系见 login_guard.go 顶部）。
	h.rateLimitPolicy = func(context.Context) (bool, int) { return false, store.DefaultRateLimitPerMinute }
	guard := newLoginGuard()
	guard.now = func() time.Time { return *now }
	h.loginGuardFactory = func() *loginGuard { return guard }
	r := gin.New()
	h.Register(r)
	return r, guard
}

func loginAttempt(t *testing.T, r *gin.Engine, username string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"username":"` + username + `","password":"WrongGuess!!"}`
	return doJSON(t, r, http.MethodPost, "/api/auth/login", "", body)
}

// 重现报告中的五次匿名失败：下一次仍必须进入凭据校验，不能锁死账号。
func TestLoginGuardAccountFailuresDoNotHardLock(t *testing.T) {
	now := time.Now()
	r, guard := newLoginGuardTestServer(t, &now)
	keys := loginIdentifiers(loginPayload{Username: "uiwalk-bruteforce"})
	for i := 0; i < 5; i++ {
		guard.recordFailure(keys, "198.51.100.7")
	}
	w := loginAttempt(t, r, "uiwalk-bruteforce")
	if w.Code != http.StatusUnauthorized || w.Header().Get("Retry-After") != "" {
		t.Fatalf("账号失败不能硬锁后续登录: %d %s", w.Code, w.Body.String())
	}
}

func TestLoginGuardHTTPBlocksOnlyFailingSource(t *testing.T) {
	now := time.Now()
	r, guard := newLoginGuardTestServer(t, &now)
	// doJSON uses this source address. Seed counters to avoid repeated bcrypt/delays.
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	ip, _, _ := net.SplitHostPort(request.RemoteAddr)
	for i := 0; i < loginGuardIPMaxFailure; i++ {
		guard.recordFailure([]string{"uiwalk-bruteforce"}, ip)
	}
	w := loginAttempt(t, r, "uiwalk-bruteforce")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("失败来源应被 429 拦下，实际 %d / %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Retry-After"); got == "" {
		t.Fatal("429 必须带 Retry-After，客户端才知道退避多久")
	} else if got == "0" {
		t.Fatalf("Retry-After 不能是 0（会诱导立刻重试），实际 %q", got)
	}
	var body struct {
		Error string `json:"error"`
	}
	decodeInto(t, w, &body)
	if body.Error != loginGuardErrorCode {
		t.Fatalf("错误码应为 %s（与请求速率层的 rate_limited 区分），实际 %q", loginGuardErrorCode, body.Error)
	}

	if code := loginAttempt(t, r, "uiwalk-bruteforce").Code; code != http.StatusTooManyRequests {
		t.Fatalf("窗口内应持续 429，实际 %d", code)
	}

	// 窗口过期：自动恢复，不需要人工解锁。
	now = now.Add(loginGuardWindow + time.Minute)
	if code := loginAttempt(t, r, "uiwalk-bruteforce").Code; code != http.StatusUnauthorized {
		t.Fatalf("窗口过后应恢复为 401（自动解锁），实际 %d", code)
	}
}

// IP 维度：每个账号只错 4 次，同一来源累计满 20 次后仍必须拦住账号喷洒。
//
// 走 guard 的公开判定面而不是 20 次 HTTP 请求：每次失败登录的 bcrypt 比较要 1.4s 左右，
// 20 次会把这条用例拖到 30s；中间件接线由本文件其余用例覆盖。
func TestLoginGuardBlocksBruteforceByIP(t *testing.T) {
	guard := newLoginGuard()
	now := time.Now()
	guard.now = func() time.Time { return now }
	ip := "198.51.100.7"

	for i := 0; i < loginGuardIPMaxFailure; i++ {
		keys := loginIdentifiers(loginPayload{Username: "spray-" + strconv.Itoa(i/4)})
		if blocked, _, _ := guard.check(keys, ip); blocked {
			t.Fatalf("第 %d 次（账号 %s）不该被拦：账号维度每次只错 4 次，IP 维度还没到 %d", i+1, keys[0], loginGuardIPMaxFailure)
		}
		guard.recordFailure(keys, ip)
	}

	blocked, retryAfter, _ := guard.check(loginIdentifiers(loginPayload{Username: "spray-fresh"}), ip)
	if !blocked {
		t.Fatalf("同一来源累计满 %d 次失败后应被拦（否则账号喷洒无成本）", loginGuardIPMaxFailure)
	}
	if retryAfter <= 0 {
		t.Fatal("被拦时必须给出 Retry-After 的正数秒数")
	}

	// 窗口过后 IP 维度同样自动恢复。
	now = now.Add(loginGuardWindow + time.Minute)
	if blocked, _, _ := guard.check(loginIdentifiers(loginPayload{Username: "spray-fresh"}), ip); blocked {
		t.Fatal("窗口过后 IP 维度应自动恢复")
	}

	// 即使多个来源累计超过 IP 阈值，账号也只有封顶延迟；新来源仍可校验口令。
	account := loginIdentifiers(loginPayload{Username: "spray-account"})
	for i := 0; i < loginGuardIPMaxFailure+5; i++ {
		if blocked, _, _ := guard.check(account, ""); blocked {
			t.Fatalf("账号第 %d 次失败仍应可重试（不能把普通用户锁死）", i+1)
		}
		guard.recordFailure(account, "")
	}
	if blocked, _, delay := guard.check(account, "198.51.100.8"); blocked || delay != loginGuardDelayMax {
		t.Fatalf("新来源不应因账号计数被硬锁: blocked=%v delay=%v", blocked, delay)
	}
}

// 递增延迟：无失败史的登录不等待；失败 3 次后开始出现延迟，且随失败次数增长。
func TestLoginGuardAddsIncrementalDelay(t *testing.T) {
	now := time.Now()
	r, _ := newLoginGuardTestServer(t, &now)

	start := time.Now()
	if code := loginAttempt(t, r, "delay-probe").Code; code != http.StatusUnauthorized {
		t.Fatalf("第 1 次应 401，实际 %d", code)
	}
	firstRun := time.Since(start)

	for i := 0; i < loginGuardDelayAfterFailure; i++ {
		loginAttempt(t, r, "delay-probe")
	}
	// 计数已到 4：按 loginGuardDelay 的换算，这次请求至少要等 400ms。
	want := loginGuardDelay(loginGuardDelayAfterFailure + 1)
	start = time.Now()
	loginAttempt(t, r, "delay-probe")
	got := time.Since(start)
	if got < want {
		t.Fatalf("失败 %d 次后应至少延迟 %s，实际 %s", loginGuardDelayAfterFailure+1, want, got)
	}
	if firstRun > 150*time.Millisecond {
		t.Fatalf("无失败史的正常登录不该被拖慢，首次耗时 %s", firstRun)
	}
}

// 成功登录清零账号维度：否则用户被拦之前成功登录一次，之后的失败仍按旧计数算。
func TestLoginGuardResetsAccountCounterOnSuccess(t *testing.T) {
	now := time.Now()
	guard := newLoginGuard()
	guard.now = func() time.Time { return now }
	keys := loginIdentifiers(loginPayload{Username: "UIWalk-Reset"})
	guard.recordFailure(keys, "192.0.2.9")
	guard.recordFailure(keys, "192.0.2.9")
	guard.recordSuccess(keys)
	if blocked, _, _ := guard.check(keys, "192.0.2.9"); blocked {
		t.Fatal("成功登录后账号维度应清零，不该再被判为封锁")
	}
	if _, ok := guard.accounts["uiwalk-reset"]; ok {
		t.Fatal("成功登录应删掉账号桶（标识按小写归一后作为键）")
	}
}

// 读不出体的请求不计数：空体刷 100 次也不能把 IP 桶刷满（否则任何人都能拿空请求误锁别人）。
func TestLoginGuardIgnoresUnparseableRequest(t *testing.T) {
	now := time.Now()
	r, guard := newLoginGuardTestServer(t, &now)
	for i := 0; i < loginGuardIPMaxFailure+5; i++ {
		if w := doJSON(t, r, http.MethodPost, "/api/auth/login", "", ""); w.Code != http.StatusBadRequest {
			t.Fatalf("空体应 400，实际 %d", w.Code)
		}
	}
	if len(guard.ips) != 0 {
		t.Fatalf("空体请求不该产生 IP 计数，实际 %d 个桶", len(guard.ips))
	}
}
