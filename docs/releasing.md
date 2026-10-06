# Release preparation

Public Windows release executables are **intentionally unsigned**. Microsoft
Defender SmartScreen and other Windows protections may warn about them. Download
only from the [official GitHub Releases page](https://github.com/tlfjar/ArcourtDownloader/releases).
The release supplies SHA-256 checksums and GitHub artifact attestations for source
and build provenance. They help establish which source and workflow produced the
assets; they do not establish an Authenticode publisher identity. Signing may be
added in the future, but it is not a release prerequisite.

The portable Windows x64 ZIP contains the production desktop application and CLI.
There is no installer or updater. No published release is established by this
source alone; inspect the Releases page for current availability.

## Local development checks

From the repository root on Windows, with the [testing prerequisites](testing.md):

```powershell
.\scripts\check.ps1
.\scripts\check-fixtures.ps1
.\scripts\build-desktop.ps1
.\scripts\build-cli.ps1
.\scripts\package-windows.ps1
```

Packaging requires PowerShell 7.4+, Go 1.26.8 and Node 24+. It selects the
reviewed Go toolchain from `scripts/licenses/catalog.json` and restores the
caller's `GOTOOLCHAIN` setting. The default command builds both executables and
creates a unique ignored `build/packages/` directory containing a development
ZIP, external SPDX SBOM and `SHA256SUMS.txt`. It verifies the archive. No upload
occurs. Repeat verification against unchanged source with:

```powershell
.\scripts\verify-release-assets.ps1 -AssetsPath <printed-directory> -Tag development -Local
```

`-Local` accepts only development identity. Tagged release verification requires
the explicit `-UnsignedRelease` switch and a clean checkout of the matching tag.
Both modes require each executable's Authenticode status to be exactly
`NotSigned`, with no signer, timestamp certificate, or PE certificate table. Invalid, ambiguous, or
unexpectedly signed states fail.

## Version and source identity

CLI `--version`, the desktop About display, and `BUILD.json` carry the full
[SemVer 2.0.0](https://semver.org/) version and source commit. Release builds
must use a clean HEAD at the exact existing tag. `resolve-build-info.ps1` checks
tag syntax, the peeled tag commit, HEAD, and tracked and relevant untracked
source. Fixture executables cannot be built in release mode. A local release
build can be made with:

```powershell
.\scripts\build-cli.ps1 -Release -Tag v0.1.0-beta.1
.\scripts\build-desktop.ps1 -Release -Tag v0.1.0-beta.1
```

`-Version` is an optional matching assertion for these build commands. Windows
fixed file/product versions use four numeric fields: major, minor and patch
must fit `0..65535`; prereleases map to `major.minor.patch.0` and stable
releases to `major.minor.patch.65535`. Full SemVer and commit remain in About,
`--version`, and the Windows resource Comments field.

For manual packaging from a clean checkout at that tag, supply the **same**
production executable bytes built from it:

```powershell
.\scripts\package-windows.ps1 -UnsignedRelease -Tag v0.1.0-beta.1 `
  -BinariesPath C:\release-build-inputs `
  -OutputPath .\build\releases\v0.1.0-beta.1
.\scripts\verify-release-assets.ps1 `
  -AssetsPath .\build\releases\v0.1.0-beta.1 `
  -Tag v0.1.0-beta.1 -UnsignedRelease
```

The input directory contains exactly `ArcourtDownloader.exe` and
`arcourt-download.exe`. The packager does not rebuild them. It checks their
unsigned status, embedded version/commit/role, production Go build settings and
dependencies. The output must be the checkout's absent or empty
`build/releases/<tag>/` without junctions or symlinks. Reused or concurrent
candidates are rejected. Preserve a failed candidate for diagnosis and use a
fresh checkout for a retry.

## Release payload and evidence

The public assets are exactly:

- `ArcourtDownloader-<tag>-windows-amd64.zip`
- `ArcourtDownloader-<tag>-SBOM.spdx.json`
- `SHA256SUMS.txt`, which hashes the ZIP and external SBOM

The ZIP contains the two executables, `LICENSE`, `THIRD_PARTY_NOTICES.txt`,
`README.md`, `SUPPORT.md`, `SECURITY.md`, `BUILD.json`, `SBOM.spdx.json`,
`FILE_SHA256SUMS.txt`, and the allowlisted `docs/command-line-workflow.md` and
`docs/releasing.md`. The internal checksum file hashes every other ZIP payload
file, including both final executables. Both checksum files use sorted paths,
lowercase SHA-256, two spaces, UTF-8 without BOM, and LF endings.

The verifier requires matching clean source and Windows tooling. It checks the
three public files and their hashes, rejects unexpected, duplicate, traversal,
link, oversized and fixture ZIP entries before extraction, and verifies the exact
payload, source documents, build identity, both `NotSigned` statuses and a
regenerated schema-validated SPDX SBOM. It never runs a downloaded executable.
Notice generation, module checksums, reviewed license texts and embedded assets
are checked. Checksums alone do not establish authenticity or source provenance.

## Source verification and GitHub setup

`check.yml` runs on pushes and pull requests and is reused by `release.yml` at
the exact tag event commit. Its stable source checks are:

| Job | Check name |
| --- | --- |
| `unit` | `Browserless checks (ubuntu-24.04)` and `Browserless checks (windows-2025)` |
| `browser-fixtures` | `Local browser DOM and CLI fixtures (Windows)` |
| `desktop` | `Windows desktop build and local package` |
| `govulncheck` | `Go vulnerability scan (root)` and `Go vulnerability scan (desktop)` |
| `workflow-policy` | `Release workflow policy and rehearsals` |
| `dependency-review` | `Dependency review (public PR)` |

The Go vulnerability scans inspect Windows packages. Public PR dependency
review and CodeQL default setup need observed hosted results. Enable the
dependency graph and configure protected `main`, required observed checks,
immutable releases, and `v*` rules that prevent updating or deleting tags.
The release preflight reads GitHub's protected-main state, checks the exact
tag commit against fetched main history, and fails on stale history or API
errors. Main ancestry alone is insufficient: the tag workflow reruns all source
verification jobs at the event commit. Hosted attestations require GitHub
support; if unavailable, no draft is created.

## Tagged artifact flow

All jobs check out `github.sha` with full history and without persisted checkout
credentials. The `build` job builds production GUI and CLI from the tagged
source, checks identity and `NotSigned` status, and uploads exactly those two
executables plus `build-manifest.json`. The manifest records repository, tag,
commit, run ID, producing attempt, and both executable SHA-256 hashes.

The Windows `package` job requires successful preflight, source verification and
build gates. It repeats preflight, downloads the build artifact by the producing
job's ID and digest, verifies GitHub metadata for run, source, name and digest,
hashes the artifact archive, and enforces its exact file set. It checks the
manifest's run attempt and executable hashes. It packages those exact executable
bytes, regenerates notices and SPDX evidence, verifies the final assets, and
compares both final ZIP executable hashes with the build manifest. There is no
rebuild between build and packaging.

| Workflow artifact | Exact files |
| --- | --- |
| `unsigned-<run-id>-<build-attempt>` | Both EXEs and `build-manifest.json` |
| `candidate-<run-id>-<package-attempt>` | Versioned ZIP, versioned SPDX JSON, `SHA256SUMS.txt` |

Artifacts have 14-day retention and are never overwritten. Failed-job reruns
retain producing attempt IDs; full reruns create new artifacts. Consumers use
artifact ID and digest, never a newest-by-name lookup. Only `attest` receives
`id-token: write` and `attestations: write`; only `draft` receives
`contents: write`. Jobs downloading artifacts receive `actions: read`.

The attestation job creates GitHub build-provenance attestations for the final
ZIP and external SPDX SBOM, plus an SPDX SBOM attestation for the ZIP. The draft
job downloads the exact candidate and verifies repository, source SHA, tag ref,
workflow and attestation type before creating a **draft**. It checks the remote
tag again and reads back remote asset bytes. Attestation bundles remain in
GitHub's attestation store; the public asset set remains three files.

## Create and inspect a draft

Only after the release change is merged to protected `main`, GitHub settings are
in place, and the exact source checks are green, an operator may create a **new**
tag. This task does not create one. On the public repository, with an
authenticated current GitHub CLI:

```powershell
$ErrorActionPreference = 'Stop'
function Checked { param([string]$Tool, [string[]]$Arguments)
    & $Tool @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$Tool failed" }
}
$repo = 'tlfjar/ArcourtDownloader'
$tag = 'v0.1.0-beta.1' # Choose a reviewed NEW version; never reuse a tag.
Checked git @('switch','main')
Checked git @('pull','--ff-only')
if (git status --porcelain=v1 --untracked-files=all) { throw 'Dirty source' }
if ($LASTEXITCODE -ne 0) { throw 'Unable to inspect source' }
$sha = git rev-parse HEAD
if ($LASTEXITCODE -ne 0) { throw 'Unable to resolve source' }
Checked gh @('run','list','--repo',$repo,'--workflow','check.yml','--commit',$sha)
# Inspect the exact successful source run and required CodeQL results.
Checked git @('tag','-a',$tag,'-m',"Arcourt Downloader $tag",$sha)
Checked git @('push','origin',"refs/tags/$tag")
Checked gh @('run','list','--repo',$repo,'--workflow','release.yml','--commit',$sha)
$run = Read-Host 'Release run ID'
Checked gh @('run','watch',$run,'--repo',$repo,'--exit-status')
Checked gh @('release','view',$tag,'--repo',$repo,'--json','isDraft,isPrerelease,tagName,targetCommitish,assets,body,url')
```

Only a `v*` tag push starts `release.yml`; there is no manual dispatch, PR,
branch, or publication trigger. Prerelease status follows the SemVer prerelease
field. Concurrency serializes the same tag without cancellation. A published or
immutable release stops staging. An existing draft must have the same tag,
target commit, prerelease status, candidate checksum marker, asset names and
actual downloaded asset hashes. Identical complete drafts are left alone;
matching partial drafts receive only missing assets. There is no clobber,
delete, or publish fallback. Preserve the hidden `arcourt-candidate-v1` marker
when editing notes. A full rerun may make a different ZIP, so prefer rerunning
failed jobs while original artifacts exist. Otherwise choose a new version or
review removal of an unpublished failed draft outside this workflow. Do not
edit or publish a draft while staging is active.

## Fresh-download verification and publication

On Windows, use a new clean checkout of the same public tag. Reuse `$repo`, `$tag`,
`$sha`, and `Checked` above, or resolve them from the tag in a new session:

```powershell
Checked git @('fetch','origin','--tags')
Checked git @('switch','--detach',$tag)
$sha = git rev-parse "$tag^{commit}"
if ($LASTEXITCODE -ne 0) { throw 'Unable to peel tag' }
$assets = Join-Path $env:TEMP ("arcourt-$tag-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $assets | Out-Null
Checked gh @('release','download',$tag,'--repo',$repo,'--dir',$assets)
.\scripts\verify-release-assets.ps1 -AssetsPath $assets -Tag $tag -UnsignedRelease
foreach ($name in @("ArcourtDownloader-$tag-windows-amd64.zip","ArcourtDownloader-$tag-SBOM.spdx.json")) {
    Checked gh @('attestation','verify',(Join-Path $assets $name),'--repo',$repo,
        '--signer-workflow',"$repo/.github/workflows/release.yml",
        '--source-digest',$sha,'--source-ref',"refs/tags/$tag")
}
Checked gh @('attestation','verify',(Join-Path $assets "ArcourtDownloader-$tag-windows-amd64.zip"),
    '--repo',$repo,'--signer-workflow',"$repo/.github/workflows/release.yml",
    '--source-digest',$sha,'--source-ref',"refs/tags/$tag",'--predicate-type','https://spdx.dev/Document')
```

Inspect the three assets, hashes, ZIP contents, source and attestations. Complete
the human production GUI walkthrough and authorized live check using the
**downloaded unsigned** executables. Record version, commit, hashes, date and
scope without case information. Update draft notes with only actual evidence;
automated fixtures do not establish live compatibility. Confirm immutable
releases are enabled and staging has finished. Only then deliberately publish:

```powershell
Checked gh @('release','edit',$tag,'--repo',$repo,'--draft=false')
Checked gh @('release','verify',$tag,'--repo',$repo)
```

The second command checks published release immutability; it does not replace
executable, checksum or provenance checks. After publication, fix problems with
a reviewed commit and **new version/tag**. Do not move a tag, replace assets,
delete and recreate a release, or relabel old bytes as the correction.
