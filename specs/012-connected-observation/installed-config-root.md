# First-install config-directory creation

Status: narrow unused Linux implementation slice. This does not install or activate a worker.

The first connected installation needs exactly one new config directory at
`/etc/home-lab-observer-connected` before publishing its preparing marker. The fixed creator opens `/etc` through
a no-symlink root-anchored descriptor and verifies root:root, non-writable ownership and no POSIX ACL. It uses
exclusive `mkdirat`; an existing directory, link or other object is never adopted, repaired or removed. The new
directory is set to root:root 0755, verified through a no-symlink descriptor, then it and `/etc` are synced.

Any create, metadata or sync failure is recovery-required and preserves the new directory as evidence. A retry
refuses the existing path even when it appears correctly owned. Synthetic root-owned acceptance covers normal
creation, non-root refusal, foreign/writable/ACL parent, existing and symlinked target, and failures injected at
both sync stages with retained residue and refused retry. No test writes `/etc`, creates an account or unit, or
starts a worker. The future caller must separately hold the installation lease and prove an entirely new setup;
this creator alone is not evidence of a complete installation or durability under abrupt power loss.
