# ADR031: Non-root action listeners in broker-owned private leaves

Status: Accepted for local paired-runtime implementation/tests after root and independent source review; installation and activation remain gated.
Date: 2026-11-02.
Supersedes only Spec011's root-owned listener-leaf requirement referenced by ADR030, not its other decisions.

## Context and decision

The draft requires root-owned0750/0700 listener directories, but a non-root broker cannot create sockets there.
Do not fix that by granting the worker shared-group directory-write authority or adding ambient root execution.
There is no installed action profile to migrate; observation remains independent.

Keep separate static non-root worker/broker identities. Root provisions protected ancestors and broker-owned
terminal listener directories. Worker leaf: broker:IPC0750, socket broker:IPC0660. Admin leaf: broker:broker0700,
socket broker:broker0600. Root-only administration means initial-host UID0 with sufficient host DAC authority
to connect AND mandatory SO_PEERCRED UID0, not UID0 ownership of the inode. CLI verifies the exact broker peer,
protected pathname and socket witness. Ordinary worker/observation identities remain denied.

The [fixed process profile](../../specs/011-container-actions/process-profile.md) owns concrete names/files.
No group-write, socket activation, inherited descriptor, generic path/UID option or automatic adoption/repair.
Refuse foreign/pre-existing endpoints; after join, normal cleanup targets only the captured owned listener witness.

## Alternatives and consequences

A root broker simplifies socket creation but adds ambient identity privileges without containing Docker authority.
Root-created listener handoff preserves root-owned leaves but adds privileged bootstrap/inherited-FD semantics
rejected by the current profile. Broker-owned leaves allow ordinary creation without worker directory-write access.
Broker compromise still permits root-equivalent Docker abuse; this decision does not add daemon authorization.
Host-root/broker compromise is outside the containment claim.

Test actual identities, worker denial, initial-host-root success, wrong peers, ACLs/links/foreign sockets and exact
cleanup. Effective packaged systemd/VM tests precede installation/release; temp fixtures are not that evidence.
Install disabled. Rollback stops/joins while preserving compatible journal, mappings and credentials.

## Primary evidence

Checked 2026-11-02: [Linux unix(7)](https://man7.org/linux/man-pages/man7/unix.7.html) documents directory write/search,
socket write permissions and connection-time SO_PEERCRED. [Linux capabilities(7)](https://man7.org/linux/man-pages/man7/capabilities.7.html)
explains host DAC authority; unrelated user-namespace UID0 is not the selected administrator.
[systemd255 execution settings](https://www.freedesktop.org/software/systemd/man/255/systemd.exec.html) supplies
identity/sandbox controls whose actual effective profile must be verified, not inferred from a template.
[Linux namespaces(7)](https://man7.org/linux/man-pages/man7/namespaces.7.html) documents namespace handles and bind
mount lifetime; [NSFS ioctls](https://man7.org/linux/man-pages/man2/ioctl_nsfs.2.html) identifies namespace type.
The fixed protected native reference in the profile supplies host namespace provenance; self/PID1 equality in
an arbitrary container, a copied inode number or UID-map spelling does not.
