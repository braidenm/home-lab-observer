#!/usr/bin/env python3
"""Fixed Docker candidate interface; no arbitrary profile or policy input."""
import argparse
import hashlib
import os
from pathlib import Path
import re
import subprocess
import sys
import threading
from datetime import datetime, timezone
sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
import _connected_candidate_core as _core
from _connected_candidate_core import (REPOSITORY, REPOSITORY_ID, OWNER_ID, MAX_PAYLOAD_BYTES,
    MAX_METADATA_BYTES, MAX_SAFE_INTEGER, INVALID, FIELDS, VERSION, SHA256, SHA1,
    _require, _integer, _pairs, _json, _read_at, _gh_json)

WORKFLOW_PATH = _core._DOCKER.workflow_path
SCHEMA_VERSION = _core._DOCKER.schema

DEFINITION = _core._DOCKER.definition

def asset_names(version):
    return _core.asset_names(_core._DOCKER, version)


def artifact_name(metadata):
    return _core.artifact_name(_core._DOCKER, metadata)


def encode_metadata(metadata):
    return _core.encode_metadata(_core._DOCKER, metadata)


def validate_metadata(metadata, expected_workflow_id=None):
    return _core.validate_metadata(_core._DOCKER, metadata, expected_workflow_id)


def decode_metadata(data, expected_workflow_id=None):
    return _core.decode_metadata(_core._DOCKER, data, expected_workflow_id)


def authorize(version, environment, get=_gh_json):
    return _core.authorize(_core._DOCKER, version, environment, get)


def validate_sbom(data, metadata, manifest_data, required_modules):
    return _core.validate_sbom(_core._DOCKER, data, metadata, manifest_data, required_modules)


def validate_payload_bytes(contents, metadata, trusted_inputs):
    return _core.validate_payload_bytes(_core._DOCKER, contents, metadata, trusted_inputs)


def validate_payload(directory, metadata, trusted_inputs):
    return _core.validate_payload(_core._DOCKER, directory, metadata, trusted_inputs)


def create_metadata(directory, version, source, workflow_id, run_id, run_attempt, trusted_inputs):
    return _core.create_metadata(_core._DOCKER, directory, version, source, workflow_id, run_id, run_attempt, DEFINITION, trusted_inputs)


def _bounded_command(argv, limit):
    """Fixed shell-free source reads, bounded while collecting (not after capture)."""
    environment = dict(os.environ)
    for name in tuple(environment):
        if name.startswith('GIT_'): environment.pop(name)
    environment.update(GIT_CONFIG_NOSYSTEM='1',GIT_CONFIG_GLOBAL=os.devnull,GIT_TERMINAL_PROMPT='0')
    process = subprocess.Popen(argv,env=environment,stdin=subprocess.DEVNULL,stdout=subprocess.PIPE,
                               stderr=subprocess.DEVNULL,shell=False)
    result=[]
    def read():
        try:
            result.append(process.stdout.read(limit+1))
            if len(result[0])>limit: process.kill()
        except OSError:
            result.append(b'')
    reader=threading.Thread(target=read,daemon=True); reader.start()
    try:
        _require(process.wait(timeout=30)==0)
        reader.join(timeout=2)
        _require(not reader.is_alive() and len(result)==1 and 0<len(result[0])<=limit)
        return result[0]
    except (OSError,subprocess.SubprocessError) as error:
        raise ValueError(INVALID) from error
    finally:
        if process.poll() is None: process.kill()
        process.wait(timeout=2); reader.join(timeout=2); process.stdout.close()


def _source_blob(source,path,limit):
    _require(type(source) is str and SHA1.fullmatch(source) is not None)
    return _bounded_command(['git','--no-replace-objects','--no-pager','-C',
                             str(Path(__file__).resolve().parent.parent),'show',source+':'+path],limit)


def load_trusted_inputs(source):
    from _connected_docker_archive import RESOURCE_PATHS, BOOTSTRAP_PATHS
    return {name:_source_blob(source,path,16384 if name in RESOURCE_PATHS else 65536)
            for name,path in dict(RESOURCE_PATHS,**BOOTSTRAP_PATHS).items()}


def required_modules(source):
    text=_source_blob(source,'go.mod',65536).decode('ascii')
    result={}
    for name in ('github.com/shirou/gopsutil/v4','golang.org/x/sys','modernc.org/sqlite'):
        matches=re.findall(r'^\s*'+re.escape(name)+r' (v[^\s]+)(?: // indirect)?$',text,re.MULTILINE)
        _require(len(matches)==1); result[name]=matches[0]
    return result


def comparison_inputs(identity, primary_id, secondary_id, primary_name, secondary_name, get=_gh_json, now=None):
    """Run before downloads; self-comparison refuses even when bytes would match."""
    _require(_integer(primary_id) and _integer(secondary_id) and primary_id!=secondary_id)
    expected=f"connected-linux-docker-{identity['version']}-{identity['run_id']}-{identity['run_attempt']}"
    _require(primary_name==expected and secondary_name==expected+'-independent' and primary_name!=secondary_name)
    now=now or datetime.now(timezone.utc)
    for identifier,name in ((primary_id,primary_name),(secondary_id,secondary_name)):
        record=get(f'/repos/{REPOSITORY}/actions/artifacts/{identifier}')
        _require(type(record) is dict and type(record.get('id')) is int and record['id']==identifier)
        _require(record.get('name')==name and record.get('expired') is False)
        _require(type(record.get('size_in_bytes')) is int and 0<record['size_in_bytes']<=MAX_PAYLOAD_BYTES+1048576)
        _require(type(record.get('digest')) is str and re.fullmatch(r'sha256:[a-f0-9]{64}',record['digest']) is not None)
        expires=record.get('expires_at'); _require(type(expires) is str and len(expires)<=32)
        _require(datetime.fromisoformat(expires.replace('Z','+00:00'))>now)
        run=record.get('workflow_run'); _require(type(run) is dict)
        for key,value in (('id',identity['run_id']),('repository_id',REPOSITORY_ID),('head_repository_id',REPOSITORY_ID),
                          ('head_branch','main'),('head_sha',identity['source'])):
            _require(type(run.get(key)) is type(value) and run[key]==value)


