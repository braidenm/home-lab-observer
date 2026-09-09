# Spec 005 traceability

| Requirement | Contract evidence | Fixture/test evidence |
| --- | --- | --- |
| R4 | Allowlisted GET contract and closed series schema | valid full/unsupported and invalid arbitrary-query fixtures; exact query assertions |
| R4 | UTC timestamps, nullable points, and `sample_interval_seconds` | `metric-series-6h.json`; AJV plus semantic fixture checks |
| R4 | Six metric IDs, 2,048 points per series, 1 MiB response | OpenAPI/schema maximum assertions and fixture byte checks |
| R5 | Series-facing history excludes process identity and log bodies | explicit privacy constants and prohibited-key scan; physical SQLite retention tests |
| R10 | Transport-neutral quality/gap fields and exact local adapter | real-fixture adapter tests and null preservation assertions |
| R12 | Existing contract workflow is bounded to five minutes | root `npm run test:contracts` exercises OpenAPI, AJV, fixtures, semantics, privacy, and traceability |

| R1, R7, R8 | Loopback listener, account-protected token, strict local boundary | `internal/localauth`, `internal/localapi` security tests; unsafe CLI bind tests |
| R2, R11 | Single-flight collector and graceful ownership lifecycle | `internal/scheduler` tests; `cmd/observer/serve_test.go` drain/reopen integration |
| R3 | Current/capabilities/Problem contract and bounded query envelopes | `internal/localapi` tests; actual-binary AJV smoke on three native CI hosts |
| R5, R6 | Schema migration, WAL reclamation, process lock, atomic reads, quarantine | `internal/history` restart, retention, corruption, lock and read-transaction tests |
| R9, R10 | Embedded same-origin dashboard and token gate | embedded UI tests, deterministic bundle check, packaged browser smoke |
| R12 | Parallel GitHub-hosted native and UI jobs | `.github/workflows/runtime.yml`, `dashboard.yml`, `contracts.yml` |
