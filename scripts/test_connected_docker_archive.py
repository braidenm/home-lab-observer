"""Synthetic, never-executed ELFs exercise the fixed streaming archive boundary."""
import copy
import gzip
import hashlib
import json
import struct
import unittest

import _connected_candidate_core as core
import _connected_docker_archive as archive


def encoded(value):
    return json.dumps(value, separators=(',', ':'), ensure_ascii=True).encode('ascii')


def fixture():
    metadata = {'version': '0.1.0-canary.3', 'source': 'a' * 40,
                'contract_sha256': core._DOCKER.definition}
    trusted = {name: (name + '\n').encode('ascii') for name in archive.RESOURCE_PATHS}
    contents = dict(trusted)
    for role in core._DOCKER.roles:
        header = bytearray(512)
        header[:7] = b'\x7fELF\x02\x01\x01'
        struct.pack_into('<HHI', header, 16, 2, 62, 1)
        struct.pack_into('<Q', header, 32, 64)
        struct.pack_into('<HHH', header, 52, 64, 56, 1)
        struct.pack_into('<II', header, 64, 1, 5)
        identity = f"HLO-CONNECTED-IDENTITY-V2[{role}|{metadata['version']}|{metadata['source']}|linux|amd64|{core._DOCKER.definition}]END-HLO-CONNECTED-IDENTITY".encode('ascii')
        header[160:160 + len(identity)] = identity
        contents['observer-connected-' + role] = bytes(header)
    manifest = dict(zip(('schema','version','commit','os','arch','profile','contract_sha256','files'),
                       ('observer-connected-bundle/v2',metadata['version'],metadata['source'],'linux','amd64',
                        core._DOCKER.manifest_profile,core._DOCKER.definition,
                        [{'name': name,'sha256': hashlib.sha256(data).hexdigest(),'size': len(data),
                          'mode': 493 if name.startswith('observer-connected-') else 420}
                         for name,data in sorted(contents.items())])))
    return metadata, trusted, contents, manifest


def pack(contents, manifest, mutate_header=None, trailer=b''):
    all_files = dict(contents, **{'connected-manifest.json': encoded(manifest)})
    raw = bytearray()
    for name,data in sorted(all_files.items()):
        mode = 493 if name.startswith('observer-connected-') else 420
        header = archive.canonical_header(name, len(data), mode)
        raw.extend(mutate_header(name, header) if mutate_header else header)
        raw.extend(data)
        raw.extend(b'\0' * (-len(data) % 512))
    result=gzip.compress(bytes(raw) + b'\0' * 1024 + trailer, mtime=0, compresslevel=6)
    return result[:9]+b'\xff'+result[10:]