def compare_payloads(primary,secondary,identity):
    _require(Path(primary).resolve()!=Path(secondary).resolve())
    first=_metadata(primary)
    second=_metadata(secondary)
    _same_identity(first,identity)
    _require(first==second)
    trusted=load_trusted_inputs(first['source'])
    left=_payload_bytes(primary,first,trusted); right=_payload_bytes(secondary,second,trusted)
    _require(left==right)  # Actual equality, not only equal metadata/hash claims.


def _same_identity(metadata,identity):
    for key in ('version','source','workflow_id','run_id','run_attempt'):
        _require(metadata[key]==identity[key])


def verify_current(directory,version,environment,get=_gh_json):
    """A rerun may retain earlier job outputs, never an earlier attempt's subjects."""
    identity=authorize(version,environment,get)
    metadata=_metadata(directory)
    _same_identity(metadata,identity)
    _payload_bytes(directory,metadata,load_trusted_inputs(metadata['source']))
    return metadata


def _metadata(directory):
    descriptor=os.open(directory,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW|os.O_CLOEXEC)
    try: return decode_metadata(_read_at(descriptor,'candidate.json',MAX_METADATA_BYTES))
    finally: os.close(descriptor)


def _payload_bytes(directory,metadata,trusted):
    descriptor=os.open(directory,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW|os.O_CLOEXEC)
    try:
        names=asset_names(metadata['version'])+['candidate.json']
        _require(sorted(os.listdir(descriptor))==sorted(names))
        contents={'candidate.json':_read_at(descriptor,'candidate.json',MAX_METADATA_BYTES)}
        for item in metadata['assets']: contents[item['name']]=_read_at(descriptor,item['name'],item['size'])
        validate_payload_bytes(contents,metadata,trusted)
        _require(sorted(os.listdir(descriptor))==sorted(names))
        return contents
    finally: os.close(descriptor)


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    commands=parser.add_subparsers(dest='command',required=True)
    auth=commands.add_parser('authorize'); auth.add_argument('--version',required=True)
    create=commands.add_parser('create')
    for key in ('directory','version','source'): create.add_argument('--'+key,required=True)
    for key in ('workflow-id','run-id','run-attempt'): create.add_argument('--'+key,type=int,required=True)
    verify=commands.add_parser('verify'); verify.add_argument('--directory',required=True)
    current=commands.add_parser('verify-current'); current.add_argument('--version',required=True); current.add_argument('--directory',required=True)
    sbom=commands.add_parser('verify-sbom'); sbom.add_argument('--directory',required=True); sbom.add_argument('--sbom',required=True)
    check=commands.add_parser('compare-inputs')
    for key in ('version','primary-name','secondary-name'): check.add_argument('--'+key,required=True)
    for key in ('primary-id','secondary-id'): check.add_argument('--'+key,type=int,required=True)
    compare=commands.add_parser('compare')
    for key in ('version','primary','secondary'): compare.add_argument('--'+key,required=True)
    args=parser.parse_args()
    try:
        if args.command=='authorize':
            identity=authorize(args.version,os.environ)
            with open(os.environ['GITHUB_OUTPUT'],'a',encoding='ascii',newline='\n') as output:
                for key,value in identity.items(): output.write(f'{key}={value}\n')
        elif args.command=='create':
            create_metadata(args.directory,args.version,args.source,args.workflow_id,args.run_id,args.run_attempt,load_trusted_inputs(args.source))
        elif args.command=='compare-inputs':
            identity=authorize(args.version,os.environ)
            comparison_inputs(identity,args.primary_id,args.secondary_id,args.primary_name,args.secondary_name)
        elif args.command=='compare':
            compare_payloads(args.primary,args.secondary,authorize(args.version,os.environ))
        elif args.command=='verify-current':
            verify_current(args.directory,args.version,os.environ)
        else:
            metadata=_metadata(args.directory); trusted=load_trusted_inputs(metadata['source'])
            contents=_payload_bytes(args.directory,metadata,trusted)
            if args.command=='verify-sbom':
                path=Path(args.sbom)
                descriptor=os.open(path.parent,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW|os.O_CLOEXEC)
                try: data=_read_at(descriptor,path.name,16*1024*1024)
                finally: os.close(descriptor)
                validate_sbom(data,metadata,contents['connected-manifest.json'],required_modules(metadata['source']))
        print('CONNECTED_DOCKER_CANDIDATE_OK'); return 0
    except (ValueError,OSError,KeyError,TypeError,AttributeError,subprocess.SubprocessError):
        print('CONNECTED_DOCKER_CANDIDATE_FAILED',file=sys.stderr); return 1


if __name__=='__main__': sys.exit(main())
