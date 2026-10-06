param([switch]$Desktop)
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
    $env:ARCOURT_BROWSER_SMOKE = '1'
    $env:ARCOURT_DOM_FIXTURES = '1'
    $env:ARCOURT_CLI_FIXTURES = '1'
    Invoke-Native go @('test', './arcourt', '-run', '^Test(BrowserSmoke|DocketDOMFixtures)$', '-v', '-count=1', '-timeout=180s')
    Invoke-Native go @('test', './cmd/arcourt-download', '-run', '^TestCLIBrowserFixtures$', '-v', '-count=1', '-timeout=180s')
    if ($Desktop) {
        if ($env:OS -ne 'Windows_NT') { throw 'Desktop smoke requires an interactive Windows session.' }
        & (Join-Path $PSScriptRoot 'build-desktop.ps1')
        & (Join-Path $PSScriptRoot 'build-desktop.ps1') -Fixture
        $env:ARCOURT_DESKTOP_SMOKE = '1'
        Push-Location (Join-Path $repo 'desktop')
        try { Invoke-Native go @('test', '.', '-run', '^TestDesktop(GUI|Startup)$', '-v', '-count=1', '-timeout=300s') }
        finally { Pop-Location }
    }
} finally {
    Pop-Location
    foreach ($name in $names) { [Environment]::SetEnvironmentVariable($name, $saved[$name], 'Process') }
}
