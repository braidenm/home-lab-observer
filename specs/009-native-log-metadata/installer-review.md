# Paired installer review

Root independently reviewed the static manifest verifier, code-owned schema selection, online checksum coverage,
offline v2 requirements, complete pair staging, interruption cleanup, and retained v1 rollback behavior.

Review found that non-Linux v2 binaries could otherwise accept an old-schema manifest with matching identity. The
verifier now requires the schema derived from its embedded identity envelope; the caller cannot choose that policy.
Tests include actual built native observer verification, fixed errors, manifest downgrade rejection, helper tampering,
legacy installer-hook refusal, and four-file v2 packages refusing a missing manifest.

Root verification passed: full Go tests and vet, repository policy, Windows PowerShell 5.1 synthetic installer
lifecycle tests, and the Bash lifecycle suite under Ubuntu WSL. The Bash suite exercised interrupted helper copy,
whole-version selection, rollback and uninstall using only owned temporary fixtures. No observer helper, host log
query, real service registration or production installation was performed. macOS execution remains a required CI
gate; actual final v2 release bundles and publication remain separate delivery gates.
