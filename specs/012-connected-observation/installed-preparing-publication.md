# First-install preparing marker publication

Status: narrow unused Linux implementation slice. This grants no installation or activation authority.

Before the first connected installation creates accounts, credentials or units, it records a canonical bounded
`observer-connected-preparing/v1` marker under the fixed root-owned config directory. The marker contains only the
validated server ID and bundle manifest SHA-256; it contains no grant, credential, hostname or command. The publisher
accepts a typed record, never a caller-selected path or filename. The eventual installer must hold the fixed
installation lease, prove the destination is a new owned setup, and stop on any incomplete state before calling it.

The Linux publisher opens `/etc` and `/etc/home-lab-observer-connected` through no-symlink anchored descriptors.
`/etc` must be root-controlled; the config directory must be root:root, exactly 0755 and ACL-free. It must be empty.
The only temporary role is `.preparing-next`, created exclusively without following links as root:root 0600,
one-link, regular and ACL-free. The publisher syncs its bounded contents and directory, renames to `preparing.json`
without replacement, and syncs the directory again. Existing names, unknown entries, failed writes or interrupted
sync stages return recovery-required; the publisher never adopts, unlinks, overwrites or cleans up evidence.

Synthetic root-owned acceptance covers successful canonical bytes, non-root refusal, foreign/broad/ACL directories,
unknown/linked/special entries, existing target/temp and a post-sync interruption whose residue blocks retry. No
test touches the fixed production path or creates accounts, units or credentials. Installed ext4 durability, held
lease, stopped workers and full crash-recovery behavior remain separate acceptance gates.
