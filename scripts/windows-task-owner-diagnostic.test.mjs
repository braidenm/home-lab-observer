import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import path from "node:path";
import test from "node:test";

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
foreach($other in @('OTHER\owner','LAB\another','','S-1-5-21-100-200-300-401')) {
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
