#requires -Version 7.4
param([switch]$Check)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'release-common.ps1')
$temp = Join-Path ([IO.Path]::GetTempPath()) ('arcourt-notices-' + [Guid]::NewGuid().ToString('N') + '.txt')
Push-Location $script:ReleaseRepo
try {
    Push-Location desktop/frontend
    try { Invoke-Native node @('build.mjs') } finally { Pop-Location }
    Invoke-Native go @('run','-mod=readonly','./scripts/release-tool','-mode','notices','-out',$temp)
    if ($Check) { if ((Get-SHA256 $temp) -cne (Get-SHA256 'THIRD_PARTY_NOTICES.txt')) { throw 'Notices are stale.' } }
    else { Copy-Item -LiteralPath $temp -Destination 'THIRD_PARTY_NOTICES.txt' }
} finally { Pop-Location; if (Test-Path -LiteralPath $temp) { Remove-Item -LiteralPath $temp } }
