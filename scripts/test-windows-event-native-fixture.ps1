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

function Invoke-OwnedPublisher([string] $Path, [string] $Phase) {
    if ($Phase -cne 'before' -and $Phase -cne 'after') { Fail 'unknown publisher phase' }
    & $Path $Phase 1>$null 2>$null
    if ($LASTEXITCODE -eq 3) { Fail 'generated fixture descriptor was not enabled' }
    if ($LASTEXITCODE -eq 4) { Fail 'generated fixture descriptors did not match the fixed manifest' }
    if ($LASTEXITCODE -ne 0) { Fail 'owned fixture publisher failed' }
}

function Wait-OwnedRecords([string] $Phase) {
    if ($Phase -cne 'before' -and $Phase -cne 'after') { Fail 'unknown record readiness phase' }
    $expected = if ($Phase -ceq 'before') { 2 } else { 1 }
    $watch = [Diagnostics.Stopwatch]::StartNew()
    while ($watch.Elapsed -lt [TimeSpan]::FromSeconds(10)) {
        if ($Phase -ceq 'before') {
            $systemCount = [OwnedEventFixtureMetadataProbe]::SystemBeforeCount()
            $applicationCount = [OwnedEventFixtureMetadataProbe]::ApplicationBeforeCount()
            $systemIDCount = [OwnedEventFixtureMetadataProbe]::SystemBeforeIDCount()
            $applicationIDCount = [OwnedEventFixtureMetadataProbe]::ApplicationBeforeIDCount()
        } else {
            $systemCount = [OwnedEventFixtureMetadataProbe]::SystemAfterCount()
            $applicationCount = [OwnedEventFixtureMetadataProbe]::ApplicationAfterCount()
            $systemIDCount = [OwnedEventFixtureMetadataProbe]::SystemAfterIDCount()
            $applicationIDCount = [OwnedEventFixtureMetadataProbe]::ApplicationAfterIDCount()
        }
        if ($systemCount -lt 0 -or $applicationCount -lt 0 -or $systemIDCount -lt 0 -or $applicationIDCount -lt 0) {
            Fail 'owned fixture record readiness query failed'
        }
        if ($systemCount -gt $expected -or $applicationCount -gt $expected -or
            $systemIDCount -gt $expected -or $applicationIDCount -gt $expected) {
            Fail 'owned fixture record count exceeded its fixed bound'
        }
        if ($systemCount -eq $expected -and $applicationCount -eq $expected) { return }
        Start-Sleep -Milliseconds 25
    }
    if ($systemIDCount -eq $expected -and $applicationIDCount -eq $expected) {
        Fail 'owned fixture records were visible but the fixed provider filter did not match'
    }
    $systemLogCount = if ($Phase -ceq 'before') {
        [OwnedEventFixtureMetadataProbe]::SystemBeforeLogCount()
    } else {
        [OwnedEventFixtureMetadataProbe]::SystemAfterLogCount()
    }
    $applicationLogCount = if ($Phase -ceq 'before') {
        [OwnedEventFixtureMetadataProbe]::ApplicationBeforeLogCount()
    } else {
        [OwnedEventFixtureMetadataProbe]::ApplicationAfterLogCount()
    }
    if ($systemLogCount -lt 0 -or $applicationLogCount -lt 0) {
        Fail 'owned fixture channel record-count metadata query failed'
    }
    if ($systemLogCount -gt $expected -or $applicationLogCount -gt $expected) {
        Fail 'owned fixture channel record-count metadata exceeded its fixed bound'
    }
    if ($systemLogCount -eq $expected -and $applicationLogCount -eq $expected) {
        Fail 'owned fixture channels contained records outside the fixed event-ID queries'
    }
    if ($systemLogCount -gt 0 -or $applicationLogCount -gt 0) {
        Fail 'owned fixture channel record-count metadata remained partial at the fixed deadline'
    }
    if ($systemIDCount -gt 0 -or $applicationIDCount -gt 0) {
        Fail 'owned fixture record visibility remained partial at the fixed deadline'
    }
    Fail 'owned fixture channel record-count metadata remained zero at the fixed deadline'
}

