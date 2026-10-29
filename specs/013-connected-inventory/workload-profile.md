# First executable workload profile

Status: implementation contract only; no receiver admission, worker, installed policy,
upload or control activation. Existing numeric/native v1 bytes remain unchanged.

## Closed document and consent

`home-lab-workload-inventory/v1`, profile `process-container/v1`, is a separate
latest-only document. Its source is the enrolled `srv_` identity, never a hostname,
connector ID or a local observation source. Collector version is verified release
metadata. The host snapshot supplies the UTC cycle timestamp; each enabled section
retains its own observation timestamp. Neither timestamp asserts freshness at receipt.

The pure encoder accepts normalized `observation.Snapshot`, `containerobs.Inventory`,
release identity, and explicit `Consent{Processes, Containers}`. Both consent fields
default false. Disabled groups suppress retained input completely, including counts,
timestamps, invalid values and names. Enabling one group never enables the other.
This argument is not persisted consent or authorization: installed local policy and
the receiver must independently enforce the agreed profile before future activation.

The document advertises `SENSITIVE` and `read_only=true`. Enabled process basenames
and container name/image references remain sensitive, not anonymous. Text normalization
removes Unicode formatting characters (Cf), normalizes control characters (Cc),
bounds UTF-8 bytes, strips process paths and replaces common secret,
credential-URL, email and private-address patterns. Known project enrollment/connector
tokens (`hle_`/`hlc_` plus 43 base64url characters) and common three-segment `eyJ` JWT
shapes are filtered in every display field, and rejected by the producer-byte validator.
This is pattern-based defense in depth, not a
guarantee that arbitrary names contain no confidential meaning. No raw local source,
account, executable path, argument, environment, socket, inspect body or error text is
projected, and there are no dedicated/raw network-address fields. Opted-in names and
image references can still contain addresses or confidential text not recognized by
the defensive patterns. No action, target authority or grant is present.

## Exact bounds and quality

- Document: at most 262144 encoded UTF-8 bytes. Each section: at most 130560
  encoded bytes, reserving 1024 bytes for the closed envelope. These are future
  profile budgets, **not** changes to the installed 16384-byte handoff or transport.
- Output: at most 200 process and 500 container rows. Encoder input is additionally
  bounded to 4096 rows per enabled group before per-row work. Eligible rows, including
  those later omitted by output caps, must have distinct valid display identities.
- Process order: resident bytes descending, then CPU descending, then PID ascending.
  Container order: display alias ascending. Row and byte limits retain a deterministic
  prefix and set `truncated=true`, `DEGRADED/PARTIAL_COLLECTION`.
- Text limits in UTF-8 bytes: process/container name 128, image display reference 240.
  Reject malformed UTF-8 or input strings over 1024 bytes. Truncate on code-point
  boundaries after redaction. Validators/schema reject all Cc/Cf characters, including
  bidi controls and zero-width formatting. JSON escaping is included in section/document budgets.
- Numeric integers are nonnegative and at most `2^53-1`; PID is 1..2147483647.
  Each section declares a fixed `cpu_measurement_basis`. Processes use
  `LIFETIME_SINGLE_CORE`: pinned gopsutil computes total CPU time / process age *100,
  not a recent sample or a fraction of whole-host capacity. Docker uses
  `ENGINE_HOST_CAPACITY`: valid CPU/system counter deltas *100. Do not compare these
  as equivalent percentages or infer `top` parity. The existing process normalizer
  rejects values above 100 although multicore processes can exceed 100 on this basis;
  therefore busy processes can be omitted and coverage degraded. The new contract
  correctly admits finite process CPU 0..409600 (a bounded 4096-logical-core maximum),
  allowing that acquisition bug to be fixed without another wire version. Container
  CPU remains 0..100. No value is silently renormalized. Changing measurement basis,
  unlike correcting acquisition within this basis, requires an explicit contract update.
- Sections expose closed quality/reason, nullable observation time and total,
  returned count, truncation, and rows. Available healthy-empty has known total zero;
  disabled/unknown/failure has null total and no rows. Degraded process coverage has
  unknown total; Docker's successfully enumerated list retains its known total even
  when individual stats fail. Output-only truncation preserves a previously known total.
- Process state is normalized into running/sleeping/stopped/zombie/idle/waiting/unknown.
  CPU and RSS come from successfully normalized rows. Elapsed seconds are nullable:
  current local normalization conflates an unknown start time with zero age, so zero
  becomes null rather than a fabricated measurement; positive elapsed values survive.
- Container state retains the existing closed Docker enum. CPU/memory are nullable,
  with AVAILABLE/PARTIAL/UNAVAILABLE/NOT_RUNNING and closed metric reasons. Non-running
  containers have null live metrics. Missing health, restarts, uptime, network and
  block I/O are omitted from this first contract, not invented from legacy requirements.
- Invalid eligible identities, duplicate identities, inconsistent counts/metric states,
  nonfinite/out-of-range numbers or invalid envelopes reject the document with fixed
  errors and no rejected values. Unknown local failure text maps to fixed reasons.

The code-owned schema rejects additional fields. The canonical producer validator
also rejects duplicates, alternate encodings/order, malformed UTF-8, oversized sections
and source mismatch. Validation is neither authorization nor freshness verification.

## Proof and next boundary

TDD covers byte-identical shared fixtures, consent suppression, privacy canaries,
quality and empty/stopped/partial states, duplicate IDs, numeric/text/envelope bounds,
deterministic row/byte truncation, canonical rejection, and old upload-admission refusal.
Node/Ajv validates the same fixtures and negative schema mutations. A real JVM parser,
owner/delegate HTTP journey, installed isolation and profile-changing migration remain
separate mandatory gates. The existing workload adapters are reused without collecting
host data in these tests. No dependency, listener, persistence or installed authority
is added by this slice.
