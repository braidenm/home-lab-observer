# Runtime integration CI gates

The integration PR remains draft until native acceptance is complete. Its Native delivery workflow builds the v2
main/helper pairs from the exact checked-out GitHub SHA, packs the closed v2 manifest, and runs the existing bounded
archive/install/service smoke on Linux, Windows and macOS. These jobs do not opt in to reading machine logs and do
not publish releases. The separately protected manual release workflow remains unchanged in this slice.

A separate, parallel Ubuntu job runs the explicit two-build reproducibility script, with a 12-minute job cap. It is
change-filtered to runtime/build inputs and never repeated for each native smoke target. The test uses isolated caches
and conflicting ambient Go settings, checks exact output identities and helper digest pairing, and executes no target
binary. It does not upload its temporary outputs. Its first hosted result is required before claiming reproducibility.

Local full Go tests/vet, repository policy, 14 release tests, schema/contract tests and actual current-snapshot
validation passed before draft submission. Native Windows fixture acceptance and final packaged enabled-source
history/restart checks remain separate gates, not inferred from these results.
