$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repositoryRoot = Split-Path -Parent $PSScriptRoot
$requiredFiles = @(
    'AGENTS.md',
    'README.md',
    'SECURITY.md',
    'CONTRIBUTING.md',
    'SUPPORT.md',
    'LICENSE',
    '.specify/memory/constitution.md',
    'docs/coding-standards.md',
    'docs/security/threat-model.md',
    'docs/privacy/data-policy.md',
    'docs/architecture-research/001-cross-platform-observer.md',
    'docs/adr/README.md',
    'docs/adr/001-single-go-binary.md',
    'docs/adr/002-api-first-local-dashboard.md',
    'docs/adr/003-bounded-observations-and-logs.md',
    'docs/adr/004-independent-release-supply-chain.md',
    'specs/001-repository-foundation/spec.md',
    'specs/001-repository-foundation/plan.md',
    'specs/001-repository-foundation/tasks.md',
    'specs/002-cross-platform-observer/spec.md',
    'specs/002-cross-platform-observer/plan.md',
    'specs/002-cross-platform-observer/tasks.md',
    'specs/002-cross-platform-observer/traceability.md',
    'specs/003-native-host-snapshot-preview/spec.md',
    'specs/003-native-host-snapshot-preview/plan.md',
    'specs/003-native-host-snapshot-preview/tasks.md',
    'specs/005-local-data-plane/traceability.md',
    'api/openapi.v1.json',
    'schemas/v1/capabilities-v1.schema.json',
    'schemas/v1/current-snapshot-v1.schema.json',
    'schemas/v1/metric-series-v1.schema.json',
    'schemas/v1/problem-details-v1.schema.json',
    'schemas/v1/fixtures/manifest.json',
    'scripts/validate-contracts.mjs',
    'package.json',
    'package-lock.json',
    '.github/workflows/contracts.yml'
)

$missingFiles = @(
    foreach ($relativePath in $requiredFiles) {
        $absolutePath = Join-Path $repositoryRoot $relativePath
        if (-not (Test-Path -LiteralPath $absolutePath -PathType Leaf)) {
            $relativePath
        }
        elseif ((Get-Item -LiteralPath $absolutePath).Length -eq 0) {
            "$relativePath (empty)"
        }
    }
)

if ($missingFiles.Count -gt 0) {
    throw "Missing or empty required files:`n$($missingFiles -join "`n")"
}

$trackedFiles = & git -C $repositoryRoot ls-files
if ($LASTEXITCODE -ne 0) {
    throw 'Unable to enumerate tracked files.'
}

$forbiddenPaths = @(
    '.env',
    'id_rsa',
    'id_ed25519',
    '*.p12',
    '*.pfx',
    '*.key'
)

foreach ($pattern in $forbiddenPaths) {
    if ($trackedFiles | Where-Object { $_ -like $pattern -or $_ -like "*/$pattern" }) {
        throw "A prohibited secret-bearing path is tracked: $pattern"
    }
}

$textFiles = $trackedFiles | Where-Object {
    $_ -match '\.(md|ya?ml|json|go|ts|tsx|js|ps1|sh)$' -and
    $_ -ne 'scripts/check-repository.ps1'
}
$legacyName = 'platform-demo-' + 'home-lab-observer'
foreach ($relativePath in $textFiles) {
    $absolutePath = Join-Path $repositoryRoot $relativePath
    $content = Get-Content -LiteralPath $absolutePath -Raw
    if ($content -match [regex]::Escape($legacyName)) {
        throw "Legacy project name found in $relativePath"
    }
    if ($content -match '(?i)(ghp_|github_pat_|-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----)') {
        throw "Potential credential material found in $relativePath"
    }
}

Write-Output "Repository foundation validation passed ($($requiredFiles.Count) required files)."
