# ADR 030: Separate trusted action delivery and networkless Docker execution

Status: Accepted for local consumed paired-runtime design and implementation/tests only (2026-11-02 root/peer review).
First-release complete recovery prerequisite superseded by [ADR034](034-locked-uncertain-outcomes-first-release.md); uncertainty fences and other safety contracts remain.
Default product delivery path superseded by [ADR032](032-reuse-first-management.md); dormant-code safety contracts remain unchanged.
Listener-leaf ownership is superseded by [ADR031](031-nonroot-action-listeners.md); other decisions are unchanged.
Date: 2026-11-01. Installation, release and production action activation remain gated; broader Spec011/ADR027 is not accepted.

## Question and current evidence

How do we turn the merged typed coordinator, durable journal and fixed Engine adapter into a usable packaged
worker without giving observation credentials host-control authority? Observer main `c2f11b2` contains those
three unused components and disposable Engine proof, but no action command, service, remote authentication,
mapping administration or owner reconciliation. Platform ADR041 selects separate outbound worker/local proxy;
ADR052 defines separate action credentials and their prepared/active renewal. Spec011 and ADR027 remain Proposed.
Standalone action authentication is deferred beyond this first connected profile; standalone observation is unchanged.

## Proposed decision

Deliver one Linux-first vertical package: a separately credentialed outbound action worker, a networkless local
Docker broker, and an explicit root-run installer/admin CLI. Reuse actioncore/actionjournal/actionengine rather
than creating a second coordinator. Keep all observation executables, secrets, services and interfaces unchanged.
The owning [packaged-worker contract](../../specs/011-container-actions/packaged-worker.md) defines acceptance.
Runtime base c2f11b2 is historical; subsequent5fd9e335 adds accepted disposable native race tests, not installed authority.

Both action processes belong to the authority trusted computing base. The worker authenticates fixed-origin TLS
claims and obtains fresh request-bound current-permission decisions at both core checkpoints. The broker accepts
that provenance only from the fixed action-worker peer over protected, bounded, correlated IPC, then independently
enforces its protected local approvals and durable journal. Peer credentials authenticate the local bridge, not
the remote user or permission. A compromised worker can falsely assert remote authority and abuse locally approved
operations. It cannot obtain a generic Docker/host operation through the typed broker protocol, but can disrupt
approved containers within the broker's bounds. This is an explicit residual, not compromise-proof delegation.

Docker access remains root-equivalent inside the broker. Network isolation and typed code do not contain a fully
compromised broker from abusing that socket. The read-only worker/collector and ordinary local users must have
neither action IPC access nor action secrets. Root/host-owner compromise is outside this boundary.

No signing-key or ticket framework is added. Authentic TLS plus a deliberately trusted local bridge is sufficient
for this threat model. If resistance to a compromised action worker becomes a requirement, supersede this decision
with an independently verifiable authority design; do not retroactively call local UID checks such verification.
Root administration uses a separate protected root-only ingress; the action-worker socket cannot invoke local
approval or reconciliation initiation. The owning root-admin codec and fixed bounded policy representation select
the Proposed members/limits; exact vectors and effective profile remain implementation acceptance obligations.

## Reconciliation and operational limits

An unknown outcome is never rewritten as success, retried, or removed. Proposed initial recovery requires BOTH
an explicit root-local CLI ceremony and fresh authenticated current remote-owner approval bound to the exact
remote receipt/evidence class/lane and fresh broker check. Root privilege alone does not prove current remote ownership. Reconciliation
records bounded current-state inspection and acknowledgement of uncertainty, releases only the acknowledged
fence, and requires a new separately authorized request for further work. It does not prove causal outcome.
Credential loss stops remote actions; explicit re-enrollment may restore the same binding only after current-owner
verification, preserving the existing journal. Ownership/installation changes cannot adopt that journal.
Because unknown intents retain two GLOBAL permits and failed inspection cannot release them, two inaccessible
targets can disable all installation actions indefinitely. No force-clear or journal replacement is implied.

