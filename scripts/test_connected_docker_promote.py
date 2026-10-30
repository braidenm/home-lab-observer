"""Synthetic fixed publisher operations only; no credentials/network mutations."""
import copy
import datetime as dt
import inspect
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

import connected_promote as numeric
import connected_docker_promote as docker
import _connected_promote_core as core
from _connected_candidate_core import _NUMERIC,_DOCKER
import connected_docker_promotion_policy as policy
from test_connected_promote import FakeProvider as NumericFake, encoded
from test_connected_docker_promotion_policy import fixture, artifact_info, run_info, jobs_info, provenance, sbom_fixture
from _connected_docker_archive import RESOURCE_PATHS, BOOTSTRAP_PATHS


class FakeProvider(NumericFake):
    """Reuse the existing closed release simulator, not a second provider protocol."""
    def __init__(self, directory):
        super().__init__(directory)
        self.record,self.payload,self.packed,self.independent,self.trusted=fixture()
        self.attestations=[]
        self.downloads=[]
        self.independent_changed=False
        self.jobs=jobs_info(self.record)
        self.info_hook=lambda value: value

    def git_bytes(self,*args,**kwargs):
        if args[0]=='show':
            name=args[1].split(':',1)[1]
            paths=dict(RESOURCE_PATHS,**BOOTSTRAP_PATHS)
            if name=='go.mod':
                return b'github.com/shirou/gopsutil/v4 v4.26.9\ngolang.org/x/sys v0.48.0\nmodernc.org/sqlite v1.59.0\n'
            for key,path in paths.items():
                if name==path: return self.trusted[key]
            if name==policy.evidence_path(self.record): return encoded(self.record)
            raise AssertionError(name)
        if args[0]=='diff':
            return b':000000 100644 '+b'0'*40+b' '+b'e'*40+b' A\0'+policy.evidence_path(self.record).encode()+b'\0'
        return super().git_bytes(*args,**kwargs)

    def json(self,endpoint,method='GET',data=None,**kwargs):
        if method=='GET' and ('/actions/' in endpoint):
            self.calls.append((method,endpoint)); self.hook(method,endpoint)
            if '/actions/artifacts/' in endpoint:
                independent=endpoint.endswith('/'+str(self.record['artifacts']['independent']['id']))
                return self.info_hook(artifact_info(self.record,independent))
            if '/actions/workflows/' in endpoint:
                return {'id':policy.WORKFLOW_ID,'path':policy.WORKFLOW_PATH,'state':'active'}
            if '/attempts/' in endpoint: return copy.deepcopy(self.jobs)
            value=run_info(self.record)
            if self.rerun: value['run_attempt']+=1
            return value
        return super().json(endpoint,method,data,**kwargs)

    def binary(self,endpoint,limit):
        self.downloads.append(endpoint)
        if '/actions/artifacts/' in endpoint:
            if '/'+str(self.record['artifacts']['independent']['id'])+'/' in endpoint:
                return self.independent+(b'changed' if self.independent_changed else b'')
            return self.packed
        return super().binary(endpoint,limit)

    def attest(self,path,source,predicate_type):
        self.attestations.append((path.name,predicate_type))
        predicate=None
        if predicate_type==policy.SPDX_PREDICATE_TYPE:
            predicate,_=sbom_fixture(self.record,self.payload)
        return provenance(self.record['candidate'],path.name,policy.digest(path.read_bytes()),predicate_type,predicate)


