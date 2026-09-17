# F1 fixed endpoint policy evidence

The installed uploader and enrollment invocation use only the compiled Platform
hostname. Root preflight resolves that absolute hostname, rejects every special
or private DNS result and more than eight results, then attempts bounded TLS
handshakes with the exact hostname and fixed root-owned Ubuntu CA bundle. It
sends no HTTP request or credential. Only successfully authenticated reachable
addresses enter the sorted hosts/filter generation. An unreachable IPv6 address
therefore does not invalidate a proven IPv4 route. At least one verified address
is required. SSL environment variables and HTTP proxies are not authority.

Resolution is bounded to thirty seconds overall, five seconds for DNS and three
seconds per TLS attempt. This is a point-in-time endpoint check, not availability
monitoring or automatic DNS refresh; explicit root refresh remains required.

Before enrollment/uploader start, `ValidateEffective` reads the manager's exact
loaded unit properties and follows its actual Slice chain to `-.slice`. The owned
unit must have the exact configured address allows and both-family deny-all,
no drop-ins, no pending daemon reload and the expected system slice. Every
ancestor allow must be a subset of those exact endpoint addresses. Broader,
unrelated, symbolic, missing, malformed, duplicate, cyclic and oversized input
fails closed. `ValidateParent` performs the ancestor check before units exist.
No global slice or manager setting is changed by these functions. Collector
isolation uses its separately tested private network/socket-denial profile.

Manager commands use a fixed executable/arguments and clean environment, bounded
output and deadlines; their raw output/error is never diagnostic text. This
validates policy meaning, not kernel enforcement or all unit sandbox settings.
The installer still owns exact unit verification and the real installed-profile
acceptance gate. It must recheck on start/refresh; this cannot stop a privileged
administrator from changing the host policy after validation.

## Evidence (2026-09-11)

- An owned transient service on WSL Ubuntu/systemd 255.4 returned space-separated
  canonical `IPAddressAllow` CIDRs and `IPAddressDeny=::/0 0.0.0.0/0`, with
  `Slice=system.slice` and the expected loaded/no-reload properties. Read-only
  ancestor queries confirmed `system.slice -> -.slice -> empty` on this host.
  The transient service was collected; no persistent units/accounts were added.
- Windows pure policy tests and vet passed. Static Linux tests ran five times
  under WSL, covering exact inherited allows, dangerous inherited allows,
  absent IPv6 deny, unit drift, output bounds through `io.Copy`, cycles and
  authenticated reachable subsets with a deterministic verifier.
- Actual TLS/CA failure injection, live hostile ancestor policy, final packaged
  startup, BPF enforcement, reboot and server canary remain separate acceptance
  gates. These tests do not authorize or claim an installed production profile.

The systemd v255 primary-source allow-precedence and unsupported-BPF caveats are
recorded in [the accepted composition plan](installed-composition.md).

## Offline enrollment validation gate (2026-09-17)

Both `validate-enrollment` and `validate-ledger` are held on empty stdin until
the installer verifies its transient invocation and `ValidateOffline` checks
loaded numeric identities, exact artifact/state binds, minimal root, private
network/IPC, empty capabilities and required syscall denials. The installer must
recheck invocation ownership before releasing input; this gate never starts a
service or sends a credential. Each mode permits only its own private directory.

`systemctl show` renders credential arrays as `[unprintable]` even when empty.
The gate therefore queries five typed D-Bus properties: LoadCredential,
LoadCredentialEncrypted, SetCredential, SetCredentialEncrypted and
ImportCredential. Only the exact empty-array sequence is accepted. A streaming
matcher retains only a byte position/failure flag, not credential content; raw
output and errors are never diagnostic text. Commands share a bounded deadline
and use fixed paths, arguments and a clean environment. The supported v255 types
are defined in [systemd's execution interface](https://github.com/systemd/systemd/blob/v255/src/core/dbus-execute.c)
and were confirmed against systemd 255.4. Windows policy tests and five Linux
test repetitions passed; independent review found no remaining slice blocker.

The checked-in four-mode primitive fixture also passed on the owner-authorized
Ubuntu 24.04 x86_64 server: baseline, denied IPC/socket operations, collector
socket denial and synthetic empty-credential ACL isolation. Temporary services
and files were removed; no application/container was restarted. These primitive
results are not final installed-profile, BPF endpoint, filesystem-coverage or
production activation approval. Private server connection details stay outside
this public repository.
