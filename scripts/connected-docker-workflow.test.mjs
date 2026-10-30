import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import test from 'node:test';

const file = new URL('../.github/workflows/connected-linux-docker-candidate.yml',import.meta.url);
const workflow = (await readFile(file,'utf8')).replaceAll('\r\n','\n');
const name = 'connected-linux-docker-${{ needs.authorize.outputs.version }}-${{ needs.authorize.outputs.run_id }}-${{ needs.authorize.outputs.run_attempt }}';

function closedGraph(source) {
  assert.match(source,/on:\n  workflow_dispatch:/u);
  assert.doesNotMatch(source,/^  (?:push|pull_request|workflow_call):/mu);
  assert.doesNotMatch(source,/runner\.temp/u);
  const blocks = source.split('\njobs:\n')[1].split(/\n(?= {2}[a-z]+:\n)/u);
  const jobs = Object.fromEntries(blocks.map(block=>[block.match(/^ {2}([a-z]+):/u)[1],block]));
  assert.deepEqual(Object.keys(jobs),['authorize','build','independent','compare','attest']);
  assert.match(jobs.authorize,/github\.repository_id == '1362278980'.*github\.repository_owner_id == '44215293'.*github\.ref == 'refs\/heads\/main'.*github\.actor == 'braidenm'.*github\.triggering_actor == 'braidenm'/u);
  for (const role of ['build','independent']) {
    const job=jobs[role];
    assert.match(job,/needs: authorize/u);
    assert.match(job,/runs-on: ubuntu-24\.04/u);
    assert.match(job,/go-version: '1\.27\.3'\n {10}cache: false/u);
    assert.doesNotMatch(job,/runner\.temp/u,'runner context is not available in job env');
    const cacheRole=role==='build'?'primary':'independent';
    assert(job.includes(`mkdir -m 700 -- "$RUNNER_TEMP/docker-${cacheRole}-go-cache" "$RUNNER_TEMP/docker-${cacheRole}-module-cache"`));
    assert(job.includes(`"$RUNNER_TEMP/docker-${cacheRole}-go-cache" "$RUNNER_TEMP/docker-${cacheRole}-module-cache" >> "$GITHUB_ENV"`));
    assert(job.indexOf('mkdir -m 700 -- "$RUNNER_TEMP/')<job.indexOf('uses: actions/setup-go@'));
    assert.match(job,/go mod download\n {10}go mod verify/u);
    assert.match(job,/bash scripts\/build-connected-rich-docker-linux\.sh "\$VERSION" "\$SOURCE" dist\/docker/u);
    assert.doesNotMatch(job,/download-artifact|actions\/cache|needs\.(?:build|independent)/u);
    const fixed=role==='build'?name:name+'-independent';
    assert(job.includes('      artifact_name: '+fixed+'\n'));
    assert(job.includes('          name: '+fixed+'\n'));
    assert.equal([...job.matchAll(/uses: actions\/upload-artifact@/gu)].length,1);
    assert.match(job,/artifact_id: \$\{\{ steps\.upload\.outputs\.artifact-id \}\}/u);
  }
  assert.match(jobs.compare,/needs: \[authorize, build, independent\]/u);
  assert(jobs.compare.indexOf(' compare-inputs ')<jobs.compare.indexOf('uses: actions/download-artifact@'));
  assert.match(jobs.compare,/PRIMARY_ID: \$\{\{ needs\.build\.outputs\.artifact_id \}\}/u);
  assert.match(jobs.compare,/SECONDARY_ID: \$\{\{ needs\.independent\.outputs\.artifact_id \}\}/u);
  assert.match(jobs.compare,/PRIMARY_NAME: \$\{\{ needs\.build\.outputs\.artifact_name \}\}/u);
  assert.match(jobs.compare,/SECONDARY_NAME: \$\{\{ needs\.independent\.outputs\.artifact_name \}\}/u);
  assert.deepEqual([...jobs.compare.matchAll(/artifact-ids: (.+)\n {10}path: (.+)/gu)].map(match=>match.slice(1)),[
    ['${{ needs.build.outputs.artifact_id }}','dist/primary'],
    ['${{ needs.independent.outputs.artifact_id }}','dist/independent']]);
  assert.match(jobs.compare,/ compare --version "\$VERSION" --primary dist\/primary --secondary dist\/independent/u);
  assert.match(jobs.attest,/needs: \[authorize, build, compare\]/u);
  assert.equal([...source.matchAll(/id-token: write/gu)].length,1);
  assert.equal([...source.matchAll(/attestations: write/gu)].length,1);
  assert.match(jobs.attest,/id-token: write\n {6}attestations: write/u);
  const attest=jobs.attest.split(/\n {6}- name: /u).filter(block=>block.includes('uses: actions/attest@'));
  assert.equal(attest.length,2);
  assert.match(attest[0],/subject-path: dist\/payload\/home-lab-observer-connected-rich-processes-docker_.*_linux_amd64.tar.gz\n {10}sbom-path: dist\/connected-docker.spdx.json/u);
  assert.match(attest[1],/subject-path: dist\/payload\/\*/u);
  assert.doesNotMatch(attest[1],/sbom-path:|subject-path: \||connected-docker.spdx.json/u);
  assert.match(jobs.attest,/verify-sbom --directory dist\/payload --sbom dist\/connected-docker.spdx.json/u);
  assert.match(jobs.attest,/ verify-current --version "\$VERSION" --directory dist\/payload/u);
  assert(jobs.attest.indexOf(' verify-current ')<jobs.attest.indexOf('uses: anchore/sbom-action@'));
  assert.doesNotMatch(jobs.attest,/ authorize --version/u,'fresh authorization must be correlated with payload metadata');
  assert.doesNotMatch(source,/contents: write|self-hosted|gh release|gh workflow run|connected_promote\.py/u);
  for (const action of source.matchAll(/uses: (\S+)/gu)) assert.match(action[1],/^[^@]+@[a-f0-9]{40}$/u);
  const roots=[...source.matchAll(/\.\/cmd\/observer-connected-rich-docker-(collector|reader|uploader|install)(?= |\n)/gu)].map(match=>match[1]);
  assert.deepEqual(roots,['collector','reader','uploader','install']);
}

