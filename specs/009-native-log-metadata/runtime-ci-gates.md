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

The first actual scratch proof passed at PR 30 head `67ff165`, in hosted job `102779980756` on 2026-09-10. The probe
took four seconds and the complete cross-build job took 3m7s. Earlier head `05fc316` also passed the isolated paired
reproducibility gate and all three native archive/installer smoke targets. These are evidence for those exact heads,
not a substitute for rerunning CI after the independently identified HTTP shutdown fix.

At head `7f5a1e3`, all 14 CI checks passed. Native delivery run `34451723217`, cross-build job `102788742772`,
executed the extended packaged scratch proof and reported `MISSING_RUNTIME_PROOF_PASSED`: one synthetic durable
event survives graceful restart and forced termination, while the new session ring stays empty and the unavailable
native source remains explicit. The proof took about four seconds; the cross-build job took 2m50s. Paired-build
run `34451723117` passed in 4m56s, and all three native archive/installer smoke targets passed at this same head.
This is actual packaged persistence/restart evidence, not successful native journal acquisition.

Local full Go tests/vet, repository policy, release tests, schema/contract tests and actual current-snapshot
validation passed before draft submission. Native Windows fixture acceptance subsequently passed on both architectures
and merged in PR #31; see [owned Windows evidence](windows-native-fixture.md). The manual
release workflow still needs a separately reviewed v2 promotion before preview publication.
