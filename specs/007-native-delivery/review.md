# Native delivery acceptance and review

## Scope and traceability

| Requirement | Implementation / verification |
| --- | --- |
| D1: six bounded archives | `internal/releasepack`, deterministic rooted ZIP/USTAR fixtures, native release matrix |
| D2: closed manifest and final bytes | `schemas/release-v1.schema.json`, adversarial AJV tests, release finalizer, final-byte checksums and provenance |
| D3: independent publication | Manual main-only release workflow; public hosted native checks; immutable public repository releases |
| D4: verified installation | Bash and PowerShell helpers; checksum, archive allowlist, private staging and explicit per-user roots |
| D5: lifecycle safety | Managed version directories, atomic selection, retained rollback, uninstall preserving external state |
| D6: version and direct run | `cmd/observer/version_test.go`, injected foreground startup tests, actual versioned native binaries |
| D7: preview trust | ADR 004/007, README, install guide and archive instructions explicitly describe unsigned previews |
| D8: review and release proof | Independent cross-slice review, local and hosted installer/native tests, anonymous publication verification |

## Review findings addressed before publication

- Installation lock cleanup must only remove locks owned by the current process; a failed second installer must not
  release the first installer's lock.
- Enforce expansion limits before extraction, not after disk space has already been consumed.
- Use a nonexisting release output directory, matching the packager's refusal to overwrite existing artifacts.
- Inspect Windows ZIPs with a ZIP-capable reader on Linux; GNU tar does not provide this capability.
- Do not hide duplicate archive members when validating the archive allowlist.
- The owner verifies release immutability before dispatch. GitHub's settings endpoint requires administrator-read
  permission, which is intentionally not added to CI. Require explicit dispatcher confirmation, verify uploaded draft
  bytes before publishing, and require the public release API to report `immutable: true` before reporting success.
- Reuse the native delivery cross-build as the existing required `cross-build` check instead of compiling six targets
  twice in two workflows.
- Require both the original dispatcher and rerun actor to be the repository owner. Create the release tag atomically
  against the tested commit and recheck its exact target before and after immutable publication.
- Pin the SBOM generator version as well as its enclosing action; an action SHA alone does not pin downloaded tools.

## Evidence recorded so far

- Full local Go tests and static analysis pass, including version/direct-run behavior and releasepack tests.
- Current local dependency vulnerability scan reports no known reachable vulnerabilities.
- Existing API contract suite: five schemas, 12 valid and 10 invalid fixtures. The release schema has separate
  positive SemVer and adversarial field/platform/version/URL/size tests.
- Six real native binaries built locally with explicit version/source metadata and packaged successfully.
- The actual Windows archive binary passes authenticated API, history, JSON Schema and packaged browser tests at
  390, 768 and 1440 CSS pixels. `OBSERVER_SMOKE_BINARY` lets the existing smoke suite test an extracted artifact without
  rebuilding it; production startup behavior is unaffected.
- Independent reviewer `api_local_ui` approved the releasepack, CLI/version, install documentation and packaged UI
  boundary after targeted Go tests, AJV tests, actual six-archive manifest inspection and mobile screenshot review.
  Review confirmed fixed archive members, private/non-overlapping staging, bounded regular reads, truthful trust
  disclosures and no unexpected startup/privilege/Docker changes. Installer review is recorded separately.
- Windows PowerShell 5.1 installer lifecycle/security tests pass, including a real versioned Windows archive install,
  identity check, managed launch and removal. Bash scripts pass syntax checks; Linux/macOS runtime coverage is gated
  by their native CI jobs, not inferred from Windows results.
- Installer independent review covered ancestor-junction containment, lock ownership, interrupted-install recovery,
  private-stage cleanup and marker-only partial uninstall. Findings were revised before integration.
- Hosted installer results, independent final reviews and first-release download verification are recorded below
  when those checks complete. These pending checks are not represented as passed evidence.

## Explicit limits

This specification does not register background services, configure remote upload, enable container access, install
Docker, or change system-wide security policy. It does not make the private infrastructure repository or legacy
package public. Windows/macOS publisher signing and notarization remain a GA gate, not a claim made by this preview.
