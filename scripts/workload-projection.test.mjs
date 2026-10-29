import assert from "node:assert/strict";
import fs from "node:fs";
import test from "node:test";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";

const read = (path) => JSON.parse(fs.readFileSync(new URL(path, import.meta.url), "utf8"));
// Conditions use the enclosing object's required list; this does not relax validation.
const ajv = new Ajv2020({ strict: true, strictRequired: false, allErrors: true });
addFormats(ajv);
const validate = ajv.compile(read("../schemas/remote/workload-inventory-v1.schema.json"));
const partial = read("../schemas/remote/fixtures/workloads-partial.json");

for (const name of ["workloads-disabled", "workloads-partial"]) {
  test(`${name} shared producer fixture has closed schema and bounded fields`, () => {
    const value = read(`../schemas/remote/fixtures/${name}.json`);
    assert.equal(validate(value), true, ajv.errorsText(validate.errors));
    assert.ok(Buffer.byteLength(JSON.stringify(value)) <= 262144);
    for (const group of [value.processes, value.containers]) {
      assert.ok(Buffer.byteLength(JSON.stringify(group)) <= 130560);
      assert.equal(group.returned_count, group.items.length);
      assert.ok(group.total_count === null || group.total_count >= group.items.length);
      for (const row of group.items) {
        assert.ok(Buffer.byteLength(row.name) <= 128);
        if (row.image !== undefined) assert.ok(Buffer.byteLength(row.image) <= 240);
      }
    }
  });
}

const mutations = {
  "old upload schema": v => { v.schema_version = "home-lab-native-host-metrics/v1"; },
  "wrong profile": v => { v.projection_profile = "numeric-host/v1"; },
  "raw host source": v => { v.source_id = "machine.example.invalid"; },
  "raw hostname": v => { v.hostname = "machine.example.invalid"; },
  "unknown field": v => { v.processes.items[0].environment = {}; },
  "raw local account": v => { v.processes.items[0].account = "sample"; },
  "command line": v => { v.processes.items[0].command_line = "sample --private"; },
  "process path": v => { v.processes.items[0].name = "/private/sample"; },
  "action authority": v => { v.containers.items[0].actions = ["restart"]; },
  "raw Docker ID": v => { v.containers.items[0].id = "a".repeat(64); },
  "Docker labels": v => { v.containers.items[0].labels = {}; },
  "implicit process consent": v => { v.policy.processes_enabled = false; },
  "implicit container consent": v => { v.policy.containers_enabled = false; },
  "pretend anonymous policy": v => { v.policy.data_classification = "ANONYMOUS"; },
  "write policy": v => { v.policy.read_only = false; },
  "arbitrary quality": v => { v.processes.quality.state = "private-state"; },
  "arbitrary reason": v => { v.containers.quality.reason_code = "private-error"; },
  "available missing count": v => { v.processes.total_count = null; },
  "disabled retained rows": v => { v.processes.quality = { state: "DISABLED", reason_code: "DISABLED_BY_CONFIGURATION" }; },
  "stopped live CPU": v => { v.containers.items[1].cpu_percent = 0; },
  "partial fabricated CPU": v => { v.containers.items[0].cpu_percent = 0; },
  "missing unavailable reason": v => { v.containers.items[1].reason_code = null; },
  "uncollected health": v => { v.containers.items[0].health = "healthy"; },
  "unknown container state": v => { v.containers.items[0].state = "private-state"; },
  "zero PID": v => { v.processes.items[0].pid = 0; },
  "unknown elapsed rendered zero": v => { v.processes.items[0].elapsed_seconds = 0; },
  "unsafe integer": v => { v.processes.items[0].memory_rss_bytes = 2 ** 53; },
  "negative bytes": v => { v.containers.items[0].memory_bytes = -1; },
  "fractional bytes": v => { v.containers.items[0].memory_bytes = 0.5; },
  "invalid process CPU": v => { v.processes.items[0].cpu_percent = 409601; },
  "invalid container CPU": v => { v.containers.items[0].cpu_percent = 101; },
  "wrong process CPU basis": v => { v.processes.cpu_measurement_basis = "ENGINE_HOST_CAPACITY"; },
  "wrong container CPU basis": v => { v.containers.cpu_measurement_basis = "LIFETIME_SINGLE_CORE"; },
  "oversized name": v => { v.containers.items[0].name = "x".repeat(129); },
  "control character": v => { v.processes.items[0].name = "sample\nprivate"; },
  "process bidi formatting": v => { v.processes.items[0].name = "sample\u202eservice"; },
  "container zero width": v => { v.containers.items[0].name = "sample\u200bservice"; },
  "image directional isolation": v => { v.containers.items[0].image = "example.invalid/\u2066sample\u2069"; },
  "non UTC time": v => { v.collected_at = "2026-10-29T12:00:00+01:00"; },
  "missing policy": v => { delete v.policy; },
  "process row cap": v => { v.processes.items = Array(201).fill(v.processes.items[0]); },
  "container row cap": v => { v.containers.items = Array(501).fill(v.containers.items[0]); },
};
for (const [name, mutate] of Object.entries(mutations)) {
  test(`workload schema rejects ${name}`, () => {
    const value = structuredClone(partial);
    mutate(value);
    assert.equal(validate(value), false);
  });
}

test("lifetime single-core process CPU can exceed 100 without claiming host-capacity percent", () => {
  for (const cpu of [101, 409600]) {
    const value = structuredClone(partial);
    value.processes.items[0].cpu_percent = cpu;
    assert.equal(validate(value), true, ajv.errorsText(validate.errors));
  }
});
