# Tasks: Native host snapshot preview

- [x] Accept the bounded preview specification.
- [x] Preserve schema-required safe filesystem, process, and truncation metadata.
- [x] Implement the exact current-snapshot v1 projection.
- [x] Emit honest empty unsupported sections.
- [x] Add deterministic fixture lockstep and contract validation.
- [x] Add pinned, GitHub-hosted cross-platform runtime CI.
- [x] Harden projection caps, totals, sanitization, and projected collection quality.
- [x] Preserve categorized process scan failures and distinguish unsupported and permission-denied results.
- [x] Validate a real native snapshot through AJV on Windows, macOS, and Linux.
- [x] Document and test CLI help, usage, and failed-snapshot exit semantics.
- [x] Add HTTP, storage and the embedded UI in [Spec 005](../005-local-data-plane/tasks.md).
- [x] Add optional containers in [Spec 006](../006-container-observations/tasks.md).
- [ ] Add OS services and opt-in logs only in later accepted specifications.
- [x] Cap native provider detail acquisition before process construction, and verify cancellation/failure accounting.
- [x] Independently review cutoff/cancellation behavior; local Go 1.27.3 Windows full suite and focused vet pass.
- [x] Prove scan-budget truncation stays partial through the existing snapshot projection; PR 128 native CI passed before merge.
- [ ] Preserve legitimate multicore process CPU values internally, declare lifetime/single-core basis and retain v1 projection compatibility.

CPU-range correction evidence (2026-10-29): regressions failed before implementation; local Windows Go 1.27.3 full
suite and focused vet pass. Independent review found no P0-P2 issues. Exact-head native/race CI remains required;
this source correction has not been packaged or installed on a connected host.
