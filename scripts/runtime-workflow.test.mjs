import assert from "node:assert/strict";
import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const linux = "runner.os == 'Linux'";
const other = "runner.os != 'Linux'";
const stress = "./internal/handoff -run TestConcurrentReaderNeverReturnsPartialDocument -count=25";
const runtimeCacheFiles = ["go.mod", "go.sum", ".github/workflows/runtime.yml"];

function goSetupBlocks(workflow) {
  const blocks = workflow.split(/\n(?= {6}- )/u).slice(1)
    .filter((block) => /^(?: {6}- | {8})uses: actions\/setup-go@[a-f0-9]{40}\b/mu.test(block));
  const uses = workflow.match(/^\s*(?:-\s+)?uses:\s*actions\/setup-go@/gmu) ?? [];
  assert.equal(blocks.length, uses.length, "every setup-go use must be parsed before checking cache ownership");
  return blocks;
}

function isWorkflow(name) {
  return /\.ya?ml$/u.test(name);
}

function cacheFiles(block) {
  const value = block.match(/^ {10}cache-dependency-path: \|\r?\n((?: {12}\S[^\r\n]*(?:\r?\n|$))+)/mu);
  return value ? value[1].trim().split(/\r?\n/u).map((line) => line.trim()) : [];
}

function checkRuntimeCache(block) {
  assert.match(block, /^ {10}cache: true$/mu);
  assert.deepEqual(cacheFiles(block), runtimeCacheFiles);
}

function steps(workflow) {
  return workflow.split(/\n(?= {6}- )/u).slice(1).map((block) => ({
    run: block.match(/^ {6}- run: (.+)$/mu)?.[1] ?? block.match(/^ {8}run: (.+)$/mu)?.[1],
    condition: block.match(/^ {8}if: (.+)$/mu)?.[1],
  }));
}

function raceConstrained(source) {
  return /^\s*\/\/\s*(?:go:build|\+build)\b[^\r\n]*\brace\b/mu.test(source);
}

async function goSources(directory) {
  const names = [];
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    if (entry.name === ".git" || entry.name === "node_modules") continue;
    const name = path.join(directory, entry.name);
    if (entry.isDirectory()) names.push(...await goSources(name));
    else if (entry.isFile() && entry.name.endsWith(".go")) names.push(name);
  }
  return names;
}

test("runtime runs every Linux package once under race and preserves platform stress", async () => {
  const workflow = await readFile(path.join(root, ".github/workflows/runtime.yml"), "utf8");
  const selected = steps(workflow);
  const expected = new Map([
    ["go test ./...", other],
    ["go test -race ./...", linux],
    [`go test ${stress}`, other],
    [`go test -race ${stress}`, linux],
  ]);
  assert.equal((workflow.match(/\bgo test\b/gu) ?? []).length, expected.size);
  for (const [command, condition] of expected) {
    const matches = selected.filter((step) => step.run === command);
    assert.equal(matches.length, 1, `exactly one bounded test command: ${command}`);
    assert.equal(matches[0].condition, condition, `platform selection: ${command}`);
  }
  const guard = selected.filter((step) => step.run === "node --test scripts/runtime-workflow.test.mjs");
  assert.equal(guard.length, 1, "the cheap workflow contract must run in this job");
  assert.equal(guard[0].condition, undefined);
  assert.match(workflow, /^  native-test:$/mu);
  assert.match(workflow, /^        os: \[ubuntu-latest, windows-latest, macos-latest\]$/mu);
  assert.match(workflow, /^    runs-on: \$\{\{ matrix\.os \}\}$/mu);
  assert.match(workflow, /^    timeout-minutes: 8$/mu);
  for (const command of [
    "go vet ./...",
    "node scripts/validate-runtime-snapshot.mjs",
    "node scripts/validate-localapi-scenarios.mjs",
    "node scripts/smoke-local-service.mjs",
    "node scripts/smoke-background-runtime.mjs",
    "node scripts/smoke-docker-service.mjs",
  ]) {
    const matches = selected.filter((step) => step.run === command);
    assert.equal(matches.length, 1, `retained runtime gate: ${command}`);
    assert.equal(matches[0].condition, command.endsWith("smoke-docker-service.mjs") ? linux : undefined);
  }
});

