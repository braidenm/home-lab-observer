# Fixed connected installation lease

Status: narrow unused Linux implementation slice. This does not authorize installation, refresh, recovery or worker activation.

The first installed profile serializes privileged setup and lifecycle work on one fixed file,
`/run/lock/home-lab-observer-connected.install.lock`. Only real/effective root can acquire it. The parent must be a
root-owned directory reached without symlinks; group/other write is accepted only when the sticky bit protects the
directory. The lock file must be root:root, one-link, empty, regular, exactly 0600 and free of POSIX ACLs. Opening it
never follows a link or blocks on a FIFO. A second holder receives a distinct busy result rather than waiting or
continuing. Closing releases the kernel lock but does not unlink the file, which would permit split-brain locking.

This package intentionally has no caller-supplied production path, file writer, installed-profile reader, service
operation, or remote API. A private descriptor-taking seam exists only for synthetic root-owned tests. The later
installer must acquire this lease before reading transition authority and retain it through stop/join, durable writes,
verification and completion. Holding the lease alone proves none of those obligations. A disposable VM still must
prove the exact installed manager/profile and crash-recovery transaction before the first connected release.

Acceptance for this slice is root-owned synthetic refusal of foreign/broad parents, foreign/broad/nonempty/linked/
special lock entries and contention, plus repeated acquisition after release. Non-root calls refuse without touching
the fixed path. No test creates persistent accounts, units, credentials or a live registration.
