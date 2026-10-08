# Local PDF download service

`arcourt.DownloadService` is the framework-independent application API for the
CLI and desktop shell. It uses a `CaseFetcher`, allows one active preview
or download per service, and saves PDFs sequentially. There is no daemon, database,
queue, polling, byte-range resume, or frontend dependency.

## Preview, select, download

Construct the browser fetcher using the [fetch contract](document-fetch-contract.md)
and keep its existing shutdown responsibility. The endpoint remains configured
by the host application; no default court URL is invented.

```go
func saveSelected(ctx context.Context, fetcher arcourt.CaseFetcher,
    caseNumber, absoluteOutputDirectory string,
    choose func(*arcourt.CasePreview) []arcourt.DocketEntry,
) (*arcourt.LocalDownloadResult, error) {
    service, err := arcourt.NewDownloadService(fetcher)
    if err != nil {
        return nil, err
    }
    preview, err := service.Preview(ctx, caseNumber)
    if err != nil {
        return nil, err
    }
    // Present preview.Discovery with the document list: completeness is unknown.
    // choose returns only the entries the user explicitly selected.
    selected := choose(preview)
    return service.Download(ctx, arcourt.DownloadRequest{
        CaseNumber:      preview.CaseInfo.CaseNumber,
        Selection:       selected,
        OutputDirectory: absoluteOutputDirectory,
    }, nil) // optional chan<- arcourt.DownloadEvent
}
```

Call this from the host's worker goroutine to keep its interface responsive.
Cancel its context to stop a download. On application shutdown, cancel, wait for
`Download` to return, and close the browser fetcher before exiting.

`Selection` freezes the selected preview entries. **Select All displayed** copies
`preview.DocketEntries`; it never silently includes new or unrendered rows. This
differs deliberately from the lower-level fetcher's `DocumentSelection{All: true}`,
which reloads all discovered identities. The browser fetcher still reloads and
validates the case before fetching any missing selected documents. A verified
local repeat needs no court connection. Canonical duplicate selections collapse
in first-selection order; blank identities are ignored. Empty selection creates
no directory or manifest and performs no network I/O.

`SkipSourceURLs` explicitly skips members of the selection; a skip outside that
selection is an error. An unavailable selected identity remains an outcome.
Inputs and preview entries are not mutated. Only identities are passed to the
fetcher's fresh selection: caller-supplied request URLs are never downloaded
directly. Fresh docket descriptions/dates supplied by the fetcher accompany saved
files; original, unsanitized descriptions remain in metadata.

## Directory and overwrite policy

