// Manual, separately reviewed disposable-fixture proof. Never an automatic PR job.
import { spawnSync } from 'node:child_process';
import { createHash, randomUUID } from 'node:crypto';
import { chmod, copyFile, lstat, mkdir, mkdtemp, readFile, realpath, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('..', import.meta.url));
export const fullID = value => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value);
export const imageID = value => typeof value === 'string' && /^sha256:[a-f0-9]{64}$/.test(value);
const runID = value => typeof value === 'string' && /^[a-f0-9]{32}$/.test(value);
class Refusal extends Error { constructor(code) { super(code); this.code = code; } }
function need(value, code) { if (!value) throw new Refusal(code); }

export function cleanEnvironment(parent) {
  // No Docker/TLS/context/proxy/SSH/credential-helper or Go flag inheritance.
  const result = {};
  for (const name of ['PATH', 'Path', 'SystemRoot', 'WINDIR', 'ComSpec', 'PATHEXT', 'TEMP', 'TMP', 'HOME', 'USERPROFILE', 'LOCALAPPDATA', 'APPDATA']) {
    if (typeof parent[name] === 'string') result[name] = parent[name];
  }
  result.LC_ALL = 'C';
  return result;
}
export function fixedEndpoint(platform, canonical = '') {
  if (platform === 'win32') return 'npipe:////./pipe/dockerDesktopLinuxEngine';
  need(platform === 'linux' && (canonical === '/run/docker.sock' || canonical.startsWith('/mnt/wsl/docker-desktop/')) &&
    /^\/[A-Za-z0-9_./-]+$/.test(canonical) && path.posix.normalize(canonical) === canonical, 'ENDPOINT_REFUSED');
  return `unix://${canonical}`;
}
export function versionTuple(raw) {
  need(typeof raw === 'string' && raw.length < 160, 'VERSION_REFUSED');
  const value = raw.trim();
  need(/^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}(?:[-+][A-Za-z0-9.-]{1,32})?\|[1-9][0-9]{0,2}\.[0-9]{1,3}\|[1-9][0-9]{0,2}\.[0-9]{1,3}\|linux\|amd64$/.test(value), 'VERSION_REFUSED');
  const [, max, min] = value.split('|');
  const api = x => { const [a, b] = x.split('.').map(Number); need(`${a}.${b}` === x, 'VERSION_REFUSED'); return a * 1000 + b; };
  need(api(min) <= 1045 && api(max) >= 1045 && api(min) <= api(max), 'VERSION_REFUSED');
  return value;
}
export function createArgs(id, run, image, mode) {
  need(runID(run) && imageID(image) && ['graceful', 'ignore'].includes(mode) && id === `hlo-action-${run}-${mode}`, 'CREATE_REFUSED');
  return ['create', '--pull=never', '--name', id, '--label', `hlo.action-fixture=${run}`, '--network=none', '--read-only',
    '--cap-drop=ALL', '--security-opt=no-new-privileges=true', '--pids-limit=64', '--memory=32m', '--memory-swap=32m',
    '--cpus=0.25', '--restart=no', '--log-driver=none', image, mode];
}
export function ownedContainer(c, expected) {
  const h = c?.HostConfig, cfg = c?.Config;
  const empty = value => value == null || (Array.isArray(value) ? value.length === 0 : typeof value === 'object' && Object.keys(value).length === 0);
  return fullID(expected?.id) && imageID(expected?.image) && runID(expected?.run) && ['graceful', 'ignore'].includes(expected?.mode) &&
    c?.Id === expected.id && c.Image === expected.image && cfg?.User === '65532:65532' &&
    JSON.stringify(cfg.Entrypoint) === '["/fixture"]' && JSON.stringify(cfg.Cmd) === JSON.stringify([expected.mode]) &&
    cfg.Labels?.['hlo.action-fixture'] === expected.run && Object.keys(cfg.Labels).length === 1 &&
    h?.NetworkMode === 'none' && h.ReadonlyRootfs === true && h.Privileged === false && h.PublishAllPorts === false && h.AutoRemove === false &&
    h.Memory === 33554432 && h.MemorySwap === 33554432 && h.NanoCpus === 250000000 && h.PidsLimit === 64 &&
    empty(h.CapAdd) && JSON.stringify(h.CapDrop) === '["ALL"]' && JSON.stringify(h.SecurityOpt) === '["no-new-privileges:true"]' &&
    [h.Binds, h.VolumesFrom, h.Devices, h.DeviceRequests, h.PortBindings, h.Tmpfs, c.Mounts].every(empty) &&
    h.PidMode === '' && h.IpcMode === 'private' && h.UTSMode === '' && h.RestartPolicy?.Name === 'no' && h.LogConfig?.Type === 'none';
}
export function cleanupArgs(container, expected) {
  need(ownedContainer(container, expected), 'CLEANUP_IDENTITY_REFUSED');
  return ['rm', '--force', expected.id];
}
export function resultReceipt(source, version, image, hashes, run, fixtures) {
  need(/^[a-f0-9]{40}$/.test(source) && imageID(image) && runID(run) && fullID(hashes?.fixture) && fullID(hashes?.test_binary), 'RECEIPT_REFUSED');
  need(Array.isArray(fixtures) && fixtures.length === 2 &&
    fixtures.every((entry, index) => entry && typeof entry === 'object' && !Array.isArray(entry) && entry.mode === ['graceful', 'ignore'][index] && fullID(entry.fullID) && entry.removed_acknowledged === true &&
      Object.keys(entry).sort().join('|') === 'fullID|mode|removed_acknowledged') && fixtures[0].fullID !== fixtures[1].fullID, 'RECEIPT_REFUSED');
  const record = { schema: 'hlo-owned-action-fixture/v1', source, version: versionTuple(version), image,
    hashes: { fixture: hashes.fixture, test_binary: hashes.test_binary }, run_id: run, fixtures,
    cases: 2, scope: 'DISPOSABLE_PROTOCOL_PROOF_NOT_AUTHORITY_OR_RELEASE' };
  need(Buffer.byteLength(JSON.stringify(record), 'utf8') <= 2048, 'RECEIPT_REFUSED');
  return record;
}

