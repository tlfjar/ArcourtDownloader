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
checked, before the recovery receipt and no-overwrite publication operation.
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
intended final name, size, and hash. PDF, first-manifest, and receipt publication
all use the same selected no-replace backend. There is no copying into a visible
final PDF and no overwrite fallback.

| Destination | Publication | Interrupted recovery |
| --- | --- | --- |
| Windows with hard-link capability (including NTFS) | Existing `os.Root.Link` path | Version-1 receipt plus same-file temporary witness |
| Local Windows FAT32/exFAT without hard links | Handle-relative `FileRenameInformation`, replacement disabled; runtime probe required | Version-2 receipt; uncommitted publication requires manual review |
| Other non-Windows local filesystems | Existing hard-link path | Existing version-1 recovery |
| Other Windows filesystems without hard links, network shares | Rejected | No weaker fallback |

Windows queries the volume through a contained directory handle and checks the
hard-link capability flag. FAT32/exFAT candidates must then pass a disposable
same-directory probe: a collision must fail without changing either file, and a
successful publication must consume the staging name and preserve its bytes.
The case lock has already been acquired. Access/sharing failures remain distinct
from unsupported-capability errors; they do not trigger a different backend.
The runtime probe is an operational prerequisite, not certification of every
filesystem driver or removable device. See [actual test evidence](fat-filesystem-evidence.md).

The new Windows operation opens the source relative to the pinned directory with
`NtCreateFile`, rejects reparse points/nonregular files and verifies the opened
identity, then uses `NtSetInformationFile(FileRenameInformation)` with
`ReplaceIfExists = FALSE` and the same directory handle as `RootDirectory`.
Microsoft documents [no-replace and directory-relative rename semantics](https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/ntifs/ns-ntifs-_file_rename_information).
A racing target produces a collision, after which PDF naming may choose a new
suffix. Ordinary Go `Rename` is still used **only for replacing a recognized
manifest**, never for PDF, receipt, or first-manifest publication. Path-based
`MoveFileEx` was not selected because publication must retain directory-handle
containment.

After publication, the manifest is saved before the receipt is retired. For the
hard-link backend, the complete temporary witness remains until this commit;
version-1 recovery still requires the temporary and final names to refer to the
**same filesystem file**, then verifies PDF screening, size, and SHA-256 before
adoption. Identical bytes at an unrelated replacement never prove ownership.

For FAT32/exFAT the successful rename consumes the temporary name. The service
never deletes that old basename after success, since a later file at it is not
owned. A version-2 receipt left by interruption is treated conservatively:

- If the trusted manifest already records the same document/name/size/hash, the
  final file passes normal repeat verification, and the temporary name remains
  absent, only the receipt is retired. No PDF is adopted or deleted.
- Otherwise the run stops **before network I/O**, preserves the receipt and all
  data files, and reports `ErrRecoveryRequired`. It does not reconstruct identity
  from hashes or FAT file IDs, which can change on rename/reuse. This includes a
  receipt published before its PDF was renamed.
- To proceed, use a new empty output folder. Alternatively, with no download
  active, inspect the receipt and the specific files it names, keep any needed
  PDFs, and move that receipt outside the case folder. Leave uncertain files
  intact. A retry reserves existing filenames and chooses a distinct suffix.
  Do not remove all files merely because they have an `.arcourt-` prefix.

A failed manifest commit still reports `Saved: true` with filename, size, hash,
and `ErrManifest`; the published PDF remains usable. On FAT32/exFAT the next run
may require the manual review above. Manifest version 1 is unchanged. Only the
new consuming backend emits version-2 receipts, so older binaries reject pending
new receipts instead of silently treating them as hard-link witnesses.

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
the deterministic filename through the bounded local receipt/publication/manifest path.
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
