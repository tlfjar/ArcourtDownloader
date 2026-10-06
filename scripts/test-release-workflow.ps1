#requires -Version 7.4
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'workflow-common.ps1')
$repo = $script:ReleaseRepo
$testRoot = New-BuildDirectory $repo ('tests/workflow-' + [Guid]::NewGuid().ToString('N'))
function Reject([string]$Name, [scriptblock]$Action, [string]$Expected) {
    try { & $Action | Out-Null } catch {
        if ($_.ToString() -notmatch $Expected) { throw "Wrong failure for ${Name}: $_" }
        Write-Host "PASS rejection: $Name"
        return
    }
    throw "Accepted invalid input: $Name"
}
function Save-JSON([string]$Path, $Value) { Write-Utf8 $Path (($Value | ConvertTo-Json -Depth 20) + "`n") }

# Only this throwaway repository gets commits/tags. Never change the real remote.
$scratch = Join-Path $testRoot 'source'
New-Item -ItemType Directory -Path (Join-Path $scratch 'scripts') -Force | Out-Null
Write-Utf8 (Join-Path $scratch '.gitignore') "/build/`n"
Save-JSON (Join-Path $scratch 'scripts/signing-policy.json') @{schemaVersion=1;provider='';subject='';certificateSha256=@()}
Invoke-Native git @('-C',$scratch,'init','--quiet','--initial-branch=main')
Invoke-Native git @('-C',$scratch,'add','.')
Invoke-Native git @('-C',$scratch,'-c','user.name=Workflow Test','-c','user.email=workflow@example.invalid','-c','commit.gpgsign=false','commit','--quiet','-m','isolated test source')
$commit = (& git -C $scratch rev-parse HEAD).Trim()
Invoke-Native git @('-C',$scratch,'update-ref','refs/remotes/origin/main',$commit)
Invoke-Native git @('-C',$scratch,'-c','tag.gpgsign=false','tag','v1.2.3-beta.1+build.2')
$tag = 'v1.2.3-beta.1+build.2'
$main = [pscustomobject]@{name='main';protected=$true;commit=@{sha=$commit}}
$identity = Assert-ReleaseSource $tag $commit $main $scratch
foreach ($badTag in @('v01.2.3','v1.2','v1.2.3-01','v1.2.3;echo','1.2.3','v65536.0.0')) {
    Reject "malformed $badTag" { Assert-ReleaseSource $badTag $commit $main $scratch } 'SemVer|leading zeroes|Windows version'
}
Reject 'wrong event commit' { Assert-ReleaseSource $tag ('0'*40) $main $scratch } 'event commit'
$main.protected=$false
Reject 'unprotected main' { Assert-ReleaseSource $tag $commit $main $scratch } 'protected main'
$main.protected=$true
Write-Utf8 (Join-Path $scratch 'another.txt') 'outside main'
Invoke-Native git @('-C',$scratch,'add','.')
Invoke-Native git @('-C',$scratch,'-c','user.name=Workflow Test','-c','user.email=workflow@example.invalid','-c','commit.gpgsign=false','commit','--quiet','-m','outside main')
$outside = (& git -C $scratch rev-parse HEAD).Trim()
Invoke-Native git @('-C',$scratch,'-c','tag.gpgsign=false','tag','v2.0.0')
Reject 'tag outside main' { Assert-ReleaseSource 'v2.0.0' $outside $main $scratch } 'outside protected main'
$main.commit.sha = '0'*40
Reject 'missing or stale history' { Assert-ReleaseSource 'v2.0.0' $outside $main $scratch } 'Missing/stale main'
$good = @{preflight=@{result='success'};verify=@{result='success'};build=@{result='success'}}
Assert-ReleaseGates $good @('preflight','verify','build')
foreach ($result in @('failure','cancelled','skipped','')) {
    Reject "source gate $result" { Assert-ReleaseGates @{verify=@{result=$result}} @('verify') } 'did not succeed'
}
Reject 'source gate missing' { Assert-ReleaseGates @{} @('verify') } 'did not succeed'
$script:ReleaseRepo = $scratch
try { Reject 'absent signing configuration' { Assert-SigningConfiguration } 'configuration is missing' }
finally { $script:ReleaseRepo = $repo }
Reject 'release fixture build' { & (Join-Path $PSScriptRoot 'build-desktop.ps1') -Release -Fixture -Tag $tag } 'Fixture desktop builds are forbidden'

