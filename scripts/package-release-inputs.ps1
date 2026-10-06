#requires -Version 7.4
param(
    [Parameter(Mandatory)][string]$Tag,
    [Parameter(Mandatory)][ValidatePattern('^[1-9][0-9]*$')][string]$BuildAttempt,
    [Parameter(Mandatory)][ValidatePattern('^[1-9][0-9]*$')][string]$ArtifactId,
    [Parameter(Mandatory)][ValidatePattern('^[0-9a-f]{64}$')][string]$ArtifactDigest
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'workflow-common.ps1')
$identity = & (Join-Path $PSScriptRoot 'resolve-build-info.ps1') -Release -Tag $Tag
$inputs = Join-Path $script:ReleaseRepo 'build/unsigned'
& (Join-Path $PSScriptRoot 'get-workflow-artifact.ps1') -ArtifactId $ArtifactId -Digest $ArtifactDigest -Name "unsigned-$env:GITHUB_RUN_ID-$BuildAttempt" -Destination $inputs -Files @('ArcourtDownloader.exe','arcourt-download.exe','build-manifest.json')
Assert-InputManifest $inputs $identity $env:GITHUB_REPOSITORY $env:GITHUB_RUN_ID $BuildAttempt
foreach ($name in @('ArcourtDownloader.exe','arcourt-download.exe')) { Assert-UnsignedExecutable (Join-Path $inputs $name) }
if (Test-Path -LiteralPath (Join-Path $script:ReleaseRepo 'build/package-inputs')) { throw 'Package input directory must be new.' }
$packageInputs = New-BuildDirectory $script:ReleaseRepo 'package-inputs'
foreach ($name in @('ArcourtDownloader.exe','arcourt-download.exe')) { Copy-Item -LiteralPath (Join-Path $inputs $name) -Destination $packageInputs }
Assert-FileSet $packageInputs @('ArcourtDownloader.exe','arcourt-download.exe')
& (Join-Path $PSScriptRoot 'generate-notices.ps1') -Check
$assets = Join-Path $script:ReleaseRepo "build/releases/$Tag"
& (Join-Path $PSScriptRoot 'package-windows.ps1') -UnsignedRelease -Tag $Tag -BinariesPath $packageInputs -OutputPath $assets
& (Join-Path $PSScriptRoot 'verify-release-assets.ps1') -AssetsPath $assets -Tag $Tag -UnsignedRelease

# Check the final ZIP bytes against the manifest from the pinned build artifact.
Assert-CandidateBuildBinaries (Join-Path $assets "ArcourtDownloader-$Tag-windows-amd64.zip") (Join-Path $inputs 'build-manifest.json')
"attempt=$env:GITHUB_RUN_ATTEMPT" >> $env:GITHUB_OUTPUT
