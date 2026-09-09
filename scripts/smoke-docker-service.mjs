import assert from 'node:assert/strict';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawn, spawnSync } from 'node:child_process';
import { createServer } from 'node:net';
import { setTimeout as delay } from 'node:timers/promises';
import { randomUUID } from 'node:crypto';
import Ajv from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';

assert.equal(process.platform, 'linux', 'real engine smoke runs only on the isolated Linux CI host');
const root = resolve(fileURLToPath(new URL('..', import.meta.url)));
const temporary = await mkdtemp(join(tmpdir(), 'observer-docker-smoke-'));
const suffix = randomUUID().slice(0, 8);
const image = `observer-smoke:${suffix}`;
const names = [`observer-smoke-${suffix}-running`, `observer-smoke-${suffix}-stopped`];
const created = [];
const containerIDs = [];
let imageCreated = false;
let observer;
let exited;
let processOutput = '';
const canary = 'synthetic-private-env-canary';
try {
  command('docker', ['version', '--format', '{{.Server.Version}}']);
  command('go', ['build', '-trimpath', '-o', join(temporary, 'fixture'), './scripts/fixtures/docker-observation'], { ...process.env, CGO_ENABLED: '0' });
  command('docker', ['build', '--quiet', '--tag', image, '--file', join(root, 'scripts/fixtures/docker-observation/Dockerfile'), temporary]);
  imageCreated = true;
  for (const name of names) {
    const id = command('docker', ['run', '--detach', '--name', name, '--network', 'none', '--read-only', '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges', '--memory', '32m', '--cpus', '0.25', '--log-driver', 'none', '--env', `OBSERVATION_TEST=${canary}`, '--label', `private=${canary}`, image]).trim();
    created.push(name);
    containerIDs.push(id);
  }
  command('docker', ['stop', '--timeout', '5', names[1]]);
  const binary = join(temporary, 'observer');
  command('go', ['build', '-trimpath', '-o', binary, './cmd/observer']);
  const reservation = createServer();
  await new Promise((resolve, reject) => { reservation.once('error', reject); reservation.listen(0, '127.0.0.1', resolve); });
  const port = reservation.address().port;
  await new Promise(resolve => reservation.close(resolve));
  const origin = `http://127.0.0.1:${port}`;
  const state = join(temporary, 'state');
  observer = spawn(binary, ['serve', '--listen', `127.0.0.1:${port}`, '--state-dir', state, '--docker-endpoint', 'unix:///var/run/docker.sock'], { cwd: root, stdio: ['ignore', 'pipe', 'pipe'] });
  exited = new Promise(resolve => { observer.once('exit', resolve); observer.once('error', resolve); });
  for (const stream of [observer.stdout, observer.stderr]) stream.on('data', chunk => { processOutput = (processOutput + chunk).slice(-16384); });
  await until(async () => (await fetch(`${origin}/health/live`)).ok);
  const token = (await readFile(join(state, 'local-api.token'), 'utf8')).trim();
  const headers = { Authorization: `Bearer ${token}` };
  const ajv = new Ajv({ strict: false, allErrors: true });
  addFormats(ajv);
  const validate = ajv.compile(JSON.parse(await readFile(join(root, 'schemas/v1/container-inventory-v1.schema.json'), 'utf8')));
  let inventory;
  await until(async () => {
    const response = await fetch(`${origin}/api/v1/containers?limit=500`, { headers });
    if (!response.ok) return false;
    inventory = await response.json();
    return inventory.support_state === 'SUPPORTED' && names.every(name => inventory.items.some(item => item.name === name))
      && inventory.items.some(item => item.name === names[0] && item.memory_bytes !== null && item.cpu_percent !== null && item.metrics_state === 'AVAILABLE');
  });
  assert(validate(inventory), ajv.errorsText(validate.errors));
  const running = inventory.items.find(item => item.name === names[0]);
  const stopped = inventory.items.find(item => item.name === names[1]);
  assert.equal(running.state, 'running');
  assert.equal(stopped.state, 'exited');
  assert.equal(stopped.metrics_state, 'NOT_RUNNING');
  assert.equal(stopped.cpu_percent, null);
  assert.equal(stopped.memory_bytes, null);
  assert(running.memory_bytes !== null && running.memory_bytes >= 0, 'real Linux engine returned no memory reading');
  assert(running.cpu_percent !== null && running.cpu_percent >= 0 && running.cpu_percent <= 100, 'real Linux engine returned no bounded host-capacity CPU reading after warmup');
  assert.equal(running.metrics_state, 'AVAILABLE');
  const encoded = JSON.stringify(inventory);
  assert(!encoded.includes(canary) && !processOutput.includes(canary), 'source secret escaped normalization');
  assert(!encoded.includes(token) && !processOutput.includes(token), 'local token escaped');
  for (const id of containerIDs) assert(!encoded.includes(id), 'raw container ID escaped');
  const limited = await (await fetch(`${origin}/api/v1/containers?limit=1`, { headers })).json();
  assert(validate(limited), ajv.errorsText(validate.errors));
  assert.equal(limited.items.length, 1);
  assert.equal(limited.truncated, true);
  assert.equal((await fetch(`${origin}/api/v1/containers`)).status, 401);
  assert.equal((await fetch(`${origin}/api/v1/containers`, { method: 'POST', headers })).status, 405);
  assert.equal((await fetch(`${origin}/health/ready`)).status, 200);
  assert.equal(command('docker', ['inspect', '--format', '{{.State.Status}}', names[0]]).trim(), 'running');
  assert.equal(command('docker', ['inspect', '--format', '{{.State.Status}}', names[1]]).trim(), 'exited');
  console.log('Real Docker smoke passed: running/stopped inventory, nullable stats, privacy, authenticated bounds and unchanged workloads.');
} finally {
  if (observer && observer.exitCode === null) {
    observer.kill('SIGTERM');
    const forced = setTimeout(() => observer.kill('SIGKILL'), 15000);
    try { await exited; } finally { clearTimeout(forced); }
  }
  const cleanupErrors = [];
  for (const name of created) {
    try { command('docker', ['rm', '--force', name]); } catch (error) { cleanupErrors.push(error); }
  }
  if (imageCreated) {
    try { command('docker', ['image', 'rm', image]); } catch (error) { cleanupErrors.push(error); }
  }
  await rm(temporary, { recursive: true, force: true });
  if (cleanupErrors.length) throw new AggregateError(cleanupErrors, 'Docker fixture cleanup failed');
}

function command(program, args, env = process.env) {
  const result = spawnSync(program, args, { cwd: root, env, encoding: 'utf8', timeout: 120000 });
  assert.equal(result.status, 0, `${program} fixture operation failed`);
  return result.stdout;
}
async function until(test) {
  const deadline = Date.now() + 45000;
  while (Date.now() < deadline) {
    if (observer.exitCode !== null) throw new Error('Observer exited during Docker smoke');
    try { if (await test()) return; } catch { /* Listener or first collection may not be ready. */ }
    await delay(200);
  }
  throw new Error('Timed out waiting for real Docker inventory');
}
