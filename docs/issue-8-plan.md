# Issue 8 implementation plan

Source: [issue #8](https://github.com/tlfjar/ArcourtDownloader/issues/8) and the retained [implementation request](issue-8-implementation-prompt.md). Branch: `tlfjar/issue8`; starting commit: `391180bf1f65be9fcb99154f35e51a3cedaa1e6e` (clean checkout, Windows/amd64, Go 1.27.1, Node 24.12.0, npm 11.6.2).

## Work sequence

1. Evaluate current Go connector candidates against all four provider contracts and compile a small spike. Record the decision and dependency/license impact.
2. Extract only bounded opening-page text from independently validated staged PDFs. Select title-centered excerpts with a larger bounded baseline and explicit scan/unreadable fallback.
3. Implement one short-label workflow with bounded complete payloads, calls, deadlines, result validation and controlled errors. Add four real provider transports with injected HTTP tests.
4. Select the label after PDF verification and before receipt/no-overwrite publication. Persist only a controlled naming outcome, preserve verified-repeat and recovery behavior, and finalize a complete validated PDF with a deterministic name if naming is canceled.
5. Add desktop settings, secure Windows credential storage, provider-specific consent, and naming result display. Keep core/CLI behavior AI-free by default.
6. Build a synthetic corpus and an offline benchmark exercising production extraction/workflow; add opt-in, budgeted live synthetic evaluation without assuming access or spending authorization.
7. Update product and contract documentation, notices, and checks; review the integrated diff; commit coherent changes and prepare a PR description.

## Acceptance evidence map

| Issue criterion | Planned evidence |
| --- | --- |
| Four selectable providers, model IDs, keys, default off | Desktop form, secret-store tests, provider transport tests |
| Unified connector selection | Dependency evaluation, source/API review, compile spike |
| Actual PDF, bounded context, no full upload | Extractor and payload tests, documented limits |
| Small/expanded/baseline comparison and quality target | Reproducible synthetic benchmark with explicit denominators and usage provenance |
| Accurate short labels and safe abstention | Synthetic title/layout and adversarial output regressions |
| Scanned PDF treatment | Explicit image-only fallback tests or bounded image path |
| Filesystem integrity/recovery/reruns | Existing writer/manifest integration tests, byte hashes, collision/cancel/failure tests |
| Consent/credential/privacy boundary | DPAPI or Credential Manager implementation, snapshots/settings tests, transport host checks |
| Timeout/retry/cancellation and ordinary download | Bounded-call tests, cancellation integration tests, full existing checks |
| Documentation/build compatibility | README/contracts/notices, both Go modules, frontend, packaging/native checks |

Quality targets declared before measurement: at least 99% accepted-name precision and 90% correct-label coverage on nameable supported-text synthetic fixtures, no material type/party/amendment errors in the clear-title regression cohort, and no more than a 2 percentage point coverage loss versus the larger bounded baseline. These are gates, not results; if live evaluation is unavailable, provider quality remains unmeasured.

## Implemented decisions and evidence status

- Pinned GoAI v0.10.6 for OpenAI Responses, Anthropic Messages, and Google Gemini REST, with xAI through its documented OpenAI-compatible Chat Completions endpoint. The [connector evaluation](ai-connector-evaluation.md) records the four-package comparison, compilable mock spike, licenses, and footprint. Direct provider destinations are pinned; no gateway or custom endpoint is configured.
- Pinned `giraffesyo/pdf` v0.7.0 for portable local text extraction. Production inspects only the first two pages of validated PDFs up to 16 MiB, with 64 MiB cumulative file reads, parser work ceilings, and a five-second extraction context. Its page-tree walk is structurally bounded at 50,000 nodes/depth 64. Parser in-memory work and blocked kernel reads are not forcibly preemptible.
- Production sends a 512-character targeted excerpt first, with at most one 1,024-character expansion after explicit `ABSTAIN`; the 4,096-character opening-page excerpt is the benchmark baseline. The complete serialized provider request is capped at 16 KiB; output at 64 KiB; generation at 96 requested tokens; and a twelve-second naming context covers extraction and provider work. SDK retries are disabled. Image-only PDFs fall back without OCR or image transmission.
- Manifest version 1 retains optional controlled naming provenance. Older records without it remain valid, and a historical bounded strategy string does not invalidate a saved record. Recovery and verified repeats do not call a provider again. Windows Credential Manager holds desktop API keys; settings contain only non-secret choices and recipient consent.
- The `synthetic-court-v3` offline report, reproducible with [`scripts/benchmark-naming.ps1`](../scripts/benchmark-naming.ps1), has 59 cases (43 readable, 8 ambiguous, 8 scanned; 14 held out). Baseline: 43/43 readable labels, 29,286 complete request bytes/51 calls. Targeted small: 42/43, 28,581 bytes/51 calls. Targeted expansion: 43/43, 29,910 bytes/52 calls. Accepted-name precision is 100% in this replay; all ambiguous and scanned cases fall back. The small strategy **fails** the declared two-point non-inferiority margin by losing 2.326 percentage points of readable coverage. Expansion restores coverage but uses 624 more bytes than baseline. These fixture responses measure contract/evidence behavior, not provider accuracy or billing. No paid live call was authorized or made, so the minimum-total-context production choice and real-provider quality remain unverified.
- `scripts/check.ps1`, the native Wails build, Chrome browser/CLI fixtures, the Chrome-backed native GUI fixture, local unsigned Windows packaging/asset verification, notice check, and both module vulnerability checks passed. Packaging used the reviewed Go 1.26.8 toolchain on Windows/amd64. An Edge fixture retry still failed immediately at headless browser startup (`chrome failed to start` with no further stderr). The fixture now requires an actual case-header mismatch error before crediting that check, and the Chrome override covers its setup and restoration. A human production-GUI walkthrough and live-provider quality measurements have not occurred. The race detector was unavailable in this session (`CGO_ENABLED=0`, no GCC).