class DockerOrchestrationTests(unittest.TestCase):
    def setUp(self):
        self.temporary=tempfile.TemporaryDirectory(prefix='hlo-docker-promotion-')
        self.addCleanup(self.temporary.cleanup)
        self.provider=FakeProvider(self.temporary.name)
        self.now=dt.datetime(2026,10,30,tzinfo=dt.timezone.utc)

    def publish(self):
        return docker.publish(self.provider,self.provider.record['candidate']['version'],policy.WORKFLOW_ID,self.now)

    def prepare(self):
        p=self.provider; p.head=p.record['candidate']['source']
        return docker.prepare(p,p.record['candidate']['run_id'],5678,5679,
                              p.root/'prepared',policy.WORKFLOW_ID,self.now)

    def test_complete_publication_reuses_exact_bytes_and_verifies_five_provenances_one_archive_spdx(self):
        self.assertEqual(self.publish(),900)
        p=self.provider
        self.assertEqual([x[0] for x in p.mutations],['POST','POST']+['UPLOAD']*4+['PATCH'])
        self.assertEqual(p.attestations,[(name,'https://slsa.dev/provenance/v1') for name in p.payload]+
                         [(p.record['candidate']['assets'][0]['name'],policy.SPDX_PREDICATE_TYPE)])
        self.assertFalse(p.release['draft']); self.assertTrue(p.release['immutable'])
        for method,_,data in p.mutations:
            if method in ('POST','PATCH') and 'make_latest' in data: self.assertEqual(data['make_latest'],'false')
        self.assertTrue(any('/attempts/2/jobs?per_page=100&page=1' in path for _,path in p.calls))
        self.assertEqual(self.publish(),900); self.assertEqual(len(p.mutations),7)

    def test_prepare_exports_only_two_builder_verified_nonexecutable_payload_without_requiring_receipt(self):
        record=self.prepare(); p=self.provider
        self.assertEqual(record['artifacts'],p.record['artifacts'])
        self.assertNotIn('vm',record)
        self.assertEqual(set(x.name for x in (p.root/'prepared'/'payload').iterdir()),set(p.payload))
        self.assertEqual(json.loads((p.root/'prepared'/'candidate-identity.json').read_bytes()),record)
        self.assertFalse(p.mutations)
        self.assertEqual(p.downloads[:2],[docker.PREFIX+'/actions/artifacts/5678/zip',docker.PREFIX+'/actions/artifacts/5679/zip'])
        with self.assertRaises(FileExistsError): self.prepare()

    def test_both_external_envelopes_checked_before_any_download_and_self_comparison_refused(self):
        p=self.provider
        for mutate in (lambda x:x.update(expired=True),lambda x:x.update(size_in_bytes=True),
                       lambda x:x.update(digest='sha256:'+'0'*63),
                       lambda x:x['workflow_run'].update(head_sha='b'*40)):
            p.info_hook=lambda value,mutate=mutate: (mutate(value) or value) if value['id']==5679 else value
            with self.subTest(mutate=mutate),self.assertRaises(ValueError): self.prepare()
            self.assertFalse(p.downloads); self.assertFalse(p.mutations)
        p.info_hook=lambda value:value
        with self.assertRaises(ValueError):
            docker.prepare(p,p.record['candidate']['run_id'],5678,5678,p.root/'bad',policy.WORKFLOW_ID,self.now)
        self.assertFalse(p.downloads)

    def test_stale_attempt_failed_or_incomplete_jobs_and_changed_independent_bytes_refuse_before_write(self):
        p=self.provider
        for mutate in (lambda: setattr(p,'rerun',True),lambda:p.jobs['jobs'][2].update(conclusion='skipped'),
                       lambda:p.jobs.update(total_count=6),lambda:setattr(p,'independent_changed',True)):
            with self.subTest(mutate=mutate):
                p.rerun=False; p.jobs=jobs_info(p.record); p.independent_changed=False; mutate()
                with self.assertRaises(ValueError): self.publish()
                self.assertFalse(p.mutations)

    def test_prepare_run_jobs_attempt_and_fixed_names_refuse_before_either_zip(self):
        p=self.provider
        for index,mutate in enumerate((lambda:setattr(p,'rerun',True),lambda:p.jobs['jobs'][2].update(conclusion='skipped'),
                       lambda:p.jobs.update(total_count=6),
                       lambda:setattr(p,'info_hook',lambda x:dict(x,name='foreign') if x['id']==5679 else x),
                       lambda:setattr(p,'info_hook',lambda x:dict(x,name=x['name'].replace('canary.3','canary.4')) if x['id']==5679 else x))):
            with self.subTest(mutate=mutate):
                p.rerun=False; p.jobs=jobs_info(p.record); p.info_hook=lambda x:x; mutate()
                p.downloads=[]; p.attestations=[]; p.head=p.record['candidate']['source']
                with self.assertRaises(ValueError):
                    docker.prepare(p,p.record['candidate']['run_id'],5678,5679,p.root/('bad-'+str(index)),policy.WORKFLOW_ID,self.now)
                self.assertFalse(p.downloads); self.assertFalse(p.attestations)
        self.assertFalse(p.mutations)

    def test_signed_spdx_must_match_archive_and_its_actual_predicate_not_other_result(self):
        p=self.provider; original=p.attest
        def attest(path,source,predicate):
            result=original(path,source,predicate)
            if predicate==policy.SPDX_PREDICATE_TYPE:
                result[0]['verificationResult']['statement']['predicate']={'spdxVersion':'SPDX-2.3'}
            return result
        p.attest=attest
        with self.assertRaises(ValueError): self.publish()
        self.assertFalse(p.mutations)

    def test_each_mutation_rechecks_independent_artifact_and_jobs_not_only_primary(self):
        p=self.provider; original=p.upload
        def upload(*args):
            value=original(*args)
            p.info_hook=lambda item:dict(item,digest='sha256:'+'0'*64) if item['id']==5679 else item
            return value
        p.upload=upload
        with self.assertRaises(ValueError): self.publish()
        self.assertTrue(p.release['draft'])
        self.assertFalse(any(x[0]=='PATCH' for x in p.mutations))

    def test_main_advance_after_download_refuses_and_missing_module_inventory_cannot_sign(self):
        p=self.provider; original=p.binary
        def binary(endpoint,limit):
            value=original(endpoint,limit); p.head='e'*40; return value
        p.binary=binary
        with self.assertRaises(ValueError): self.publish()
        self.assertFalse(p.mutations)
        p.binary=original; p.head='b'*40
        original_git=p.git_bytes
        p.git_bytes=lambda *args,**kw:b'' if args[0]=='show' and args[1].endswith(':go.mod') else original_git(*args,**kw)
        with self.assertRaises(ValueError): self.publish()
        self.assertFalse(p.mutations)

    def test_code_owned_workflow_cannot_be_overridden_before_provider_access(self):
        p=self.provider
        with self.assertRaises(ValueError):
            docker.prepare(p,p.record['candidate']['run_id'],5678,5679,p.root/'wrong',397507617,self.now)
        with self.assertRaises(ValueError): docker.publish(p,'0.1.0-canary.3',397507617,self.now)
        self.assertFalse(p.calls); self.assertFalse(p.downloads)

    def test_trusted_resource_reads_are_fixed_source_not_worktree_or_caller_paths(self):
        p=self.provider; calls=[]; original=p.git_bytes
        def git(*args,**kw): calls.append((args,kw)); return original(*args,**kw)
        p.git_bytes=git
        self.publish()
        source=p.record['candidate']['source']
        actual={args[1]:kw['limit'] for args,kw in calls if args[0]=='show' and args[1].startswith(source+':')}
        expected={source+':'+path:16384 for path in RESOURCE_PATHS.values()}
        expected.update({source+':'+path:65536 for path in BOOTSTRAP_PATHS.values()})
        expected[source+':go.mod']=65536
        self.assertEqual(actual,expected)

    def test_rerun_between_draft_byte_downloads_and_publish_blocks_immutable_publish(self):
        p=self.provider; original=p.binary
        def binary(endpoint,limit):
            value=original(endpoint,limit)
            if '/releases/assets/' in endpoint: p.jobs['jobs'][3]['conclusion']='failure'
            return value
        p.binary=binary
        with self.assertRaises(ValueError): self.publish()
        self.assertTrue(p.release['draft']); self.assertFalse(any(x[0]=='PATCH' for x in p.mutations))

    def test_stale_main_changed_draft_identity_and_anonymous_bytes_are_not_success(self):
        p=self.provider; original=p.binary
        def binary(endpoint,limit):
            value=original(endpoint,limit)
            if '/releases/assets/' in endpoint: p.release['assets'][0]['id']+=100
            return value
        p.binary=binary
        with self.assertRaises(ValueError): self.publish()
        self.assertFalse(any(x[0]=='PATCH' for x in p.mutations))
        p.binary=original; p.anon_changed=True
        with self.assertRaises(ValueError): self.publish()
        self.assertFalse(p.release['draft']); self.assertTrue(p.release['immutable'])
        self.assertFalse(any(x[0]=='DELETE' for x in p.mutations))

    def test_draft_telemetry_and_closed_same_candidate_races_converge_without_clobber(self):
        p=self.provider; p.race=True; original=p.binary
        def binary(endpoint,limit):
            value=original(endpoint,limit)
            if '/releases/assets/' in endpoint:
                p.release['download_count']=1
                for asset in p.release['assets']: asset['download_count']=asset.get('download_count',0)+1
            return value
        p.binary=binary
        self.assertEqual(self.publish(),900)
        self.assertEqual(sum(x[0]=='UPLOAD' for x in p.mutations),4)

    def test_native_cli_fixed_arguments_and_sanitized_provider_failure(self):
        p=self.provider
        with patch.object(docker,'Provider',return_value=p):
            with patch('sys.stdout',new_callable=io.StringIO) as output:
                p.head=p.record['candidate']['source']
                self.assertEqual(docker.main(['--prepare','--run-id',str(p.record['candidate']['run_id']),
                    '--artifact-id','5678','--independent-artifact-id','5679','--output',str(p.root/'cli')]),0)
            self.assertIn('CONNECTED_CANDIDATE_VERIFIED',output.getvalue())
        with patch.object(docker,'Provider',side_effect=RuntimeError('synthetic-private-value')):
            with patch('sys.stderr',new_callable=io.StringIO) as error:
                self.assertEqual(docker.main(['--publish','--version','0.1.0-canary.3']),1)
            self.assertEqual(error.getvalue(),'CONNECTED_PROMOTION_FAILED\n')

    def test_numeric_main_calls_public_operation_and_zip_fault_is_reached_not_masked(self):
        with patch.object(numeric,'Provider',return_value=object()),patch.object(numeric,'prepare',side_effect=ValueError('private-canary')) as prepare:
            with patch('sys.stderr',new_callable=io.StringIO) as error:
                self.assertEqual(numeric.main(['--prepare','--run-id','1','--artifact-id','2','--output','unused']),1)
            prepare.assert_called_once()
            self.assertEqual(error.getvalue(),'CONNECTED_PROMOTION_FAILED\n')
        record,_,_=__import__('test_connected_promote').fixture()
        with patch.object(numeric,'Provider',return_value=object()),patch.object(numeric,'prepare',return_value=record) as prepare:
            with patch('sys.stdout',new_callable=io.StringIO) as output:
                self.assertEqual(numeric.main(['--prepare','--run-id','1','--artifact-id','2','--output','unused']),0)
            prepare.assert_called_once()
            self.assertIn('CONNECTED_CANDIDATE_VERIFIED',output.getvalue())


