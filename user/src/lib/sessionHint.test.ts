import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { hasSessionHintIn } from "./sessionHint";

describe("session hint", () => {
  it("detects the hint cookie", () => {
    assert.equal(hasSessionHintIn("a=1; mf_has_session=1"), true);
    assert.equal(hasSessionHintIn("mf_has_session=1"), true);
  });
  it("absent or cleared hint means logged out", () => {
    assert.equal(hasSessionHintIn(""), false);
    assert.equal(hasSessionHintIn("mf_has_session="), false);
    assert.equal(hasSessionHintIn("x_mf_has_session=1"), false);
  });
});
