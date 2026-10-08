# Optional AI document naming

AI naming is off by default. It runs only for a newly downloaded, selected PDF after the file has been closed and independently checked for a PDF header, end marker, size, and SHA-256 hash. Preview, unselected documents, failed downloads, and verified existing files do not make naming requests. The core and CLI keep ordinary deterministic names unless a caller explicitly supplies a `NamingRequest`.

## Desktop workflow

1. In **Settings & diagnostics → Optional AI document names**, enable naming, choose OpenAI, xAI, Anthropic, or Google, and enter a model ID available to your provider account. The application validates configuration locally; saving settings does not make a paid request.
2. Review the direct recipient shown for the selected provider, the bounded-text disclosure, possible sensitive content, possible API charges, and the fallback behavior. Check the recipient consent box and save settings. Changing provider clears the box and requires consent for the new recipient.
3. Save or replace that provider's API key in the separate credential form. The form displays configured, missing, or unavailable status and can remove the key. The entered value is cleared from the UI after submission. Windows Credential Manager stores the key for the current Windows user; `settings.json` stores only the enable flag, provider, model ID, and approved recipient. Snapshots and download manifests never contain the key.
4. Preview the case, verify its header, select documents, and download as usual. The per-document result shows **AI label** when a label was accepted or **Standard filename** with a controlled fallback reason. A naming fallback does not change a successfully saved PDF into a failed download. Missing consent, missing keys, or unavailable secure storage leave ordinary downloading usable.

The selected non-secret settings and credential are captured when the download job starts. Settings and credential changes are disabled while a job is busy. Requests go directly to the selected provider's documented HTTPS route:

| Provider | Recipient |
| --- | --- |
| [OpenAI](https://developers.openai.com/api/reference/resources/responses/methods/create) | `https://api.openai.com/v1/responses` |
| [xAI](https://docs.x.ai/developers/rest-api-reference/inference/chat-completions) | `https://api.x.ai/v1/chat/completions` |
| [Anthropic](https://platform.claude.com/docs/en/api/messages/create) | `https://api.anthropic.com/v1/messages` |
| [Google](https://ai.google.dev/api/generate-content) | `https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent` |

The connector selection is detailed in [the connector evaluation](ai-connector-evaluation.md). There is no configured gateway or app telemetry. Provider privacy, retention, and training practices depend on the provider and account; review the provider's current terms before enabling this feature.

## Data flow and limits

The completed staged PDF stays local. A portable Go parser reads only the first two pages' text; it may inspect PDF structure to locate those pages, but does not extract later page content. Local selection favors a line containing a document-type word near the top of an opening page, preserves nearby wrapped heading and explanatory lines, removes common caption and contact boilerplate, and deduplicates repeated lines. This is a heuristic, so ambiguous or unusable evidence falls back.

| Boundary | Current maximum or behavior |
| --- | --- |
| PDF eligible for optional parsing | 16 MiB; larger validated downloads keep an ordinary name. |
| Pages and parser work | First two pages; a 5-second extraction context and at most 64 MiB cumulative local file reads. The parser caps page-tree traversal at 50,000 nodes and depth 64. Per page: 16,000 glyphs, 100,000 operators, 1 MiB stream, form depth 8, up to 64 images, 1 MiB image bytes, 1 million image pixels. |
| Local evidence | Targeted small excerpt: 512 characters / 2 KiB; targeted medium excerpt: 1,024 characters / 4 KiB; opening-page benchmark baseline: 4,096 characters / 8 KiB. |
| Production transmission | Small targeted excerpt first. Medium targeted excerpt is sent only after an explicit `ABSTAIN` and only if it contains more evidence. Baseline is not sent by the normal download path. |
| Final provider boundary | System instruction at most 2,048 bytes; excerpt at most 4,096 characters and 8 KiB; entire SDK-serialized JSON request at most 16 KiB; response body at most 64 KiB. These are byte/character ceilings, not exact provider-token guarantees. |
| Calls and time | At most two transmissions per document, including expansion; SDK retries and tool steps are disabled. A twelve-second context deadline covers extraction and naming; each HTTP call has a ten-second timeout within that deadline. Requested output limit: 96 tokens. |
| Accepted filename label | Short ASCII label, at most 64 bytes. A set of document-type and qualifier words in the result is checked against the transmitted excerpt. `ABSTAIN`, unsafe, unsupported, truncated, or overlong results fall back; material qualifiers are not silently truncated. This validation is conservative but does not establish that every accepted label is correct. |

The provider receives a short system instruction and one document excerpt as text. It does not receive the PDF, image, source or signed URL, case history, other documents, court cookies, or a conversation. The AI request uses a separate HTTP client with fixed HTTPS destination and no environment proxy or redirect forwarding. No file upload, remote URL-fetch tool, web tool, automatic provider change, or image mode is configured. API keys are sent as provider-specific request headers.

Cancellation is checked on parser file reads and provider requests. Portable Go cannot forcibly interrupt a parser's in-memory structural step or a blocked kernel file read, so the context deadline is not a hard operating-system time limit. The file-size, cumulative-read, parser-work, and page-tree ceilings limit the work that can continue before fallback.

Scanned or image-only pages without usable text use the `image_only` ordinary-name fallback. The application does not perform OCR or send page images. Encrypted, malformed, oversized, or otherwise unreadable PDFs also fall back locally. A fallback here is a saved PDF with a standard filename, not a successful content-based name.

`RequestBytes` records the complete serialized outbound JSON body for an attempted request, including SDK-added JSON. `InputTokenEstimate` uses one token per request-body byte as a conservative local estimate; it is not a tokenizer result or provider charge. Provider-reported input, output, cached, and reasoning token counts are kept separately. Usage missing from a failed or incomplete request is **unknown**, not zero. A transport error or cancellation can happen after a request is sent, so recorded bytes do not establish whether the provider completed or billed it. The app does not automatically replay such uncertain calls.

## Results shown in the desktop

| Result | Meaning |
| --- | --- |
| **AI label** | The bounded response passed local format and evidence checks, and its label was used in the saved filename. |
| **Standard filename** | The PDF was saved with its ordinary deterministic name. The displayed reason may identify unusable local text (`image_only`, `encrypted`, `too_large`, `unreadable`), insufficient or rejected model output (`insufficient_context`, `unsafe_result`, `unsupported_result`, `length_exhaustion`), a provider/configuration problem (`configuration`, `provider_auth`, `provider_model`, `rate_limited`, `provider_refusal`, `provider_error`, `payload_limit`), or cancellation/timeout. |

The download's own saved, skipped, failed, unavailable, or canceled status remains separate from the naming result. When naming was disabled or omitted before the download, no AI naming result is recorded.

## File integrity, recovery, and reruns

The name is chosen after the staged PDF passes the existing independent byte check and before its receipt and no-overwrite hard-link publication. The accepted AI result is a label, never a path. The existing sanitizer and case-insensitive collision logic construct a filename from filing date, label or docket description, document identity, and `.pdf`, within the existing 120-byte basename limit. The original PDF bytes and original docket description stay unchanged.

If naming fails or times out, the validated PDF is published with the ordinary deterministic name. If cancellation occurs during optional naming after byte validation, the request is canceled and the PDF is still finalized locally with a deterministic name; the overall job reports cancellation. Previously published PDFs remain in place. Disk, receipt, or manifest failures remain download failures and are not recast as naming fallbacks.

The version 1 manifest and recovery receipt keep only controlled naming source, accepted label or fallback reason, strategy version, call count, and usage metadata. They do not store excerpts, prompts, raw model replies, provider error bodies, URLs, or credentials. Older version 1 entries without naming fields remain readable; incompatible manifest versions are rejected. On recovery, the receipt is reconciled only when the complete temporary hard link and final PDF are the same file and its saved size and hash still match. Recovery restores the filename and naming outcome without another provider call.

On a later run, a saved file is skipped only after its actual size and hash match the manifest record. This includes files previously saved with fallback names; no download or paid naming is repeated. Changing AI settings does not retroactively rename those files.

## Benchmark and evidence

Run the reproducible synthetic benchmark from the repository root:

```powershell
./scripts/benchmark-naming.ps1
```

Its default offline mode uses generated fixtures and replayed responses with no court documents or paid requests. It writes `build/naming-benchmark-offline.json` and compares a 4,096-character opening-page baseline, a roughly 512-character targeted excerpt, and a targeted excerpt with a bounded expansion to 1,024 characters. The report counts all attempts and complete serialized request bodies, distinguishes provider-reported usage from estimates and missing usage, and separates readable, ambiguous, scanned, and overall cohorts.

Live evaluation against synthetic fixtures requires an explicit provider/model, finite call and request-byte caps, a masked key prompt, and charge acknowledgment:

```powershell
./scripts/benchmark-naming.ps1 -Live -Provider openai -Model <model-id> -MaxCalls <positive-int> -MaxRequestBytes <positive-int> -AcknowledgeCharges
```

Its report path is `build/naming-benchmark-live.json`. The script passes the deliberately prompted key to its Go test child through a temporary process environment variable, then restores the previous environment state; it does not write the key to the report. The script accepts at most 512 transmitted calls and 1 MiB of serialized request bodies across the run; the adapter's per-document limits still apply. Set both caps deliberately for the intended run; a partial budget is a partial evaluation.

The quality gates were declared before measurement: at least 99% accepted-name precision, 90% correct-label coverage on nameable supported-text fixtures, no material clear-title type/party/amendment errors, and no more than a two percentage point coverage loss against the bounded baseline. The repository's `docs/issue-8-plan.md` retains that declaration. Offline replay and injected HTTP tests establish contracts and regression behavior; they do not measure live OpenAI, xAI, Anthropic, or Gemini naming quality. No live-provider quality or context-efficiency result is claimed here yet.

The offline `synthetic-court-v3` replay report contains 59 generated PDFs: 43 readable, 8 ambiguous, and 8 image-only scans, with 14 cases assigned to its synthetic held-out split. Its audited result was:

| Strategy | Correct readable labels | Accepted-name precision | Ambiguous abstentions | Scan fallbacks | Calls | Complete request bytes |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Bounded 4,096-character baseline | 43/43 | 100% | 8/8 | 8/8 | 51 | 29,286 |
| Targeted 512-character excerpt | 42/43 | 100% | 8/8 | 8/8 | 51 | 28,581 |
| Targeted excerpt with expansion allowed | 43/43 | 100% | 8/8 | 8/8 | 52 | 29,910 |

The overall cohort has 51 nameable PDFs when all 8 scans are included. Baseline and expansion cover 43/51 (84.3%) nameable PDFs and provide useful labels for 43/59 (72.9%) of all cases; small covers 42/51 (82.4%) and 42/59 (71.2%). The conservative local input estimate is one token per complete request-body byte, including every attempt: 496.4, 484.4, and 506.9 estimated input tokens per PDF respectively. These numbers are proxy estimates, not provider-reported tokens.

The small strategy saved 705 bytes (2.41%) against the baseline but lost one correct readable label, a 2.326 percentage point coverage loss that **fails the declared two-point non-inferiority margin**. Expansion recovered that label on its second call, but its aggregate request bytes exceeded the baseline by 624 bytes (2.13%). Thus the offline replay does not establish the production default as the lowest-total-context strategy meeting the gates. It demonstrates why a smaller first request cannot be evaluated without counting expansion. The production default remains title-centered small evidence with one bounded expansion while real-provider quality and total-context selection remain unverified; the baseline is the lowest-byte strategy among these replay results that meets the declared replay gates.

All 43 accepted readable replay labels came from the simulated AI response in the baseline and expansion strategies; the small strategy accepted 42. There is no zero-call local-title route. All 16 ambiguous and scanned cases fell back, and offline replay has no provider-reported usage. Replayed fixture responses cannot establish real-provider accuracy, token billing, or performance on court documents. The generated JSON report records corpus and strategy versions, source revision, case-level results, usage provenance, and a dirty-worktree flag.
