[CmdletBinding()]
param([switch] $ValidateConditionsOnly)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$provider = 'BraidenM-HomeLabObserver-NativeFixture'
$systemChannel = 'BraidenM-HomeLabObserver/Fixture-System'
$applicationChannel = 'BraidenM-HomeLabObserver/Fixture-Application'
$markerName = '.hlo-owned-windows-event-fixture-v1'
$marker = "HLO_OWNED_WINDOWS_EVENT_FIXTURE_V1`n"
$registrationAttempted = $false
$ownedRoot = $null
$manifest = $null
$testProcess = $null
$testProcessReaped = $true
$fixturePassed = $false

function Fail([string] $Message) { throw "owned Windows event fixture failed: $Message" }

function Invoke-Quiet([string] $File, [string[]] $Arguments) {
    & $File @Arguments 1>$null 2>$null
    if ($LASTEXITCODE -ne 0) { Fail "required fixture command failed" }
}

function Test-WevtMissing([string] $Kind, [string] $Name) {
    if ($Kind -eq 'provider' -and $Name -ceq $provider) { $exitCode = [OwnedEventFixtureMetadataProbe]::Publisher() }
    elseif ($Kind -eq 'channel' -and $Name -ceq $systemChannel) { $exitCode = [OwnedEventFixtureMetadataProbe]::SystemChannel() }
    elseif ($Kind -eq 'channel' -and $Name -ceq $applicationChannel) { $exitCode = [OwnedEventFixtureMetadataProbe]::ApplicationChannel() }
    else { Fail 'unknown fixture registration kind' }
    if ($exitCode -eq 0) { return $false }
    if (Test-IsDocumentedMissing $Kind $exitCode) { return $true }
    Fail 'fixture metadata probe did not return a documented missing status'
}

function Test-IsDocumentedMissing([string] $Kind, [int] $ExitCode) {
    # Microsoft documents these Win32 Event Log results as provider metadata
    # missing (15002) and channel missing (15007). Every other CLI failure is
    # permission/service/invalid-state uncertainty and therefore fails closed.
    return ($Kind -eq 'provider' -and $ExitCode -eq 15002) -or
        ($Kind -eq 'channel' -and $ExitCode -eq 15007)
}

function Test-AllMissing([bool] $ProviderMissing, [bool] $SystemMissing, [bool] $ApplicationMissing) {
    return $ProviderMissing -and $SystemMissing -and $ApplicationMissing
}

function Test-AnyMissing([bool] $ProviderMissing, [bool] $SystemMissing, [bool] $ApplicationMissing) {
    return $ProviderMissing -or $SystemMissing -or $ApplicationMissing
}

if ($ValidateConditionsOnly) {
    if (-not (Test-AllMissing $true $true $true) -or (Test-AllMissing $true $false $true) -or
        (Test-AnyMissing $false $false $false) -or -not (Test-AnyMissing $false $true $false)) {
        Fail 'registration condition self-test failed'
    }
    if (-not (Test-IsDocumentedMissing 'provider' 15002) -or -not (Test-IsDocumentedMissing 'channel' 15007) -or
        (Test-IsDocumentedMissing 'provider' 15007) -or (Test-IsDocumentedMissing 'channel' 15002) -or
        (Test-IsDocumentedMissing 'provider' 0) -or (Test-IsDocumentedMissing 'channel' 5) -or
        (Test-IsDocumentedMissing 'channel' 1722)) {
        Fail 'registration exit-code self-test failed'
    }
    Write-Output 'Windows event fixture registration conditions passed.'
    return
}

function Assert-PrivateRegular([string] $Path, [Int64] $Maximum) {
    if (-not [IO.Path]::IsPathFullyQualified($Path) -or -not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        Fail 'owned output is missing'
    }
    $item = Get-Item -LiteralPath $Path -Force
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or $item.Length -lt 1 -or $item.Length -gt $Maximum) {
        Fail 'owned output is linked, empty, or oversized'
    }
}

