# Development setup and source builds

This guide is for contributors and anyone building ArcourtDownloader from source.
End users can use the portable executables described in the [README](../README.md).

## Prerequisites

Use Windows 11 x64, Git, PowerShell 5.1 or 7, Go 1.26+, and Node.js 24+ with npm.
The checked workflow uses Go 1.26.8, Node.js 24.12.0, npm 11.6.2, and Wails
v2.16.0. Initial tool and module downloads need internet access. The scripts
invoke a pinned Wails version, so a global Wails installation and a C compiler
are not needed for ordinary Windows builds.

The desktop app needs WebView2 Runtime to run, plus an installed Edge or Chrome
for browser automation. Go and Node are build tools, not runtime dependencies
for the packaged executables.

## Clone, check, and build

From PowerShell:

```powershell
git clone https://github.com/tlfjar/ArcourtDownloader.git
if ($LASTEXITCODE -ne 0) { throw 'Clone failed' }
Set-Location ArcourtDownloader

.\scripts\check.ps1
.\scripts\build-desktop.ps1
& '.\desktop\build\bin\ArcourtDownloader.exe'
```

To build only the CLI, run `.\scripts\build-cli.ps1`; its executable is written
under `build/bin/`. If execution policy blocks a script, invoke it for that
process with `powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\check.ps1`.
Keep actual downloads outside the checkout or under ignored `downloads/`.

## Repository layout

The root Go module, `github.com/tlfjar/ArcourtDownloader`, contains the shared
`arcourt` package and the thin CLI under `cmd/arcourt-download/`. The separate
`desktop/` Go module holds the Wails shell and controller; its in-repository
replacement points to the root module. The frontend lives under
`desktop/frontend/`. Keep both Go lockfiles and the frontend lockfile. Use
`GOWORK=off` so root checks do not accidentally include the desktop module.

The [desktop architecture](desktop-architecture.md) covers the UI and its build.
The [browser runtime](browser-runtime.md) covers browser selection, process
isolation, configuration, and lifecycle. Library callers should also read the
[document fetch contract](document-fetch-contract.md) and
[filesystem service guide](filesystem-download-service.md).

## Checks and contribution workflow

```powershell
.\scripts\check.ps1                 # Core, CLI, frontend, and desktop checks
.\scripts\check.ps1 -CoreOnly       # Root Go module only
.\scripts\check-fixtures.ps1        # Installed-browser, local fixtures
```

Automated tests use synthetic data and do not contact the court. The
[testing guide](testing.md) describes fixture requirements, native GUI checks,
and the separate manual live boundary. A production GUI walkthrough is still
pending; the [verification record](verification.md) gives the date and scope of
past live checks. See [release preparation](releasing.md) for packaging and the
unsigned release procedure, [VS Code setup](../.vscode/README.md) for editor
tasks, and [contributing](../CONTRIBUTING.md) for pull request expectations.
