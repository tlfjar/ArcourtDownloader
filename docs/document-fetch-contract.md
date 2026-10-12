# Discovery and document download contract

`arcourt.CaseFetcher` provides
`PreviewCaseDocuments` and `DownloadCaseDocuments`. `BrowserFetcher` implements
both and owns installed-browser sessions; call `Close()` at application shutdown.
The [shared filesystem download service](filesystem-download-service.md) wraps
this contract. The [CLI](command-line-workflow.md) and
[Windows desktop GUI](desktop-architecture.md) both use that service.
Optional AI document naming runs in the shared Go service only after a selected
PDF has been downloaded and independently checked. It uses a separate provider
transport and never inherits the fetcher's court cookies, request URL, or HTTP
client. See the [AI naming guide](ai-document-naming.md).

## Identity and discovery

Both operations validate the requested case number and independently extracted
case number before returning a usable preview or requesting a PDF. The existing
pattern is now anchored: two digits, zero through four letters, a hyphen, one
through six digits, a hyphen, one through six digits. Input is trimmed and letters
are uppercased. Explicitly synthetic examples include `60CV-2026-1`, `99ZZZ-99-1`, and
`60DR-24-42`; this is not a claim to support every Arkansas court numbering scheme.
Malformed requests return `ErrInvalidCaseNumber`; absent, malformed, or different
rendered identity returns `ErrCaseNumberMismatch`. The address bar is no longer
used as independent identity evidence. Explicit labelled identity takes priority
over the existing body-text fallback, including when the label is malformed.

`CasePreview` retains `CaseInfo` (number, title, county, judge, parties) and
`DocketEntry` values (description, filing date, source identity, request URL):

