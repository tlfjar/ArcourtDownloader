param([switch]$CoreOnly, [switch]$Race)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')
$repo = Split-Path -Parent $PSScriptRoot
$names = @('GOWORK', 'GOFLAGS', 'ARCOURT_BROWSER_SMOKE', 'ARCOURT_DOM_FIXTURES', 'ARCOURT_CLI_FIXTURES', 'ARCOURT_DESKTOP_SMOKE')
$saved = @{}
foreach ($name in $names) { $saved[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
Push-Location $repo
try {
    $env:GOWORK = 'off'
    $env:GOFLAGS = '-mod=readonly'
    foreach ($name in $names | Where-Object { $_ -like 'ARCOURT_*' }) { [Environment]::SetEnvironmentVariable($name, $null, 'Process') }
    Invoke-Native go @('mod', 'verify')
    $testArgs = @('test', './...', '-count=1', '-timeout=180s')
    if ($Race) { $testArgs += '-race' }
    Invoke-Native go $testArgs
    Invoke-Native go @('vet', './...')
    Invoke-Native go @('build', './...')
    $bin = New-BuildDirectory $repo 'bin'
    $exe = if ($env:OS -eq 'Windows_NT') { 'arcourt-download.exe' } else { 'arcourt-download' }
    Invoke-Native go @('build', '-trimpath', '-o', (Join-Path $bin $exe), './cmd/arcourt-download')
    if (-not $CoreOnly) {
        Push-Location (Join-Path $repo 'desktop/frontend')
        try {
            Invoke-Native npm @('ci', '--ignore-scripts')
            Invoke-Native npm @('test')
            Invoke-Native npm @('run', 'build')
        } finally { Pop-Location }
        Push-Location (Join-Path $repo 'desktop')
        try {
            Invoke-Native go @('mod', 'verify')
            if ($env:OS -eq 'Windows_NT') {
                Invoke-Native go $testArgs
                Invoke-Native go @('vet', './...')
                # Avoid an unversioned desktop.exe in the source directory.
                Invoke-Native go @('build', '-o', (Join-Path $bin 'desktop-check.exe'), '.')
            } else {
                Invoke-Native go @('test', './internal/app', '-count=1', '-timeout=180s')
                Invoke-Native go @('vet', './internal/app')
            }
        } finally { Pop-Location }
    }
} finally {
    Pop-Location
    foreach ($name in $names) { [Environment]::SetEnvironmentVariable($name, $saved[$name], 'Process') }
}
