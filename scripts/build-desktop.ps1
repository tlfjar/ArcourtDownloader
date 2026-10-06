param([switch]$Fixture, [switch]$Release, [string]$Tag, [string]$Version)
$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
if ($Fixture -and $Release) { throw 'Fixture desktop builds are forbidden in release mode.' }
$identity = & (Join-Path $PSScriptRoot 'resolve-build-info.ps1') -Release:$Release -Tag $Tag -Version $Version
$configPath = Join-Path $repo 'desktop/wails.json'
$originalConfig = [IO.File]::ReadAllBytes($configPath)
$previousWork = $env:GOWORK
$previousCGO = $env:CGO_ENABLED
$env:GOWORK = 'off'
$env:CGO_ENABLED = '0'
try {
    $config = Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json
    $config.info.productVersion = $identity.windowsVersion
    $config.info.comments = "$($identity.version); source $($identity.commit)" + $(if ($identity.release) { '; release source' } else { '; development, unsigned' })
    [IO.File]::WriteAllText($configPath, ($config | ConvertTo-Json -Depth 10) + "`n", [Text.UTF8Encoding]::new($false))
    Push-Location (Join-Path $repo 'desktop')
    try {
        # appicon.png is reviewed source. Regenerating it here can change PNG
        # encoding between Go versions and dirty the source during a build.
        if (-not (Test-Path -LiteralPath 'build/appicon.png' -PathType Leaf)) { throw 'Missing committed desktop/build/appicon.png.' }
        $buildArgs = @('build', '-platform', 'windows/amd64', '-trimpath', '-webview2', 'error', '-m', '-nosyncgomod')
        $flags = "-X github.com/tlfjar/ArcourtDownloader/buildinfo.Version=$($identity.version) -X github.com/tlfjar/ArcourtDownloader/buildinfo.Commit=$($identity.commit) -X github.com/tlfjar/ArcourtDownloader/buildinfo.Release=$($identity.release.ToString().ToLowerInvariant())"
        $flags += " -X github.com/tlfjar/ArcourtDownloader/buildinfo.Record=ARCOURT_BUILD_V1|$($identity.version)|$($identity.commit)|$($identity.release.ToString().ToLowerInvariant())|gui|END_ARCOURT_BUILD"
        $buildArgs += @('-ldflags', $flags)
        if ($Fixture) { $buildArgs += @('-tags', 'desktopfixture', '-o', 'ArcourtDownloader-fixture.exe') }
        go run github.com/wailsapp/wails/v2/cmd/wails@v2.16.0 @buildArgs
        if ($LASTEXITCODE -ne 0) { throw 'Desktop build failed.' }
    } finally { Pop-Location }
} finally {
    try { [IO.File]::WriteAllBytes($configPath, $originalConfig) }
    finally { $env:GOWORK = $previousWork; $env:CGO_ENABLED = $previousCGO }
}