test("race build-constraint detection covers modern and legacy exclusions", () => {
  for (const header of [
    "//go:build !race\n", "//go:build linux && !race\n", "// +build linux,!race\n",
    "//go:build race || windows\n", "// +build race\n",
  ]) assert.equal(raceConstrained(header + "\npackage fixture\n"), true);
  for (const source of [
    "//go:build linux\n\npackage fixture\n", "// +build linux,amd64\n\npackage fixture\n",
    "package fixture\n// ordinary race discussion, not a build constraint\n",
  ]) assert.equal(raceConstrained(source), false);
});

test("all Go sources remain selected independently of the race build tag", async () => {
  const sources = await goSources(root);
  assert(sources.length > 0);
  for (const name of sources) {
    assert.equal(raceConstrained(await readFile(name, "utf8")), false,
      `race-dependent source requires a separately reviewed coverage strategy: ${path.relative(root, name)}`);
  }
});

test("runtime owns an exact module, checksum and build-plan cache key", async () => {
  const workflow = await readFile(path.join(root, ".github/workflows/runtime.yml"), "utf8");
  const blocks = goSetupBlocks(workflow);
  assert.equal(blocks.length, 1);
  checkRuntimeCache(blocks[0]);
});

test("runtime cache refuses missing, extra, duplicate and wrong input files", () => {
  const block = "      - uses: actions/setup-go@" + "a".repeat(40) + "\n        with:\n          cache: true\n";
  const configured = (files) => block + "          cache-dependency-path: |\n" + files.map((name) => `            ${name}\n`).join("");
  checkRuntimeCache(configured(runtimeCacheFiles));
  assert.throws(() => checkRuntimeCache(block));
  for (const files of [
    runtimeCacheFiles.slice(1), runtimeCacheFiles.filter((name) => name !== "go.sum"), runtimeCacheFiles.slice(0, 2),
    [...runtimeCacheFiles, "**/*.go"], [...runtimeCacheFiles, "go.sum"],
    ["go.mod", "go.sum", ".github/workflows/dashboard.yml"],
  ]) assert.throws(() => checkRuntimeCache(configured(files)));
  assert.throws(() => checkRuntimeCache(configured(runtimeCacheFiles).replace("cache: true", "cache: false")));
});

test("other Go workflows do not claim runtime's cache salt", async () => {
  const directory = path.join(root, ".github/workflows");
  let checked = 0;
  for (const name of await readdir(directory)) {
    if (name === "runtime.yml" || !isWorkflow(name)) continue;
    for (const block of goSetupBlocks(await readFile(path.join(directory, name), "utf8"))) {
      checked++;
      assert.equal(block.includes(".github/workflows/runtime.yml"), false,
        `runtime's build-plan salt must remain runtime-owned: ${name}`);
    }
  }
  assert(checked > 0);
});

test("Go setup parsing covers valid step-field order before uses", () => {
  const use = "uses: actions/setup-go@" + "a".repeat(40);
  for (const step of [
    `      - ${use}\n`,
    `      - name: Set up Go\n        ${use}\n`,
    `      - id: go\n        ${use}\n`,
    `      - if: runner.os == 'Linux'\n        ${use}\n`,
    `      - env:\n          FIXTURE: safe\n        ${use}\n`,
    `      - name: Set up Go\n        id: go\n        if: runner.os == 'Linux'\n        env:\n          FIXTURE: safe\n        ${use}\n`,
  ]) assert.equal(goSetupBlocks("jobs:\n" + step).length, 1);
});

