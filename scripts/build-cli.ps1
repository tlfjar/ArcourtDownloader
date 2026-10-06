param([switch]$Release, [string]$Tag, [string]$Version, [string]$OutputPath)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')
$repo = Split-Path -Parent $PSScriptRoot
$identity = & (Join-Path $PSScriptRoot 'resolve-build-info.ps1') -Release:$Release -Tag $Tag -Version $Version
$previousWork = $env:GOWORK
$previousCGO = $env:CGO_ENABLED
$previousOS = $env:GOOS
$previousArch = $env:GOARCH
try {
    $env:GOWORK = 'off'
    if ($Release) { $env:CGO_ENABLED = '0'; $env:GOOS = 'windows'; $env:GOARCH = 'amd64' }
    if (-not $OutputPath) {
        $bin = New-BuildDirectory $repo 'bin'
        $OutputPath = Join-Path $bin 'arcourt-download.exe'
    }
    $flags = "-X github.com/tlfjar/ArcourtDownloader/buildinfo.Version=$($identity.version) -X github.com/tlfjar/ArcourtDownloader/buildinfo.Commit=$($identity.commit) -X github.com/tlfjar/ArcourtDownloader/buildinfo.Release=$($identity.release.ToString().ToLowerInvariant())"
    $flags += " -X github.com/tlfjar/ArcourtDownloader/buildinfo.Record=ARCOURT_BUILD_V1|$($identity.version)|$($identity.commit)|$($identity.release.ToString().ToLowerInvariant())|cli|END_ARCOURT_BUILD"
    Push-Location $repo
    try { Invoke-Native go @('build', '-mod=readonly', '-trimpath', '-ldflags', $flags, '-o', $OutputPath, './cmd/arcourt-download') }
    finally { Pop-Location }
} finally { $env:GOWORK = $previousWork; $env:CGO_ENABLED = $previousCGO; $env:GOOS = $previousOS; $env:GOARCH = $previousArch }
Write-Output $identity
