# Contributing

Contributions are accepted under the [Zero-Clause BSD (0BSD) license](LICENSE).
By submitting a contribution, you agree to provide it under those terms. Submit
only material you have the right to contribute, preserve existing copyright and
license notices, and identify any third-party code and its license separately.
The project license does not replace dependency licenses.

Use a branch and pull request with a focused explanation of the problem, resulting
behavior, and verification. For larger changes, discuss the intended behavior in
an issue first. Dependency updates receive ordinary review; there is no automatic
merge policy.

The root Go module contains `arcourt/` and the thin `cmd/arcourt-download/` CLI.
The separate `desktop/` module contains the Wails shell and controller, and
`desktop/frontend/` contains static UI assets and Node tests. Keep the desktop's
in-repository `replace ... => ..` directive and use `GOWORK=off`. Root Go checks
do not include the desktop module. Keep both Go lockfiles and the npm lockfile;
dependency/toolchain changes should be deliberate and explained.

Follow [testing instructions](docs/testing.md). Run `scripts/check.ps1`; changes
to discovery/browser/CLI behavior also need `scripts/check-fixtures.ps1`.
Desktop shell changes need the native build and relevant interactive checks.
Record exact commands, results, skips, and unavailable gates in the pull request.
Add regression coverage for changed behavior at its boundary. Keep fixtures
synthetic and tests independent of the court. Do not introduce live requests in
automated tests or CI.

Preserve explicit preview and selection, sequential bounded downloads, partial
results, local manifests, and isolated owned browser processes. Keep network/file
logic in the Go core and the desktop UI thin. The product has no server, telemetry,
cloud storage, OCR, AI processing, updater, or installer requirement.

Public issues and pull requests must exclude court PDFs, client documents,
credentials, signed URLs, browser profiles, and unredacted client screenshots.
Never commit real case captures or local output. Use synthetic reproductions and
sanitized environment/version details; avoid sensitive attachments. Report
vulnerabilities via [SECURITY.md](SECURITY.md), and see [SUPPORT.md](SUPPORT.md)
for software support boundaries.
