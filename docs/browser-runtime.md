# Browser runtime

This guide covers browser selection, configuration, and lifecycle for callers of
the shared `arcourt` package. For application setup, start with the
[README](../README.md).

## Fetcher and service APIs

Construct `arcourt.NewBrowserFetcher` with `BrowserFetcherConfig`, supplying
`CaseURLTemplate` containing `{case_number}` and setting `Headless` as desired.
The endpoint is caller-configured; the library does not read endpoint configuration
from the environment or supply a default court URL. `PreviewCaseDocuments` returns
validated case metadata, document identities, and discovery coverage.
`DownloadCaseDocuments` takes an explicit selection and a per-document staging
writer, and returns outcomes that preserve partial success. See the
[public fetch contract](document-fetch-contract.md) for API semantics, limits,
HTTP policy, and a programmatic example.

Application hosts should use `arcourt.NewDownloadService(fetcher)`, then `Preview`
and `Download` with explicit preview entries and an existing absolute output
directory. The service saves PDFs in a safe case subdirectory, verifies repeat-run
skips, preserves existing files, and maintains a versioned local manifest with
recovery after interrupted publication. Inspect both results and errors for partial
success. See the [filesystem service guide](filesystem-download-service.md)
for a minimal example, manifest schema, progress, cancellation, and output policy.

## Browser discovery and configuration

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

## Process isolation and cleanup

Each operation using browser discovery launches an application-owned process with
a fresh temporary profile. It does not attach to existing browsers or use their
cookies or profiles.
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

On Windows, an inherited `__COMPAT_LAYER` value such as `RunAsInvoker` can make
Edge immediately relaunch into a child process. The original process then exits
before chromedp receives its DevTools address. For Edge only, the runtime clears
that variable in the new browser process; the application, user, and machine
environments are unchanged. This was reproduced with an isolated headless Edge
profile and verified by the browser fixture. For an older build, clearing the
variable in the launching PowerShell process with
`Remove-Item Env:__COMPAT_LAYER -ErrorAction SilentlyContinue` is a temporary
workaround; it does not alter Windows environment settings.

## Browser smoke test

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
recorded in [verification](verification.md). Expected local TLS handshake errors
indicate certificate rejection.

## Discovery and selection semantics

Empty selection means no downloads; `DocumentSelection{All: true}` is explicit.
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

## DOM fixtures

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
