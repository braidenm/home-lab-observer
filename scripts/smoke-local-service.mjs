import assert from 'node:assert/strict';
import { mkdtemp, readFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawn, spawnSync } from 'node:child_process';
import { createServer } from 'node:net';
import { setTimeout as delay } from 'node:timers/promises';
import Ajv from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';

const root = resolve(fileURLToPath(new URL('..', import.meta.url)));
const temporary = await mkdtemp(join(tmpdir(), 'observer-smoke-'));
const binary = join(temporary, process.platform === 'win32' ? 'observer.exe' : 'observer');
const built = spawnSync('go', ['build', '-o', binary, './cmd/observer'], { cwd: root, encoding: 'utf8', timeout: 180000 });
assert.equal(built.status, 0, `observer build failed: ${built.stderr}`);
const port = await freePort();
const origin = `http://127.0.0.1:${port}`;
const state = join(temporary, 'state');
const processHandle = spawn(binary, ['serve', '--listen', `127.0.0.1:${port}`, '--state-dir', state], { cwd: root, stdio: ['ignore', 'pipe', 'pipe'] });
let output = '';
for (const stream of [processHandle.stdout, processHandle.stderr]) stream.on('data', chunk => { output = (output + chunk).slice(-16384); });
let browser;
try {
  await until(async () => (await fetch(`${origin}/health/live`)).ok);
  const token = (await readFile(join(state, 'local-api.token'), 'utf8')).trim();
  const headers = { Authorization: `Bearer ${token}` };
  const ajv = new Ajv({ allErrors: true, strict: false });
  addFormats(ajv);
  const validators = {};
  for (const name of ['capabilities', 'current-snapshot', 'metric-series', 'problem-details']) {
    validators[name] = ajv.compile(JSON.parse(await readFile(join(root, `schemas/v1/${name}-v1.schema.json`), 'utf8')));
  }
  async function validated(path, name, status = 200, requestHeaders = headers) {
    const response = await fetch(`${origin}${path}`, { headers: requestHeaders });
    assert.equal(response.status, status, `${path}: unexpected status`);
    const text = await response.text();
    assert(text.length <= 1048576, 'response exceeded size bound');
    assert(!text.includes(token), 'token leaked into API response');
    const value = JSON.parse(text);
    assert(validators[name](value), `${name}: ${ajv.errorsText(validators[name].errors)}`);
    return value;
  }
  await validated('/api/v1/capabilities', 'problem-details', 401, {});
  await validated('/api/v1/capabilities', 'capabilities');
  let current;
  await until(async () => {
    const response = await fetch(`${origin}/api/v1/snapshots/current`, { headers });
    if (!response.ok) return false;
    current = await response.json();
    return current.sequence > 0;
  });
  const firstSequence = current.sequence;
  await validated('/api/v1/snapshots/current?process_limit=1', 'current-snapshot');
  await validated('/api/v1/snapshots/current?section=observer', 'current-snapshot');
  await validated('/api/v1/snapshots/current?process_limit=201', 'problem-details', 400);
  await validated('/api/v1/metrics/series?range=forever&metric=process.count', 'problem-details', 400);
  const query = '/api/v1/metrics/series?range=1h&metric=memory.utilization.percent&metric=network.receive.bytes_per_second';
  await validated(query, 'metric-series');
  await until(async () => {
    const response = await fetch(`${origin}/api/v1/snapshots/current`, { headers });
    return response.ok && (await response.json()).sequence > firstSequence;
  });
  const trends = await validated(query, 'metric-series');
  assert(trends.series.some(series => series.points.some(point => point.value !== null)), 'real history contains no observations');
  assert(!output.includes(token), 'token leaked into process output');

  if (process.argv.includes('--browser')) {
    const { chromium } = await import('../web/observer-ui/node_modules/playwright/index.mjs');
    browser = await chromium.launch(process.env.OBSERVER_BROWSER_CHANNEL ? { channel: process.env.OBSERVER_BROWSER_CHANNEL } : {});
    const page = await browser.newPage();
    const failures = [];
    page.on('pageerror', error => failures.push(error.message));
    for (const width of [390, 768, 1440]) {
      await page.setViewportSize({ width, height: 1000 });
      await page.goto(origin);
      const unlock = page.getByRole('button', { name: 'Unlock dashboard', exact: true });
      await unlock.or(page.getByRole('button', { name: 'Overview', exact: true })).waitFor();
      if (await unlock.isVisible()) {
        await page.getByLabel('Local access token', { exact: true }).fill(token);
        await unlock.click();
      }
      await page.getByRole('button', { name: 'Overview', exact: true }).waitFor();
      for (const view of ['Overview', 'Trends', 'Workloads', 'Logs', 'Observer & privacy']) {
        await page.getByRole('button', { name: view, exact: true }).click();
        await page.waitForTimeout(100);
        if (view === 'Trends') {
          await page.getByText('Actual sample interval:', { exact: false }).waitFor();
        }
        if (view === 'Workloads') {
          const processes = page.getByRole('tab', { name: 'Processes', exact: true });
          await processes.focus();
          await page.keyboard.press('ArrowRight');
          await page.waitForFunction(() => document.activeElement?.textContent === 'Services');
          assert.equal(await page.getByRole('tab', { name: 'Services', exact: true }).getAttribute('aria-selected'), 'true');
          await page.keyboard.press('ArrowRight');
          await page.waitForFunction(() => document.activeElement?.textContent === 'Containers');
          assert.equal(await page.getByRole('tab', { name: 'Containers', exact: true }).getAttribute('aria-selected'), 'true');
          await page.getByText('No records: collector support is UNSUPPORTED.', { exact: true }).waitFor();
        }
        assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), `${view} overflows at ${width}px`);
      }
      assert(!(await page.locator('body').innerText()).includes(token), 'token visible in page text');
      await page.screenshot({ path: join(temporary, `dashboard-${width}.png`), fullPage: true });
    }
    await page.getByRole('button', { name: 'Lock dashboard', exact: true }).click();
    await page.getByRole('button', { name: 'Unlock dashboard', exact: true }).waitFor();
    assert(!(await page.evaluate(() => JSON.stringify(sessionStorage))).includes(token), 'lock retained the token');
    assert.equal(failures.length, 0, `browser errors: ${failures.join('; ')}`);
    console.log(`Packaged browser checks passed at 390, 768 and 1440 pixels. Screenshots: ${temporary}`);
  }
  console.log('Native local service passed real snapshot, history, authentication and schema smoke checks.');
} finally {
  await browser?.close();
  processHandle.kill('SIGTERM');
}

async function until(test) {
  const deadline = Date.now() + 45000;
  while (Date.now() < deadline) {
    if (processHandle.exitCode !== null) throw new Error('Observer exited before smoke checks completed');
    try { if (await test()) return; } catch { /* Startup may not have opened the listener yet. */ }
    await delay(200);
  }
  throw new Error('Timed out waiting for observer data');
}

async function freePort() {
  const server = createServer();
  await new Promise((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve); });
  const port = server.address().port;
  await new Promise(resolve => server.close(resolve));
  return port;
}
