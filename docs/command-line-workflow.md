# Command-line workflow

Run `arcourt-download --version` to see the embedded full version, source commit,
and whether the binary is a development build. This command requires no browser
or court connection.

`arcourt-download.exe` is the standalone CLI for the shared `arcourt.DownloadService`.
Runtime use requires the executable, Microsoft Edge or Google Chrome, and a writable
local output directory. It requires **no Go installation, GitHub authentication,
or repository checkout**. The [Windows desktop shell](desktop-architecture.md)
uses the same service. Commands are in [testing](testing.md) and [releasing](releasing.md).
The CLI keeps its existing deterministic filenames and makes no AI provider calls.
Optional AI naming is a desktop setting and an opt-in request in the reusable Go
core; see [AI document naming](ai-document-naming.md).

## Preview, select, download in Windows PowerShell

The court URL template has no default. The example below uses the configured OPAD
template. This exact template reads public case JSON directly; custom templates
use browser discovery. Keep the literal
`{case_number}` placeholder in the template.

Use the PowerShell call operator `&` for executable paths containing spaces. Pass
paths as single arguments; do not put literal quote characters inside their values.
Replace the executable path and case number with your installation and case:

```powershell
$cli = 'C:\Tools\Arcourt Downloader\arcourt-download.exe'
$caseNumber = Read-Host 'Enter the explicitly authorized public case number'
$template = 'https://caseinfonew.arcourts.gov/opad/case/{case_number}'
$output = Join-Path $env:USERPROFILE 'Documents\Court Downloads'
New-Item -ItemType Directory -Force -Path $output | Out-Null

# Preview JSON is on stdout; warnings and progress remain visible on stderr.
$previewText = & $cli preview --case $caseNumber --case-url-template $template --json
if ($LASTEXITCODE -ne 0) { throw 'Preview failed; inspect the diagnostic above.' }
$preview = ($previewText -join [Environment]::NewLine) | ConvertFrom-Json
$preview.preview.case
$preview.warnings
$preview.preview.documents | Format-Table document_id, filing_date, description -Wrap

# Choose a full document_id from the preview, not a URL or a displayed row number.
$documentId = Read-Host 'Paste the full document_id to download'
& $cli download --case $caseNumber --case-url-template $template `
    --output $output --id $documentId
$downloadExit = $LASTEXITCODE
Write-Host "Download exit code: $downloadExit"
```

Repeat `--id` for several documents. Duplicate IDs are collapsed. To download every
document in a fresh, bounded preview, use **explicit** `--all`:

```powershell
# These are alternative download commands.
$anotherId = Read-Host 'Paste another full document_id'
& $cli download --case $caseNumber --case-url-template $template `
    --output $output --id $documentId --id $anotherId
Write-Host "Download exit code: $LASTEXITCODE"

& $cli download --case $caseNumber --case-url-template $template `
    --output $output --all --max-documents 200 --timeout 10m
Write-Host "Download exit code: $LASTEXITCODE"

# Browser override with spaces; headful mode shows the isolated browser window.
& $cli preview --case $caseNumber --case-url-template $template `
    --browser 'C:\Program Files\Google\Chrome\Application\chrome.exe' --headless=false
if ($LASTEXITCODE -ne 0) { throw 'Preview failed' }

& $cli --help
if ($LASTEXITCODE -ne 0) { throw 'Help failed' }
& $cli download --help
if ($LASTEXITCODE -ne 0) { throw 'Help failed' }
```

Use `--headless=false`, not `--headless false`: boolean flag values use `=`.
All options follow `preview` or `download`. Paths containing spaces are supported,
including relative output paths resolved against the current working directory.
The output directory must already exist. The service rejects unsafe destinations,
symlinks/reparse paths, and network shares; Windows NTFS is the verified filesystem.

`download` always calls `Preview` again, validates the case identity, and resolves
IDs against that fresh preview before calling the service's `Download`.
Unknown or disappeared IDs fail the whole selection before any output is written.
The browser fetcher rechecks identity and selection once more before network
transfers; a document disappearing at that stage is an unavailable outcome.
Previously verified files can be skipped by the service after the CLI's fresh
preview. Selection never trusts a saved preview's URLs, and users never paste
expiring signed URLs. IDs are the full SHA-256 of the service's canonical document
identity, matching the manifest. They remain stable across known S3 signature
rotation; unfamiliar changing URL identity fields can change an ID and require
another preview. See the [identity contract](document-fetch-contract.md).