test("step-field order cannot hide another workflow claiming runtime's salt", () => {
  for (const literalStyle of ["|", "|-"]) {
    const workflow = "jobs:\n      - name: Set up Go\n        id: go\n        if: runner.os == 'Linux'\n" +
      "        uses: actions/setup-go@" + "a".repeat(40) + `\n        with:\n          cache-dependency-path: ${literalStyle}\n` +
      "            go.mod\n            go.sum\n            .github/workflows/runtime.yml\n";
    assert.throws(() => {
      for (const block of goSetupBlocks(workflow)) {
        assert.equal(block.includes(".github/workflows/runtime.yml"), false);
      }
    });
  }
});

test("unparsed Go setup uses fail closed and both YAML extensions are inspected", () => {
  const use = "uses: actions/setup-go@" + "a".repeat(40);
  assert.throws(() => goSetupBlocks("jobs:\n    - " + use + "\n"));
  assert.throws(() => goSetupBlocks("jobs:\n      - " + use + "\n        " + use + "\n"));
  assert.throws(() => goSetupBlocks("jobs:\n      - uses: actions/setup-go@v7\n"));
  assert.equal(goSetupBlocks("jobs:\n      - uses: actions/checkout@" + "a".repeat(40) + "\n").length, 0);
  for (const name of ["runtime.yml", "another.yaml"]) assert.equal(isWorkflow(name), true);
  for (const name of ["notes.md", "workflow.yml.backup"]) assert.equal(isWorkflow(name), false);
});

function checkActionJournalFixture(workflow, dockerfile) {
  assert.match(workflow, /^    timeout-minutes: 5$/mu);
  for (const input of ["internal/actionjournal/**", "internal/actioncore/**", "scripts/fixtures/action-journal/Dockerfile"])
    assert.equal(workflow.split(`      - ${input}`).length - 1, 2);
  const profile = workflow.slice(workflow.indexOf("      - name: Compile the unused action journal fixture"));
  for (const token of ["go test -c", "./internal/actionjournal", "docker build --network=none", "docker run --rm --network=none --read-only --cap-drop=ALL --security-opt=no-new-privileges",
    "--cpus=2 --memory=256m --memory-swap=256m --pids-limit=64", "--tmpfs /fixture:rw,noexec,nosuid,nodev,size=64m,mode=0700,uid=65532,gid=65532",
    "-e TMPDIR=/fixture -e GOMAXPROCS=2 -e HLO_ACTION_PRESSURE=1", "-test.run='^Test' -test.count=3 -test.timeout=90s -test.v"])
    assert.ok(profile.includes(token), `fixed action journal fixture: ${token}`);
  assert.doesNotMatch(profile, /--(?:privileged|cap-add|volume|mount|network[= ]host)|docker\.sock|-test\.(?:skip|short)\b/u);
  assert.equal(dockerfile.trim(), 'FROM scratch\nCOPY actionjournal.test /actionjournal.test\nUSER 65532:65532\nENTRYPOINT ["/actionjournal.test"]');
  assert.ok(workflow.includes('HLO_LEDGER_PRESSURE=1 "$RUNNER_TEMP/uploadledger.test"'), "existing pressure proof retained");
}

test("action-journal pressure proof is explicit, bounded and credential-free", async () => {
  const workflow = await readFile(path.join(root, ".github/workflows/ledger-storage-pressure.yml"), "utf8");
  const dockerfile = (await readFile(path.join(root, "scripts/fixtures/action-journal/Dockerfile"), "utf8")).replaceAll("\r\n", "\n");
  checkActionJournalFixture(workflow, dockerfile);
  for (const token of ["-e HLO_ACTION_PRESSURE=1", "--memory=256m", "--cap-drop=ALL", "-test.run='^Test'", "size=64m"])
    assert.throws(() => checkActionJournalFixture(workflow.replace(token, "REMOVED"), dockerfile));
  assert.throws(() => checkActionJournalFixture(workflow + "\n          --volume /:/host\n", dockerfile));
});
