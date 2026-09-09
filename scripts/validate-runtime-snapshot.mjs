import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const schema = JSON.parse(fs.readFileSync(path.join(root, "schemas/v1/current-snapshot-v1.schema.json"), "utf8"));
const executable = process.platform === "win32" ? "go.exe" : "go";
const result = spawnSync(executable, ["run", "./cmd/observer", "collect-once", "--max-processes", "5", "--timeout", "20s"], {
  cwd: root,
  encoding: "utf8",
  timeout: 60000,
  maxBuffer: 1048576
});
if (result.error) throw result.error;
if (result.status !== 0 && result.status !== 1) throw new Error(`collect-once exited ${result.status}: ${result.stderr}`);
const output = result.stdout;
const snapshot = JSON.parse(output);
const ajv = new Ajv2020({ allErrors: true, strict: false });
addFormats(ajv);
const validate = ajv.compile(schema);
if (!validate(snapshot)) throw new Error(`Runtime snapshot violates v1 schema: ${ajv.errorsText(validate.errors)}`);
if ((snapshot.collection_state === "FAILED") !== (result.status === 1)) throw new Error(`Exit ${result.status} disagrees with ${snapshot.collection_state} snapshot`);
if (Buffer.byteLength(output, "utf8") > 1048576) throw new Error("Runtime snapshot exceeds the 1 MiB contract ceiling");
console.log("Runtime collect-once output conforms to observer-current-snapshot/v1.");
