"""Fixed candidate CLI/comparison wiring, with no provider or candidate execution."""
import copy
from datetime import datetime,timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

import connected_candidate as numeric
import connected_docker_candidate as docker
import _connected_docker_archive as archive
from test_connected_candidate import fixture_metadata
from test_connected_docker_archive import fixture,pack,encoded


def payload():
    identity,trusted,contents,manifest=fixture()
    trusted.update({'bootstrap.sh.tmpl':b'{{.Version}}\n{{.Commit}}\n{{.Extraction}}\n{{.Coordinator}}',
                    'extract.py':b'# synthetic extractor not executed',
                    'coordinator.py':b'# @VERSION@ @ARCHIVE_SIZE@ @ARCHIVE_SHA256@ @MANIFEST_SHA256@ @ARCHIVE_BASENAME@ @INSTALL_BINARY@ @RELEASE_ROOT@'})
    tar=pack(contents,manifest); raw=encoded(manifest)
    helper=archive.expected_installer(raw,tar,identity,trusted)
    names=docker.asset_names(identity['version']); values=[tar,raw,helper]
    values.append(''.join(hashlib.sha256(data).hexdigest()+'  '+name+'\n' for name,data in zip(names,values)).encode('ascii'))
    metadata=fixture_metadata(); metadata.update(schema_version=docker.SCHEMA_VERSION,workflow_path=docker.WORKFLOW_PATH,
                                               version=identity['version'],contract_sha256=docker.DEFINITION)
    metadata['assets']=[{'name':name,'size':len(data),'sha256':hashlib.sha256(data).hexdigest()} for name,data in zip(names,values)]
    result=dict(zip(names,values)); result['candidate.json']=docker.encode_metadata(metadata)
    return metadata,result,trusted