- `SourceURL` is the canonical selection identity. Prefer the original court
  document endpoint, rather than its JSON-resolved storage URL. Canonical matching
  ignores host/scheme case, default ports, fragments, trailing slashes, and query
  ordering. On the one recognized S3 bucket, known SigV4 and legacy authentication
  parameters are omitted from identity; resource selectors such as `versionId`
  remain. These parameters are authentication described by
  [AWS SigV4 documentation](https://docs.aws.amazon.com/AmazonS3/latest/developerguide/sigv4-query-string-auth.html)
  and [legacy S3 documentation](https://docs.aws.amazon.com/AmazonS3/latest/developerguide/RESTAuthentication.html).
- `RequestURL` is kept separately, preserving the extracted query order and
  escaping. It alone is used for HTTP. It is excluded from JSON serialization.
  It may contain credentials or expire: never log or persist it. Unknown hosts'
  query parameters are not guessed away; an unfamiliar expiring identity requires
  a verified source-ID rule before supporting it.

Rows with the same canonical source identity merge missing metadata and keep the
first request URL. Download reloads discovery, so a selection uses the current
request URL rather than a stale preview URL. It never blindly trusts a caller's
preview or requests a URL merely because it appeared in the selection.

`DiscoveryCoverage` reports:

| Field | Meaning |
| --- | --- |
| `DiscoverableDocuments` | Distinct observed document links before the preview limit |
| `Truncated` | Preview entries were omitted by `MaxDocuments` |
| `Complete` | Whether discovery establishes the whole docket; false for both public OPAD and DOM discovery |
| `TotalDocuments` | Whole-docket document count; nil when unknown, as with the current reader |
| `PaginationDetected`, `VirtualizationDetected` | Diagnostic DOM signals, not proof of completeness when absent |
| `ReportedDocketRows` | An API docket row count or detected ARIA row count; never treated as a count of PDFs |
| `Limitations` | Explanation suitable for callers to show with a preview |

For the exact template `https://caseinfonew.arcourts.gov/opad/case/{case_number}`,
discovery reads the same public `/opad/api/cases/{case_number}` JSON used by the
court application. It validates case identity, extracts title/county/judge and
participants, and includes all returned docket and case-level PDF references.
Document requests use `documentFileId`, not a filename-derived guess. Filing
dates use America/Chicago, including daylight saving time. The case request
shares the bounded, TLS-verified document transport, its JSON byte limit and
retry policy, with `PageTimeout` bounding the entire case request and retries.
An access rejection returns `ErrCaseAccessDenied`; no browser fallback runs after
an API rejection. Missing schema fields fail instead of becoming an empty preview.
`ReportedDocketRows` counts the returned docket rows; `Complete` stays false and
`TotalDocuments` stays unknown because withheld/unreturned records are unknowable.
Browser installation is still checked at fetcher construction, but this route
does not launch a browser. Other hosts, paths, query strings and custom templates
retain browser discovery.

For those other templates, the parser enumerates rendered table rows, document anchors, and PDF
title buttons. It does not traverse pages, scroll a virtualized grid, or trigger
lazy loading. Generic next-page, Material UI pagination/grid, and ARIA signals
are inspected. Their current live-court use is unverified. Show previews as
**discovered documents**, accompanied by coverage, never as a complete docket.
The known discoverable total includes rows omitted by the preview cap; unknown
unrendered documents are not fabricated as individual outcomes.

## Selection and outcomes

`DocumentSelection{}` and an empty/whitespace-only `SourceURLs` list select
nothing. A syntactically valid case number is still required, but no browser,
HTTP request, or sink is needed. `DocumentSelection{All: true}` explicitly selects
every document discovered on a fresh load. Mixing `All` and `SourceURLs` is an
invalid request. For UI “Select All visible,” pass the displayed source IDs if
the user intends only the preview rows; `All` can discover more than the preview.

Explicit source IDs are canonicalized, blanks removed, and duplicates collapsed
in first-selection order. There is one outcome per distinct selected document.
An unmatched ID is `unavailable`, including IDs absent from incomplete discovery.
Unsupported URL identities are never silently dropped or directly fetched.

`MaxDocuments` defaults to 200. It caps preview entries and distinct document
attempts per download. Retries count as the same document; failed documents
consume a slot and missing IDs do not. Download discovery itself is not truncated:
explicit selections beyond the preview cap can still be found. All selected
overflow gets `skipped_limit` with `ErrDocumentLimit`, rather than empty PDF bytes.

| Status | Meaning |
| --- | --- |
| `succeeded` | Validated bytes were accepted by the writer's successful `Commit` |
| `failed` | HTTP, content, writer, or case-wide error |
| `unavailable` | Selected identity absent from refreshed discovery |
| `skipped_limit` | Available selection exceeded the document-attempt limit |
| `canceled` | Caller deadline/cancellation or application shutdown stopped this or remaining work |

`DownloadResult` retains case metadata, coverage, and every outcome. Successful
outcomes include committed byte count and SHA-256; failures have zero committed
bytes and an error. Attempts counts whole HTTP attempts, not redirect/JSON hops.
Earlier successes survive later failures; independent next documents continue.

Always inspect the result even when the returned error is non-nil.
`ErrDocumentsIncomplete` means at least one selected outcome did not succeed.
`All` additionally returns `ErrIncompleteDiscovery` whenever whole-docket
completeness is unknown, even if every discovered PDF succeeded. Explicit
selections can succeed while coverage remains incomplete. Cancellation is also
available through `errors.Is(err, context.Canceled/DeadlineExceeded)`.
Case-wide discovery/identity errors prevent all HTTP work and mark every known
explicit selection failed/canceled. Before discovery, `All` has no known IDs to
enumerate. Invalid selection combinations are rejected before any work.

## Writer lifecycle and bounds

`DocumentSink.OpenDocument(ctx, entry)` supplies a fresh `DocumentWriter` for an
attempt. The fetcher streams chunks, then calls `Commit` only after content and
length validation. `Abort` is called after any failure with an open writer,
including a failed commit. The sink must stage unpublished bytes, release its
resources on commit/abort, and remove its failed staging output. If opening fails,
the sink itself must release partially acquired resources. Writer operations must
honor the supplied context and finish promptly; Go cannot forcibly interrupt an
arbitrary blocking writer. A successful commit is terminal even if cancellation
arrives immediately afterward.

Retries open a new writer; they never append to a failed attempt. Writer failures
are not retried, and abort failure also suppresses retries. No queue or batch PDF
buffer is used. PDF transfer memory is independent of PDF/batch byte size: a
32 KiB read buffer, 32 KiB copy buffer, bounded trailer window and hashing state.
JSON uses a separately capped buffer. Metadata/outcomes scale with discovered
and selected document counts. Sink implementations must also avoid accumulating
all document bytes. `DownloadService` supplies filesystem staging/publishing.

```go
preview, err := fetcher.PreviewCaseDocuments(ctx, caseNumber)
if err != nil {
    return err
}
// Present preview.DocketEntries and preview.Discovery to the user.
// chosenSourceURLs contains the identities the user explicitly selected.
result, downloadErr := fetcher.DownloadCaseDocuments(ctx, caseNumber,
    arcourt.DocumentSelection{SourceURLs: chosenSourceURLs}, sink)
// Consume result.Outcomes even when downloadErr != nil.
// For explicit All, use arcourt.DocumentSelection{All: true} instead.
```

## HTTP policy

The same policy covers initial targets, relative/absolute JSON targets, redirects,
and every DNS answer before connecting. Recognized destinations are `arcourts.gov`,
its subdomains, and the exact
`cdr-prod-cmslegacy-images-bucket.s3.us-gov-west-1.amazonaws.com` host.
Only HTTP/HTTPS and their default ports are accepted; userinfo and HTTPS downgrade
are rejected. The configured **browser page endpoint** remains caller-controlled;
these document transport rules are not a browser subresource firewall.

All resolved IPs must be public; private, loopback, link-local, multicast,
unspecified, shared, documentation, reserved, and known translation/tunnel ranges
are rejected. Mixed public/private answers fail before dialing. The dialer receives
the validated numeric IP, preventing a second DNS lookup from bypassing policy.
Environment proxies are disabled and each request uses a checked new connection.
TLS verification remains enabled with the original hostname. The implementation
uses the connection hooks and redirect policy in Go's
[HTTP transport/client](https://pkg.go.dev/net/http#Transport) and
[context-aware dialer](https://pkg.go.dev/net#Dialer.DialContext).

`BrowserFetcherConfig.DocumentHTTP` supplies bounded settings:

| Setting | Default | Maximum |
| --- | --- | --- |
| PDF body | 64 MiB | 1 GiB |
| JSON body | 1 MiB | 4 MiB |
| Whole attempt, including JSON hops | 45 seconds | 5 minutes |
| DNS/connection and TLS handshake | 10 seconds | 30 seconds |
| Header / individual socket read | 15 seconds | 1 minute |
| Attempts | 3 | 5 |
| Linear retry base delay | 250 milliseconds | 5 seconds |

Nonpositive values use defaults; larger values are capped. `MaxAttempts: 1`
disables retries. Each HTTP request permits at most five redirects and an attempt
permits at most three JSON indirections. HTTP headers are capped at 64 KiB.
Bodies are bounded even with missing/false lengths or chunked transfer encoding.
There is no automatic decompression that can evade the body limits.

Transient HTTP 408/429/500/502/503/504, temporary network failures, timeouts, and
short reads may retry within those bounds. Cancellation never retries, including
during backoff. 401/403/410 identify denied/expired links; refresh discovery rather
than retrying indefinitely. Non-2xx and unsolicited 206 responses fail. JSON
supports `url`, `downloadUrl`, `documentUrl`, then `signedUrl` in that precedence;
relative URLs resolve against the final response URL after redirects.

Success requires a PDF version signature at byte zero, full body consumption
within the size limit, matching declared length when present, and a final `%%EOF`
marker in a bounded trailer window. HTML/error pages, mislabeled JSON, empty,
truncated, and oversized bodies fail. This is transfer/content screening, not a
full PDF object/xref parser or a guarantee that a PDF viewer can render every file.
Leading junk and non-whitespace after the last EOF marker are intentionally rejected.

The HTTP client does not forward browser cookies or authorization. It has no
cookie jar or session credentials and clears redirect headers, including Referer. A storage host never
receives court session state. If live behavior later requires authentication,
verify it and add exact-origin scoping with regression tests first. Error messages
omit request URLs and response bodies. Do not log raw entries or unwrapped
transport causes, which can contain sensitive URL data.

`LookupIP` and `DialContext` are trusted test/host integration hooks, not options
to pass from untrusted UI input. Tests inject public DNS answers and map the
already validated numeric address to a local fixture server; production validation
has no localhost exception. Leave both nil in production.

## Testing and remaining confirmation

`IsRetryable(context.Canceled)` is false. Empty selection never means all, and
PDFs are streamed rather than accumulated in a batch. See [testing](testing.md)
for reproducible automated checks and the separate manual/live boundaries.

Synthetic tests exercise the retained metadata, anchor/button, relative-link,
filing-date, and party extraction. Opt in with `ARCOURT_DOM_FIXTURES=1`; optionally
set `ARCOURT_BROWSER_EXECUTABLE`. Live selectors, pagination/virtualization,
supported case formats outside existing examples, direct signed-link behavior,
authenticated document requirements, and real court PDF acceptance still need
manual confirmation. No live page, personal case document, credential, or real
signed URL is checked in or logged.