function Test-WevtMissing([string] $Kind, [string] $Name) {
    if ($Kind -eq 'provider' -and $Name -ceq $provider) { $exitCode = [OwnedEventFixtureMetadataProbe]::Publisher() }
    elseif ($Kind -eq 'channel' -and $Name -ceq $systemChannel) { $exitCode = [OwnedEventFixtureMetadataProbe]::SystemChannel() }
    elseif ($Kind -eq 'channel' -and $Name -ceq $applicationChannel) { $exitCode = [OwnedEventFixtureMetadataProbe]::ApplicationChannel() }
    else { Fail 'unknown fixture registration kind' }
    if ($exitCode -eq 0) { return $false }
    if (Test-IsDocumentedMissing $Kind $exitCode) { return $true }
    Fail ("fixture metadata probe returned unexpected fixed-$Kind status $exitCode")
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
using System.Text;

public static class OwnedEventFixtureMetadataProbe
{
    private const string ProviderName = "BraidenM-HomeLabObserver-NativeFixture";
    private const string SystemChannelName = "BraidenM-HomeLabObserver/Fixture-System";
    private const string ApplicationChannelName = "BraidenM-HomeLabObserver/Fixture-Application";
    private const int MaxPublisherCount = 4096;
    private const int MaxPublisherCharacters = 2048;
    private const int ErrorNoMoreItems = 259;
    private const int MissingProvider = 15002;
    private const int EvtOpenChannelPath = 0x1;
    private const int EvtLogNumberOfLogRecords = 5;
    private const uint EvtVarTypeUInt64 = 10;
    private const int EvtQueryChannelPath = 0x1;
    private const int EvtQueryForwardDirection = 0x100;
    private const string SystemBeforeQuery = "*[System[Provider[@Name='BraidenM-HomeLabObserver-NativeFixture'] and (EventID=101 or EventID=102)]]";
    private const string ApplicationBeforeQuery = "*[System[Provider[@Name='BraidenM-HomeLabObserver-NativeFixture'] and (EventID=201 or EventID=202)]]";
    private const string SystemAfterQuery = "*[System[Provider[@Name='BraidenM-HomeLabObserver-NativeFixture'] and EventID=103]]";
    private const string ApplicationAfterQuery = "*[System[Provider[@Name='BraidenM-HomeLabObserver-NativeFixture'] and EventID=203]]";
    private const string SystemBeforeIDQuery = "*[System[(EventID=101 or EventID=102)]]";
    private const string ApplicationBeforeIDQuery = "*[System[(EventID=201 or EventID=202)]]";
    private const string SystemAfterIDQuery = "*[System[EventID=103]]";
    private const string ApplicationAfterIDQuery = "*[System[EventID=203]]";

    [DefaultDllImportSearchPaths(DllImportSearchPath.System32)]
    [DllImport("wevtapi.dll", ExactSpelling = true, SetLastError = true)]
    private static extern IntPtr EvtOpenPublisherEnum(IntPtr session, int flags);

    [DefaultDllImportSearchPaths(DllImportSearchPath.System32)]
    [DllImport("wevtapi.dll", CharSet = CharSet.Unicode, ExactSpelling = true, SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool EvtNextPublisherId(IntPtr publisherEnum, int bufferSize, [Out] StringBuilder buffer, out int bufferUsed);

    [DefaultDllImportSearchPaths(DllImportSearchPath.System32)]
    [DllImport("wevtapi.dll", CharSet = CharSet.Unicode, ExactSpelling = true, SetLastError = true)]
    private static extern IntPtr EvtOpenChannelConfig(IntPtr session, string channelPath, int flags);

    [DefaultDllImportSearchPaths(DllImportSearchPath.System32)]
    [DllImport("wevtapi.dll", CharSet = CharSet.Unicode, ExactSpelling = true, SetLastError = true)]
    private static extern IntPtr EvtOpenLog(IntPtr session, string path, int flags);

    [DefaultDllImportSearchPaths(DllImportSearchPath.System32)]
    [DllImport("wevtapi.dll", ExactSpelling = true, SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool EvtGetLogInfo(IntPtr log, int propertyId, int propertyValueBufferSize,
        out EvtVariant propertyValueBuffer, out int propertyValueBufferUsed);

    [DefaultDllImportSearchPaths(DllImportSearchPath.System32)]
    [DllImport("wevtapi.dll", CharSet = CharSet.Unicode, ExactSpelling = true, SetLastError = true)]
    private static extern IntPtr EvtQuery(IntPtr session, string path, string query, int flags);

    [DefaultDllImportSearchPaths(DllImportSearchPath.System32)]
    [DllImport("wevtapi.dll", ExactSpelling = true, SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool EvtNext(IntPtr resultSet, int eventsSize, [Out] IntPtr[] events, int timeout, int flags, out int returned);

    [DefaultDllImportSearchPaths(DllImportSearchPath.System32)]
    [DllImport("wevtapi.dll", ExactSpelling = true, SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool EvtClose(IntPtr handle);

    [StructLayout(LayoutKind.Explicit, Size = 16)]
    private struct EvtVariant
    {
        [FieldOffset(0)] public ulong UInt64Value;
        [FieldOffset(8)] public uint Count;
        [FieldOffset(12)] public uint Type;
    }

    public static int Publisher()
    {
        IntPtr publisherEnum = EvtOpenPublisherEnum(IntPtr.Zero, 0);
        if (publisherEnum == IntPtr.Zero) return 1;
        int result = 1;
        long started = Environment.TickCount64;
        try {
            for (int index = 0; index < MaxPublisherCount; index++) {
                if (Environment.TickCount64 - started > 5000) break;
                StringBuilder buffer = new StringBuilder(MaxPublisherCharacters);
                int used;
                bool found = EvtNextPublisherId(publisherEnum, MaxPublisherCharacters, buffer, out used);
                int nextError = Marshal.GetLastWin32Error();
                if (Environment.TickCount64 - started > 5000) break;
                if (!found) {
                    if (nextError == ErrorNoMoreItems) result = MissingProvider;
                    break;
                }
                if (used < 1 || used > MaxPublisherCharacters || buffer.Length < 1 || buffer.Length >= MaxPublisherCharacters) break;
                if (String.Equals(buffer.ToString(), ProviderName, StringComparison.Ordinal)) {
                    result = 0;
                    break;
                }
            }
        } finally {
            if (!EvtClose(publisherEnum)) result = 1;
        }
        return result;
    }
    public static int SystemChannel() { return Probe(EvtOpenChannelConfig(IntPtr.Zero, SystemChannelName, 0)); }
    public static int ApplicationChannel() { return Probe(EvtOpenChannelConfig(IntPtr.Zero, ApplicationChannelName, 0)); }
    public static int SystemBeforeCount() { return Count(SystemChannelName, SystemBeforeQuery, 2); }
    public static int ApplicationBeforeCount() { return Count(ApplicationChannelName, ApplicationBeforeQuery, 2); }
    public static int SystemAfterCount() { return Count(SystemChannelName, SystemAfterQuery, 1); }
    public static int ApplicationAfterCount() { return Count(ApplicationChannelName, ApplicationAfterQuery, 1); }
    public static int SystemBeforeIDCount() { return Count(SystemChannelName, SystemBeforeIDQuery, 2); }
    public static int ApplicationBeforeIDCount() { return Count(ApplicationChannelName, ApplicationBeforeIDQuery, 2); }
    public static int SystemAfterIDCount() { return Count(SystemChannelName, SystemAfterIDQuery, 1); }
    public static int ApplicationAfterIDCount() { return Count(ApplicationChannelName, ApplicationAfterIDQuery, 1); }
    public static int SystemBeforeLogCount() { return LogCount(SystemChannelName, 2); }
    public static int ApplicationBeforeLogCount() { return LogCount(ApplicationChannelName, 2); }
    public static int SystemAfterLogCount() { return LogCount(SystemChannelName, 1); }
    public static int ApplicationAfterLogCount() { return LogCount(ApplicationChannelName, 1); }

    private static int Probe(IntPtr handle)
    {
        if (handle == IntPtr.Zero) {
            int openError = Marshal.GetLastWin32Error();
            return openError == 0 ? 1 : openError;
        }
        if (!EvtClose(handle)) return 1;
        return 0;
    }

    private static int Count(string channel, string queryText, int expected)
    {
        IntPtr query = EvtQuery(IntPtr.Zero, channel, queryText, EvtQueryChannelPath | EvtQueryForwardDirection);
        if (query == IntPtr.Zero) return -1;
        int count = 0;
        try {
            while (count <= expected) {
                IntPtr[] events = new IntPtr[1];
                int returned;
                bool found = EvtNext(query, 1, events, 0, 0, out returned);
                int nextError = Marshal.GetLastWin32Error();
                if (!found) {
                    if (events[0] != IntPtr.Zero) {
                        EvtClose(events[0]);
                        count = -1;
                        break;
                    }
                    if (nextError == ErrorNoMoreItems && returned == 0) break;
                    count = -1;
                    break;
                }
                if (returned != 1 || events[0] == IntPtr.Zero) {
                    if (events[0] != IntPtr.Zero) EvtClose(events[0]);
                    count = -1;
                    break;
                }
                if (!EvtClose(events[0])) {
                    count = -1;
                    break;
                }
                count++;
            }
        } finally {
            if (!EvtClose(query)) count = -1;
        }
        return count;
    }

    private static int LogCount(string channel, int expected)
    {
        IntPtr log = EvtOpenLog(IntPtr.Zero, channel, EvtOpenChannelPath);
        if (log == IntPtr.Zero) return -1;
        int result = -1;
        try {
            EvtVariant value;
            int used;
            if (!EvtGetLogInfo(log, EvtLogNumberOfLogRecords, 16, out value, out used)) return -1;
            if (used != 16 || value.Count != 0 || value.Type != EvtVarTypeUInt64) return -1;
            result = value.UInt64Value > (ulong)expected ? expected + 1 : (int)value.UInt64Value;
        } finally {
            if (!EvtClose(log)) result = -1;
        }
        return result;
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

    foreach ($tool in @('mc.exe', 'rc.exe', 'link.exe', 'cl.exe', 'go.exe', 'wevtutil.exe')) {
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

    $publisherSource = Join-Path $PSScriptRoot '..\internal\eventnative\testdata\fixturepublisher\main_windows.c'
    Assert-PrivateRegular $publisherSource 65536
    $ownedPublisherSource = Join-Path $ownedRoot 'fixturepublisher.c'
    [IO.File]::Copy([IO.Path]::GetFullPath($publisherSource), $ownedPublisherSource, $false)
    $publisher = Join-Path $ownedRoot 'fixture-publisher.exe'
    Push-Location -LiteralPath $ownedRoot
    try {
        Invoke-Quiet 'cl.exe' @('/nologo', '/W4', '/WX', '/guard:cf', '/DUNICODE', '/D_UNICODE',
            ('/Fe:' + $publisher), 'fixturepublisher.c', '/link', 'advapi32.lib')
    } finally {
        Pop-Location
    }
    Assert-PrivateRegular $publisher (20 * 1024 * 1024)

    $registrationAttempted = $true
    Invoke-Quiet 'wevtutil.exe' @('im', $manifest, ('/rf:' + $resourceDLL), ('/mf:' + $resourceDLL))
    $providerMissing = (Test-WevtMissing 'provider' $provider)
    $systemMissing = (Test-WevtMissing 'channel' $systemChannel)
    $applicationMissing = (Test-WevtMissing 'channel' $applicationChannel)
    if (Test-AnyMissing $providerMissing $systemMissing $applicationMissing) {
        Fail 'fixture registration was not observable'
    }
    Invoke-Quiet 'wevtutil.exe' @('sl', $systemChannel, '/e:true')
    Invoke-Quiet 'wevtutil.exe' @('sl', $applicationChannel, '/e:true')
    Invoke-OwnedPublisher $publisher 'before'
    Wait-OwnedRecords 'before'

    $systemBefore = Join-Path $ownedRoot 'system-before.evtx'
    $applicationBefore = Join-Path $ownedRoot 'application-before.evtx'
    Invoke-Quiet 'wevtutil.exe' @('epl', $systemChannel, $systemBefore, "/q:*[System[Provider[@Name='$provider'] and (EventID=101 or EventID=102)]]")
    Invoke-Quiet 'wevtutil.exe' @('epl', $applicationChannel, $applicationBefore, "/q:*[System[Provider[@Name='$provider'] and (EventID=201 or EventID=202)]]")

    Invoke-Quiet 'wevtutil.exe' @('cl', $systemChannel)
    Invoke-Quiet 'wevtutil.exe' @('cl', $applicationChannel)
    Invoke-OwnedPublisher $publisher 'after'
    Wait-OwnedRecords 'after'
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
    if ($exitCode -ne 0) {
        $allowedStages = @(
            'system-before-query-open',
            'system-before-next',
            'system-before-selected-types',
            'system-before-selected-values',
            'system-before-forward-eof',
            'application-before-query-open',
            'application-before-next',
            'application-before-selected-types',
            'application-before-selected-values',
            'application-before-forward-eof',
            'system-before-native-next-error',
            'system-before-native-next-eof',
            'system-before-native-next-invalid',
            'system-before-native-next-record',
            'system-before-render-start',
            'system-before-render-error',
            'system-before-render-ok',
            'application-before-native-next-error',
            'application-before-native-next-eof',
            'application-before-native-next-invalid',
            'application-before-native-next-record',
            'application-before-render-start',
            'application-before-render-error',
            'application-before-render-ok',
            'system-after-native-next-error',
            'system-after-native-next-eof',
            'system-after-native-next-invalid',
            'system-after-native-next-record',
            'system-after-render-start',
            'system-after-render-error',
            'system-after-render-ok',
            'system-reverse-tail',
            'system-bookmark-roundtrip',
            'system-reset',
            'handle-accounting'
        )
        $stage = 'unknown'
        foreach ($line in [IO.File]::ReadLines($stdoutPath, [Text.Encoding]::UTF8)) {
            if ($line -match 'owned-fixture-stage: ([a-z-]+)') {
                $candidateStage = $Matches[1]
                if ($allowedStages -ccontains $candidateStage) { $stage = $candidateStage }
            }
        }
        Fail ("native fixture assertions failed at fixed stage $stage")
    }
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