class OperatorBoundaryTests(unittest.TestCase):
    def test_numeric_public_signatures_and_global_version_lock_are_preserved(self):
        self.assertEqual(str(inspect.signature(numeric.prepare)),'(provider, run_id, artifact_id, output, workflow_id, now)')
        self.assertEqual(str(inspect.signature(numeric.verify_candidate)),'(provider, record, packed, workflow_id, directory, now)')
        self.assertEqual(str(inspect.signature(numeric.publish)),'(provider, version, workflow_id, now)')
        self.assertEqual(tuple(inspect.signature(numeric.Provider).parameters),('root','run','source_environment'))
        self.assertEqual((numeric.WORKFLOW_ID,docker.WORKFLOW_ID),(397507617,401377430))
        with tempfile.TemporaryDirectory() as directory:
            provider=type('Provider',(),{'git_bytes':lambda self,*a: str(Path(directory)).encode()})()
            with numeric.version_lock(provider,'0.1.0-canary.3'):
                with self.assertRaises(FileExistsError):
                    with docker.version_lock(provider,'0.1.0-canary.3'): pass

    def test_only_actual_fixed_presets_bind_provider_or_policy(self):
        import dataclasses
        core._bindings(_NUMERIC); core._bindings(_DOCKER)
        for value in (None,{},'docker',dataclasses.replace(_DOCKER)):
            with self.assertRaises(ValueError): core._bindings(value)

    def test_fixed_provider_attestation_uses_exact_profile_certificate_uri(self):
        for facade,path in ((numeric,'connected-linux-candidate.yml'),(docker,'connected-linux-docker-candidate.yml')):
            calls=[]
            def run(argv,cwd,env,**kwargs): calls.append((argv,env,kwargs)); return 0,b'[]'
            with patch.object(core.shutil,'which',side_effect=lambda name,**_:str(Path('/trusted')/name)):
                provider=facade.Provider(run=run,source_environment={'PATH':'fixed','GH_TOKEN':'secret','GITHUB_TOKEN':'secret','GIT_DIR':'hostile'})
            provider.attest(Path('public.tar.gz'),'a'*40,'https://slsa.dev/provenance/v1')
            argv,env,bounds=calls[0]
            self.assertEqual(argv[argv.index('--cert-identity')+1],f'https://github.com/braidenm/home-lab-observer/.github/workflows/{path}@refs/heads/main')
            self.assertNotIn('--signer-workflow',argv)
            self.assertFalse({'GH_TOKEN','GITHUB_TOKEN','GIT_DIR'}&set(env))
            self.assertEqual(bounds,{'limit':1048576,'timeout':120})

    def test_isolated_cli_from_unrelated_hostile_cwd_uses_only_fixed_sibling_code(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory)
            # A fake ambient helper must never be imported even without cwd isolation.
            (root/'_connected_promote_core.py').write_text("raise RuntimeError('ambient-private-canary')\n")
            for name in ('connected_promote.py','connected_docker_promote.py'):
                script=Path(__file__).parent/name
                result=subprocess.run([sys.executable,'-I','-B',str(script),'--help'],cwd=root,
                    env=dict(os.environ,PYTHONPATH=directory),capture_output=True,timeout=10)
                self.assertEqual(result.returncode,0)
                self.assertIn(b'--prepare',result.stdout)
                self.assertNotIn(b'ambient-private-canary',result.stdout+result.stderr)
                expected=name=='connected_docker_promote.py'
                self.assertEqual(b'--independent-artifact-id' in result.stdout,expected)
                refusal=subprocess.run([sys.executable,'-I','-B',str(script),'--profile','untrusted'],cwd=root,
                    capture_output=True,timeout=10)
                self.assertEqual(refusal.returncode,2)

    def test_windows_native_stored_auth_provider_argv_never_extracts_or_copies_credentials(self):
        calls=[]
        def run(argv,cwd,env,**bounds):
            calls.append((argv,env,bounds)); return 0,b'HTTP/2 200 OK\r\n\r\n{"total_count":5,"jobs":[]}'
        with patch.object(core.shutil,'which',side_effect=lambda name,**_:str(Path(tempfile.gettempdir())/(name+'.exe'))):
            provider=docker.Provider(run=run,source_environment={'PATH':'fixed','GH_CONFIG_DIR':'hostile','GH_DEBUG':'api',
                'GIT_CONFIG_COUNT':'1','GIT_CONFIG_KEY_0':'url.x.insteadOf','GIT_CONFIG_VALUE_0':'secret'})
        endpoint=docker.PREFIX+'/actions/runs/98765/attempts/2/jobs?per_page=100&page=1'
        provider.json(endpoint)
        argv,environment,bounds=calls[0]
        self.assertEqual(argv[1:6],['api','--hostname','github.com','--method','GET'])
        self.assertEqual(argv[-1],endpoint)
        self.assertEqual(bounds,{'limit':1048576,'timeout':120})
        self.assertFalse(any(x in argv for x in ('auth','token','--with-token')))
        self.assertFalse({'GH_CONFIG_DIR','GH_DEBUG','GIT_CONFIG_COUNT','GIT_CONFIG_KEY_0','GIT_CONFIG_VALUE_0'}&set(environment))


if __name__=='__main__': unittest.main()
