#requires -Version 7.4
param(
    [Parameter(Mandatory)][string]$Tag,
    [Parameter(Mandatory)][string]$BuildAttempt,
    [Parameter(Mandatory)][string]$ArtifactId,
    [Parameter(Mandatory)][string]$ArtifactDigest
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'workflow-common.ps1')
$policy = Assert-SigningConfiguration
$identity = & (Join-Path $PSScriptRoot 'resolve-build-info.ps1') -Release -Tag $Tag
$unsigned = Join-Path $script:ReleaseRepo 'build/unsigned'
Assert-InputManifest $unsigned $identity $env:GITHUB_REPOSITORY $env:GITHUB_RUN_ID $BuildAttempt
$manifestHash = Get-SHA256 (Join-Path $unsigned 'build-manifest.json')
$request = [ordered]@{schemaVersion=1;artifactId=$ArtifactId;artifactDigest=$ArtifactDigest;build=(Get-Content -LiteralPath (Join-Path $unsigned 'build-manifest.json') -Raw | ConvertFrom-Json)}
$requestPath = Join-Path $script:ReleaseRepo 'build/signing-request.json'
Write-Utf8 $requestPath (($request | ConvertTo-Json -Depth 10) + "`n")
$requestHash = Get-SHA256 $requestPath
$signed = Join-Path $script:ReleaseRepo 'build/provider-output'
if (Test-Path -LiteralPath $signed) { throw 'Provider output directory must be new.' }
& (Join-Path $PSScriptRoot 'invoke-signing-provider.ps1') -RequestPath $requestPath -UnsignedPath $unsigned -OutputPath $signed
Assert-FileSet $signed @('ArcourtDownloader.exe','arcourt-download.exe','provider-receipt.json')
$receipt = Get-Content -LiteralPath (Join-Path $signed 'provider-receipt.json') -Raw | ConvertFrom-Json
if ((Get-SHA256 $requestPath) -cne $requestHash -or $receipt.schemaVersion -ne 1 -or $receipt.provider -cne $policy.provider -or $receipt.requestSha256 -cne $requestHash -or $receipt.requestId -cnotmatch '^[A-Za-z0-9._:-]{1,200}$') { throw 'Provider receipt is not associated with the exact request.' }
# A provider may not modify the unsigned reference bytes while signing.
Assert-InputManifest $unsigned $identity $env:GITHUB_REPOSITORY $env:GITHUB_RUN_ID $BuildAttempt
if ((Get-SHA256 (Join-Path $unsigned 'build-manifest.json')) -cne $manifestHash) { throw 'Provider changed the unsigned build manifest.' }
Assert-SignedAssociation $unsigned $signed
$output = New-BuildDirectory $script:ReleaseRepo 'signed'
$hashes = [ordered]@{}
$signers = [ordered]@{}
foreach ($name in @('ArcourtDownloader.exe','arcourt-download.exe')) {
    Copy-Item -LiteralPath (Join-Path $signed $name) -Destination $output
    $hashes[$name] = Get-SHA256 (Join-Path $output $name)
    $signature = Get-AuthenticodeSignature -LiteralPath (Join-Path $output $name)
    $signers[$name] = [ordered]@{subject=$signature.SignerCertificate.Subject;certificateSha256=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($signature.SignerCertificate.RawData)).ToLowerInvariant();timestampSubject=$signature.TimeStamperCertificate.Subject}
}
$evidence = [ordered]@{schemaVersion=1;request=$request;receipt=$receipt;signedSha256=$hashes;signers=$signers}
Write-Utf8 (Join-Path $output 'signing-evidence.json') (($evidence | ConvertTo-Json -Depth 15) + "`n")
"attempt=$env:GITHUB_RUN_ATTEMPT" >> $env:GITHUB_OUTPUT
"Approved timestamped signatures and exact unsigned input association verified for $Tag ($($identity.commit)). Provider request: $($receipt.requestId)." >> $env:GITHUB_STEP_SUMMARY
