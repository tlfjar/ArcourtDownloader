$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')
$repo = Split-Path -Parent $PSScriptRoot
$fixture = New-BuildDirectory $repo ('tests/build-info-' + [Guid]::NewGuid().ToString('N'))
$resolver = Join-Path $PSScriptRoot 'resolve-build-info.ps1'

function Expect-Failure {
    param([string]$Name, [scriptblock]$Action)
    try { & $Action | Out-Null }
    catch { Write-Output "PASS: $Name"; return }
    throw "Expected failure: $Name"
}

Invoke-Native git @('-C', $fixture, 'init', '-q')
$unborn = & $resolver -Repository $fixture
if ($unborn.version -ne 'development' -or $unborn.commit -ne 'unborn' -or $unborn.release) { throw 'Unborn identity is incorrect.' }
Write-Output 'PASS: unborn development build'

Invoke-Native git @('-C', $fixture, 'config', 'user.name', 'Build Info Test')
Invoke-Native git @('-C', $fixture, 'config', 'user.email', 'build-info@example.invalid')
[IO.File]::WriteAllText((Join-Path $fixture 'source.txt'), "one`n", [Text.UTF8Encoding]::new($false))
Invoke-Native git @('-C', $fixture, 'add', 'source.txt')
Invoke-Native git @('-C', $fixture, 'commit', '-q', '-m', 'fixture')
$head = (& git -C $fixture rev-parse HEAD).Trim()

foreach ($tag in @('v0.1.0-beta.1', 'v0.1.0', 'v1.2.3+build.5')) {
    Invoke-Native git @('-C', $fixture, 'tag', '-a', $tag, '-m', $tag)
    $actual = & $resolver -Repository $fixture -Release -Tag $tag
    $fourth = if ($tag.Contains('-')) { '0' } else { '65535' }
    if ($actual.version -ne $tag.Substring(1) -or $actual.commit -ne $head -or -not $actual.release -or -not $actual.windowsVersion.EndsWith(".$fourth")) {
        throw "Release identity is incorrect for $tag."
    }
    Write-Output "PASS: $tag"
}
Expect-Failure 'malformed SemVer' { & $resolver -Repository $fixture -Release -Tag 'v01.2.3' }
Expect-Failure 'numeric prerelease leading zero' { & $resolver -Repository $fixture -Release -Tag 'v1.2.3-beta.01' }
Expect-Failure 'Windows field overflow' { & $resolver -Repository $fixture -Release -Tag 'v65536.0.0' }
Expect-Failure 'version disagreement' { & $resolver -Repository $fixture -Release -Tag 'v0.1.0' -Version '0.2.0' }
Expect-Failure 'missing tag' { & $resolver -Repository $fixture -Release -Tag 'v0.2.0' }
Expect-Failure 'tag without release mode' { & $resolver -Repository $fixture -Tag 'v0.1.0' }
Expect-Failure 'fixture desktop in release mode' { & (Join-Path $PSScriptRoot 'build-desktop.ps1') -Fixture -Release -Tag 'v0.1.0' }

[IO.File]::WriteAllText((Join-Path $fixture 'untracked.txt'), "new`n", [Text.UTF8Encoding]::new($false))
Expect-Failure 'untracked source' { & $resolver -Repository $fixture -Release -Tag 'v0.1.0' }
Remove-Item -LiteralPath (Join-Path $fixture 'untracked.txt')
[IO.File]::AppendAllText((Join-Path $fixture 'source.txt'), "two`n")
Expect-Failure 'dirty tracked source' { & $resolver -Repository $fixture -Release -Tag 'v0.1.0' }
Invoke-Native git @('-C', $fixture, 'add', 'source.txt')
Invoke-Native git @('-C', $fixture, 'commit', '-q', '-m', 'later')
Expect-Failure 'HEAD and tag mismatch' { & $resolver -Repository $fixture -Release -Tag 'v0.1.0' }
Write-Output "Build identity test fixtures: $fixture"
