#requires -Version 7.4
param([Parameter(Mandatory)][string]$AssetsPath)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'release-common.ps1')
$sourceRepo = $script:ReleaseRepo
$verify = Join-Path $PSScriptRoot 'verify-release-assets.ps1'
$assets = [IO.Path]::GetFullPath($AssetsPath)
$testRoot = New-BuildDirectory $sourceRepo ('tests/release-' + [Guid]::NewGuid().ToString('N'))
$zipName = 'ArcourtDownloader-development-windows-amd64.zip'
$sbomName = 'ArcourtDownloader-development-SBOM.spdx.json'
function Reject([string]$Name, [scriptblock]$Action, [string]$Expected) {
    try { & $Action | Out-Null } catch {
        if ($_.ToString() -notmatch $Expected) { throw "Wrong failure for ${Name}: $_" }
        Write-Host "PASS rejection: $Name"
        return
    }
    throw "Accepted invalid input: $Name"
}
function Mutation([string]$Name, [scriptblock]$Edit, [string]$Expected, [switch]$KeepHash) {
    $dir = Join-Path $testRoot $Name
    New-Item -ItemType Directory -Path $dir | Out-Null
    foreach ($file in @($zipName,$sbomName,'SHA256SUMS.txt')) { Copy-Item -LiteralPath (Join-Path $assets $file) -Destination $dir }
    $zip = [IO.Compression.ZipFile]::Open((Join-Path $dir $zipName), [IO.Compression.ZipArchiveMode]::Update)
    try { & $Edit $zip } finally { $zip.Dispose() }
    if (-not $KeepHash) { Write-Checksums $dir @($zipName,$sbomName) 'SHA256SUMS.txt' }
    Reject $Name { & $verify -AssetsPath $dir -Tag development -Local } $Expected
}
& $verify -AssetsPath $assets -Tag development -Local
Reject 'missing verification mode' { & $verify -AssetsPath $assets -Tag development } 'Choose exactly one verification mode'
Reject 'conflicting verification modes' { & $verify -AssetsPath $assets -Tag development -Local -UnsignedRelease } 'Choose exactly one verification mode'
Reject 'release mode on development assets' { & $verify -AssetsPath $assets -Tag development -UnsignedRelease } 'requires a tagged release'

function Get-AuthenticodeSignature { [pscustomobject]@{Status='Valid';SignerCertificate=$null;TimeStamperCertificate=$null} }
try {
    Reject 'unexpected signature at asset verifier' { & $verify -AssetsPath $assets -Tag development -Local } 'Expected Authenticode status NotSigned'
    foreach ($status in @('Valid','HashMismatch','UnknownError','NotTrusted','NotSupportedFileFormat')) {
        function Get-AuthenticodeSignature { [pscustomobject]@{Status=$status;SignerCertificate=$null;TimeStamperCertificate=$null} }
        Reject "unexpected signature state $status" { Assert-UnsignedExecutable (Join-Path $assets 'ArcourtDownloader.exe') } 'Expected Authenticode status NotSigned'
    }
    function Get-AuthenticodeSignature { [pscustomobject]@{Status='NotSigned';SignerCertificate=[pscustomobject]@{Subject='unexpected'};TimeStamperCertificate=$null} }
    Reject 'ambiguous unsigned state with certificate' { Assert-UnsignedExecutable (Join-Path $assets 'arcourt-download.exe') } 'Expected Authenticode status NotSigned'
} finally { Remove-Item Function:Get-AuthenticodeSignature }
Mutation 'altered-hash' { param($z) $z.GetEntry('README.md').Delete() } 'Checksum mismatch' -KeepHash
Mutation 'missing-notice' { param($z) $z.GetEntry('THIRD_PARTY_NOTICES.txt').Delete() } 'Missing archive payload'
Mutation 'missing-sbom' { param($z) $z.GetEntry('SBOM.spdx.json').Delete() } 'Missing archive payload'
Mutation 'fixture-inclusion' { param($z) $z.CreateEntry('ArcourtDownloader-fixture.exe') | Out-Null } 'Unsafe/unexpected'
Mutation 'traversal' { param($z) $z.CreateEntry('../outside.txt') | Out-Null } 'Unsafe/unexpected'
Mutation 'duplicate-name' { param($z) $z.CreateEntry('LICENSE') | Out-Null } 'Unsafe/unexpected'
Mutation 'unsafe-profile' { param($z) $z.CreateEntry('settings.json') | Out-Null } 'Unsafe/unexpected'

