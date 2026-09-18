# Reproducible disposable first-install VM acceptance harness

Status: implementation in progress. The harness is manual, never a PR CI job or a live-host installer.

An operator supplies a checksum-verified official Ubuntu 24.04 amd64 cloud image, the exact reviewed
three-binary connected bundle and a dedicated empty work directory on a KVM host. The harness refuses
unless the operator supplies the independently reviewed source commit, manifest digest, archive digest and receiver
digest; it binds
those values to the sidecar, guest-extracted bundle, installed release and installed config. The host
resolves only root-owned tools from a fixed system PATH and treats unreadable process state as busy.
It also refuses
the wrong image digest, non-absolute paths, preexisting output, insufficient 2-vCPU/3-GiB/12-GiB
capacity, any other active QEMU guest, or missing cloud-init/QEMU/ext4 tools. It never reuses the
Platform CI base disk, mounts a host directory into a guest, enables an external NIC or connects to
Platform Demo. Every case uses a new copy-on-write overlay and seed. A read-only ext4 payload image
transfers only the reviewed bundle, synthetic receiver and test driver into the guest's own filesystem.

The guest verifies Ubuntu/systemd 255, no NIC/default route, bundle checksums, then binds a public
test address only on loopback and trusts a throwaway guest-only TLS certificate for the compiled
origin. A fixed synthetic one-use grant is entered through a pseudo-terminal after the installer's
no-echo prompt. The receiver accepts one exact enrollment exchange and no upload. No grant, credential,
HTTP request body or raw environment is printed to the serial evidence channel.
On an install failure, the PTY driver retains at most 8 KiB of output in memory
and emits only an allowlisted phase enum, normalized numeric exit status and
prompt-seen bit. Unknown or mixed output is `UNRECOGNIZED`; neither raw PTY
bytes nor the synthetic grant or credential are emitted.

The minimum matrix is: refused preflight without grant; successful stopped install and exact unit,
principal, ledger and filesystem assertions; normal reboot and stopped retry refusal; abrupt QEMU
power cut immediately after PREPARING is durable, fresh-boot retained-residue and retry refusal;
the driver must stop the installer and prove no state or release root exists before requesting the
cut, while the synthetic receiver is incapable of consuming a grant in this scenario. A late cut
is a failed case, never accepted evidence. Across reboot and retry, compare inode, size and byte
hash of installed metadata, credential and ledger rather than just their existence;
abrupt QEMU power cut after successful stopped install, fresh-boot stopped-state persistence.
Synthetic root sync-failure tests remain supplementary; the VM harness must not claim they simulate
real hardware cache loss. Isolation assertions for mounts, sockets, credentials and systemd policy
remain explicit evidence items, with failures a hard no-go.

The executable three-case harness is a prerequisite but not a claim of full acceptance. The larger
manual inventory additionally calls for hostile pre-publish targets, fault injection and runtime
worker isolation. Exact guest unit fragments, effective systemd properties, no-ACL ownership and
unknown-member checks narrow that gap; any unexecuted item must still be reported as unexecuted.

The host bounds every QEMU process by timeout and PID, holds an exclusive fixture lease, and prints
fixed redacted PASS/FAIL case identifiers. It preserves evidence on failure. On success, an explicit
cleanup operation may remove only the uniquely named, resolved fixture directory it created, after
QEMU exits and the path is verified under the admitted work root. Never recursively delete the image
root or CI directories. The harness does not activate observer services, install on the owner host,
or merge its parent PR. Independent review and operator sign-off remain mandatory.
