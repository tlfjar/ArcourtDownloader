# VS Code setup

Open the repository root in a local Windows VS Code window for Windows browser
and desktop development. WSL/container sessions use their own toolchain, browser,
debugger and filesystem and do not validate Windows behavior.

## First opening

1. Install the [build prerequisites](../docs/testing.md#tooling-and-module-layout)
   on the host where VS Code runs. The root and `desktop/` modules each have their
   own `go.mod`/`go.sum`; the desktop's replacement points to the repository root.
2. Open Extensions and filter by `@recommended` to choose workspace extensions.
   Recommendations do not change account sign-in or agent permissions.
3. Run **Go: Install/Update Tools** and select `gopls`, `goimports` and `dlv`.
   The optional lint task also needs a compatible `golangci-lint` v2 on PATH.
4. Run **Go: Environment** to inspect the root toolchain. On local Windows,
   `GOOS` should be `windows`, `GOMOD` should point to the root `go.mod`, and
   `GOWORK` should be `off`. Run the same command from `desktop/` in a terminal
   to inspect that module. If the language server needs a separate module folder,
   add `desktop/` to the VS Code workspace; no Go workspace file is required.

## Ordinary tasks and debugging

| Action | VS Code entry | Scope / prerequisite |
| --- | --- | --- |
| Inspect toolchain | Go: Environment | Root module, Go on PATH |
| Compile core/CLI | Ctrl+Shift+B | Root module |
| Ordinary tests | Tasks: Run Test Task | Root module, browserless |
| Build, vet, test | Go: Verify core | Root module |
| Test current package | Go: Test active package (short) | Open a Go file, including desktop/controller files |
| Debug package tests | F5, arcourt or active-package launch | Delve installed |
| Debug one test | Go extension Debug Test CodeLens | Delve installed |
| Coverage | Go: Coverage (short) | Root module; ignored coverage.out |
| Race detector | Go: Race tests (optional native C toolchain) | Supported native CGO toolchain |
| Optional lint | Go: Lint (optional golangci-lint v2) | Compatible v2 binary |

Root tasks do not cover the nested desktop module. Run `scripts/check.ps1` from
the integrated terminal for core, CLI, frontend and desktop checks, and
`scripts/build-desktop.ps1` for the production Windows GUI. Use
`scripts/check-fixtures.ps1` for browser fixtures; `-Desktop` additionally opens
the native GUI fixtures. See [testing](../docs/testing.md) for requirements and
the distinction between automated, interactive and live checks. Build frontend
assets before debugging desktop package tests that embed them.

Tasks execute programs directly with argument arrays, so paths containing spaces
do not rely on shell quoting. Nothing runs automatically on opening the folder.
Select a different terminal profile if the detected PowerShell profile is unsuitable.

## Settings

The configuration provides `goimports` formatting/import organization, `gopls`
analysis, semantic highlighting, CodeLens, file nesting, a five-minute test timeout,
an 88-column ruler and LF endings. Tests and coverage do not run on save. Delve
launches use `dlv-dap` with a 2048-character string limit.

`GOWORK=off` is set for Go tools, debug launches, tasks and new terminals; it does
not change the machine environment. `GOOS`, `GOARCH` and `CGO_ENABLED` remain
native. No browser/live test opt-ins or integration tags are enabled by default.
The optional standalone linter is off on save. Build output and local downloads
are excluded from search/watch activity; source and build configuration remain
visible. Keep sensitive output outside the checkout or under ignored `downloads/`.

References: [Go extension settings](https://github.com/golang/vscode-go/wiki/settings),
[debugging](https://github.com/golang/vscode-go/wiki/debugging), and
[VS Code tasks](https://code.visualstudio.com/docs/debugtest/tasks).