function command(program, args, env, code, timeout = 120000) {
  const result = spawnSync(program, args, { cwd: root, env, encoding: 'utf8', timeout, maxBuffer: 512 * 1024, windowsHide: true });
  // Never include captured compiler/daemon/test output in an exception or log.
  if (result.error || result.status !== 0 || typeof result.stdout !== 'string') {
    if (code === 'NATIVE_FIXTURE_REFUSED' && typeof result.stdout === 'string') {
      const stages = result.stdout.split(/\r?\n/).filter(line => /^HLO_ENGINE_NATIVE_STAGE=(VERSION|IDENTITY|START|STOP|RESTART|REPLAY)$/.test(line));
      if (stages.length) throw new Refusal(`NATIVE_${stages.at(-1).split('=')[1]}_REFUSED`);
    }
    throw new Refusal(code);
  }
  return result.stdout;
}
function exactMarker(output, prefix) {
  const matches = output.split(/\r?\n/).filter(line => line.startsWith(prefix));
  need(matches.length === 1, 'NATIVE_MARKER_REFUSED');
  return matches[0].slice(prefix.length);
}
export function wslPathArgs(filename) {
  need(typeof filename === 'string' && /^[A-Za-z]:[\\/]/.test(filename) && !filename.includes('\0'), 'WSL_PATH_REFUSED');
  return ['-d', 'Ubuntu', '--user', 'root', '--exec', '/usr/bin/wslpath', '-a', '-u', filename];
}
export function wslNativeArgs(binary, variables) {
  need(typeof binary === 'string' && /^\/mnt\/[a-z]\/[A-Za-z0-9_./ -]+$/.test(binary) && !binary.includes('..'), 'WSL_PATH_REFUSED');
  const keys = Object.keys(variables);
  need(keys.every(key => ['HLO_ENGINE_NATIVE_PHASE', 'HLO_ENGINE_NATIVE_VERSION', 'HLO_ENGINE_NATIVE_ID', 'HLO_ENGINE_NATIVE_IMAGE', 'HLO_ENGINE_NATIVE_RUN'].includes(key)), 'NATIVE_ARGUMENTS_REFUSED');
  const phase = variables.HLO_ENGINE_NATIVE_PHASE;
  need(['version', 'graceful', 'ignore'].includes(phase), 'NATIVE_ARGUMENTS_REFUSED');
  versionTuple(variables.HLO_ENGINE_NATIVE_VERSION);
  need(phase === 'version' ? keys.length === 2 : keys.length === 5 && fullID(variables.HLO_ENGINE_NATIVE_ID) && imageID(variables.HLO_ENGINE_NATIVE_IMAGE) && runID(variables.HLO_ENGINE_NATIVE_RUN), 'NATIVE_ARGUMENTS_REFUSED');
  return ['-d', 'Ubuntu', '--user', 'root', '--exec', '/usr/bin/env', '-i',
    ...Object.entries(variables).map(([key, value]) => `${key}=${value}`), binary,
    '-test.run=^TestOwnedNativeFixture$', '-test.count=1', '-test.timeout=150s'];
}

