# ArcourtDownloader

An **unofficial Windows utility** for previewing and downloading publicly available
Arkansas court case documents. It is not affiliated with or endorsed by the
Arkansas Judiciary or Administrative Office of the Courts (AOC).

The desktop app and CLI run locally and save selected PDFs and a manifest in your
chosen local folder. Court requests go directly to the configured public site;
there is no application server, telemetry, account service, or cloud storage.
The app does not provide official notice or guarantee complete court records.

**Runtime:** Windows 11 x64, Microsoft Edge WebView2 Runtime for the desktop UI,
and a separately installed Microsoft Edge or Google Chrome for browser automation.
Use a writable local NTFS output folder. End users do not need Go or Node.

## Download or build

No published release or signed candidate is established yet. The planned download
location is [GitHub Releases](https://github.com/tlfjar/ArcourtDownloader/releases).
When a reviewed portable ZIP is available, extract it to a local folder and launch
`ArcourtDownloader.exe`; the CLI is `arcourt-download.exe`. There is no installer
or automatic updater. Current source builds are unsigned development artifacts.

Install [WebView2 Runtime](https://developer.microsoft.com/en-us/microsoft-edge/webview2/)
for the UI and Edge or Chrome separately. Source builds use Git, Go 1.26.8,
Node.js 24.12.0 (npm 11.6.2), and PowerShell 5.1 or 7. Modules require Go 1.26+;
the frontend requires Node 24+. Wails v2.16.0 is invoked by version, without a
global Wails install or a C compiler for ordinary Windows builds.

```powershell
git clone https://github.com/tlfjar/ArcourtDownloader.git
if ($LASTEXITCODE -ne 0) { throw 'Clone failed' }
Set-Location ArcourtDownloader
.\scripts\check.ps1
.\scripts\build-desktop.ps1
& '.\desktop\build\bin\ArcourtDownloader.exe'
```

Initial tool/module downloads need internet access. If execution policy blocks a
script, use a process-local invocation such as
`powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\check.ps1`.
See [testing](docs/testing.md) for checks and [releasing](docs/releasing.md) for
local unsigned packaging and the remaining distribution gates. A human production
GUI walkthrough remains pending; [past live evidence](docs/verification.md) is
limited to its recorded date and scope.

## Desktop workflow

1. Open **Settings & diagnostics** and configure a public case-page template
   containing `{case_number}`. The exact template
   `https://caseinfonew.arcourts.gov/opad/case/{case_number}` uses the public OPAD
   case API; custom templates use browser discovery. Leave Browser override empty
   for detection or enter an unquoted executable path.
2. Enter the case you intend to access and select **Preview case**. Confirm its
   number, title, parties and other header details, then mark the header verified.
   Mismatched cases are rejected.
3. Select documents or **Select All**. Read discovery warnings: Select All covers
   this preview. Records unavailable online, including sealed or withheld records,
   cannot be checked. Unknown whole-docket coverage does not mean a download failed.
4. Choose an existing local output folder, select **Download selected**, and
   inspect saved/skipped/failed/unavailable/canceled results. **Open output folder**
   opens the case directory. Retry failed items by selecting them again.

Downloads are conservative and sequential. Cancel stops pending work and preserves
completed PDFs. Window close waits for active work and owned-browser cleanup.
Changing the case clears the old selection and cancels its work; empty/unverified
selections cannot download. When every selected document is saved or verified as
already downloaded, the GUI reports success unless another run error or concrete
Select All discovery limit applies. Truncation, detected pagination and virtualized
rows still cause a partial Select All result. See the
[desktop guide](docs/desktop-architecture.md) for settings and lifecycle details.

## CLI and development

Use `arcourt-download.exe preview` to inspect metadata and document IDs, then
`arcourt-download.exe download` with explicit `--id` options or `--all`. Configure
the case URL template explicitly. The [CLI guide](docs/command-line-workflow.md)
documents PowerShell examples, configuration, JSON output, limits and exit codes.
CLI `--all` retains its conservative incomplete-discovery exit status even when
all discovered documents download successfully.

The root Go module is `github.com/tlfjar/ArcourtDownloader`; import its `arcourt`
package. It contains the shared core and thin CLI. The separate `desktop/` module
keeps Wails dependencies out of core builds, with an in-repository replacement
pointing to the root. Both Go modules and the frontend keep their lockfiles.

```powershell
.\scripts\check.ps1                 # Core, CLI, frontend and desktop checks
.\scripts\check.ps1 -CoreOnly       # Root Go module only
.\scripts\check-fixtures.ps1        # Installed-browser, loopback fixtures
```

Tests use synthetic data and do not contact the court. See
[testing](docs/testing.md), [contributing](CONTRIBUTING.md), and
[VS Code setup](.vscode/README.md). Local builds belong under ignored `build/`;
keep downloads outside the checkout or under ignored `downloads/`.

## Browser fetching

Construct `arcourt.NewBrowserFetcher` with `BrowserFetcherConfig`, supplying
`CaseURLTemplate` containing `{case_number}` and setting `Headless` as desired.
The endpoint is caller-configured; the library does not read endpoint configuration
from the environment or supply a default court URL. `PreviewCaseDocuments` returns
validated case metadata, document identities, and discovery coverage.
`DownloadCaseDocuments` takes an explicit selection and a per-document staging
writer, and returns outcomes that preserve partial success. See the
[public fetch contract](docs/document-fetch-contract.md) for API semantics, limits,
HTTP policy, and a programmatic example.

Application hosts should use `arcourt.NewDownloadService(fetcher)`, then `Preview`
and `Download` with explicit preview entries and an existing absolute output
directory. The service saves PDFs in a safe case subdirectory, verifies repeat-run
skips, preserves existing files, and maintains a versioned local manifest with
recovery after interrupted publication. Inspect both results and errors for partial
success. See the [filesystem service guide](docs/filesystem-download-service.md)
for a minimal example, manifest schema, progress, cancellation, and output policy.

Browser installation is a runtime prerequisite: install Microsoft Edge or Google
Chrome before constructing the fetcher. This is separate from any webview runtime
(WebView2 for the desktop GUI). The library does
not download browsers, modify the registry, or require administrator access.

On Windows, discovery prefers Edge, then Chrome. For each browser it probes
`ProgramFiles(x86)`, `ProgramW6432`, `ProgramFiles`, and `LOCALAPPDATA`, then PATH
(`msedge.exe` or `chrome.exe`). The installation suffixes are
`Microsoft/Edge/Application/msedge.exe` and `Google/Chrome/Application/chrome.exe`.
Missing or relative environment roots, directories, and other non-files are
skipped. Thus an Edge executable on PATH takes priority over installed Chrome.
These locations follow the vendors' installation conventions; see Microsoft's
[Edge executable and temporary profile example](https://learn.microsoft.com/en-us/troubleshoot/microsoft-edge/performance/edge-crashes-fails-to-launch#start-by-using-a-temporary-profile)
and Chromium's [Windows installer documentation](https://chromium.googlesource.com/chromium/src/+/HEAD/chrome/installer/mini_installer/README.md).
Linux searches PATH for Chrome, Chromium, and Edge; macOS also checks system and
per-user Applications bundles. Unusual installations can use the explicit option.

Set `BrowserFetcherConfig.ExecutablePath` to an unquoted executable file path to
override discovery, including paths containing spaces. Relative explicit paths
are resolved against the working directory at construction. An invalid override
fails immediately without selecting a different browser. `BrowserInfo()` reports
the selection before launch. `Headless: true` runs without a window;
`Headless: false` (the zero value) displays the isolated browser window.
`PageTimeout` applies to startup and page loading for both previews and fetches
(default 90 seconds). No source edits are required to change these options.

```go
fetcher, err := arcourt.NewBrowserFetcher(arcourt.BrowserFetcherConfig{
    CaseURLTemplate: configuredCaseURLTemplate,
    ExecutablePath:  configuredBrowserPath, // empty enables discovery
    Headless:        true,
    PageTimeout:     90 * time.Second,
})
if err != nil {
    return err
}
defer func() {
    if err := fetcher.Close(); err != nil {
        log.Printf("browser cleanup: %v", err)
    }
}()
preview, err := fetcher.PreviewCaseDocuments(ctx, caseNumber)
```

Each operation using browser discovery launches an application-owned process with a fresh temporary
profile. It does not attach to existing browsers or use their cookies or profiles.
Cleanup waits for the owned process and removes its profile on success, page or
launch failure, cancellation, or timeout. Call `Close()` on application shutdown
and wait for it before process exit; it cancels active operations, waits for
cleanup, and reports profile removal failures. New operations after close fail.
Forced termination or power loss cannot run Go cleanup; orderly application
shutdown must call `Close()` rather than exiting directly.

The normal sandbox and TLS verification remain enabled, including when chromedp
would otherwise disable the sandbox for Linux root. There is no sandbox bypass
option; Linux development should use a non-root account with a working browser
sandbox. Startup errors identify the executable, selection source, headless mode,
timeout, and relevant troubleshooting steps. They omit raw browser stderr, page
URLs, and profile contents; callers should display the returned error rather than
logging its underlying cause. No browser output is forwarded to application logs.

Ordinary unit tests use injected discovery/launch probes. An opt-in smoke test
launches an installed browser against synthetic local fixtures, verifies profile
isolation and closure, checks the actual sandbox/headless flags, and verifies
that an untrusted HTTPS certificate is rejected:

```powershell
$env:ARCOURT_BROWSER_SMOKE = '1'
go test ./arcourt -run '^TestBrowserSmoke$' -v -count=1 -timeout=90s
if ($LASTEXITCODE -ne 0) { throw 'Browser smoke failed' }
# Optional test-only override, for example to exercise a second installed browser:
$env:ARCOURT_BROWSER_EXECUTABLE = 'C:\Program Files\Google\Chrome\Application\chrome.exe'
go test ./arcourt -run '^TestBrowserSmoke$' -v -count=1 -timeout=90s
if ($LASTEXITCODE -ne 0) { throw 'Browser smoke failed' }
Remove-Item Env:ARCOURT_BROWSER_SMOKE, Env:ARCOURT_BROWSER_EXECUTABLE
```

Those environment variables belong only to the test harness. Runtime callers
supply configuration through `BrowserFetcherConfig`. The smoke test uses no court
connection, credentials, or case documents. The separate live OPAD check is
recorded in verification. Expected local TLS handshake errors indicate certificate rejection.

Empty selection now means no downloads; `DocumentSelection{All: true}` is explicit.
Every distinct selected identity receives an outcome, including unavailable,
failed, canceled, and limit-skipped documents. `MaxDocuments` defaults to 200.
Discovery reads public OPAD case data for the verified template, or rendered
links for other templates: previews report observed totals and truncation
and explicitly state that whole-docket completeness is unknown. The core API's
`All` selection and CLI `--all` retain an incomplete-discovery error alongside
outcomes when coverage is unknown; clients must inspect the results. The desktop
separately reports selected-document success and the public-records coverage caveat.
Downloads revalidate case identity, stream bounded PDF bodies, validate content,
and retry transient failures without retrying cancellation. They preserve raw
request query order separately from canonical source identity.

Synthetic DOM parsing fixtures run separately from ordinary tests:

```powershell
$env:ARCOURT_DOM_FIXTURES = '1'
go test ./arcourt -run '^TestDocketDOMFixtures$' -v -count=1 -timeout=90s
if ($LASTEXITCODE -ne 0) { throw 'DOM fixtures failed' }
Remove-Item Env:ARCOURT_DOM_FIXTURES
```

These fixtures cover case metadata and identity, docket anchors/PDF buttons, dates,
parties, and pagination/virtualization signals without live court access. Production
HTTP policy is tested through injected DNS/dialing against local servers. No browser
cookies or authorization are forwarded. The application download service supplies
filesystem staging, safe filenames, and local manifests on top of this contract.

## Configuration, output, and privacy

| Setting | Desktop | CLI |
| --- | --- | --- |
| Case template | Settings; no built-in endpoint | `--case-url-template`, then `ARCOURT_CASE_URL_TEMPLATE` |
| Browser path | Settings override, then discovery | `--browser`, then `ARCOURT_BROWSER`, then discovery |
| Output directory | Picker or Settings | `--output`, then `ARCOURT_OUTPUT` |

Desktop preferences persist in `%APPDATA%\ArcourtDownloader\settings.json`;
WebView2 shell data in `%LOCALAPPDATA%\ArcourtDownloader\WebView2`. Preferences
contain the public template and local paths, not secrets. These folders persist
until removed while the app is closed. Automation profiles are separate and
removed after orderly operations; forced termination may leave temporary data.

Each case gets a sanitized subdirectory and bounded PDF names. The local
`arcourt-manifest.json` records case identity, labels, hashed document identity,
relative filenames, sizes, SHA-256, timestamps, and controlled outcomes/errors.
Request URLs, signed links, and credentials are excluded. Labels and PDFs can
still be sensitive: choose storage and backups appropriately. There is no
telemetry, upload, OCR, AI processing, cloud service, or background scheduler.
Browsing necessarily connects to the configured site and allowed document hosts.
Keep real documents outside the source checkout.

Repeats verify recorded files before skipping transfers. Existing unrelated or
modified files are preserved. Every selected document has an outcome; failed or
unavailable items do not erase successes. A manifest failure after publication can
report failure with `Saved: true`: the PDF remains usable and a later run can
reconcile ownership receipts. Do not remove recovery metadata during an operation.
Use local NTFS on Windows: hard links/locks are required; FAT/exFAT, network shares,
and symlink/junction destinations are unsupported. Arbitrary power-loss durability
is not guaranteed. Details: [filesystem contract](docs/filesystem-download-service.md).

## Troubleshooting and known limitations

| Symptom | Next step |
| --- | --- |
| GUI will not start | Install/repair WebView2; Edge/Chrome alone is insufficient. |
| Browser setup/launch error | Install Edge/Chrome or correct the override; inspect the reported path and timeout. |
| Wrong/empty preview | Verify template and case against the authorized site; do not download a mismatched header. |
| Output failure | Choose an existing writable local NTFS folder; avoid junctions, shares, and files held open elsewhere. |
| Partial download result | Inspect each outcome and retry eligible items; for Select All, also check truncation, pagination, and virtualized-row warnings. |
| Public-records coverage note after success | All selected documents completed; the court may have records unavailable online, including sealed or withheld records. This note does not indicate failed downloads. |
| Authentication, CAPTCHA, or denied document | Use the site's authorized workflow; the app does not bypass controls or import credentials. |
| Cleanup failure | Read the diagnostic and allow the owned process to exit; saved PDFs remain available. |

For custom templates, pagination, lazy loading, and virtualized rows are not
traversed. OPAD discovery includes every PDF attachment returned by its public
case response, including case-level documents. Cookies/auth headers are not
forwarded to PDFs. Content checks are not a general PDF reader or malware scanner.
Past live verification does not establish current access to other cases or
documents; see the [date/scope statement](docs/verification.md). Linux/macOS
core portability does not imply desktop support on those systems.

## License and project help

Project contributions are licensed under [0BSD](LICENSE), using the
[official Zero-Clause BSD text](https://opensource.org/license/0bsd).
Third-party components retain their own copyright and license terms; this license
does not claim ownership of them. Dependency-backed distribution notices remain
part of the [release preparation](docs/releasing.md).

See [software support](SUPPORT.md), [private vulnerability reporting](SECURITY.md),
and [contribution guidance](CONTRIBUTING.md). Public issues must exclude court PDFs,
client documents, credentials, signed URLs, browser profiles, and unredacted client
screenshots. Use synthetic reproduction steps and environment/version details.
