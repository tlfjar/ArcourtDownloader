# Windows desktop application

Arcourt Downloader pins **Wails v2.16.0** with embedded HTML/CSS/JavaScript. Framework and CLI versions match.
The choice follows the official [release](https://github.com/wailsapp/wails/releases/tag/v2.16.0),
[installation prerequisites](https://v2.wails.io/docs/gettingstarted/installation/),
[build guide](https://v2.wails.io/docs/gettingstarted/building/), and
[window lifecycle options](https://v2.wails.io/docs/reference/options/).
This provides a small Go binding and
an OS webview, with the tradeoff that users need WebView2 and native UI tests need
an interactive Windows session.

`desktop/main.go` connects dialogs and window lifecycle to `desktop/internal/app`.
That controller calls the existing `arcourt.DownloadService`; the frontend never
fetches, retries, names, writes, validates, or accounts for documents. Each job
constructs the existing browser fetcher/service and closes it before releasing
the busy slot. The root service and fetcher are shared with the CLI.

## Dependencies and build

The nested `desktop/go.mod` deliberately replaces the root module with `..`, the
root of **this same repository**, and has its own `go.sum`. No parent workspace or
external repository is required. Wails resolves `golang.org/x/sys` v0.46.0 in the
desktop module; the root keeps v0.42.0. Root Go tests/builds exclude the nested
module and need no GUI tools.

Use Windows 11 x64, a supported patched Go 1.26+ toolchain, and Node.js 24 LTS or
newer. Validation used Go 1.26.8, Node 24.12.0, and npm 11.6.2. There are **no
third-party frontend dependencies**. `package-lock.json` records the empty
dependency graph. Node checks syntax, runs state tests, and copies four static
assets for embedding. No dev server, bundler, C compiler, or browser download is
needed. The GUI is supported on Windows x64; root core/CLI portability is unchanged.

From the repository root in PowerShell, with Go and Node on PATH:

```powershell
$env:GOWORK = 'off'
.\scripts\build-desktop.ps1
& '.\desktop\build\bin\ArcourtDownloader.exe'
```

The script uses the committed courthouse/download icon, invokes the
pinned Wails CLI, installs the locked frontend using `npm ci --ignore-scripts`,
builds assets/bindings, embeds Windows name/version/icon resources, and builds
`windows/amd64` with `-trimpath -webview2 error -m -nosyncgomod`. It does not upgrade
module versions or clean other builds. The ignored executable is unsigned; this
build does not produce an installer or public release. Icon source:
`desktop/tools/icon/main.go`. To deliberately update the artwork, run
`go run ./tools/icon` from `desktop/`, review `build/appicon.png`, and commit
it before building. Ordinary builds leave this tracked asset unchanged.

For a ZIP containing both production executables, source/toolchain provenance,
SHA-256 checksums, and documentation, run `scripts/package-windows.ps1` from the
repository root. It uses a unique directory under `build/packages/` and performs
no deletion. See [local packaging and release preparation](releasing.md).

## Runtime and workflow

**Microsoft Edge WebView2 Runtime** renders the interface. **Installed Microsoft
Edge or Google Chrome** separately loads court pages with chromedp. Microsoft's
[WebView2 distribution documentation](https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/distribution)
explains the distinction. Install WebView2 from
[Microsoft](https://developer.microsoft.com/en-us/microsoft-edge/webview2/).
The build's `error` strategy reports a missing runtime instead of downloading it.
`go run github.com/wailsapp/wails/v2/cmd/wails@v2.16.0 doctor` checks prerequisites.
End users need neither Go nor Node.

1. Open **Settings & diagnostics**. Supply a verified public HTTP(S) case page
   template containing `{case_number}`. No guessed court endpoint is built in.
   Leave Browser override blank for Edge/Chrome detection, or enter an absolute,
   unquoted executable path. The resolved browser is displayed.
2. Enter a case number and select **Preview case**. Check the number, title,
   county, judge, and parties, then mark the header verified.
3. Select individual dated documents or **Select All**. Selection starts empty.
   Select All covers only this preview. A coverage note explains that the court
   may have records unavailable online, including sealed or withheld records;
   unknown whole-docket coverage does not indicate failed downloads. Warnings
   identify concrete discovery limits such as truncation, pagination, and
   virtualized rows, which still make a Select All result partial.
4. **Choose output folder…**, then **Download selected**. Inspect saved, skipped,
   failed, unavailable, and canceled totals and per-document results. **Open output
   folder** opens the case subdirectory after a run. The shared service owns PDFs,
   the manifest, verified repeat skips, destination restrictions, and recovery.

Optional naming is configured in the same Settings panel. Select a provider and
model, review and consent to its direct recipient and bounded text disclosure,
then save or replace that provider's API key through the separate credential
action. The form reports only configured/missing/unavailable key status. The
setting is off by default, and a missing key or consent keeps the normal filename
without preventing download. Per-document results distinguish accepted AI labels
from controlled deterministic fallbacks. See [AI document naming](ai-document-naming.md).

Defaults: sequential downloads, at most 200 discovered documents, the core's
PDF/HTTP/retry limits, 90-second page timeout, and a 30-minute operation deadline.
The CLI offers advanced overrides. There are no accounts, telemetry, application
cloud backend, OCR, or automatic application updates. Optional AI naming sends
bounded text directly to the selected provider after downloaded PDF bytes pass
the independent local validation step.

## State, settings, and shutdown

Go owns selected IDs and the private preview. View types expose hashed IDs,
metadata, controlled errors, progress, and results. Source/request URLs and raw
error causes never cross the binding. URL-shaped strings in fetched metadata are
omitted. Settings reject credential-bearing/signed template parameters and are
not a secret store. External text is rendered with `textContent`; all UI assets
are embedded locally.

Changing the case cancels active work, clears its selection/results, and advances
the preview generation. Selection/download calls include that generation. Jobs
run on goroutines; the busy slot stays held through service return, progress
draining, and fetcher `Close`. Obsolete events/results are discarded. Revisioned
snapshots are polled every 200 ms with one poll outstanding. Local edits invalidate
in-flight snapshots and UI mutations are serialized. Terminal service results
are authoritative even if progress events were dropped. Empty/unverified
selections cannot download. A desktop run may display success when every selected
document is saved or verified as already downloaded, even when whole-docket
coverage is unknown. The coverage note remains separate from the download result.
Partial document results, cancellation, concrete Select All discovery limits,
and cleanup errors cannot display a success banner. The core API and CLI `--all`
incomplete-discovery semantics are unchanged.

Cancel interrupts the operation context. Native window close cancels off the UI
thread and waits for owned browser/file cleanup before allowing exit. Completed
PDFs remain available. Cleanup errors are surfaced before closing. Forced
termination and power loss cannot run orderly cleanup.

Preferences use `os.UserConfigDir()`:
`%APPDATA%\ArcourtDownloader\settings.json`. They contain output preference,
browser override, configured public case template, and non-secret AI naming
choices/recipient consent. API keys live in per-provider generic credentials in
the current Windows user's Credential Manager and are never returned by normal
snapshots. Credential Manager failure disables AI naming rather than storing a
plaintext key. Settings writes use a sibling temporary file followed by rename.
Corrupt/inaccessible settings show a diagnostic.
WebView2 shell data uses `%LOCALAPPDATA%\ArcourtDownloader\WebView2`.
Automation uses the core's separate temporary profiles and never attaches to
personal browser sessions. The app does not modify browser installations or the
registry.

## Verification

```powershell
$env:GOWORK = 'off'
.\scripts\check.ps1
.\scripts\build-desktop.ps1
```

The controller tests alone can also run without the native shell using
`go test ./internal/app` from `desktop`.

Run the opt-in **actual Windows GUI** fixture workflow:

```powershell
.\scripts\check-fixtures.ps1 -Desktop
```

This visibly opens `ArcourtDownloader-fixture.exe` on the current Windows desktop.
It requires installed Edge or Chrome (select Chrome with the test-only
`ARCOURT_BROWSER_EXECUTABLE` environment variable), WebView2, and an
interactive English Windows session for native dialogs. It creates
only synthetic cases/PDFs and temporary settings/output. Production excludes its
loopback fixture server, DNS/dial seams, and authenticated test-control endpoint.
Test control uses Wails' normal script executor to drive the real DOM and bindings;
no external debugging port is enabled. PowerShell/Windows UI Automation controls
the actual picker, verifies Explorer's folder, applies/removes a deny-write ACL
on a unique test directory, and checks owned browser/profile cleanup. Never
distribute the fixture executable. `TestDesktopStartup` also launches and closes
the normal executable with an isolated application-data directory. Optionally set
`ARCOURT_DESKTOP_CAPTURE` to an absolute PNG path to capture only the fixture app
window during its successful workflow.

For manual testing, launch the fixture executable with an existing absolute
`ARCOURT_DESKTOP_FIXTURE_DIR` set in the launching shell. Case `60CV-2026-1` supplies
two PDFs; `-2` removes the second after its first preview; `-3` stalls the second
PDF for Cancel/window-close checks; `-4` supplies two PDFs for output-permission
testing; `-5` returns a different case header to verify mismatch rejection.
Restart with a new fixture directory to reset load counts. No fixture
test accesses a court site or personal data.

See [testing](testing.md) for reproducible checks and separate manual/live gates. Synthetic testing cannot verify current court
selectors, authentication requirements, or complete-docket coverage.
