import test from 'node:test';
import assert from 'node:assert/strict';
import { dockerPlan } from './test-missing-linux-runtime.mjs';

test('scratch run has no network/socket/host mount and only private bounded state', () => {
  const plan = dockerPlan('/owned/context', '01234567-0123-0123-0123-0123456789ab');
  for (const value of ['--network=none', '--read-only', '--cap-drop=ALL', '--security-opt=no-new-privileges', '--log-driver=none', '--pids-limit=64']) {
    assert(plan.create.includes(value));
  }
  assert(plan.create.includes('/state:rw,noexec,nosuid,nodev,size=64m,mode=0700,uid=65532,gid=65532'));
  for (const value of ['--volume', '-v', '--mount', '--privileged', '--publish', '-p']) assert(!plan.create.includes(value));
  assert(plan.build.includes('--network=none'));
  assert(plan.build.includes('--pull=false'));
  assert.throws(() => dockerPlan('/owned/context', 'untrusted'), /MISSING_RUNTIME_PROOF_FAILED/);
});
