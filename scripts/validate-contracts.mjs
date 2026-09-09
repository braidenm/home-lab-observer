import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import SwaggerParser from "@apidevtools/swagger-parser";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const readJson = (relative) => JSON.parse(fs.readFileSync(path.join(root, relative), "utf8"));
const fail = (message) => { throw new Error(message); };

const schemaPaths = [
  "schemas/v1/capabilities-v1.schema.json",
  "schemas/v1/current-snapshot-v1.schema.json",
  "schemas/v1/problem-details-v1.schema.json"
];
const schemas = schemaPaths.map(readJson);
const ajv = new Ajv2020({ allErrors: true, strict: false });
addFormats(ajv);
for (const schema of schemas) ajv.addSchema(schema);

const apiPath = path.join(root, "api/openapi.v1.json");
const api = await SwaggerParser.validate(apiPath);
if (api.openapi !== "3.1.0") fail("OpenAPI must use 3.1.0");
for (const server of api.servers ?? []) {
  const host = new URL(server.url).hostname;
  if (!new Set(["127.0.0.1", "[::1]"]).has(host)) fail(`Non-loopback server declared: ${server.url}`);
}

const methodKeys = new Set(["get", "head", "parameters", "summary", "description", "servers"]);
for (const [route, pathItem] of Object.entries(api.paths)) {
  for (const key of Object.keys(pathItem)) if (!methodKeys.has(key)) fail(`Mutation or unknown path key ${key} at ${route}`);
  if (!route.startsWith("/api/v1")) continue;
  const operation = pathItem.get;
  if (!operation) fail(`${route} must expose GET`);
  const security = operation.security ?? api.security;
  if (!security?.some((item) => Object.hasOwn(item, "localBearer"))) fail(`${route} must require localBearer`);
  for (const [status, response] of Object.entries(operation.responses)) {
    if (status === "200") continue;
    const resolved = response.$ref ? api.components.responses.Problem : response;
    if (!resolved.content?.["application/problem+json"]) fail(`${route} ${status} must use application/problem+json`);
  }
}

const caps = api.paths["/api/v1/capabilities"].get;
const snapshot = api.paths["/api/v1/snapshots/current"].get;
if (caps["x-max-response-bytes"] !== 131072) fail("Capabilities response cap must be 128 KiB");
if (snapshot["x-max-response-bytes"] !== 1048576) fail("Snapshot response cap must be 1 MiB");
const expectedQueries = {
  process_limit: { minimum: 1, maximum: 200, default: 50 },
  container_limit: { minimum: 1, maximum: 500, default: 100 },
  log_limit: { minimum: 0, maximum: 200, default: 50 },
  include_log_bodies: { default: false }
};
for (const [name, expected] of Object.entries(expectedQueries)) {
  const parameter = snapshot.parameters.find((item) => item.name === name);
  if (!parameter) fail(`Missing query parameter ${name}`);
  for (const [key, value] of Object.entries(expected)) if (parameter.schema[key] !== value) fail(`${name}.${key} must equal ${value}`);
}

const forbiddenKeys = new Set(["authorization", "token", "secret", "environment", "env", "argv", "command_line", "raw_body", "stack_trace", "exception", "private_key", "password"]);
const riskyString = /(ghp_|github_pat_|-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----|https?:\/\/[^\s/@:]+:[^\s/@]+@|\b(?:10\.\d{1,3}|192\.168|172\.(?:1[6-9]|2\d|3[01]))\.\d{1,3}\.\d{1,3}\b)/i;
function scan(value, location = "$") {
  if (typeof value === "string" && riskyString.test(value)) fail(`Sensitive-looking string in valid fixture at ${location}`);
  if (Array.isArray(value)) return value.forEach((item, index) => scan(item, `${location}[${index}]`));
  if (value && typeof value === "object") for (const [key, item] of Object.entries(value)) {
    if (forbiddenKeys.has(key.toLowerCase())) fail(`Forbidden key ${key} in valid fixture at ${location}`);
    scan(item, `${location}.${key}`);
  }
}

const manifest = readJson("schemas/v1/fixtures/manifest.json");
let validCount = 0;
let invalidCount = 0;
for (const testCase of manifest.cases) {
  const schema = readJson(testCase.schema);
  const fixture = readJson(testCase.fixture);
  const validate = ajv.getSchema(schema.$id) ?? fail(`Schema not registered: ${schema.$id}`);
  const actual = validate(fixture);
  if (actual !== testCase.valid) fail(`${testCase.fixture} expected valid=${testCase.valid}: ${ajv.errorsText(validate.errors)}`);
  if (testCase.valid) {
    validCount += 1;
    scan(fixture);
    const bytes = fs.statSync(path.join(root, testCase.fixture)).size;
    const ceiling = testCase.schema.includes("current-snapshot") ? 1048576 : 131072;
    if (bytes > ceiling) fail(`${testCase.fixture} exceeds ${ceiling} bytes`);
  } else invalidCount += 1;
}
if (!validCount || !invalidCount) fail("Fixture manifest must include valid and invalid cases");

const snapshotSchema = schemas.find((item) => item.$id.includes("current-snapshot"));
if (snapshotSchema.properties.privacy.properties.remote_projection.const !== "home-lab-server-snapshot/v1") fail("Legacy projection boundary is missing");
const spec = fs.readFileSync(path.join(root, "specs/002-cross-platform-observer/spec.md"), "utf8");
const trace = fs.readFileSync(path.join(root, "specs/002-cross-platform-observer/traceability.md"), "utf8");
for (let index = 1; index <= 14; index += 1) {
  const id = `R${index}`;
  if (!spec.includes(`**${id} `) || !trace.includes(`| ${id} |`)) fail(`Missing traceability for ${id}`);
}

console.log(`Contract validation passed: OpenAPI 3.1, ${schemas.length} schemas, ${validCount} valid and ${invalidCount} invalid fixtures.`);
