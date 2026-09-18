# Fixed Linux connected-worker unit templates

Status: implementation slice under Spec 012 and ADR 020. Rendering is an
unused library until packaged installation and live policy acceptance pass.

The collector, uploader and transient enrollment modes are separate fixed
systemd 255 profiles. Render only reviewed embedded templates and a closed
input structure: numeric dedicated UIDs/GIDs, an immutable artifact digest,
and at most eight canonical sorted public IP addresses. Reject malformed or
duplicate addresses, unknown modes and invalid principals. No caller supplies
a template, executable path, shell, URL, hostname or arbitrary unit property.

The collector is network-private and cannot import credentials or uploader
transport. The uploader has a fixed root directory, exact outbound IP rules,
one manager-loaded credential and no shell or automatic restart. Enrollment
and ledger-validation modes use separate fixed arguments; offline validation
must not inherit a credential, handoff mount or IP allowlist. Every profile
retains its explicit memory limit and disables generated memory-pressure
environment variables. Resource bytes are returned detached for later exact
bundle hashing, never fetched from the network.

Tests assert required restrictions, excluded authority, fixed transient
properties and resource shapes. Templates are not evidence that the host
actually enforces loaded rules. Disposable-VM tests must still prove actual
UID/mount/descriptor behavior, IP and TLS enforcement, systemd credentials,
ledger survival, and stopped-on-failure installation before release.
