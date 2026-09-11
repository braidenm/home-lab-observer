# Slice A evidence

Local validation on 2026-09-10 (Windows amd64, Go 1.27.1, Node 22):

| Requirement | Evidence |
| --- | --- |
| R1, R4, R8 | Package has no runtime caller; explicit private DTO; `TestExcludedLocalTextNeverCrossesBoundary` |
| R2, R3 | `TestFixtureMatchesIndependentSchemaFixture`, `TestEnvelopeAndOS`; strict producer schema |
| R5 | `TestUnavailableDoesNotExposeEvenMalformedRetainedData`, `TestIncompleteOverview` |
| R6 | `TestInvalidEligibleDataReturnsNoBytes`, `TestEnvelopeAndOS`, `TestMaximumBoundAndDeterminism` |
| R7 | Full `go test ./...`, `go vet ./...`, `npm run test:contracts` (34 tests), repository policy, `git diff --check` |

Focused encoder statement coverage: 96.4%; encoding failure/oversize defense remains structurally unreachable for valid
bounded DTO values. Cross-platform required CI and independent implementation review are pending. Schema validation
does not replace encoder checks for used-versus-total relationships. Actual receiver acceptance is a separate test
slice in Platform Demo; no installed connector behavior changes before those and subsequent activation gates pass.

Independent review found that CPU/memory/uptime error/truncation flags also need to suppress the overview, not just
filesystem flags. Fixed with six additional cases; unrelated process/network failures do not suppress valid host data.
Cross-repository audit corrected the fixture from connector-instance identity to exchanged `srv_` server identity;
both producer code and schema now reject an `agent_` instance as a source. Runtime activation remains absent.
