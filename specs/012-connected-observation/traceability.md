# Slice A evidence

Local validation on 2026-09-10 (Windows amd64, Go 1.27.1, Node 22):

| Requirement | Evidence |
| --- | --- |
| R1, R4, R8 | Package has no runtime caller; explicit private DTO; `TestExcludedLocalTextNeverCrossesBoundary` |
| R2, R3 | `TestFixtureMatchesIndependentSchemaFixture`, `TestEnvelopeAndOS`; strict producer schema |
| R5 | `TestUnavailableDoesNotExposeEvenMalformedRetainedData`, `TestIncompleteOverview` |
| R6 | `TestInvalidEligibleDataReturnsNoBytes`, `TestEnvelopeAndOS`, `TestMaximumBoundAndDeterminism` |
| R7 | Full `go test ./...`, `go vet ./...`, `npm run test:contracts` (33 tests), repository policy, `git diff --check` |

Focused encoder statement coverage: 96.3%; encoding failure/oversize defense remains structurally unreachable for valid
bounded DTO values. Cross-platform required CI and independent implementation review are pending. Schema validation
does not replace encoder checks for used-versus-total relationships. Actual receiver acceptance is a separate test
slice in Platform Demo; no installed connector behavior changes before those and subsequent activation gates pass.
