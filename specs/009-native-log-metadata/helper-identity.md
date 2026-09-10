# Fixed helper executable identity

Status: Bounded resolver slice; does not launch a child or enable native sources. Transport and native binding are
separate. `resolveHelper(build, embeddedLinuxSHA256)` takes no executable/path/environment from the caller. It uses
`os.Executable`, verifies runtime OS/architecture and the private protocol's build grammar, then returns only a private
command specification. Errors are fixed `LOG_HELPER_UNAVAILABLE` or `LOG_HELPER_MISMATCH`, never OS/path text.

Linux resolves only adjacent `observer-journal-helper`, with no arguments. Its bounded SHA-256 must match the exact
64-lowercase-hex digest embedded in the main release. Main/helper must be regular, nonempty, single-link files no larger
than the existing 200-MiB release binary cap. Reject symlinks, non-executable or setuid/setgid files and file capabilities.
Walk every existing ancestor: owner is current effective UID or root, with no group/world write. A root-owned sticky
ancestor (such as /tmp) can grant creation because all existing child owners are separately trusted and sticky deletion
rules protect their entries. No other writable ancestor is accepted. Reject effective/real UID or GID mismatches.
Open the helper without following the final symlink, compare lstat/fstat identities, bound hashing, and recheck identity
and size afterward. No PATH lookup, shell, sidecar checksum, inherited loader/systemd environment or permission repair.

Windows resolves only the same executable with the fixed argument `__log-helper`. Verify a local absolute drive path
(no UNC/device path or alternate data stream), regular disk file, single link, bounded size, and no reparse points in
any component. Security descriptors and identity come from opened handles, not a separate named ACL lookup. Each
component's owner must be current user, SYSTEM, Administrators or the exact TrustedInstaller service SID. No generic
service-SID prefix is trusted. Null/unreadable or unsupported ACLs fail closed; ordinary read/execute grants are allowed.

Effective allow ACEs for anyone else must grant no file content/append writes, deletion, security takeover, attribute
or extended-attribute mutation. Directory ancestors additionally reject delete-child and security/attribute takeover;
creation-only directory grants are not replacement authority over already-existing trusted children. Inherit-only
ACEs do not apply to that object, and each actual descendant is independently checked. Deny ACEs do not rescue an
otherwise unsafe allow grant: this is a conservative structural policy, not a complete Windows access-check emulator.
Unknown effective ACE types and excessive ACE counts refuse execution rather than guessing conditional semantics.
OWNER RIGHTS grants apply only to the already-trusted object owner; CREATOR OWNER is not a blanket trusted principal.
Unknown access-mask bits also fail closed. Native positive full-path CI tests require a trusted fixture; local runs may
skip only that positive test when preexisting host ancestors genuinely grant replacement to other principals.
This accommodates ordinary drive-root creation grants without allowing replacement of the installed executable.
[file rights](https://learn.microsoft.com/en-us/windows/win32/fileio/file-access-rights-constants),
[descriptor retrieval](https://learn.microsoft.com/en-us/windows/win32/api/aclapi/nf-aclapi-getsecurityinfo),
[ACE inheritance](https://learn.microsoft.com/en-us/windows/win32/secauthz/ace-inheritance-rules)

The Linux environment is exactly LANG=C, LC_ALL=C and TZ=UTC. Windows adds only SystemRoot read from
`GetSystemWindowsDirectory`, not the inherited environment. No PATH, profile, temporary-directory, token, proxy,
loader or instrumentation variables are copied. The transport selects the verified executable directory as cwd.
Native Windows bindings must use system-DLL loading; the minimal environment is not a substitute for that rule.
[system Windows directory](https://learn.microsoft.com/en-us/windows/win32/api/sysinfoapi/nf-sysinfoapi-getsystemwindowsdirectoryw)

Verification handles close before the command specification is returned. Trusted owners/admins can change their own
installation, permissions or process state and are outside this boundary; this is not protection against a malicious
owner, hostile kernel/filesystem, or a proof of the already-running binary's publisher signature. Bound bytes and path
depth do not impose a hard deadline on stalled filesystem I/O. Unsupported mounts/ACL layouts, reparse-based installs,
hardlinked files, missing permissions and mismatched release pairs conservatively disable only native logs. Remediation
is an owner-reviewed reinstall into a local trusted directory; the resolver never chmods, edits ACLs, elevates or repairs.

Tests use synthetic ACL descriptors and temporary files only: identities, unsafe allow/owner rules, root creation-only
grants, inherit-only semantics, link/reparse refusal, size/hash and privileged-mode bounds, environment poisoning,
fixed source/build selection and fixed errors. Actual Windows ACL tests never change parent/host ACLs. Native Linux
filesystem tests run on Linux CI; cross-compilation alone does not claim native execution coverage.
