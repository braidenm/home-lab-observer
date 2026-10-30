import assert from "node:assert/strict";
import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const linux = "runner.os == 'Linux'";
const other = "runner.os != 'Linux'";
const stress = "./internal/handoff -run TestConcurrentReaderNeverReturnsPartialDocument -count=25";

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
