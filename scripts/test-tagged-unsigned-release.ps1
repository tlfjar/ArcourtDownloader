#requires -Version 7.4
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'workflow-common.ps1')
$repo = $script:ReleaseRepo
$scratch = New-BuildDirectory $repo ('tests/tagged-unsigned-' + [Guid]::NewGuid().ToString('N'))
$tag = 'v9.8.7-beta.1'

# Commit only a throwaway copy of the current tracked source. The real checkout
# receives no tag, commit, release, or remote write.
$files = & git -C $repo ls-files
if ($LASTEXITCODE -ne 0) { throw 'Unable to enumerate source files.' }
foreach ($relative in $files) {
    $source = Join-Path $repo $relative
    if (-not (Test-Path -LiteralPath $source -PathType Leaf)) { continue }
    $target = Join-Path $scratch $relative
    New-Item -ItemType Directory -Path (Split-Path -Parent $target) -Force | Out-Null
    Copy-Item -LiteralPath $source -Destination $target
}

# Replace only the GitHub artifact download boundary in the isolated checkout.
# Workflow rehearsals separately test artifact ID, digest, run, and source checks.
$stub = @'
#requires -Version 7.4
param([string]$ArtifactId, [string]$Digest, [string]$Name, [string]$Destination, [string[]]$Files)
if ($ArtifactId -cne '42' -or $Digest -cne ('a' * 64) -or $Name -cne "unsigned-$env:GITHUB_RUN_ID-$env:GITHUB_RUN_ATTEMPT" -or @($Files).Count -ne 3) { throw 'Wrong isolated artifact association.' }
if (Test-Path -LiteralPath $Destination) { throw 'Artifact destination reused.' }
New-Item -ItemType Directory -Path $Destination | Out-Null
foreach ($file in $Files) { Copy-Item -LiteralPath (Join-Path $PSScriptRoot "../build/release-inputs/$file") -Destination $Destination }
'@
Write-Utf8 (Join-Path $scratch 'scripts/get-workflow-artifact.ps1') $stub
Invoke-Native git @('-C',$scratch,'init','--quiet','--initial-branch=main')
Invoke-Native git @('-C',$scratch,'add','.')
Invoke-Native git @('-C',$scratch,'-c','user.name=Tagged Test','-c','user.email=tagged@example.invalid','-c','commit.gpgsign=false','commit','--quiet','-m','isolated unsigned release rehearsal')
Invoke-Native git @('-C',$scratch,'-c','tag.gpgsign=false','tag',$tag)

$saved = @{}
foreach ($key in @('GITHUB_REPOSITORY','GITHUB_RUN_ID','GITHUB_RUN_ATTEMPT','GITHUB_OUTPUT')) { $saved[$key] = [Environment]::GetEnvironmentVariable($key) }
try {
    $env:GITHUB_REPOSITORY = 'test/ArcourtDownloader'
    $env:GITHUB_RUN_ID = '12345'
    $env:GITHUB_RUN_ATTEMPT = '1'
    $env:GITHUB_OUTPUT = Join-Path $scratch 'build/outputs.txt'
    Push-Location $scratch
    try {
        & ./scripts/stage-release-inputs.ps1 -Tag $tag | Out-Null
        & ./scripts/package-release-inputs.ps1 -Tag $tag -BuildAttempt '1' -ArtifactId '42' -ArtifactDigest ('a' * 64) | Out-Null
        $assets = Join-Path $scratch "build/releases/$tag"
        & ./scripts/verify-release-assets.ps1 -AssetsPath $assets -Tag $tag -UnsignedRelease | Out-Null
        $identity = & ./scripts/resolve-build-info.ps1 -Release -Tag $tag
        Assert-InputManifest (Join-Path $scratch 'build/release-inputs') $identity $env:GITHUB_REPOSITORY $env:GITHUB_RUN_ID $env:GITHUB_RUN_ATTEMPT
        Assert-CandidateBuildBinaries (Join-Path $assets "ArcourtDownloader-$tag-windows-amd64.zip") (Join-Path $scratch 'build/release-inputs/build-manifest.json')
        Get-CandidateChecksums $assets $tag | Out-Null
    } finally { Pop-Location }
} finally {
    foreach ($key in $saved.Keys) { [Environment]::SetEnvironmentVariable($key, $saved[$key]) }
}
Write-Host "PASS isolated tagged unsigned release and exact build-to-ZIP handoff: $scratch"
