#requires -Version 7.4
param(
    [Parameter(Mandatory)][string]$RequestPath,
    [Parameter(Mandatory)][string]$UnsignedPath,
    [Parameter(Mandatory)][string]$OutputPath
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'workflow-common.ps1')
Assert-SigningConfiguration | Out-Null
# Replace this failure with ONE enrolled provider's pinned integration.
# Read docs/releasing.md for request, response, environment and receipt contracts.
# Never return unsigned inputs or generate a self-signed certificate here.
throw 'Signing provider integration is not configured. See docs/releasing.md. No release candidate was generated.'
