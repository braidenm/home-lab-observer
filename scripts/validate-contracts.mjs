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
  "schemas/v1/metric-series-v1.schema.json",
  "schemas/v1/container-inventory-v1.schema.json",
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
const series = api.paths["/api/v1/metrics/series"].get;
const containers = api.paths["/api/v1/containers"].get;
if (caps["x-max-response-bytes"] !== 131072) fail("Capabilities response cap must be 128 KiB");
if (snapshot["x-max-response-bytes"] !== 1048576) fail("Snapshot response cap must be 1 MiB");
if (series["x-max-response-bytes"] !== 1048576) fail("Metric series response cap must be 1 MiB");
if (containers["x-max-response-bytes"] !== 1048576) fail("Container inventory response cap must be 1 MiB");
if (containers.parameters.length !== 1 || containers.parameters[0].name !== "limit" || containers.parameters[0].in !== "query") fail("Container inventory accepts only the limit query parameter");
for (const [key, value] of Object.entries({ minimum: 1, maximum: 500, default: 100 })) {
  if (containers.parameters[0].schema[key] !== value) fail(`Container inventory limit.${key} must equal ${value}`);
}
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

const metricIds = [
  "cpu.utilization.percent",
  "memory.utilization.percent",
  "filesystem.aggregate.utilization.percent",
  "network.receive.bytes_per_second",
  "network.transmit.bytes_per_second",
  "process.count"
];
const rangeParameter = series.parameters.find((item) => item.name === "range");
if (!rangeParameter?.required || JSON.stringify(rangeParameter.schema.enum) !== JSON.stringify(["1h", "6h", "24h", "7d"])) {
  fail("Metric series range must be required and limited to 1h, 6h, 24h, or 7d");
}
const metricParameter = series.parameters.find((item) => item.name === "metric");
if (series.parameters.length !== 2 || series.parameters.some((item) => item.in !== "query" || !new Set(["range", "metric"]).has(item.name))) fail("Metric series accepts only range and metric query parameters");
if (!metricParameter?.required || metricParameter.style !== "form" || metricParameter.explode !== true) fail("Metric must be a required repeatable query parameter");
if (metricParameter.schema.minItems !== 1 || metricParameter.schema.maxItems !== 6 || metricParameter.schema.uniqueItems !== true) fail("Metric query must contain one to six unique identifiers");
if (JSON.stringify(metricParameter.schema.items.enum) !== JSON.stringify(metricIds)) fail("Metric query allowlist is not the code-owned v1 set");

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

function validateMetricSeriesFixture(fixture, fixturePath) {
  const requested = fixture.requested_metrics;
  const returned = fixture.series.map((item) => item.metric_id);
  if (new Set(returned).size !== returned.length || JSON.stringify([...returned].sort()) !== JSON.stringify([...requested].sort())) {
    fail(`${fixturePath} must return exactly one series for every requested metric`);
  }
  const start = Date.parse(fixture.window_start);
  const end = Date.parse(fixture.window_end);
  if (!(start < end) || !fixture.generated_at.endsWith("Z") || !fixture.window_start.endsWith("Z") || !fixture.window_end.endsWith("Z")) fail(`${fixturePath} must have an increasing UTC window`);
  for (const item of fixture.series) {
    if (item.point_count !== item.points.length) fail(`${fixturePath} has an incorrect point_count for ${item.metric_id}`);
    if (item.gap_count !== item.points.filter((point) => point.value === null).length) fail(`${fixturePath} has an incorrect gap_count for ${item.metric_id}`);
    let previous = Number.NEGATIVE_INFINITY;
    for (const point of item.points) {
      const instant = Date.parse(point.at);
      if (!point.at.endsWith("Z") || instant < start || instant > end || instant <= previous) fail(`${fixturePath} points must be UTC and ordered within the requested window`);
      previous = instant;
    }
  }
  const prohibited = new Set(["query", "expression", "sql", "labels", "process_name", "pid", "log_body", "raw_log_body"]);
  const inspect = (value, location = "$") => {
    if (Array.isArray(value)) return value.forEach((item, index) => inspect(item, `${location}[${index}]`));
    if (value && typeof value === "object") for (const [key, item] of Object.entries(value)) {
      if (prohibited.has(key.toLowerCase())) fail(`Forbidden series field ${key} in ${fixturePath} at ${location}`);
      inspect(item, `${location}.${key}`);
    }
  };
  inspect(fixture);
}

function validateContainerInventoryFixture(fixture, fixturePath) {
  if (fixture.returned_count !== fixture.items.length || fixture.returned_count > fixture.total_count) {
    fail(`${fixturePath} has inconsistent container counts`);
  }
  if (!fixture.truncated && fixture.total_count > fixture.returned_count) {
    fail(`${fixturePath} must expose container truncation`);
  }
  const aliases = fixture.items.map((item) => item.id_alias);
  if (new Set(aliases).size !== aliases.length) fail(`${fixturePath} has duplicate container aliases`);
  const prohibited = new Set(["id", "raw_id", "command", "commands", "environment", "env", "mount", "mounts", "label", "labels", "port", "ports", "daemon_error"]);
  const inspect = (value, location = "$") => {
    if (Array.isArray(value)) return value.forEach((item, index) => inspect(item, `${location}[${index}]`));
    if (value && typeof value === "object") for (const [key, item] of Object.entries(value)) {
      if (prohibited.has(key.toLowerCase())) fail(`Forbidden container field ${key} in ${fixturePath} at ${location}`);
      inspect(item, `${location}.${key}`);
    }
  };
  inspect(fixture);
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
    if (testCase.schema.includes("metric-series")) validateMetricSeriesFixture(fixture, testCase.fixture);
    if (testCase.schema.includes("container-inventory")) validateContainerInventoryFixture(fixture, testCase.fixture);
    const bytes = fs.statSync(path.join(root, testCase.fixture)).size;
    const ceiling = testCase.schema.includes("current-snapshot") || testCase.schema.includes("metric-series") || testCase.schema.includes("container-inventory") ? 1048576 : 131072;
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
const seriesSpec = fs.readFileSync(path.join(root, "specs/005-local-data-plane/spec.md"), "utf8");
const seriesTrace = fs.readFileSync(path.join(root, "specs/005-local-data-plane/traceability.md"), "utf8");
for (const id of ["R4", "R5", "R10", "R12"]) {
  if (!seriesSpec.includes(`**${id} `) || !seriesTrace.includes(`| ${id} |`)) fail(`Missing Spec 005 traceability for ${id}`);
}

console.log(`Contract validation passed: OpenAPI 3.1, ${schemas.length} schemas, ${validCount} valid and ${invalidCount} invalid fixtures.`);
