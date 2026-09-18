# First-install state-directory creation

Status: narrow unused Linux implementation slice; this grants no installation or worker-activation authority.

The first connected installation creates only the fixed `/var/lib/home-lab-observer-connected` state root. The
creator must be real/effective root and independently pin `/var` and `/var/lib` through no-symlink descriptors.
Both ancestors must be root:root, non-writable by group/other and free of POSIX ACLs. It exclusively creates the
absent child with `mkdirat`, sets and verifies root:root 0755 without following links, then syncs the new directory
and its `/var/lib` parent. Existing directories, symlinks and other objects are never adopted, removed or repaired.
After creation, any metadata or sync failure is recovery-required and preserves the directory as evidence; retry
must refuse it even if it looks correctly owned. No caller can choose a path or mode.

Synthetic root-owned acceptance covers successful exact metadata, non-root refusal, hostile ancestors, existing
targets, and injected child/parent sync failures with retained residue and refused retry. The fixture never touches
the production `/var/lib` path or creates credentials, accounts, units, release trees or services. A future caller
must hold the installation lease and prove a new setup; this creator does not prove that, nor does it establish
abrupt-power-loss durability or a complete installed profile.
