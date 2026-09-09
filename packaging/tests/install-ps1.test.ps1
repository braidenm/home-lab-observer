$ErrorActionPreference = 'Stop'
$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$Installer = Join-Path $RepositoryRoot 'packaging\install.ps1'
$WindowsPowerShell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
$MachineArchitecture = if ([string]::IsNullOrWhiteSpace($env:PROCESSOR_ARCHITEW6432)) { $env:PROCESSOR_ARCHITECTURE } else { $env:PROCESSOR_ARCHITEW6432 }
$Architecture = if ($MachineArchitecture -eq 'ARM64') { 'arm64' } else { 'amd64' }
$Temporary = Join-Path ([IO.Path]::GetTempPath()) "observer installer test $([Guid]::NewGuid().ToString('N'))"
[IO.Directory]::CreateDirectory($Temporary) | Out-Null

function Assert-True([bool] $Condition, [string] $Message) { if (-not $Condition) { throw $Message } }

function Invoke-Installer([string[]] $Arguments, [bool] $ExpectSuccess = $true) {
    $savedPreference = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $output = & $WindowsPowerShell -NoProfile -NonInteractive -ExecutionPolicy Bypass -File $Installer @Arguments 2>&1 | Out-String
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $savedPreference
    }
    $succeeded = $exitCode -eq 0
    if ($succeeded -ne $ExpectSuccess) { throw "installer outcome mismatch ($exitCode): $output" }
    return $output
}

function New-TestBinary([string] $Path, [string] $Version) {
    $source = @"
using System;
public static class ObserverFixture {
    public static int Main(string[] args) {
        if (args.Length > 1 && args[0] == "version" && args[1] == "--json") {
            Console.WriteLine("{\"schema_version\":\"observer-build/v1\",\"version\":\"$Version\",\"commit\":\"0123456789abcdef0123456789abcdef01234567\",\"os\":\"windows\",\"arch\":\"$Architecture\",\"go_version\":\"go1.27.1\"}");
            return 0;
        }
        if (args.Length > 0 && args[0] == "version") {
            Console.WriteLine("Home Lab Observer $Version (windows/$Architecture, commit 0123456789abcdef0123456789abcdef01234567)");
            return 0;
        }
        Console.WriteLine("${Version}:" + String.Join(" ", args));
        return 0;
    }
}
"@
    Add-Type -TypeDefinition $source -Language CSharp -OutputAssembly $Path -OutputType ConsoleApplication
}

function New-Archive([string] $Version, [string] $IdentityVersion = $Version, [string] $Unsafe = 'safe') {
    $rootName = "home-lab-observer_${Version}_windows_${Architecture}"
    $build = Join-Path $Temporary "build-$Version-$IdentityVersion-$Unsafe"
    $root = Join-Path $build $rootName
    [IO.Directory]::CreateDirectory($root) | Out-Null
    New-TestBinary (Join-Path $root 'observer.exe') $IdentityVersion
    [IO.File]::Copy((Join-Path $RepositoryRoot 'packaging\resources\LICENSE'), (Join-Path $root 'LICENSE'))
    [IO.File]::Copy((Join-Path $RepositoryRoot 'packaging\resources\START-HERE.md'), (Join-Path $root 'START-HERE.md'))
    [IO.File]::Copy((Join-Path $RepositoryRoot 'packaging\resources\Run-Observer.cmd'), (Join-Path $root 'Run-Observer.cmd'))
    Add-Type -AssemblyName System.IO.Compression
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $archive = Join-Path $Temporary "$rootName-$IdentityVersion-$Unsafe.zip"
    $stream = New-Object IO.FileStream($archive, [IO.FileMode]::CreateNew)
    $zip = New-Object IO.Compression.ZipArchive($stream, [IO.Compression.ZipArchiveMode]::Create)
    try {
        $zip.CreateEntry("$rootName/") | Out-Null
        foreach ($name in @('observer.exe', 'LICENSE', 'START-HERE.md', 'Run-Observer.cmd')) {
            $entry = $zip.CreateEntry("$rootName/$name", [IO.Compression.CompressionLevel]::Optimal)
            if ($Unsafe -eq 'link' -and $name -eq 'START-HERE.md') {
                $entry.ExternalAttributes = [BitConverter]::ToInt32([byte[]] @(0, 0, 255, 161), 0)
            }
            $input = [IO.File]::OpenRead((Join-Path $root $name))
            $output = $entry.Open()
            try { $input.CopyTo($output) } finally { $output.Dispose(); $input.Dispose() }
        }
        if ($Unsafe -eq 'extra') {
            $entry = $zip.CreateEntry("$rootName/secret.txt")
            $writer = New-Object IO.StreamWriter($entry.Open())
            try { $writer.Write('unexpected') } finally { $writer.Dispose() }
        }
    } finally { $zip.Dispose(); $stream.Dispose() }
    return $archive
}

