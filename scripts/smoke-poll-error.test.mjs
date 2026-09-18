import assert from "node:assert/strict";
import test from "node:test";
import { retryableSmokePollError } from "./smoke-poll-error.mjs";

test("bounded smoke polling retries connection and per-request timeout errors", () => {
  for (const error of [
    { code: "ECONNREFUSED" },
    { name: "TypeError" },
    { name: "TimeoutError" },
    { name: "AbortError" },
  ]) {
    assert.equal(retryableSmokePollError(error), true);
  }
  for (const error of [null, new Error("contract failure"), { code: "EACCES" }]) {
    assert.equal(retryableSmokePollError(error), false);
  }
});