if ($env:GITHUB_ACTIONS -cne 'true' -or $env:RUNNER_ENVIRONMENT -cne 'github-hosted' -or
    [string]::IsNullOrWhiteSpace($env:RUNNER_TEMP) -or [Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
    Fail 'this test is restricted to an ephemeral GitHub-hosted Windows runner'
}
$principal = [Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Fail 'provider registration requires the hosted runner administrator token'
}
$probeSource = @'
using System;
using System.Runtime.InteropServices;

public static class OwnedEventFixtureMetadataProbe
{
    private const string ProviderName = "BraidenM-HomeLabObserver-NativeFixture";
    private const string SystemChannelName = "BraidenM-HomeLabObserver/Fixture-System";
    private const string ApplicationChannelName = "BraidenM-HomeLabObserver/Fixture-Application";

    [DefaultDllImportSearchPaths(DllImportSearchPath.System32)]
    [DllImport("wevtapi.dll", CharSet = CharSet.Unicode, ExactSpelling = true, SetLastError = true)]
    private static extern IntPtr EvtOpenPublisherMetadata(IntPtr session, string publisherId, string logFilePath, int locale, int flags);

    [DefaultDllImportSearchPaths(DllImportSearchPath.System32)]
    [DllImport("wevtapi.dll", CharSet = CharSet.Unicode, ExactSpelling = true, SetLastError = true)]
    private static extern IntPtr EvtOpenChannelConfig(IntPtr session, string channelPath, int flags);

    [DefaultDllImportSearchPaths(DllImportSearchPath.System32)]
    [DllImport("wevtapi.dll", ExactSpelling = true, SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool EvtClose(IntPtr handle);

    public static int Publisher() { return Probe(EvtOpenPublisherMetadata(IntPtr.Zero, ProviderName, null, 0, 0)); }
    public static int SystemChannel() { return Probe(EvtOpenChannelConfig(IntPtr.Zero, SystemChannelName, 0)); }
    public static int ApplicationChannel() { return Probe(EvtOpenChannelConfig(IntPtr.Zero, ApplicationChannelName, 0)); }

    private static int Probe(IntPtr handle)
    {
        if (handle == IntPtr.Zero) {
            int openError = Marshal.GetLastWin32Error();
            return openError == 0 ? 1 : openError;
        }
        if (!EvtClose(handle)) return 1;
        return 0;
    }
}
'@
$null = Add-Type -TypeDefinition $probeSource -Language CSharp
$runnerRoot = [IO.Path]::GetFullPath($env:RUNNER_TEMP).TrimEnd([IO.Path]::DirectorySeparatorChar)
$runnerItem = Get-Item -LiteralPath $runnerRoot -Force
if (-not $runnerItem.PSIsContainer -or ($runnerItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
    Fail 'RUNNER_TEMP is not a direct owned directory'
}
$providerMissing = (Test-WevtMissing 'provider' $provider)
$systemMissing = (Test-WevtMissing 'channel' $systemChannel)
$applicationMissing = (Test-WevtMissing 'channel' $applicationChannel)
if (-not (Test-AllMissing $providerMissing $systemMissing $applicationMissing)) {
    Fail 'fixture provider or channel already exists'
}

try {
    $ownedRoot = Join-Path $runnerRoot ('.hlo-event-fixture-' + [Guid]::NewGuid().ToString('N'))
    [IO.Directory]::CreateDirectory($ownedRoot) | Out-Null
    $ownedItem = Get-Item -LiteralPath $ownedRoot -Force
    if (($ownedItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or
        [IO.Directory]::GetParent($ownedItem.FullName).FullName.TrimEnd([IO.Path]::DirectorySeparatorChar) -cne $runnerRoot) {
        Fail 'owned fixture root was not created directly beneath RUNNER_TEMP'
    }
    [IO.File]::WriteAllText((Join-Path $ownedRoot $markerName), $marker, [Text.UTF8Encoding]::new($false))

    foreach ($tool in @('mc.exe', 'rc.exe', 'link.exe', 'go.exe', 'wevtutil.exe')) {
        if ($null -eq (Get-Command $tool -ErrorAction SilentlyContinue)) { Fail 'required hosted build tool is unavailable' }
    }
    $sourceManifest = Join-Path $PSScriptRoot '..\internal\eventnative\testdata\fixture.man'
    Assert-PrivateRegular $sourceManifest 65536
    $manifest = Join-Path $ownedRoot 'fixture.man'
    [IO.File]::Copy([IO.Path]::GetFullPath($sourceManifest), $manifest, $false)
    Push-Location -LiteralPath $ownedRoot
    try {
        Invoke-Quiet 'mc.exe' @('-um', '-h', '.', '-r', '.', 'fixture.man')
        $resourceScript = @(Get-ChildItem -LiteralPath $ownedRoot -Filter '*.rc' -File)
        if ($resourceScript.Count -ne 1) { Fail 'message compiler did not produce one resource script' }
        $resource = Join-Path $ownedRoot 'fixture.res'
        Invoke-Quiet 'rc.exe' @('/nologo', ('/fo' + $resource), $resourceScript[0].FullName)
        $resourceDLL = Join-Path $ownedRoot 'fixture.dll'
        $machine = if ($env:PROCESSOR_ARCHITECTURE -ceq 'ARM64') { 'ARM64' } else { 'X64' }
        Invoke-Quiet 'link.exe' @('/nologo', '/dll', '/noentry', ('/machine:' + $machine), ('/out:' + $resourceDLL), $resource)
    } finally {
        Pop-Location
    }
    Assert-PrivateRegular $resourceDLL (2 * 1024 * 1024)

    $publisher = Join-Path $ownedRoot 'fixture-publisher.exe'
    Invoke-Quiet 'go.exe' @('build', '-trimpath', '-o', $publisher, './internal/eventnative/testdata/fixturepublisher')
    Assert-PrivateRegular $publisher (20 * 1024 * 1024)

    $registrationAttempted = $true
    Invoke-Quiet 'wevtutil.exe' @('im', $manifest, ('/rf:' + $resourceDLL), ('/mf:' + $resourceDLL))
    $providerMissing = (Test-WevtMissing 'provider' $provider)
    $systemMissing = (Test-WevtMissing 'channel' $systemChannel)
    $applicationMissing = (Test-WevtMissing 'channel' $applicationChannel)
    if (Test-AnyMissing $providerMissing $systemMissing $applicationMissing) {
        Fail 'fixture registration was not observable'
    }
    Invoke-Quiet $publisher @('before')

    $systemBefore = Join-Path $ownedRoot 'system-before.evtx'
    $applicationBefore = Join-Path $ownedRoot 'application-before.evtx'
    Invoke-Quiet 'wevtutil.exe' @('epl', $systemChannel, $systemBefore, "/q:*[System[Provider[@Name='$provider'] and (EventID=101 or EventID=102)]]")
    Invoke-Quiet 'wevtutil.exe' @('epl', $applicationChannel, $applicationBefore, "/q:*[System[Provider[@Name='$provider'] and (EventID=201 or EventID=202)]]")

    Invoke-Quiet 'wevtutil.exe' @('cl', $systemChannel)
    Invoke-Quiet 'wevtutil.exe' @('cl', $applicationChannel)
    Invoke-Quiet $publisher @('after')
    $systemAfter = Join-Path $ownedRoot 'system-after.evtx'
    $applicationAfter = Join-Path $ownedRoot 'application-after.evtx'
    Invoke-Quiet 'wevtutil.exe' @('epl', $systemChannel, $systemAfter, "/q:*[System[Provider[@Name='$provider'] and EventID=103]]")
    Invoke-Quiet 'wevtutil.exe' @('epl', $applicationChannel, $applicationAfter, "/q:*[System[Provider[@Name='$provider'] and EventID=203]]")
    foreach ($file in @($systemBefore, $applicationBefore, $systemAfter, $applicationAfter)) {
        Assert-PrivateRegular $file (2 * 1024 * 1024)
    }

    $testBinary = Join-Path $ownedRoot 'eventnative.test.exe'
    Invoke-Quiet 'go.exe' @('test', '-c', '-o', $testBinary, './internal/eventnative')
    Assert-PrivateRegular $testBinary (200 * 1024 * 1024)
    $stdoutPath = Join-Path $ownedRoot 'eventnative.stdout'
    $stderrPath = Join-Path $ownedRoot 'eventnative.stderr'
    $priorFixtureEnabled = [Environment]::GetEnvironmentVariable('OBSERVER_TEST_OWNED_WINDOWS_EVENT_FIXTURE', 'Process')
    $priorFixtureRoot = [Environment]::GetEnvironmentVariable('OBSERVER_TEST_EVTX_ROOT', 'Process')
    try {
        [Environment]::SetEnvironmentVariable('OBSERVER_TEST_OWNED_WINDOWS_EVENT_FIXTURE', '1', 'Process')
        [Environment]::SetEnvironmentVariable('OBSERVER_TEST_EVTX_ROOT', $ownedRoot, 'Process')
        $testProcess = Start-Process -FilePath $testBinary -ArgumentList @('-test.run=^TestOwnedWindowsNativeFixture$', '-test.count=1') `
            -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdoutPath -RedirectStandardError $stderrPath
    } finally {
        [Environment]::SetEnvironmentVariable('OBSERVER_TEST_OWNED_WINDOWS_EVENT_FIXTURE', $priorFixtureEnabled, 'Process')
        [Environment]::SetEnvironmentVariable('OBSERVER_TEST_EVTX_ROOT', $priorFixtureRoot, 'Process')
    }
    if ($null -eq $testProcess) { Fail 'native fixture test did not start' }
    $testProcessReaped = $false
    $deadline = [DateTime]::UtcNow.AddSeconds(30)
    $testFailure = $null
    while (-not $testProcess.WaitForExit(100)) {
        foreach ($outputPath in @($stdoutPath, $stderrPath)) {
            if (Test-Path -LiteralPath $outputPath -PathType Leaf) {
                $outputItem = Get-Item -LiteralPath $outputPath -Force
                if (($outputItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or $outputItem.Length -gt 65536) {
                    $testFailure = 'native fixture test output exceeded its bound'
                }
            }
        }
        if ($null -eq $testFailure -and [DateTime]::UtcNow -ge $deadline) {
            $testFailure = 'native fixture test exceeded its external deadline'
        }
        if ($null -ne $testFailure) { break }
    }
    if ($null -ne $testFailure) {
        if (-not $testProcess.HasExited) { $testProcess.Kill($true) }
        if (-not $testProcess.WaitForExit(5000)) { Fail 'native fixture test could not be reaped' }
        $testProcessReaped = $true
        Fail $testFailure
    }
    $testProcessReaped = $true
    $exitCode = $testProcess.ExitCode
    foreach ($outputPath in @($stdoutPath, $stderrPath)) {
        $outputItem = Get-Item -LiteralPath $outputPath -Force
        if ($outputItem.PSIsContainer -or ($outputItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or
            $outputItem.Length -gt 65536) {
            Fail 'native fixture test output is linked, missing, or oversized'
        }
    }
    if ($exitCode -ne 0) { Fail 'native fixture assertions failed' }
    $fixturePassed = $true
} finally {
    $cleanupFailed = $false
    if ($null -ne $testProcess) {
        if (-not $testProcessReaped) {
            try {
                if (-not $testProcess.HasExited) { $testProcess.Kill($true) }
                if (-not $testProcess.WaitForExit(5000)) { $cleanupFailed = $true }
                else { $testProcessReaped = $true }
            } catch {
                $cleanupFailed = $true
            }
        }
        $testProcess.Dispose()
    }
    if ($registrationAttempted) {
        & wevtutil.exe um $manifest 1>$null 2>$null
        $providerMissing = (Test-WevtMissing 'provider' $provider)
        $systemMissing = (Test-WevtMissing 'channel' $systemChannel)
        $applicationMissing = (Test-WevtMissing 'channel' $applicationChannel)
        if (-not (Test-AllMissing $providerMissing $systemMissing $applicationMissing)) { $cleanupFailed = $true }
    }
    if ($null -ne $ownedRoot -and (Test-Path -LiteralPath $ownedRoot -PathType Container)) {
        $candidate = [IO.Path]::GetFullPath($ownedRoot)
        $parent = [IO.Directory]::GetParent($candidate).FullName.TrimEnd([IO.Path]::DirectorySeparatorChar)
        $markerPath = Join-Path $candidate $markerName
        $candidateItem = Get-Item -LiteralPath $candidate -Force
        $markerItem = if (Test-Path -LiteralPath $markerPath -PathType Leaf) { Get-Item -LiteralPath $markerPath -Force } else { $null }
        if ($parent -cne $runnerRoot -or -not $candidateItem.PSIsContainer -or
            ($candidateItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or $null -eq $markerItem -or
            ($markerItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or
            [IO.File]::ReadAllText($markerPath, [Text.Encoding]::UTF8) -cne $marker) {
            $cleanupFailed = $true
        } elseif (-not $cleanupFailed) {
            Remove-Item -LiteralPath $candidate -Recurse -Force
        }
    }
    if ($cleanupFailed) { Fail 'owned fixture cleanup was not confirmed' }
}
if ($fixturePassed) { Write-Output 'Owned Windows WEVTAPI fixture passed without production-channel access.' }
