#requires -Version 7.4
param(
    [Parameter(Mandatory)][string]$Tag,
    [Parameter(Mandatory)][string]$BuildAttempt,
    [Parameter(Mandatory)][string]$ArtifactId,
    [Parameter(Mandatory)][string]$ArtifactDigest
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'workflow-common.ps1')
$identity = & (Join-Path $PSScriptRoot 'resolve-build-info.ps1') -Release -Tag $Tag
$unsigned = Join-Path $script:ReleaseRepo 'build/unsigned'
$signed = Join-Path $script:ReleaseRepo 'build/signed'
Assert-InputManifest $unsigned $identity $env:GITHUB_REPOSITORY $env:GITHUB_RUN_ID $BuildAttempt
Assert-FileSet $signed @('ArcourtDownloader.exe','arcourt-download.exe','signing-evidence.json')
$evidence = Get-Content -LiteralPath (Join-Path $signed 'signing-evidence.json') -Raw | ConvertFrom-Json
if ($evidence.schemaVersion -ne 1 -or $evidence.request.artifactId -cne $ArtifactId -or $evidence.request.artifactDigest -cne $ArtifactDigest) { throw 'Signed output artifact association mismatch.' }
$manifest = Get-Content -LiteralPath (Join-Path $unsigned 'build-manifest.json') -Raw | ConvertFrom-Json | ConvertTo-Json -Depth 10 -Compress
if (($evidence.request.build | ConvertTo-Json -Depth 10 -Compress) -cne $manifest) { throw 'Signed output build association mismatch.' }
Assert-SignedAssociation $unsigned $signed
$inputs = New-BuildDirectory $script:ReleaseRepo 'package-inputs'
foreach ($name in @('ArcourtDownloader.exe','arcourt-download.exe')) {
    if ($evidence.signedSha256.$name -cne (Get-SHA256 (Join-Path $signed $name))) { throw 'Signed evidence checksum mismatch.' }
    Copy-Item -LiteralPath (Join-Path $signed $name) -Destination $inputs
}
& (Join-Path $PSScriptRoot 'generate-notices.ps1') -Check
$assets = Join-Path $script:ReleaseRepo "build/releases/$Tag"
& (Join-Path $PSScriptRoot 'package-windows.ps1') -Release -Tag $Tag -SignedBinariesPath $inputs -OutputPath $assets
& (Join-Path $PSScriptRoot 'verify-release-assets.ps1') -AssetsPath $assets -Tag $Tag
"attempt=$env:GITHUB_RUN_ATTEMPT" >> $env:GITHUB_OUTPUT
