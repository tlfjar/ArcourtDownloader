param(
    [ValidateSet('FAT32', 'exFAT')][string]$Filesystem = 'FAT32',
    [switch]$Require
)
$ErrorActionPreference = 'Stop'
# This harness only formats a newly created, disposable VHD. It never selects a
# physical disk, accepts a disk number, or formats a caller-supplied destination.
if ($env:OS -ne 'Windows_NT') {
    if ($Require) { throw 'Native Windows is required for filesystem integration tests.' }
    Write-Output 'SKIP: native Windows FAT32/exFAT fixture unavailable.'
    return
}
$principal = [Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    if ($Require) { throw 'Administrator privileges are required to attach the disposable VHD.' }
    Write-Output 'SKIP: no administrator privileges for a disposable FAT32/exFAT VHD.'
    return
}
$repo = Split-Path -Parent $PSScriptRoot
$fixture = Join-Path ([IO.Path]::GetTempPath()) ('arcourt-fs-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $fixture | Out-Null
$vhd = Join-Path $fixture 'fixture.vhd'
$logDir = Join-Path $repo 'build/filesystem-tests'
New-Item -ItemType Directory -Force -Path $logDir | Out-Null
$letter = @('R','S','T','U','V','W','X','Y','Z','Q','P') | Where-Object {
    -not (Test-Path ($_ + ':\')) -and -not (Get-PSDrive -Name $_ -ErrorAction SilentlyContinue)
} | Select-Object -First 1
if (-not $letter) { throw 'No unused drive letter is available for the disposable VHD.' }
$oldRoot = $env:ARCOURT_TEST_VOLUME_ROOT
$oldFS = $env:ARCOURT_TEST_VOLUME_FS
$oldWork = $env:GOWORK
$oldFlags = $env:GOFLAGS
try {
    $commands = @"
create vdisk file="$vhd" maximum=128 type=expandable
select vdisk file="$vhd"
attach vdisk
create partition primary
format fs=$Filesystem quick
assign letter=$letter
exit
"@
    $setup = Join-Path $fixture 'setup.txt'
    Set-Content -LiteralPath $setup -Value $commands -Encoding ASCII
    & diskpart.exe /s $setup | Tee-Object -FilePath (Join-Path $logDir "$Filesystem-diskpart.txt")
    if ($LASTEXITCODE -ne 0) { throw 'Failed to create/attach the disposable volume.' }
    $env:ARCOURT_TEST_VOLUME_ROOT = $letter + ':\'
    $env:ARCOURT_TEST_VOLUME_FS = $Filesystem
    $env:GOWORK = 'off'
    $env:GOFLAGS = '-mod=readonly'
    Push-Location $repo
    try {
        # The fixture test verifies the actual filesystem; TestMove uses automatic
        # production capability selection here, not the synthetic backend override.
        & go test ./arcourt -run '^(TestMove|TestWindowsFilesystemFixture)' -v -count=1 -timeout=180s 2>&1 |
            Tee-Object -FilePath (Join-Path $logDir "$Filesystem-tests.txt")
        if ($LASTEXITCODE -ne 0) { throw "$Filesystem integration tests failed." }
    } finally { Pop-Location }
} finally {
    $env:ARCOURT_TEST_VOLUME_ROOT = $oldRoot
    $env:ARCOURT_TEST_VOLUME_FS = $oldFS
    $env:GOWORK = $oldWork
    $env:GOFLAGS = $oldFlags
    # Detach by the exact VHD path we created, never by drive/disk enumeration.
    $detach = Join-Path $fixture 'detach.txt'
    Set-Content -LiteralPath $detach -Value "select vdisk file=`"$vhd`"`r`ndetach vdisk`r`nexit" -Encoding ASCII
    & diskpart.exe /s $detach | Out-Host
    if ($LASTEXITCODE -ne 0) {
        Write-Warning "Could not detach disposable VHD; retained at $vhd"
    } else {
        Remove-Item -LiteralPath $fixture -Recurse -Force
    }
}
