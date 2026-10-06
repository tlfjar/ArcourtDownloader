# Dependency evidence maintenance

`catalog.json` is the reviewed license inventory, pinned by the source commit.
It preserves upstream text, its normalized UTF-8 SHA-256, the source-relative
filename and the Go module content checksum. `THIRD_PARTY_NOTICES.txt` is its
deterministic rendering. The standard-library-only generator is
`scripts/release-tool` version `arcourt-release-tool-1`; there is no floating
generator download. Go **1.26.8** is required until its license inventory is
reviewed again. Packaging requires PowerShell **7.4+** and Node **24+**.

Run `./scripts/generate-notices.ps1` to render notices, or add `-Check` to check
the committed rendering. Packaging verifies license excerpts against resolved
upstream module files and verifies both module caches, not just the catalog's
stored hashes. Unknown modules, replacements, versions, checksums, embedded
upstream assets, toolchains or npm dependencies fail packaging. For dependency
upgrades, review upstream license/copyright/NOTICE files and copied code, amend
the catalog deliberately, regenerate notices, and exercise packaging again.
Do not fill unknown license fields with guessed identifiers or `NOASSERTION`.

The SPDX 2.3 JSON schema is vendored from
https://raw.githubusercontent.com/spdx/spdx-spec/v2.3/schemas/spdx-schema.json
with SHA-256
`239208b7ac287b3cf5d9a9af23f9d69863971102a5e1587a27a398b43490b89b`.
It is unmodified tooling, excluded from the release payload. Attribution: SPDX
Workgroup, SPDX Specification 2.3, https://spdx.github.io/spdx-spec/v2.3/,
licensed under https://creativecommons.org/licenses/by/3.0/.

## Reviewed Windows production inventory

Both `go.sum` files contain test/tool/other-platform dependencies that are not
shipped. The inventory uses `go list -deps` for each Windows amd64 entry point
and compares the complete module set and sums with `debug/buildinfo` from the
actual PE files. The GUI tags must be exactly
`desktop,wv2runtime.error,production`; CLI tags must be empty. The desktop's
local root-module replacement is checked and uses the same stamped source
commit, not the placeholder `v0.0.0`. `golang.org/x/sys` is v0.42.0 in the CLI
and v0.46.0 in the GUI; both versions remain in notices and SPDX.

- The application npm lockfile contains no third-party dependencies. The four
  local frontend files are 0BSD, checked against source and bytes embedded in
  the GUI. A clean `dist` prevents stale files from entering `go:embed all:`.
  Generated `wailsjs` bindings/wrappers are outside `dist` and are not shipped.
- Wails v2.16.0 embeds production JavaScript/IPC, a fallback HTML page, and
  go-toast embeds PowerShell/XML templates. These are recorded with their exact
  hashes. Wails runtime's package.json says ISC, but the repository's explicit
  MIT license grants permission for this source; the MIT grant and Lea Anthony
  copyright are retained. Its npm devDependencies (esbuild, Svelte, Vitest,
  happy-dom, npm-run-all) are not imported by the production desktop runtime.
- Wails includes copied winc and common-file-dialog MIT code and Apache-2.0
  typescriptify code, a copied MIT ringqueue and BSD-3-Clause atotto clipboard
  code. Winc also retains the original BSD-3-Clause w32/gform notices and
  distinct source copyright headers. Their full licenses are retained, including
  the upstream Apache copyright appendix. go-webview2 includes its loader's
  separate MIT notice and copied go-ole notice. Mimetype's copied Go JSON parser
  has a BSD-3-Clause notice, included alongside its MIT license.
- The default Go WebView2 loader is compiled; `native_webview2loader` is
  forbidden. `wv2runtime.error` does not retain the Microsoft bootstrapper.
  The generator intersects Go's embed inventory with bytes present in the
  final executable: the unreferenced installer and development runtime are
  absent. Microsoft Edge/Chrome and WebView2 Runtime are user prerequisites,
  not redistributed tools. No Wails/Go/Node/npm executable is packaged.
- Go's runtime/standard library license and PATENTS are retained, along with
  the copied amd64 Inferno memmove MIT notice, fiat-crypto's BSD-1-Clause notice,
  SunPro math notices, and the source's Cephes and public-domain Rijndael grants
  (the latter two use SPDX LicenseRef entries with their full extracted text).
  These are source-component notices; individual unused functions may be removed
  by the linker. Unicode
  15.0.0 data in Go, x/text and uniseg is covered by the Unicode notice fetched
  from https://www.unicode.org/license.txt on 2026-10-02 and pinned in the catalog.
  Go 1.26 selects x/text's 15.0.0 tables; the Go 1.27-only 17.0.0 files do not ship.
- `desktop/tools/icon` creates original artwork; Windows manifest/version
  resources are project assets. No external font/image/icon library is embedded.

SPDX includes both final executable SHA-256 values, resolved modules with purls,
embedded runtime/frontend file hashes, licenses and containment/dependency
relationships. `NOASSERTION` on composite executable license conclusions does
not replace dependency license information. Its fixed metadata epoch supports
repeatable output for identical inputs; it is not a signing/build time. Binary
identity is read without executing either deliverable. This identifies the
declared build; the pinned build-artifact handoff and provenance must establish
its association with the approved build, since a stamp is not an attestation.
