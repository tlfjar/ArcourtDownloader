#requires -Version 7.4
param(
    [Parameter(Mandatory)][ValidatePattern('^[1-9][0-9]*$')][string]$ArtifactId,
    [Parameter(Mandatory)][ValidatePattern('^[0-9a-f]{64}$')][string]$Digest,
    [Parameter(Mandatory)][string]$Name,
    [Parameter(Mandatory)][string]$Destination,
    [Parameter(Mandatory)][string[]]$Files
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'workflow-common.ps1')
$metadata = Get-GitHubJson "repos/$env:GITHUB_REPOSITORY/actions/artifacts/$ArtifactId"
Assert-WorkflowArtifact $metadata $ArtifactId $Digest $Name $env:GITHUB_RUN_ID $env:GITHUB_SHA
$destinationPath = [IO.Path]::GetFullPath($Destination)
Assert-OrdinaryPath $destinationPath
if (Test-Path -LiteralPath $destinationPath) { throw 'Artifact destination must be new.' }
$archive = Join-Path ([IO.Path]::GetTempPath()) ('arcourt-artifact-' + [Guid]::NewGuid().ToString('N') + '.zip')
try {
    Receive-GitHubActionsArtifactArchive "repos/$env:GITHUB_REPOSITORY/actions/artifacts/$ArtifactId/zip" $archive
    Expand-WorkflowArtifact $archive $Digest $destinationPath $Files
} finally { if (Test-Path -LiteralPath $archive) { Remove-Item -LiteralPath $archive } }
