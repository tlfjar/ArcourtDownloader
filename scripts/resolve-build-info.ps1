param(
    [switch]$Release,
    [string]$Tag,
    [string]$Version,
    [string]$Repository = (Split-Path -Parent $PSScriptRoot)
)
$ErrorActionPreference = 'Stop'

function Git-Value {
    param([string[]]$Arguments)
    $result = & git -C $Repository @Arguments 2>$null
    if ($LASTEXITCODE -ne 0) { throw "Git identity check failed: git $($Arguments -join ' ')" }
    return ([string]($result -join "`n")).Trim()
}

if (-not $Release -and ($Tag -or $Version)) {
    throw 'Tag and version overrides require -Release.'
}
if ($Release -and -not $Tag) { throw 'Release builds require -Tag.' }
if ($Release) {
    # SemVer 2.0.0. Numeric identifiers cannot contain leading zeroes.
    $semver = '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$'
    if (-not $Tag.StartsWith('v', [StringComparison]::Ordinal) -or -not [regex]::IsMatch($Tag.Substring(1), $semver)) {
        throw 'Release tag must be v followed by valid SemVer 2.0.0.'
    }
    $semantic = $Tag.Substring(1)
    $match = [regex]::Match($semantic, $semver)
    if ($match.Groups[4].Success) {
        foreach ($identifier in $match.Groups[4].Value.Split('.')) {
            if ($identifier -match '^[0-9]+$' -and $identifier.Length -gt 1 -and $identifier[0] -eq '0') {
                throw 'Numeric prerelease identifiers cannot have leading zeroes.'
            }
        }
    }
    if ($Version -and $Version -cne $semantic) { throw 'Version override disagrees with the release tag.' }
    foreach ($part in @(1, 2, 3)) {
        if ([int64]::Parse($match.Groups[$part].Value) -gt 65535) {
            throw 'Major, minor, and patch must fit Windows version fields (0..65535).'
        }
    }
    $head = Git-Value -Arguments @('rev-parse', '--verify', 'HEAD^{commit}')
    Git-Value -Arguments @('show-ref', '--verify', '--quiet', "refs/tags/$Tag") | Out-Null
    $tagCommit = Git-Value -Arguments @('rev-parse', '--verify', "refs/tags/$Tag^{commit}")
    if ($tagCommit -cne $head) { throw 'Release tag does not peel to HEAD.' }
    $changes = Git-Value -Arguments @('status', '--porcelain=v1', '--untracked-files=all')
    if ($changes) { throw 'Release source has tracked or relevant untracked changes.' }
    # Windows numeric resources have four 16-bit fields. Reserve the highest
    # fourth field for stable releases; prerelease labels remain in text identity.
    $fourth = if ($match.Groups[4].Success) { 0 } else { 65535 }
    return [pscustomobject]@{
        version = $semantic
        tag = $Tag
        commit = $head
        release = $true
        windowsVersion = "$($match.Groups[1].Value).$($match.Groups[2].Value).$($match.Groups[3].Value).$fourth"
    }
}

# A source-only export is built before its first public commit. Git has no HEAD
# then, so embed an explicit placeholder rather than the old private source SHA.
# --quiet keeps the expected unborn-HEAD probe off stderr. Windows PowerShell
# 5.1 otherwise promotes that native stderr to a terminating NativeCommandError.
$head = & git -C $Repository rev-parse --verify --quiet 'HEAD^{commit}' 2>$null
if ($LASTEXITCODE -ne 0) { $head = 'unborn' }
$head = ([string]$head).Trim()
$changes = & git -C $Repository status --porcelain=v1 --untracked-files=all 2>$null
if ($LASTEXITCODE -ne 0) { throw 'Development build requires a Git working tree.' }
$commit = if ($changes) { "$head-dirty" } else { $head }
return [pscustomobject]@{
    version = 'development'
    tag = ''
    commit = $commit
    release = $false
    windowsVersion = '0.0.0.0'
}
