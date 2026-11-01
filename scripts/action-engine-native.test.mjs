import test from 'node:test';
import assert from 'node:assert/strict';
import { lstat, mkdtemp, readFile, rmdir, unlink } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import * as native from './test-action-engine-native.mjs';
import { cleanEnvironment, fixedEndpoint, fullID, imageID, versionTuple, approvedDefaultBuilder, createArgs, ownedContainer, cleanupArgs, resultReceipt, wslPathArgs, wslNativeArgs, nativeOptions, approvedBuildxSource, installBuildxSource, buildxPin, main } from './test-action-engine-native.mjs';

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
test('read-only default builder must be one docker node bound to the explicit local endpoint', () => {
  const endpoint = 'unix:///run/docker.sock';
  const output = 'Name: default\nDriver: docker\n\nNodes:\nName: default\nEndpoint: default\nStatus: running\nBuildKit version: v0.25.1\nPlatforms: linux/amd64\n';
  assert.equal(approvedDefaultBuilder(output, endpoint, endpoint), true);
  assert.equal(approvedDefaultBuilder(output.replace('Endpoint: default', `Endpoint: ${endpoint}`), endpoint, endpoint), true);
  for (const bad of [output.replace('Driver: docker', 'Driver: docker-container'), output.replace('Endpoint: default', 'Endpoint: tcp://foreign.example:2375'),
    output + 'Endpoint: default\n', output + 'Driver: docker\n', output.replace('Status: running', 'Status: error'), output + 'Error: refused\n',
    'malformed', 'x'.repeat(16385)]) assert.equal(approvedDefaultBuilder(bad, endpoint, endpoint), false);
  assert.equal(approvedDefaultBuilder(output, endpoint, 'unix:///foreign.sock'), false);
});
test('only full IDs and exact pinned image IDs are valid', () => {
  assert.equal(fullID(expected.id), true); assert.equal(imageID(expected.image), true);
  for (const value of ['fixture-name', 'a'.repeat(12), 'A'.repeat(64), 'a'.repeat(64) + '/start', null]) assert.equal(fullID(value), false);
  assert.equal(imageID('fixture:latest'), false);
});
test('create uses one fixed owned immutable mode and all isolation caps', () => {
  const args = createArgs(`hlo-action-${expected.run}-graceful`, expected.run, expected.image, 'graceful');
  assert.deepEqual(args, ['create', '--pull=never', '--name', `hlo-action-${expected.run}-graceful`, '--label', `hlo.action-fixture=${expected.run}`, '--network=none', '--read-only',
    '--cap-drop=ALL', '--security-opt=no-new-privileges:true', '--pids-limit=64', '--memory=32m', '--memory-swap=32m', '--cpus=0.25', '--restart=no', '--log-driver=none', expected.image, 'graceful']);
  assert.equal(args.find(arg => arg.startsWith('--security-opt=')).slice('--security-opt='.length), container().HostConfig.SecurityOpt[0]);
  const wrongSpelling = container(); wrongSpelling.HostConfig.SecurityOpt = ['no-new-privileges=true'];
  assert.equal(ownedContainer(wrongSpelling, expected), false);
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
test('only Linux may opt into one pinned private Buildx source', () => {
  assert.deepEqual(nativeOptions(['--allow-owned-fixtures'], 'win32'), { buildxSource: null });
  assert.deepEqual(nativeOptions(['--allow-owned-fixtures', '--buildx-source', '/private/task/buildx'], 'linux'), { buildxSource: '/private/task/buildx' });
  for (const args of [['--allow-owned-fixtures', '--buildx-source', '/private/task/buildx', '--buildx-source', '/other'],
    ['--allow-owned-fixtures', '--buildx-source'], ['--allow-owned-fixtures', '--buildx-source', 'relative'],
    ['--allow-owned-fixtures', '--buildx-source', 'https://invalid.example/tool'], ['--allow-owned-fixtures', '--buildx-version', 'latest']]) assert.throws(() => nativeOptions(args, 'linux'));
  assert.throws(() => nativeOptions(['--allow-owned-fixtures', '--buildx-source', '/private/task/buildx'], 'win32'));
});
test('Buildx source requires exact private root-owned file metadata and code-owned hash', async () => {
  const file = () => ({ isFile: () => true, isSymbolicLink: () => false, nlink: 1, uid: 0, mode: 0o100700, size: buildxPin.size });
  const parent = () => ({ isDirectory: () => true, isSymbolicLink: () => false, uid: 0, mode: 0o40700 });
  assert.doesNotThrow(() => approvedBuildxSource(file(), parent(), buildxPin.sha256));
  for (const mutation of [f => f.isFile = () => false, f => f.isSymbolicLink = () => true, f => f.nlink = 2, f => f.uid = 1000, f => f.mode = 0o100755, f => f.size++]) {
    const f = file(); mutation(f); assert.throws(() => approvedBuildxSource(f, parent(), buildxPin.sha256));
  }
  for (const mutation of [p => p.isDirectory = () => false, p => p.isSymbolicLink = () => true, p => p.uid = 1000, p => p.mode = 0o40755]) {
    const p = parent(); mutation(p); assert.throws(() => approvedBuildxSource(file(), p, buildxPin.sha256));
  }
  assert.throws(() => approvedBuildxSource(file(), parent(), '0'.repeat(64)));
  await assert.rejects(installBuildxSource('/this-fixed-fixture-does-not-exist/buildx', '/unused'), /BUILDX_SOURCE_REFUSED/);
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
  const withBuildx = resultReceipt('1'.repeat(40), '29.9.1|1.56|1.40|linux|amd64', expected.image, hashes, expected.run, fixtures, { version: buildxPin.version, sha256: buildxPin.sha256 });
  assert.equal(withBuildx.buildx.sha256, buildxPin.sha256);
  assert.throws(() => resultReceipt('1'.repeat(40), '29.9.1|1.56|1.40|linux|amd64', expected.image, hashes, expected.run, fixtures, { version: 'latest', sha256: buildxPin.sha256 }));
  assert.doesNotMatch(JSON.stringify(record), /verified_absence|endpoint|credential/);
  for (const entries of [[], fixtures.slice(0, 1), [...fixtures, fixtures[0]], [null, fixtures[1]], [{ ...fixtures[0], removed_acknowledged: false }, fixtures[1]],
    [fixtures[0], { ...fixtures[1], fullID: fixtures[0].fullID }], [{ ...fixtures[0], endpoint: 'foreign' }, fixtures[1]]]) assert.throws(() => receipt(entries));
});
test('Linux recovery stages are persistent and the public marker is only a bounded synthetic basename', () => {
  assert.equal(native.stageParent('linux', '/session/tmp'), '/var/tmp');
  assert.equal(native.stageParent('win32', 'C:\\Synthetic Owner\\Temp'), 'C:\\Synthetic Owner\\Temp');
  assert.throws(() => native.stageParent('darwin', '/tmp'));
  assert.equal(native.stageReference('/var/tmp/hlo-action-engine-Ab123Z'), 'hlo-action-engine-Ab123Z');
  for (const value of ['/var/tmp/foreign', '/var/tmp/hlo-action-engine-secret-canary', '/var/tmp/hlo-action-engine-a\nsecret']) assert.throws(() => native.stageReference(value));
});
test('created identity audit is bounded, closed and explicitly not verified ownership', async () => {
  const source = '1'.repeat(40);
  const record = native.createdReceipt(source, expected);
  assert.deepEqual(record, { schema: 'hlo-created-action-fixture/v1', source, run_id: expected.run, full_id: expected.id,
    image: expected.image, mode: expected.mode, scope: 'CAPTURED_ID_NOT_VERIFIED_OWNERSHIP' });
  assert.ok(Buffer.byteLength(JSON.stringify(record)) <= 512);
  assert.doesNotMatch(JSON.stringify(record), /endpoint|credential|removed|accepted|verified_absence/);
  for (const value of [{ ...expected, id: 'short' }, { ...expected, mode: '../foreign' }, { ...expected, run: 'secret-canary' }, { ...expected, image: 'latest' }]) assert.throws(() => native.createdReceipt(source, value));
  assert.throws(() => native.createdReceipt('secret-canary', expected));
  const directory = await mkdtemp(path.join(tmpdir(), 'hlo-created-audit-test-'));
  const filename = path.join(directory, 'created-graceful.json');
  try {
    await native.recordCreatedFixture(directory, source, expected);
    assert.deepEqual(JSON.parse(await readFile(filename, 'utf8')), record);
    const stat = await lstat(filename);
    assert.equal(stat.isFile(), true); assert.equal(stat.nlink, 1);
    if (process.platform !== 'win32') assert.equal(stat.mode & 0o777, 0o600);
    await assert.rejects(native.recordCreatedFixture(directory, source, { ...expected, id: 'd'.repeat(64) }));
    assert.deepEqual(JSON.parse(await readFile(filename, 'utf8')), record);
  } finally { await unlink(filename).catch(() => {}); await rmdir(directory); }
});
test('creation intent records only the exact derived name and refuses overwrite before CLI creation', async () => {
  const source = '1'.repeat(40);
  const record = native.creationIntent(source, expected.run, expected.image, expected.mode);
  assert.deepEqual(record, { schema: 'hlo-action-fixture-intent/v1', source, run_id: expected.run, image: expected.image,
    mode: expected.mode, name: `hlo-action-${expected.run}-graceful`, scope: 'CREATE_INTENT_NOT_CREATION_ACKNOWLEDGEMENT' });
  assert.ok(Buffer.byteLength(JSON.stringify(record)) <= 512);
  for (const args of [['secret-canary', expected.run, expected.image, 'graceful'], [source, 'short', expected.image, 'graceful'],
    [source, expected.run, 'latest', 'graceful'], [source, expected.run, expected.image, '../foreign']]) assert.throws(() => native.creationIntent(...args));
  const directory = await mkdtemp(path.join(tmpdir(), 'hlo-intent-audit-test-'));
  const filename = path.join(directory, 'intent-graceful.json');
  try {
    await native.recordCreationIntent(directory, source, expected.run, expected.image, expected.mode);
    assert.deepEqual(JSON.parse(await readFile(filename, 'utf8')), record);
    const stat = await lstat(filename);
    assert.equal(stat.isFile(), true); assert.equal(stat.nlink, 1);
    if (process.platform !== 'win32') assert.equal(stat.mode & 0o777, 0o600);
    await assert.rejects(native.recordCreationIntent(directory, source, expected.run, expected.image, expected.mode));
    assert.deepEqual(JSON.parse(await readFile(filename, 'utf8')), record);
  } finally { await unlink(filename).catch(() => {}); await rmdir(directory); }
});
test('source keeps checked-in scratch-only build, sequential cases, fixed bridge and bounded subprocess capture', async () => {
  const source = await readFile(new URL('./test-action-engine-native.mjs', import.meta.url), 'utf8');
  const dockerfile = await readFile(new URL('./fixtures/action-engine/Dockerfile', import.meta.url), 'utf8');
  assert.equal(dockerfile.replaceAll('\r\n', '\n'), 'FROM scratch\nCOPY fixture /fixture\nUSER 65532:65532\nENTRYPOINT ["/fixture"]\n');
  assert.match(source, /for \(const mode of \['graceful', 'ignore'\]\)/);
  assert.match(source, /'--config', config, '--host', endpoint/);
  assert.match(source, /\['buildx', 'build', '--builder', 'default', '--network=none', '--pull=false'/);
  assert.match(source, /copyFile\(source, target, fsConstants\.COPYFILE_EXCL\)/);
  assert.match(source, /chmod\(target, 0o700\)/);
  assert.match(source, /if \(privateBuildx\) await pinnedBuildx\(privateBuildx\.target\)/);
  assert.match(source, /\['buildx', 'inspect', 'default'\]/);
  assert.match(source, /\['context', 'inspect', 'default', '--format', '\{\{\.Endpoints\.docker\.Host\}\}'\]/);
  assert.doesNotMatch(source, /'--bootstrap'|\['buildx', 'create'/);
  assert.match(source, /'Ubuntu', '--user', 'root', '--exec', '\/usr\/bin\/env', '-i'/);
  assert.doesNotMatch(source, /'root', '--', '\/usr\/bin\/(?:wslpath|env)'/);
  assert.match(source, /maxBuffer: 512 \* 1024/);
  assert.match(source, /mkdtemp\(path.join\(stageParent\(process.platform, tmpdir\(\)\), 'hlo-action-engine-'\)\)/);
  assert.match(source, /ACTION_ENGINE_NATIVE_STAGE=\$\{stageReference\(temporary\)\}/);
  assert.ok(source.indexOf('await recordCreatedFixture(temporary, source, expected);') > source.indexOf("need(fullID(id), 'CREATED_ID_REFUSED');"));
  assert.ok(source.indexOf('await recordCreatedFixture(temporary, source, expected);') < source.indexOf('const inspect = async'));
  const intentAt = source.indexOf('await recordCreationIntent(temporary, source, run, image, mode);');
  assert.ok(intentAt >= 0 && intentAt < source.indexOf('const id = (await docker(createArgs('));
  assert.match(source, /await handle.sync\(\)/);
  assert.match(source, /await directoryHandle.sync\(\)/);
  assert.ok(source.indexOf("await docker(cleanupArgs(await inspect(), expected), 'CLEANUP_REFUSED');") < source.indexOf('removed.push('));
  assert.doesNotMatch(source, /Promise\.all|docker.*prune|console\.(?:log|error)\([^)]*(?:stdout|stderr|raw)/);
});
