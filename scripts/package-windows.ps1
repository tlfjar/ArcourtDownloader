#requires -Version 7.4
param([switch]$UnsignedRelease, [string]$Tag, [string]$BinariesPath, [string]$OutputPath)
$ErrorActionPreference = 'Stop'
if ($env:OS -ne 'Windows_NT') { throw 'Packaging requires Windows x64 build tooling.' }
. (Join-Path $PSScriptRoot 'release-common.ps1')
$repo = $script:ReleaseRepo
if (-not $UnsignedRelease -and ($Tag -or $BinariesPath -or $OutputPath)) { throw 'Tag, BinariesPath and OutputPath require -UnsignedRelease.' }
if ($UnsignedRelease -and (-not $Tag -or -not $BinariesPath -or -not $OutputPath)) { throw 'UnsignedRelease requires Tag, BinariesPath and an empty OutputPath.' }
$previous = @{}
foreach ($key in @('GOWORK','GOOS','GOARCH','CGO_ENABLED','GOFLAGS','GOEXPERIMENT','GOTOOLCHAIN')) { $previous[$key] = [Environment]::GetEnvironmentVariable($key) }
try {
    $env:GOWORK='off'; $env:GOOS='windows'; $env:GOARCH='amd64'; $env:CGO_ENABLED='0'; $env:GOFLAGS='-mod=readonly'; $env:GOEXPERIMENT=''
    $identity = & (Join-Path $PSScriptRoot 'resolve-build-info.ps1') -Release:$UnsignedRelease -Tag $Tag
    if ($UnsignedRelease) {
        $destination = [IO.Path]::GetFullPath($OutputPath)
        $expected = [IO.Path]::GetFullPath((Join-Path $repo "build/releases/$Tag"))
        if ($destination.TrimEnd('\','/') -ine $expected) { throw 'Release OutputPath must be build/releases/<tag> in this checkout.' }
        Assert-OrdinaryPath $destination
        if (Test-Path -LiteralPath $destination) {
            if (-not (Test-Path -LiteralPath $destination -PathType Container) -or @(Get-ChildItem -LiteralPath $destination -Force).Count) { throw 'OutputPath must be an empty directory; reused outputs are forbidden.' }
        }
        $inputs = [IO.Path]::GetFullPath($BinariesPath)
        Assert-FileSet $inputs @('ArcourtDownloader.exe','arcourt-download.exe')
        foreach ($name in @('ArcourtDownloader.exe','arcourt-download.exe')) { Assert-UnsignedExecutable (Join-Path $inputs $name) }
        $assetTag = $Tag
    } else {
        $assetTag = 'development'
        $destination = New-BuildDirectory $repo ('packages/' + [DateTime]::UtcNow.ToString('yyyyMMdd-HHmmss') + '-' + [Guid]::NewGuid().ToString('N').Substring(0,8))
    }
    if (-not (Test-Path -LiteralPath (Join-Path $repo 'THIRD_PARTY_NOTICES.txt') -PathType Leaf)) { throw 'Missing THIRD_PARTY_NOTICES.txt.' }
    # Use the exact reviewed runtime for both binaries and their evidence,
    # even when PATH or the caller's GOTOOLCHAIN selects a newer Go release.
    $env:GOTOOLCHAIN = Get-ReviewedGoToolchain
    Push-Location $repo
    try { Invoke-Native go @('version') } finally { Pop-Location }
    $stage = New-BuildDirectory $repo ('staging/' + [Guid]::NewGuid().ToString('N'))
    if ($UnsignedRelease) {
        foreach ($name in @('ArcourtDownloader.exe','arcourt-download.exe')) { Copy-Item -LiteralPath (Join-Path $inputs $name) -Destination $stage }
        # Recreate frontend evidence only; never rebuild or modify workflow executables.
        Push-Location (Join-Path $repo 'desktop/frontend')
        try { Invoke-Native node @('build.mjs') } finally { Pop-Location }
    } else {
        & (Join-Path $PSScriptRoot 'build-desktop.ps1')
        Copy-Item -LiteralPath (Join-Path $repo 'desktop/build/bin/ArcourtDownloader.exe') -Destination $stage
        $cliIdentity = & (Join-Path $PSScriptRoot 'build-cli.ps1') -OutputPath (Join-Path $stage 'arcourt-download.exe')
        if ($cliIdentity.commit -cne $identity.commit) { throw 'Source changed during local build.' }
    }
    New-Item -ItemType Directory -Path (Join-Path $stage 'docs') | Out-Null
    foreach ($name in @('LICENSE','THIRD_PARTY_NOTICES.txt','README.md','SUPPORT.md','SECURITY.md','docs/ai-connector-evaluation.md','docs/ai-document-naming.md','docs/command-line-workflow.md','docs/releasing.md')) {
        Write-Utf8 (Join-Path $stage $name) ([IO.File]::ReadAllText((Join-Path $repo $name)))
    }
    Write-Utf8 (Join-Path $stage 'BUILD.json') (($identity | ConvertTo-Json) + "`n")
    Push-Location $repo
    try { Invoke-Native go @('mod','verify'); Push-Location desktop; try { Invoke-Native go @('mod','verify') } finally { Pop-Location } } finally { Pop-Location }
    $lock = Get-Content -LiteralPath (Join-Path $repo 'desktop/frontend/package-lock.json') -Raw | ConvertFrom-Json -AsHashtable
    if ($lock.packages.Count -ne 1 -or -not $lock.packages.Contains('')) { throw 'Frontend dependency graph changed; review licenses and SBOM generator.' }
    Invoke-Evidence $stage $identity (Join-Path $stage 'SBOM.spdx.json')
    Write-Checksums $stage @($script:PayloadNames | Where-Object { $_ -ne 'FILE_SHA256SUMS.txt' }) 'FILE_SHA256SUMS.txt'
    Assert-Payload $stage $identity
    if ($UnsignedRelease) {
        & (Join-Path $PSScriptRoot 'resolve-build-info.ps1') -Release -Tag $Tag | Out-Null
        foreach ($name in @('ArcourtDownloader.exe','arcourt-download.exe')) { if ((Get-SHA256 (Join-Path $inputs $name)) -cne (Get-SHA256 (Join-Path $stage $name))) { throw 'Build input changed during packaging.' } }
        New-Item -ItemType Directory -Path $destination -Force | Out-Null
        if (@(Get-ChildItem -LiteralPath $destination -Force).Count) { throw 'OutputPath was reused during packaging.' }
    }
    $zipName = "ArcourtDownloader-$assetTag-windows-amd64.zip"
    $sbomName = "ArcourtDownloader-$assetTag-SBOM.spdx.json"
    $stream = [IO.File]::Open((Join-Path $destination $zipName), [IO.FileMode]::CreateNew)
    try {
        $zip = [IO.Compression.ZipArchive]::new($stream, [IO.Compression.ZipArchiveMode]::Create, $true)
        try { foreach ($name in ($script:PayloadNames | Sort-Object -CaseSensitive)) { [IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip, (Join-Path $stage $name), $name) | Out-Null } } finally { $zip.Dispose() }
    } finally { $stream.Dispose() }
    [IO.File]::Copy((Join-Path $stage 'SBOM.spdx.json'), (Join-Path $destination $sbomName), $false)
    Write-Checksums $destination @($zipName,$sbomName) 'SHA256SUMS.txt'
    if ($UnsignedRelease) { & (Join-Path $PSScriptRoot 'verify-release-assets.ps1') -AssetsPath $destination -Tag $assetTag -UnsignedRelease }
    else { & (Join-Path $PSScriptRoot 'verify-release-assets.ps1') -AssetsPath $destination -Tag $assetTag -Local }
    Write-Output "Package assets: $destination"
} finally { foreach ($key in $previous.Keys) { [Environment]::SetEnvironmentVariable($key,$previous[$key]) } }