export async function main(argv = process.argv.slice(2)) {
  need(argv.length === 1 && argv[0] === '--allow-owned-fixtures', 'EXECUTION_NOT_APPROVED');
  need(['linux', 'win32'].includes(process.platform) && process.arch === 'x64', 'PLATFORM_REFUSED');
  const env = cleanEnvironment(process.env);
  need(command('git', ['status', '--porcelain', '--untracked-files=all'], env, 'SOURCE_REFUSED').trim() === '', 'SOURCE_REFUSED');
  const source = command('git', ['rev-parse', 'HEAD'], env, 'SOURCE_REFUSED').trim();
  need(/^[a-f0-9]{40}$/.test(source), 'SOURCE_REFUSED');
  need(/^go version go1\.27\.3 (linux|windows)\/amd64\s*$/.test(command('go', ['version'], env, 'TOOLCHAIN_REFUSED')), 'TOOLCHAIN_REFUSED');
  const temporary = await mkdtemp(path.join(tmpdir(), 'hlo-action-engine-'));
  await chmod(temporary, 0o700);
  const config = path.join(temporary, 'docker-config');
  await mkdir(config, { mode: 0o700 });
  await writeFile(path.join(config, 'config.json'), '{}\n', { flag: 'wx', mode: 0o600 });
  const endpoint = fixedEndpoint(process.platform, process.platform === 'linux' ? await realpath('/var/run/docker.sock') : '');
  const docker = async (args, code, timeout) => {
    need((await readFile(path.join(config, 'config.json'), 'utf8')) === '{}\n', 'CONFIG_REFUSED');
    return command('docker', ['--config', config, '--host', endpoint, ...args], env, code, timeout);
  };
  const cliVersion = () => docker(['version', '--format', '{{.Server.Version}}|{{.Server.APIVersion}}|{{.Server.MinAPIVersion}}|{{.Server.Os}}|{{.Server.Arch}}'], 'CLI_VERSION_REFUSED');
  const version = versionTuple(await cliVersion());
  const buildEnv = { ...env, GOOS: 'linux', GOARCH: 'amd64', CGO_ENABLED: '0', GOPROXY: 'off', GOSUMDB: 'off',
    GOTOOLCHAIN: 'local', GOENV: 'off', GOFLAGS: '-mod=readonly', GOMAXPROCS: '2', GOMEMLIMIT: '256MiB' };
  command('go', ['mod', 'verify'], buildEnv, 'MODULES_REFUSED');
  const contextDir = path.join(temporary, 'context');
  await mkdir(contextDir, { mode: 0o700 });
  await copyFile(path.join(root, 'scripts/fixtures/action-engine/Dockerfile'), path.join(contextDir, 'Dockerfile'));
  command('go', ['build', '-trimpath', '-o', path.join(contextDir, 'fixture'), './scripts/fixtures/action-engine'], buildEnv, 'FIXTURE_BUILD_REFUSED');
  const testBinary = path.join(temporary, 'actionengine.test');
  command('go', ['test', '-c', '-trimpath', '-o', testBinary, './internal/actionengine'], buildEnv, 'TEST_BUILD_REFUSED');
  await chmod(testBinary, 0o700);
  const hashes = {};
  for (const [name, filename] of [['fixture', path.join(contextDir, 'fixture')], ['test_binary', testBinary]]) {
    const stat = await lstat(filename);
    need(stat.isFile() && !stat.isSymbolicLink() && stat.nlink === 1 && stat.size > 0 && stat.size < 32 * 1024 * 1024, 'BINARY_REFUSED');
    hashes[name] = createHash('sha256').update(await readFile(filename)).digest('hex');
  }
  need((await readFile(path.join(contextDir, 'Dockerfile'), 'utf8')).replaceAll('\r\n', '\n') ===
    'FROM scratch\nCOPY fixture /fixture\nUSER 65532:65532\nENTRYPOINT ["/fixture"]\n', 'DOCKERFILE_REFUSED');
  let linuxBinary = testBinary;
  if (process.platform === 'win32') {
    linuxBinary = command('wsl.exe', wslPathArgs(testBinary), env, 'WSL_PATH_REFUSED').trim();
    need(/^\/mnt\/[a-z]\/[A-Za-z0-9_./ -]+$/.test(linuxBinary) && !linuxBinary.includes('..'), 'WSL_PATH_REFUSED');
  }
  const native = (phase, extra = {}) => {
    const variables = { HLO_ENGINE_NATIVE_PHASE: phase, HLO_ENGINE_NATIVE_VERSION: version, ...extra };
    const testArgs = ['-test.run=^TestOwnedNativeFixture$', '-test.count=1', '-test.timeout=150s'];
    if (process.platform === 'win32') {
      return command('wsl.exe', wslNativeArgs(linuxBinary, variables), env, 'NATIVE_FIXTURE_REFUSED', 180000);
    }
    return command(linuxBinary, testArgs, { ...env, ...variables }, 'NATIVE_FIXTURE_REFUSED', 180000);
  };
  need(versionTuple(exactMarker(native('version'), 'HLO_ENGINE_NATIVE_VERSION=')) === version, 'DAEMON_MISMATCH');
  const run = randomUUID().replaceAll('-', '');
  const image = (await docker(['build', '--builder', 'default', '--network=none', '--pull=false', '--quiet', '--tag', `hlo-action-fixture:${run}`, '--file', path.join(contextDir, 'Dockerfile'), contextDir], 'IMAGE_BUILD_REFUSED')).trim();
  need(imageID(image), 'IMAGE_ID_REFUSED');
  need((await docker(['image', 'inspect', '--format', '{{.Id}}', image], 'IMAGE_ID_REFUSED')).trim() === image, 'IMAGE_ID_REFUSED');
  let cases = 0;
  const removed = [];
  for (const mode of ['graceful', 'ignore']) {
    need(versionTuple(await cliVersion()) === version && versionTuple(exactMarker(native('version'), 'HLO_ENGINE_NATIVE_VERSION=')) === version, 'DAEMON_MISMATCH');
    const name = `hlo-action-${run}-${mode}`;
    const id = (await docker(createArgs(name, run, image, mode), 'CREATE_REFUSED')).trim();
    need(fullID(id), 'CREATED_ID_REFUSED');
    const expected = { id, run, image, mode };
    const inspect = async () => {
      const raw = await docker(['inspect', '--type=container', id], 'OWNERSHIP_REFUSED');
      let result;
      try { result = JSON.parse(raw); } catch { throw new Refusal('OWNERSHIP_REFUSED'); }
      need(Array.isArray(result) && result.length === 1 && ownedContainer(result[0], expected), 'OWNERSHIP_REFUSED');
      return result[0];
    };
    let primary;
    try {
      await inspect();
      const output = native(mode, { HLO_ENGINE_NATIVE_ID: id, HLO_ENGINE_NATIVE_IMAGE: image, HLO_ENGINE_NATIVE_RUN: run });
      need(exactMarker(output, 'HLO_ENGINE_NATIVE_CASE_PASS=') === mode, 'NATIVE_MARKER_REFUSED');
      cases++;
    } catch (error) { primary = error; }
    try {
      need(versionTuple(await cliVersion()) === version, 'DAEMON_MISMATCH');
      await docker(cleanupArgs(await inspect(), expected), 'CLEANUP_REFUSED');
      // Acknowledgement only. Independent exact-ID absence checks are separate.
      removed.push({ mode, fullID: id, removed_acknowledged: true });
    } catch {
      if (primary) console.error(`ACTION_ENGINE_NATIVE_PRIMARY=${primary instanceof Refusal ? primary.code : 'UNEXPECTED_REFUSAL'}`);
      throw new Refusal('CLEANUP_REFUSED_RETAINED_FIXTURE');
    }
    if (primary) throw primary;
  }
  need(command('git', ['status', '--porcelain', '--untracked-files=all'], env, 'SOURCE_REFUSED').trim() === '', 'SOURCE_REFUSED');
  need(cases === 2, 'RECEIPT_REFUSED');
  await writeFile(path.join(temporary, 'result.json'), JSON.stringify(resultReceipt(source, version, image, hashes, run, removed)) + '\n', { flag: 'wx', mode: 0o600 });
  // Keep the private source-built image/context/result for audit. Never prune
  // images or enumerate/delete another workload to clean a test environment.
  console.log('ACTION_ENGINE_NATIVE_PASS cases=2 scope=DISPOSABLE_PROTOCOL_ONLY');
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch(error => { console.error(`ACTION_ENGINE_NATIVE_REFUSED=${error instanceof Refusal ? error.code : 'UNEXPECTED_REFUSAL'}`); process.exitCode = 1; });
}
