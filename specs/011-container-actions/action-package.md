# First connected action package and provisioning plan

Status: Package direction accepted; only the linked cut1 credential consumer is approved for local authoring/tests.
No package build, download, VM, host, credential, service or activation approval. Baselines: Observer4193c3d (#175), paired Platform
b07a2335 (fresh main inspection, 2026-11-04 local / 2026-11-05 UTC). Use actual reviewed merged inputs at delivery.
This implements [ADR034](../../docs/adr/034-locked-uncertain-outcomes-first-release.md), preserving
[ADR030](../../docs/adr/030-packaged-typed-action-workers.md) and [ADR031](../../docs/adr/031-nonroot-action-listeners.md).
It does not revive the deferred admin/recovery/v2 package or amend accepted decisions.

## Outcome, existing source and minimum cuts

Install a separate Ubuntu24.04/amd64 action package STOPPED/DISABLED, explicitly enroll the current owner’s
existing logical installation with an ACTION bootstrap, approve a FIXED target set, then separately prove and
authorize activation. Observation packages, secrets, users, manifests, units and release formats remain unchanged.
First live acceptance uses one exact disposable target on a separate VM/throwaway registration, never the normal
observed server or its workloads. No automatic uncertain-action retry, fence clear, replacement or re-enrollment.

| Existing source | Reuse / actual missing work |
| --- | --- |
| `cmd/connectedpack`, `internal/connectedbundle/{bundle,archive,rich_docker}.go`, `internal/connectedidentity/identity.go` | Reuse exact-content verification, canonical manifest, bounded ELF/build-info/embedded identity scan and reproducible archive machinery through a new CLOSED action profile. Current roles/profile do NOT accept action binaries. |
| `scripts/build-connected-rich-docker-linux.sh`, `internal/connectedbootstrap` | Reuse clean exact-commit, pinned Go1.27.3/CGO0/Linux-amd64 and two-build comparison/bootstrap patterns. Do not edit observation artifact lists/digests or execute candidates merely to discover identity. |
| `internal/connectedinstall{lease,/files_linux.go,/layout_linux.go,/manager_linux.go,/principals_linux.go}` | Reuse the fixed privileged lease and reviewed descriptor/fsync/manager ownership mechanisms only where policy matches. Existing installer enrolls OBSERVATION and its collector/uploader pair is NOT an action role/profile. No generic command/path/UID framework. |
| `internal/actionprofile`, `actionworker`, `actionremote`, `actionbroker`, `actionengine`, `actionjournal` | Current consumed execution path exists. Missing: credential lifecycle client/writer/renewal, package identity embedding, stopped provisioning, effective units, host-namespace preparation, lifecycle/status and exact-package acceptance. |

Proposed coherent sequence (not a chain of unused libraries):

The exact first consumer contract is [credential-setup-worker.md](credential-setup-worker.md); root/independent R2
review accepted that narrow design and source-first d3aaf3d preceded its local runtime authoring. Local source/test
receipts live there; they do not approve the remaining package cuts or installed authority.

1. **Consumed ACTION credential setup:** closed lifecycle codecs/client, protected atomic handoff store, one
   worker-identity bootstrap mode of the setup command, and renewal integrated at quiet boundaries of the existing worker loop.
   Tests cover real typed TLS and private filesystem faults. It is source-only until the package consumes it.
   The paired owner UI/API-client issuance/metadata/revoke cut is a parallel prerequisite, not observation enrollment.
2. **Stopped action bundle + provisioning/lifecycle:** new closed pack profile, three binaries below,
   fixed units and root manager consuming the existing runtimes plus cut1. Install/enroll remain stopped;
   initial approval and fresh v1 journal provisioning are explicit. Setup has a closed root/worker/broker mode
   matrix, not another static user or service. This is the next package implementation PR.
3. **Exact artifact acceptance + explicit canary:** separately approved disposable ordinary-user VM/network plan,
   actual installed roles/isolation/reboot/lifecycle and real paired API/Engine faults. No unrelated framework,
   privileged test runner, host cap change or activation by a green source checklist.

Delivered local/source checkpoint: [PR175](https://github.com/braidenm/home-lab-observer/pull/175) merged as4193c3d
after all18 current-head hosted checks passed. Exact61 reviewed/merged files and clean source parity were verified;
ordinary Windows checks and Linux race/vet exercised actual owned handlers/Unix/SQLite with synthetic authority/
actuation. Snapshot/COMMIT/Write guards, fixed sources and joined shutdown are source-complete, not installed proof.
ACTION lifecycle/setup, effective units/roles, paired API/Docker faults and exact-package canary remain OPEN.

## Actual paired credential contract (not the execution codec)

Inventory source: counterpart `HomeLabActionCredentialSetupEndpoint`, `HomeLabActionCredentialRequest`,
`HomeLabActionCredentialSetupService`, `HomeLabActionCredentialTransitions` and Spec075 `credential-setup-http.md`.
O=`/v1/admin/home-lab/servers/{serverId}/action-credentials`;
W=`/v1/connectors/home-lab/servers/{serverId}/action-credentials`.

| Existing route | Authority, body and successful result | Loss/retry rule |
| --- | --- | --- |
| POST O/enrollments | Current owner Bearer; exactly `{expected_connector_instance_id}`; 201 instance/one-use authorization_secret/expires_at | Explicit reissue revokes unconsumed enrollment; ACTIVE conflicts. Never read back a lost secret. |
| GET O | Current owner Bearer, no body; current instance plus nullable active/prepared metadata | Fresh metadata, no secret. UI can obtain the expected instance here rather than assume an observation DTO carries it. |
| POST O/revoke | Current owner Bearer; exact expected instance; 204 | Monotonic action-only revoke, no observation credential change. Never a Docker uncertainty unlock. |
| POST W/exchange | `ActionEnrollment` plus `hlae_` secret; no body; 201 ACTION secret/ACTIVE metadata | Single use; no automatic POST retry or replay-based secret recovery. |
| POST W/prepare | Current ACTIVE `ActionConnector`/`hlac_`; no body; 201 secret/PREPARED metadata | One successor, exact predecessor; lost secret is unavailable, not another prepare loop. |
| POST W/activate | PREPARED `ActionConnector`; no body; 200 metadata | Exact predecessor CAS; already-ACTIVE same successor returns metadata only. Ambiguity requires status lookup, not execution. |
| GET W/status | ACTION-shaped `ActionConnector`, no body; ACTIVE/PREPARED/INACTIVE metadata | Read-only credential inquiry, not positive owner/execution permission. |

All worker calls require the exact `X-Home-Lab-Connector-Instance`. Tokens are81-byte purpose-specific values
(`hlae_`/`hlac_`,32lowerhex, dot, canonical43-character base64url/32bytes); schemes alone are case-insensitive.
An observation `hle_` grant/snapshot Connector secret, JWT or malformed/wrong-purpose token is never substituted.
Metadata has credential_id, revision, state, expires_at; revision is a JSON signed-Long NUMBER here, unlike execution
revision STRINGS. INACTIVE is null/0/INACTIVE/null. Server DTO uses Instant: freeze actual supported UTC-microsecond
response forms with real HTTP fixtures, not an assumed six-fraction execution decoder; format NEW local records
to the existing exact six-fraction UTC contract without rounding a non-microsecond value.

Bootstrap and PREPARED TTLs are10min; ACTIVE TTL30days; equality expires. Renewal starts seven days early (worker
scheduling, not a server prepare-window restriction). One ACTIVE and one PREPARED; activation revokes predecessor
atomically, without overlap. Missing eligible verifier keys/schema/dependency yields503, not invalid/INACTIVE.
400 validation,401 auth,403 owner deny,409 instance/slot/CAS,429 admission and503 dependency are distinct; all mapped
responses are no-store/correlated. Preserve quotas; do not retry a lifecycle POST on429/5xx/timeout/response loss.

**Current gaps:** Observer Client hard-wires container-actions paths and validates ACTION only; it cannot exchange
an ENROLLMENT. Add typed lifecycle operations sharing only its private fixed-origin TLS/bounds/no-replay machinery,
not an exported arbitrary request/scheme/path API. The first-party owner UI currently has no ACTION issuance flow;
its observation-enrollment affordance is not this feature. No curl/header/file-edit production-secret workaround.
The proposed supported UI discloses the one-use bootstrap once; a bounded no-echo installer prompt pipes it only
to the worker-identity bootstrap process. No owner JWT in the package, argv, environment or unit properties.

## Credential persistence and renewal: required, not pretend optional

Keep `active.json` and settings exactly as [process-profile.md](process-profile.md). Add one bounded worker0600
handoff record in the existing worker0700 directory, with exact binding/predecessor, finite phase and optional
PREPARED secret/metadata; freeze its exact closed bytes/CAS and fault tests before runtime code. Use one fixed
worker-file lease to serialize supported bootstrap/worker renewal, not a database or action-permission token.
Atomic publication requires validated retained directory FD, exclusive temporary leaf, owner/mode/ACL/link checks,
write-all/fsync, same-directory rename and directory fsync. Failure preserves the last known evidence; no wipe/adopt.

Before exchange or prepare, durably mark that attempt; a restart must not hide an unconfirmed POST behind an old
ACTIVE file. Persist PREPARED before activation. On activation ambiguity, query new then old status, exact binding/
ID/revision: only positively matching ACTIVE successor permits atomic local promotion. PREPARED, INACTIVE,
dependency failure or absent successor secret remains credential-recovery-required, with no claim or POST retry.
Status resolution is credential-only; it never clears the worker’s uncertainty latch or local/central target fences.
Lost bootstrap/prepare disclosure or unpersisted secrets can leave this first profile unavailable; no automatic
re-enrollment, replacement journal or manual active-file fabrication is offered as repair.

The existing worker is sequential. Renew only after its current conversation/report has joined and before another
BEGIN/claim; recheck due/expiry on wake. Do not rotate in the middle of an owned lease or create a concurrent timer
writer. Keep the same process latch across successful renewal; no automatic service restart/resume. A retained
handoff blocks claims after process restart until positively resolved. Lack of renewal is NOT an indefinitely
operable30-day package; any proposal to defer it needs an explicit bounded-use scope decision, not a checkbox.

## Closed artifact and installed profile proposal

New profile `ubuntu24.04-systemd255-amd64-actions-v1-canary`, new action-only resource/contract digest using the
existing manifest-v2 envelope; three exact roles/binaries: action-worker/observer-action-worker,
action-broker/observer-action-broker, action-setup/observer-action-setup. Add build identity embedding/checking to the two current launchers;
preserve their no-argument/manager-environment admission. No native-v2 or observation-role validator loosening.
Frozen resources include only worker/broker units, bootstrap properties, bounded installed/settings/credential/
handoff/initial-policy schemas and instructions. Exact final filenames/bytes/digest precede pack implementation.
No admin command/socket/unit, journal-v2, ACK, updater, downloader, shell or generic service manager.

The three launchers require an embedded connected v2 identity with their exact action role and the digest of
the immutable ASCII contract marker `ubuntu24.04-systemd255-amd64-actions-v1-canary:identity-v1` (no newline).
Missing, mismatched and non-Linux/amd64 identities refuse before accessing configuration or credentials.
This marker identifies the binary contract only; the eventual package manifest separately binds every resource
and binary byte. It is not a signature or a claim that an installation has passed acceptance.

The binary build step uses clean pinned Linux/amd64 source and Go 1.27.3, builds all three roles twice, and
publishes the matching binaries and a checksum list into a new local output directory. This intermediate
output is not an installable release: it contains no units, installer, host configuration or credentials.
The build-only candidate archive wraps that exact four-file payload in a deterministic ustar/gzip archive,
compares two archive builds and publishes a separate archive checksum. It is for disposable acceptance staging,
not for owner download or service installation. The installed manifest and verified service resources remain separate gates.

Root-owned `/opt/home-lab-observer-actions/releases/<manifestSHA>` holds verified immutable payload; separate
root-owned installed record and worker-root/broker-root skeletons use existing protected publication patterns.
Keep ADR031 exact role/path/mode/ACL rules. Resolve unique numeric IDs and reject foreign primary/supplementary
memberships; worker gets IPC only, broker gets IPC and the explicitly approved Docker group. Never chmod the global
Docker socket or grant observation users action/Engine access. Do not copy observation grants/config/ledgers.

Initial stopped root approval explicitly confirms the fixed socket witness, random Engine generation and full64
container ID, then mints opaque target and independent START/STOP/RESTART entries with owner-fixed1..30s stop grace
(START null). Freeze the whole public entry set; later edits/replacement/re-enrollment are unsupported. Alias or
current socket stat alone is not approval. Before any HTTP claim, create the v1 journal under the BROKER identity
using the actual Provision API in a closed networkless transient provisioning mode of the verified setup binary.
Root-created/chowned SQLite state is not a substitute for its owner checks. Worker/broker normal startup remains
OpenExisting only; any existing/missing-after-install/corrupt/foreign state refuses instead of reprovisioning.

The setup binary accepts exactly `provision-journal` for this step, with the same manager environment guard as
`exchange`. It accepts no paths, binding arguments or input. The broker's existing protected policy/Engine loader
supplies the installation binding and verifies the broker identity. This command creates only the v1 journal in
the prepared private directory; it opens no listener or Engine connection and never removes retained files.
The stopped installer must invoke it inside the broker view before allowing normal service startup. The command
is not an installer or an acceptance receipt on its own.

The stopped installer prepares the private worker directory and empty lock. The verified setup binary's
`seed-enrollment` verb, run as the worker, reads only protected settings and writes the canonical UNENROLLED
record under that lock. It requires both credential records to be absent and publishes without replacement.
It consumes no input and makes no network request. An existing record or failed publication is not reset.

Runtime proposal: two disabled `Type=exec` services, Restart=no, no socket activation/inherited FDs; zero core dumps,
capability sets empty, NoNewPrivileges, fixed executable allowlist, protected RootDirectory views and restrictive
syscalls. Worker alone sees fixed first-party HTTPS origin, reviewed CA/DNS/host inputs, own credential directory
and worker IPC. Use the existing first-party origin/egress-resolution policy; no URL/proxy/CA CLI overrides. Broker
has AF_UNIX/PrivateNetwork only, approved Engine bind at /engine/docker.sock, policy/Engine witness and journal,
never credentials/IP egress. Both retain actual host PID/user namespace: PrivateUsers=false, no private PID/proc;
expose required read-only proc namespace handles/identity files, not a broad host filesystem. Initial proposed caps
per runtime: MemoryMax256M, TasksMax64, CPUQuota50%; no GOMAXPROCS/GOMEMLIMIT environment (current launch guard rejects
them). Exact unit/metadata/effective-probe closure is a pre-code gate, not template proof.

No automatic boot start in this first profile. Explicit root start, after stopped/joined validation, prepares ONE
read-only NSFS bind from trusted host /proc/1/ns/user to ADR031’s fixed host-userns path and binds it into both
views. Refuse a foreign mount/regular marker; services never mount/repair it. After reboot everything stays stopped
until explicit root preparation/start; no new persistent privileged preparation daemon is needed. Prove actual
Linux6.6 empty-path chmod capability, same namespace provenance and safe absent/foreign refusal on Ubuntu24.04.

Stop fences new work and stops worker then broker under ONE original root90s operation budget, not90s per service;
proposed worker/broker manager stop ceilings10s/80s leave the existing broker native75s join allowance intact.
Verify exact unit invocation/executable/cgroup and no remaining owners before cleanup/new start. Forced termination
means possible effect/UNKNOWN, never successful no-effect or an unlock. No process-name kills or broad cleanup.
Upgrade/rollback stays stopped, preserves compatible journal/mapping/handoff/credential/lane state and floors;
removal disables/retains evidence. Status is bounded non-secret metadata, not raw file/HTTP/Docker/log access.

## Proof, rollback and decisions before implementation

- Freeze exact lifecycle codec vectors (numeric revisions/Instant forms), handoff bytes/state/CAS, role matrix,
  artifact list/build identity and effective unit closure. Root/peer review precedes source changes.
- Local ordinary tests: wrong-purpose tokens/bindings; bootstrap replay/loss; prepare/activation/status ambiguity;
  fsync/rename/restart failures; no claims during handoff; expiry equality; no secret sinks; no POST retry; fixed
  package extra/missing/link/hash/role refusal; stopped-only installation and owned invocation cleanup.
- Existing observation outputs/contracts/resources remain byte-identical; source-only additions do not change its
  installed services. No new dependency, workflow tier, generic bootstrap framework or coverage exception.
- Exact merged clean Ubuntu24.04/amd64 artifacts: reproducibility, checksums/attestation and hostile-input verifier
  tests before separate build/VM approval. Green CI is not an installed activation receipt.
- A separately frozen private VM plan must use an ordinary-user2vCPU/2GiB disposable profile, exact source/images,
  owned cleanup and explicitly approved real Platform HTTPS/DNS connectivity. Existing root-only/no-NIC3GiB harness
  is not that profile and cannot be bypassed or silently repurposed. No VM/image/network operation is approved here.
- Installed tests must observe actual UID/GID (including saved IDs), capabilities, namespaces, mounts, ACLs/egress,
  socket access and denied observation/worker/broker cross-access, restart/reboot/stop/rollback and v1 storage bounds.
- Then actual paired owner/API/credential/worker/broker/Engine disposable proof: each independent operation, denials,
  lost claim/report, credential and consumer restart, expiry/new-key/native-alias locks and unrelated-target safety.
  Candidate/fallback image reader eligibility alone is insufficient; strict owner-source/credential/execution
  composition and operational floor must also be verified. Legacy execution auth stays rejecting.
- Server setup requires its own keyring/schema/role/current-owner source composition; typed execution also requires
  its default-OFF deployment capability and HOME_LAB/HOME_LAB_ACTIONS site features. The installer never changes
  server flags, keyring, lanes or policy. Retained/status reads and uncertainty semantics are not activation proof.

Recommended over alternatives: reuse the closed connected packaging mechanisms with an action-only profile and
consumed lifecycle integration. A one-process HTTPS+Docker package violates the accepted separation; copying the
observation installer verbatim confuses token purpose/roles; an exchange-only package without renewal/handoff is
not the accepted operable lifecycle. These are project-fit conclusions, not new architecture approval.

## Primary checks and fit

Checked2026-11-04 local /2026-11-05UTC; retain pinned Go1.27.3, x/sys0.48 and systemd255 first profile, no upgrades:

- [systemd255 execution](https://www.freedesktop.org/software/systemd/man/255/systemd.exec.html): private roots need
  auxiliary files/mounts; environment is not a secret channel. Use actual mount/role tests, not a template assertion.
- [systemd255 service source](https://raw.githubusercontent.com/systemd/systemd/v255/man/systemd.service.xml) and
  [kill behavior](https://www.freedesktop.org/software/systemd/man/255/systemd.kill.html): explicit restart policy and
  whole service termination matter. We deliberately choose Restart=no; killing a client proves no daemon rollback.
- [Go1.27.3 HTTP](https://pkg.go.dev/net/http@go1.27.3#Transport): transport replay has conditions; preserve fresh
  connections, nil GetBody, fixed routes and no mutation retry instead of assuming POST alone prevents replay.
- [Linux namespaces](https://man7.org/linux/man-pages/man7/namespaces.7.html): retained/bind-mounted namespace
  handles identify a namespace. Trusted host provisioning supplies provenance; an inode number alone does not.
- [OWASP secrets lifecycle](https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html):
  minimize disclosure and support rotation/revocation/expiry. It does not select our TTLs or justify Docker retry;
  those remain the existing server contract and ADR034’s lock semantics.
