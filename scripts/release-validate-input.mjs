#!/usr/bin/env node

const VERSION_PATTERN = /^0\.1\.0-preview\.[1-9][0-9]*$/;
const COMMIT_PATTERN = /^[a-f0-9]{40}$/;

function fail(message) {
  console.error(`release input rejected: ${message}`);
  process.exit(1);
}

function parseArgs(argv) {
  const values = new Map();
  for (let index = 0; index < argv.length; index += 2) {
    const name = argv[index];
    const value = argv[index + 1];
    if (!name?.startsWith("--") || value === undefined || value.startsWith("--")) {
      fail(`expected --name value arguments, received ${JSON.stringify(argv)}`);
    }
    if (values.has(name)) fail(`duplicate argument ${name}`);
    values.set(name, value);
  }
  return values;
}

const args = parseArgs(process.argv.slice(2));
const allowed = new Set(["--version", "--commit", "--ref"]);
for (const name of args.keys()) {
  if (!allowed.has(name)) fail(`unknown argument ${name}`);
}

const version = args.get("--version");
const commit = args.get("--commit");
const ref = args.get("--ref");

if (!VERSION_PATTERN.test(version ?? "")) {
  fail("version must match 0.1.0-preview.N, where N is a positive integer without leading zeroes");
}
if (!COMMIT_PATTERN.test(commit ?? "")) {
  fail("commit must be the full lowercase 40-character SHA");
}
if (ref !== undefined && ref !== "refs/heads/main") {
  fail("publication is allowed only from refs/heads/main");
}

console.log(`release input accepted: ${version} at ${commit}`);
