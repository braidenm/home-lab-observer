#!/usr/bin/env node

import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { lstat, mkdtemp, realpath, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { managerSmokeFailure } from "./windows-manager-smoke-failure.mjs";

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
  const shape = queryFixedTaskXMLShape();
  primaryFailure = new Error(`${error instanceof Error ? error.message : "Windows manager smoke failed"}; task_xml_shape=${JSON.stringify(shape)}`);
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

const finalFailure = managerSmokeFailure(primaryFailure, cleanupConfirmed);
if (finalFailure) throw finalFailure;
console.log("Windows user-manager smoke passed enable, status, graceful stop, restart, and verified disable.");

function fixedTaskXMLShapeScript() {
  return String.raw`
$ErrorActionPreference='Stop'
try {
  $service=New-Object -ComObject 'Schedule.Service'
  $service.Connect()
  $task=$service.GetFolder('\').GetTask('\Home Lab Observer')
  $raw=[string]$task.Xml
  $hasDeclaration=$raw.TrimStart().StartsWith('<?xml')
  [xml]$document=$raw
  $elementCounts=@{}
  $attributeCounts=@{}
  $nodes=@($document.SelectNodes('//*'))
  foreach ($node in $nodes) {
    $elementName=[string]$node.LocalName
    if ($elementName -notmatch '^[A-Za-z][A-Za-z0-9._-]{0,63}$') { throw 'unsafe element name' }
    if ($elementCounts.ContainsKey($elementName)) { $elementCounts[$elementName]++ } else { $elementCounts[$elementName]=1 }
    foreach ($attribute in @($node.Attributes)) {
      $attributeName=[string]$attribute.LocalName
      if ($attributeName -notmatch '^[A-Za-z][A-Za-z0-9._-]{0,63}$') { throw 'unsafe attribute name' }
      $key=$elementName+'|'+$attributeName
      if ($attributeCounts.ContainsKey($key)) { $attributeCounts[$key]++ } else { $attributeCounts[$key]=1 }
    }
  }
  $elements=@($elementCounts.GetEnumerator() | Sort-Object Name | ForEach-Object { [ordered]@{name=[string]$_.Name; count=[int]$_.Value} })
  $attributes=@($attributeCounts.GetEnumerator() | Sort-Object Name | ForEach-Object {
    $parts=([string]$_.Name).Split('|',2)
    [ordered]@{element=$parts[0]; name=$parts[1]; count=[int]$_.Value}
  })
  [ordered]@{code='TASK_XML_SHAPE_AVAILABLE'; xml_declaration=[bool]$hasDeclaration; element_count=[int]$nodes.Count; elements=$elements; attributes=$attributes} | ConvertTo-Json -Compress -Depth 4
} catch {
  $code=if (($_.Exception.HResult -band 0xffff) -eq 2) {'TASK_XML_TASK_MISSING'} else {'TASK_XML_QUERY_FAILED'}
  [ordered]@{code=$code} | ConvertTo-Json -Compress
}
`;
}

function queryFixedTaskXMLShape() {
  try {
    const output = execFileSync("powershell.exe", ["-NoProfile", "-NonInteractive", "-Command", fixedTaskXMLShapeScript()], {
      encoding: "utf8",
      maxBuffer: 16_384,
      timeout: 15_000,
      windowsHide: true,
    });
    assert(Buffer.byteLength(output, "utf8") <= 16_384, "task XML shape output exceeded its fixed bound");
    return validateTaskXMLShape(JSON.parse(output));
  } catch {
    return { code: "TASK_XML_QUERY_FAILED" };
  }
}

function validateTaskXMLShape(value) {
  assert(value && typeof value === "object" && !Array.isArray(value), "task XML shape must be an object");
  const allowedCodes = new Set(["TASK_XML_SHAPE_AVAILABLE", "TASK_XML_TASK_MISSING", "TASK_XML_QUERY_FAILED"]);
  assert(allowedCodes.has(value.code), "task XML shape returned an unknown code");
  if (value.code !== "TASK_XML_SHAPE_AVAILABLE") {
    assert.deepEqual(Object.keys(value), ["code"], "unavailable task XML shape returned extra data");
    return { code: value.code };
  }
  assert.deepEqual(Object.keys(value).sort(), ["attributes", "code", "element_count", "elements", "xml_declaration"], "task XML shape changed");
  assert.equal(typeof value.xml_declaration, "boolean", "task XML declaration flag must be boolean");
  assert(Number.isInteger(value.element_count) && value.element_count >= 1 && value.element_count <= 4096, "task XML element count is out of bounds");
  assert(Array.isArray(value.elements) && value.elements.length <= 128, "task XML element summary is out of bounds");
  assert(Array.isArray(value.attributes) && value.attributes.length <= 128, "task XML attribute summary is out of bounds");
  const namePattern = /^[A-Za-z][A-Za-z0-9._-]{0,63}$/;
  const summarize = (entry, keys) => {
    assert(entry && typeof entry === "object" && !Array.isArray(entry), "task XML summary entry must be an object");
    assert.deepEqual(Object.keys(entry).sort(), [...keys].sort(), "task XML summary entry changed");
    for (const key of keys.filter((key) => key !== "count")) assert(namePattern.test(entry[key]), "task XML summary name is unsafe");
    assert(Number.isInteger(entry.count) && entry.count >= 1 && entry.count <= 4096, "task XML summary count is out of bounds");
    return Object.fromEntries(keys.map((key) => [key, entry[key]]));
  };
  return {
    code: value.code,
    xml_declaration: value.xml_declaration,
    element_count: value.element_count,
    elements: value.elements.map((entry) => summarize(entry, ["name", "count"])),
    attributes: value.attributes.map((entry) => summarize(entry, ["element", "name", "count"])),
  };
}

function requiredPath(name) {
  const value = process.env[name];
  assert(value, `${name} is required after explicit manager-test opt-in`);
  return path.resolve(value);
}

function runObserver(args) {
  const remaining = deadline - Date.now();
  assert(remaining > 0, "Windows manager smoke exceeded its 110-second deadline");
  try {
    return execFileSync(binary, args, { encoding: "utf8", maxBuffer: 65_536, timeout: Math.min(45_000, remaining), windowsHide: true });
  } catch (error) {
    const stderr = typeof error?.stderr === "string" ? error.stderr : Buffer.isBuffer(error?.stderr) ? error.stderr.toString("utf8") : "";
    const stableCode = stderr.match(/\bBACKGROUND_[A-Z_]+\b/)?.[0];
    throw new Error(`observer ${args.slice(0, 2).join(" ")} failed${stableCode ? ` (${stableCode})` : ""}`);
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
