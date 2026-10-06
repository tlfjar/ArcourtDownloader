# Packaging and verification require PowerShell 7.4+.
. (Join-Path $PSScriptRoot 'common.ps1')
$script:ReleaseRepo = Split-Path -Parent $PSScriptRoot
$script:PayloadNames = @('ArcourtDownloader.exe', 'arcourt-download.exe', 'LICENSE', 'THIRD_PARTY_NOTICES.txt', 'README.md', 'SUPPORT.md', 'SECURITY.md', 'docs/command-line-workflow.md', 'docs/releasing.md', 'BUILD.json', 'SBOM.spdx.json', 'FILE_SHA256SUMS.txt')

function Get-ReviewedGoToolchain {
    $catalog = Get-Content -LiteralPath (Join-Path $script:ReleaseRepo 'scripts/licenses/catalog.json') -Raw | ConvertFrom-Json
    $go = @($catalog.Components | Where-Object { $_.Name -ceq 'Go standard library' })
    if ($go.Count -ne 1 -or $go[0].Version -cnotmatch '^go1\.[0-9]+\.[0-9]+$') { throw 'License catalog must specify one reviewed Go toolchain.' }
    return $go[0].Version
}

function Assert-OrdinaryPath([string]$Path) {
    $probe = [IO.Path]::GetFullPath($Path)
    while ($probe) {
        if (Test-Path -LiteralPath $probe) {
            if ((Get-Item -LiteralPath $probe -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Reparse points are forbidden in package paths.' }
        }
        $probe = Split-Path -Parent $probe
    }
}
function Write-Utf8([string]$Path, [string]$Text) {
    [IO.File]::WriteAllText($Path, $Text.Replace("`r`n", "`n"), [Text.UTF8Encoding]::new($false))
}
function Get-SHA256([string]$Path) { (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() }
function Assert-FileSet([string]$Directory, [string[]]$Expected) {
    Assert-OrdinaryPath $Directory
    $actual = @(Get-ChildItem -LiteralPath $Directory -Force -Recurse | ForEach-Object {
        Assert-OrdinaryPath $_.FullName
        if (-not $_.PSIsContainer) { $_.FullName.Substring($Directory.TrimEnd('\', '/').Length + 1).Replace('\', '/') }
        else {
            $prefix = $_.FullName.Substring($Directory.TrimEnd('\', '/').Length + 1).Replace('\', '/') + '/'
            if (-not @($Expected | Where-Object { $_.StartsWith($prefix, [StringComparison]::Ordinal) }).Count) { throw 'Unexpected payload directory.' }
        }
    })
    if (@(Compare-Object ($Expected | Sort-Object) ($actual | Sort-Object) -CaseSensitive).Count) { throw 'Missing or unexpected payload files.' }
}
function Write-Checksums([string]$Directory, [string[]]$Names, [string]$OutputName) {
    $lines = foreach ($name in ($Names | Sort-Object -CaseSensitive)) { '{0}  {1}' -f (Get-SHA256 (Join-Path $Directory $name)), $name }
    Write-Utf8 (Join-Path $Directory $OutputName) (($lines -join "`n") + "`n")
}
function Assert-Checksums([string]$Directory, [string[]]$Names, [string]$ChecksumName) {
    $expected = foreach ($name in ($Names | Sort-Object -CaseSensitive)) { '{0}  {1}' -f (Get-SHA256 (Join-Path $Directory $name)), $name }
    if ([IO.File]::ReadAllText((Join-Path $Directory $ChecksumName)) -cne (($expected -join "`n") + "`n")) { throw "Checksum mismatch: $ChecksumName" }
}
function Assert-ApprovedSignature([string]$Path) {
    $policy = Get-Content -LiteralPath (Join-Path $script:ReleaseRepo 'scripts/signing-policy.json') -Raw | ConvertFrom-Json
    if ($policy.schemaVersion -ne 1 -or -not $policy.provider -or -not $policy.subject -or @($policy.certificateSha256).Count -eq 0) { throw 'Approved signing provider configuration is missing. See docs/releasing.md.' }
    $sig = Get-AuthenticodeSignature -LiteralPath $Path
    if ($sig.Status -ne 'Valid' -or -not $sig.SignerCertificate -or -not $sig.TimeStamperCertificate) { throw 'Missing/invalid timestamped Authenticode signature.' }
    $fingerprint = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($sig.SignerCertificate.RawData)).ToLowerInvariant()
    if ($sig.SignerCertificate.Subject -cne $policy.subject -or $fingerprint -cnotin @($policy.certificateSha256)) { throw 'Authenticode signer does not match the approved provider identity.' }
}
function Invoke-Evidence([string]$Binaries, $Identity, [string]$Output) {
    if ((Get-SHA256 (Join-Path $script:ReleaseRepo 'scripts/licenses/spdx-schema-2.3.json')) -cne '239208b7ac287b3cf5d9a9af23f9d69863971102a5e1587a27a398b43490b89b') { throw 'Pinned SPDX schema changed.' }
    $previousToolchain = $env:GOTOOLCHAIN
    Push-Location $script:ReleaseRepo
    try {
        $env:GOTOOLCHAIN = Get-ReviewedGoToolchain
        Invoke-Native go @('run', '-mod=readonly', './scripts/release-tool', '-repo', $script:ReleaseRepo, '-binaries', $Binaries, '-version', $Identity.version, '-commit', $Identity.commit, '-release', $Identity.release.ToString().ToLowerInvariant(), '-out', $Output)
    } finally { Pop-Location; $env:GOTOOLCHAIN = $previousToolchain }
    if (-not (Test-Json -LiteralPath $Output -SchemaFile (Join-Path $script:ReleaseRepo 'scripts/licenses/spdx-schema-2.3.json'))) { throw 'SBOM does not match the pinned SPDX 2.3 schema.' }
}
function Assert-Payload([string]$Directory, $Identity, [switch]$Local) {
    Assert-FileSet $Directory $script:PayloadNames
    Assert-Checksums $Directory @($script:PayloadNames | Where-Object { $_ -ne 'FILE_SHA256SUMS.txt' }) 'FILE_SHA256SUMS.txt'
    $build = Get-Content -LiteralPath (Join-Path $Directory 'BUILD.json') -Raw | ConvertFrom-Json
    foreach ($field in @('version','commit','release','tag','windowsVersion')) { if ($build.$field -cne $Identity.$field) { throw "BUILD.json identity mismatch: $field" } }
    if (@($build.PSObject.Properties).Count -ne 5) { throw 'Unexpected build identity fields.' }
    $resource = (Get-Item -LiteralPath (Join-Path $Directory 'ArcourtDownloader.exe')).VersionInfo
    $suffix = if ($Identity.release) { '; release source' } else { '; development, unsigned' }
    if ($resource.FileVersion -cne $Identity.windowsVersion -or $resource.ProductVersion -cne $Identity.windowsVersion -or $resource.Comments -cne "$($Identity.version); source $($Identity.commit)$suffix") { throw 'GUI Windows resource identity mismatch.' }
    foreach ($name in @('ArcourtDownloader.exe','arcourt-download.exe')) { if (-not $Local) { Assert-ApprovedSignature (Join-Path $Directory $name) } }
    foreach ($name in @('LICENSE','THIRD_PARTY_NOTICES.txt','README.md','SUPPORT.md','SECURITY.md','docs/command-line-workflow.md','docs/releasing.md')) {
        if ([IO.File]::ReadAllText((Join-Path $Directory $name)) -cne [IO.File]::ReadAllText((Join-Path $script:ReleaseRepo $name)).Replace("`r`n","`n")) { throw "Payload differs from reviewed source: $name" }
    }
    $temporary = Join-Path ([IO.Path]::GetTempPath()) ('arcourt-sbom-' + [Guid]::NewGuid().ToString('N') + '.json')
    try {
        Invoke-Evidence $Directory $Identity $temporary
        if ((Get-SHA256 $temporary) -cne (Get-SHA256 (Join-Path $Directory 'SBOM.spdx.json'))) { throw 'SBOM does not describe the final binaries/current source.' }
    } finally { if (Test-Path -LiteralPath $temporary) { Remove-Item -LiteralPath $temporary } }
}