$inputs = New-BuildDirectory $repo ('tests/workflow-inputs-' + [Guid]::NewGuid().ToString('N'))
foreach ($name in @('ArcourtDownloader.exe','arcourt-download.exe')) { Write-Utf8 (Join-Path $inputs $name) "synthetic $name" }
$manifest = Get-InputManifest $inputs $identity 'test/repo' '12' '1'
Save-JSON (Join-Path $inputs 'build-manifest.json') $manifest
Assert-InputManifest $inputs $identity 'test/repo' '12' '1'
Reject 'wrong build run' { Assert-InputManifest $inputs $identity 'test/repo' '13' '1' } 'association mismatch'
Write-Utf8 (Join-Path $inputs 'arcourt-download.exe') 'changed'
Reject 'changed unsigned input' { Assert-InputManifest $inputs $identity 'test/repo' '12' '1' } 'association mismatch'
Write-Utf8 (Join-Path $inputs 'ArcourtDownloader-fixture.exe') 'fixture'
Reject 'fixture in signing inputs' { Assert-InputManifest $inputs $identity 'test/repo' '12' '1' } 'unexpected payload files'

$assets = Join-Path $testRoot 'assets'
New-Item -ItemType Directory -Path $assets | Out-Null
$names = @("ArcourtDownloader-$tag-windows-amd64.zip","ArcourtDownloader-$tag-SBOM.spdx.json")
foreach ($name in $names) { Write-Utf8 (Join-Path $assets $name) 'synthetic candidate bytes' }
Write-Checksums $assets $names 'SHA256SUMS.txt'
$hashes = Get-CandidateChecksums $assets $tag
$marker = Get-CandidateMarker $tag $commit $hashes
$draft = [pscustomobject]@{draft=$true;immutable=$false;tag_name=$tag;target_commitish=$commit;prerelease=$true;body="Review notes`n$marker";assets=@()}
Assert-DraftCandidate $draft $tag $commit $marker $true $hashes
$draft.assets = @([pscustomobject]@{name=$names[0];state='uploaded';digest="sha256:$($hashes[$names[0]])"})
Assert-DraftCandidate $draft $tag $commit $marker $true $hashes
$draft.draft=$false
Reject 'published release overwrite' { Assert-DraftCandidate $draft $tag $commit $marker $true $hashes } 'Published/immutable'
$draft.draft=$true; $draft.immutable=$true
Reject 'immutable draft overwrite' { Assert-DraftCandidate $draft $tag $commit $marker $true $hashes } 'Published/immutable'
$draft.immutable=$false; $draft.target_commitish='main'
Reject 'rerun commit mismatch' { Assert-DraftCandidate $draft $tag $commit $marker $true $hashes } 'tag/commit'
$draft.target_commitish=$commit; $draft.body='unrecognized draft'
Reject 'rerun candidate mismatch' { Assert-DraftCandidate $draft $tag $commit $marker $true $hashes } 'checksum identity'
$draft.body=$marker; $draft.assets[0].digest='sha256:' + ('0'*64)
Reject 'rerun checksum mismatch' { Assert-DraftCandidate $draft $tag $commit $marker $true $hashes } 'checksum mismatch'
$draft.assets[0].name='other.exe'
Reject 'unexpected remote asset' { Assert-DraftCandidate $draft $tag $commit $marker $true $hashes } 'Unexpected'