The chosen output directory must already exist and be an absolute local path.
For the synthetic case `60CV-2026-1`, the service creates `case-60cv-2026-1` beneath it.
Invalid case numbers, `..`, UNC/device paths, symlink ancestors, symlink case
directories, and Windows reparse-point/junction directories are rejected. A
link-free path is required, including on systems with symlinked temporary roots.
`os.Root` directory handles contain subsequent operations; file reads reject
links/nonregular files and compare the opened file identity. See Go's
[directory-root API](https://pkg.go.dev/os#Root) and
[traversal-resistant file APIs](https://go.dev/blog/osroot).

Example PDF name: `2026-09-01_order-1c5103bff26fd97a.pdf`. Components contain an
ASCII description/date label, 16 hex characters of document identity, and a PDF
extension. `SanitizeFilename` is bounded to 120 bytes, strips invalid characters,
protects Windows device names, removes trailing dots/spaces, normalizes an
existing `.pdf` extension, and appends `.pdf` after other extensions. The service
bounds its label further to leave space for identity and collision suffixes.
Full original text is preserved in the manifest. URLs are never used as paths.

With an explicit `DownloadRequest.Naming`, the service can select an AI label
from bounded opening-page text after the staged PDF has been independently
checked, before the recovery receipt and no-overwrite hard link are created.
Accepted labels are at most 64 ASCII bytes and are combined with the existing
filing-date and 16-hex identity convention. The complete basename remains at
most 120 bytes, including the PDF extension and collision suffix. The label is
never used as a path. If a label is unsafe, unsupported, ambiguous, or too long
to fit without losing a qualifier, the ordinary docket-derived filename is used.
The CLI passes no naming request; see [AI document naming](ai-document-naming.md).

Existing directory entries and historical manifest filenames reserve names
case-insensitively on every platform. Collisions receive `-1`, `-2`, etc. A
modified, missing, unrelated, or symlinked file is never assumed complete.
Repeat-run skipping requires matching case/document identity **and** reading the
current regular file to verify PDF screening, size, and full SHA-256. Modified
files remain intact; missing or modified documents download under a distinct
name. Valid older saved versions remain usable. Select only failed/missing
entries to retry those independently.

A persistent `.arcourt.lock` uses a nonblocking OS lock for each case directory:
Windows `LockFileEx`, Unix `flock`. A concurrent service/process gets
`ErrOutputBusy`; concurrent operations on one service get `ErrDownloadBusy`.
Closing the handle or process exit releases ownership. The lock file is never
unlinked, which avoids splitting ownership across different file identities.
Microsoft documents automatic release on process/handle closure in
[LockFileEx](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-lockfileex).

## Publishing and recovery

Each attempt opens a cryptographically random, exclusively created
`.arcourt-pdf-<nonce>.tmp` in the case directory. Writes are streamed, then synced,
closed, reopened, and independently screened for a PDF header/trailer and hashed.
There is a 1 GiB service ceiling even for custom fetchers; the browser fetcher's
default 64 MiB transfer limit is tighter. Neither whole PDFs nor batches are
buffered in memory. PDF screening is not a full object/xref validator.

Optional naming reads only the verified closed temporary file through the
contained directory handle. Naming never modifies the PDF bytes. An unreadable
or image-only PDF can still pass ordinary PDF screening and be published under
the standard filename. Naming failure is a successful PDF download with a
controlled naming fallback reason, not `ErrDocumentsIncomplete`.

Before publishing, the service writes a complete versioned recovery receipt
`.arcourt-receipt-<nonce>.json` describing the case, document, temporary name,
intended final name, size, and hash. It then creates a **hard link** from the closed
temporary file to the final filename. This is the no-overwrite publication
primitive on both Windows and Unix; a racing existing filename makes it fail or
choose another name. Ordinary rename is never used for PDF publication.

Supported destinations must implement local hard links and file locks, such as
Windows NTFS and ordinary Linux/macOS local filesystems. FAT/exFAT and network
shares are outside this version's supported output contract. Unsupported
operations return errors; there is no fallback that copies into a visible final
file or replaces an existing PDF. Antivirus/editor sharing conflicts on Windows
also return errors without erasing completed PDFs.

After publication, the manifest is saved; only then are the receipt and complete
temporary hard link retired. A failed manifest commit returns a failed document
with `Saved: true`, filename, size, hash, and `ErrManifest`. The PDF remains usable,
and its receipt and complete temporary link remain for reconciliation. A later
run verifies that the final file and temporary witness are the **same filesystem
file**, then verifies size/hash and records the result before retiring them.
Identical bytes at an unrelated replacement file do not establish ownership.
A subprocess test exercises actual process termination in this interval.

Failure/cancellation removes only the current attempt's owned incomplete files.
It never removes previously published PDFs or sweeps temporary files by prefix.
Forced termination can leave orphan staging files, especially before a receipt
is published. Recovery leaves unpaired, modified, or otherwise unproven artifacts
alone. They may be inspected and removed by the user when no download is active.
Corrupt/unsupported manifests or receipts stop the run and are preserved.

This contract covers ordinary I/O failure and process interruption. File sync and
complete-file replacement do not promise survival of arbitrary power loss,
hardware faults, or network filesystem behavior. The chosen local directory and
manifest are user-owned, trusted state; OS locks coordinate service writers, not
arbitrary programs intentionally modifying metadata while a run is active.

## Manifest version 1

`arcourt-manifest.json` is UTF-8 JSON, bounded to 8 MiB. Its public Go type is
`DownloadManifest`. The root contains:

| Field | Meaning |
| --- | --- |
| `version` | `1`; unsupported versions are rejected without rewriting |
| `case_number` | Validated, normalized case identity |
| `updated_at` | UTC RFC 3339 timestamp of the snapshot |
| `documents` | Saved versions and latest non-file attempts |

Each document record contains:

| Field | Meaning |
| --- | --- |
| `document_id` | Full SHA-256 of the canonical source identity |
| `description`, `filing_date` | Original docket labels, without filename sanitizing |
| `filename` | A single relative PDF basename; absent when no file was saved |
| `size`, `sha256` | Actual verified content size and SHA-256; zero/absent without a saved file |
| `timestamp` | UTC timestamp for this saved/attempted/verified result |
| `outcome` | `succeeded`, `failed`, `unavailable`, `skipped`, or `canceled` |
| `saved` | Whether the record refers to published content, including persistence failures |
| `skip_reason` | `verified_existing`, `explicit`, or `limit`, when skipped |
| `error` | Controlled error summary; raw transport/custom-fetcher errors are never serialized |
| `naming` | Optional source (`ai` or `deterministic`), accepted short label or controlled fallback reason, strategy, call count and limited usage metadata; absent on older/AI-disabled records |

Neither source/request URLs nor expiring signed URLs are persisted. Stable
identity follows the fetcher's canonicalization rules, including removal of known
S3 authentication parameters. Unknown-host credentials are not guessed away:
rotating unknown query fields produce distinct hashed identities. Hashing is not
anonymization, and case numbers/descriptions remain local case metadata.

Records with the same identity and filename are updated; distinct saved versions
are retained. A latest failed/skipped attempt with no filename does not remove a
saved version. Repeated failed attempts replace the previous non-file attempt.
The file is a compact local state snapshot, not an immutable audit history.
Version-1 manifests and receipts without `naming` remain valid. Unsupported
versions are still rejected without rewriting. A recovery receipt retains the
selected filename and naming outcome, so reconciliation does not call a provider.
Verified repeats, including prior naming fallbacks, skip both fetch and naming.

Manifest changes stage a new complete, synced, closed JSON file and replace the
recognized manifest under the case lock. The first manifest is published without
clobbering any existing file. Later updates verify the existing file identity
before replacement. No JSON file is truncated in place. Rename failures retain
the prior manifest and recovery evidence. Windows replacement can fail while an
editor holds a non-sharing handle; Go also does not guarantee rename atomicity
on all platforms (see [os.Rename](https://pkg.go.dev/os#Rename)).

## Results, progress, and cancellation

Always inspect both `LocalDownloadResult` **and** the returned error.
`Counts.Selected` equals `Succeeded + Failed + Unavailable + Skipped + Canceled`.
`Unavailable` is separate from `Failed`. Explicit skips and verified-existing
skips are successful decisions; limit skips leave the selection incomplete.
`ErrDocumentsIncomplete` reports any failed, unavailable, canceled, or limit-skipped
selection. `Partial` means usable saved content exists alongside incomplete work
or a run-level error. In particular, a failed document can have `Saved: true` if
its PDF was published before persistence/cleanup failed. A final snapshot error
can coexist with succeeded documents whose individual commits were already saved.

`LocalDocumentResult.Error` is safe display/persistence text. `Err` and the returned
error retain causes for `errors.Is`; custom fetchers may supply sensitive underlying
errors, so do not indiscriminately log unwrapped errors. A failed setup still
returns one terminal result per distinct selection, although it cannot persist
those outcomes if output storage itself is unavailable.

Optional typed events are `started` (per attempt), `transferring` (attempt bytes),
`document_done`, and `finished` (final counts). Delivery is nonblocking and
best-effort: a slow or absent subscriber cannot stall downloads or cancellation.
The caller owns the channel, must not close it before `Download` returns, and
must use the returned result as the authoritative terminal state. Per-document
terminal events are emitted when the fetch batch returns; transfer events are
emitted during I/O. Retries can restart byte counts.

Cancellation prevents new work and interrupts the fetcher's context-bound HTTP
I/O. Filesystem writes/verification check cancellation between operations;
portable Go cannot forcibly interrupt a blocked kernel disk write or `Sync`.
Once publication succeeds, manifest/cleanup work completes even if cancellation
arrives. Already saved PDFs remain usable. Hosts must use a fetcher honoring the
existing context and sequential writer contract.
If cancellation happens during optional naming after independent verification,
the provider call is canceled and the already complete PDF is published under
the deterministic filename through the bounded local receipt/link/manifest path.
The batch still reports cancellation and pending documents remain canceled.
Portable Go cannot forcibly interrupt a blocked kernel disk operation in that
finalization path. A remote request canceled after transmission has an uncertain
billing window; the service does not replay naming for a verified saved PDF.

## Verification

Ordinary tests require only synthetic fixtures and temporary local directories.
They inject disk operation failures deterministically, without permission tricks,
and include real local HTTP transfers, process termination, process locking,
modified/unrelated files, case-insensitive collisions, and manifest recovery.
Windows-only tests exercise junctions and open-handle rename failure. Symbolic
link tests explicitly skip if the account cannot create links. See
[testing](testing.md) for native commands, possible skips, and remaining manual gates.
