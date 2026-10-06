#requires -Version 7.4
param([Parameter(Mandatory)][string]$AssetsPath, [Parameter(Mandatory)][string]$Tag, [switch]$Local)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'release-common.ps1')
if ($env:OS -ne 'Windows_NT') { throw 'Authenticode verification requires Windows.' }
if ($Local -and $Tag -cne 'development') { throw 'Local verification only accepts the development tag.' }
$assets = [IO.Path]::GetFullPath($AssetsPath).TrimEnd('\','/')
$identity = if ($Local) { & (Join-Path $PSScriptRoot 'resolve-build-info.ps1') } else { & (Join-Path $PSScriptRoot 'resolve-build-info.ps1') -Release -Tag $Tag }
$zipName = "ArcourtDownloader-$Tag-windows-amd64.zip"
$sbomName = "ArcourtDownloader-$Tag-SBOM.spdx.json"
Assert-FileSet $assets @($zipName,$sbomName,'SHA256SUMS.txt')
Assert-Checksums $assets @($zipName,$sbomName) 'SHA256SUMS.txt'
$temporaryRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\','/')
$extract = Join-Path $temporaryRoot ('arcourt-release-' + [Guid]::NewGuid().ToString('N'))
Assert-OrdinaryPath $extract
New-Item -ItemType Directory -Path $extract | Out-Null
try {
    $zip = [IO.Compression.ZipFile]::OpenRead((Join-Path $assets $zipName))
    try {
        $seen = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
        # Check every entry before extraction: no traversal, ADS, duplicates,
        # directories, links, fixture binaries or unreviewed payloads.
        foreach ($entry in $zip.Entries) {
            if ($entry.FullName -cnotin $script:PayloadNames -or -not $seen.Add($entry.FullName)) { throw 'Unsafe/unexpected/duplicate ZIP entry.' }
            $type = ($entry.ExternalAttributes -shr 16) -band 0xF000
            if ($type -ne 0 -and $type -ne 0x8000) { throw 'Non-regular ZIP entry.' }
            if ($entry.ExternalAttributes -band 0x410) { throw 'ZIP directory/reparse attributes are forbidden.' }
            if ($entry.Length -gt 100MB) { throw 'Oversized ZIP entry.' }
        }
        if ($seen.Count -ne $script:PayloadNames.Count) { throw 'Missing archive payload (license/notice/SBOM required).' }
        New-Item -ItemType Directory -Path (Join-Path $extract 'docs') | Out-Null
        foreach ($entry in $zip.Entries) { [IO.Compression.ZipFileExtensions]::ExtractToFile($entry, (Join-Path $extract $entry.FullName), $false) }
    } finally { $zip.Dispose() }
    if ((Get-SHA256 (Join-Path $extract 'SBOM.spdx.json')) -cne (Get-SHA256 (Join-Path $assets $sbomName))) { throw 'Internal/external SBOM mismatch.' }
    Push-Location (Join-Path $script:ReleaseRepo 'desktop/frontend')
    try { Invoke-Native node @('build.mjs') } finally { Pop-Location }
    Assert-Payload $extract $identity -Local:$Local
    if ($Local) { Write-Output 'Verified unsigned development assets; public signature readiness is not established.' }
    else { Write-Output "Verified signed release assets for $Tag ($($identity.commit))." }
} finally {
    $resolved = [IO.Path]::GetFullPath($extract)
    if (-not $resolved.StartsWith($temporaryRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase) -or (Split-Path -Leaf $resolved) -notlike 'arcourt-release-*') { throw 'Unsafe temporary cleanup path.' }
    Assert-OrdinaryPath $resolved
    Remove-Item -LiteralPath $resolved -Recurse -Force
}
