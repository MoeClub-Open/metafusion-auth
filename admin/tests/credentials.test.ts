import { test } from "node:test";
import assert from "node:assert/strict";
import {
  deleteAdminCredential, deleteAdminOAuthToken, fetchAdminCredentials, fetchAdminOAuthGrants,
  fetchAdminOAuthTokens, revokeAdminCredential, revokeAdminOAuthGrant, updateAdminCredential,
} from "../src/lib/endpoints";
import { canEnterSection } from "../src/lib/permissions";

test("credentials navigation accepts either credential or OAuth management permissions", () => {
  const user = { id: "admin", username: "admin", permissions: ["auth.users.manage"] };
  assert.equal(canEnterSection(user, "credentials"), true);
  assert.equal(canEnterSection({ ...user, permissions: ["auth.oauth.manage"] }, "credentials"), true);
  assert.equal(canEnterSection({ ...user, permissions: ["auth.audit.read"] }, "credentials"), false);
  assert.equal(canEnterSection({ ...user, permissions: ["*"] }, "credentials"), true);
  assert.equal(canEnterSection(null, "credentials"), false);
});

test("admin inventory preserves pagination/filter fields and sends mutations to the owner-independent governance endpoints", async (context) => {
  const originalFetch = globalThis.fetch;
  context.after(() => { globalThis.fetch = originalFetch; });
  const requests: { path: string; init?: RequestInit }[] = [];
  globalThis.fetch = (async (path: string | URL | Request, init?: RequestInit) => {
    requests.push({ path: String(path), init });
    return new Response(JSON.stringify({ items: [], total: 0, limit: 25, offset: 50, has_more: false }), { headers: { "content-type": "application/json" } });
  }) as typeof fetch;

  await fetchAdminCredentials({ owner_id: "owner", kind: "mcp", status: "revoked", limit: 25, offset: 50 });
  const keys = new URL(requests.at(-1)!.path, "https://site.invalid");
  assert.equal(keys.pathname, "/api/admin/credentials");
  assert.deepEqual(Object.fromEntries(keys.searchParams), { owner_id: "owner", kind: "mcp", status: "revoked", limit: "25", offset: "50" });
  await fetchAdminOAuthGrants({ owner_id: "owner", client_id: "app", limit: 25, offset: 0 });
  assert.equal(new URL(requests.at(-1)!.path, "https://site.invalid").pathname, "/api/admin/oauth/grants");
  await fetchAdminOAuthTokens({ owner_id: "", client_id: "app", status: "expired", limit: 25, offset: 0 });
  const tokens = new URL(requests.at(-1)!.path, "https://site.invalid");
  assert.equal(tokens.pathname, "/api/admin/oauth/tokens");
  assert.equal(tokens.searchParams.has("owner_id"), false);
  assert.equal(tokens.searchParams.get("status"), "expired");

  await updateAdminCredential("key/id", { name: "Updated", scopes: ["catalog.entity.edit"] });
  assert.equal(requests.at(-1)!.path, "/api/admin/credentials/key%2Fid");
  assert.equal(requests.at(-1)!.init?.method, "PATCH");
  assert.deepEqual(JSON.parse(String(requests.at(-1)!.init?.body)), { name: "Updated", scopes: ["catalog.entity.edit"] });
  await revokeAdminCredential("key/id");
  assert.equal(requests.at(-1)!.path, "/api/admin/credentials/key%2Fid/revoke");
  assert.equal(requests.at(-1)!.init?.method, "POST");
  await deleteAdminCredential("key/id");
  assert.equal(requests.at(-1)!.path, "/api/admin/credentials/key%2Fid");
  assert.equal(requests.at(-1)!.init?.method, "DELETE");
  await revokeAdminOAuthGrant("owner", "app");
  assert.equal(requests.at(-1)!.path, "/api/admin/oauth/grants/revoke");
  assert.deepEqual(JSON.parse(String(requests.at(-1)!.init?.body)), { user_id: "owner", client_id: "app" });
  await deleteAdminOAuthToken("safe-id");
  assert.equal(requests.at(-1)!.path, "/api/admin/oauth/tokens/safe-id");
  assert.equal(requests.at(-1)!.init?.method, "DELETE");
  assert.ok(requests.every(({ init }) => init?.credentials === "include"));
});
