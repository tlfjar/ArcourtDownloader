#requires -Version 7.4
param([Parameter(Mandatory)][string]$Tag)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'workflow-common.ps1')
$identity = & (Join-Path $PSScriptRoot 'resolve-build-info.ps1') -Release -Tag $Tag
$directory = Join-Path $script:ReleaseRepo 'build/release-inputs'
if (Test-Path -LiteralPath $directory) { throw 'Unsigned staging directory must be new.' }
& (Join-Path $PSScriptRoot 'build-desktop.ps1') -Release -Tag $Tag
New-Item -ItemType Directory -Path $directory | Out-Null
Copy-Item -LiteralPath (Join-Path $script:ReleaseRepo 'desktop/build/bin/ArcourtDownloader.exe') -Destination $directory
& (Join-Path $PSScriptRoot 'build-cli.ps1') -Release -Tag $Tag -OutputPath (Join-Path $directory 'arcourt-download.exe') | Out-Null
& (Join-Path $PSScriptRoot 'generate-notices.ps1') -Check
Push-Location $script:ReleaseRepo
try { Invoke-Native go @('run','-mod=readonly','./scripts/release-tool','-mode','inspect','-binaries',$directory,'-version',$identity.version,'-commit',$identity.commit,'-release','true') } finally { Pop-Location }
$manifest = Get-InputManifest $directory $identity $env:GITHUB_REPOSITORY $env:GITHUB_RUN_ID $env:GITHUB_RUN_ATTEMPT
Write-Utf8 (Join-Path $directory 'build-manifest.json') (($manifest | ConvertTo-Json -Depth 10) + "`n")
& (Join-Path $PSScriptRoot 'resolve-build-info.ps1') -Release -Tag $Tag | Out-Null
"attempt=$env:GITHUB_RUN_ATTEMPT" >> $env:GITHUB_OUTPUT
