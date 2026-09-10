import { spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { copyFile, mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('..', import.meta.url));
const label = 'observer.missing-runtime-fixture';
const fail = () => { throw new Error('MISSING_RUNTIME_PROOF_FAILED'); };

export function dockerPlan(context, nonce) {
  if (!/^[a-f0-9-]{36}$/.test(nonce)) fail();
  const name = `observer-missing-runtime-${nonce}`;
  const image = `observer-missing-runtime:${nonce}`;
  return {
    name, image,
    build: ['build', '--network=none', '--pull=false', '--label', `${label}=${nonce}`, '--tag', image, context],
    create: ['create', '--name', name, '--label', `${label}=${nonce}`, '--network=none', '--read-only',
      '--cap-drop=ALL', '--security-opt=no-new-privileges', '--memory=256m', '--cpus=1', '--pids-limit=64',
      '--log-driver=none', '--tmpfs', '/state:rw,noexec,nosuid,nodev,size=64m,mode=0700,uid=65532,gid=65532', image],
  };
}

function command(program, args, timeout = 120_000, env = process.env) {
  const result = spawnSync(program, args, { cwd: root, env, encoding: 'utf8', timeout,
    killSignal: 'SIGKILL', maxBuffer: 1024 * 1024, windowsHide: true });
  if (result.error || result.signal || result.status !== 0) fail();
  return result.stdout;
}

async function main() {
  if (process.platform !== 'linux' || !['x64', 'arm64'].includes(process.arch) || process.argv.length !== 3) fail();
  const release = resolve(process.argv[2]);
  const temporary = await mkdtemp(join(tmpdir(), 'observer-missing-runtime-'));
  const context = join(temporary, 'context');
  const nonce = randomUUID();
  const plan = dockerPlan(context, nonce);
  let buildAttempted = false;
  let createAttempted = false;
  let failure = false;
  try {
    const goEnv = { ...process.env, CGO_ENABLED: '0', GOOS: 'linux', GOARCH: process.arch === 'x64' ? 'amd64' : 'arm64',
      GOENV: 'off', GOWORK: 'off', GOFLAGS: '-buildvcs=false', GOEXPERIMENT: '', GOTOOLCHAIN: 'local', GOAMD64: 'v1', GOARM64: 'v8.0' };
    const probe = join(temporary, 'probe');
    command('go', ['build', '-trimpath', '-o', probe, './scripts/fixtures/missing-linux-runtime'], 180_000, goEnv);
    command(probe, ['--stage', release, context]);
    await copyFile(probe, join(context, 'probe'));
    await copyFile(join(root, 'scripts/fixtures/missing-linux-runtime/Dockerfile'), join(context, 'Dockerfile'));
    buildAttempted = true;
    command('docker', plan.build, 180_000);
    createAttempted = true;
    const id = command('docker', plan.create, 30_000).trim();
    if (!/^[a-f0-9]{64}$/.test(id)) fail();
    const output = command('docker', ['start', '--attach', id], 80_000);
    if (output.trim() !== 'MISSING_RUNTIME_PROOF_PASSED') fail();
    if (command('docker', ['inspect', '--format', '{{.State.ExitCode}}', id], 15_000).trim() !== '0') fail();
  } catch { failure = true; }
  finally {
    // Labels prevent a coincidentally matching name from authorizing deletion. Never prune global Docker state.
    for (const [attempted, kind, target] of [[createAttempted, 'container', plan.name], [buildAttempted, 'image', plan.image]]) {
      if (!attempted) continue;
      try {
        const observed = command('docker', [kind, 'inspect', '--format', `{{index .Config.Labels "${label}"}}`, target], 15_000).trim();
        if (observed !== nonce) fail();
        command('docker', kind === 'container' ? ['container', 'rm', '--force', target] : ['image', 'rm', target], 30_000);
      } catch { failure = true; }
    }
    try { await rm(temporary, { recursive: true, force: true }); } catch { failure = true; }
  }
  if (failure) fail();
  console.log('MISSING_RUNTIME_PROOF_PASSED');
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch(() => { console.error('MISSING_RUNTIME_PROOF_FAILED'); process.exitCode = 1; });
}
