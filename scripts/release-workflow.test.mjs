import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { EventEmitter } from "node:events";
import { waitForProcessExit } from "./wait-for-process-exit.mjs";
import { managerSmokeFailure } from "./windows-manager-smoke-failure.mjs";
import { verifyArchivedHelper, verifyHelperFile, verifyManifest } from "./smoke-native-delivery.mjs";

test("process exit waits release listeners and recognize signal termination", async () => {
  const child = Object.assign(new EventEmitter(), { exitCode: null, signalCode: null });
  const stopped = waitForProcessExit(child, 1000);
  child.emit("exit", 0);
  assert.equal(await stopped, true);
  assert.equal(child.listenerCount("exit"), 0);
  assert.equal(await waitForProcessExit(child, 1), false);
  assert.equal(child.listenerCount("exit"), 0);
  child.signalCode = "SIGTERM";
  assert.equal(await waitForProcessExit(child, 1000), true);
  assert.equal(child.listenerCount("exit"), 0);
});

test("manager smoke preserves sanitized task shape when cleanup is unconfirmed", () => {
  const primary = new Error('observer background enable failed (BACKGROUND_REGISTRATION_MISMATCH); task_xml_shape={"code":"TASK_XML_DIAGNOSTIC_AVAILABLE","expected_vs_com":{"kind":"TEXT_VALUE","path":"/Task[1]/Principals[1]/Principal[1]/UserId[1]","field":""}}');
  const failure = managerSmokeFailure(primary, false);
  assert(failure instanceof Error);
  assert.match(failure.message, /BACKGROUND_REGISTRATION_MISMATCH/);
  assert.match(failure.message, /TASK_XML_DIAGNOSTIC_AVAILABLE/);
  assert.match(failure.message, /cleanup=BACKGROUND_CLEANUP_UNCONFIRMED$/);
});

const repositoryRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
// Exercise the actual CI fixture identity through finalization and manifest checks.
const version = "0.1.0-preview.999999";
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

