#!/usr/bin/env node

import { createHash } from "node:crypto";
import { execFileSync, spawn } from "node:child_process";
import { chmod, lstat, mkdtemp, mkdir, readFile, readdir, realpath, rm, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { createServer } from "node:net";
import os from "node:os";
import path from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { waitForProcessExit as waitForExit } from "./wait-for-process-exit.mjs";

const PLATFORMS = [
  ["linux", "amd64", "tar.gz"], ["linux", "arm64", "tar.gz"],
  ["darwin", "amd64", "tar.gz"], ["darwin", "arm64", "tar.gz"],
  ["windows", "amd64", "zip"], ["windows", "arm64", "zip"],
];

function fail(message) { throw new Error(`native delivery smoke failed: ${message}`); }

function parseArgs(argv) {
  const values = new Map();
  for (let index = 0; index < argv.length; index += 2) {
    const name = argv[index];
    const value = argv[index + 1];
    if (!name?.startsWith("--") || value === undefined || value.startsWith("--") || values.has(name)) fail("invalid arguments");
    values.set(name, value);
  }
  for (const name of values.keys()) {
    if (!["--mode", "--version", "--commit", "--directory", "--repository"].includes(name)) fail(`unknown argument ${name}`);
  }
  return values;
}

function names(version) {
  return PLATFORMS.map(([platform, arch, format]) => `home-lab-observer_${version}_${platform}_${arch}.${format}`);
}

function parseChecksums(text, expectedNames) {
  const result = new Map();
  const lines = text.split(/\r?\n/u).filter(Boolean);
  for (const line of lines) {
    const match = /^([a-f0-9]{64})  ([A-Za-z0-9._-]+)$/.exec(line);
    if (!match || result.has(match[2])) fail("SHA256SUMS has malformed or duplicate entries");
    result.set(match[2], match[1]);
  }
  const actualNames = [...result.keys()].sort();
  if (actualNames.join("\0") !== [...expectedNames].sort().join("\0")) fail("SHA256SUMS does not cover the exact release payload");
  return result;
}

async function sha256(filename) {
  return createHash("sha256").update(await readFile(filename)).digest("hex");
}

async function verifyChecksums(directory, version) {
  const payload = [...names(version), "release-manifest.json", "install.sh", "install.ps1", "observer.spdx.json"];
  const expectedDirectory = [...payload, "SHA256SUMS"].sort();
  const actualDirectory = (await readdir(directory)).sort();
  if (actualDirectory.join("\0") !== expectedDirectory.join("\0")) fail("release directory contains missing or unexpected files");
  for (const filename of actualDirectory) {
    const stat = await lstat(path.join(directory, filename));
    const maximum = names(version).includes(filename) ? 220 * 1024 * 1024 : 1024 * 1024;
    if (!stat.isFile() || stat.isSymbolicLink() || stat.size < 1 || stat.size > maximum) {
      fail(`${filename} is not a bounded regular release file`);
    }
  }
  const checksums = parseChecksums(await readFile(path.join(directory, "SHA256SUMS"), "utf8"), payload);
  for (const [filename, expected] of checksums) {
    if (await sha256(path.join(directory, filename)) !== expected) fail(`checksum mismatch for ${filename}`);
  }
  return checksums;
}

function archiveMembers(directory, filename) {
  const isZip = filename.endsWith(".zip");
  const command = isZip && process.platform !== "win32" ? "unzip" : "tar";
  const args = isZip && process.platform !== "win32" ? ["-Z1", path.join(directory, filename)] : ["-tf", path.join(directory, filename)];
  return execFileSync(command, args, { encoding: "utf8", timeout: 30_000 })
    .split(/\r?\n/u).filter(Boolean).map((member) => member.replace(/\/$/u, ""));
}

async function verifyArchiveLayouts(directory, version) {
  for (const [platform, arch, format] of PLATFORMS) {
    const filename = `home-lab-observer_${version}_${platform}_${arch}.${format}`;
    const root = `home-lab-observer_${version}_${platform}_${arch}`;
    const binary = platform === "windows" ? "observer.exe" : "observer";
    const helper = platform === "windows" ? "Run-Observer.cmd" : platform === "darwin" ? "Run-Observer.command" : "run-observer.sh";
    const expected = [`${root}/${binary}`, `${root}/LICENSE`, `${root}/START-HERE.md`, `${root}/${helper}`].sort();
    const actual = archiveMembers(directory, filename).filter((member) => member !== root).sort();
    if (actual.join("\0") !== expected.join("\0")) fail(`${filename} does not have the exact four-file rooted layout`);
  }
}

function platformTuple() {
  const platform = process.platform === "win32" ? "windows" : process.platform;
  const arch = process.arch === "x64" ? "amd64" : process.arch === "arm64" ? "arm64" : process.arch;
  if (!PLATFORMS.some(([candidatePlatform, candidateArch]) => candidatePlatform === platform && candidateArch === arch)) {
    fail(`unsupported smoke runner ${platform}/${arch}`);
  }
  return { platform, arch };
}

function runObserver(executable, args) {
  if (process.platform === "win32" && executable.endsWith(".cmd")) {
    // cmd.exe consumes its own outer quotes; Node's usual C-runtime argument
    // escaping would turn the quoted path into literal backslash-quote bytes.
    return execFileSync("cmd.exe", ["/d", "/s", "/c", `""${executable}" ${args.join(" ")}"`], {
      encoding: "utf8", timeout: 15_000, windowsVerbatimArguments: true,
    });
  }
  return execFileSync(executable, args, { encoding: "utf8", timeout: 15_000 });
}

function verifyVersion(output, version, commit, platform, arch) {
  const parsed = JSON.parse(output);
  const expectedKeys = ["arch", "commit", "go_version", "os", "schema_version", "version"].sort();
  if (Object.keys(parsed).sort().join("\0") !== expectedKeys.join("\0")) fail("version JSON is not the closed observer-build/v1 shape");
  if (parsed.schema_version !== "observer-build/v1" || parsed.version !== version || parsed.commit !== commit || parsed.os !== platform || parsed.arch !== arch) {
    fail("native executable does not identify the requested version, commit, OS and architecture");
  }
  if (typeof parsed.go_version !== "string" || parsed.go_version.length === 0) fail("native executable omitted the Go version");
}

async function freePort() {
  const server = createServer();
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  if (address === null || typeof address === "string") fail("could not allocate a local smoke-test port");
  await new Promise((resolve) => server.close(resolve));
  return address.port;
}

async function smokeAuthenticatedService(executable, temporary) {
  const port = await freePort();
  const state = path.join(temporary, "service state");
  await mkdir(state);
  const processHandle = spawn(executable, ["serve", "--listen", `127.0.0.1:${port}`, "--state-dir", state], {
    stdio: ["ignore", "pipe", "pipe"],
  });
  let spawnError;
  processHandle.once("error", (error) => { spawnError = error; });
  let output = "";
  for (const stream of [processHandle.stdout, processHandle.stderr]) {
    stream.on("data", (chunk) => { output = (output + chunk).slice(-16_384); });
  }
  try {
    const deadline = Date.now() + 45_000;
    let live = false;
    while (Date.now() < deadline && processHandle.exitCode === null) {
      if (spawnError) fail(`packaged service could not start: ${spawnError.message}`);
      try {
        const response = await fetch(`http://127.0.0.1:${port}/health/live`, { signal: AbortSignal.timeout(2_000) });
        if (response.ok) { live = true; break; }
      } catch { /* The listener may still be starting. */ }
      await delay(200);
    }
    if (!live) fail(`packaged service did not become live: ${output}`);
    const token = (await readFile(path.join(state, "local-api.token"), "utf8")).trim();
    if (token.length < 32) fail("packaged service did not create a strong local token");
    const unauthorized = await fetch(`http://127.0.0.1:${port}/api/v1/capabilities`, { signal: AbortSignal.timeout(5_000) });
    if (unauthorized.status !== 401) fail("packaged API did not enforce bearer authentication");
    const authorized = await fetch(`http://127.0.0.1:${port}/api/v1/capabilities`, {
      headers: { Authorization: `Bearer ${token}` },
      signal: AbortSignal.timeout(5_000),
    });
    const body = await authorized.text();
    if (!authorized.ok || Buffer.byteLength(body) > 131_072) fail("packaged capabilities endpoint failed its response bound");
    if (body.includes(token) || output.includes(token)) fail("packaged service leaked its local access token");
    const capabilities = JSON.parse(body);
    if (capabilities.schema_version !== "observer-capabilities/v1") fail("packaged capabilities response used the wrong contract");
  } finally {
    if (processHandle.pid !== undefined && processHandle.exitCode === null) {
      processHandle.kill("SIGTERM");
      if (!(await waitForExit(processHandle, 5_000))) {
        processHandle.kill("SIGKILL");
        if (!(await waitForExit(processHandle, 5_000))) fail("packaged service did not stop after forced termination");
      }
    }
  }
}

async function nativeSmoke(directory, version, commit, checksums) {
  const { platform, arch } = platformTuple();
  const format = platform === "windows" ? "zip" : "tar.gz";
  const archiveName = `home-lab-observer_${version}_${platform}_${arch}.${format}`;
  const archive = path.join(directory, archiveName);
  const temporary = await realpath(await mkdtemp(path.join(os.tmpdir(), "observer native delivery ")));
  let preserveForManagerFailure = false;
  try {
    execFileSync("tar", ["-xf", archive, "-C", temporary], { stdio: "inherit", timeout: 30_000 });
    const root = path.join(temporary, `home-lab-observer_${version}_${platform}_${arch}`);
    const executable = path.join(root, platform === "windows" ? "observer.exe" : "observer");
    if (platform !== "windows") await chmod(executable, 0o755);
    verifyVersion(runObserver(executable, ["version", "--json"]), version, commit, platform, arch);
    await smokeAuthenticatedService(executable, temporary);
    // Exercise the exact packaged binary on every native release architecture.
    // This starts only temporary foreground children, never a login registration.
    execFileSync(process.execPath, [fileURLToPath(new URL("./smoke-background-runtime.mjs", import.meta.url))], {
      env: { ...process.env, OBSERVER_SMOKE_BINARY: executable },
      stdio: "inherit", timeout: 180_000, windowsHide: true,
    });

    const installRoot = path.join(temporary, "managed install root");
    const stateSentinel = path.join(temporary, "observer-state-must-survive.txt");
    await writeFile(stateSentinel, "preserve me", "utf8");
    if (platform === "windows") {
      const powershell = `${process.env.SystemRoot ?? "C:\\Windows"}\\System32\\WindowsPowerShell\\v1.0\\powershell.exe`;
      const installer = path.join(directory, "install.ps1");
      const common = ["-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", installer];
      execFileSync(powershell, [...common, "-Version", version, "-Archive", archive, "-Checksum", checksums.get(archiveName), "-InstallRoot", installRoot], { stdio: "inherit", timeout: 60_000 });
      const launcher = path.join(installRoot, "bin", "observer.cmd");
      verifyVersion(runObserver(launcher, ["version", "--json"]), version, commit, platform, arch);
      if (process.env.OBSERVER_TEST_USER_MANAGER === "1") {
        preserveForManagerFailure = true;
        execFileSync(process.execPath, [fileURLToPath(new URL("./smoke-windows-manager.mjs", import.meta.url))], {
          env: { ...process.env, OBSERVER_SMOKE_BINARY: executable, OBSERVER_SMOKE_INSTALL_ROOT: installRoot },
          stdio: "inherit", timeout: 150_000, windowsHide: true,
        });
        preserveForManagerFailure = false;
      }
      execFileSync(powershell, [...common, "-Uninstall", "-InstallRoot", installRoot], { stdio: "inherit", timeout: 30_000 });
    } else {
      const installer = path.join(directory, "install.sh");
      execFileSync("bash", [installer, "--version", version, "--archive", archive, "--checksum", checksums.get(archiveName), "--install-root", installRoot], { stdio: "inherit", timeout: 60_000 });
      const launcher = path.join(installRoot, "bin", "observer");
      verifyVersion(runObserver(launcher, ["version", "--json"]), version, commit, platform, arch);
      execFileSync("bash", [installer, "--uninstall", "--install-root", installRoot], { stdio: "inherit", timeout: 30_000 });
    }
    if (await readFile(stateSentinel, "utf8") !== "preserve me") fail("uninstall modified observer state outside its managed root");
  } finally {
    if (preserveForManagerFailure) {
      console.error("Preserving disposable runner files because manager cleanup was not confirmed.");
    } else {
      await rm(temporary, { recursive: true, force: true });
    }
  }
}

async function anonymousSmoke(version, repository) {
  if (process.env.GH_TOKEN || process.env.GITHUB_TOKEN) fail("anonymous download smoke must not receive a GitHub token");
  if (repository !== "braidenm/home-lab-observer") fail("anonymous smoke repository must be the canonical public repository");
  const directory = await mkdtemp(path.join(os.tmpdir(), "observer anonymous release "));
  try {
    const filenames = [...names(version), "release-manifest.json", "install.sh", "install.ps1", "observer.spdx.json", "SHA256SUMS"];
    for (const filename of filenames) {
      const url = `https://github.com/${repository}/releases/download/v${version}/${filename}`;
      const response = await fetch(url, { redirect: "follow", headers: { Accept: "application/octet-stream" }, signal: AbortSignal.timeout(60_000) });
      if (!response.ok) fail(`anonymous download returned ${response.status} for ${filename}`);
      if (response.body === null) fail(`anonymous ${filename} had no response body`);
      const maximum = names(version).includes(filename) ? 220 * 1024 * 1024 : 1024 * 1024;
      const chunks = [];
      let received = 0;
      for await (const chunk of response.body) {
        received += chunk.length;
        if (received > maximum) fail(`anonymous ${filename} exceeded its size limit`);
        chunks.push(chunk);
      }
      if (received < 1) fail(`anonymous ${filename} was empty`);
      await writeFile(path.join(directory, filename), Buffer.concat(chunks, received));
    }
    await verifyChecksums(directory, version);
    await verifyArchiveLayouts(directory, version);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const mode = args.get("--mode") ?? fail("--mode is required");
  const version = args.get("--version") ?? fail("--version is required");
  if (!/^0\.1\.0-preview\.[1-9][0-9]*$/.test(version)) fail("invalid preview version");
  if (mode === "anonymous") {
    await anonymousSmoke(version, args.get("--repository") ?? "braidenm/home-lab-observer");
  } else {
    const directory = path.resolve(args.get("--directory") ?? fail("--directory is required"));
    const checksums = await verifyChecksums(directory, version);
    await verifyArchiveLayouts(directory, version);
    if (mode === "native") {
      const commit = args.get("--commit") ?? fail("--commit is required for native mode");
      if (!/^[a-f0-9]{40}$/.test(commit)) fail("invalid commit SHA");
      await nativeSmoke(directory, version, commit, checksums);
    } else if (mode !== "bundle") {
      fail(`unknown mode ${mode}`);
    }
  }
  console.log(`${mode} native delivery smoke passed for ${version}`);
}

main().catch((error) => {
  console.error(error.message);
  process.exit(1);
});
