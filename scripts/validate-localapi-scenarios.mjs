import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { spawnSync } from 'node:child_process';
import Ajv from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';

const run = spawnSync('go', ['test', '-count=1', '-run', '^TestSchemaScenarioContracts$', '-v', './internal/localapi'], { encoding: 'utf8', timeout: 120000 });
assert.equal(run.status, 0, `Synthetic handler scenarios failed: ${run.stdout}\n${run.stderr}`);
const ajv = new Ajv({ allErrors: true, strict: false });
addFormats(ajv);
const validators = new Map();
for (const name of ['capabilities', 'current-snapshot', 'metric-series', 'problem-details']) {
  validators.set(name, ajv.compile(JSON.parse(await readFile(`schemas/v1/${name}-v1.schema.json`, 'utf8'))));
}
let count = 0;
for (const line of run.stdout.split('\n')) {
  const match = /SCHEMA_SCENARIO ([a-z-]+) (\{.*\})/.exec(line);
  if (!match) continue;
  const validate = validators.get(match[1]);
  assert(validate, 'unrecognized emitted schema');
  assert(validate(JSON.parse(match[2])), `${match[1]} scenario: ${ajv.errorsText(validate.errors)}`);
  count++;
}
assert(count >= 10, 'expected all synthetic handler scenarios');
console.log(`Validated ${count} synthetic handler responses, including partial/stale/selected/unavailable reads.`);
