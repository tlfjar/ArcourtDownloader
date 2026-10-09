param(
    [switch]$Live,
    [ValidateSet('v3', 'v4')][string]$Corpus = 'v3',
    [ValidateSet('openai', 'xai', 'anthropic', 'google')][string]$Provider,
    [string]$Model,
    [int]$MaxCalls,
    [int]$MaxRequestBytes,
    [switch]$AcknowledgeCharges,
    [string]$ReportPath
)

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
if (-not $ReportPath) {
    $name = if ($Corpus -eq 'v3') {
        if ($Live) { 'naming-benchmark-live.json' } else { 'naming-benchmark-offline.json' }
    } elseif ($Live) { 'naming-benchmark-v4-live.json' } else { 'naming-benchmark-v4-offline.json' }
    $ReportPath = Join-Path $repo (Join-Path 'build' $name)
} elseif (-not [System.IO.Path]::IsPathRooted($ReportPath)) {
    $ReportPath = Join-Path $repo $ReportPath
}
$ReportPath = [System.IO.Path]::GetFullPath($ReportPath)

if ($Live) {
    if (-not $AcknowledgeCharges) { throw 'Live synthetic evaluation requires -AcknowledgeCharges.' }
    if ([string]::IsNullOrWhiteSpace($Provider) -or [string]::IsNullOrWhiteSpace($Model)) {
        throw 'Live synthetic evaluation requires -Provider and -Model.'
    }
    if ($MaxCalls -le 0 -or $MaxCalls -gt 512) { throw '-MaxCalls must be between 1 and 512.' }
    if ($MaxRequestBytes -le 0 -or $MaxRequestBytes -gt 1048576) {
        throw '-MaxRequestBytes must be between 1 and 1048576 (1 MiB).'
    }
    Write-Host "Live synthetic evaluation: $Provider / $Model, corpus $Corpus. Only generated fixture excerpts leave this computer. Requests may incur charges."
    Write-Host "Hard limits: $MaxCalls transmitted calls, $MaxRequestBytes total serialized request bytes, and 96 output tokens per call."
    $secureKey = Read-Host 'Enter the provider API key for this run' -AsSecureString
    $keyPointer = [IntPtr]::Zero
    try {
        $keyPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secureKey)
        $plainKey = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($keyPointer)
    } finally {
        if ($keyPointer -ne [IntPtr]::Zero) {
            [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($keyPointer)
        }
    }
    if ([string]::IsNullOrWhiteSpace($plainKey)) { throw 'A key must be intentionally supplied for the live run.' }
} elseif ($Provider -or $Model -or $MaxCalls -or $MaxRequestBytes -or $AcknowledgeCharges) {
    throw 'Provider, model, budgets, and charge acknowledgement are live-only options; add -Live for a paid synthetic run.'
}

$environmentNames = @(
    'GOWORK', 'GOFLAGS', 'ARCOURT_NAMING_BENCH', 'ARCOURT_BENCH_CORPUS', 'ARCOURT_BENCH_REPORT',
    'ARCOURT_BENCH_ACK', 'ARCOURT_BENCH_PROVIDER', 'ARCOURT_BENCH_MODEL',
    'ARCOURT_BENCH_MAX_CALLS', 'ARCOURT_BENCH_MAX_REQUEST_BYTES', 'ARCOURT_BENCH_KEY'
)
$savedEnvironment = @{}
foreach ($name in $environmentNames) {
    $savedEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}

Push-Location $repo
try {
    $env:GOWORK = 'off'
    $env:GOFLAGS = '-mod=readonly'
    $env:ARCOURT_NAMING_BENCH = if ($Live) { 'live' } else { 'offline' }
    $env:ARCOURT_BENCH_CORPUS = $Corpus
    $env:ARCOURT_BENCH_REPORT = $ReportPath
    if ($Live) {
        $env:ARCOURT_BENCH_ACK = 'I_ACCEPT_SYNTHETIC_API_CHARGES'
        $env:ARCOURT_BENCH_PROVIDER = $Provider
        $env:ARCOURT_BENCH_MODEL = $Model
        $env:ARCOURT_BENCH_MAX_CALLS = [string]$MaxCalls
        $env:ARCOURT_BENCH_MAX_REQUEST_BYTES = [string]$MaxRequestBytes
        $env:ARCOURT_BENCH_KEY = $plainKey
        $plainKey = $null
    } else {
        foreach ($name in @('ARCOURT_BENCH_ACK', 'ARCOURT_BENCH_PROVIDER', 'ARCOURT_BENCH_MODEL', 'ARCOURT_BENCH_MAX_CALLS', 'ARCOURT_BENCH_MAX_REQUEST_BYTES', 'ARCOURT_BENCH_KEY')) {
            [Environment]::SetEnvironmentVariable($name, $null, 'Process')
        }
    }
    & go test ./arcourt -run '^TestNamingBenchmark$' -count=1 -v -timeout=60m
    if ($LASTEXITCODE -ne 0) { throw "Naming benchmark failed with exit code $LASTEXITCODE." }
    Write-Host "Report: $ReportPath"
} finally {
    [Environment]::SetEnvironmentVariable('ARCOURT_BENCH_KEY', $null, 'Process')
    foreach ($name in $environmentNames) {
        [Environment]::SetEnvironmentVariable($name, $savedEnvironment[$name], 'Process')
    }
    Pop-Location
}
