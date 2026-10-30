# Owner-operated Docker connected publication

Status (2026-10-30): `0.1.0-canary.3` is published as an immutable, non-latest prerelease after independent review,
protected builder/provenance/SBOM verification, both packaged cases across four boots, receipt-only PR154 and all
16 triggered CI checks. Anonymous downloads of all four public files matched the accepted bytes.
The [canonical receipt](../../docs/acceptance/connected-linux-docker/0.1.0-canary.3.json) records the accepted source;
this does not enable a site's catalog, enrollment/admission, owner-host collection or sharing.
The code-owned workflow registration is `401377430`, independently read after PR148 merged.
No CLI workflow override, generic profile selector or numeric-policy fallback is supported.
The exact [receipt and publisher contract](docker-promotion.md) owns this operation.

Use native Python3.12+, Git and the owner's existing native GitHub CLI stored login on a trusted workstation.
Windows uses Windows Python/GitHub CLI, not copied credentials or a WSL login. Run Python with `-B`.
No command requests or prints a GitHub token. Ambient tokens/configuration/debug overrides are excluded.
The account's stored release authority and workstation remain trusted; this program is not their sandbox.

## Prepare only after the protected candidate succeeds

Read both upload artifact IDs from the fixed candidate workflow's primary and independent jobs.
These illustrative IDs are synthetic; the program verifies their provider records, names, current run/attempt,
exact source and bytes. Use a new directory, not a failed prior output:

Both external artifact envelopes, the latest successful run/attempt, all five successful jobs and the exact fixed
primary/independent names are checked before either ZIP download. A partial rerun is not a successful candidate.

```text
python -B scripts/connected_docker_promote.py --prepare --run-id 123456789 --artifact-id 987654321 --independent-artifact-id 987654322 --output docker-candidate-check
```

Preparation reads APIs and writes only that new local directory. It does not publish, install, enroll or run assets.
It checks both ZIPs and literal five-file equality, provenance for all five retained subjects, and the archive's
signed SPDX predicate including four-role/core-module coverage. No separate raw SBOM file is required or trusted.
The private output holds the primary `payload/` and public `candidate-identity.json`; partial outputs are not success.

Run the separately reviewed full `rich-docker` two-case first-boot/reboot VM proof against these exact public bytes,
with same-source numeric/process baselines and pinned image/receiver/probe/twelve-file harness/Docker fixtures.
Retain private evidence and verify owned-process/mount/loop cleanup. Commit only the canonical
`docs/acceptance/connected-linux-docker/<version>.json` through an independent acceptance review.
The receipt is reviewed evidence, not cryptographic VM attestation or an activation credential.

## Publish unchanged bytes

After the acceptance addition merges, update the trusted checkout to exact clean main, then:

```text
python -B scripts/connected_docker_promote.py --publish --version 0.1.0-canary.3
```

The version is illustrative, not an existing release claim. Only that one regular acceptance-file addition may
separate candidate source from current main. Any other delta requires a fresh candidate and matching VM proof.
Both artifact identities and expiry, latest attempt and all five jobs are rechecked before every mutation.
Draft bytes are independently downloaded; publication is immutable/non-latest prerelease with anonymous byte proof.
Wrong/stale/expired bindings refuse. No overwrite/delete/clobber or speculative repair occurs; partial provider
state remains. Retry only the same version from still-valid trusted main, never reuse another profile's release.
The global version lock is shared with numeric publication because both use the same `v<version>` tag namespace.

Publication does not activate the Platform catalog, feature intent, owner approval or sharing. Docker controls,
Kubernetes, unattended lifecycle and upgrades/removal remain separate. No candidate dispatch is part of PR CI.
