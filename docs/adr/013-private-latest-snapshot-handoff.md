# ADR 013: Private disposable latest-snapshot handoff

Status: Accepted for the credential-free library; installation/network activation remains gated.
Date: 2026-09-11.

## Decision

Use a pinned Go `os.Root`, opened-handle privacy checks and three fixed filenames for the latest numeric-host document,
staging and writer lock. Call the explicit projector on publication and its strict validator on read. Existing path
shape helpers alone do not verify ownership/permissions and must not be treated as credential isolation.

This is replaceable observational state, not an upload transaction ledger. File sync precedes replacement; Unix directory
sync follows it. On Windows, reject transient/missing/invalid reads and report replacement failure rather than claiming
universal atomicity. Restart can discard incomplete staging and recollect. Never recover a pending action or upload
sequence by inferring it from this slot. Filesystem failures have fixed errors with no contents/paths.

## Alternatives and evidence

| Option | Fit | Tradeoff |
| --- | --- | --- |
| Existing pathname checks then ReadFile/WriteFile | Small | Replacement races; existing permissions not proven |
| Pinned root + validated handles + bounded slot | Chosen for disposable state | OS-specific privacy checks and truthful durability limits |
| Authenticated local socket/pipe | Avoids persistent latest file | Peer ACL and lifecycle complexity; no offline slot; reconsider for differently owned workers |

Primary sources consulted 2026-09-11:

- [Go traversal-resistant APIs](https://go.dev/blog/osroot) and [Go os.Root](https://pkg.go.dev/os#Root): rooted operations
  prevent traversal escape on supported native targets, but do not forbid all symlinks or privileged mount changes.
  Add link checks and validate opened objects; only fixed single-component children are used. Runtime: Go 1.27.1.
- [Linux fsync](https://man7.org/linux/man-pages/man2/fsync.2.html): syncing a file alone does not flush its directory
  entry. Directory-sync failure is a failure outcome, not a claimed durable commit.
- [Microsoft GetSecurityInfo](https://learn.microsoft.com/en-us/windows/win32/api/aclapi/nf-aclapi-getsecurityinfo)
  supports querying the actual opened object's owner/DACL. A missing/null DACL must not be accepted as privacy.
- [Go os.Rename](https://pkg.go.dev/os#Rename): non-Unix platforms do not promise atomic rename. Deliberately require
  only validated whole-document reads or a fixed error on Windows. Do not apply this weaker latest-slot guarantee
  to credentials, admission/audit records or mutation retries.
- [Apple Libc ACL implementation](https://github.com/apple-oss-distributions/Libc/blob/main/posix1e/acl_file.c),
  [statx](https://github.com/apple-oss-distributions/Libc/blob/main/sys/statx_np.c) and
  [filesec](https://github.com/apple-oss-distributions/Libc/blob/main/gen/filesec.c): macOS mode bits alone do not
  exclude extended ACL grants. Query the opened descriptor through the fixed system library, accepting only
  documented `ENOENT` absence or an ACL whose validated size equals a freshly allocated empty ACL. Reject all
  nonempty ACLs and other failures; never interpret arbitrary ACL enumeration errors as absence. Linux POSIX ACL
  named-user/group effective access is constrained by the already-zero group-class mode mask.

## Threat model, operation and rollback

Owner-only directories protect against other ordinary users, not compromised code running as that owner or administrators.
Two processes sharing an OS user do not provide compromise containment. Elevated/headless networking still requires the
accepted collector/uploader isolation design and egress restrictions before deployment. This library does not activate it.
Future distinct service accounts need a separate explicitly provisioned shared read boundary, not relaxed generic permissions.

No new dependency, listener, secret, domain event or schema migration. Native temp-fixture tests and fault injection must
pass; never test against a user's server directories. Rollback removes an unused library; installed slot cleanup later
must target only its owned fixed files. Preserve existing standalone and legacy Compose installations unchanged.

## Implementation clarification: newly created Windows ownership

Windows assigns new objects the creating token's default owner, which can be Administrators for an elevated token rather
than its user SID. Preserve the exact-current-user rule by proving exclusive creation before initializing an empty file's
owner. Use ReOpenFile on that same handle to request WRITE_OWNER, then SetSecurityInfo; validate the original handle before
payload writes. Existing objects are never repaired. The ordinary Go read/write handle does not itself request WRITE_OWNER.
This initializes new storage rather than accepting a broader principal or mutating the process token. Tests also set the
owner of their newly created temp directory explicitly; no existing owner directory or machine policy is changed.
Sources: [Windows object ownership](https://learn.microsoft.com/en-us/windows/win32/secauthz/owner-of-a-new-object) and
[ReOpenFile](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-reopenfile), accessed 2026-09-11.
