[CmdletBinding(DefaultParameterSetName = 'Install')]
param(
    [Parameter(Mandatory = $true, ParameterSetName = 'Install')]
    [string] $Version,

    [Parameter(ParameterSetName = 'Install')]
    [string] $Archive,

    [Parameter(ParameterSetName = 'Install')]
    [string] $Checksum,

    [Parameter(Mandatory = $true, ParameterSetName = 'Rollback')]
    [string] $Rollback,

    [Parameter(Mandatory = $true, ParameterSetName = 'Uninstall')]
    [switch] $Uninstall,

    [string] $InstallRoot
)

$ErrorActionPreference = 'Stop'
$Repository = 'braidenm/home-lab-observer'
$ManagedMarker = 'home-lab-observer-managed-v1'
$MaxArchiveBytes = 230686720
$script:StageDirectory = $null
$script:LockDirectory = $null
$script:IncomingDirectory = $null

function Fail([string] $Message) {
    throw "Home Lab Observer installer: $Message"
}

function Test-Version([string] $Value) {
    if ([string]::IsNullOrWhiteSpace($Value) -or $Value.Length -gt 64) { return $false }
    if ($Value -cnotmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*$') { return $false }
    $prerelease = $Value.Substring($Value.IndexOf('-') + 1)
    foreach ($identifier in $prerelease.Split('.')) {
        if ($identifier -cmatch '^[0-9]+$' -and $identifier.Length -gt 1 -and $identifier.StartsWith('0', [StringComparison]::Ordinal)) { return $false }
    }
    return $true
}

function Test-Checksum([string] $Value) {
    return $null -ne $Value -and $Value -match '^[0-9A-Fa-f]{64}$'
}

function Get-PlatformArchitecture {
    $machine = $env:PROCESSOR_ARCHITEW6432
    if ([string]::IsNullOrEmpty($machine)) { $machine = $env:PROCESSOR_ARCHITECTURE }
    switch ($machine.ToUpperInvariant()) {
        'AMD64' { return 'amd64' }
        'ARM64' { return 'arm64' }
        default { Fail "unsupported Windows architecture: $machine" }
    }
}

function Get-DefaultInstallRoot {
    if ([string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) { Fail 'LOCALAPPDATA is required for the default per-user install root' }
    return [IO.Path]::Combine($env:LOCALAPPDATA, 'Programs', 'Home Lab Observer')
}

function Test-ReparsePoint([IO.FileSystemInfo] $Item) {
    return ($Item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0
}

function Resolve-SafeInstallRoot([string] $Requested, [bool] $Create) {
    if ([string]::IsNullOrWhiteSpace($Requested)) { Fail 'install root is empty' }
    $resolved = [IO.Path]::GetFullPath($Requested).TrimEnd([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar)
    $volumeRoot = [IO.Path]::GetPathRoot($resolved).TrimEnd([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar)
    $forbidden = @(
        $volumeRoot,
        [Environment]::GetFolderPath([Environment+SpecialFolder]::UserProfile),
        [Environment]::GetFolderPath([Environment+SpecialFolder]::Windows),
        [Environment]::GetFolderPath([Environment+SpecialFolder]::ProgramFiles),
        [Environment]::GetFolderPath([Environment+SpecialFolder]::ProgramFilesX86)
    )
    foreach ($path in $forbidden) {
        if (-not [string]::IsNullOrWhiteSpace($path) -and $resolved.Equals($path.TrimEnd('\', '/'), [StringComparison]::OrdinalIgnoreCase)) {
            Fail "refusing broad install root: $resolved"
        }
    }
    $relative = $resolved.Substring([IO.Path]::GetPathRoot($resolved).Length)
    $walk = [IO.Path]::GetPathRoot($resolved)
    foreach ($component in $relative.Split(@([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar), [StringSplitOptions]::RemoveEmptyEntries)) {
        $walk = Join-Path $walk $component
        if (Test-Path -LiteralPath $walk) {
            if (Test-ReparsePoint (Get-Item -LiteralPath $walk -Force)) { Fail "install root path contains a link or reparse point: $walk" }
        } else { break }
    }
    if (Test-Path -LiteralPath $resolved) {
        $item = Get-Item -LiteralPath $resolved -Force
        if (-not $item.PSIsContainer -or (Test-ReparsePoint $item)) { Fail 'install root must be a real directory, not a link or reparse point' }
    } elseif ($Create) {
        [IO.Directory]::CreateDirectory($resolved) | Out-Null
    } else {
        Fail 'install root does not exist'
    }
    return $resolved
}

function New-PrivateStage {
    $path = [IO.Path]::Combine([IO.Path]::GetTempPath(), "home-lab-observer-install-$([Guid]::NewGuid().ToString('N'))")
    [IO.Directory]::CreateDirectory($path) | Out-Null
    $script:StageDirectory = $path
    $sid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
    & (Join-Path $env:SystemRoot 'System32\icacls.exe') $path '/inheritance:r' '/grant:r' "*${sid}:(OI)(CI)F" '/q' | Out-Null
    if ($LASTEXITCODE -ne 0) { Fail 'cannot secure private staging directory' }
    return $path
}

function Get-Sha256([string] $Path) {
    # Get-FileHash is a module function that may be absent when a parent process
    # inherits PowerShell 7's module path into Windows PowerShell 5.1.
    $stream = [IO.File]::OpenRead($Path)
    $algorithm = [Security.Cryptography.SHA256]::Create()
    try {
        return ([BitConverter]::ToString($algorithm.ComputeHash($stream))).Replace('-', '').ToLowerInvariant()
    } finally {
        $algorithm.Dispose()
        $stream.Dispose()
    }
}

function Invoke-Download([string] $Uri, [string] $Destination, [Int64] $MaximumBytes) {
    if (-not $Uri.StartsWith('https://', [StringComparison]::Ordinal)) { Fail 'download URL must use HTTPS' }
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    $current = $Uri
    for ($redirect = 0; $redirect -le 5; $redirect += 1) {
        $request = [Net.HttpWebRequest]::Create($current)
        $request.AllowAutoRedirect = $false
        $request.Timeout = 15000
        $request.ReadWriteTimeout = 15000
        $request.UserAgent = 'home-lab-observer-installer/1'
        $response = $request.GetResponse()
        $status = [int] $response.StatusCode
        if ($status -ge 300 -and $status -lt 400) {
            $location = $response.Headers['Location']
            $response.Dispose()
            if ([string]::IsNullOrWhiteSpace($location)) { Fail 'download redirect omitted its destination' }
            $next = New-Object Uri((New-Object Uri($current)), $location)
            if ($next.Scheme -cne 'https') { Fail 'download redirect must use HTTPS' }
            $current = $next.AbsoluteUri
            continue
        }
        if ($status -lt 200 -or $status -ge 300) { $response.Dispose(); Fail "download failed with HTTP status $status" }
        if ($response.ContentLength -gt $MaximumBytes) { $response.Dispose(); Fail 'download exceeds its size limit' }
        $input = $response.GetResponseStream()
        $output = New-Object IO.FileStream($Destination, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
        $timer = [Diagnostics.Stopwatch]::StartNew()
        try {
            $buffer = New-Object byte[] 65536
            $total = [Int64]0
            while (($read = $input.Read($buffer, 0, $buffer.Length)) -gt 0) {
                $total += $read
                if ($total -gt $MaximumBytes) { Fail 'download exceeds its size limit' }
                if ($timer.Elapsed.TotalSeconds -gt 120) { Fail 'download exceeded its time limit' }
                $output.Write($buffer, 0, $read)
            }
        } finally {
            $output.Dispose()
            $input.Dispose()
            $response.Dispose()
        }
        return
    }
    Fail 'download exceeded the redirect limit'
}

function Assert-RegularFile([string] $Path, [string] $Label) {
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { Fail "$Label is missing" }
    if (Test-ReparsePoint (Get-Item -LiteralPath $Path -Force)) { Fail "$Label must not be a link or reparse point" }
}

function Assert-VersionDirectory([string] $Directory, [string] $ExpectedVersion) {
    if (-not (Test-Version $ExpectedVersion)) { Fail "managed version directory has an unsafe name: $ExpectedVersion" }
    if (-not (Test-Path -LiteralPath $Directory -PathType Container)) { Fail "managed version is missing: $ExpectedVersion" }
    $directoryItem = Get-Item -LiteralPath $Directory -Force
    if (Test-ReparsePoint $directoryItem) { Fail "managed version directory is a reparse point: $ExpectedVersion" }
    $expected = @('observer.exe', 'LICENSE', 'START-HERE.md', 'Run-Observer.cmd')
    $members = @(Get-ChildItem -LiteralPath $Directory -Force)
    if ($members.Count -ne $expected.Count) { Fail "managed version directory has unexpected contents: $ExpectedVersion" }
    foreach ($name in $expected) { Assert-RegularFile (Join-Path $Directory $name) "$ExpectedVersion/$name" }
    foreach ($member in $members) {
        if ($expected -notcontains $member.Name) { Fail "managed version directory has an unexpected entry: $($member.Name)" }
    }
}

function Assert-ManagedRoot([string] $Root) {
    $marker = Join-Path $Root '.home-lab-observer-managed'
    Assert-RegularFile $marker 'managed-root marker'
    if (([IO.File]::ReadAllText($marker)).TrimEnd("`r", "`n") -cne $ManagedMarker) { Fail 'managed-root marker is invalid' }
    $allowed = @('.home-lab-observer-managed', '.install-lock', 'bin', 'versions', 'current', 'previous')
    foreach ($member in @(Get-ChildItem -LiteralPath $Root -Force)) {
        if ($allowed -notcontains $member.Name) { Fail "managed root contains an unknown entry: $($member.Name)" }
        if (Test-ReparsePoint $member) { Fail "managed root contains a reparse point: $($member.Name)" }
    }
    $bin = Join-Path $Root 'bin'
    if (Test-Path -LiteralPath $bin) {
        $binMembers = @(Get-ChildItem -LiteralPath $bin -Force)
        if ($binMembers.Count -gt 1 -or ($binMembers.Count -eq 1 -and $binMembers[0].Name -cne 'observer.cmd')) { Fail 'managed launcher directory contains unknown files' }
        if ($binMembers.Count -eq 1) { Assert-RegularFile $binMembers[0].FullName 'managed launcher' }
    }
    $versions = Join-Path $Root 'versions'
    if (Test-Path -LiteralPath $versions) {
        $versionsItem = Get-Item -LiteralPath $versions -Force
        if (-not $versionsItem.PSIsContainer -or (Test-ReparsePoint $versionsItem)) { Fail 'versions entry is unsafe' }
        foreach ($directory in @(Get-ChildItem -LiteralPath $versions -Force)) {
            if (-not $directory.PSIsContainer) { Fail "versions contains a non-directory entry: $($directory.Name)" }
            Assert-VersionDirectory $directory.FullName $directory.Name
        }
    }
    foreach ($pointerName in @('current', 'previous')) {
        $pointer = Join-Path $Root $pointerName
        if (Test-Path -LiteralPath $pointer) {
            Assert-RegularFile $pointer "$pointerName version pointer"
            $pointerVersion = ([IO.File]::ReadAllText($pointer)).TrimEnd("`r", "`n")
            if (-not (Test-Version $pointerVersion)) { Fail "$pointerName version pointer is invalid" }
            Assert-VersionDirectory (Join-Path $versions $pointerVersion) $pointerVersion
        }
    }
}

function Enter-InstallLock([string] $Root) {
    $candidate = Join-Path $Root '.install-lock'
    try { New-Item -ItemType Directory -Path $candidate -ErrorAction Stop | Out-Null } catch { Fail 'another installer operation is active' }
    $script:LockDirectory = $candidate
}

function Write-AtomicText([string] $Destination, [string] $Value) {
    $temporary = Join-Path $script:LockDirectory ((Split-Path -Leaf $Destination) + '.next')
    [IO.File]::WriteAllText($temporary, $Value + [Environment]::NewLine, (New-Object Text.UTF8Encoding($false)))
    if (Test-Path -LiteralPath $Destination) {
        $backup = Join-Path $script:LockDirectory ((Split-Path -Leaf $Destination) + '.backup')
        [IO.File]::Replace($temporary, $Destination, $backup)
        [IO.File]::Delete($backup)
    } else {
        [IO.File]::Move($temporary, $Destination)
    }
}

function Test-ZipEntryLink([IO.Compression.ZipArchiveEntry] $Entry) {
    $unixType = (($Entry.ExternalAttributes -shr 16) -band 0xF000)
    $dosReparse = ($Entry.ExternalAttributes -band 0x400) -ne 0
    return $unixType -eq 0xA000 -or $unixType -eq 0x6000 -or $unixType -eq 0x2000 -or $dosReparse
}

function Expand-ValidatedArchive([string] $ArchivePath, [string] $RootName) {
    if ((Get-Item -LiteralPath $ArchivePath).Length -gt $MaxArchiveBytes) { Fail 'archive exceeds the 220 MiB limit' }
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip = [IO.Compression.ZipFile]::OpenRead($ArchivePath)
    try {
        $expectedFiles = @("$RootName/observer.exe", "$RootName/LICENSE", "$RootName/START-HERE.md", "$RootName/Run-Observer.cmd")
        $entries = @($zip.Entries)
        if ($entries.Count -ne 5) { Fail 'archive must contain exactly one root directory and four files' }
        $seen = @{}
        $totalLength = [Int64]0
        foreach ($entry in $entries) {
            $name = $entry.FullName
            if ($name.Contains('\') -or $name.StartsWith('/') -or $name.Contains(':') -or $name.Split('/') -contains '..') { Fail "archive contains an unsafe member: $name" }
            if (Test-ZipEntryLink $entry) { Fail "archive contains a link, device, or reparse entry: $name" }
            if ($name -ceq "$RootName/") {
                if (-not [string]::IsNullOrEmpty($entry.Name)) { Fail 'archive root entry is not a directory' }
                $rootUnixType = (($entry.ExternalAttributes -shr 16) -band 0xF000)
                if ($rootUnixType -ne 0 -and $rootUnixType -ne 0x4000) { Fail 'archive root entry has an invalid type' }
            } elseif ($expectedFiles -ccontains $name) {
                if ([string]::IsNullOrEmpty($entry.Name)) { Fail "archive file is a directory: $name" }
                $fileUnixType = (($entry.ExternalAttributes -shr 16) -band 0xF000)
                if ($fileUnixType -ne 0 -and $fileUnixType -ne 0x8000) { Fail "archive member is not a regular file: $name" }
                if ($seen.ContainsKey($name)) { Fail "archive contains a duplicate member: $name" }
                $seen[$name] = $true
                $limit = if ($name.EndsWith('/observer.exe', [StringComparison]::Ordinal)) { 209715200 } else { 1048576 }
                if ($entry.Length -le 0 -or $entry.Length -gt $limit) { Fail "archive member size is invalid: $name" }
                $totalLength += $entry.Length
            } else {
                Fail "archive contains an unexpected member: $name"
            }
        }
        if ($seen.Count -ne 4 -or $totalLength -gt 220200960) { Fail 'archive contents are incomplete or exceed their size limit' }
        $extractRoot = Join-Path $script:StageDirectory 'extract'
        [IO.Directory]::CreateDirectory($extractRoot) | Out-Null
        $sourceDirectory = Join-Path $extractRoot $RootName
        [IO.Directory]::CreateDirectory($sourceDirectory) | Out-Null
        foreach ($entry in $entries) {
            if ($entry.FullName -ceq "$RootName/") { continue }
            $target = Join-Path $sourceDirectory $entry.Name
            $input = $entry.Open()
            try {
                $output = New-Object IO.FileStream($target, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
                try { $input.CopyTo($output) } finally { $output.Dispose() }
            } finally { $input.Dispose() }
        }
        return $sourceDirectory
    } finally { $zip.Dispose() }
}

function Assert-BinaryIdentity([string] $Binary, [string] $ExpectedVersion, [string] $ExpectedArch) {
    $start = New-Object Diagnostics.ProcessStartInfo
    $start.FileName = $Binary
    $start.Arguments = 'version --json'
    $start.UseShellExecute = $false
    $start.CreateNoWindow = $true
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    $process = [Diagnostics.Process]::Start($start)
    if (-not $process.WaitForExit(10000)) { $process.Kill(); Fail 'observer version check timed out' }
    $stdout = $process.StandardOutput.ReadToEnd()
    $stderr = $process.StandardError.ReadToEnd()
    if ($process.ExitCode -ne 0 -or -not [string]::IsNullOrEmpty($stderr) -or $stdout.Length -gt 4096) { Fail 'observer version check failed' }
    try { $build = $stdout | ConvertFrom-Json } catch { Fail 'observer version output is invalid JSON' }
    $actualKeys = @($build.PSObject.Properties.Name | Sort-Object)
    $expectedKeys = @('arch', 'commit', 'go_version', 'os', 'schema_version', 'version')
    if (@(Compare-Object $actualKeys $expectedKeys -CaseSensitive).Count -ne 0) { Fail 'observer version output has unexpected fields' }
    if ($build.schema_version -cne 'observer-build/v1' -or $build.version -cne $ExpectedVersion -or $build.os -cne 'windows' -or $build.arch -cne $ExpectedArch) { Fail 'observer binary identity does not match the requested version and platform' }
    if ($build.commit -cnotmatch '^[0-9a-f]{40}$' -or $build.go_version -cnotmatch '^go[0-9]+\.[0-9]+(\.[0-9]+)?([A-Za-z0-9.-]+)?$') { Fail 'observer binary build identity is invalid' }
}

function Install-Launcher([string] $Root) {
    $content = @'
@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "observer_root=%~dp0.."
set "observer_version="
"%SystemRoot%\System32\findstr.exe" /r /x "[0-9A-Za-z][0-9A-Za-z.+-]*" "%observer_root%\current" >nul || exit /b 2
"%SystemRoot%\System32\findstr.exe" /v /r /x "[0-9A-Za-z][0-9A-Za-z.+-]*" "%observer_root%\current" >nul && exit /b 2
set /p "observer_version="<"%observer_root%\current"
if not defined observer_version exit /b 2
if not exist "%observer_root%\versions\%observer_version%\observer.exe" exit /b 2
"%observer_root%\versions\%observer_version%\observer.exe" %*
exit /b %errorlevel%
'@
    $temporary = Join-Path $script:LockDirectory 'observer.next'
    [IO.File]::WriteAllText($temporary, $content, [Text.Encoding]::ASCII)
    $bin = Join-Path $Root 'bin'
    [IO.Directory]::CreateDirectory($bin) | Out-Null
    $launcher = Join-Path $bin 'observer.cmd'
    if (Test-Path -LiteralPath $launcher) {
        $backup = Join-Path $script:LockDirectory 'observer.backup'
        [IO.File]::Replace($temporary, $launcher, $backup)
        [IO.File]::Delete($backup)
    } else { [IO.File]::Move($temporary, $launcher) }
}

function Remove-VersionDirectory([string] $Directory) {
    foreach ($name in @('observer.exe', 'LICENSE', 'START-HERE.md', 'Run-Observer.cmd')) {
        [IO.File]::Delete((Join-Path $Directory $name))
    }
    [IO.Directory]::Delete($Directory, $false)
}

function Cleanup {
    if ($null -ne $script:IncomingDirectory -and (Test-Path -LiteralPath $script:IncomingDirectory -PathType Container)) {
        foreach ($name in @('observer.exe', 'LICENSE', 'START-HERE.md', 'Run-Observer.cmd')) { [IO.File]::Delete((Join-Path $script:IncomingDirectory $name)) }
        try { [IO.Directory]::Delete($script:IncomingDirectory, $false) } catch { }
    }
    if ($null -ne $script:LockDirectory -and (Test-Path -LiteralPath $script:LockDirectory -PathType Container)) {
        foreach ($name in @('current.next', 'previous.next', 'observer.next', 'current.backup', 'previous.backup', 'observer.backup')) { [IO.File]::Delete((Join-Path $script:LockDirectory $name)) }
        try { [IO.Directory]::Delete($script:LockDirectory, $false) } catch { }
    }
    if ($null -ne $script:StageDirectory -and (Test-Path -LiteralPath $script:StageDirectory -PathType Container)) {
        $extract = Join-Path $script:StageDirectory 'extract'
        if (Test-Path -LiteralPath $extract -PathType Container) {
            foreach ($root in @(Get-ChildItem -LiteralPath $extract -Force)) {
                if ($root.PSIsContainer -and -not (Test-ReparsePoint $root)) {
                    foreach ($file in @(Get-ChildItem -LiteralPath $root.FullName -Force)) { if (-not $file.PSIsContainer) { [IO.File]::Delete($file.FullName) } }
                    try { [IO.Directory]::Delete($root.FullName, $false) } catch { }
                }
            }
            try { [IO.Directory]::Delete($extract, $false) } catch { }
        }
        foreach ($name in @('archive.zip', 'SHA256SUMS')) { [IO.File]::Delete((Join-Path $script:StageDirectory $name)) }
        try { [IO.Directory]::Delete($script:StageDirectory, $false) } catch { }
    }
}

try {
    if ([string]::IsNullOrWhiteSpace($InstallRoot)) { $InstallRoot = Get-DefaultInstallRoot }
    if ($PSCmdlet.ParameterSetName -eq 'Install') {
        if (-not (Test-Version $Version)) { Fail '-Version must be an explicit SemVer prerelease without a leading v' }
        if ([string]::IsNullOrWhiteSpace($Archive) -xor [string]::IsNullOrWhiteSpace($Checksum)) { Fail 'offline installation requires both -Archive and -Checksum' }
        if (-not [string]::IsNullOrWhiteSpace($Checksum) -and -not (Test-Checksum $Checksum)) { Fail '-Checksum must be exactly 64 hexadecimal characters' }
        $architecture = Get-PlatformArchitecture
        $rootName = "home-lab-observer_${Version}_windows_${architecture}"
        $archiveName = "$rootName.zip"
        $script:StageDirectory = New-PrivateStage
        $stagedArchive = Join-Path $script:StageDirectory 'archive.zip'
        if (-not [string]::IsNullOrWhiteSpace($Archive)) {
            $resolvedArchive = [IO.Path]::GetFullPath($Archive)
            Assert-RegularFile $resolvedArchive 'offline archive'
            if ((Get-Item -LiteralPath $resolvedArchive).Length -gt $MaxArchiveBytes) { Fail 'offline archive exceeds the 220 MiB limit' }
            [IO.File]::Copy($resolvedArchive, $stagedArchive, $false)
            $expectedChecksum = $Checksum
        } else {
            $baseUrl = "https://github.com/$Repository/releases/download/v$Version"
            Invoke-Download "$baseUrl/$archiveName" $stagedArchive $MaxArchiveBytes
            $sumPath = Join-Path $script:StageDirectory 'SHA256SUMS'
            Invoke-Download "$baseUrl/SHA256SUMS" $sumPath 1048576
            if ((Get-Item -LiteralPath $sumPath).Length -gt 1048576) { Fail 'SHA256SUMS exceeds its size limit' }
            $escapedName = [Regex]::Escape($archiveName)
            $matches = @([IO.File]::ReadAllLines($sumPath) | Where-Object { $_ -match "^([0-9A-Fa-f]{64})[ `t]+\*?$escapedName$" })
            if ($matches.Count -ne 1) { Fail "SHA256SUMS does not contain exactly one entry for $archiveName" }
            $expectedChecksum = ([Regex]::Match($matches[0], '^([0-9A-Fa-f]{64})')).Groups[1].Value
        }
        if ((Get-Sha256 $stagedArchive) -cne $expectedChecksum.ToLowerInvariant()) { Fail 'archive checksum mismatch' }
        $sourceDirectory = Expand-ValidatedArchive $stagedArchive $rootName
        Assert-BinaryIdentity (Join-Path $sourceDirectory 'observer.exe') $Version $architecture

        $resolvedRoot = Resolve-SafeInstallRoot $InstallRoot $true
        $existing = @(Get-ChildItem -LiteralPath $resolvedRoot -Force)
        if ($existing.Count -eq 0) {
            [IO.File]::WriteAllText((Join-Path $resolvedRoot '.home-lab-observer-managed'), $ManagedMarker + [Environment]::NewLine, (New-Object Text.UTF8Encoding($false)))
        } else {
            Assert-ManagedRoot $resolvedRoot
        }
        Enter-InstallLock $resolvedRoot
        $versions = Join-Path $resolvedRoot 'versions'
        [IO.Directory]::CreateDirectory($versions) | Out-Null
        $target = Join-Path $versions $Version
        if (Test-Path -LiteralPath $target) {
            Assert-VersionDirectory $target $Version
            foreach ($name in @('observer.exe', 'LICENSE', 'START-HERE.md', 'Run-Observer.cmd')) {
                if ((Get-Sha256 (Join-Path $sourceDirectory $name)) -cne (Get-Sha256 (Join-Path $target $name))) { Fail 'existing unselected version does not match the verified archive' }
            }
        } else {
            $script:IncomingDirectory = Join-Path $versions ".incoming-$Version-$PID"
            [IO.Directory]::CreateDirectory($script:IncomingDirectory) | Out-Null
            foreach ($name in @('observer.exe', 'LICENSE', 'START-HERE.md', 'Run-Observer.cmd')) {
                [IO.File]::Copy((Join-Path $sourceDirectory $name), (Join-Path $script:IncomingDirectory $name), $false)
            }
            [IO.Directory]::Move($script:IncomingDirectory, $target)
            $script:IncomingDirectory = $null
        }
        Install-Launcher $resolvedRoot
        $current = Join-Path $resolvedRoot 'current'
        if (Test-Path -LiteralPath $current) {
            $oldVersion = ([IO.File]::ReadAllText($current)).TrimEnd("`r", "`n")
            if (-not (Test-Version $oldVersion)) { Fail 'current version pointer is invalid' }
            if ($oldVersion -cne $Version) { Write-AtomicText (Join-Path $resolvedRoot 'previous') $oldVersion }
        }
        Write-AtomicText $current $Version
        Write-Output "Installed Home Lab Observer $Version. Nothing was started."
        Write-Output "Run: $(Join-Path $resolvedRoot 'bin\observer.cmd')"
    } elseif ($PSCmdlet.ParameterSetName -eq 'Rollback') {
        if (-not (Test-Version $Rollback)) { Fail '-Rollback requires an installed SemVer prerelease' }
        $resolvedRoot = Resolve-SafeInstallRoot $InstallRoot $false
        Assert-ManagedRoot $resolvedRoot
        Assert-VersionDirectory (Join-Path (Join-Path $resolvedRoot 'versions') $Rollback) $Rollback
        Enter-InstallLock $resolvedRoot
        $current = Join-Path $resolvedRoot 'current'
        $oldVersion = ([IO.File]::ReadAllText($current)).TrimEnd("`r", "`n")
        if ($oldVersion -ceq $Rollback) { Fail "version is already selected: $Rollback" }
        Write-AtomicText (Join-Path $resolvedRoot 'previous') $oldVersion
        Write-AtomicText $current $Rollback
        Write-Output "Selected Home Lab Observer $Rollback. Nothing was started."
    } else {
        $resolvedRoot = Resolve-SafeInstallRoot $InstallRoot $false
        Assert-ManagedRoot $resolvedRoot
        Enter-InstallLock $resolvedRoot
        $bin = Join-Path $resolvedRoot 'bin'
        if (Test-Path -LiteralPath $bin) {
            [IO.File]::Delete((Join-Path $bin 'observer.cmd'))
            [IO.Directory]::Delete($bin, $false)
        }
        $versions = Join-Path $resolvedRoot 'versions'
        if (Test-Path -LiteralPath $versions) {
            foreach ($directory in @(Get-ChildItem -LiteralPath $versions -Force)) { Remove-VersionDirectory $directory.FullName }
            [IO.Directory]::Delete($versions, $false)
        }
        foreach ($name in @('current', 'previous', '.home-lab-observer-managed')) { [IO.File]::Delete((Join-Path $resolvedRoot $name)) }
        [IO.Directory]::Delete($script:LockDirectory, $false)
        $script:LockDirectory = $null
        [IO.Directory]::Delete($resolvedRoot, $false)
        Write-Output 'Removed managed Home Lab Observer program files. Observation and token state were preserved.'
    }
} catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
} finally {
    Cleanup
}