# Rehash inner and outer containers so semantic checks must detect these changes.
$unpacked = Join-Path $testRoot 'payload'
[IO.Compression.ZipFile]::ExtractToDirectory((Join-Path $assets $zipName),$unpacked)
foreach ($name in @('ArcourtDownloader.exe','arcourt-download.exe')) { Assert-UnsignedExecutable (Join-Path $unpacked $name) }
$badPe = Join-Path $testRoot 'certificate-table.exe'
$bytes = [IO.File]::ReadAllBytes((Join-Path $unpacked 'arcourt-download.exe'))
$pe = [BitConverter]::ToInt32($bytes, 0x3C)
$optional = $pe + 24
$directory = if ([BitConverter]::ToUInt16($bytes, $optional) -eq 0x20B) { 112 } else { 96 }
[BitConverter]::GetBytes([uint32]8).CopyTo($bytes, ($optional + $directory + 32))
[BitConverter]::GetBytes([uint32]8).CopyTo($bytes, ($optional + $directory + 36))
[IO.File]::WriteAllBytes($badPe, $bytes)
Reject 'malformed PE certificate directory' { Assert-NoPeCertificateTable $badPe } 'PE certificate table must be absent'
$identity = & (Join-Path $PSScriptRoot 'resolve-build-info.ps1')
$buildPath = Join-Path $unpacked 'BUILD.json'
$original = [IO.File]::ReadAllText($buildPath)
$wrong = $original | ConvertFrom-Json; $wrong.version = '9.9.9'
Write-Utf8 $buildPath ($wrong | ConvertTo-Json)
Write-Checksums $unpacked @($script:PayloadNames | Where-Object { $_ -ne 'FILE_SHA256SUMS.txt' }) 'FILE_SHA256SUMS.txt'
Reject 'wrong-version-rehashed' { Assert-Payload $unpacked $identity } 'identity mismatch'
Write-Utf8 $buildPath $original
$sbomPath = Join-Path $unpacked 'SBOM.spdx.json'
$sbomOriginal = [IO.File]::ReadAllText($sbomPath)
$sbom = $sbomOriginal | ConvertFrom-Json; $sbom.packages[0].versionInfo = '9.9.9'
Write-Utf8 $sbomPath ($sbom | ConvertTo-Json -Depth 30)
Write-Checksums $unpacked @($script:PayloadNames | Where-Object { $_ -ne 'FILE_SHA256SUMS.txt' }) 'FILE_SHA256SUMS.txt'
Reject 'sbom-rehashed' { Assert-Payload $unpacked $identity } 'SBOM does not describe'
Write-Utf8 $sbomPath $sbomOriginal
$badIdentity = $identity | ConvertTo-Json | ConvertFrom-Json; $badIdentity.commit = '0000000000000000000000000000000000000000'
Push-Location $sourceRepo
try {
    $result = & go run ./scripts/release-tool -mode inspect -binaries $unpacked -version development -commit $badIdentity.commit 2>&1
    if ($LASTEXITCODE -eq 0 -or ($result -join "`n") -notmatch 'Wrong version/commit') { throw 'Binary identity mismatch was not rejected.' }
    Write-Host 'PASS rejection: actual binary wrong commit'
} finally { Pop-Location }

