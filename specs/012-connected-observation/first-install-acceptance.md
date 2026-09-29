# Connected First-Install Acceptance Record

Date: 2026-09-18

## Accepted scope

The exact package built from commit `8dd7eeb79625076e70966431ffb1ad98dd29b47b` completed the manually invoked,
no-NIC Ubuntu 24.04/systemd 255 disposable-VM matrix. This accepts only a first connected Linux install that remains
disabled and inactive. It does not accept worker activation, a production endpoint canary, refresh, rollback,
uninstall, Docker actions, or an owner-host installation.

An independent security review found no blocking issue in the final harness or fixed-output evidence boundary. The
latest exact head also passed repository policy, contracts, dashboard, Go runtime, native reproducibility, Linux and
Windows owned fixtures, cross-build, dependency review, native smoke, and vulnerability scanning on GitHub-hosted
runners.

## Independently pinned inputs

| Input | SHA-256 |
| --- | --- |
| Canonical Ubuntu 24.04 server cloud image | `612b2c0cc1bc413a6cb8c38fd611794caf0f2b436c50013d8b3794db12ad7354` |
| Connected manifest | `dffb054afef6c78d715536f10319b77d51ee8fa048a1db077579b1e8bc818d86` |
| Connected archive | `4489f4e842b76ee83ba1b906654a709bf9e3f8844385d3014c64f11ba73d5c38` |
| Synthetic receiver | `8686f8ab9370435619de5c2b134f4488945ef5e0ccb98aea7bf0f4441fc3e5a3` |
| Exact production-policy probe | `068244895912f0869bc56621bd73ab6c7a6447d119fe1de3f844a110d76aa34f` |

The historical `8dd7eeb` run did not independently bind the harness sources, so it is not by itself sufficient for the
current release gate. The current reviewed five-file canonical harness manifest is
`c568a9ba7979202b3dcb598c400adc049d11d9bd1debf438fd42d36b13de3fa8`; an exact-head replay must supply that digest
through `--expected-harness-sha256` and record the resulting fixed markers in the pull request before merge.

## Case results

| Case | Fixed evidence | Result |
| --- | --- | --- |
| Successful stopped install and reboot | `HLO_VM_SUCCESS_REBOOT_PASS` | Pass |
| Power cut after durable PREPARING marker and reboot | `HLO_VM_POWER_CUT_READY`, `HLO_VM_INTERRUPT_REBOOT_PASS` | Pass |
| Power cut after successful stopped install and reboot | `HLO_VM_SUCCESS_READY_FOR_CUT`, `HLO_VM_SUCCESS_REBOOT_PASS` | Pass |

Each guest had no NIC or host share. The harness proved exact bundle/config/unit identity, closed ownership/mode/ACL
roles, disabled and inactive services, ledger/config/credential identity across reboot, refusal on retry, and no running
worker process at the fixture's observation points. This bounded evidence does not establish historical non-activation
between observations. Only fixed allowlisted markers were promoted into this record; grants, credentials, raw console content,
host paths, and infrastructure identifiers are excluded.

## Residual gates

- Activation requires separate installed-runtime egress, credential, handoff, shutdown, upload, and revocation proof.
- The synthetic receiver accepts enrollment only; it does not prove sustained production snapshot ingestion.
- Hardware flush behavior outside the tested ext4/QEMU boundaries remains outside this claim.
- Owner-host installation, release publication, live registration, rollback, refresh, and uninstall remain unexecuted.
