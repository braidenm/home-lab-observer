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
function New-Comparison([string]$Kind,[string]$Path,[string]$Field) {
  return [ordered]@{kind=$Kind;path=$Path;field=$Field}
}
function Get-DirectText($Node) {
  $text=''
  foreach ($child in @($Node.ChildNodes)) {
    if ($child.NodeType -eq [Xml.XmlNodeType]::Text -or $child.NodeType -eq [Xml.XmlNodeType]::CDATA) { $text += [string]$child.Value }
  }
  return $text.Trim()
}
function Test-NormalizedElement($Node,[string]$LogicalPath) {
  if ([string]$Node.NamespaceURI -cne 'http://schemas.microsoft.com/windows/2004/02/mit/task') { return $false }
  if (@($Node.Attributes).Count -ne 0 -or @($Node.ChildNodes | Where-Object {$_.NodeType -eq [Xml.XmlNodeType]::Element}).Count -ne 0) { return $false }
  $expected=$null
  switch -CaseSensitive ($LogicalPath) {
    'Task/Triggers/LogonTrigger/Enabled' {$expected='true'}
    'Task/Principals/Principal/RunLevel' {$expected='LeastPrivilege'}
    'Task/Settings/AllowHardTerminate' {$expected='true'}
    'Task/Settings/RunOnlyIfNetworkAvailable' {$expected='false'}
    'Task/Settings/AllowStartOnDemand' {$expected='true'}
    'Task/Settings/Enabled' {$expected='true'}
    'Task/Settings/Hidden' {$expected='false'}
    'Task/Settings/RunOnlyIfIdle' {$expected='false'}
    'Task/Settings/DisallowStartOnRemoteAppSession' {$expected='false'}
    'Task/Settings/WakeToRun' {$expected='false'}
    'Task/Settings/Priority' {$expected='7'}
    'Task/RegistrationInfo/URI' {$expected='\Home Lab Observer'}
    default {return $false}
  }
  return (Get-DirectText $Node) -ceq $expected
}
function Get-ComparableChildren($Node,[string]$LogicalPath) {
  foreach ($child in @($Node.ChildNodes)) {
    if ($child.NodeType -ne [Xml.XmlNodeType]::Element) { continue }
    $childPath=$LogicalPath+'/'+[string]$child.LocalName
    if (-not (Test-NormalizedElement $child $childPath)) { Write-Output $child }
  }
}
function Get-SafeName([string]$Name) {
  if ($Name -cmatch '^[A-Za-z][A-Za-z0-9._-]{0,63}$') { return $Name }
  return 'UNSAFE'
}
function Compare-TaskElement($Expected,$Actual,[string]$LogicalPath,[string]$DisplayPath) {
  if ([string]$Expected.LocalName -cne [string]$Actual.LocalName) { return New-Comparison 'ELEMENT_NAME' $DisplayPath '' }
  if ([string]$Expected.NamespaceURI -cne [string]$Actual.NamespaceURI) { return New-Comparison 'ELEMENT_NAMESPACE' $DisplayPath '' }
  $expectedAttributes=@($Expected.Attributes | Sort-Object @{Expression={([string]$_.NamespaceURI)+'|'+([string]$_.LocalName)}})
  $actualAttributes=@($Actual.Attributes | Sort-Object @{Expression={([string]$_.NamespaceURI)+'|'+([string]$_.LocalName)}})
  if ($expectedAttributes.Count -ne $actualAttributes.Count) { return New-Comparison 'ATTRIBUTE_COUNT' $DisplayPath '' }
  for ($index=0; $index -lt $expectedAttributes.Count; $index++) {
    $field=Get-SafeName ([string]$expectedAttributes[$index].LocalName)
    if ([string]$expectedAttributes[$index].LocalName -cne [string]$actualAttributes[$index].LocalName) { return New-Comparison 'ATTRIBUTE_NAME' $DisplayPath $field }
    if ([string]$expectedAttributes[$index].NamespaceURI -cne [string]$actualAttributes[$index].NamespaceURI) { return New-Comparison 'ATTRIBUTE_NAMESPACE' $DisplayPath $field }
    if ([string]$expectedAttributes[$index].Value -cne [string]$actualAttributes[$index].Value) { return New-Comparison 'ATTRIBUTE_VALUE' $DisplayPath $field }
  }
  if ((Get-DirectText $Expected) -cne (Get-DirectText $Actual)) { return New-Comparison 'TEXT_VALUE' $DisplayPath '' }
  $expectedChildren=@(Get-ComparableChildren $Expected $LogicalPath)
  $actualChildren=@(Get-ComparableChildren $Actual $LogicalPath)
  $shared=[Math]::Min($expectedChildren.Count,$actualChildren.Count)
  for ($index=0; $index -lt $shared; $index++) {
    $safeName=Get-SafeName ([string]$expectedChildren[$index].LocalName)
    $childPath=$DisplayPath+'/'+$safeName+'['+($index+1)+']'
    if ([string]$expectedChildren[$index].LocalName -cne [string]$actualChildren[$index].LocalName) { return New-Comparison 'CHILD_SEQUENCE' $childPath '' }
    $difference=Compare-TaskElement $expectedChildren[$index] $actualChildren[$index] ($LogicalPath+'/'+[string]$expectedChildren[$index].LocalName) $childPath
    if ($null -ne $difference) { return $difference }
  }
  if ($expectedChildren.Count -gt $actualChildren.Count) {
    $safeName=Get-SafeName ([string]$expectedChildren[$shared].LocalName)
    return New-Comparison 'MISSING_CHILD' ($DisplayPath+'/'+$safeName+'['+($shared+1)+']') ''
  }
  if ($actualChildren.Count -gt $expectedChildren.Count) { return New-Comparison 'EXTRA_CHILD' $DisplayPath '' }
  return $null
}
function Compare-TaskDocuments($Expected,$Actual) {
  if ($null -eq $Expected -or $null -eq $Actual) { return New-Comparison 'PARSE_UNAVAILABLE' '' '' }
  $difference=Compare-TaskElement $Expected.DocumentElement $Actual.DocumentElement 'Task' '/Task[1]'
  if ($null -eq $difference) { return New-Comparison 'MATCH' '' '' }
  return $difference
}
function Get-TaskVersion($Document) {
  if ($null -eq $Document -or $null -eq $Document.DocumentElement) { return 'UNAVAILABLE' }
  $attribute=$Document.DocumentElement.GetAttributeNode('version')
  if ($null -eq $attribute) { return 'MISSING' }
  switch -CaseSensitive ([string]$attribute.Value) {
    '1.1' {return 'V1_1'}
    '1.2' {return 'V1_2'}
    '1.3' {return 'V1_3'}
    '1.4' {return 'V1_4'}
    default {return 'OTHER'}
  }
}
function Decode-TaskBytes([byte[]]$Bytes) {
  if ($Bytes.Length -eq 0) { return [pscustomobject]@{encoding='EMPTY';decode='FAILED';text=$null} }
  $encoding='UTF8_NO_BOM';$offset=0;$decoder=$null
  if ($Bytes.Length -ge 3 -and $Bytes[0] -eq 0xef -and $Bytes[1] -eq 0xbb -and $Bytes[2] -eq 0xbf) {
    $encoding='UTF8_BOM';$offset=3;$decoder=New-Object Text.UTF8Encoding($false,$true)
  } elseif ($Bytes.Length -ge 2 -and $Bytes[0] -eq 0xff -and $Bytes[1] -eq 0xfe) {
    $encoding='UTF16LE_BOM';$offset=2;$decoder=New-Object Text.UnicodeEncoding($false,$true,$true)
  } elseif ($Bytes.Length -ge 2 -and $Bytes[0] -eq 0xfe -and $Bytes[1] -eq 0xff) {
    $encoding='UTF16BE_BOM';$offset=2;$decoder=New-Object Text.UnicodeEncoding($true,$true,$true)
  } else {
    $sample=[Math]::Min($Bytes.Length,128);$evenNull=0;$oddNull=0
    for ($index=0; $index -lt $sample; $index++) { if ($Bytes[$index] -eq 0) { if (($index % 2) -eq 0) {$evenNull++} else {$oddNull++} } }
    if ($oddNull -ge 4 -and $evenNull -eq 0) {$encoding='UTF16LE_NO_BOM';$decoder=New-Object Text.UnicodeEncoding($false,$false,$true)}
    elseif ($evenNull -ge 4 -and $oddNull -eq 0) {$encoding='UTF16BE_NO_BOM';$decoder=New-Object Text.UnicodeEncoding($true,$false,$true)}
    else {$decoder=New-Object Text.UTF8Encoding($false,$true)}
  }
  try { return [pscustomobject]@{encoding=$encoding;decode='OK';text=$decoder.GetString($Bytes,$offset,$Bytes.Length-$offset)} }
  catch { return [pscustomobject]@{encoding=$encoding;decode='FAILED';text=$null} }
}
function Convert-TaskDocument($Decoded) {
  if ($Decoded.decode -cne 'OK' -or $null -eq $Decoded.text -or ([string]$Decoded.text).Length -gt 131072) { return $null }
  $reader=$null;$stringReader=$null
  try {
    $settings=New-Object Xml.XmlReaderSettings
    $settings.DtdProcessing=[Xml.DtdProcessing]::Prohibit
    $settings.XmlResolver=$null
    $settings.MaxCharactersInDocument=131072
    $settings.MaxCharactersFromEntities=0
    $stringReader=[IO.StringReader]::new([string]$Decoded.text)
    $reader=[Xml.XmlReader]::Create($stringReader,$settings)
    $document=New-Object Xml.XmlDocument
    $document.PreserveWhitespace=$false
    $document.XmlResolver=$null
    $document.Load($reader)
    if ($document.DocumentElement.LocalName -cne 'Task' -or $document.DocumentElement.NamespaceURI -cne 'http://schemas.microsoft.com/windows/2004/02/mit/task') { return $null }
    return $document
  } catch { return $null }
  finally { if ($null -ne $reader) {$reader.Dispose()};if ($null -ne $stringReader) {$stringReader.Dispose()} }
}
function Read-SchtasksBytes {
  $start=New-Object Diagnostics.ProcessStartInfo
  $start.FileName=[IO.Path]::Combine([Environment]::GetFolderPath('System'),'schtasks.exe')
  $start.Arguments='/Query /TN "\Home Lab Observer" /XML'
  $start.UseShellExecute=$false;$start.CreateNoWindow=$true;$start.RedirectStandardOutput=$true;$start.RedirectStandardError=$true
  $process=$null
  $memory=New-Object IO.MemoryStream
  try {
    $deadline=[DateTime]::UtcNow.AddSeconds(5)
    $process=[Diagnostics.Process]::Start($start)
    $discardError=$process.StandardError.BaseStream.CopyToAsync([IO.Stream]::Null)
    $buffer=New-Object byte[] 4096
    while ($true) {
      $remaining=[int][Math]::Ceiling(($deadline-[DateTime]::UtcNow).TotalMilliseconds)
      if ($remaining -le 0) { throw 'bounded query timed out' }
      $read=$process.StandardOutput.BaseStream.ReadAsync($buffer,0,$buffer.Length)
      if (-not $read.Wait($remaining)) { throw 'bounded query timed out' }
      $count=$read.Result
      if ($count -eq 0) { break }
      if ($memory.Length+$count -gt 131072) { throw 'bounded query output exceeded' }
      $memory.Write($buffer,0,$count)
    }
    $remaining=[int][Math]::Ceiling(($deadline-[DateTime]::UtcNow).TotalMilliseconds)
    if ($remaining -le 0 -or -not $process.WaitForExit($remaining)) { throw 'bounded query timed out' }
    $remaining=[int][Math]::Ceiling(($deadline-[DateTime]::UtcNow).TotalMilliseconds)
    if ($remaining -le 0 -or -not $discardError.Wait($remaining)) { throw 'bounded query timed out' }
    return [pscustomobject]@{exit=if($process.ExitCode -eq 0){'ZERO'}else{'NONZERO'};bytes=[Convert]::ToBase64String($memory.ToArray())}
  } finally {
    if ($null -ne $process) {
      try { if (-not $process.HasExited) {$process.Kill();[void]$process.WaitForExit(2000)} } catch {}
      $process.Dispose()
    }
    $memory.Dispose()
  }
}
try {
  $expectedPath=[IO.Path]::Combine($env:OBSERVER_SMOKE_INSTALL_ROOT,'background','task.xml')
  $expectedItem=Get-Item -LiteralPath $expectedPath -Force
  if ($expectedItem.PSIsContainer -or ($expectedItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or $expectedItem.Length -gt 131072) { throw 'unsafe expected task file' }
  $expectedDecoded=Decode-TaskBytes ([IO.File]::ReadAllBytes($expectedPath))
  $expectedDocument=Convert-TaskDocument $expectedDecoded
  $service=New-Object -ComObject 'Schedule.Service'
  $service.Connect()
  $task=$service.GetFolder('\').GetTask('\Home Lab Observer')
  $comText=[string]$task.Xml
  if ($comText.Length -gt 131072 -or [Text.Encoding]::UTF8.GetByteCount($comText) -gt 131072) { throw 'bounded COM task XML exceeded' }
  $comDecoded=[pscustomobject]@{encoding='DOTNET_STRING';decode='OK';text=$comText}
  $comDocument=Convert-TaskDocument $comDecoded
  $queryExit='START_FAILED';$queryDecoded=[pscustomobject]@{encoding='UNKNOWN';decode='NOT_RUN';text=$null};$queryDocument=$null
  try {
    $query=Read-SchtasksBytes
    $queryExit=$query.exit
    if ($queryExit -ceq 'ZERO') {
      $queryDecoded=Decode-TaskBytes ([Convert]::FromBase64String([string]$query.bytes))
      $queryDocument=Convert-TaskDocument $queryDecoded
    }
  } catch {}
  [ordered]@{
    code='TASK_XML_DIAGNOSTIC_AVAILABLE'
    expected_encoding=$expectedDecoded.encoding
    expected_parse=if($null -ne $expectedDocument){'OK'}else{'FAILED'}
    com_parse=if($null -ne $comDocument){'OK'}else{'FAILED'}
    query_exit=$queryExit
    query_encoding=$queryDecoded.encoding
    query_decode=$queryDecoded.decode
    query_parse=if($null -ne $queryDocument){'OK'}else{'FAILED'}
    expected_version=Get-TaskVersion $expectedDocument
    com_version=Get-TaskVersion $comDocument
    query_version=Get-TaskVersion $queryDocument
    expected_vs_com=Compare-TaskDocuments $expectedDocument $comDocument
    com_vs_query=Compare-TaskDocuments $comDocument $queryDocument
  } | ConvertTo-Json -Compress -Depth 4
} catch {
  $code=if (($_.Exception.HResult -band 0xffff) -eq 2) {'TASK_XML_TASK_MISSING'} else {'TASK_XML_DIAGNOSTIC_FAILED'}
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
    assert(Buffer.byteLength(output, "utf8") <= 16_384, "task XML diagnostic output exceeded its fixed bound");
    return validateTaskXMLShape(JSON.parse(output));
  } catch {
    return { code: "TASK_XML_DIAGNOSTIC_FAILED" };
  }
}

function validateTaskXMLShape(value) {
  assert(value && typeof value === "object" && !Array.isArray(value), "task XML diagnostic must be an object");
  const allowedCodes = new Set(["TASK_XML_DIAGNOSTIC_AVAILABLE", "TASK_XML_TASK_MISSING", "TASK_XML_DIAGNOSTIC_FAILED"]);
  assert(allowedCodes.has(value.code), "task XML diagnostic returned an unknown code");
  if (value.code !== "TASK_XML_DIAGNOSTIC_AVAILABLE") {
    assert.deepEqual(Object.keys(value), ["code"], "unavailable task XML diagnostic returned extra data");
    return { code: value.code };
  }
  const expectedKeys = ["code", "com_parse", "com_version", "com_vs_query", "expected_encoding", "expected_parse", "expected_version", "expected_vs_com", "query_decode", "query_encoding", "query_exit", "query_parse", "query_version"];
  assert.deepEqual(Object.keys(value).sort(), expectedKeys.sort(), "task XML diagnostic shape changed");
  const parseStates = new Set(["OK", "FAILED"]);
  assert(parseStates.has(value.expected_parse) && parseStates.has(value.com_parse) && parseStates.has(value.query_parse), "task XML parse state is invalid");
  assert(new Set(["ZERO", "NONZERO", "START_FAILED"]).has(value.query_exit), "task XML query exit state is invalid");
  assert(new Set(["OK", "FAILED", "NOT_RUN"]).has(value.query_decode), "task XML query decode state is invalid");
  const encodings = new Set(["UTF8_NO_BOM", "UTF8_BOM", "UTF16LE_BOM", "UTF16BE_BOM", "UTF16LE_NO_BOM", "UTF16BE_NO_BOM", "EMPTY", "UNKNOWN"]);
  assert(encodings.has(value.expected_encoding) && encodings.has(value.query_encoding), "task XML encoding state is invalid");
  const versions = new Set(["V1_1", "V1_2", "V1_3", "V1_4", "MISSING", "OTHER", "UNAVAILABLE"]);
  assert(versions.has(value.expected_version) && versions.has(value.com_version) && versions.has(value.query_version), "task XML version state is invalid");
  const namePattern = /^[A-Za-z][A-Za-z0-9._-]{0,63}$/;
  const pathPattern = /^(?:|\/(?:[A-Za-z][A-Za-z0-9._-]{0,63})\[[1-9][0-9]{0,3}\](?:\/(?:[A-Za-z][A-Za-z0-9._-]{0,63})\[[1-9][0-9]{0,3}\])*)$/;
  const comparisonKinds = new Set(["MATCH", "PARSE_UNAVAILABLE", "ELEMENT_NAME", "ELEMENT_NAMESPACE", "ATTRIBUTE_COUNT", "ATTRIBUTE_NAME", "ATTRIBUTE_NAMESPACE", "ATTRIBUTE_VALUE", "TEXT_VALUE", "CHILD_SEQUENCE", "MISSING_CHILD", "EXTRA_CHILD"]);
  const validateComparison = (entry) => {
    assert(entry && typeof entry === "object" && !Array.isArray(entry), "task XML comparison must be an object");
    assert.deepEqual(Object.keys(entry).sort(), ["field", "kind", "path"], "task XML comparison shape changed");
    assert(comparisonKinds.has(entry.kind), "task XML comparison kind is invalid");
    assert(typeof entry.path === "string" && entry.path.length <= 512 && pathPattern.test(entry.path), "task XML comparison path is unsafe");
    assert(entry.field === "" || (typeof entry.field === "string" && namePattern.test(entry.field)), "task XML comparison field is unsafe");
    return entry;
  };
  return {
    ...value,
    expected_vs_com: validateComparison(value.expected_vs_com),
    com_vs_query: validateComparison(value.com_vs_query),
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
