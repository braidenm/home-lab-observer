# Spec 005 series-contract traceability

| Requirement | Contract evidence | Fixture/test evidence |
| --- | --- | --- |
| R4 | Allowlisted GET contract and closed series schema | valid full/unsupported and invalid arbitrary-query fixtures; exact query assertions |
| R4 | UTC timestamps, nullable points, and `sample_interval_seconds` | `metric-series-6h.json`; AJV plus semantic fixture checks |
| R4 | Six metric IDs, 2,048 points per series, 1 MiB response | OpenAPI/schema maximum assertions and fixture byte checks |
| R5 | Series-facing history excludes process identity and log bodies; SQLite/WAL/retention evidence remains deferred | explicit privacy constants and prohibited-key scan |
| R10 | Transport-neutral quality/gap fields and exact local adapter | real-fixture adapter tests and null preservation assertions |
| R12 | Existing contract workflow is bounded to five minutes | root `npm run test:contracts` exercises OpenAPI, AJV, fixtures, semantics, privacy, and traceability |

This focused matrix covers the series-contract subtask only. The remaining Spec 005 requirements retain their planned
runtime, store, listener, embedded-build, and browser evidence in later subtasks.