function Remove-TestTree([string] $Path) {
    for ($attempt = 0; $attempt -lt 20 -and (Test-Path -LiteralPath $Path); $attempt += 1) {
        try { Remove-Item -LiteralPath $Path -Recurse -Force -ErrorAction Stop } catch { Start-Sleep -Milliseconds 100 }
    }
}

try {
    $InstallRoot = Join-Path $Temporary 'install root with spaces'
    $StateRoot = Join-Path $Temporary 'state kept outside programs'
    [IO.Directory]::CreateDirectory($StateRoot) | Out-Null
    [IO.File]::WriteAllText((Join-Path $StateRoot 'sentinel'), 'keep-token-and-history')

    $V1 = '0.1.0-preview.1'
    $Archive1 = New-Archive $V1
    $Checksum1 = (Get-FileHash -LiteralPath $Archive1 -Algorithm SHA256).Hash
    Invoke-Installer @('-Version', $V1, '-Archive', $Archive1, '-Checksum', $Checksum1, '-InstallRoot', $InstallRoot) | Out-Null
    $selected = ([IO.File]::ReadAllText((Join-Path $InstallRoot 'current'))).Trim()
    Assert-True -Condition ($selected -ceq $V1) -Message 'first install did not select v1'
    $versionOutput = & (Join-Path $InstallRoot 'bin\observer.cmd') version --json | Out-String
    Assert-True -Condition ($versionOutput.Contains($V1)) -Message 'launcher did not run v1'

    Invoke-Installer @('-Version', '0.1.0', '-Archive', $Archive1, '-Checksum', $Checksum1, '-InstallRoot', (Join-Path $Temporary 'unsafe version')) $false | Out-Null
    Invoke-Installer @('-Version', '0.1.0-preview.01', '-Archive', $Archive1, '-Checksum', $Checksum1, '-InstallRoot', (Join-Path $Temporary 'unsafe prerelease')) $false | Out-Null
    Invoke-Installer @('-Version', $V1, '-Archive', $Archive1, '-Checksum', ('0' * 64), '-InstallRoot', (Join-Path $Temporary 'bad checksum')) $false | Out-Null
    Assert-True -Condition (-not (Test-Path -LiteralPath (Join-Path $Temporary 'bad checksum'))) -Message 'checksum failure created an install root'

    $UnsafeArchive = New-Archive $V1 $V1 'link'
    Invoke-Installer @('-Version', $V1, '-Archive', $UnsafeArchive, '-Checksum', (Get-FileHash $UnsafeArchive -Algorithm SHA256).Hash, '-InstallRoot', (Join-Path $Temporary 'unsafe archive')) $false | Out-Null
    $ExtraArchive = New-Archive $V1 $V1 'extra'
    Invoke-Installer @('-Version', $V1, '-Archive', $ExtraArchive, '-Checksum', (Get-FileHash $ExtraArchive -Algorithm SHA256).Hash, '-InstallRoot', (Join-Path $Temporary 'extra archive')) $false | Out-Null

    $V2 = '0.1.0-preview.2'
    $Archive2 = New-Archive $V2
    Invoke-Installer @('-Version', $V2, '-Archive', $Archive2, '-Checksum', (Get-FileHash $Archive2 -Algorithm SHA256).Hash, '-InstallRoot', $InstallRoot) | Out-Null
    $selected = ([IO.File]::ReadAllText((Join-Path $InstallRoot 'current'))).Trim()
    $previous = ([IO.File]::ReadAllText((Join-Path $InstallRoot 'previous'))).Trim()
    Assert-True -Condition ($selected -ceq $V2) -Message 'upgrade did not select v2'
    Assert-True -Condition ($previous -ceq $V1) -Message 'upgrade did not retain v1 as previous'

    $V3 = '0.1.0-preview.3'
    $Mismatch = New-Archive $V3 $V2
    Invoke-Installer @('-Version', $V3, '-Archive', $Mismatch, '-Checksum', (Get-FileHash $Mismatch -Algorithm SHA256).Hash, '-InstallRoot', $InstallRoot) $false | Out-Null
    $selected = ([IO.File]::ReadAllText((Join-Path $InstallRoot 'current'))).Trim()
    Assert-True -Condition ($selected -ceq $V2) -Message 'identity failure changed current version'

    $lock = Join-Path $InstallRoot '.install-lock'
    [IO.Directory]::CreateDirectory($lock) | Out-Null
    [IO.File]::WriteAllText((Join-Path $lock 'sentinel'), 'owned-by-other-process')
    $Archive3 = New-Archive $V3
    Invoke-Installer @('-Version', $V3, '-Archive', $Archive3, '-Checksum', (Get-FileHash $Archive3 -Algorithm SHA256).Hash, '-InstallRoot', $InstallRoot) $false | Out-Null
    Assert-True -Condition (Test-Path -LiteralPath (Join-Path $lock 'sentinel')) -Message 'failed installer removed another operation lock'
    [IO.File]::Delete((Join-Path $lock 'sentinel'))
    [IO.Directory]::Delete($lock, $false)

    $rootName3 = "home-lab-observer_${V3}_windows_${Architecture}"
    $recovered = Join-Path $InstallRoot "versions\$V3"
    [IO.Directory]::CreateDirectory($recovered) | Out-Null
    foreach ($name in @('observer.exe', 'LICENSE', 'START-HERE.md', 'Run-Observer.cmd')) {
        [IO.File]::Copy((Join-Path (Join-Path $Temporary "build-$V3-$V3-safe\$rootName3") $name), (Join-Path $recovered $name), $false)
    }
    Invoke-Installer @('-Version', $V3, '-Archive', $Archive3, '-Checksum', (Get-FileHash $Archive3 -Algorithm SHA256).Hash, '-InstallRoot', $InstallRoot) | Out-Null
    $selected = ([IO.File]::ReadAllText((Join-Path $InstallRoot 'current'))).Trim()
    Assert-True -Condition ($selected -ceq $V3) -Message 'retry did not recover an already-promoted unselected version'

    Invoke-Installer @('-Rollback', $V1, '-InstallRoot', $InstallRoot) | Out-Null
    $selected = ([IO.File]::ReadAllText((Join-Path $InstallRoot 'current'))).Trim()
    $previous = ([IO.File]::ReadAllText((Join-Path $InstallRoot 'previous'))).Trim()
    Assert-True -Condition ($selected -ceq $V1) -Message 'rollback did not select v1'
    Assert-True -Condition ($previous -ceq $V3) -Message 'rollback did not retain v3'

    $Background = Join-Path $InstallRoot 'background'
    [IO.Directory]::CreateDirectory($Background) | Out-Null
    [IO.File]::WriteAllText((Join-Path $Background '.managed'), "home-lab-observer-background-v1`r`n")
    [IO.File]::WriteAllText((Join-Path $Background 'settings.json'), '{}')
    [IO.File]::WriteAllText((Join-Path $Background 'task.xml'), '<Task/>')
    [IO.File]::WriteAllText((Join-Path $Background '.operation-lock'), '')
    Invoke-Installer @('-Rollback', $V3, '-InstallRoot', $InstallRoot) | Out-Null
    Invoke-Installer @('-Rollback', $V1, '-InstallRoot', $InstallRoot) | Out-Null
    Assert-True -Condition (Test-Path -LiteralPath (Join-Path $Background '.managed')) -Message 'rollback removed background registration evidence'
    Invoke-Installer @('-Uninstall', '-InstallRoot', $InstallRoot) $false | Out-Null
    Assert-True -Condition (Test-Path -LiteralPath (Join-Path $InstallRoot 'current')) -Message 'refused uninstall removed program files'
    foreach ($name in @('.managed', 'settings.json', 'task.xml')) { [IO.File]::Delete((Join-Path $Background $name)) }
    [IO.File]::WriteAllText((Join-Path $Background 'unknown'), 'owner-data')
    Invoke-Installer @('-Uninstall', '-InstallRoot', $InstallRoot) $false | Out-Null
    Assert-True -Condition (Test-Path -LiteralPath (Join-Path $Background 'unknown')) -Message 'uninstall removed unknown background data'
    [IO.File]::Delete((Join-Path $Background 'unknown'))

    [IO.File]::WriteAllText((Join-Path $InstallRoot 'versions\.unknown'), 'unknown')
    Invoke-Installer @('-Uninstall', '-InstallRoot', $InstallRoot) $false | Out-Null
    Assert-True -Condition (Test-Path -LiteralPath (Join-Path $InstallRoot 'versions\.unknown')) -Message 'uninstall removed unknown version content'
    [IO.File]::Delete((Join-Path $InstallRoot 'versions\.unknown'))

    $Unmanaged = Join-Path $Temporary 'unmanaged root'
    [IO.Directory]::CreateDirectory($Unmanaged) | Out-Null
    [IO.File]::WriteAllText((Join-Path $Unmanaged 'user-file'), 'do-not-delete')
    Invoke-Installer @('-Uninstall', '-InstallRoot', $Unmanaged) $false | Out-Null
    Assert-True -Condition (Test-Path -LiteralPath (Join-Path $Unmanaged 'user-file')) -Message 'uninstall removed an unmanaged file'

    $JunctionTarget = Join-Path $Temporary 'junction target'
    $JunctionPath = Join-Path $Temporary 'junction parent'
    [IO.Directory]::CreateDirectory($JunctionTarget) | Out-Null
    New-Item -ItemType Junction -Path $JunctionPath -Target $JunctionTarget | Out-Null
    Invoke-Installer @('-Version', $V1, '-Archive', $Archive1, '-Checksum', $Checksum1, '-InstallRoot', (Join-Path $JunctionPath 'programs')) $false | Out-Null
    Assert-True -Condition (-not (Test-Path -LiteralPath (Join-Path $JunctionTarget 'programs'))) -Message 'installer traversed a junction parent'
    [IO.Directory]::Delete($JunctionPath, $false)

    $Partial = Join-Path $Temporary 'partial managed root'
    [IO.Directory]::CreateDirectory($Partial) | Out-Null
    [IO.File]::WriteAllText((Join-Path $Partial '.home-lab-observer-managed'), "home-lab-observer-managed-v1`r`n")
    Invoke-Installer @('-Uninstall', '-InstallRoot', $Partial) | Out-Null
    Assert-True -Condition (-not (Test-Path -LiteralPath $Partial)) -Message 'uninstall could not recover a marker-only partial root'

    Invoke-Installer @('-Uninstall', '-InstallRoot', $InstallRoot) | Out-Null
    Assert-True -Condition (-not (Test-Path -LiteralPath $InstallRoot)) -Message 'uninstall left managed program files'
    $state = [IO.File]::ReadAllText((Join-Path $StateRoot 'sentinel'))
    Assert-True -Condition ($state -ceq 'keep-token-and-history') -Message 'uninstall removed state outside the program root'
    Write-Output 'PowerShell installer lifecycle and safety tests passed.'
} finally {
    Remove-TestTree $Temporary
}