class DockerCandidateTests(unittest.TestCase):
    def test_verify_current_binds_downloaded_candidate_to_fresh_attempt_before_downstream_work(self):
        metadata,_,trusted=payload()
        identity={key:metadata[key] for key in ('version','source','workflow_id','run_id','run_attempt')}
        for key,replacement in (('run_attempt',3),('run_id',98766),('source','b'*40),
                                ('workflow_id',12346),('version','0.1.0-canary.4')):
            fresh=dict(identity,**{key:replacement})
            with patch.object(docker,'authorize',return_value=fresh),patch.object(docker,'_metadata',return_value=metadata), \
                 patch.object(docker,'_payload_bytes') as downstream:
                with self.subTest(key=key),self.assertRaises(ValueError):
                    docker.verify_current('synthetic',identity['version'],{},lambda *_:None)
                downstream.assert_not_called()
        with patch.object(docker,'authorize',return_value=identity),patch.object(docker,'_metadata',return_value=metadata), \
             patch.object(docker,'load_trusted_inputs',return_value=trusted),patch.object(docker,'_payload_bytes') as downstream:
            self.assertEqual(docker.verify_current('synthetic',identity['version'],{}),metadata)
            downstream.assert_called_once_with('synthetic',metadata,trusted)

    def test_real_isolated_cli_from_hostile_unrelated_working_directory(self):
        scripts=Path(__file__).resolve().parent
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory); marker=root/'shadow-imported'
            (root/'_connected_candidate_core.py').write_text('raise RuntimeError("shadowed")',encoding='ascii')
            env=dict(os.environ,PYTHONPATH=directory)
            for script in ('connected_candidate.py','connected_docker_candidate.py'):
                for argv,expected in ((['--help'],0),(['authorize','--version','bad-input'],1)):
                    result=subprocess.run([sys.executable,'-I','-B',str(scripts/script),*argv],cwd=root,env=env,
                                          stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=10,check=False)
                    self.assertEqual(result.returncode,expected,result.stderr.decode())
                    self.assertNotIn(b'Traceback',result.stderr)
                    self.assertNotIn(b'shadowed',result.stderr)
            self.assertFalse(marker.exists())
            result=subprocess.run([sys.executable,'-I','-B',str(scripts/'connected_docker_candidate.py'),
                                   'verify','--directory',directory],cwd=root,env=env,
                                  stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=10,check=False)
            self.assertEqual((result.returncode,result.stderr),(1,b'CONNECTED_DOCKER_CANDIDATE_FAILED'+os.linesep.encode('ascii')))
            result=subprocess.run([sys.executable,'-I','-B',str(scripts/'connected_docker_candidate.py'),
                                   'verify-current','--version','bad-input','--directory',directory],cwd=root,env=env,
                                  stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=10,check=False)
            self.assertEqual((result.returncode,result.stderr),(1,b'CONNECTED_DOCKER_CANDIDATE_FAILED'+os.linesep.encode('ascii')))

    def test_complete_payload_helper_and_cross_profile_refusal(self):
        metadata,contents,trusted=payload()
        docker.validate_payload_bytes(contents,metadata,trusted)
        with self.assertRaises(ValueError): numeric.validate_payload_bytes(contents,metadata)
        changed=dict(contents); name=docker.asset_names(metadata['version'])[2]
        changed[name]+=b'\n# substituted helper'
        changed_metadata=copy.deepcopy(metadata)
        changed_metadata['assets'][2].update(size=len(changed[name]),sha256=hashlib.sha256(changed[name]).hexdigest())
        changed['SHA256SUMS']=''.join(e['sha256']+'  '+e['name']+'\n' for e in changed_metadata['assets'][:3]).encode('ascii')
        changed_metadata['assets'][3].update(size=len(changed['SHA256SUMS']),sha256=hashlib.sha256(changed['SHA256SUMS']).hexdigest())
        changed['candidate.json']=docker.encode_metadata(changed_metadata)
        with self.assertRaises(ValueError): docker.validate_payload_bytes(changed,changed_metadata,trusted)

    def test_fixed_source_reads_no_replace_shell_or_working_tree_resource_read(self):
        calls=[]
        def command(argv,limit):
            calls.append((argv,limit)); return b'synthetic'
        with patch.object(docker,'_bounded_command',side_effect=command):
            result=docker.load_trusted_inputs('a'*40)
        self.assertEqual(len(result),9)
        self.assertEqual({argv[-1] for argv,_ in calls},
                         {'a'*40+':'+path for path in dict(archive.RESOURCE_PATHS,**archive.BOOTSTRAP_PATHS).values()})
        self.assertTrue(all(argv[:3]==['git','--no-replace-objects','--no-pager'] for argv,_ in calls))
        with patch.object(docker,'_bounded_command') as command:
            with self.assertRaises(ValueError): docker.load_trusted_inputs('HEAD')
            command.assert_not_called()

    def test_command_output_is_bounded_and_nonzero_refuses(self):
        self.assertEqual(docker._bounded_command([sys.executable,'-I','-B','-c','print("ok")'],8),b'ok'+os.linesep.encode('ascii'))
        for code in ('print("x"*10000)','raise SystemExit(1)'):
            with self.assertRaises(ValueError): docker._bounded_command([sys.executable,'-I','-B','-c',code],64)

    def test_real_git_source_read_ignores_hostile_caller_git_environment(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory); (root/'scripts').mkdir()
            for name,path in dict(archive.RESOURCE_PATHS,**archive.BOOTSTRAP_PATHS).items():
                target=root/path; target.parent.mkdir(parents=True,exist_ok=True); target.write_bytes(b'trusted-'+name.encode('ascii'))
            def git(*args):
                return subprocess.check_output(['git','-C',str(root),*args],stderr=subprocess.DEVNULL)
            git('init','-q'); git('-c','core.autocrlf=false','add','.')
            git('-c','user.name=Synthetic Test','-c','user.email=synthetic@example.invalid','commit','-qm','fixture')
            source=git('rev-parse','HEAD').decode('ascii').strip()
            hostile={'GIT_DIR':str(root/'missing'),'GIT_WORK_TREE':str(root/'other'),'GIT_INDEX_FILE':str(root/'bad-index'),
                     'GIT_OBJECT_DIRECTORY':str(root/'bad-objects'),'GIT_CONFIG_COUNT':'1','GIT_CONFIG_KEY_0':'core.bare','GIT_CONFIG_VALUE_0':'true'}
            with patch.object(docker,'__file__',str(root/'scripts/connected_docker_candidate.py')),patch.dict(os.environ,hostile):
                result=docker.load_trusted_inputs(source)
            self.assertEqual(result,{name:b'trusted-'+name.encode('ascii') for name in dict(archive.RESOURCE_PATHS,**archive.BOOTSTRAP_PATHS)})

    def test_actual_syft_four_role_fixture_and_required_core_coverage(self):
        # Reduced only to the fields consumed by this guard; captured from pinned
        # Syft1.55's actual old four-ELF archive scan, never a release acceptance.
        fixture_path=Path(__file__).parent/'fixtures/connected-docker-syft-1.55.json'
        data=fixture_path.read_bytes(); sbom=json.loads(data)
        metadata=fixture_metadata(); metadata.update(schema_version=docker.SCHEMA_VERSION,workflow_path=docker.WORKFLOW_PATH,
                                                   version='0.1.0-canary.3',source='4eee2be1991c7c1cac46d374390bf25611cbe0b8',contract_sha256=docker.DEFINITION)
        for entry,name in zip(metadata['assets'],docker.asset_names(metadata['version'])): entry['name']=name
        metadata['assets'][0]['sha256']='3a3d2e08b3a98a60473b53d9319fdebd2b35b39224374ffe4f608131a2320367'
        manifest={'files':[{'name':entry['fileName'],'sha256':next(c['checksumValue'] for c in entry['checksums'] if c['algorithm']=='SHA256')}
                           for entry in sbom['files']]}
        required={'github.com/shirou/gopsutil/v4':'v4.26.9','golang.org/x/sys':'v0.48.0','modernc.org/sqlite':'v1.59.0'}
        docker.validate_sbom(data,metadata,encoded(manifest),required)
        mutations=[lambda x:x['files'].pop(),lambda x:next(c for c in x['files'][0]['checksums'] if c['algorithm']=='SHA256').update(checksumValue='0'*64),
                   lambda x:x.update(packages=[p for p in x['packages'] if p['name']!='modernc.org/sqlite']),
                   lambda x:x.update(packages=[p for p in x['packages'] if p['name']!='stdlib']),
                   lambda x:x.update(files=[])]
        for mutate in mutations:
            changed=copy.deepcopy(sbom); mutate(changed)
            with self.assertRaises(ValueError): docker.validate_sbom(encoded(changed),metadata,encoded(manifest),required)

    def test_provider_comparison_refuses_self_wrong_mapping_and_stale_metadata_before_download(self):
        identity={'version':'0.1.0-canary.3','source':'a'*40,'run_id':98765,'run_attempt':2}
        first='connected-linux-docker-0.1.0-canary.3-98765-2'; second=first+'-independent'
        def record(identifier):
            return {'id':identifier,'name':first if identifier==101 else second,'expired':False,'size_in_bytes':1234,
                    'digest':'sha256:'+'b'*64,'expires_at':'2030-01-01T00:00:00Z',
                    'workflow_run':{'id':98765,'repository_id':docker.REPOSITORY_ID,'head_repository_id':docker.REPOSITORY_ID,
                                    'head_branch':'main','head_sha':'a'*40}}
        calls=[]
        def get(endpoint): calls.append(endpoint); return record(int(endpoint.rsplit('/',1)[1]))
        now=datetime(2026,10,30,tzinfo=timezone.utc)
        docker.comparison_inputs(identity,101,102,first,second,get,now)
        self.assertEqual(calls,[f'/repos/{docker.REPOSITORY}/actions/artifacts/101',f'/repos/{docker.REPOSITORY}/actions/artifacts/102'])
        for ids,names in (((101,101),(first,second)),((101,102),(first,first)),((102,101),(first,second)),((0,102),(first,second))):
            with self.subTest(ids=ids,names=names),self.assertRaises(ValueError): docker.comparison_inputs(identity,*ids,*names,get,now)
        for mutate in (lambda r:r.update(expired=True),lambda r:r.update(expires_at='2020-01-01T00:00:00Z'),
                       lambda r:r.update(digest=None),lambda r:r['workflow_run'].update(head_sha='c'*40),
                       lambda r:r['workflow_run'].update(id=98764),lambda r:r.update(id=True)):
            def bad(endpoint):
                result=get(endpoint); mutate(result); return result
            with self.assertRaises(ValueError): docker.comparison_inputs(identity,101,102,first,second,bad,now)

    @unittest.skipUnless(hasattr(os,'O_DIRECTORY'),'Linux descriptor boundary')
    def test_independent_directories_byte_equality_and_changed_second_refuse(self):
        metadata,contents,trusted=payload()
        identity={key:metadata[key] for key in ('version','source','workflow_id','run_id','run_attempt')}
        with tempfile.TemporaryDirectory() as directory:
            roots=[Path(directory)/name for name in ('first','second')]
            for root in roots:
                root.mkdir(mode=0o700)
                for name,data in contents.items(): (root/name).write_bytes(data); (root/name).chmod(0o644)
            with patch.object(docker,'load_trusted_inputs',return_value=trusted):
                docker.compare_payloads(*roots,identity)
                with patch.object(docker,'_payload_bytes',side_effect=[{'synthetic':b'left'},{'synthetic':b'right'}]):
                    with self.assertRaises(ValueError): docker.compare_payloads(*roots,identity)
                with self.assertRaises(ValueError): docker.compare_payloads(roots[0],roots[0],identity)
                (roots[1]/'SHA256SUMS').write_bytes(b'bad')
                with self.assertRaises(ValueError): docker.compare_payloads(*roots,identity)


if __name__=='__main__': unittest.main()
