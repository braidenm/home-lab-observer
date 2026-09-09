import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const repositoryRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const version = "0.1.0-preview.7";
const commit = "0123456789abcdef0123456789abcdef01234567";
const platforms = [
  ["linux", "amd64", "tar.gz"], ["linux", "arm64", "tar.gz"],
  ["darwin", "amd64", "tar.gz"], ["darwin", "arm64", "tar.gz"],
  ["windows", "amd64", "zip"], ["windows", "arm64", "zip"],
];

function run(script, args) {
  return spawnSync(process.execPath, [path.join(repositoryRoot, "scripts", script), ...args], {
    cwd: repositoryRoot,
    encoding: "utf8",
  });
}

async function shaAndSize(filename) {
  const bytes = await readFile(filename);
  return { sha256: createHash("sha256").update(bytes).digest("hex"), size_bytes: bytes.length };
}

async function fixture(directory) {
  const assets = [];
  for (const [platform, arch, format] of platforms) {
    const filename = `home-lab-observer_${version}_${platform}_${arch}.${format}`;
    await writeFile(path.join(directory, filename), `final-${platform}-${arch}-archive-bytes`, "utf8");
    const digest = await shaAndSize(path.join(directory, filename));
    assets.push({
      os: platform,
      arch,
      filename,
      sha256: digest.sha256,
      size_bytes: digest.size_bytes,
      format,
      download_url: `https://github.com/braidenm/home-lab-observer/releases/download/v${version}/${filename}`,
    });
  }
  const manifest = {
    schema_version: "observer-release/v1",
    version,
    tag: `v${version}`,
    repository: "braidenm/home-lab-observer",
    commit_sha: commit,
    signing_policy: "UNSIGNED_PREVIEW_WITH_CHECKSUMS_AND_PROVENANCE",
    assets,
  };
  await writeFile(path.join(directory, "release-manifest.json"), `${JSON.stringify(manifest)}\n`, "utf8");
  await writeFile(path.join(directory, "install.sh"), "#!/usr/bin/env bash\nexit 0\n", "utf8");
  await writeFile(path.join(directory, "install.ps1"), "exit 0\n", "utf8");
  await writeFile(path.join(directory, "observer.spdx.json"), JSON.stringify({ spdxVersion: "SPDX-2.3", dataLicense: "CC0-1.0", packages: [] }), "utf8");
  await writeFile(path.join(directory, "SHA256SUMS"), "provisional\n", "utf8");
}

async function schemaFixture() {
  const directory = await mkdtemp(path.join(os.tmpdir(), "observer release schema "));
  const filename = path.join(directory, "release.schema.json");
  await writeFile(filename, JSON.stringify({ $schema: "https://json-schema.org/draft/2020-12/schema", type: "object" }), "utf8");
  return { directory, filename };
}

test("release inputs accept only the explicit main-branch preview identity", () => {
  const accepted = run("release-validate-input.mjs", ["--version", version, "--commit", commit, "--ref", "refs/heads/main"]);
  assert.equal(accepted.status, 0, accepted.stderr);
  for (const rejectedArgs of [
    ["--version", "0.1.0", "--commit", commit],
    ["--version", "0.1.0-preview.0", "--commit", commit],
    ["--version", version, "--commit", commit.slice(0, -1)],
    ["--version", version, "--commit", commit, "--ref", "refs/heads/feature"],
  ]) {
    assert.notEqual(run("release-validate-input.mjs", rejectedArgs).status, 0);
  }
});

test("finalization replaces provisional sums with exact final-byte coverage", async () => {
  const directory = await mkdtemp(path.join(os.tmpdir(), "observer release finalize "));
  const schema = await schemaFixture();
  try {
    await fixture(directory);
    const result = run("release-finalize.mjs", ["--version", version, "--commit", commit, "--directory", directory, "--schema", schema.filename]);
    assert.equal(result.status, 0, result.stderr);
    const lines = (await readFile(path.join(directory, "SHA256SUMS"), "utf8")).trim().split("\n");
    assert.equal(lines.length, 10);
    assert.deepEqual(lines.map((line) => line.slice(66)).sort(), [
      ...platforms.map(([platform, arch, format]) => `home-lab-observer_${version}_${platform}_${arch}.${format}`),
      "release-manifest.json", "install.sh", "install.ps1", "observer.spdx.json",
    ].sort());
  } finally {
    await rm(directory, { recursive: true, force: true });
    await rm(schema.directory, { recursive: true, force: true });
  }
});

test("finalization rejects unknown assets and manifest drift", async () => {
  const directory = await mkdtemp(path.join(os.tmpdir(), "observer release reject "));
  const schema = await schemaFixture();
  try {
    await fixture(directory);
    await writeFile(path.join(directory, "debug-symbols.zip"), "not allowed", "utf8");
    assert.notEqual(run("release-finalize.mjs", ["--version", version, "--commit", commit, "--directory", directory, "--schema", schema.filename]).status, 0);
    await rm(path.join(directory, "debug-symbols.zip"));
    const manifestPath = path.join(directory, "release-manifest.json");
    const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
    manifest.assets[0].sha256 = "0".repeat(64);
    await writeFile(manifestPath, JSON.stringify(manifest), "utf8");
    assert.notEqual(run("release-finalize.mjs", ["--version", version, "--commit", commit, "--directory", directory, "--schema", schema.filename]).status, 0);
  } finally {
    await rm(directory, { recursive: true, force: true });
    await rm(schema.directory, { recursive: true, force: true });
  }
});

test("workflows keep publication manual, permission-scoped and fully pinned", async () => {
  const delivery = await readFile(path.join(repositoryRoot, ".github/workflows/native-delivery.yml"), "utf8");
  const release = await readFile(path.join(repositoryRoot, ".github/workflows/native-release.yml"), "utf8");
  assert.match(release, /^on:\n  workflow_dispatch:/mu);
  assert.doesNotMatch(release, /^  (push|pull_request|schedule):/mu);
  assert.match(release, /release-validate-input\.mjs.+--ref "\$GITHUB_REF"/u);
  assert.match(release, /inputs\.immutability_confirmed/u);
  assert.match(release, /releases\/tags\/\$\{tag\} --jq \.immutable/u);
  assert.match(release, /id-token: write/u);
  assert.match(release, /attestations: write/u);
  assert.match(release, /contents: write/u);
  assert.doesNotMatch(delivery, /(id-token|attestations): write/u);
  assert.doesNotMatch(delivery, /contents: write/u);
  for (const workflow of [delivery, release]) {
    for (const line of workflow.match(/^\s*- uses: .+$/gmu) ?? []) {
      assert.match(line, /@[a-f0-9]{40}(?:\s+#|$)/u, `action is not full-SHA pinned: ${line}`);
    }
  }
  for (const runner of ["ubuntu-24.04", "ubuntu-24.04-arm", "macos-15-intel", "macos-15", "windows-2025", "windows-11-arm"]) {
    assert.match(release, new RegExp(`runner: ${runner.replaceAll(".", "\\.")}`));
  }
  assert.match(release, /needs: \[authorize, build, vulnerability-scan, native-verify\]/u);
  assert.match(release, /needs: \[authorize, build, attest\]/u);
});
