# Shared by repository scripts; every native failure must stop the calling script.
function Invoke-Native {
    param([string]$Command, [string[]]$Arguments)
    & $Command @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$Command failed with exit code $LASTEXITCODE." }
}

function New-BuildDirectory {
    param([string]$Repository, [string]$RelativePath)
    $root = [IO.Path]::GetFullPath((Join-Path $Repository 'build'))
    $target = [IO.Path]::GetFullPath((Join-Path $root $RelativePath))
    if (-not $target.StartsWith($root + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
        throw 'Build destination must be inside the repository build directory.'
    }
    # Refuse junction/symlink destinations, including existing ancestors.
    $probe = $target
    while ($probe) {
        if (Test-Path -LiteralPath $probe) {
            if ((Get-Item -LiteralPath $probe -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) {
                throw "Build destination traverses a reparse point: $probe"
            }
        }
        $probe = Split-Path -Parent $probe
    }
    New-Item -ItemType Directory -Path $target -Force | Out-Null
    return $target
}
