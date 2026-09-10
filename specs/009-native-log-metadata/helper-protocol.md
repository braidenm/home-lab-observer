# Private native-helper protocol checkpoint

Status: Accepted bounded contract slice; no helper execution, native reads or pipe lifecycle is implemented here.

`internal/logprotocol` is a static, transport-only package shared by the Linux and Windows helper adapters. It imports
only standard-library code and `logobs`. Expected build identity is supplied by the trusted parent, never from a UI or
remote caller, and cannot select an executable, library, path, command or source outside the accepted presets.

## Exact envelopes

All listed keys are required, including false/zero values and explicit nulls. Unknown, differently cased, duplicate,
or missing keys are rejected at every object level. Each payload is exactly one UTF-8 JSON object followed only by
whitespace; a BOM, invalid UTF-8, trailing value, or nesting deeper than eight value levels is invalid. A bounded
16,384-token preflight (counting values and object keys) also limits malformed packet work before typed decoding.

The request has exactly `protocol`, `build`, `source`, `query_started_at`, and `checkpoint`:

- `protocol` is `observer-log-helper/v1`.
- `build` has exactly `version`, `commit`, `os`, `arch`. Version uses the current release prerelease grammar and is at
  most 64 ASCII bytes; commit is exactly 40 lowercase hex characters; OS is `linux` or `windows`; architecture is
  `amd64` or `arm64`. Every value must equal the trusted expected identity, not merely have a valid shape.
- `source` is `system`, or `application` only with Windows.
- `query_started_at` is canonical Go UTC RFC3339Nano text, including the final `Z` and a representable nonzero date.
- `checkpoint` has exactly `revision`, `reset_pending`, `opaque_base64`, `previous_attempt_at`, `coverage_through`.
  Absent cursor/timestamps are null. Its values construct and must pass a complete `logobs.ReadRequest` validation.

The response has exactly `protocol`, `build`, and `batch`. `batch` has exactly:

```text
kind, source, expected_revision, query_started_at, started_at, finished_at,
support_state, collection_state, reason_code, events, discards,
examined_count, probe_count, discarded_count, deferred, caught_up, next_opaque_base64
```

Each event has exactly `observed_at`, `source`, `severity`, `event_code`; each discard has exactly `at`, `count`.
Arrays are never null. Absent reason/cursor is null. Timestamps use the same canonical spelling as the request.
The reconstructed `logobs.Batch` must pass its native state, bounds, probe/count and reset validators. Response build,
source, expected revision and query time must match the original trusted request. No success/error side channel or
arbitrary native error string is included: expected acquisition outcomes are closed batches, and an invalid packet
is never returned as a usable batch. An ordinary batch cannot bypass a pending checkpoint; an initial cursorless
request cannot claim a stale-cursor reset. Reset-pending requests may establish or remain pending, never resume normal
work in the same attempt.

## Bounds and privacy

Request bytes are capped at 32 KiB and response bytes at 2 MiB before decode and after encode, including framing.
Decoding preserves integer precision with no float conversion; integer fields reject fractional/exponent forms and
are range checked by their typed/domain contracts. Per-array bounds are applied before constructing domain records.
Private cursors use canonical padded standard Base64: null means absent, and a non-null value must decode to 1..16384
bytes and re-encode identically. This rejects empty strings, embedded line breaks, alternate alphabets and noncanonical
padding bits. Bounds apply to decoded bytes as well as the encoded field.

Only explicit private wire structs encode cursors; public/domain JSON remains unchanged and omits them. Encoders and
decoders return fixed non-revealing error codes, never the input, cursor, parser error, source text or build text. All
returned private bytes are owned copies. Invalid input returns a zero domain value, not partially decoded state.

This checkpoint does not prove process safety: helper identity/digest and file ownership checks, hardening before input,
bounded stdin/stdout/stderr, deadlines, kill/reap and native acquisition remain the future process-adapter tests. No
filesystem, network, native loader or process operation belongs in this package.
