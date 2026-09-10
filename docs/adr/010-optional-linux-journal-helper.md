# ADR 010: Bundle an optional Linux journal helper without changing core portability

Status: Accepted (owner approved optional bundled native helper on 2026-09-10).

Partially supersedes [ADR 001](001-single-go-binary.md) only for the Linux log-reader artifact count.
Extends [ADR 007](007-native-preview-installation.md); installation still grants no new authority.

## Context and evidence

The proposed journalctl continuation cannot prove the referenced record still exists. Loading libsystemd through
purego in the main executable would add dynamic-loader requirements even when logs are disabled. The existing archive
allowlist contains exactly four files and cannot silently acquire another executable. See
[the primary-source investigation](../architecture-research/004-safe-native-log-observations.md).

## Decision

Keep the main Linux observer CGO-disabled and free of the journal helper's dynamic-loader imports. Include a separate
`observer-journal-helper` in each new Linux archive, built from this repository and the same release commit. One
download/install still supplies the product. Windows keeps its fixed same-binary WEVTAPI helper; macOS logs remain
unsupported. No helper is downloaded or launched merely by opening the UI, installing, or using host/Docker metrics.

Only explicit `--log-source system` may invoke the Linux helper. It dynamically loads the host's existing libsystemd
using pinned purego (already an indirect dependency); no library is bundled, installed, or upgraded. Missing loader,
library, symbol, helper, or incompatible runtime yields a fixed unavailable capability, never a main-process crash or
healthy zero. A source test must prove this fallback, including a minimal non-glibc host running the main executable.

The parent resolves only the fixed helper adjacent to its own executable, validates file identity/links/ownership and
the SHA-256 embedded during the same release build, and checks the helper's closed version/protocol identity. No PATH,
caller-selected executable, shell, remote library or arbitrary command is allowed. The helper accepts a bounded private
request on stdin and returns only a closed normalized batch on stdout; stderr is discarded. Its environment is a
minimal code-owned allowlist, excluding loader injection and systemd overrides before exec. No cursor or secret enters
argv, environment, files, diagnostics or public responses. The helper has no history-write or HTTP role. This is a
failure-isolation boundary at the same user's authority, not a sandbox against a malicious owner account.

The parent enforces two seconds per source (within the four-second lane deadline), bounds request/output bytes, and
kills then reaps timed-out children with bounded cleanup. Native journal work stays on its creating OS thread. Read
only local system-journal realtime, priority, message ID and private cursor. Never enumerate fields or read MESSAGE.
Selected native fields are capped at 4 KiB; cursor/request bounds remain 16 KiB/32 KiB and complete helper responses,
including JSON framing, remain at most 2 MiB. Malformed/oversized output never becomes a committed success.

Continuation requires seek, next and `sd_journal_test_cursor` exact-presence proof before moving to the first unseen
record. A missing cursor causes the same bounded tail-reset transition, without captured/discarded counts. Initial
empty windows require metadata-only visible-tail evidence before claiming zero coverage; no visible tail reports
`NO_VISIBLE_JOURNAL`, not a healthy zero. Coverage refers only to the caller-accessible local system-journal view:
libsystemd can silently omit inaccessible files, so the product never claims all host events were observed.

## Package compatibility

Introduce a closed release-manifest v2 with exact per-asset content profiles. Linux profiles require the original four
files plus the fixed helper; other platforms retain four. V2 records the helper filename, hash and size (null when
absent), and all entries remain covered by archive hashes and provenance. Build the helper first, embed its digest in
the main binary, then package both. Keep existing archive size limits unless a measured build requires a reviewed
change. Reject extra/missing/linked entries, wrong OS/architecture, helper digest/version mismatch and mixed versions.

New installers understand v1 four-file rollback archives and v2 exact profiles. Old installers are not silently taught
to accept unknown files: owners use the installer belonging to the chosen release. Managed upgrade stages both
executables before selecting a version; explicit restart remains required. Uninstall removes only recognized program
files and preserves observations. Missing helper at runtime degrades logs only; a missing helper in a downloaded v2
archive is an invalid installation, not a reason to skip verification.

## Verification and consequences

Tests cover native exact/stale/empty cursors, permission-limited views, source/field allowlists, secret canaries,
malformed protocol, timeout/reaping, dynamic-loader absence, helper mismatch, atomic installer interruption/rollback,
archive traversal/link attacks, and same-commit provenance. Test supported systemd versions and Linux amd64/arm64;
false cursor-test failures must produce conservative reset/gaps, never fabricated continuity. Before declaring supported
versions, record actual native fixtures against representative older and current systemd releases.

The alternative of leaving Linux logs unsupported is simpler but does not meet the owner's chosen completeness goal.
Putting the library in the core risks unrelated startup failure; relaxing cursor privacy or claiming unproved coverage
is rejected. The extra executable increases packaging and release tests, but remains an optional, bounded component.
Publisher signing/notarization, macOS native logs and remote management remain separate readiness requirements.
