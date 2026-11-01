import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { cleanEnvironment, fixedEndpoint, fullID, imageID, versionTuple, createArgs, ownedContainer, cleanupArgs, resultReceipt, wslPathArgs, wslNativeArgs, main } from './test-action-engine-native.mjs';

const expected = { id: 'a'.repeat(64), image: `sha256:${'b'.repeat(64)}`, run: 'c'.repeat(32), mode: 'graceful' };
function container() {
  return { Id: expected.id, Image: expected.image, Config: { User: '65532:65532', Entrypoint: ['/fixture'], Cmd: ['graceful'], Labels: { 'hlo.action-fixture': expected.run } },
    HostConfig: { NetworkMode: 'none', ReadonlyRootfs: true, Privileged: false, PublishAllPorts: false, AutoRemove: false,
      Memory: 33554432, MemorySwap: 33554432, NanoCpus: 250000000, PidsLimit: 64, CapAdd: null, CapDrop: ['ALL'], SecurityOpt: ['no-new-privileges:true'],
      Binds: null, VolumesFrom: null, Devices: [], DeviceRequests: null, PortBindings: {}, Tmpfs: null,
      PidMode: '', IpcMode: 'private', UTSMode: '', RestartPolicy: { Name: 'no' }, LogConfig: { Type: 'none' } }, Mounts: [] };
}
test('scrub daemon/context/proxy/TLS/credentials and compiler overrides without repurposing home', () => {
  const parent = { PATH: '/fixed/tools', HOME: '/real-user', USERPROFILE: 'C:\\real-user', DOCKER_HOST: 'ssh://bad.example', DOCKER_CONTEXT: 'foreign',
    DOCKER_API_VERSION: '1.999', DOCKER_TLS_VERIFY: '1', DOCKER_CERT_PATH: '/foreign', HTTP_PROXY: 'http://bad.example', HTTPS_PROXY: 'http://bad.example',
    ALL_PROXY: 'http://bad.example', DOCKER_CONFIG: '/foreign', SSH_AUTH_SOCK: '/foreign', GOFLAGS: '-toolexec=foreign', GOTOOLCHAIN: 'auto' };
  assert.deepEqual(cleanEnvironment(parent), { PATH: '/fixed/tools', HOME: '/real-user', USERPROFILE: 'C:\\real-user', LC_ALL: 'C' });
});
test('fixed local endpoints, not inherited remote contexts or arbitrary socket options', () => {
  assert.equal(fixedEndpoint('win32'), 'npipe:////./pipe/dockerDesktopLinuxEngine');
  assert.equal(fixedEndpoint('linux', '/run/docker.sock'), 'unix:///run/docker.sock');
  assert.equal(fixedEndpoint('linux', '/mnt/wsl/docker-desktop/shared-sockets/guest-services/docker.sock'), 'unix:///mnt/wsl/docker-desktop/shared-sockets/guest-services/docker.sock');
  for (const value of ['/tmp/docker.sock', '/run/../tmp/docker.sock', 'ssh://bad.example', '/mnt/wsl/docker-desktop/x?override']) assert.throws(() => fixedEndpoint('linux', value));
  assert.throws(() => fixedEndpoint('darwin'));
});
test('version profile is canonical and requires exact1.45 intersection', () => {
  for (const value of ['29.9.1|1.56|1.40|linux|amd64', '29.0.0|1.52|1.44|linux|amd64']) assert.equal(versionTuple(value), value);
  for (const value of ['29.9.1|1.56|1.46|linux|amd64', '29.9.1|1.44|1.40|linux|amd64', '29.9.1|1.040|1.40|linux|amd64',
    '29.9.1|1.56|1.40|windows|amd64', '29.9.1|1.56|1.40|linux|arm64', 'secret-canary', 'x'.repeat(160)]) assert.throws(() => versionTuple(value));
});
test('only full IDs and exact pinned image IDs are valid', () => {
  assert.equal(fullID(expected.id), true); assert.equal(imageID(expected.image), true);
  for (const value of ['fixture-name', 'a'.repeat(12), 'A'.repeat(64), 'a'.repeat(64) + '/start', null]) assert.equal(fullID(value), false);
  assert.equal(imageID('fixture:latest'), false);
});
test('create uses one fixed owned immutable mode and all isolation caps', () => {
  const args = createArgs(`hlo-action-${expected.run}-graceful`, expected.run, expected.image, 'graceful');
  assert.deepEqual(args, ['create', '--pull=never', '--name', `hlo-action-${expected.run}-graceful`, '--label', `hlo.action-fixture=${expected.run}`, '--network=none', '--read-only',
    '--cap-drop=ALL', '--security-opt=no-new-privileges=true', '--pids-limit=64', '--memory=32m', '--memory-swap=32m', '--cpus=0.25', '--restart=no', '--log-driver=none', expected.image, 'graceful']);
  assert.throws(() => createArgs('existing-workload', expected.run, expected.image, 'graceful'));
  assert.throws(() => createArgs(`hlo-action-${expected.run}-graceful`, expected.run, 'image:tag', 'graceful'));
  assert.throws(() => createArgs(`hlo-action-${expected.run}-arbitrary`, expected.run, expected.image, 'arbitrary'));
});
test('cleanup revalidates exact ownership and every authority-relevant cap', () => {
  assert.equal(ownedContainer(container(), expected), true);
	const withoutTmpfs = container(); delete withoutTmpfs.HostConfig.Tmpfs; assert.equal(ownedContainer(withoutTmpfs, expected), true);
  assert.deepEqual(cleanupArgs(container(), expected), ['rm', '--force', expected.id]);
  const changes = [c => c.Id = 'd'.repeat(64), c => c.Image = `sha256:${'d'.repeat(64)}`, c => c.Config.Labels['hlo.action-fixture'] = 'd'.repeat(32),
    c => c.Config.Labels.extra = 'foreign', c => c.Config.User = '0', c => c.Config.Cmd = ['ignore'], c => c.Config.Entrypoint = ['/bin/sh'],
    ...['ReadonlyRootfs', 'Privileged', 'PublishAllPorts', 'AutoRemove'].map(key => c => c.HostConfig[key] = !c.HostConfig[key]),
    ...['Memory', 'MemorySwap', 'NanoCpus', 'PidsLimit'].map(key => c => c.HostConfig[key]++),
    c => c.HostConfig.NetworkMode = 'host', c => c.HostConfig.CapAdd = ['SYS_ADMIN'], c => c.HostConfig.CapDrop = [], c => c.HostConfig.SecurityOpt = [],
    ...['Binds', 'VolumesFrom', 'Devices', 'DeviceRequests'].map(key => c => c.HostConfig[key] = ['foreign']),
    c => c.HostConfig.PortBindings = { '80/tcp': [{}] }, c => c.HostConfig.Tmpfs = { '/foreign': '' }, c => c.Mounts = [{}],
    c => c.HostConfig.PidMode = 'host', c => c.HostConfig.IpcMode = 'host', c => c.HostConfig.UTSMode = 'host',
    c => c.HostConfig.RestartPolicy.Name = 'always', c => c.HostConfig.LogConfig.Type = 'json-file'];
  for (const change of changes) { const c = container(); change(c); assert.equal(ownedContainer(c, expected), false); assert.throws(() => cleanupArgs(c, expected)); }
});
test('manual gate refuses before any tool or daemon command', async () => {
  await assert.rejects(main([]), /EXECUTION_NOT_APPROVED/);
  await assert.rejects(main(['--host=ssh://bad.example']), /EXECUTION_NOT_APPROVED/);
});
test('both fixed WSL bridges execute directly and preserve backslashes/spaces as single arguments', () => {
  const windowsPath = String.raw`C:\Users\Synthetic Owner\AppData\Local\Temp\fixture path\actionengine.test`;
  assert.deepEqual(wslPathArgs(windowsPath), ['-d', 'Ubuntu', '--user', 'root', '--exec', '/usr/bin/wslpath', '-a', '-u', windowsPath]);
  const binary = '/mnt/c/Users/Synthetic Owner/AppData/Local/Temp/fixture path/actionengine.test';
  const variables = { HLO_ENGINE_NATIVE_PHASE: 'version', HLO_ENGINE_NATIVE_VERSION: '29.9.1|1.56|1.40|linux|amd64' };
  assert.deepEqual(wslNativeArgs(binary, variables), ['-d', 'Ubuntu', '--user', 'root', '--exec', '/usr/bin/env', '-i',
    'HLO_ENGINE_NATIVE_PHASE=version', 'HLO_ENGINE_NATIVE_VERSION=29.9.1|1.56|1.40|linux|amd64', binary,
    '-test.run=^TestOwnedNativeFixture$', '-test.count=1', '-test.timeout=150s']);
  assert.throws(() => wslPathArgs('relative.test'));
  assert.throws(() => wslNativeArgs(binary, { ...variables, DOCKER_HOST: 'foreign' }));
  assert.throws(() => wslNativeArgs(binary, { ...variables, HLO_ENGINE_NATIVE_PHASE: 'arbitrary' }));
});
test('bounded private receipt names only two exact synthetic removals, not verified absence', () => {
  const fixtures = [{ mode: 'graceful', fullID: expected.id, removed_acknowledged: true }, { mode: 'ignore', fullID: 'd'.repeat(64), removed_acknowledged: true }];
  const hashes = { fixture: 'e'.repeat(64), test_binary: 'f'.repeat(64) };
  const receipt = entries => resultReceipt('1'.repeat(40), '29.9.1|1.56|1.40|linux|amd64', expected.image, hashes, expected.run, entries);
  const record = receipt(fixtures);
  assert.equal(record.run_id, expected.run);
  assert.deepEqual(record.fixtures, fixtures);
  assert.equal(record.cases, 2);
  assert.ok(Buffer.byteLength(JSON.stringify(record)) <= 2048);
  assert.doesNotMatch(JSON.stringify(record), /verified_absence|endpoint|credential/);
  for (const entries of [[], fixtures.slice(0, 1), [...fixtures, fixtures[0]], [null, fixtures[1]], [{ ...fixtures[0], removed_acknowledged: false }, fixtures[1]],
    [fixtures[0], { ...fixtures[1], fullID: fixtures[0].fullID }], [{ ...fixtures[0], endpoint: 'foreign' }, fixtures[1]]]) assert.throws(() => receipt(entries));
});
test('source keeps checked-in scratch-only build, sequential cases, fixed bridge and bounded subprocess capture', async () => {
  const source = await readFile(new URL('./test-action-engine-native.mjs', import.meta.url), 'utf8');
  const dockerfile = await readFile(new URL('./fixtures/action-engine/Dockerfile', import.meta.url), 'utf8');
  assert.equal(dockerfile.replaceAll('\r\n', '\n'), 'FROM scratch\nCOPY fixture /fixture\nUSER 65532:65532\nENTRYPOINT ["/fixture"]\n');
  assert.match(source, /for \(const mode of \['graceful', 'ignore'\]\)/);
  assert.match(source, /'--config', config, '--host', endpoint/);
  assert.match(source, /'--builder', 'default', '--network=none', '--pull=false'/);
  assert.match(source, /'Ubuntu', '--user', 'root', '--exec', '\/usr\/bin\/env', '-i'/);
  assert.doesNotMatch(source, /'root', '--', '\/usr\/bin\/(?:wslpath|env)'/);
  assert.match(source, /maxBuffer: 512 \* 1024/);
  assert.ok(source.indexOf("await docker(cleanupArgs(await inspect(), expected), 'CLEANUP_REFUSED');") < source.indexOf('removed.push('));
  assert.doesNotMatch(source, /Promise\.all|docker.*prune|console\.(?:log|error)\([^)]*(?:stdout|stderr|raw)/);
});
