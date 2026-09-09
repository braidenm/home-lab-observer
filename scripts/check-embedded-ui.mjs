import { readdirSync, readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const root = resolve(fileURLToPath(new URL("..", import.meta.url)));
const ui = join(root, "web", "observer-ui");
const assets = join(root, "internal", "webui", "assets");
run(process.execPath, [join(ui, "node_modules", "vite", "bin", "vite.js"), "build", "--config", "vite.embedded.config.ts"], ui);

const files = walk(assets);
const expected = ["index.html", "static/dashboard.css", "static/dashboard.js"];
if (JSON.stringify(files) !== JSON.stringify(expected)) {
  fail(`embedded asset set differs from the closed manifest:\n${files.join("\n")}`);
}
for (const name of files) {
  const content = readFileSync(join(assets, name), "utf8");
  if (name.endsWith(".map") || /sourceMappingURL/i.test(content)) fail(`source map found in ${name}`);
  if ((name.endsWith(".html") || name.endsWith(".css")) && /(?:src|href|url|@import)[^\n]*(?:https?:)?\/\//i.test(content)) {
    fail(`external asset reference found in ${name}`);
  }
}

const diff = spawnSync("git", ["diff", "--exit-code", "--", "internal/webui/assets"], { cwd: root, encoding: "utf8" });
const untracked = spawnSync("git", ["ls-files", "--others", "--exclude-standard", "--", "internal/webui/assets"], { cwd: root, encoding: "utf8" });
if (diff.status !== 0 || untracked.stdout.trim()) {
  process.stderr.write(diff.stdout);
  process.stderr.write(untracked.stdout);
  fail("embedded dashboard assets are stale; run npm run build:embedded in web/observer-ui and commit the result");
}
console.log(`Embedded dashboard assets are deterministic and current (${files.length} files).`);

function walk(directory, prefix = "") {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const name = prefix ? `${prefix}/${entry.name}` : entry.name;
    return entry.isDirectory() ? walk(join(directory, entry.name), name) : [name];
  }).sort();
}

function run(command, args, cwd) {
  const result = spawnSync(command, args, { cwd, stdio: "inherit" });
  if (result.status !== 0) process.exit(result.status ?? 1);
}

function fail(message) { console.error(message); process.exit(1); }
