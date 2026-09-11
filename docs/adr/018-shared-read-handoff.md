# ADR 018: Separate shared-read handoff policy

Status: Accepted for the uninstalled Linux library and owned DAC/ACL fixture only.
Date: 2026-09-11.

## Decision proposed

Add a Linux-only shared-read handoff library with exact collector UID, dedicated read GID, uploader UID and server binding.
Use separate Writer and Reader types and trusted provisioned numeric policy. Preserve ADR 013's owner-private Store without
relaxation. The new directory is collector-owned/read-group-readable, final data 0640, and uploader never receives mutation
methods or filesystem write permission. No root provisioning or worker activation is implemented in this slice.

The alternative of reusing owner-private permissions cannot share between distinct principals. Generalized arbitrary mode
or ACL policy would broaden the API and make the private-store guarantees harder to review. Duplication of a small fixed
publication sequence is preferable to mixing incompatible security policies; reuse canonical projection and identical
path utilities only. Named-user ACL sharing could express a direct reader identity, but is intentionally not selected:
the accepted installed profile uses a dedicated group and refuses extra/default ACLs.

Group exclusivity requires root installer/account management; kernel metadata and current process groups cannot prove
that no other user belongs to that group. Document and test this prerequisite, rather than implying the library establishes
the complete installed isolation boundary. Same-owner malicious collector/root and compromised group provisioning remain
outside library confidentiality guarantees. A restricted service profile is still mandatory before deployment.

## Evidence

Primary sources accessed 2026-09-11:

- [Linux getxattr](https://man7.org/linux/man-pages/man2/getxattr.2.html): descriptor-relative attribute lookup and zero-size
  queries. Generic ENODATA can involve access controls; constrain absence handling to reviewed POSIX ACL behavior.
- [Linux v6.8 POSIX ACL implementation](https://github.com/torvalds/linux/blob/v6.8/fs/posix_acl.c): vfs_get_acl permits VFS
  ACL reads, delegates security-module decisions, and returns ENODATA for absent ACLs. Unexpected policy errors fail closed.
- [Linux v6.8 xattr dispatch](https://github.com/torvalds/linux/blob/v6.8/fs/xattr.c): POSIX ACL names use the ACL path.
- [Linux group credentials](https://man7.org/linux/man-pages/man2/setgroups.2.html): supplementary process groups and
  privileged credential changes; tests must use actual distinct identities rather than mock successful permission strings.
- [Linux user namespaces](https://man7.org/linux/man-pages/man7/user_namespaces.7.html): namespace identities and authority
  differ from host identities; a one-UID namespace mapping would not prove this multi-principal profile.

The proposed root-orchestrated chroot fixture avoids host account creation and confines ownership changes to synthetic
temporary state. It requires separate setup review before privileged execution. See [E1 contract](../../specs/012-connected-observation/shared-handoff.md).