class DockerArchiveTests(unittest.TestCase):
    def setUp(self):
        self.metadata,self.trusted,self.contents,self.manifest = fixture()

    def verify(self, data=None, manifest=None, trusted=None):
        manifest = manifest or self.manifest
        archive.validate_archive(data or pack(self.contents,manifest),encoded(manifest),self.metadata,
                                 self.trusted if trusted is None else trusted)

    def test_exact_four_roles_six_resources_and_self_manifest(self):
        self.verify()
        self.assertEqual(archive.MAX_TAR_BYTES, 838982144)
        self.assertEqual(archive.MAX_ELF_BYTES, 200 << 20)

    def test_manifest_closed_order_names_modes_identity_and_bounds(self):
        mutations = [lambda m: m.update(extra=1), lambda m: m['files'].reverse(),
                     lambda m: m['files'].pop(), lambda m: m['files'].append(copy.deepcopy(m['files'][0])),
                     lambda m: m['files'][0].update(mode=493),
                     lambda m: m['files'][0].update(size=16385),
                     lambda m: m.update(profile=core._NUMERIC.manifest_profile),
                     lambda m: m.update(commit='b'*40), lambda m: m.update(contract_sha256='b'*64)]
        for mutate in mutations:
            with self.subTest(mutate=mutate):
                changed = copy.deepcopy(self.manifest); mutate(changed)
                with self.assertRaisesRegex(ValueError,'connected_candidate_invalid'): self.verify(manifest=changed)

    def test_header_type_path_padding_and_trailing_refuse(self):
        for offset,value in ((0,b'../'),(100,b'0000777'),(108,b'0000001'),(156,b'2'),(265,b'root')):
            def mutate(name, header):
                changed=bytearray(header); changed[offset:offset+len(value)]=value; return bytes(changed)
            with self.subTest(offset=offset), self.assertRaises(ValueError):
                self.verify(pack(self.contents,self.manifest,mutate))
        for suffix in (b'X', b'\0'*512):
            with self.assertRaises(ValueError): self.verify(pack(self.contents,self.manifest,trailer=suffix))
        valid=pack(self.contents,self.manifest)
        for invalid in (valid+b'X',valid+valid,valid[:-1]):
            with self.assertRaises(ValueError): self.verify(invalid)

    def test_tampered_resources_binary_hash_and_unique_identity(self):
        for name in (next(iter(self.trusted)), 'observer-connected-rich-docker-reader'):
            changed=dict(self.contents); changed[name]=changed[name]+b'X'
            with self.assertRaises(ValueError): self.verify(pack(changed,self.manifest))
        for suffix in (b'HLO-CONNECTED-IDENTITY-V2[bad', self.contents['observer-connected-rich-docker-reader'][160:]):
            changed=dict(self.contents); name='observer-connected-rich-docker-reader'; changed[name]+=suffix
            manifest=copy.deepcopy(self.manifest)
            entry=next(item for item in manifest['files'] if item['name']==name)
            entry.update(size=len(changed[name]),sha256=hashlib.sha256(changed[name]).hexdigest())
            with self.assertRaises(ValueError): self.verify(pack(changed,manifest),manifest)
        with self.assertRaises(ValueError): self.verify(trusted={})

    def test_dynamic_wrong_arch_and_mixed_role_refuse_even_with_rehashed_manifest(self):
        for offset,value in ((18,struct.pack('<H',183)),(64,struct.pack('<I',3)),(64,struct.pack('<I',2)),(4,b'\x01')):
            changed=dict(self.contents); name='observer-connected-rich-docker-reader'
            body=bytearray(changed[name]); body[offset:offset+len(value)]=value; changed[name]=bytes(body)
            manifest=copy.deepcopy(self.manifest); entry=next(e for e in manifest['files'] if e['name']==name)
            entry['sha256']=hashlib.sha256(changed[name]).hexdigest()
            with self.subTest(offset=offset,value=value),self.assertRaises(ValueError): self.verify(pack(changed,manifest),manifest)

    def test_decompressed_limit_applies_before_allocation_and_truncation_refuses(self):
        from unittest.mock import patch
        with patch.object(archive,'MAX_TAR_BYTES',1024),self.assertRaises(ValueError): self.verify()
        valid=pack(self.contents,self.manifest)
        for end in (1,10,len(valid)//2):
            with self.assertRaises(ValueError): self.verify(valid[:end])

    def positioned(self,positions,malformed=False):
        changed=dict(self.contents); name='observer-connected-rich-docker-reader'
        original=changed[name]; start=original.index(b'HLO-CONNECTED-IDENTITY-V')
        end=original.index(b']END-HLO-CONNECTED-IDENTITY')+len(b']END-HLO-CONNECTED-IDENTITY')
        identity=original[start:end]
        body=bytearray(max(3*archive.CHUNK+512,max(positions)+len(identity)))
        body[:160]=original[:160]
        for index,position in enumerate(positions):
            value=identity if not malformed or index==0 else identity.replace(b'|linux|',b'|wrong|')
            body[position:position+len(value)]=value
        changed[name]=bytes(body); manifest=copy.deepcopy(self.manifest)
        entry=next(item for item in manifest['files'] if item['name']==name)
        entry.update(size=len(body),sha256=hashlib.sha256(body).hexdigest())
        return pack(changed,manifest),manifest,len(body),len(identity)

    def test_identity_is_exact_at_every_stream_boundary_and_last_position(self):
        for position in (archive.CHUNK-600,archive.CHUNK-512,archive.CHUNK-511,archive.CHUNK-100,
                         archive.CHUNK-1,archive.CHUNK,archive.CHUNK+1):
            with self.subTest(position=position):
                data,manifest,_,_=self.positioned([position]); self.verify(data,manifest)
        _,_,size,length=self.positioned([160])
        data,manifest,_,_=self.positioned([size-length]); self.verify(data,manifest)
        for position in (archive.CHUNK-1,archive.CHUNK,archive.CHUNK+1):
            for malformed in (False,True):
                data,manifest,_,_=self.positioned([160,position],malformed)
                with self.subTest(position=position,malformed=malformed),self.assertRaises(ValueError): self.verify(data,manifest)

    def test_duplicate_identity_refuses_before_consuming_remaining_elf(self):
        from unittest.mock import patch
        data,manifest,size,_=self.positioned([160,archive.CHUNK+20])
        total=0; original=archive._Gzip.read
        def read(stream,count):
            nonlocal total
            total+=count
            return original(stream,count)
        with patch.object(archive._Gzip,'read',read),self.assertRaises(ValueError): self.verify(data,manifest)
        self.assertLess(total,size)


if __name__=='__main__': unittest.main()
