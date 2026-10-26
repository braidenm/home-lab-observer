# Public-helper disposable VM acceptance

Status: independently reviewed plan and fixture implementation;
actual VM execution remains open. No public release or owner-host installation
is authorized by these fixture changes alone.

## Scope

Extend the existing no-NIC Ubuntu 24.04/systemd 255/amd64 fixture with a manually
selected `--scope bootstrap`. Keep existing direct stopped/activation scopes
and `--scope all` (the original four direct cases) unchanged. Bootstrap is a
separate opt-in, not added to ordinary CI or the old combined scope.
Use one fresh overlay: pre-enrollment download refusals followed
by a successful stopped installation, then reboot/stopped-state assertions.
No production credentials, host shares, forwarded ports or network interface.
Keep the existing 2-vCPU/3-GiB budget and bounded, redacted assertions.

## Identity and trust

Bootstrap scope requires an additional independently supplied SHA-256 of the
exact generated `install-connected-linux.sh`, in addition to current source,
archive, manifest, receiver, probe and full harness pins. A checksum sidecar is
not the independent pin. Copy only the admitted root-owned 0644, 1..64-KiB
helper onto read-only fixture media. Extend the canonical harness list with the
new fixed `bootstrap_guest.py`; record its new digest explicitly.

The fixture creates a synthetic non-root owner with fixed UID/GID 22001 and a
0700 owned home. Its approved sudo rule in this disposable guest permits the
fixed `/bin/bash <owned-private-stage>/install-connected-linux.sh <synthetic-srv>`
handoff only; no test runner or host sudo configuration is changed. Run the
public app's secret-free download/hash/sudo command as that owner under a PTY:
fixed system PATH, LC_ALL=C, no proxy/auth/cookie/CA variables, umask 077,
fresh exclusive owner-owned 0700 staging and regular 0600 public script.
Use real `/usr/bin/curl -q` with HTTPS-only redirects, bounded size/time,
fixed version URL and independent SHA-256 check before `sudo -- /bin/bash`.

Immediately before elevation re-open/re-hash the fixed script and record its
device/inode/owner/mode/size; after execution require the same identity and hash.
The fixture performs no intervening mutation. This checks that the actual
executed test bytes are unchanged, not stronger same-UID compromise protection:
the installing owner and its same-UID processes remain trusted exactly as the
accepted public-bootstrap threat model says. Do not invent a different privileged
installer or claim that a path-based sudo handoff defeats a malicious owner.
Its original stdin must reach the Go no-echo enrollment prompt. No grant in
argv, environment, files or shell history; only the existing synthetic PTY input.
Do not patch the helper, replace curl, disable TLS or override root download pins.

## Synthetic download service

Only the explicitly selected bootstrap receiver modes additionally answer DNS
for `github.com` and `release-assets.githubusercontent.com`, using the existing
guest-local loopback alias. Existing receiver modes still refuse those names.
The throwaway guest CA has all three fixed SANs; it is installed only in this
disposable guest's normal CA store. No resolver or request is ever forwarded.

DNS admits only exact fixed QNAMEs, IN A/AAAA, with the existing authoritative
REFUSED/NODATA behavior for other names/types; no forwarding. TLS SNI and HTTP
Host must match the exact closed host/path pair, GET only, no query or
Authorization/Cookie/Proxy-Authorization headers. Do not accept a Host alias
merely because it reaches the same loopback address.

The receiver serves only the exact manifest-derived release archive/script
paths and fixed redirect targets, GET only, no query, no directory listing or
arbitrary file path. Archive bytes come from the independently admitted
read-only media. Fixed bounded request/redirect/enrollment counters persist
across fixture receiver restarts. Join the old listener/process and confirm its
ports closed before starting the next mode; never signal an unowned PID.
Restart only this guest fixture receiver between fixed modes:

1. `bootstrap-helper-corrupt`: change one downloaded helper byte. The unprivileged
   independent hash check must refuse before sudo/helper execution.
2. `bootstrap-corrupt`: follow HTTPS redirect and serve an equal-size archive
   with one changed byte. Require refusal before token prompt or local install.
3. `bootstrap-downgrade`: redirect the archive to HTTP. Require protocol refusal
   before token prompt or local install and exactly zero requests at port 80.
4. `bootstrap-loop`: exceed five redirects. Require bounded refusal before
   token prompt or local install.
5. `bootstrap-valid`: follow a fixed HTTPS cross-host redirect and serve the
   exact archive. Enter only the existing synthetic grant, require stopped
   install, exact installed assertions, no remaining bootstrap staging and no
   enabled/active workers. Reboot and rerun stopped-state assertions.

For refusal cases verify no PREPARING/config/state/release or dedicated account
was created, exactly zero enrollment requests occurred, and no bootstrap
child/process or root/user staging remains.
Do not infer safety from a nonzero exit alone. The terminal driver bounds output
and deadlines and emits only fixed case/result codes, never captured terminal
output, grants, credentials, HTTP bodies or observations.

## Automated versus installed evidence

Ordinary tests cover closed mode/path/host/DNS admission, byte corruption and
redirect shape, mandatory helper pin, identity decoding and redacted terminal
failure handling. They never invoke a root installer on the test runner. Hosted
PR CI runs only static/unit checks. Run the actual no-NIC VM manually against
the reviewed artifact and record exact pins and pass/fail/not-run results.
These checks do not substitute for remaining ADR 020/021 fault/lifecycle gates,
public release provenance/anonymous download checks or live owner/share tests.
