#requires -Version 7.4
. (Join-Path $PSScriptRoot 'release-common.ps1')

function Get-GitHubJson([string]$Endpoint) {
    $json = & gh api $Endpoint
    if ($LASTEXITCODE -ne 0) { throw "GitHub read failed: $Endpoint" }
    return ($json | ConvertFrom-Json)
}
function Receive-GitHubFile([string]$Endpoint, [string]$Path) {
    # PowerShell 7.4+ preserves native stdout bytes, including binary ZIP/EXE data.
    & gh api $Endpoint -H 'Accept: application/octet-stream' > $Path
    if ($LASTEXITCODE -ne 0) { throw 'GitHub asset download failed.' }
}
function Assert-ReleaseGates($Results, [string[]]$Required) {
    foreach ($name in $Required) {
        if ($Results.$name.result -cne 'success') { throw "Required exact-commit gate did not succeed: $name" }
    }
}
function Assert-WorkflowArtifact($Metadata, [string]$ArtifactId, [string]$Digest, [string]$Name, [string]$RunId, [string]$Commit) {
    if ($Metadata.id.ToString() -cne $ArtifactId -or $Metadata.name -cne $Name -or $Metadata.expired -or $Metadata.digest -cne "sha256:$Digest" -or $Metadata.workflow_run.id.ToString() -cne $RunId -or $Metadata.workflow_run.head_sha -cne $Commit) { throw 'Artifact ID/digest/run/source association mismatch.' }
}
function Expand-WorkflowArtifact([string]$Archive, [string]$Digest, [string]$Destination, [string[]]$Files) {
    if ((Get-SHA256 $Archive) -cne $Digest) { throw 'Artifact archive digest mismatch.' }
    Assert-OrdinaryPath $Destination
    if (Test-Path -LiteralPath $Destination) { throw 'Artifact destination must be new.' }
    $zip = [IO.Compression.ZipFile]::OpenRead($Archive)
    try {
        $seen = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
        [long]$total = 0
        foreach ($entry in $zip.Entries) {
            $total += $entry.Length
            if ($entry.FullName -cnotin $Files -or -not $seen.Add($entry.FullName) -or $entry.Length -gt 536870912 -or $total -gt 1073741824 -or (($entry.ExternalAttributes -shr 16) -band 0xF000) -eq 0xA000) { throw 'Unexpected/unsafe workflow artifact entry.' }
        }
        if ($seen.Count -ne $Files.Count) { throw 'Incomplete workflow artifact.' }
    } finally { $zip.Dispose() }
    [IO.Compression.ZipFile]::ExtractToDirectory($Archive,$Destination)
    Assert-FileSet $Destination $Files
}
function Assert-ReleaseSource([string]$Tag, [string]$Commit, $Main, [string]$Repository = $script:ReleaseRepo) {
    if ($Commit -cnotmatch '^[0-9a-f]{40}$') { throw 'Invalid event commit.' }
    $identity = & (Join-Path $PSScriptRoot 'resolve-build-info.ps1') -Release -Tag $Tag -Repository $Repository
    if ($identity.commit -cne $Commit) { throw 'Checkout/tag differs from immutable event commit.' }
    if ($Main.name -cne 'main' -or $Main.protected -ne $true) { throw 'Release requires protected main.' }
    $localMain = & git -C $Repository rev-parse --verify refs/remotes/origin/main
    if ($LASTEXITCODE -ne 0 -or $localMain -cne $Main.commit.sha) { throw 'Missing/stale main history; rerun from a fresh checkout.' }
    & git -C $Repository merge-base --is-ancestor $Commit refs/remotes/origin/main
    if ($LASTEXITCODE -ne 0) { throw 'Tag commit is outside protected main.' }
    return $identity
}
function Get-InputManifest([string]$Directory, $Identity, [string]$Repository, [string]$RunId, [string]$Attempt) {
    if ($Repository -notmatch '^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$' -or $RunId -notmatch '^[1-9][0-9]*$' -or $Attempt -notmatch '^[1-9][0-9]*$') { throw 'Invalid workflow identity.' }
    $files = [ordered]@{}
    foreach ($name in @('ArcourtDownloader.exe','arcourt-download.exe')) { $files[$name] = Get-SHA256 (Join-Path $Directory $name) }
    return [ordered]@{schemaVersion=1;repository=$Repository;tag=$Identity.tag;commit=$Identity.commit;runId=$RunId;runAttempt=$Attempt;files=$files}
}
function Assert-InputManifest([string]$Directory, $Identity, [string]$Repository, [string]$RunId, [string]$Attempt) {
    Assert-FileSet $Directory @('ArcourtDownloader.exe','arcourt-download.exe','build-manifest.json')
    $expected = Get-InputManifest $Directory $Identity $Repository $RunId $Attempt | ConvertTo-Json -Depth 10 -Compress
    $actual = Get-Content -LiteralPath (Join-Path $Directory 'build-manifest.json') -Raw | ConvertFrom-Json -AsHashtable | ConvertTo-Json -Depth 10 -Compress
    if ($actual -cne $expected) { throw 'Unsigned input/build association mismatch.' }
}
function Assert-CandidateBuildBinaries([string]$ZipPath, [string]$ManifestPath) {
    $manifest = Get-Content -LiteralPath $ManifestPath -Raw | ConvertFrom-Json
    $zip = [IO.Compression.ZipFile]::OpenRead($ZipPath)
    try {
        foreach ($name in @('ArcourtDownloader.exe','arcourt-download.exe')) {
            $entry = $zip.GetEntry($name)
            if ($null -eq $entry) { throw "Final ZIP is missing build executable: $name" }
            $stream = $entry.Open()
            try { $hash = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($stream)).ToLowerInvariant() } finally { $stream.Dispose() }
            if ($hash -cne $manifest.files.$name) { throw "Final ZIP executable differs from pinned build artifact: $name" }
        }
    } finally { $zip.Dispose() }
}
function Get-CandidateChecksums([string]$Assets, [string]$Tag) {
    $names = @("ArcourtDownloader-$Tag-windows-amd64.zip", "ArcourtDownloader-$Tag-SBOM.spdx.json")
    Assert-FileSet $Assets ($names + 'SHA256SUMS.txt')
    Assert-Checksums $Assets $names 'SHA256SUMS.txt'
    $hashes = [ordered]@{}
    foreach ($name in ($names + 'SHA256SUMS.txt' | Sort-Object -CaseSensitive)) { $hashes[$name] = Get-SHA256 (Join-Path $Assets $name) }
    return $hashes
}
function Get-CandidateMarker([string]$Tag, [string]$Commit, $Hashes) {
    $record = [ordered]@{schemaVersion=1;tag=$Tag;commit=$Commit;assets=$Hashes} | ConvertTo-Json -Depth 10 -Compress
    return '<!-- arcourt-candidate-v1:' + [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($record)) + ' -->'
}
function Assert-DraftCandidate($Release, [string]$Tag, [string]$Commit, [string]$Marker, [bool]$Prerelease, $Hashes) {
    if (-not $Release.draft -or $Release.immutable -eq $true) { throw 'Published/immutable releases cannot be modified.' }
    if ($Release.tag_name -cne $Tag -or $Release.target_commitish -cne $Commit -or $Release.prerelease -ne $Prerelease) { throw 'Draft tag/commit/prerelease mismatch.' }
    $markers = [regex]::Matches([string]$Release.body, '<!-- arcourt-candidate-v1:[^\r\n]*? -->')
    if ($markers.Count -ne 1 -or $markers[0].Value -cne $Marker) { throw 'Draft checksum identity differs from this candidate.' }
    $seen = @{}
    foreach ($asset in $Release.assets) {
        if (-not $Hashes.Contains($asset.name) -or $seen.ContainsKey($asset.name) -or $asset.state -cne 'uploaded') { throw 'Unexpected/duplicate/incomplete draft asset.' }
        $seen[$asset.name] = $true
        if ($asset.digest -and $asset.digest -cne "sha256:$($Hashes[$asset.name])") { throw 'Draft asset checksum mismatch.' }
    }
}
