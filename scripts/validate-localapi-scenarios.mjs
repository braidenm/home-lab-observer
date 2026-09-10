import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { spawnSync } from 'node:child_process';
import Ajv from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';
import { validateLogSummaryFixture } from './log-summary-contract.mjs';

const run = spawnSync('go', ['test', '-count=1', '-run', '^Test(?:SchemaScenarioContracts|ContainerSchemaScenarioContract|LogSummarySchemaScenarioContract)$', '-v', './internal/localapi'], { encoding: 'utf8', timeout: 120000 });
assert.equal(run.status, 0, `Synthetic handler scenarios failed: ${run.stdout}\n${run.stderr}`);
const ajv = new Ajv({ allErrors: true, strict: false });
addFormats(ajv);
const validators = new Map();
for (const name of ['capabilities', 'current-snapshot', 'metric-series', 'container-inventory', 'log-summary', 'problem-details']) {
  validators.set(name, ajv.compile(JSON.parse(await readFile(`schemas/v1/${name}-v1.schema.json`, 'utf8'))));
}
let count = 0;
for (const line of run.stdout.split('\n')) {
  const match = /SCHEMA_SCENARIO ([a-z-]+) (\{.*\})/.exec(line);
  if (!match) continue;
  const validate = validators.get(match[1]);
  assert(validate, 'unrecognized emitted schema');
  const fixture = JSON.parse(match[2]);
  assert(validate(fixture), `${match[1]} scenario: ${ajv.errorsText(validate.errors)}`);
  if (match[1] === 'log-summary') validateLogSummaryFixture(fixture, 'Go handler log summary scenario');
  count++;
}
assert(count >= 12, 'expected all synthetic handler scenarios');
console.log(`Validated ${count} synthetic handler responses, including container, log summary, and partial/stale/selected/unavailable reads.`);
