# Release preparation

There is no published release or signed candidate established by this source.
Current builds are unsigned development artifacts. The planned distribution is
a portable Windows x64 ZIP containing the production desktop application and
CLI; no installer or updater is provided. The planned download location is
[GitHub Releases](https://github.com/tlfjar/ArcourtDownloader/releases).

## Current local commands

From the repository root on Windows, with the [testing prerequisites](testing.md):

```powershell
.\scripts\check.ps1
.\scripts\check-fixtures.ps1
.\scripts\build-desktop.ps1
.\scripts\build-cli.ps1
.\scripts\package-windows.ps1
```

Packaging requires PowerShell 7.4+, Go 1.26.8 and Node 24+. The packaging command
selects the exact Go toolchain from `scripts/licenses/catalog.json` for builds
and evidence generation, then restores the caller's `GOTOOLCHAIN` setting.
Standalone asset verification selects the same toolchain. Go may download it
on first use; see [Go toolchain selection](https://go.dev/doc/toolchain#select).
The default command builds the production GUI and CLI and creates a unique
directory under ignored `build/packages/` with a development ZIP, SPDX JSON SBOM
and asset checksums.
It verifies the archive automatically. These unsigned packages are for local
review; keep private-source identities private until the public source cutover.
No upload occurs. To repeat verification before changing source:

```powershell
.\scripts\verify-release-assets.ps1 -AssetsPath <printed-directory> -Tag development -Local
```

`-Local` is explicitly restricted to development identity. It makes no signature
claim and cannot verify a tagged release. Local verification requires the same
source revision and working-tree identity used to package the files.

## Version and source identity

The CLI's `--version` and the desktop footer's About display use the same embedded
identity. The full [SemVer 2.0.0](https://semver.org/) string, including prerelease
and build metadata, is displayed with the full source commit. Ordinary builds say
`development, unsigned`. A source export built before its first public commit says
`source unborn` rather than carrying the old private commit. These labels make no
claim that a binary was signed; signature verification is a separate release gate.

For a local development build, run:

```powershell
.\scripts\build-cli.ps1
.\scripts\build-desktop.ps1
.\build\bin\arcourt-download.exe --version
```

After a public commit is intentionally tagged, build both binaries from the same
clean HEAD with the exact tag. For example:

```powershell
.\scripts\build-cli.ps1 -Release -Tag v0.1.0-beta.1
.\scripts\build-desktop.ps1 -Release -Tag v0.1.0-beta.1
```

`-Version 0.1.0-beta.1` is an optional assertion with either release build command;
it must match the tag exactly. The shared `scripts/resolve-build-info.ps1` checks
the full SemVer syntax, tag existence, peeled tag commit, HEAD, and clean tracked
and relevant untracked source. The tag alone determines the release version. A
missing tag, mismatch, dirty source or fixture build in release mode stops the
build. Untagged CI commits continue to use development identity.

Windows fixed file/product version and assembly manifest use four numeric fields.
Major, minor and patch must each fit `0..65535`. Prereleases map to
`major.minor.patch.0`; stable releases map to `major.minor.patch.65535`. Distinct
prerelease labels for the same base version share a Windows numeric version. The
complete SemVer string and source SHA remain in About, `--version`, and the
Windows resource Comments field. Build metadata after `+` is displayed verbatim
and does not alter the Windows numeric version. The public archive
name uses the validated full tag: `ArcourtDownloader-<tag>-windows-amd64.zip`.

## Work remaining before a release

- **Hosted rehearsal:** the workflows and local rejection tests are implemented;
  real provider signing, hosted attestations, and public PR dependency review
  still need hosted evidence. A private skipped control is not a passed control.
- **Signing:** enroll an actual provider, configure its approved identity/policy,
  sign both production executables, and verify signatures. No provider, account,
  certificate or secret is assumed; an unsigned fallback is not a signed release.
- **Candidate and publication review:** verify exact source, assets, checksums,
  dependency evidence and provenance; complete the human production GUI/live
  check described in [testing](testing.md#manual-live-boundary); review notes and
  deliberately publish. Passing automated source checks does not satisfy these gates.

Do not publish fixture executables, settings, browser profiles,
downloads, or local diagnostic records. Published notes should include only
verified dates/scope, without real case details or a claim of complete records.

## Signed packaging contract

The release workflow hands off the exact production GUI and CLI built from the
clean tagged commit to the signing provider, then validates the returned bytes'
association with those build inputs. Commit approved `scripts/signing-policy.json`
configuration before tagging: provider name, exact certificate subject and allowed SHA-256
certificate fingerprints (lowercase hex). These are public policy, not secrets.
The current empty policy intentionally prevents release packaging. Certificate
rotation requires a reviewed policy update before tagging. Valid Windows trust,
a timestamp certificate, and the approved signer identity are all required.

From a clean checkout at the exact tag, after signing both executables:

```powershell
.\scripts\package-windows.ps1 -Release -Tag v0.1.0-beta.1 `
  -SignedBinariesPath C:\signed-inputs `
  -OutputPath .\build\releases\v0.1.0-beta.1
.\scripts\verify-release-assets.ps1 `
  -AssetsPath .\build\releases\v0.1.0-beta.1 -Tag v0.1.0-beta.1
```

The input directory must contain exactly `ArcourtDownloader.exe` and
`arcourt-download.exe`. Release packaging never rebuilds those executables.
It checks Authenticode, the stamped version/commit/role, Go build settings and
resolved dependencies before archiving. The output path must be the checkout's
ignored `build/releases/<tag>/`, absent or empty, without junctions/symlinks.
Reused, partial and concurrent output candidates are rejected; preserve failed
outputs for diagnosis and choose a fresh checkout for a retry.

The three public assets are:

- `ArcourtDownloader-<tag>-windows-amd64.zip`
- `ArcourtDownloader-<tag>-SBOM.spdx.json`
- `SHA256SUMS.txt` (hashes only the final ZIP and external SBOM)

The ZIP has a flat root containing the two production executables, `LICENSE`,
`THIRD_PARTY_NOTICES.txt`, `README.md`, `SUPPORT.md`, `SECURITY.md`, `BUILD.json`,
`SBOM.spdx.json`, `FILE_SHA256SUMS.txt`, and two allowlisted documents under
`docs/`: `command-line-workflow.md` and `releasing.md`. The internal checksum
file hashes every payload file except itself, including the final signed
executable bytes. Both checksum lists use sorted, forward-slash relative paths,
lowercase SHA-256, two spaces, UTF-8 without BOM, and LF line endings.

The verifier uses the matching clean source/tag, Go/Node tools and module cache
(or access to download modules), and Windows certificate trust. It checks the
three-file asset allowlist and hashes, validates all archive entries before
extracting into a new temporary directory, then checks internal hashes, exact
payload, source documents, build identity, signatures and a regenerated,
schema-validated SBOM. It never runs a downloaded executable. Extra files,
traversal, links, duplicate names, oversized entries and fixture payloads fail.
Run it against freshly downloaded assets before publication. The workflow
verifies hosted provenance; publication also requires production/live manual checks.

Notice generation and SPDX evidence are repeatable for identical inputs using
the generator pinned in the tagged source. Module checksums, reviewed license
texts and embedded assets are validated. This does not promise byte-identical
ZIP timestamps or Authenticode output. Hashes alone are not authenticity or
source provenance, and declared build identity is not a build attestation.

## Source verification and GitHub setup

`check.yml` runs with `contents: read` on pushes, pull requests and manual runs.
It is also the reusable verification gate called by `release.yml` at the exact
tag event commit. The stable source check names are:

| Job | Check name |
| --- | --- |
| `unit` | `Browserless checks (ubuntu-24.04)` and `Browserless checks (windows-2025)` |
| `browser-fixtures` | `Local browser DOM and CLI fixtures (Windows)` |
| `desktop` | `Windows desktop build and local package` |
| `govulncheck` | `Go vulnerability scan (root)` and `Go vulnerability scan (desktop)` |
| `workflow-policy` | `Release workflow policy and rehearsals` |
| `dependency-review` | `Dependency review (public PR)` |

Both Go scans pin `golang.org/x/vuln/cmd/govulncheck@v1.1.4` and inspect Windows
packages; the desktop scan includes production Wails build tags. Vulnerability
database results are current at scan time, not frozen by the tool pin. A reported
reachable vulnerability or scan failure fails CI. Dependency review fails for
new vulnerabilities at any severity on public PRs. Enable the dependency graph
before the public bootstrap PR. Private runs deliberately skip dependency review;
record it as pending, and require the context only after observing a successful
public PR. See [GitHub dependency review availability](https://docs.github.com/en/code-security/concepts/supply-chain-security/dependency-review).

Configure CodeQL **default setup**, then verify its actual results and
required context. These workflows intentionally contain no advanced CodeQL setup.
Also configure protected `main`, required observed source checks,
immutable releases, and `v*` rules that prohibit updating/deleting existing tags.
The release preflight reads GitHub's protected-branch state and checks the peeled
tag against fetched main history. Missing permissions, unavailable protection,
stale history or API errors stop it. Being an ancestor of main does not prove CI
passed: every candidate re-runs all reusable verification jobs at its source SHA.

No PR job receives signing secrets, an environment credential, OIDC permission or
a write token. The signing environment is referenced only by the tag workflow.
GitHub provides public artifact attestations; availability for private rehearsals
depends on the repository plan. If unavailable, the attestation job fails and no
draft is created. Do not bypass it or report it as proven. See
[GitHub attestation requirements](https://docs.github.com/en/actions/how-tos/security-for-github-actions/using-artifact-attestations/using-artifact-attestations-to-establish-provenance-for-builds).

## Provider integration seam

The empty signing policy and `scripts/invoke-signing-provider.ps1` deliberately
fail closed. No provider account or credential is configured. Integrate the
selected provider with these reviewed inputs before tagging:

- One enrolled provider, its organization/project/policy identifiers, public
  signer subject and SHA-256 certificate fingerprints, and timestamp service.
- A protected GitHub `release-signing` environment or the provider's real approval
  mechanism. Restrict it to this repository's `v*` tag workflow and approved main
  source. Provision credentials through GitHub/provider settings, never source.
- The actual secret names and/or OIDC audience/subject restrictions. Add only the
  selected provider's necessary permissions to `sign`; OIDC is currently granted
  only to `attest`. No credentials are inherited by the reusable source workflow.
- A full-commit-pinned provider action/tool and one concrete implementation of
  `invoke-signing-provider.ps1`, with renewal/rotation ownership and a hosted test.

The provider script accepts `-RequestPath`, `-UnsignedPath` and `-OutputPath`.
The output directory must be new. Inputs contain exactly the two production EXEs
and `build-manifest.json`. That manifest records schema version 1, repository,
tag, full commit, build run ID/attempt, and each unsigned executable's SHA-256.
`signing-request.json` wraps it with the immutable unsigned artifact ID and archive
digest. The provider must sign those inputs and preserve them for comparison.

Return exactly the two signed executables and `provider-receipt.json`:

```json
{
  "schemaVersion": 1,
  "provider": "same reviewed name as signing-policy.json",
  "requestSha256": "lowercase SHA-256 of the exact signing-request.json bytes",
  "requestId": "non-sensitive-provider-request-id"
}
```

The adapter must bind that request ID to the provider's actual accepted job and
returned output. The receipt is diagnostic association evidence, not a substitute
for signature trust or provenance. Do not include credentials, private policy
internals or case data. The wrapper checks timestamped Authenticode trust and
the approved subject/fingerprint. The repository's `signing-tool` then compares
the complete unsigned PE bytes to the signed file: only the PE checksum, security
directory, zero alignment padding and one appended WIN_CERTIFICATE may differ.
Any executable code, resources, embedded build identity or other bytes changed by
the provider cause rejection. A provider requiring other changes needs a reviewed
contract change before integration. The packager independently checks production
role/build tags, exact source/version, trust and dependency evidence.

## Artifact and privilege contracts

All release jobs check out `github.sha` with `fetch-depth: 0` and
`persist-credentials: false`. Source verification and production builds are
unprivileged. Signing occurs separately; packaging has no signing authority.
Only `attest` gets `id-token: write` and `attestations: write`. Only `draft` gets
`contents: write`. Jobs downloading artifacts also need `actions: read`; the draft
job uses `attestations: read` for provenance verification. Release builds disable
Go/Node caches to avoid carrying PR build caches into privileged work.

| Workflow artifact | Exact file set |
| --- | --- |
| `unsigned-<run-id>-<build-attempt>` | `ArcourtDownloader.exe`, `arcourt-download.exe`, `build-manifest.json` |
| `signed-<run-id>-<sign-attempt>` | both EXEs, `signing-evidence.json` |
| `candidate-<run-id>-<package-attempt>` | versioned ZIP, versioned SPDX JSON, `SHA256SUMS.txt` |

Artifacts have a 14-day retention period and are never overwritten. Each consumer
uses the producing job's artifact ID and SHA-256 output, verifies the GitHub API
run/source/name/digest metadata, hashes the downloaded archive, and enforces the
exact extracted file set. It never searches for the newest artifact by name.
Failed-job reruns retain the original producing attempt IDs. Full reruns produce
new artifacts. `signing-evidence.json` records the request/receipt, approved signer
policy and final executable hashes. Inspect it in the run's signed artifact.

The package job regenerates/checks dependency notices, generates/validates SPDX
from final signed bytes, and verifies the final three assets on a matching clean
checkout. The attestation job creates GitHub provenance for both the ZIP and SPDX
JSON and an SPDX SBOM attestation for the ZIP. The draft job downloads those exact
bytes and verifies the repository, source SHA, tag ref and signer workflow before
creating a draft. Attestation bundles remain in GitHub's attestation store and
the run summary, keeping the public three-file contract intact.

## Create and inspect a draft

Run these commands only in the intended **public** repository after repository settings,
provider enrollment/integration, and reviewed green source checks are in
place. Go 1.26.8, Node 24.12.0, PowerShell 7.4+ and an authenticated current GitHub
CLI are required. Native-command failures must stop the operator sequence.

```powershell
$ErrorActionPreference = 'Stop'
function Checked { param([string]$Tool, [string[]]$Arguments)
    & $Tool @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$Tool failed" }
}
$repo = 'tlfjar/ArcourtDownloader'
$tag = 'v0.1.0-beta.1' # Choose the reviewed NEW version; never reuse a tag.
Checked git @('switch','main')
Checked git @('pull','--ff-only')
if (git status --porcelain=v1 --untracked-files=all) { throw 'Dirty source' }
if ($LASTEXITCODE -ne 0) { throw 'Unable to inspect source' }
$sha = git rev-parse HEAD
if ($LASTEXITCODE -ne 0) { throw 'Unable to resolve source' }
Checked gh @('run','list','--repo',$repo,'--workflow','check.yml','--commit',$sha)
# Inspect the exact successful verification run and required CodeQL results.
Checked git @('tag','-a',$tag,'-m',"Arcourt Downloader $tag",$sha)
Checked git @('push','origin',"refs/tags/$tag")
Checked gh @('run','list','--repo',$repo,'--workflow','release.yml','--commit',$sha)
# Set $run to the displayed release run ID, then inspect all jobs:
$run = Read-Host 'Release run ID'
Checked gh @('run','watch',$run,'--repo',$repo,'--exit-status')
Checked gh @('release','view',$tag,'--repo',$repo,'--json','isDraft,isPrerelease,tagName,targetCommitish,assets,body,url')
```

Only a `v*` tag push starts `release.yml`; there is no manual-dispatch, PR, branch
or publication trigger. The workflow uses `gh release create --draft --verify-tag`
and prerelease status follows the SemVer prerelease field (build metadata alone
does not make a prerelease). The tag is peeled and checked again remotely during
draft staging. Concurrency serializes runs of the same tag without cancellation.

An existing published or immutable release always stops staging. For an existing
draft, its tag, exact target commit, prerelease status, candidate checksum marker,
every existing asset name and actual downloaded checksum must match. Matching
partial drafts can receive missing assets; identical complete drafts are left
alone. There is no clobber/delete/publish fallback. Preserve the hidden
`arcourt-candidate-v1` marker when editing notes. Full rebuilds can have different
ZIP timestamps/signatures and therefore fail this comparison even at the same
commit. Prefer **re-run failed jobs** while the original artifacts exist. Otherwise
use a new version, or explicitly review removal of an unpublished failed draft
outside this workflow. Never edit/publish a draft while its staging run is active.

## Fresh-download verification and publication

On Windows, use a new clean checkout of the same public tag. Reuse `$repo`, `$tag`,
`$sha` and `Checked` from the operator session above, resolving `$sha` from the
peeled local tag if starting a new session. Set an unused download directory:

```powershell
Checked git @('fetch','origin','--tags')
Checked git @('switch','--detach',$tag)
$sha = git rev-parse "$tag^{commit}"
if ($LASTEXITCODE -ne 0) { throw 'Unable to peel tag' }
$assets = Join-Path $env:TEMP ("arcourt-$tag-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $assets | Out-Null
Checked gh @('release','download',$tag,'--repo',$repo,'--dir',$assets)
.\scripts\verify-release-assets.ps1 -AssetsPath $assets -Tag $tag
foreach ($name in @("ArcourtDownloader-$tag-windows-amd64.zip","ArcourtDownloader-$tag-SBOM.spdx.json")) {
    Checked gh @('attestation','verify',(Join-Path $assets $name),'--repo',$repo,
        '--signer-workflow',"$repo/.github/workflows/release.yml",
        '--source-digest',$sha,'--source-ref',"refs/tags/$tag")
}
Checked gh @('attestation','verify',(Join-Path $assets "ArcourtDownloader-$tag-windows-amd64.zip"),
    '--repo',$repo,'--signer-workflow',"$repo/.github/workflows/release.yml",
    '--source-digest',$sha,'--source-ref',"refs/tags/$tag",'--predicate-type','https://spdx.dev/Document')
```

Review the signing evidence and final ZIP contents. Complete the human
production GUI walkthrough and authorized live check against the **downloaded
signed** executables. Record version, commit, hashes, date and scope without case
information. Update the notes with only actual evidence; automated success must
not replace the pending live/GUI statements. Confirm immutable releases are enabled
and the staging run is finished. Only then deliberately publish:

```powershell
Checked gh @('release','edit',$tag,'--repo',$repo,'--draft=false')
Checked gh @('release','verify',$tag,'--repo',$repo)
```

The second command verifies that the published release is immutable; it is not a
replacement for the executable, checksum and provenance checks. See the official
[release-create contract](https://cli.github.com/manual/gh_release_create) and
[attestation-verification flags](https://cli.github.com/manual/gh_attestation_verify).
After publication, fix problems with a reviewed commit and **new version/tag**.
Do not move the tag, replace assets, delete/recreate the release or re-label old
bytes as the correction. Publish a concise correction notice referencing the new
version and the affected old version.