The current renderer does not traverse pages or virtualized rows. Preview warns
when discovery is incomplete or capped. `--all` means all entries in the **fresh
bounded preview**, including entries added since an earlier preview; it does not
mean a proven complete docket. Such runs return exit 1 even if all discovered
documents succeed. Explicit IDs may return exit 0 with a coverage warning. If an
ID falls outside a smaller refreshed preview cap, increase `--max-documents` and
preview again. Empty selections, including an empty `--all`, return exit 2.

## Configuration and limits

Precedence is **explicit flag > corresponding environment variable > default**.
An explicitly empty flag overrides the environment (empty `--browser=` restores
automatic discovery; empty case template or output fails validation). Repeated
scalar flags use the last value; repeat `--id` to accumulate IDs. No configuration
file is read. Case numbers, selections, modes, and limits are flags only.

| Option | Environment fallback | Default / allowed range |
| --- | --- | --- |
| `--case` | None | Required; supported case-number syntax |
| `--case-url-template` | `ARCOURT_CASE_URL_TEMPLATE` | Required absolute HTTP(S) template without URL user credentials; `{case_number}` in path, query, or fragment |
| `--output` | `ARCOURT_OUTPUT` | Required existing directory for download; environment ignored by preview |
| `--browser` | `ARCOURT_BROWSER` | Auto-discover installed Edge/Chrome |
| `--headless` | None | `true`; `false` is headful |
| `--timeout` | None | `10m`; greater than zero, at most `24h` for the command |
| `--page-timeout` | None | `90s`; greater than zero, at most `5m` per browser load |
| `--document-timeout` | None | `45s`; greater than zero, at most `5m` per HTTP attempt |
| `--max-documents` | None | `200`; 1–10000 preview entries/selected documents |
| `--max-pdf-bytes` | None | `67108864` (64 MiB); 1–1073741824 (1 GiB) |
| `--max-attempts` | None | `3`; 1–5 attempts per document |
| `--id`, `--all`, `--json` | None | No selection; text output |

Out-of-range values fail instead of disabling limits. The shared document client's
other bounded defaults remain in effect: JSON body, connect/read deadlines,
redirects, indirections, and retry delay. See the [fetch limits](document-fetch-contract.md).
`ARCOURT_BROWSER_EXECUTABLE` configures only the test harness, not this CLI.

For example, environment configuration can reduce repetition:

```powershell
$env:ARCOURT_CASE_URL_TEMPLATE = $template
$env:ARCOURT_OUTPUT = $output
& $cli preview --case $caseNumber
& $cli download --case $caseNumber --id $documentId --json
Remove-Item Env:ARCOURT_CASE_URL_TEMPLATE, Env:ARCOURT_OUTPUT
```

## Results, cancellation, and JSON

| Exit code | Meaning |
| --- | --- |
| `0` | Full success for the requested preview/selection, including verified-existing skips; help also succeeds |
| `1` | Partial result or operational failure; includes timeouts, unavailable/failed/limit-skipped documents, incomplete `--all`, browser setup/cleanup, and output/manifest errors |
| `2` | Invalid invocation or selection: unknown flags, bounds, case syntax, missing output/template/selection, mixed `--all`/`--id`, unknown IDs, or an empty refreshed selection |
| `130` | Context cancellation, including Ctrl+C; inspect results for already saved PDFs |

Ctrl+C cancels the shared context. The CLI drains progress and waits for fetcher
closure before exiting, preserving completed PDFs and service recovery semantics.
A timeout is an operational failure (1), not a user cancellation. Forced process
termination cannot perform cleanup, and kernel disk writes cannot be forcibly
interrupted by Go. The [service guide](filesystem-download-service.md) describes
repeat verification, conflict handling, manifests, and interrupted-run recovery.