test('fixed Docker candidate graph separates both fresh builders, comparison and archive-only SPDX',()=>closedGraph(workflow));

test('source wiring rejects both self-download and wrong name/output maps before attestation',()=>{
  for (const [from,to] of [
    ['SECONDARY_ID: ${{ needs.independent.outputs.artifact_id }}','SECONDARY_ID: ${{ needs.build.outputs.artifact_id }}'],
    ['artifact-ids: ${{ needs.independent.outputs.artifact_id }}','artifact-ids: ${{ needs.build.outputs.artifact_id }}'],
    ['SECONDARY_NAME: ${{ needs.independent.outputs.artifact_name }}','SECONDARY_NAME: ${{ needs.build.outputs.artifact_name }}'],
    ['artifact_name: '+name+'-independent','artifact_name: '+name],
    ['needs: [authorize, build, compare]','needs: [authorize, build]'],
    ['cache: false','cache: true'],
    ['    steps:\n      - uses: actions/checkout@','    env:\n      GOCACHE: ${{ runner.temp }}/unavailable-context\n    steps:\n      - uses: actions/checkout@'],
    [' verify-current --version "$VERSION" --directory dist/payload',' verify --directory dist/payload'],
    ['subject-path: dist/payload/home-lab-observer-connected-rich-processes-docker_','subject-path: dist/payload/* # '],
  ]) {
    assert(workflow.includes(from));
    assert.throws(()=>closedGraph(workflow.replace(from,to)));
  }
});

test('numeric workflow remains the distinct legacy contract, not a Docker alias',async()=>{
  const numeric=await readFile(new URL('../.github/workflows/connected-linux-candidate.yml',import.meta.url),'utf8');
  assert(numeric.includes('scripts/connected_candidate.py'));
  assert(!numeric.includes('connected_docker_candidate'));
  assert(!numeric.includes('rich-docker'));
});