async function fixture(directory, schemaVersion = "observer-release/v1") {
  const assets = [];
  for (const [platform, arch, format] of platforms) {
    const filename = `home-lab-observer_${version}_${platform}_${arch}.${format}`;
    await writeFile(path.join(directory, filename), `final-${platform}-${arch}-archive-bytes`, "utf8");
    const digest = await shaAndSize(path.join(directory, filename));
    const asset = {
      os: platform,
      arch,
      filename,
      sha256: digest.sha256,
      size_bytes: digest.size_bytes,
      format,
      download_url: `https://github.com/braidenm/home-lab-observer/releases/download/v${version}/${filename}`,
    };
    if (schemaVersion === "observer-release/v2") {
      asset.content_profile = platform === "linux" ? "linux-journal-helper-v1" : "native-core-v1";
      asset.journal_helper = platform === "linux"
        ? { filename: "observer-journal-helper", sha256: createHash("sha256").update(`helper-${arch}`).digest("hex"), size_bytes: 13 }
        : null;
    }
    assets.push(asset);
  }
  const manifest = {
    schema_version: schemaVersion,
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

async function readChecksums(directory) {
  return new Map((await readFile(path.join(directory, "SHA256SUMS"), "utf8")).trim().split(/\r?\n/u).map((line) => [line.slice(66), line.slice(0, 64)]));
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

test("v2 finalization keeps the download set stable and validates explicit helper profiles", async () => {
  const directory = await mkdtemp(path.join(os.tmpdir(), "observer release v2 finalize "));
  const schema = path.join(repositoryRoot, "schemas", "release-v2.schema.json");
  try {
    await fixture(directory, "observer-release/v2");
    const result = run("release-finalize.mjs", ["--version", version, "--commit", commit, "--directory", directory, "--schema", schema]);
    assert.equal(result.status, 0, result.stderr);
    const checksums = await readChecksums(directory);
    assert.equal(checksums.size, 10);
    const release = await verifyManifest(directory, version, checksums);
    assert.equal(release.schemaVersion, "observer-release/v2");
    assert.equal(release.assets.get("linux/amd64").journal_helper.filename, "observer-journal-helper");

    const manifestPath = path.join(directory, "release-manifest.json");
    const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
    manifest.assets.find((asset) => asset.os === "linux").content_profile = "native-core-v1";
    await writeFile(manifestPath, JSON.stringify(manifest), "utf8");
    assert.notEqual(run("release-finalize.mjs", ["--version", version, "--commit", commit, "--directory", directory, "--schema", schema]).status, 0);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("delivery helper verification is bounded and matches manifest bytes", async () => {
  const directory = await mkdtemp(path.join(os.tmpdir(), "observer release helper verify "));
  const helper = path.join(directory, "observer-journal-helper");
  try {
    await writeFile(helper, "synthetic-helper", "utf8");
    const metadata = await shaAndSize(helper);
    await verifyHelperFile(helper, metadata);
    await assert.rejects(() => verifyHelperFile(helper, { ...metadata, sha256: "0".repeat(64) }), /helper bytes/u);
    await assert.rejects(() => verifyHelperFile(helper, { ...metadata, size_bytes: metadata.size_bytes + 1 }), /bounded regular file/u);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("delivery streams the exact helper member from a real archive", async () => {
  const directory = await mkdtemp(path.join(os.tmpdir(), "observer release helper archive "));
  const root = "home-lab-observer_0.1.0-preview.7_linux_amd64";
  const helperMember = `${root}/observer-journal-helper`;
  const helper = path.join(directory, root, "observer-journal-helper");
  const archive = path.join(directory, "helper.tar.gz");
  try {
    await mkdir(path.dirname(helper), { recursive: true });
    await writeFile(helper, "synthetic-archived-helper", "utf8");
    const packed = spawnSync("tar", ["-czf", archive, "-C", directory, helperMember], { encoding: "utf8" });
    assert.equal(packed.status, 0, packed.stderr);
    const metadata = await shaAndSize(helper);
    await verifyArchivedHelper(archive, helperMember, metadata);
    await assert.rejects(() => verifyArchivedHelper(archive, helperMember, { ...metadata, size_bytes: metadata.size_bytes - 1 }), /does not match/u);
    await assert.rejects(() => verifyArchivedHelper(archive, helperMember, { ...metadata, sha256: "0".repeat(64) }), /does not match/u);
    await assert.rejects(() => verifyArchivedHelper(archive, `${root}/missing-helper`, metadata), /does not match/u);
  } finally {
    await rm(directory, { recursive: true, force: true });
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
  assert.match(release, /RELEASE_ACTOR: \$\{\{ github.actor \}\}/u);
  assert.match(release, /RELEASE_TRIGGERING_ACTOR: \$\{\{ github.triggering_actor \}\}/u);
  for (const job of ["attest", "publish"]) {
    assert.match(release, new RegExp(`  ${job}:\\n    if: .*github\\.actor == github\\.repository_owner && github\\.triggering_actor == github\\.repository_owner`));
  }
  assert.match(release, /"\$RELEASE_ACTOR" == "\$RELEASE_OWNER" && "\$RELEASE_TRIGGERING_ACTOR" == "\$RELEASE_OWNER"/u);
  assert.match(release, /gh api --method POST .+\/git\/refs -f ref="refs\/tags\/\$\{tag\}" -f sha="\$COMMIT"/u);
  assert.match(release, /gh release create "\$tag" --verify-tag/u);
  assert.match(release, /verify_tag\n\s+gh release edit "\$tag" --draft=false --prerelease\n\s+verify_tag/u);
  assert.match(release, /inputs\.immutability_confirmed/u);
  assert.match(release, /releases\/tags\/\$\{tag\} --jq \.immutable/u);
  assert.match(release, /id-token: write/u);
  assert.match(release, /attestations: write/u);
  assert.match(release, /contents: write/u);
  assert.doesNotMatch(delivery, /(id-token|attestations): write/u);
  assert.doesNotMatch(delivery, /contents: write/u);
  for (const workflow of [delivery, release]) {
    assert.match(workflow, /syft-version: v1\.51\.1/u);
    for (const line of workflow.match(/^\s*- uses: .+$/gmu) ?? []) {
      assert.match(line, /@[a-f0-9]{40}(?:\s+#|$)/u, `action is not full-SHA pinned: ${line}`);
    }
  }
  for (const runner of ["ubuntu-24.04", "ubuntu-24.04-arm", "macos-15-intel", "macos-15", "windows-2025", "windows-11-arm"]) {
    assert.match(release, new RegExp(`runner: ${runner.replaceAll(".", "\\.")}`));
  }
  assert.match(release, /needs: \[authorize, build, vulnerability-scan, native-verify\]/u);
  assert.match(release, /needs: \[authorize, build, attest\]/u);
  assert.match(release, /--notes-file docs\/releases\/native-preview\.md/u);
  for (const workflow of [delivery, release]) {
    assert.match(workflow, /Install schema validators for packaged runtime smoke\n\s+run: npm ci --ignore-scripts --no-audit --no-fund/u);
    assert.match(workflow, /OBSERVER_TEST_USER_MANAGER: '1'/u);
  }
  const smoke = await readFile(path.join(repositoryRoot, "scripts/smoke-native-delivery.mjs"), "utf8");
  assert.match(smoke, /smoke-background-runtime\.mjs/u);
  assert.match(smoke, /smoke-windows-manager\.mjs/u);
  assert.match(smoke, /OBSERVER_SMOKE_INSTALL_ROOT: installRoot/u);
  assert.match(smoke, /if \(preserveForManagerFailure\)/u);
  assert.match(smoke, /"--manifest".+"--checksums"/u);
  assert.match(smoke, /"-Manifest".+"-Checksums"/u);
  assert.match(smoke, /verifyHelperFile/u);
});

test("paired native delivery uses verified checkout and isolated reproducibility gate", async () => {
  const delivery = await readFile(path.join(repositoryRoot, ".github/workflows/native-delivery.yml"), "utf8");
  const proof = await readFile(path.join(repositoryRoot, ".github/workflows/native-reproducibility.yml"), "utf8");
  assert.match(delivery, /test "\$\(git rev-parse HEAD\)" = "\$GITHUB_SHA"/u);
  assert.match(delivery, /bash scripts\/build-native-v2\.sh "\$PREVIEW_VERSION" "\$GITHUB_SHA" dist\/binaries/u);
  assert.match(delivery, /--schema-version observer-release\/v2/u);
  assert(delivery.includes(`PREVIEW_VERSION: ${version}`));
  assert.match(delivery, /--schema schemas\/release-v2\.schema\.json/u);
  assert.match(delivery, /run: node scripts\/test-missing-linux-runtime\.mjs dist\/release/u);
  assert(delivery.indexOf('run: node scripts/test-missing-linux-runtime.mjs') < delivery.indexOf('name: Generate SPDX'));
  assert.match(proof, /runs-on: ubuntu-24\.04/u);
  assert.match(proof, /timeout-minutes: 12/u);
  assert.match(proof, /OBSERVER_TEST_REPRODUCIBLE_BUILDS: '1'/u);
  assert.match(proof, /run: bash scripts\/test-build-native-v2\.sh/u);
  assert.doesNotMatch(proof, /(?:contents|id-token|attestations): write|self-hosted/u);
  for (const line of proof.match(/^\s*- uses: .+$/gmu) ?? []) {
    assert.match(line, /@[a-f0-9]{40}(?:\s+#|$)/u);
  }
});

test("publication promotes the reviewed v2 profile and installs its smoke dependencies", async () => {
  const release = await readFile(path.join(repositoryRoot, ".github/workflows/native-release.yml"), "utf8");
  const build = release.split("\n  build:\n")[1].split("\n  vulnerability-scan:\n")[0];
  const publish = release.split("\n  publish:\n")[1];
  assert.match(build, /test "\$\(git rev-parse HEAD\)" = "\$COMMIT"/u);
  assert.match(build, /bash scripts\/build-native-v2\.sh "\$VERSION" "\$COMMIT" dist\/binaries/u);
  assert.doesNotMatch(build, /mkdir -p dist\/binaries|-X main\.(?:version|commit)=/u);
  assert.match(build, /--schema-version observer-release\/v2/u);
  assert.match(build, /--schema schemas\/release-v2\.schema\.json/u);
  assert.doesNotMatch(build, /release-v1\.schema\.json/u);
  const pack = build.indexOf("run: go run ./cmd/releasepack");
  const scratch = build.indexOf("run: node scripts/test-missing-linux-runtime.mjs dist/release");
  const sbom = build.indexOf("name: Generate SPDX");
  assert(pack >= 0 && scratch > pack && sbom > scratch, "scratch probe must see unfinalized verified v2 artifacts");
  assert.match(build, /node --test scripts\/test-missing-linux-runtime\.test\.mjs/u);
  assert.doesNotMatch(build, /run: bash scripts\/test-build-native-v2\.sh/u);
  assert.match(build, /independent Native reproducibility check for the authorized/u);
  const install = publish.indexOf("run: npm ci --ignore-scripts --no-audit --no-fund");
  const bundle = publish.indexOf("node scripts/smoke-native-delivery.mjs --mode bundle");
  assert(install >= 0 && bundle > install, "fresh publish runner needs Ajv before importing smoke in any mode");
  assert.match(publish, /node scripts\/smoke-native-delivery\.mjs --mode anonymous/u);
});
