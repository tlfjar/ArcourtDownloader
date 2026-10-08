# Court Document Downloader for Arkansas

**ArcourtDownloader** is an unofficial Windows app for previewing and downloading
publicly available Arkansas court case documents. It is not affiliated with or
endorsed by the Arkansas Judiciary or the Administrative Office of the Courts.
It does not provide official notice or guarantee a complete court record.

The app saves the PDFs you select, plus a local download manifest, on your
computer. It has no account, application server, telemetry, or cloud storage.
Requests go directly to the public site you configure.

## Get started

1. Check the [GitHub Releases page](https://github.com/tlfjar/ArcourtDownloader/releases)
   for a portable ZIP. When available, extract it to a local folder and open
   `ArcourtDownloader.exe`. There is no installer or automatic updater. The ZIP
   also includes `arcourt-download.exe` for command-line use.
2. On **Windows 11 x64**, install the [Microsoft Edge WebView2 Runtime](https://developer.microsoft.com/en-us/microsoft-edge/webview2/)
   if it is missing. Install Microsoft Edge or Google Chrome for case-page access;
   the desktop interface and browser automation have separate requirements.
3. Have an existing, writable **local NTFS folder** ready for downloads. Network
   shares, FAT/exFAT volumes, and symlink or junction destinations are unsupported.

Public Windows executables are unsigned, so Windows SmartScreen may show a warning.
Releases provide SHA-256 checksums and GitHub artifact attestations for verifying
the download and its source. These do not identify an Authenticode publisher.
If you prefer to build from source, follow the [development setup](docs/development.md).

## Download documents in the desktop app

1. Open **Settings & diagnostics** and enter a public case-page URL template with
   the literal `{case_number}` placeholder. For the public OPAD case API, use
   `https://caseinfonew.arcourts.gov/opad/case/{case_number}`. The app has no
   built-in court URL. Leave **Browser override** empty to detect Edge or Chrome.
2. Enter the case number and select **Preview case**. Check the case number,
   title, and parties against the case you intended to access, then mark the
   header verified. The app rejects a mismatched case.
3. Select individual documents or **Select All**. Review any discovery warnings:
   Select All covers the current preview, which may not include every court
   record. Sealed, withheld, or otherwise unavailable records cannot be checked.
4. Choose your output folder and select **Download selected**. Review each
   saved, skipped, failed, unavailable, or canceled result. **Open output folder**
   opens the case directory. Select failed items again to retry them.

Downloads run one at a time. Cancel stops pending work and keeps completed PDFs.
Changing cases clears the previous selection. If discovery was truncated or the
site uses pagination or virtualized rows, a Select All result is marked partial.
The [desktop guide](docs/desktop-architecture.md#runtime-and-workflow) explains
settings and behavior in more detail.

## Use the command line

The included `arcourt-download.exe` can preview a case, then download specific
document IDs or every document in a fresh, bounded preview. It requires a case
URL template, and downloads require an existing output folder. See the
[CLI guide](docs/command-line-workflow.md) for copyable PowerShell commands,
options, JSON output, and exit codes. In particular, `--all` can return a partial
status when discovery cannot establish full coverage, even if every discovered
document was saved.

## Files and privacy

Each case gets a subfolder with PDFs and an `arcourt-manifest.json` file. Repeated
downloads verify recorded files before skipping them; existing unrelated or
changed files are preserved. Keep case documents in a suitable location and out
of the source checkout. Labels and PDFs may contain sensitive information.

Desktop preferences are stored in `%APPDATA%\ArcourtDownloader\settings.json`;
WebView2 data is stored in `%LOCALAPPDATA%\ArcourtDownloader\WebView2`. The app
uses a separate temporary browser profile and does not use your browser cookies
or accounts. Normal shutdown removes that temporary profile. The
[download service guide](docs/filesystem-download-service.md) explains file
naming, repeat downloads, manifests, and recovery.

## Troubleshooting

| Problem | What to check |
| --- | --- |
| Desktop app will not open | Install or repair WebView2. Edge or Chrome alone does not provide the desktop runtime. |
| Browser setup fails | Install Edge or Chrome, or correct the executable path in Settings. |
| Preview is empty or shows the wrong case | Check the URL template and case number. Do not download a mismatched case. |
| Output folder fails | Choose an existing, writable local NTFS folder. Avoid shares, junctions, and files open in another app. |
| Some documents did not download | Check the per-document result and discovery warnings, then retry eligible items. |
| A record requires authentication or shows CAPTCHA | Use the site's authorized workflow; this app does not bypass access controls or import credentials. |

Successful downloads do not prove that the court has no other records. For
custom URL templates, the app does not traverse pagination or virtualized rows.
The [verification record](docs/verification.md) describes the scope and date of
past live checks. For help with an issue, see [support](SUPPORT.md). Report
security concerns through [private vulnerability reporting](SECURITY.md).
Do not include court PDFs, client documents, credentials, signed URLs, browser
profiles, or unredacted client screenshots in public issues.

## Project documentation

- [Development setup and source builds](docs/development.md)
- [Testing](docs/testing.md) and [release preparation](docs/releasing.md)
- [Browser runtime](docs/browser-runtime.md), [document fetch contract](docs/document-fetch-contract.md),
  and [download service contract](docs/filesystem-download-service.md)
- [Desktop architecture](docs/desktop-architecture.md) and [VS Code setup](.vscode/README.md)
- [Contributing](CONTRIBUTING.md)

Project contributions are licensed under [0BSD](LICENSE). Third-party components
retain their own licenses; see the [third-party notices](THIRD_PARTY_NOTICES.txt).