$archive = Join-Path $testRoot 'artifact.zip'
[IO.Compression.ZipFile]::CreateFromDirectory($assets,$archive)
$digest = Get-SHA256 $archive
$metadata = [pscustomobject]@{id=42;name='candidate-12-1';expired=$false;digest="sha256:$digest";workflow_run=@{id=12;head_sha=$commit}}
Assert-WorkflowArtifact $metadata '42' $digest 'candidate-12-1' '12' $commit
Reject 'artifact from another run' { Assert-WorkflowArtifact $metadata '42' $digest 'candidate-12-1' '13' $commit } 'association mismatch'
Reject 'artifact from another commit' { Assert-WorkflowArtifact $metadata '42' $digest 'candidate-12-1' '12' ('0'*40) } 'association mismatch'
Reject 'artifact metadata digest' { Assert-WorkflowArtifact $metadata '42' ('0'*64) 'candidate-12-1' '12' $commit } 'association mismatch'
$expanded = Join-Path $testRoot 'expanded'
Reject 'artifact archive digest' { Expand-WorkflowArtifact $archive ('0'*64) $expanded ($names + 'SHA256SUMS.txt') } 'archive digest mismatch'
Expand-WorkflowArtifact $archive $digest $expanded ($names + 'SHA256SUMS.txt')
Reject 'artifact extraction overwrite' { Expand-WorkflowArtifact $archive $digest $expanded ($names + 'SHA256SUMS.txt') } 'destination must be new'
$zip = [IO.Compression.ZipFile]::Open($archive,[IO.Compression.ZipArchiveMode]::Update)
try { $zip.CreateEntry('../outside.exe') | Out-Null } finally { $zip.Dispose() }
Reject 'artifact traversal even with valid digest' { Expand-WorkflowArtifact $archive (Get-SHA256 $archive) (Join-Path $testRoot 'unsafe') ($names + 'SHA256SUMS.txt') } 'unsafe workflow artifact'

# Exercise the actual draft entry point with an in-process gh boundary. No network.
foreach ($file in @('stage-draft-release.ps1','workflow-common.ps1','release-common.ps1','common.ps1','resolve-build-info.ps1')) {
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot $file) -Destination (Join-Path $scratch 'scripts')
}
# Replace only the binary network boundary in the throwaway copy. A PowerShell
# function cannot emulate native byte-stream redirection used by the real gh CLI.
$mockBoundary = @'

