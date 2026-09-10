import assert from 'node:assert/strict';
import { mkdtemp, readFile, realpath, stat } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawn, spawnSync } from 'node:child_process';
import { createServer } from 'node:net';
import { setTimeout as delay } from 'node:timers/promises';
import Ajv from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';
import { validateLogSummaryFixture } from './log-summary-contract.mjs';

const root = resolve(fileURLToPath(new URL('..', import.meta.url)));
const temporary = await realpath(await mkdtemp(join(tmpdir(), 'observer-smoke-')));
const binary = process.env.OBSERVER_SMOKE_BINARY
  ? resolve(process.env.OBSERVER_SMOKE_BINARY)
  : join(temporary, process.platform === 'win32' ? 'observer.exe' : 'observer');
if (process.env.OBSERVER_SMOKE_BINARY) {
  assert((await stat(binary)).isFile(), 'packaged smoke binary must be an existing file');
} else {
  const built = spawnSync('go', ['build', '-o', binary, './cmd/observer'], { cwd: root, encoding: 'utf8', timeout: 180000 });
  assert.equal(built.status, 0, `observer build failed: ${built.stderr}`);
}
const port = await freePort();
const origin = `http://127.0.0.1:${port}`;
const state = join(temporary, 'state');
const processHandle = spawn(binary, ['serve', '--listen', `127.0.0.1:${port}`, '--state-dir', state], { cwd: root, stdio: ['ignore', 'pipe', 'pipe'] });
const processExited = new Promise(resolveExit => processHandle.once('exit', resolveExit));
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
  for (const name of ['capabilities', 'current-snapshot', 'metric-series', 'container-inventory', 'diagnostics-health', 'problem-details', 'log-summary']) {
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
  const containers = await validated('/api/v1/containers?limit=500', 'container-inventory');
  assert.equal(containers.support_state, 'DISABLED');
  assert.equal(containers.policy.remote_upload_eligible, false);
  await validated('/api/v1/containers?limit=501', 'problem-details', 400);
  await validated('/api/v1/containers', 'problem-details', 401, {});
  const diagnostics = await validated('/api/v1/diagnostics/health', 'diagnostics-health');
  assert.equal(diagnostics.state, 'DISABLED', 'foreground unexpectedly retained diagnostics');
  await validated('/api/v1/diagnostics/health', 'problem-details', 401, {});
  await validated('/api/v1/diagnostics/health?path=not-a-file-reader', 'problem-details', 400);
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
          await page.getByRole('heading', { name: 'Container observations are disabled', exact: true }).waitFor();
        }
        assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), `${view} overflows at ${width}px`);
        if ((view === 'Overview' || view === 'Trends' || view === 'Workloads') && width !== 768) {
          await page.screenshot({ path: join(temporary, `${view.toLowerCase()}-${width}.png`), fullPage: true });
        }
      }
      assert(!(await page.locator('body').innerText()).includes(token), 'token visible in page text');
      await page.screenshot({ path: join(temporary, `dashboard-${width}.png`), fullPage: true });
    }
    // Synthetic transport data exercises presentation only; the real engine has its own Linux smoke.
    const fixture = JSON.parse(await readFile(join(root, 'schemas/v1/fixtures/valid/container-inventory-running-stopped.json'), 'utf8'));
    fixture.observed_at = new Date().toISOString();
    assert(validators['container-inventory'](fixture), 'container browser fixture violates contract');
    await page.route('**/api/v1/containers?limit=500', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(fixture) }));
    for (const width of [390, 768, 1440]) {
      await page.setViewportSize({ width, height: 1000 });
      await page.reload();
      await page.getByRole('button', { name: 'Workloads', exact: true }).click();
      await page.getByRole('tab', { name: 'Containers', exact: true }).click();
      const table = page.getByRole('table', { name: 'Containers: name and image, state, host-capacity CPU, engine-reported memory, metrics quality, and alias', exact: true });
      await table.waitFor();
      const stopped = table.getByRole('row').filter({ hasText: 'sample-worker' });
      assert.equal(await stopped.count(), 1);
      assert((await stopped.innerText()).includes('Unavailable'), 'stopped container lacks unavailable label');
      assert(!(await stopped.innerText()).includes('0.0%'), 'stopped container appears to measure zero CPU');
      await page.getByLabel('Filter returned records', { exact: true }).fill('metrics-limited');
      assert.equal(await table.getByRole('row').count(), 2, 'container filter did not isolate one record');
      assert((await table.innerText()).includes('0.0%'), 'measured zero was hidden');
      await page.getByLabel('Filter returned records', { exact: true }).fill('');
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), `Populated containers overflow page at ${width}px`);
      await page.screenshot({ path: join(temporary, `containers-${width}.png`), fullPage: true });
    }
    await page.unroute('**/api/v1/containers?limit=500');
    // Synthetic health states exercise presentation without altering diagnostic files or host startup.
    const availableDiagnostics = JSON.parse(await readFile(join(root, 'schemas/v1/fixtures/valid/diagnostics-health-available.json'), 'utf8'));
    const unavailableDiagnostics = structuredClone(availableDiagnostics);
    unavailableDiagnostics.available = false;
    unavailableDiagnostics.state = 'UNAVAILABLE';
    unavailableDiagnostics.reason_code = 'DIAGNOSTICS_UNAVAILABLE';
    unavailableDiagnostics.usage = { total_bytes: 0, file_count: 0 };
    for (const fixture of [availableDiagnostics, unavailableDiagnostics]) {
      assert(validators['diagnostics-health'](fixture), 'diagnostics browser fixture violates contract');
    }
    let displayedDiagnostics = availableDiagnostics;
    await page.route('**/api/v1/diagnostics/health', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(displayedDiagnostics) }));
    for (const width of [390, 768, 1440]) {
      await page.setViewportSize({ width, height: 1000 });
      displayedDiagnostics = availableDiagnostics;
      await page.reload();
      await page.getByRole('button', { name: 'Observer & privacy', exact: true }).click();
      await page.getByRole('heading', { name: 'Self-diagnostics', exact: true }).waitFor();
      const storage = page.locator('article').filter({ has: page.getByText('Storage used', { exact: true }) });
      await storage.waitFor();
      assert(!(await storage.innerText()).includes('Unavailable'), 'available diagnostics usage was hidden');
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), `Diagnostics overflows at ${width}px`);
      await page.screenshot({ path: join(temporary, `diagnostics-available-${width}.png`), fullPage: true });
      displayedDiagnostics = unavailableDiagnostics;
      await page.getByRole('button', { name: 'Refresh', exact: true }).click();
      await storage.getByText('Unavailable', { exact: true }).waitFor();
      assert(!(await storage.innerText()).includes('0 B'), 'unavailable disk usage was presented as measured zero');
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), `Unavailable diagnostics overflows at ${width}px`);
      await page.screenshot({ path: join(temporary, `diagnostics-unavailable-${width}.png`), fullPage: true });
    }
    await page.unroute('**/api/v1/diagnostics/health');
    // Synthetic log summaries exercise the real embedded client without reading
    // native host logs. The complete 168-bucket grid must fit on mobile, not clip.
    const logFixture = JSON.parse(await readFile(join(root, 'schemas/v1/fixtures/valid/log-summary-1h.json'), 'utf8'));
    const logRanges = { '1h': [3600, 60, 60], '6h': [21600, 300, 72], '24h': [86400, 900, 96], '7d': [604800, 3600, 168] };
    await page.route('**/api/v1/logs/summary?range=*', route => {
      const range = new URL(route.request().url()).searchParams.get('range');
      const [duration, interval, count] = logRanges[range];
      const fixture = structuredClone(logFixture);
      const end = Math.floor(Date.parse(fixture.generated_at) / (interval * 1000)) * interval * 1000;
      const start = end - duration * 1000;
      fixture.range = range;
      fixture.window_start = new Date(start).toISOString();
      fixture.window_end = new Date(end).toISOString();
      fixture.bucket_interval_seconds = interval;
      fixture.expected_bucket_count = count;
      for (const source of fixture.sources) {
        const original = source.buckets;
        source.buckets = Array.from({ length: count }, (_, index) => ({
          ...(index < 4 ? original[index] : { coverage_state: 'UNKNOWN', covered_seconds: 0, reason_code: 'NOT_YET_OBSERVED', counts: null }),
          ...(index === 3 ? { covered_seconds: interval } : {}),
          at: new Date(start + index * interval * 1000).toISOString()
        }));
        source.covered_seconds = interval + 30;
      }
      assert(validators['log-summary'](fixture), 'log browser fixture violates schema');
      validateLogSummaryFixture(fixture, 'synthetic browser summary');
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(fixture) });
    });
    for (const width of [390, 768, 1440]) {
      await page.setViewportSize({ width, height: 1000 });
      await page.reload();
      await page.getByRole('button', { name: 'Logs', exact: true }).click();
      assert(await page.getByLabel('Filter safe metadata', { exact: true }).evaluate(element => element.getBoundingClientRect().height <= 60), `Log filter stretches vertically at ${width}px`);
      const selector = page.getByRole('group', { name: 'Log summary range', exact: true });
      for (const range of ['1h', '7d']) {
        await selector.getByRole('button', { name: range, exact: true }).click();
        const chart = page.getByRole('img', { name: /System captured severity histogram/ });
        await chart.waitFor();
        await page.waitForFunction(expected => document.querySelectorAll('.observer-log-histogram__bucket').length === expected, logRanges[range][2]);
        assert(await chart.evaluate(element => {
          const last = element.lastElementChild.getBoundingClientRect();
          return last.right <= element.getBoundingClientRect().right + 1;
        }), `Log buckets clipped at ${width}px for ${range}`);
        assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), `Log summary overflows at ${width}px`);
        await page.screenshot({ path: join(temporary, `log-summary-${range}-${width}.png`), fullPage: true });
      }
      await page.getByText('View accessible bucket data', { exact: true }).click();
      const region = page.getByRole('region', { name: 'System log summary bucket data', exact: true });
      await region.focus();
      assert(await region.evaluate(element => element === document.activeElement), `Log table is not keyboard-focusable at ${width}px`);
      if (width <= 768) assert(await region.evaluate(element => element.scrollWidth > element.clientWidth), `Log table lacks local scrolling at ${width}px`);
      assert((await region.innerText()).includes('Unavailable'), 'Unknown counts became fabricated zero');
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), `Expanded log table overflows at ${width}px`);
    }
    await page.unroute('**/api/v1/logs/summary?range=*');
    await page.getByRole('button', { name: 'Lock dashboard', exact: true }).click();
    await page.getByRole('button', { name: 'Unlock dashboard', exact: true }).waitFor();
    assert(!(await page.evaluate(() => JSON.stringify(sessionStorage))).includes(token), 'lock retained the token');
    assert.equal(failures.length, 0, `browser errors: ${failures.join('; ')}`);
    console.log(`Packaged browser checks passed at 390, 768 and 1440 pixels. Screenshots: ${temporary}`);
  }
  console.log('Native local service passed real snapshot, history, authentication and schema smoke checks.');
} finally {
  await browser?.close();
  if (processHandle.exitCode === null && processHandle.signalCode === null) {
    processHandle.kill('SIGTERM');
    const stopped = await Promise.race([processExited.then(() => true), delay(5000, false, { ref: false })]);
    if (!stopped) processHandle.kill('SIGKILL');
  }
  await processExited;
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
