#requires -Version 7.4
param([Parameter(Mandatory)][string]$Tag, [Parameter(Mandatory)][string]$AssetsPath)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'workflow-common.ps1')
$identity = & (Join-Path $PSScriptRoot 'resolve-build-info.ps1') -Release -Tag $Tag
if ($identity.commit -cne $env:GITHUB_SHA) { throw 'Draft source differs from event commit.' }
$hashes = Get-CandidateChecksums $AssetsPath $Tag
$marker = Get-CandidateMarker $Tag $identity.commit $hashes
$prerelease = $identity.version.Split('+')[0].Contains('-')

function Assert-RemoteTag {
    $encoded = [Uri]::EscapeDataString($Tag)
    $reference = (Get-GitHubJson "repos/$env:GITHUB_REPOSITORY/git/ref/tags/$encoded").object
    for ($depth=0; $reference.type -ceq 'tag' -and $depth -lt 8; $depth++) {
        $reference = (Get-GitHubJson "repos/$env:GITHUB_REPOSITORY/git/tags/$($reference.sha)").object
    }
    if ($reference.type -cne 'commit' -or $reference.sha -cne $identity.commit) { throw 'Remote tag changed or does not peel to event commit.' }
}
function Read-Release {
    # The authenticated list includes drafts. An API error is never treated as absence.
    $json = & gh api "repos/$env:GITHUB_REPOSITORY/releases?per_page=100" --paginate --slurp
    if ($LASTEXITCODE -ne 0) { throw 'Unable to inspect existing releases.' }
    $matches = @($json | ConvertFrom-Json | ForEach-Object { foreach ($release in $_) { if ($release.tag_name -ceq $Tag) { $release } } })
    if ($matches.Count -gt 1) { throw 'Ambiguous existing release.' }
    if ($matches.Count -eq 1) { return $matches[0] }
    return $null
}
function Assert-ExistingAssets($Release) {
    Assert-DraftCandidate $Release $Tag $identity.commit $marker $prerelease $hashes
    foreach ($asset in $Release.assets) {
        # Verify actual remote bytes as well as any digest supplied by the API.
        $file = Join-Path $env:RUNNER_TEMP ('arcourt-remote-' + [Guid]::NewGuid().ToString('N'))
        try {
            Receive-GitHubFile "repos/$env:GITHUB_REPOSITORY/releases/assets/$($asset.id)" $file
            if ((Get-SHA256 $file) -cne $hashes[$asset.name]) { throw 'Existing draft asset bytes differ from this candidate.' }
        } finally { if (Test-Path -LiteralPath $file) { Remove-Item -LiteralPath $file } }
    }
}

Assert-RemoteTag
$release = Read-Release
if ($null -ne $release) { Assert-ExistingAssets $release }
# Verify hosted provenance before creating even an empty draft.
foreach ($name in @("ArcourtDownloader-$Tag-windows-amd64.zip","ArcourtDownloader-$Tag-SBOM.spdx.json")) {
    Invoke-Native gh @('attestation','verify',(Join-Path $AssetsPath $name),'--repo',$env:GITHUB_REPOSITORY,'--signer-workflow',"$env:GITHUB_REPOSITORY/.github/workflows/release.yml",'--source-digest',$identity.commit,'--source-ref',"refs/tags/$Tag")
}
Invoke-Native gh @('attestation','verify',(Join-Path $AssetsPath "ArcourtDownloader-$Tag-windows-amd64.zip"),'--repo',$env:GITHUB_REPOSITORY,'--signer-workflow',"$env:GITHUB_REPOSITORY/.github/workflows/release.yml",'--source-digest',$identity.commit,'--source-ref',"refs/tags/$Tag",'--predicate-type','https://spdx.dev/Document')
if ($null -eq $release) {
    $notesPath = Join-Path $env:RUNNER_TEMP 'arcourt-draft-notes.md'
    $notes = @(
        "Candidate $Tag; source $($identity.commit).",
        "", "Build, signing evidence and provenance: https://github.com/$env:GITHUB_REPOSITORY/actions/runs/$env:GITHUB_RUN_ID",
        "", 'Production GUI walkthrough, live compatibility, fresh-download inspection and operator publication approval: pending. Automated fixtures do not establish live compatibility.',
        "", 'Inspect the three assets, signer, source and attestations using docs/releasing.md before publication.',
        "", $marker, ""
    ) -join "`n"
    Write-Utf8 $notesPath $notes
    Assert-RemoteTag
    $arguments = @('release','create',$Tag,'--repo',$env:GITHUB_REPOSITORY,'--draft','--verify-tag','--target',$identity.commit,'--title',"Arcourt Downloader $Tag",'--notes-file',$notesPath)
    if ($prerelease) { $arguments += '--prerelease' }
    Invoke-Native gh $arguments
    $release = Read-Release
    if ($null -eq $release) { throw 'Created draft is not visible.' }
    Assert-ExistingAssets $release
}
$releaseId = $release.id
foreach ($name in $hashes.Keys) {
    Assert-RemoteTag
    $release = Read-Release
    if ($null -eq $release -or $release.id -ne $releaseId) { throw 'Draft was removed/replaced during staging.' }
    Assert-ExistingAssets $release
    if ($name -cnotin @($release.assets.name)) {
        # Never --clobber: recover partial uploads only when every existing byte matches.
        Invoke-Native gh @('release','upload',$Tag,(Join-Path $AssetsPath $name),'--repo',$env:GITHUB_REPOSITORY)
    }
}
$release = Read-Release
if ($null -eq $release -or $release.id -ne $releaseId -or @($release.assets).Count -ne 3) { throw 'Draft readback is incomplete.' }
Assert-RemoteTag
Assert-ExistingAssets $release
"Draft staged: $($release.html_url). Publication and live verification remain operator gates." >> $env:GITHUB_STEP_SUMMARY
