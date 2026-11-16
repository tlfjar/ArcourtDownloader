#requires -Version 7.4
param([Parameter(Mandatory)][string]$Tag)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'workflow-common.ps1')
if ($env:GITHUB_EVENT_NAME -cne 'workflow_dispatch' -or $env:GITHUB_REF -cne 'refs/heads/main') { throw 'Prepare release must be run manually from main.' }
$release = Get-TagRelease $Tag
if ($null -ne $release) { throw "Release '$Tag' already exists. Choose a NEW version; never publish a release before its assets are staged." }
$encoded = [Uri]::EscapeDataString($Tag)
$references = @(Get-GitHubJson "repos/$env:GITHUB_REPOSITORY/git/matching-refs/tags/$encoded")
if (@($references | Where-Object { $_.ref -ceq "refs/tags/$Tag" }).Count) { throw "Tag '$Tag' already exists. Choose a NEW version, or resume its draft build with: gh workflow run release.yml --repo $env:GITHUB_REPOSITORY --ref $Tag" }
$main = Get-GitHubJson "repos/$env:GITHUB_REPOSITORY/branches/main"
# Validate the exact release identity locally before creating anything remotely.
Invoke-Native git @('-C',$script:ReleaseRepo,'-c','tag.gpgsign=false','tag','--',$Tag,$env:GITHUB_SHA)
$identity = Assert-ReleaseSource $Tag $env:GITHUB_SHA $main
Invoke-Native gh @('api',"repos/$env:GITHUB_REPOSITORY/git/refs",'--method','POST','-f',"ref=refs/tags/$Tag",'-f',"sha=$($identity.commit)")
# Tags created with GITHUB_TOKEN do not trigger push workflows. Dispatch at the
# tag so the build and its attestations still identify refs/tags/<version>.
Write-Host "Created $Tag. If dispatch fails, resume with: gh workflow run release.yml --repo $env:GITHUB_REPOSITORY --ref $Tag"
Invoke-Native gh @('workflow','run','release.yml','--repo',$env:GITHUB_REPOSITORY,'--ref',$Tag)
"Draft build requested for $Tag at $($identity.commit). Follow **Unsigned draft release** in Actions; wait for its three assets before publishing." >> $env:GITHUB_STEP_SUMMARY