function Receive-GitHubFile([string]$Endpoint, [string]$Path) { Mock-Download $Endpoint $Path }
'@
Add-Content -LiteralPath (Join-Path $scratch 'scripts/workflow-common.ps1') -Value $mockBoundary
Invoke-Native git @('-C',$scratch,'add','.')
Invoke-Native git @('-C',$scratch,'-c','user.name=Workflow Test','-c','user.email=workflow@example.invalid','-c','commit.gpgsign=false','commit','--quiet','-m','draft entrypoint fixture')
$mockCommit = (& git -C $scratch rev-parse HEAD).Trim()
Invoke-Native git @('-C',$scratch,'-c','tag.gpgsign=false','tag','v3.0.0-rc.1')
$mockTag='v3.0.0-rc.1'
$mockAssets=Join-Path $scratch 'build/assets'
New-Item -ItemType Directory -Path $mockAssets -Force | Out-Null
$mockNames=@("ArcourtDownloader-$mockTag-windows-amd64.zip","ArcourtDownloader-$mockTag-SBOM.spdx.json")
foreach ($name in $mockNames) { [IO.File]::WriteAllText((Join-Path $mockAssets $name), "synthetic downloaded bytes$([Environment]::NewLine)", [Text.UTF8Encoding]::new($false)) }
Write-Checksums $mockAssets $mockNames 'SHA256SUMS.txt'
$mockHashes=Get-CandidateChecksums $mockAssets $mockTag
$mockMarker=Get-CandidateMarker $mockTag $mockCommit $mockHashes
$global:ArcourtWorkflowTestState=@{Release=$null;Writes=0;ApiFailure=$false;WrongTag=$false}
$global:ArcourtWorkflowTestState.Writes=0
$global:ArcourtWorkflowTestState.ApiFailure=$false
$global:ArcourtWorkflowTestState.WrongTag=$false
function Mock-Download([string]$Endpoint, [string]$Path) {
    $id=$Endpoint.Split('/')[-1]
    $asset=@($global:ArcourtWorkflowTestState.Release.assets | Where-Object { $_.id.ToString() -ceq $id })[0]
    Copy-Item -LiteralPath (Join-Path $mockAssets $asset.name) -Destination $Path
}
function gh {
    $arguments=@($args)
    $global:LASTEXITCODE=0
    switch ($arguments[0]) {
        'api' {
            if ($global:ArcourtWorkflowTestState.ApiFailure) { $global:LASTEXITCODE=1; return }
            if ($arguments[1] -like '*/git/ref/tags/*') {
                $sha = if ($global:ArcourtWorkflowTestState.WrongTag) { '0'*40 } else { $mockCommit }
                return (@{object=@{type='commit';sha=$sha}} | ConvertTo-Json -Depth 10)
            }
            if ($arguments[1] -like '*/releases?*') {
                if ($null -eq $global:ArcourtWorkflowTestState.Release) { return '[[]]' }
                return '[[' + ($global:ArcourtWorkflowTestState.Release | ConvertTo-Json -Depth 20 -Compress) + ']]'
            }
            throw 'Unexpected mocked GitHub API read'
        }
        'attestation' { return }
        'release' {
            $global:ArcourtWorkflowTestState.Writes++
            if ($arguments[1] -eq 'create') {
                if ($arguments -cnotcontains '--draft' -or $arguments -cnotcontains '--verify-tag' -or $arguments -cnotcontains '--prerelease') { throw 'Unsafe mocked create' }
                $notes=$arguments[[Array]::IndexOf($arguments,'--notes-file')+1]
                $global:ArcourtWorkflowTestState.Release=[pscustomobject]@{id=7;draft=$true;immutable=$false;tag_name=$mockTag;target_commitish=$mockCommit;prerelease=$true;body=[IO.File]::ReadAllText($notes);assets=@();html_url='https://example.invalid/draft'}
                return
            }
            if ($arguments[1] -eq 'upload' -and $arguments -cnotcontains '--clobber') {
                $name=Split-Path -Leaf $arguments[3]
                $global:ArcourtWorkflowTestState.Release.assets += [pscustomobject]@{id=($global:ArcourtWorkflowTestState.Release.assets.Count+1);name=$name;state='uploaded';digest="sha256:$($mockHashes[$name])"}
                return
            }
            throw 'Unexpected mocked release mutation'
        }
        default { throw 'Unexpected mocked gh call' }
    }
}
$savedEnv=@{}
foreach ($key in @('GITHUB_SHA','GITHUB_REPOSITORY','GITHUB_RUN_ID','RUNNER_TEMP','GITHUB_STEP_SUMMARY')) { $savedEnv[$key]=[Environment]::GetEnvironmentVariable($key) }
try {
    $env:GITHUB_SHA=$mockCommit; $env:GITHUB_REPOSITORY='test/repo'; $env:GITHUB_RUN_ID='12'; $env:RUNNER_TEMP=$testRoot; $env:GITHUB_STEP_SUMMARY=Join-Path $testRoot 'summary.txt'
    $entry=Join-Path $scratch 'scripts/stage-draft-release.ps1'
    & $entry -Tag $mockTag -AssetsPath $mockAssets
    if ($global:ArcourtWorkflowTestState.Writes -ne 4 -or $global:ArcourtWorkflowTestState.Release.assets.Count -ne 3) { throw 'Initial draft did not create exactly three assets.' }
    $global:ArcourtWorkflowTestState.Writes=0
    & $entry -Tag $mockTag -AssetsPath $mockAssets
    if ($global:ArcourtWorkflowTestState.Writes -ne 0) { throw 'Identical draft rerun wrote remote state.' }
    $global:ArcourtWorkflowTestState.Release.assets=@($global:ArcourtWorkflowTestState.Release.assets | Select-Object -First 2)
    & $entry -Tag $mockTag -AssetsPath $mockAssets
    if ($global:ArcourtWorkflowTestState.Writes -ne 1 -or $global:ArcourtWorkflowTestState.Release.assets.Count -ne 3) { throw 'Partial draft did not recover only the missing asset.' }
    $global:ArcourtWorkflowTestState.Writes=0; $global:ArcourtWorkflowTestState.Release.draft=$false
    Reject 'entrypoint published release' { & $entry -Tag $mockTag -AssetsPath $mockAssets } 'Published/immutable'
    $global:ArcourtWorkflowTestState.Release.draft=$true; $global:ArcourtWorkflowTestState.Release.body='another candidate'
    Reject 'entrypoint draft mismatch' { & $entry -Tag $mockTag -AssetsPath $mockAssets } 'checksum identity'
    $global:ArcourtWorkflowTestState.Release.body=$mockMarker; $global:ArcourtWorkflowTestState.ApiFailure=$true
    Reject 'entrypoint API failure is not absence' { & $entry -Tag $mockTag -AssetsPath $mockAssets } 'GitHub read failed'
    $global:ArcourtWorkflowTestState.ApiFailure=$false; $global:ArcourtWorkflowTestState.WrongTag=$true
    Reject 'entrypoint moved remote tag' { & $entry -Tag $mockTag -AssetsPath $mockAssets } 'Remote tag changed'
    if ($global:ArcourtWorkflowTestState.Writes -ne 0) { throw 'Rejected staging attempted a remote write.' }
    Write-Host 'PASS draft entrypoint: initial staging, identical rerun, partial recovery, fail-closed mutations'
} finally {
    Remove-Item Function:gh
    Remove-Item Function:Mock-Download
    Remove-Variable -Name ArcourtWorkflowTestState -Scope Global
    foreach ($key in $savedEnv.Keys) { [Environment]::SetEnvironmentVariable($key,$savedEnv[$key]) }
    $script:ReleaseRepo=$repo
}