Recovery must finish both the local fence and the remote admission lane through separately committed, identity-bound,
idempotent steps; it is not a distributed transaction. The exact recovery contract must also handle a lost claim
response with no local record and a retained terminal whose remote result was lost. Neither absence nor current
container state proves no earlier effect. The owning admin/v2 contracts select these four cases explicitly;
capacity/crash/authority proofs remain implementation acceptance gates. Missing exact protected mapping refuses;
there is no unbounded history service.

## Alternatives

The proposed [journal v2 contract](../../specs/011-container-actions/action-journal-v2.md) selects a separate
fixed2048-byte recovery slot per action and an explicit RECOVERY_ONLY origin for missing local evidence. Original
INTENT/terminal bytes and private v1 fingerprint remain unchanged; full remote16 receipt gets a separate digest.
Reject ordinal3 ACK/synthetic terminal and automatic v1 migration: they conflate recovery with dispatch evidence.
Versioned constructors/reader floor, exact local CAS/retention and capacity/fault tests belong to the consumed
worker slice. This Proposed addition does not amend accepted ADR028's unused v1 no-recovery contract.

| Option | Tradeoff |
| --- | --- |
| Leave actions disabled | Safe current state; no complete control feature |
| Separate trusted worker and networkless broker | Retains ADR041; small private IPC, independently enforced local scope; proposed |
| One HTTPS-and-Docker process | Less IPC, but changes accepted separation and expands one process's attack surface |
| Untrusted courier plus signed remote decisions | Stronger worker-compromise boundary; extra keys, replay/freshness/rotation contracts not required by current threat model |

## Proof, rollout and rollback

Accept the detailed protocol, remote endpoint prerequisites and journal reconciliation migration before code.
Deliver two small consumed PRs: runnable worker/broker with real isolated protocol, lookup and reconciliation;
then installed enrollment/renewal/profile/lifecycle and disposable VM proof. Do not label another unused port a
completed worker. Both cuts precede production activation. Install disabled. Enable only after identity, file, IPC, egress, journal and compatible remote
reader/authority checks. Rollback stops admission, joins processes and preserves compatible journal/credential/
mapping readers. Never downgrade below the first reader of new journal reconciliation records or remote facts.
Missing/corrupt state requires recovery, not reset. Independent review and current-head CI precede publication;
existing workloads remain excluded from fixture acceptance.

## Primary evidence and fit

Checked 2026-11-01: [Docker Engine security](https://docs.docker.com/engine/security/) explains the broad authority
of the daemon socket; our narrow broker does not install a daemon authorization plugin.
[Linux unix(7)](https://man7.org/linux/man-pages/man7/unix.7.html) distinguishes pathname permissions, peer credentials
and ancillary descriptor transfer; peer UID is deliberately not remote permission proof.
[systemd255 kill semantics](https://www.freedesktop.org/software/systemd/man/255/systemd.kill.html) describes final
process termination after stop timeouts; killing the client does not establish absence of a Docker side effect.
Existing ADR029 owns fixed HTTP/API/no-retry behavior, ADR028 owns journal durability, and Platform ADR052 owns
credential TTL/renewal. Product-selected IPC sizes and shutdown allowances below require tests, not vendor authority.
Checked 2026-11-02: [Linux clock_gettime(2)](https://man7.org/linux/man-pages/man2/clock_gettime.2.html) explains
suspend-inclusive CLOCK_BOOTTIME; [Go's monotonic-clock documentation](https://pkg.go.dev/time#hdr-Monotonic_Clocks)
warns that ordinary monotonic readings may pause during sleep and are not serialized. The proposed nonce-before-claim
budget and controlled write-boundary guard preserve conservative limits; they cannot make a deadline check and
Docker mutation atomic. Exact paired timing and recovery proofs remain required before implementation.
