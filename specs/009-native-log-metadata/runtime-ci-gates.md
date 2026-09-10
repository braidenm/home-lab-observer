# Runtime integration CI gates

The integration PR remains draft until native acceptance is complete. Its Native delivery workflow builds the v2
main/helper pairs from the exact checked-out GitHub SHA, packs the closed v2 manifest, and runs the existing bounded
archive/install/service smoke on Linux, Windows and macOS. These jobs do not opt in to reading machine logs and do
not publish releases. The separately protected manual release workflow remains unchanged in this slice.

A separate, parallel Ubuntu job runs the explicit two-build reproducibility script, with a 12-minute job cap. It is
change-filtered to runtime/build inputs and never repeated for each native smoke target. The test uses isolated caches
and conflicting ambient Go settings, checks exact output identities and helper digest pairing, and executes no target
binary. It does not upload its temporary outputs. Its first hosted result is required before claiming reproducibility.

After creating the actual v2 archives, Linux delivery runs the missing-runtime probe before finalization adds extra
assets. The probe verifies the closed release set and runs the packaged main/helper in a networkless scratch image,
without host mounts or a Docker socket inside the container. It requires an attempted unavailable log source while
the authenticated dashboard API remains usable, then verifies graceful shutdown. Only fixed pass/fail output leaves
the fixture. This is not evidence of successful journal acquisition; the owned native fixture covers that separately.

Local full Go tests/vet, repository policy, 14 release tests, schema/contract tests and actual current-snapshot
validation passed before draft submission. Native Windows fixture acceptance and final packaged enabled-source
history/restart checks remain separate gates, not inferred from these results.