# Test the official invocation using an isolated clean tagged repository.
# No tags, policy edits or commits are made in the working repository.
$scratch = Join-Path $testRoot 'source'
New-Item -ItemType Directory -Path (Join-Path $scratch 'scripts') -Force | Out-Null
foreach ($name in @('package-windows.ps1','common.ps1','release-common.ps1','resolve-build-info.ps1')) { Copy-Item -LiteralPath (Join-Path $PSScriptRoot $name) -Destination (Join-Path $scratch 'scripts') }
Write-Utf8 (Join-Path $scratch '.gitignore') "/build/`n"
Invoke-Native git @('-C',$scratch,'init','--quiet')
Invoke-Native git @('-C',$scratch,'add','.')
Invoke-Native git @('-C',$scratch,'-c','user.name=Packaging Test','-c','user.email=packaging@example.invalid','-c','commit.gpgsign=false','commit','--quiet','-m','isolated rejection fixture')
Invoke-Native git @('-C',$scratch,'tag','v1.2.3')
$pack = Join-Path $scratch 'scripts/package-windows.ps1'
$inputs = Join-Path $scratch 'build/inputs'
New-Item -ItemType Directory -Path $inputs -Force | Out-Null
foreach ($name in @('ArcourtDownloader.exe','arcourt-download.exe')) { Copy-Item -LiteralPath (Join-Path $unpacked $name) -Destination $inputs }
$output = Join-Path $scratch 'build/releases/v1.2.3'
Reject 'output-outside-release-root' { & $pack -UnsignedRelease -Tag v1.2.3 -BinariesPath $inputs -OutputPath (Join-Path $scratch 'build/wrong') } 'OutputPath must be build/releases'
New-Item -ItemType Directory -Path $output -Force | Out-Null
Write-Utf8 (Join-Path $output '.hidden') 'reused'
Reject 'reused-output' { & $pack -UnsignedRelease -Tag v1.2.3 -BinariesPath $inputs -OutputPath $output } 'empty directory'
Remove-Item -LiteralPath (Join-Path $output '.hidden')
Remove-Item -LiteralPath $output
Write-Utf8 $output 'not a directory'
Reject 'output-is-file' { & $pack -UnsignedRelease -Tag v1.2.3 -BinariesPath $inputs -OutputPath $output } 'empty directory'
Remove-Item -LiteralPath $output
New-Item -ItemType Directory -Path $output | Out-Null
Write-Utf8 (Join-Path $scratch 'dirty.txt') 'dirty'
Reject 'dirty-release-source' { & $pack -UnsignedRelease -Tag v1.2.3 -BinariesPath $inputs -OutputPath $output } 'source has tracked or relevant untracked changes'
Remove-Item -LiteralPath (Join-Path $scratch 'dirty.txt')
Reject 'wrong-tag' { & $pack -UnsignedRelease -Tag v9.9.9 -BinariesPath $inputs -OutputPath $output } 'Git identity check failed'
function Get-AuthenticodeSignature { [pscustomobject]@{Status='Valid';SignerCertificate=[pscustomobject]@{Subject='unexpected'};TimeStamperCertificate=$null} }
try { Reject 'unexpected signature at tagged package gate' { & $pack -UnsignedRelease -Tag v1.2.3 -BinariesPath $inputs -OutputPath $output } 'Expected Authenticode status NotSigned' }
finally { Remove-Item Function:Get-AuthenticodeSignature }

$evidenceRoot = Join-Path $testRoot 'bad-evidence'
New-Item -ItemType Directory -Path (Join-Path $evidenceRoot 'scripts/licenses') -Force | Out-Null
$catalogPath = Join-Path $evidenceRoot 'scripts/licenses/catalog.json'
Copy-Item -LiteralPath (Join-Path $sourceRepo 'scripts/licenses/catalog.json') -Destination $catalogPath
Push-Location $sourceRepo
try {
    $result = & go run ./scripts/release-tool -repo $evidenceRoot -binaries $inputs -out (Join-Path $testRoot 'bad-sbom.json') 2>&1
    if ($LASTEXITCODE -eq 0 -or ($result -join "`n") -notmatch 'THIRD_PARTY_NOTICES.txt') { throw 'Missing source notices were not rejected.' }
    Write-Host 'PASS rejection: missing source notices'
    $catalog = Get-Content -LiteralPath $catalogPath -Raw | ConvertFrom-Json
    $catalog.Components[0].License = 'UNKNOWN'
    Write-Utf8 $catalogPath ($catalog | ConvertTo-Json -Depth 30)
    $result = & go run ./scripts/release-tool -mode notices -repo $evidenceRoot -out (Join-Path $testRoot 'bad-notices.txt') 2>&1
    if ($LASTEXITCODE -eq 0 -or ($result -join "`n") -notmatch 'Unreviewed license') { throw 'Unknown license was not rejected.' }
    Write-Host 'PASS rejection: unknown license'
} finally { Pop-Location }
Write-Host "Unsigned release rejection checks passed. Evidence retained at $testRoot."
# Expected native failures above must not become the caller's process exit status.
exit 0
