"""Read the one fixed Docker tar contract without extraction or asset execution."""
import base64
import hashlib
import json
import struct
import zlib

from _connected_candidate_core import _DOCKER, _json, _require, SHA256

MAX_ELF_BYTES = 200 << 20  # connectedidentity.MaxFileBytes; unchanged producer limit.
MAX_RESOURCE_BYTES = 16384
MAX_TAR_BYTES = 838982144  # Four ELFs + six resources + manifest + 11 headers + end blocks.
CHUNK = 32768
RESOURCE_PATHS = {
    name + suffix: 'internal/connectedunits/templates/' + name + suffix
    for name,suffix in (
        ('home-lab-observer-connected-rich-docker-collector.service','.tmpl'),
        ('home-lab-observer-connected-rich-docker-reader.service','.tmpl'),
        ('home-lab-observer-connected-rich-docker-uploader.service','.tmpl'),
        ('rich-docker-enrollment.properties','.tmpl'),
        ('rich-docker-installed-config.schema.json',''),
        ('rich-docker-collection-policy.schema.json',''))
}
BOOTSTRAP_PATHS = {name: 'internal/connectedbootstrap/' + name
                   for name in ('bootstrap.sh.tmpl','extract.py','coordinator.py')}


def _encode(value):
    return json.dumps(value, ensure_ascii=True, separators=(',',':')).encode('ascii')


def canonical_header(name, size, mode):
    """Go archive/tar USTAR header for these fixed, short, regular file names."""
    _require(type(name) is str and name.isascii() and 0 < len(name) < 100)
    _require(type(size) is int and 0 < size <= MAX_ELF_BYTES and mode in (0o644,0o755))
    header=bytearray(512)
    header[:len(name)]=name.encode('ascii')
    for offset,length,value in ((100,8,mode),(108,8,0),(116,8,0),(124,12,size),
                                (136,12,0),(329,8,0),(337,8,0)):
        header[offset:offset+length]=(f'{value:0{length-1}o}\0').encode('ascii')
    header[148:156]=b'        '
    header[156]=ord('0')
    header[257:265]=b'ustar\x0000'
    header[148:156]=(f'{sum(header):06o}\0 ').encode('ascii')
    return bytes(header)


class _Gzip:
    """At most one chunk of decompressed input is retained, including on bombs."""
    def __init__(self,data):
        _require(type(data) is bytes and data[:10]==bytes.fromhex('1f8b08000000000000ff'))
        self.data=data; self.offset=0; self.pending=b''; self.buffer=b''
        self.total=0; self.decoder=zlib.decompressobj(31)

    def read(self,count):
        _require(0 <= count <= CHUNK)
        while len(self.buffer)<count and not self.decoder.eof:
            if not self.pending:
                self.pending=self.data[self.offset:self.offset+CHUNK]
                self.offset+=len(self.pending)
            _require(bool(self.pending))
            block=self.decoder.decompress(self.pending, min(CHUNK,MAX_TAR_BYTES-self.total+1))
            self.pending=self.decoder.unconsumed_tail
            self.total+=len(block); _require(self.total<=MAX_TAR_BYTES)
            self.buffer+=block
            _require(len(self.buffer)<=2*CHUNK)
        result,self.buffer=self.buffer[:count],self.buffer[count:]
        _require(len(result)==count)
        return result

    def finish(self):
        _require(not self.buffer)
        # Consume only the gzip trailer, never an additional decompressed byte.
        while not self.decoder.eof:
            if not self.pending:
                self.pending=self.data[self.offset:self.offset+CHUNK]; self.offset+=len(self.pending)
            _require(bool(self.pending))
            _require(self.decoder.decompress(self.pending,1)==b'')
            self.pending=self.decoder.unconsumed_tail
        _require(not self.buffer and not self.pending and not self.decoder.unused_data and self.offset==len(self.data))


def _manifest(data,metadata,trusted):
    _require(type(data) is bytes and 0<len(data)<=MAX_RESOURCE_BYTES)
    _require(type(trusted) is dict and set(RESOURCE_PATHS).issubset(trusted))
    for name in RESOURCE_PATHS:
        _require(type(trusted[name]) is bytes and 0<len(trusted[name])<=MAX_RESOURCE_BYTES)
    result=_json(data)
    _require(type(result) is dict and tuple(result)==('schema','version','commit','os','arch','profile','contract_sha256','files'))
    _require(_encode(result)==data)
    for field,value in (('schema','observer-connected-bundle/v2'),('version',metadata['version']),
                        ('commit',metadata['source']),('os','linux'),('arch','amd64'),
                        ('profile',_DOCKER.manifest_profile),('contract_sha256',_DOCKER.definition)):
        _require(result[field]==value)
    roles={'observer-connected-'+role for role in _DOCKER.roles}
    names=sorted(roles|set(RESOURCE_PATHS))
    _require(type(result['files']) is list and len(result['files'])==10)
    for name,entry in zip(names,result['files']):
        _require(type(entry) is dict and tuple(entry)==('name','sha256','size','mode'))
        _require(entry['name']==name and type(entry['sha256']) is str and SHA256.fullmatch(entry['sha256']) is not None)
        _require(type(entry['size']) is int and 0<entry['size']<=(MAX_ELF_BYTES if name in roles else MAX_RESOURCE_BYTES))
        _require(type(entry['mode']) is int and entry['mode']==(0o755 if name in roles else 0o644))
        if name in RESOURCE_PATHS:
            _require(len(trusted[name])==entry['size'] and hashlib.sha256(trusted[name]).hexdigest()==entry['sha256'])
    return result


