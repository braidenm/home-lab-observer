#!/usr/bin/env node

import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { lstat, mkdtemp, realpath, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { setTimeout as delay } from "node:timers/promises";

const optedIn = process.env.OBSERVER_TEST_USER_MANAGER === "1";
if (!optedIn) {
  console.log("Windows user-manager smoke skipped; explicit opt-in was not set.");
  process.exit(0);
}
assert.equal(process.platform, "win32", "Windows user-manager smoke may run only on Windows");
assert.equal(process.env.GITHUB_ACTIONS, "true", "Windows user-manager smoke requires GitHub Actions");
assert.equal(process.env.RUNNER_ENVIRONMENT, "github-hosted", "Windows user-manager smoke requires a GitHub-hosted runner");

const binary = requiredPath("OBSERVER_SMOKE_BINARY");
const installRoot = requiredPath("OBSERVER_SMOKE_INSTALL_ROOT");
assert((await lstat(binary)).isFile(), "OBSERVER_SMOKE_BINARY must be a regular file");
assert((await lstat(installRoot)).isDirectory(), "OBSERVER_SMOKE_INSTALL_ROOT must be a directory");
const temporary = await realpath(await mkdtemp(path.join(os.tmpdir(), "observer-manager-smoke-")));
const state = path.join(temporary, "state");
assert(!isWithin(installRoot, state) && !isWithin(state, installRoot), "manager smoke state must be separate from the install root");
const address = `127.0.0.1:${await freePort()}`;
const deadline = Date.now() + 110_000;
let enableAttempted = false;
let cleanupConfirmed = false;
let primaryFailure;

try {
  enableAttempted = true;
  runObserver(["background", "enable", "--install-root", installRoot, "--state-dir", state, "--listen", address]);
  await waitForStatus((status) => status.state === "RUNNING" && status.readiness === "READY" && status.diagnostics === "AVAILABLE", "ready background runtime");

  runObserver(["background", "stop", "--install-root", installRoot]);
  const stopped = status();
  assert.equal(stopped.state, "REGISTERED_STOPPED", "normal stop did not preserve a stopped registration");
  assert.equal(stopped.running, false, "normal stop still reported a running task");

  runObserver(["background", "start", "--install-root", installRoot]);
  await waitForStatus((value) => value.state === "RUNNING" && value.readiness === "READY" && value.diagnostics === "AVAILABLE", "restarted background runtime");

  runObserver(["background", "disable", "--install-root", installRoot]);
  cleanupConfirmed = verifyDisabled();
  assert(cleanupConfirmed, "normal disable did not remove the managed registration");
} catch (error) {
  primaryFailure = error;
} finally {
  if (enableAttempted && !cleanupConfirmed) {
    try {
      // Test cleanup explicitly authorizes force only after the normal lifecycle
      // path has already failed; the product command itself never falls through.
      runObserver(["background", "disable", "--install-root", installRoot, "--force"]);
      cleanupConfirmed = verifyDisabled();
    } catch {
      cleanupConfirmed = false;
    }
  }
  if (cleanupConfirmed) {
    await rm(temporary, { recursive: true, force: true });
  }
}

assert(cleanupConfirmed, `Windows manager cleanup could not be verified; isolated evidence was preserved at ${temporary}`);
if (primaryFailure) throw primaryFailure;
console.log("Windows user-manager smoke passed enable, status, graceful stop, restart, and verified disable.");

function requiredPath(name) {
  const value = process.env[name];
  assert(value, `${name} is required after explicit manager-test opt-in`);
  return path.resolve(value);
}

function runObserver(args) {
  const remaining = deadline - Date.now();
  assert(remaining > 0, "Windows manager smoke exceeded its 110-second deadline");
  try {
    return execFileSync(binary, args, { encoding: "utf8", timeout: Math.min(45_000, remaining), windowsHide: true });
  } catch {
    throw new Error(`observer ${args.slice(0, 2).join(" ")} failed`);
  }
}

function status() {
  const value = JSON.parse(runObserver(["background", "status", "--install-root", installRoot, "--json"]));
  const expectedKeys = ["code", "diagnostics", "readiness", "registered", "running", "schema_version", "scope", "session_limitation", "state"];
  assert.deepEqual(Object.keys(value).sort(), expectedKeys, "background status JSON shape changed");
  assert.equal(value.schema_version, "observer-background-status/v1");
  assert.equal(value.scope, "USER_SESSION");
  assert.equal(value.session_limitation, "LOGIN_SESSION_REQUIRED");
  return value;
}

async function waitForStatus(predicate, label) {
  while (Date.now() < deadline) {
    const value = status();
    if (predicate(value)) return value;
    await delay(250);
  }
  throw new Error(`timed out waiting for ${label}`);
}

function verifyDisabled() {
  const value = status();
  return value.state === "NOT_REGISTERED" && value.registered === false && value.running === false;
}

function isWithin(parent, candidate) {
  const relative = path.relative(parent, candidate);
  return relative === "" || (!relative.startsWith(".." + path.sep) && relative !== "..");
}

async function freePort() {
  const { createServer } = await import("node:net");
  const server = createServer();
  await new Promise((resolve, reject) => { server.once("error", reject); server.listen(0, "127.0.0.1", resolve); });
  const value = server.address();
  assert(value && typeof value !== "string", "could not reserve a manager smoke port");
  await new Promise((resolve) => server.close(resolve));
  return value.port;
}