With `--json`, stdout contains exactly one JSON object, including invocation and
operational failures and help. It has `command`, `status`, `exit_code`, optional
`error`, `warnings`, `preview`, `download`, and `help`. Status is `success`,
`partial_or_failure`, `invalid_invocation`, or `canceled`. If stdout itself cannot
be written, the diagnostic goes to stderr and an otherwise successful run exits 1.

- `preview.case` contains `case_number`, `title`, `county`, `judge`, and `parties`.
- `preview.documents` contains `document_id`, `filing_date`, and `description`.
- `preview.discovery` reports `discoverable_documents`, nullable `total_documents`,
  `truncated`, `complete`, pagination/virtualization signals, nullable
  `reported_docket_rows`, and `limitations`.
- `download` contains `case_number`, `directory`, `manifest_path`, `documents`,
  `counts` (selected/succeeded/failed/unavailable/skipped/canceled), and the service's
  `partial` flag. Document records follow the [manifest schema](filesystem-download-service.md).

The returned download result is authoritative; progress is best-effort stderr
output. `partial` describes the service's saved-content/incomplete-work result;
`status`, `exit_code`, and `warnings` additionally reflect CLI coverage and cleanup
errors. Thus incomplete `--all` can have `partial: false` while returning exit 1.
The CLI omits source/request URLs and underlying transport errors. Case metadata
and local paths remain in results. Do not merge stderr into a JSON pipeline:

```powershell
$resultText = & $cli download --case $caseNumber --case-url-template $template `
    --output $output --id $documentId --json 2> (Join-Path $output 'progress.log')
$resultExit = $LASTEXITCODE
$result = ($resultText -join [Environment]::NewLine) | ConvertFrom-Json
$result.download.counts
$result.download.documents | Format-Table document_id, outcome, filename, error -Wrap
Write-Host "Exit code: $resultExit"

# For a UTF-8 JSON artifact even in Windows PowerShell 5.1:
$resultText | Set-Content -LiteralPath (Join-Path $output 'result.json') -Encoding UTF8
```

Downloads belong outside the checkout or under ignored `downloads/`. Git also
ignores service-generated `case-<number>/` directories at custom in-repository
locations and `arcourt-manifest.json` anywhere. Arbitrarily named preview/result
exports are ignored only when saved under an ignored output directory.

## Build and local fixture verification (developers)

From the repository root, with Go 1.26 or newer on PATH:

```powershell
.\scripts\check.ps1 -CoreOnly
& '.\build\bin\arcourt-download.exe' --help
if ($LASTEXITCODE -ne 0) { throw 'CLI startup failed' }
```

This command builds a Windows executable when run on Windows. Distribute that
executable; Go is a build dependency only. The root module pins the CLI dependencies.

Normal unit tests use a fake service and need neither a browser nor court access.
The opt-in workflow below launches an installed browser against synthetic local
pages, routes the real document client's validated connections to a local fixture
server through test-only hooks, and uses the unchanged `DownloadService` to save
PDFs/manifests. It exercises preview, explicit selection, exact bytes/hashes,
verified repeat skipping, and incomplete `--all` under a path containing spaces.
Temporary test directories are cleaned by the test harness. There is no production
localhost exception or CLI option to weaken document destination checks.

```powershell
$env:ARCOURT_CLI_FIXTURES = '1'
try {
    go test ./cmd/arcourt-download -run '^TestCLIBrowserFixtures$' -v -count=1 -timeout=120s
    if ($LASTEXITCODE -ne 0) { throw 'CLI browser fixtures failed' }
} finally {
    Remove-Item Env:ARCOURT_CLI_FIXTURES
}
# Optional before that test: select another installed browser for the harness.
# $env:ARCOURT_BROWSER_EXECUTABLE = 'C:\Program Files\Google\Chrome\Application\chrome.exe'
# Remove-Item Env:ARCOURT_BROWSER_EXECUTABLE
```

A live court request is a separate opt-in check: supply an officially verified
template and a case you intend to access, then run the runtime examples. Automated
tests never require court connectivity, GitHub credentials, or personal documents.
See [testing](testing.md) for reproducible checks and separate manual gates.

For repository contributors only, if using GitHub CLI to configure authenticated
HTTPS Git access, run `gh auth login` followed by `gh auth setup-git` before the Git
operation. These configure repository access and are unrelated to application use.
