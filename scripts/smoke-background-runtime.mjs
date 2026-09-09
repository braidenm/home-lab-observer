#!/usr/bin/env node

import assert from "node:assert/strict";
import { spawn, spawnSync } from "node:child_process";
import { lstat, mkdtemp, readFile, realpath, rename, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:net";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { setTimeout as delay } from "node:timers/promises";
import Ajv from "ajv/dist/2020.js";
import addFormats from "ajv-formats";

const root = path.resolve(fileURLToPath(new URL("..", import.meta.url)));
const createdTemporary = await mkdtemp(path.join(os.tmpdir(), "observer-background-smoke-"));
const temporary = await realpath(createdTemporary);
const binary = process.env.OBSERVER_SMOKE_BINARY
  ? path.resolve(process.env.OBSERVER_SMOKE_BINARY)
  : path.join(temporary, process.platform === "win32" ? "observer.exe" : "observer");

try {
  if (process.env.OBSERVER_SMOKE_BINARY) {
    assert((await lstat(binary)).isFile(), "background smoke binary must be a regular file");
  } else {
    const build = spawnSync("go", ["build", "-o", binary, "./cmd/observer"], { cwd: root, encoding: "utf8", timeout: 180_000 });
    assert.equal(build.status, 0, `observer build failed: ${build.stderr}`);
  }
  const ajv = new Ajv({ allErrors: true, strict: false });
  addFormats(ajv);
  const schema = JSON.parse(await readFile(path.join(root, "schemas/v1/diagnostics-health-v1.schema.json"), "utf8"));
  const validateDiagnostics = ajv.compile(schema);
  const managedState = path.join(temporary, "managed state");

  let previousSequence = 0;
  for (let run = 0; run < 2; run += 1) {
    const child = await startObserver(managedState, true);
    try {
      const health = await readDiagnostics(child, validateDiagnostics);
      assert.equal(health.enabled, true);
      assert.equal(health.available, true);
      assert.equal(health.state, "AVAILABLE");
      assert.equal(health.policy.contains_log_contents, false);
      assert.equal(health.policy.contains_paths, false);
      assert.equal(health.policy.remote_upload_eligible, false);
      const sequence = await readCurrentSequence(child);
      assert(sequence > previousSequence, "managed restart did not continue persisted observation sequence");
      previousSequence = sequence;
      await requestNonceStop(managedState);
      assert(await waitForExit(child.process, 45_000), "managed runtime did not complete graceful stop");
      assert.equal(child.process.exitCode, 0, "managed runtime exited unsuccessfully");
      assert(!(await exists(path.join(managedState, "lifecycle", "instance.json"))), "managed runtime left a published lifecycle instance");
      assert(!child.output().includes(child.token), "managed runtime output leaked the local token");
    } finally {
      await terminateAndWait(child.process);
    }
  }

  const foreground = await startObserver(path.join(temporary, "foreground state"), false);
  try {
    const health = await readDiagnostics(foreground, validateDiagnostics);
    assert.equal(health.enabled, false);
    assert.equal(health.available, false);
    assert.equal(health.state, "DISABLED");
    assert.equal(health.reason_code, "DIAGNOSTICS_DISABLED");
    assert(!foreground.output().includes(foreground.token), "foreground output leaked the local token");
  } finally {
    await terminateAndWait(foreground.process);
  }

  console.log("Native background runtime passed nonce stop, history reopen, and diagnostics contract checks.");
} finally {
  await rm(temporary, { recursive: true, force: true });
}

async function startObserver(state, managed) {
  const port = await freePort();
  const args = ["serve", "--listen", `127.0.0.1:${port}`, "--state-dir", state];
  if (managed) args.push("--internal-background-runtime");
  const processHandle = spawn(binary, args, { cwd: root, stdio: ["ignore", "pipe", "pipe"] });
  let output = "";
  let spawnError;
  processHandle.once("error", (error) => { spawnError = error; });
  for (const stream of [processHandle.stdout, processHandle.stderr]) {
    stream.on("data", (chunk) => { output = (output + chunk).slice(-16_384); });
  }
  await until(async () => {
    if (spawnError) throw spawnError;
    const response = await fetch(`http://127.0.0.1:${port}/health/live`, { signal: AbortSignal.timeout(2_000) });
    return response.status === 200;
  }, processHandle, 45_000);
  const token = (await readFile(path.join(state, "local-api.token"), "utf8")).trim();
  assert(/^[A-Za-z0-9_-]{43}$/u.test(token), "runtime did not create a strong local token");
  return { process: processHandle, origin: `http://127.0.0.1:${port}`, token, output: () => output };
}

async function readDiagnostics(child, validator) {
  const unauthorized = await fetch(`${child.origin}/api/v1/diagnostics/health`, { signal: AbortSignal.timeout(5_000) });
  assert.equal(unauthorized.status, 401, "diagnostics endpoint did not require authentication");
  const response = await fetch(`${child.origin}/api/v1/diagnostics/health`, {
    headers: { Authorization: `Bearer ${child.token}` },
    signal: AbortSignal.timeout(5_000),
  });
  const body = await response.text();
  assert(response.ok, "diagnostics endpoint was unavailable");
  assert(Buffer.byteLength(body) <= 32_768, "diagnostics response exceeded 32 KiB");
  assert(!body.includes(child.token), "diagnostics response leaked the local token");
  const value = JSON.parse(body);
  assert(validator(value), `diagnostics contract failed: ${JSON.stringify(validator.errors)}`);
  return value;
}

async function readCurrentSequence(child) {
  let sequence = 0;
  await until(async () => {
    const response = await fetch(`${child.origin}/api/v1/snapshots/current`, {
      headers: { Authorization: `Bearer ${child.token}` },
      signal: AbortSignal.timeout(2_000),
    });
    if (!response.ok) return false;
    const body = await response.text();
    assert(Buffer.byteLength(body) <= 1_048_576, "current snapshot exceeded 1 MiB");
    assert(!body.includes(child.token), "current snapshot leaked the local token");
    const value = JSON.parse(body);
    sequence = value.sequence;
    return Number.isSafeInteger(sequence) && sequence > 0;
  }, child.process, 30_000);
  return sequence;
}

async function requestNonceStop(state) {
  const directory = path.join(state, "lifecycle");
  const instancePath = path.join(directory, "instance.json");
  const contents = await readBounded(instancePath, 1024);
  const instance = JSON.parse(contents);
  assert.deepEqual(Object.keys(instance).sort(), ["nonce", "schema_version", "started_at"]);
  assert.equal(instance.schema_version, "observer-lifecycle-instance/v1");
  assert(/^[A-Za-z0-9_-]{43}$/u.test(instance.nonce), "lifecycle instance nonce was malformed");
  const request = JSON.stringify({ schema_version: "observer-lifecycle-stop/v1", nonce: instance.nonce }) + "\n";
  const temporaryPath = path.join(directory, "stop.request.tmp");
  await writeFile(temporaryPath, request, { encoding: "utf8", mode: 0o600, flag: "wx" });
  await rename(temporaryPath, path.join(directory, "stop.request"));
}

async function readBounded(filename, maximum) {
  const info = await lstat(filename);
  assert(info.isFile() && !info.isSymbolicLink() && info.size > 0 && info.size <= maximum, "lifecycle state was not a bounded regular file");
  return readFile(filename, "utf8");
}

async function until(test, processHandle, timeoutMilliseconds) {
  const deadline = Date.now() + timeoutMilliseconds;
  while (Date.now() < deadline) {
    if (processHandle.exitCode !== null || processHandle.signalCode !== null) throw new Error("observer exited before the expected runtime state");
    try {
      if (await test()) return;
    } catch (error) {
      if (error?.code !== "ECONNREFUSED" && error?.name !== "TypeError") throw error;
    }
    await delay(100);
  }
  throw new Error("timed out waiting for observer background runtime");
}

async function exists(filename) {
  try { await lstat(filename); return true; } catch (error) { if (error?.code === "ENOENT") return false; throw error; }
}

async function freePort() {
  const server = createServer();
  await new Promise((resolve, reject) => { server.once("error", reject); server.listen(0, "127.0.0.1", resolve); });
  const address = server.address();
  assert(address && typeof address !== "string", "could not reserve a smoke port");
  await new Promise((resolve) => server.close(resolve));
  return address.port;
}

async function waitForExit(processHandle, timeoutMilliseconds) {
  if (processHandle.exitCode !== null || processHandle.signalCode !== null) return true;
  return Promise.race([
    new Promise((resolve) => processHandle.once("exit", () => resolve(true))),
    delay(timeoutMilliseconds).then(() => false),
  ]);
}

async function terminateAndWait(processHandle) {
  if (processHandle.exitCode !== null || processHandle.signalCode !== null) return;
  processHandle.kill("SIGTERM");
  if (await waitForExit(processHandle, 5_000)) return;
  processHandle.kill("SIGKILL");
  assert(await waitForExit(processHandle, 5_000), "observer child did not exit after forced cleanup");
}
