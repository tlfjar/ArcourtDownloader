# Testing

Tests use fabricated cases, generated PDF bytes, temporary local files, and
loopback HTTP servers. Automated checks and CI must not contact the court or use
client documents, credentials, signed URLs, or personal browser profiles. Test
fixtures are not evidence of current court compatibility or complete records.

## Tooling and module layout

The checked-in workflow pins Go 1.26.8 and Node 24.12.0; the desktop uses Wails
v2.16.0. Use PowerShell 5.1 or 7 on Windows 11 x64, with Go, Node and npm on PATH.
Modules require Go 1.26+ and the frontend requires Node 24+. Initial tool/module
downloads require internet access. Browserless core/controller checks also run on
Linux in CI using PowerShell 7. Windows GUI support requires WebView2, with Edge
or Chrome separately installed for browser automation.

Release/local packaging and asset verification require PowerShell 7.4+ and the
reviewed Go 1.26.8 toolchain. After `scripts/package-windows.ps1`, run
`scripts/test-release-packaging.ps1 -AssetsPath <printed-directory>` for archive
mutation and release rejection checks. These use isolated temporary assets and
a temporary Git repository; they do not create tags in this repository or sign
anything. Synthetic signer tests demonstrate policy logic only, not public
signature readiness.

The root module owns the core and CLI. The nested `desktop/` module uses its own
lockfile and a legitimate `replace` to `..`. Root `go test ./...` does not test
that module. Scripts use `GOWORK=off` and read-only module resolution, stop on
native failures, and restore their temporary environment settings. Preserve both
Go lockfiles and `desktop/frontend/package-lock.json`.

## Browserless checks

From the repository root:

```powershell
.\scripts\check.ps1
# Go core and CLI only, without Node or desktop checks:
.\scripts\check.ps1 -CoreOnly
```

The full command verifies both Go module caches, runs core/CLI tests, vet and
builds, installs the locked frontend with scripts disabled, runs Node state tests
and syntax checks, and builds frontend assets. On Windows it tests/vets the full
desktop module and compiles a desktop check executable; on other platforms it
tests/vets the controller. Browser/interactive opt-ins are cleared during these
checks. Executables are written to ignored `build/bin/`, assets to
`desktop/frontend/dist/`. A check executable is not a production desktop build.

`scripts/test-build-info.ps1` covers unborn Git repositories and malformed or
mismatched release identities. Windows CI runs it in both PowerShell 7 and
Windows PowerShell 5.1, including the expected missing-HEAD probe before the
first source commit.

Coverage includes:

- Case normalization and identity mismatch, DOM parsing, synthetic OPAD schema,
  attachments, dates, bounded discovery, and malformed/denied responses.
- Explicit selection, disappearance, partial success, retry/cancellation, PDF
  screening, URL/redirect/DNS policy, and omission of sensitive URL/error data.
- Safe paths, no-overwrite publication, process locking, receipt/manifest recovery,
  verified repeat skips, injected I/O failure, and interrupted processes.
- CLI argument bounds, text/JSON output and exit codes; desktop selection,
  generations, cancellation, progress, settings, and frontend state transitions.

Windows-only tests cover junctions and open-handle rename failure. Symbolic-link
tests may skip when the account cannot create links; inspect and record skips.
Optional race checks require a compatible native C toolchain:

```powershell
.\scripts\check.ps1 -Race
```

CI separately runs `go test -race ./... -count=1 -timeout=180s` in the root and
`go test -race ./internal/app -count=1 -timeout=180s` in `desktop/` on Linux.

## Installed-browser fixtures

```powershell
.\scripts\check-fixtures.ps1
```

This opts into `TestBrowserSmoke`, `TestDocketDOMFixtures`, and
`TestCLIBrowserFixtures` against local synthetic servers. It checks isolated
browser/profile ownership and cleanup, DOM extraction, real CLI PDF/manifest
output, hashes, repeat skips, and incomplete selection behavior, including paths
with spaces. Edge or Chrome must be installed. The optional
`ARCOURT_BROWSER_EXECUTABLE` test variable selects an installed executable; it is
not a CLI configuration option. Test transport hooks map validated destinations
to loopback without adding a production localhost exception.

## Windows builds and interactive checks

```powershell
.\scripts\build-desktop.ps1
# Requires an interactive English Windows session, Edge and WebView2:
.\scripts\check-fixtures.ps1 -Desktop
```

The build produces unsigned `desktop/build/bin/ArcourtDownloader.exe`. The
interactive command builds production and fixture executables and launches native
startup and GUI tests, including dialogs, output permission failure, cancellation,
window close, and owned-process cleanup. It opens visible windows and uses Windows
UI Automation. Never distribute `ArcourtDownloader-fixture.exe`. See the
[desktop guide](desktop-architecture.md#verification) for fixture scenarios.

Interactive fixture automation is separate from a human walkthrough of the
production GUI. Neither a cross-build nor controller tests satisfy that gate.
When an interactive session is unavailable, report it as pending. Local package
instructions and remaining release work are in [releasing](releasing.md).

## CI and clean-source checks

[The verification workflow](../.github/workflows/check.yml) runs browserless checks
on Ubuntu and Windows, Linux race checks, Windows local browser fixtures, and a
Windows desktop/local package build. It checks Git integrity and verifies that
checks/builds preserve tracked sources and lockfiles. CI does not perform live
court requests, human GUI walkthroughs, signing, or release publication.

CI also runs pinned govulncheck for each Go module's Windows packages, workflow
policy/rejection rehearsals, actionlint, and dependency review on public PRs.
The desktop/local-package job runs the archive mutation tests. Reproduce the
additional source checks with PowerShell 7.4+ and Go 1.26.8:

```powershell
.\scripts\check-vulnerabilities.ps1 -Module root
.\scripts\check-vulnerabilities.ps1 -Module desktop
.\scripts\test-release-workflow.ps1
go test ./scripts/signing-tool -count=1
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -shellcheck= -pyflakes=
if ($LASTEXITCODE -ne 0) { throw 'Workflow validation failed' }
```

Workflow rehearsals use isolated Git repositories and synthetic artifacts. They
cover malformed tags, tags outside protected main, absent/failed exact-source
gates, missing provider configuration, fixture inputs, wrong build association,
draft checksum/commit mismatch, and published/immutable overwrite refusal. The
PE comparison tests alter executable bytes, certificate layout and padding.
These tests never create a real repository tag/release or call a signing provider.
The separate tag-only release workflow reuses all source gates before signing,
packaging, attesting and staging a draft; publication remains an operator action.
Public dependency review, hosted signing and hosted attestations require actual
hosted evidence. Private skipped/unavailable controls remain pending. Configure
CodeQL default setup in GitHub; no advanced CodeQL workflow is configured here.

To check reproducibility, use a separate clean checkout, run the applicable
commands above, then run `git diff --exit-code` and `git status --short`. In
PowerShell, check `$LASTEXITCODE` after each native Git command and throw on a
nonzero result. Record source revision, tool versions, commands, results and
skips privately under ignored `build/`; never include downloaded case material.

## Manual live boundary

Past evidence is limited to the [date/scope statement](verification.md). Current
compatibility requires a separate, authorized human check. For a release
candidate, record only its version/source and executable hashes, date, and
outcome. Preview an authorized case, confirm the header, select and download a
PDF, open it, repeat to verify a skip, then close and check owned-browser cleanup.
Keep case identifiers, docket labels, documents, URLs, and screenshots private.
The production human walkthrough and final signed-candidate live check remain
pending; do not substitute synthetic tests for them.
