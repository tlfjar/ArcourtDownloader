#requires -Version 7.4
param([Parameter(Mandatory)][ValidateSet('root','desktop')][string]$Module)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')
$repo = Split-Path -Parent $PSScriptRoot
$saved = @{}
foreach ($key in @('GOOS','GOARCH','CGO_ENABLED','GOWORK','GOFLAGS')) { $saved[$key] = [Environment]::GetEnvironmentVariable($key) }
try {
    $env:GOOS='windows'; $env:GOARCH='amd64'; $env:CGO_ENABLED='0'; $env:GOWORK='off'; $env:GOFLAGS='-mod=readonly'
    Push-Location $repo
    try {
        if ($Module -eq 'desktop') {
            Push-Location desktop/frontend
            try { Invoke-Native node @('build.mjs') } finally { Pop-Location }
            Push-Location desktop
        }
        try {
            $arguments = @('run','golang.org/x/vuln/cmd/govulncheck@v1.1.4')
            if ($Module -eq 'desktop') { $arguments += '-tags=desktop,wv2runtime.error,production' }
            Invoke-Native go ($arguments + './...')
        } finally { if ($Module -eq 'desktop') { Pop-Location } }
    } finally { Pop-Location }
} finally { foreach ($key in $saved.Keys) { [Environment]::SetEnvironmentVariable($key,$saved[$key]) } }
