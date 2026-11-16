#requires -Version 7.4
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'workflow-common.ps1')
Assert-ReleaseEvent $env:GITHUB_EVENT_NAME $env:GITHUB_REF_TYPE $env:GITHUB_REF $env:GITHUB_REF_NAME
Assert-UnpublishedRelease (Get-TagRelease $env:GITHUB_REF_NAME) $env:GITHUB_REF_NAME
$main = Get-GitHubJson "repos/$env:GITHUB_REPOSITORY/branches/main"
$identity = Assert-ReleaseSource $env:GITHUB_REF_NAME $env:GITHUB_SHA $main
Write-Host "Validated $($identity.tag) at $($identity.commit) on protected main. Exact-commit verification must still succeed."