# Verify checked-in workflow contracts. actionlint performs full YAML/schema checks.
foreach ($path in (Get-ChildItem -LiteralPath (Join-Path $repo '.github/workflows') -Filter '*.yml')) {
    $yaml = Get-Content -LiteralPath $path.FullName -Raw
    foreach ($match in [regex]::Matches($yaml, '(?m)^\s+(?:- )?uses:\s+(\S+)')) {
        if ($match.Groups[1].Value -notmatch '^\./\.github/workflows/[\w.-]+\.yml$|^[\w-]+/[\w./-]+@[0-9a-f]{40}$') { throw "Unpinned action in $($path.Name)" }
    }
    foreach ($checkout in [regex]::Matches($yaml, '(?m)^      - uses: actions/checkout@[^\r\n]+\r?\n(?:(?:        |          )[^\r\n]*\r?\n)+')) {
        if ($checkout.Value -notmatch 'ref: \$\{\{ github.sha \}\}' -or $checkout.Value -notmatch 'persist-credentials: false') { throw 'Checkout must use immutable event SHA without persisted credentials.' }
    }
}
$releaseYaml = Get-Content -LiteralPath (Join-Path $repo '.github/workflows/release.yml') -Raw
if ($releaseYaml -match 'workflow_dispatch|pull_request|continue-on-error|secrets: inherit' -or $releaseYaml -notmatch "tags: \['v\*'\]" -or $releaseYaml -notmatch 'cancel-in-progress: false') { throw 'Unsafe release trigger/concurrency/fallback.' }
if ([regex]::Matches($releaseYaml,'contents: write').Count -ne 1 -or [regex]::Matches($releaseYaml,'id-token: write').Count -ne 1) { throw 'Unexpected release privilege scope.' }
$draftScript = Get-Content -LiteralPath (Join-Path $PSScriptRoot 'stage-draft-release.ps1') -Raw
if ($draftScript -notmatch "'--draft','--verify-tag'" -or $draftScript -match "'--clobber'|'--draft=false'") { throw 'Unsafe draft create/upload contract.' }
Write-Host "Release workflow rehearsals passed; no real tag, signing request, release or remote setting was created. Evidence: $testRoot"
