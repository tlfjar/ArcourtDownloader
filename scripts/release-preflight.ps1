#requires -Version 7.4
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'workflow-common.ps1')
if ($env:GITHUB_EVENT_NAME -cne 'push' -or $env:GITHUB_REF_TYPE -cne 'tag' -or $env:GITHUB_REF -cne "refs/tags/$env:GITHUB_REF_NAME") { throw 'Only a release tag push is authorized.' }
$main = Get-GitHubJson "repos/$env:GITHUB_REPOSITORY/branches/main"
$identity = Assert-ReleaseSource $env:GITHUB_REF_NAME $env:GITHUB_SHA $main
Write-Host "Validated $($identity.tag) at $($identity.commit) on protected main. Exact-commit verification must still succeed."
