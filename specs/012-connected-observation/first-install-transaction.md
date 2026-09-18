# First stopped Linux connected installation transaction

Status: implementation in progress. This is not approval to activate, release or deploy a connected worker.

## Supported first canary

An explicitly elevated `observer-connected-install install` command accepts only a verified bundle directory,
canonical manifest SHA-256 and server ID. It performs read-only Ubuntu 24.04/amd64/systemd 255, endpoint, NSS,
bundle, exact target/principal/manager-unit absence and fixed endpoint checks before requesting a one-use grant from a no-echo terminal. The grant is
never accepted on argv, logged, echoed or put in the environment. The command rechecks all prerequisites while
holding the fixed installation lease and has no automatic privilege escalation or alternate origin/path/command.

The command creates a new isolated, stopped installation only. Its fixed sequence is: publish PREPARING in the
new config root; exclusively create state and immutable release roots; provision and verify the two dedicated
non-login principals and shared-read group; copy and reverify exact bundle members; build the reviewed uploader
root and endpoint policy; perform a single scoped enrollment with the one-use grant; validate the enrollment and
pristine ledger under the uploader principal; freeze staging and promote the same ledger inode and bound credential;
publish exact disabled unit files and verify loaded policy plus inactive, disabled, no-drop-in unit state; publish installed metadata last. It must never start,
restart, enable, refresh, uninstall or adopt an existing resource. Success reports
`INSTALLED_PENDING_ACCEPTANCE`: this is a stopped artifact, not a fresh remote acknowledgement. The installed
metadata's internal `INSTALLED_READY` state records durable layout, not permission to activate it.

## Closed resource and process boundary

All destinations, names, ownership, modes, maximum sizes and execution tools are code-owned roles in a closed
layout. The digest may name one immutable release directory only after canonical SHA-256 validation. No generic
caller-selected file writer, path creator or subprocess API is exported or used by the command. Every new file is
exclusive, no-follow, verified by pinned descriptor, synced with its parent, and never replaced by default. A
collision, unknown entry or failed sync leaves evidence and reports recovery-required. The grant goes only through
the reviewed private enrollment pipe to the eventual uploader UID under the fixed transient manager profile. Before
grant handoff, the installer verifies the manager's actual transient UID/GID, root/bind mounts, capabilities,
syscall and address-family restrictions, IPC and empty credential arrays, then separately validates effective
IP/slice policy. It rechecks the owned invocation identity immediately before writing the pipe.

The initial command has no recovery/resume or cleanup verb. Errors before grant handoff distinguish local setup
incomplete; after an ambiguous grant handoff they require registration revocation and explicit owned-state
inspection before another enrollment. A retained PREPARING marker never authorizes startup. A later activation
PR must reject any setup residue and correlate actual worker invocation, policy and ledger evidence before start.

## Acceptance and release gate

Unit and synthetic-root tests inject failures at each durable setup/promotion boundary and prove no final config
or enabled/running unit appears. Refusal tests cover foreign principals, preexisting names, symlinks, hardlinks,
FIFO, ACL/mode drift, wrong bundle identity, output overflow, stale/ambiguous enrollment and ledger-inode changes.
A disposable Ubuntu 24.04/systemd 255 VM must run the exact packaged installer with a synthetic receiver and
reboot/interruption faults; it must prove dedicated principals, no inherited sockets, closed mounts and stopped
services. A passing helper fixture or command build is not this VM proof. No owner-host install or activation is
authorized by merging this PR alone; independent security review and the separate activation/packaging gates remain.

The accepted VM design is a new checksum-verified Ubuntu image with a dedicated 12 GiB overlay, 2 vCPU, 3 GiB
RAM and no external NIC. A guest-only TLS receiver, guest-only trust root and public-address loopback alias
exercise the compiled origin without contacting production. Transfer the exact bundle by read-only virtual media,
then copy it into the guest's own filesystem before verification. Run success/reboot and each interruption case on
fresh overlays; preserve only redacted assertions. This design is not evidence until it has actually run.
