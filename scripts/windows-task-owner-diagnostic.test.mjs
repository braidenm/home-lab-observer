import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { link, mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { readRuntimeSmokeDiagnostics, summarizeRuntimeDiagnostics } from "./runtime-smoke-diagnostics.mjs";

test("runtime failure evidence reads only a bounded regular fixture file", async () => {
  const state = await mkdtemp(path.join(os.tmpdir(), "observer-runtime-diagnostic-test-"));
  try {
    assert.equal((await readRuntimeSmokeDiagnostics(state)).code, "RUNTIME_DIAGNOSTIC_UNAVAILABLE");
    await mkdir(path.join(state, "diagnostics"));
    const target = path.join(state, "diagnostics", "observer.jsonl");
    const row = { observed_at: "2026-09-17T00:00:00Z", event: "STOP_REQUESTED", code: "OK", version: "dev", count: 0, duration_ms: 0 };
    await writeFile(target, JSON.stringify(row) + "\n");
    assert.deepEqual(await readRuntimeSmokeDiagnostics(state), { code: "RUNTIME_DIAGNOSTIC_AVAILABLE", events: [{ event: "STOP_REQUESTED", code: "OK" }] });
    await link(target, path.join(state, "linked"));
    assert.equal((await readRuntimeSmokeDiagnostics(state)).code, "RUNTIME_DIAGNOSTIC_UNAVAILABLE");
    await rm(path.join(state, "linked"));
    await writeFile(target, Buffer.alloc(65537));
    assert.equal((await readRuntimeSmokeDiagnostics(state)).code, "RUNTIME_DIAGNOSTIC_UNAVAILABLE");
  } finally {
    await rm(state, { recursive: true, force: true });
  }
});

test("runtime failure evidence emits only bounded code-owned pairs", () => {
  const privateValue = "synthetic-secret@example.invalid";
  const row = { observed_at: privateValue, event: "RUNTIME_STOPPED", code: "TIMEOUT", version: privateValue, count: 0, duration_ms: 0 };
  const bytes = Buffer.from((JSON.stringify(row) + "\n").repeat(10));
  const summary = summarizeRuntimeDiagnostics(bytes);
  assert.equal(summary.code, "RUNTIME_DIAGNOSTIC_AVAILABLE");
  assert.equal(summary.events.length, 8);
  assert.deepEqual(summary.events[0], { event: "RUNTIME_STOPPED", code: "TIMEOUT" });
  assert(!JSON.stringify(summary).includes(privateValue));
  for (const value of [
    Buffer.alloc(0), Buffer.alloc(65537), Buffer.from("bad\n"), Buffer.from(JSON.stringify(row)),
    Buffer.from(JSON.stringify({ ...row, event: privateValue }) + "\n"),
    Buffer.from(JSON.stringify({ ...row, code: privateValue }) + "\n"),
    Buffer.from(JSON.stringify({ ...row, message: privateValue }) + "\n"),
    Buffer.from(JSON.stringify({ ...row, duration_ms: -1 }) + "\n"),
    Buffer.from((JSON.stringify(row) + "\n").repeat(257)),
  ]) assert.deepEqual(summarizeRuntimeDiagnostics(value), { code: "RUNTIME_DIAGNOSTIC_UNAVAILABLE" });
});

test("Windows task diagnostic recognizes only exact current-owner aliases", { skip: process.platform !== "win32" }, () => {
  const source = readFileSync(new URL("./smoke-windows-manager.mjs", import.meta.url), "utf8");
  const script = source.match(/function fixedTaskXMLShapeScript\(\) \{\s*return String.raw`([\s\S]*?)`;/)?.[1];
  assert(script, "fixed diagnostic script is missing");
  const powershell = path.join(process.env.SystemRoot, "System32", "WindowsPowerShell", "v1.0", "powershell.exe");
  const run = (command, input) => execFileSync(powershell, ["-NoProfile", "-NonInteractive", "-Command", command], {
    input, encoding: "utf8", timeout: 15_000, maxBuffer: 16_384, windowsHide: true,
  }).trim();
  assert.equal(run("$tokens=$null;$errors=$null;[void][Management.Automation.Language.Parser]::ParseInput([Console]::In.ReadToEnd(),[ref]$tokens,[ref]$errors);if($errors.Count){exit 1};'PARSE_OK'", script), "PARSE_OK");
  const main = script.indexOf("\ntry {\n  $script:TaskOwnerSid=");
  assert(main > 0, "diagnostic entrypoint is missing");
  const cases = String.raw`
$script:TaskOwnerSid='S-1-5-21-100-200-300-400'
$script:TaskOwnerName='LAB\owner'
$script:TaskOwnerBare='owner'
function Make-Document([string]$Value,[string]$Location='trigger') {
  $element='<UserId>'+[Security.SecurityElement]::Escape($Value)+'</UserId>'
  $inner=if($Location -ceq 'trigger'){'<Triggers><LogonTrigger>'+$element+'</LogonTrigger></Triggers>'}else{'<Principals><Principal>'+$element+'</Principal></Principals>'}
  $doc=New-Object Xml.XmlDocument
  $doc.LoadXml('<Task xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">'+$inner+'</Task>')
  return $doc
}
$expected=Make-Document $script:TaskOwnerSid
foreach($alias in @($script:TaskOwnerSid,'LAB\owner','lab\OWNER','owner','OWNER')) {
  if((Compare-TaskDocuments $expected (Make-Document $alias)).kind -cne 'MATCH'){throw 'approved alias rejected'}
}
foreach($other in @('OTHER\owner','LAB\another','',' owner ','S-1-5-21-100-200-300-401')) {
  if((Compare-TaskDocuments $expected (Make-Document $other)).kind -ceq 'MATCH'){throw 'unapproved identity accepted'}
}
if((Compare-TaskDocuments (Make-Document $script:TaskOwnerSid 'principal') (Make-Document 'LAB\owner' 'principal')).kind -ceq 'MATCH'){throw 'principal identity was normalized'}
$script:TaskOwnerName='DOMAIN\owner';$script:TaskOwnerBare=$null
if((Compare-TaskDocuments $expected (Make-Document 'owner')).kind -ceq 'MATCH'){throw 'ambiguous bare domain user accepted'}
if((Compare-TaskDocuments $expected (Make-Document 'DOMAIN\owner')).kind -cne 'MATCH'){throw 'qualified domain user rejected'}
'OWNER_ALIASES_OK'
`;
  assert.equal(run("$ErrorActionPreference='Stop';& ([ScriptBlock]::Create([Console]::In.ReadToEnd()))", script.slice(0, main) + cases), "OWNER_ALIASES_OK");
});