def _elf(header,size):
    _require(len(header)>=64 and header[:7]==b'\x7fELF\x02\x01\x01')
    kind,machine,version=struct.unpack_from('<HHI',header,16)
    phoff=struct.unpack_from('<Q',header,32)[0]
    ehsize,phsize,phcount=struct.unpack_from('<HHH',header,52)
    _require(kind in (2,3) and machine==62 and version==1 and ehsize==64 and phsize==56 and 0<phcount<=64)
    _require(64<=phoff and phoff+phsize*phcount<=min(len(header),size))
    for position in range(phoff,phoff+phsize*phcount,phsize):
        _require(struct.unpack_from('<I',header,position)[0] not in (2,3))  # DYNAMIC / INTERP


def validate_archive(data,manifest_data,metadata,trusted):
    """Verify bounded canonical bytes. Provenance/build/VM proof remain separate."""
    try:
        manifest=_manifest(manifest_data,metadata,trusted)
        entries=manifest['files']+[{'name':'connected-manifest.json','sha256':hashlib.sha256(manifest_data).hexdigest(),
                                  'size':len(manifest_data),'mode':0o644}]
        stream=_Gzip(data)
        for entry in sorted(entries,key=lambda item:item['name']):
            name,size=entry['name'],entry['size']
            _require(stream.read(512)==canonical_header(name,size,entry['mode']))
            digest=hashlib.sha256(); remaining=size; first=b''; tail=b''; identity_count=0
            role=name.removeprefix('observer-connected-') if entry['mode']==0o755 else None
            expected=(f"HLO-CONNECTED-IDENTITY-V2[{role}|{metadata['version']}|{metadata['source']}|linux|amd64|{_DOCKER.definition}]END-HLO-CONNECTED-IDENTITY").encode('ascii')
            marker=b'HLO-CONNECTED-IDENTITY-V'
            resource=bytearray() if role is None else None
            while remaining:
                chunk=stream.read(min(CHUNK,remaining)); remaining-=len(chunk); digest.update(chunk)
                if len(first)<CHUNK: first=(first+chunk)[:CHUNK]
                if resource is not None: resource.extend(chunk)
                if role:
                    scan=tail+chunk
                    # Delay the last 512 bytes so identities split at any chunk are checked once.
                    end=len(scan) if not remaining else max(0,len(scan)-512)
                    position=0
                    while True:
                        position=scan.find(marker,position)
                        if position<0 or position>=end: break
                        _require(identity_count==0 and scan[position:position+len(expected)]==expected)
                        identity_count+=1; position+=len(expected)
                    tail=scan[end:]
            _require(digest.hexdigest()==entry['sha256'])
            if role:
                _elf(first,size); _require(identity_count==1)
            else:
                _require(bytes(resource)==(manifest_data if name=='connected-manifest.json' else trusted[name]))
            _require(stream.read(-size%512)==b'\0'*(-size%512))
        _require(stream.read(1024)==b'\0'*1024); stream.finish()
        return manifest
    except (KeyError,TypeError,AttributeError,UnicodeError,struct.error,zlib.error,OverflowError) as error:
        raise ValueError('connected_candidate_invalid') from error


def expected_installer(manifest_data,archive_data,metadata,trusted):
    """Reproduce the fixed Go bootstrap template, never execute it."""
    manifest=_manifest(manifest_data,metadata,trusted)
    for name in BOOTSTRAP_PATHS:
        _require(name in trusted and type(trusted[name]) is bytes and 0<len(trusted[name])<=65536)
    entries=manifest['files']+[{'name':'connected-manifest.json','sha256':hashlib.sha256(manifest_data).hexdigest(),
                              'size':len(manifest_data),'mode':0o644}]
    members=[dict(entry,header=base64.b64encode(canonical_header(entry['name'],entry['size'],entry['mode'])).decode('ascii'))
             for entry in sorted(entries,key=lambda item:item['name'])]
    extraction=trusted['extract.py']+b'\nMEMBERS = json.loads(base64.b64decode("'+base64.b64encode(_encode(members))+b'", validate=True))\n'
    coordinator=trusted['coordinator.py']
    replacements={'VERSION':metadata['version'],'ARCHIVE_SIZE':str(len(archive_data)),
                  'ARCHIVE_SHA256':hashlib.sha256(archive_data).hexdigest(),
                  'MANIFEST_SHA256':hashlib.sha256(manifest_data).hexdigest(),
                  'ARCHIVE_BASENAME':f"{_DOCKER.archive_prefix}_{metadata['version']}_linux_amd64.tar.gz",
                  'INSTALL_BINARY':'observer-connected-rich-docker-install',
                  'RELEASE_ROOT':'/opt/home-lab-observer-connected-rich/releases'}
    for key,value in replacements.items(): coordinator=coordinator.replace(('@'+key+'@').encode('ascii'),value.encode('ascii'))
    result=trusted['bootstrap.sh.tmpl']
    for key,value in {'Version':metadata['version'].encode('ascii'),'Commit':metadata['source'].encode('ascii'),
                      'Extraction':extraction,'Coordinator':coordinator}.items():
        result=result.replace(('{{.'+key+'}}').encode('ascii'),value)
    _require(0<len(result)<=65536 and b'{{.' not in result)
    return result
