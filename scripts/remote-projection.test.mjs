import assert from "node:assert/strict";
import fs from "node:fs";
import test from "node:test";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";

const read = (name) => JSON.parse(fs.readFileSync(new URL(name, import.meta.url), "utf8"));
const schema = read("../schemas/remote/numeric-host-v1.schema.json");
const fixture = read("../schemas/remote/fixtures/numeric-host.json");
const ajv = new Ajv2020({ strict: true, allErrors: true });
addFormats(ajv);
const validate = ajv.compile(schema);

test("numeric-host producer fixture conforms to its closed profile", () => {
  assert.equal(validate(fixture), true, ajv.errorsText(validate.errors));
  assert.ok(Buffer.byteLength(JSON.stringify(fixture)) < 16384);
});

test("unavailable overview has no numeric values", () => {
  const value = structuredClone(fixture);
  value.sections.overview = { available: false, reason_code: "HOST_DATA_UNAVAILABLE" };
  assert.equal(validate(value), true, ajv.errorsText(validate.errors));
  value.sections.overview.cpu_usage_percent = 0;
  assert.equal(validate(value), false);
});

const mutations = {
  "raw hostname": v => { v.hostname = "synthetic.example.invalid"; },
  "logs": v => { v.sections.logs = []; },
  "host field": v => { v.sections.overview.kernel_details = "private"; },
  "filesystem path": v => { v.sections.overview.filesystems[0].mount = "/private"; },
  "process content": v => { v.sections.processes.items.push({ name: "private" }); },
  "control authority": v => { v.sections.containers.actions = ["restart"]; },
  "false eligible flag": v => { v.sections.containers.available = true; },
  "unsafe integer": v => { v.sections.overview.memory_total_bytes = 9007199254740992; },
  "negative bytes": v => { v.sections.overview.memory_used_bytes = -1; },
  "fractional bytes": v => { v.sections.overview.memory_used_bytes = 1.5; },
  "invalid CPU": v => { v.sections.overview.cpu_usage_percent = 101; },
  "unknown OS": v => { v.sections.overview.operating_system = "private"; },
  "source URL": v => { v.source_id = "https://example.invalid"; },
  "connector instance instead of server": v => { v.source_id = "agent_0123456789abcdef0123456789abcdef"; },
  "source too long": v => { v.source_id = "a".repeat(65); },
  "duration too long": v => { v.duration_ms = 10001; },
  "raw version": v => { v.collector_version = "secret@example.invalid"; },
  "non UTC": v => { v.collected_at = "2026-09-10T12:00:00+01:00"; },
  "missing field": v => { delete v.projection_profile; },
  "wrong profile": v => { v.projection_profile = "all-data/v1"; },
  "too many filesystems": v => { v.sections.overview.filesystems = Array(17).fill(v.sections.overview.filesystems[0]); },
};
for (const [name, mutate] of Object.entries(mutations)) {
  test(`producer profile rejects ${name}`, () => {
    const value = structuredClone(fixture);
    mutate(value);
    assert.equal(validate(value), false);
  });
}
