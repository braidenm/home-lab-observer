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
origin. The guest must not map that hostname in `/etc/hosts`: Go can surface its IPv4 entry as an
IPv4-mapped address that the production public-address policy correctly rejects. Instead, the
already pinned synthetic receiver serves a bounded, non-recursive UDP DNS response on guest
`127.0.0.1:53`. Only the exact compiled hostname's A query returns the fixed loopback alias;
AAAA returns NODATA, and all other names/types are refused. The guest's regular
`/etc/resolv.conf` names only `127.0.0.1` with fixed timeouts and attempts. Any malformed or
oversized DNS packet is dropped, and the responder never forwards or logs a query. No other
network interface or default route is added. A fixed synthetic one-use grant is entered through a pseudo-terminal after the installer's
no-echo prompt. The receiver accepts one exact enrollment exchange and no upload. No grant, credential,
HTTP request body or raw environment is printed to the serial evidence channel.
On an install failure, the PTY driver retains at most 8 KiB of output in memory
and emits only an allowlisted phase enum, normalized numeric exit status and
prompt-seen bit. Unknown or mixed output is `UNRECOGNIZED`; neither raw PTY
bytes nor the synthetic grant or credential are emitted.
Before the real-grant PTY path, a guest-only, read-only classifier checks the
fixed host/NSS shape, absence of owned targets, synthetic DNS/TLS reachability,
and parent-slice policy. It emits one fixed failure category, never command
output, file contents or a grant. This classifier is diagnostic rather than
installer authority: a pass does not replace `CheckRequest` or establish VM
acceptance; the installed command still owns admission. Unknown and oversized
manager output is a fixed refusal. A preflight refusal in the installer after
classifier pass remains a hard no-go pending further diagnosis.
For that case only, the separately SHA-pinned, fixture-only Linux test binary
from the exact source commit runs the connected install package's actual
read-only bundle, host, target, DNS/TLS, parent and aggregate `CheckRequest`
functions before the grant prompt. It has fixed guest paths and a single
opt-in test name, binds its embedded source commit to the verified bundle
manifest, emits only code-owned stage labels and never joins the
release archive. This closes diagnostic parity without relaxing or instrumenting
the production installer's refusal boundary. A probe pass is still not
installation acceptance.
For a production `Resolve` refusal after the bundle/host/target checks pass,
the same fixture-only probe separates bounded root CA validation, Go absolute
hostname resolution, the exact public-address predicate, Go TLS and final
`Resolve` parity into fixed diagnostic categories. No raw DNS answer, TLS error,
certificate, HTTP body or host property enters serial evidence.
The guest DNS diagnostic further distinguishes an empty/oversized answer, an
IPv4-mapped result, a nonpublic result, an unexpected public result, and an
invalid fixed alias using only allowlisted categories. It requires the guest's
synthetic loopback alias before dialing, but does not weaken or replace the
production resolver or installer admission decision.

The minimum matrix is: refused preflight without grant; successful stopped install and exact unit,
principal, ledger and filesystem assertions; normal reboot and stopped retry refusal; abrupt QEMU
power cut immediately after PREPARING is durable, fresh-boot retained-residue and retry refusal;
the driver must stop the installer and prove no state or release root exists before requesting the
cut, while the synthetic receiver is incapable of consuming a grant in this scenario. A late cut
is a failed case, never accepted evidence. The promoted enrollment directory is
root-owned 0700 and retains exactly the four uploader-owned 0600 one-use records
`.enrollment-lock`, `attempt.json`, `credential.json`, and `ready.json` after the
ledger moves to its sibling. Prove their bounded canonical record shape and
installed-connector binding without printing secret bytes. Every directory
membership refusal emits only a fixed stage label, not a path or member name.
On Ubuntu 24.04/systemd 255, starting the stopped uploader unit transiently
creates empty mount-point scaffolding under its `RootDirectory=` (`root`, `usr`,
`var`, `proc`, `sys`, `dev/mqueue`, and `run/systemd/incoming`). The guest must
verify the exact observed members, root ownership, modes and empty leaves;
these are not installer payload or host bind mounts. See the
[systemd execution environment](https://www.freedesktop.org/software/systemd/man/systemd.exec.html)
for `RootDirectory=` and private mount namespace behavior.
Across reboot and retry, compare inode, size and byte hash of installed
metadata, credential, enrollment records and ledger rather than just their existence;
abrupt QEMU power cut after successful stopped install, fresh-boot stopped-state persistence.
Synthetic root sync-failure tests remain supplementary; the VM harness must not claim they simulate
real hardware cache loss. Isolation assertions for mounts, sockets, credentials and systemd policy
remain explicit evidence items, with failures a hard no-go.

The executable three-case harness is a prerequisite but not a claim of full acceptance. The larger
manual inventory additionally calls for hostile pre-publish targets, fault injection and runtime
worker isolation. Exact guest unit fragments, effective systemd properties, no-ACL ownership and
unknown-member checks narrow that gap; any unexecuted item must still be reported as unexecuted.
On systemd 255, `systemctl show` does not render `LoadCredential` as text (it
reports `[unprintable]` even for an empty value), and normalizes
`IPAddressDeny=any` to both IPv4 and IPv6 default-route ranges. The guest
therefore proves credential policy from exact unit bytes, the effective
`FragmentPath`, an empty `DropInPaths`, and `NeedDaemonReload=no`, then checks printable effective
network and namespace properties separately. It must not treat `[unprintable]`
as evidence that a credential is absent.

The host bounds every QEMU process by timeout and PID, holds an exclusive fixture lease, and prints
fixed redacted PASS/FAIL case identifiers. It preserves evidence on failure. On success, an explicit
cleanup operation may remove only the uniquely named, resolved fixture directory it created, after
QEMU exits and the path is verified under the admitted work root. Never recursively delete the image
root or CI directories. The harness does not activate observer services, install on the owner host,
or merge its parent PR. Independent review and operator sign-off remain mandatory.
