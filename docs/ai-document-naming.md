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

Its report path is `build/naming-benchmark-live.json`. The script passes the deliberately prompted key to its Go test child through a temporary process environment variable, then restores the previous environment state; it does not write the key to the report. The script accepts at most 512 transmitted calls and 1 MiB of serialized request bodies across the run; the adapter's per-document limits still apply. The current 59-case, three-strategy corpus can transmit 153–204 calls; set `-MaxCalls 205` so the runner can also finish the final no-call scan cases. Set both caps deliberately for the intended run; a partial budget is a partial evaluation. Confirm that every strategy has 59 cases before using its quality status or selected strategy.

The quality gates were declared before measurement: at least 99% accepted-name precision, 90% correct-label coverage on nameable supported-text fixtures, no material clear-title type/party/amendment errors, and no more than a two percentage point coverage loss against the bounded baseline. The repository's `docs/issue-8-plan.md` retains that declaration. Offline replay and injected HTTP tests establish contracts and regression behavior; they do not measure real-provider naming quality. The OpenAI `gpt-4.1-mini` and `gpt-6-luna` synthetic live results below are model-specific; xAI, Anthropic, and Gemini live quality remain unmeasured.

The offline `synthetic-court-v3` replay report contains 59 generated PDFs: 43 readable, 8 ambiguous, and 8 image-only scans, with 14 cases assigned to its synthetic held-out split. Its audited result was:

| Strategy | Correct readable labels | Accepted-name precision | Ambiguous abstentions | Scan fallbacks | Calls | Complete request bytes |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Bounded 4,096-character baseline | 43/43 | 100% | 8/8 | 8/8 | 51 | 29,286 |
| Targeted 512-character excerpt | 42/43 | 100% | 8/8 | 8/8 | 51 | 28,581 |
| Targeted excerpt with expansion allowed | 43/43 | 100% | 8/8 | 8/8 | 52 | 29,910 |

The overall cohort has 51 nameable PDFs when all 8 scans are included. Baseline and expansion cover 43/51 (84.3%) nameable PDFs and provide useful labels for 43/59 (72.9%) of all cases; small covers 42/51 (82.4%) and 42/59 (71.2%). The conservative local input estimate is one token per complete request-body byte, including every attempt: 496.4, 484.4, and 506.9 estimated input tokens per PDF respectively. These numbers are proxy estimates, not provider-reported tokens.

The small strategy saved 705 bytes (2.41%) against the baseline but lost one correct readable label, a 2.326 percentage point coverage loss that **fails the declared two-point non-inferiority margin**. Expansion recovered that label on its second call, but its aggregate request bytes exceeded the baseline by 624 bytes (2.13%). Thus the offline replay does not establish the production default as the lowest-total-context strategy meeting the gates. It demonstrates why a smaller first request cannot be evaluated without counting expansion; the baseline is the lowest-byte strategy among these replay results that meets the declared replay gates.

All 43 accepted readable replay labels came from the simulated AI response in the baseline and expansion strategies; the small strategy accepted 42. There is no zero-call local-title route. All 16 ambiguous and scanned cases fell back, and offline replay has no provider-reported usage. Replayed fixture responses cannot establish real-provider accuracy, token billing, or performance on court documents. The generated JSON report records corpus and strategy versions, source revision, case-level results, usage provenance, and a dirty-worktree flag.

### GPT-6 Luna follow-up

OpenAI lists `gpt-6-luna` as its efficient model for focused, high-volume tasks, at $0.10 per million input tokens and $0.50 per million output tokens on the standard tier. It is cheaper than `gpt-5.6-luna` at $0.20 and $1.20. Both Luna models default to medium reasoning; the adapter requests `reasoning.effort: none` for these exact IDs so reasoning does not consume the 96-token short-label budget. Other model requests are unchanged. This is a candidate for evaluation, not a passed quality gate. See the [GPT-6 Luna model page](https://developers.openai.com/api/docs/models/gpt-6-luna), [GPT-5.6 Luna model page](https://developers.openai.com/api/docs/models/gpt-5.6-luna), and [reasoning guide](https://developers.openai.com/api/docs/guides/reasoning).

First check account access and request compatibility with one transmitted call:

```powershell
./scripts/benchmark-naming.ps1 -Live -Provider openai -Model gpt-6-luna -MaxCalls 1 -MaxRequestBytes 16384 -AcknowledgeCharges -ReportPath build/naming-benchmark-openai-luna6-smoke.json
```

The smoke report is deliberately incomplete. If it records a transmitted call and a usable label or controlled `ABSTAIN`, run the full synthetic comparison with another masked key prompt:

```powershell
./scripts/benchmark-naming.ps1 -Live -Provider openai -Model gpt-6-luna -MaxCalls 205 -MaxRequestBytes 1048576 -AcknowledgeCharges -ReportPath build/naming-benchmark-openai-luna6-live.json
```

Inspect a model/access error or empty response before spending on the full run. All three strategies must have 59 cases before comparing their quality gates.

### OpenAI synthetic live result (October 8, 2026)

An operator ran the same 59-case corpus with `gpt-4.1-mini`, a deliberately entered API key, and caps of 205 transmitted calls and 1 MiB of complete request bodies. The report is `build/naming-benchmark-openai-live.json` at source revision `eb235e4c4aba7ebf60a4315fa945c3b371fc53b7` with a clean worktree. Every strategy completed all 59 cases. All 8 ambiguous cases abstained and all 8 image-only scans fell back per strategy; provider usage was reported for every transmitted call.

| Strategy | Correct readable labels | Accepted-name precision | Calls | Complete request bytes | Provider input/output tokens | Gate |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| Bounded 4,096-character baseline | 17/43 (39.53%) | 94.44% | 51 | 29,592 | 5,526 / 358 | Failed |
| Targeted 512-character excerpt | 20/43 (46.51%) | 95.24% | 51 | 28,887 | 5,307 / 343 | Failed |
| Targeted excerpt with expansion allowed (production) | 15/43 (34.88%) | 88.24% | 52 | 30,222 | 5,559 / 360 | Failed |

No strategy met the declared precision or coverage floors, so this run selects no minimum-context winner. The production strategy also trails the baseline by 4.65 percentage points of readable coverage, beyond the two-point margin. There were no material errors under the corpus's defined token rubric, but four accepted labels across the three strategies differed from the exact expected strings. Some look semantically plausible; crediting all four would still leave coverage far below 90%. Most readable fallbacks were `unsafe_result`, a strict output-format rejection. The report intentionally does not retain raw replies, so it cannot identify which formatting rule rejected them. A bounded synthetic-only diagnostic and a newly held-out evaluation are needed before changing normalization or claiming quality. This result says nothing about other model IDs or real court documents.

### GPT-6 Luna synthetic live result (October 9, 2026)

An operator ran the same corpus with `gpt-6-luna`, the same finite caps, and a clean source revision `238733bd2ffe99537a184608ffd3c15e6fdcad7a`. The report is `build/naming-benchmark-openai-luna6-live.json`. Each strategy completed 59 cases; all 8 ambiguous cases abstained and all 8 scans fell back per strategy. All 154 transmitted calls reported usage, with zero reasoning tokens under the explicit `none` setting.

| Strategy | Correct readable labels | Accepted-name precision | Calls | Complete request bytes | Provider input/output tokens | Gate |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| Bounded 4,096-character baseline | 33/43 (76.74%) | 84.62% | 51 | 31,020 | 5,475 / 459 | Failed |
| Targeted 512-character excerpt | 32/43 (74.42%) | 86.49% | 51 | 30,315 | 5,256 / 459 | Failed |
| Targeted excerpt with expansion allowed (production) | 31/43 (72.09%) | 81.58% | 52 | 31,678 | 5,507 / 474 | Failed |

Luna returned more exact expected labels than `gpt-4.1-mini`, but every strategy still failed the 99% precision and 90% readable-coverage gates. Production coverage was 4.65 percentage points below its Luna baseline, also failing the two-point margin. Its seven accepted labels that differed from the one exact expected string included possessive party forms, extra title detail, and a reordered ruling; none triggered this corpus's narrower material-error predicates. The production strategy also had five readable fallbacks: two `unsafe_result`, two `insufficient_context`, and one `unsupported_result`. These observations suggest the single-string scorer needs independent legal review and a prospectively defined equivalence rubric; they do not justify changing a gate after seeing its outputs. A fresh unseen evaluation is required after any scoring or naming change. No minimum-context winner was selected.

Across all three strategies, Luna used 16,238 input and 1,392 output tokens versus 16,392 and 1,061 for `gpt-4.1-mini`. At the [published standard text rates for Luna](https://developers.openai.com/api/docs/models/gpt-6-luna) and [GPT-4.1 Mini](https://developers.openai.com/api/docs/models/gpt-4.1-mini), those reported token counts imply approximately $0.00232 versus $0.00825 in model-token charges, excluding any account-specific billing adjustments. Summed per-case elapsed time was 194.7 seconds versus 133.9 seconds; this includes local work and network time and is not a model-only latency measurement. Neither synthetic run establishes performance on court/client PDFs.

### Reviewed equivalents and v4 scoring

Review of the eight distinct Luna labels that missed the v3 single-string score found the following evidence-grounded equivalents. This review does **not** change either completed v3 report or its failed gates.

| v3 case | Observed wording beyond the single expected string | Review |
| --- | --- | --- |
| `response-dismiss` | `Defendant's Response to Motion to Dismiss` | Acceptable possessive filing role; same motion and response. |
| `reply-dismiss` | `Plaintiff's Reply to Defendant's Response` | Acceptable; identifies both roles and preserves reply versus response. |
| `delayed-confidential-treatment` | `Motion for Confidential Treatment of Records` | Acceptable; the body identifies disputed records. |
| `caption-table` | `Defendant's Motion to Compel Discovery` | Acceptable in this fixture; the body identifies Defendant as movant and discovery as the subject. A caption alone would not establish movant. |
| `objection-discovery` | `Objection to Discovery (Defendant)` | Acceptable in this fixture; the body identifies Defendant as objector. |
| `hold-reply-role` | `Defendant's Reply to Plaintiff's Response` | Acceptable possessive roles; same reply and response. |
| `hold-protective-order` | `Protective Order (Order Granting)` | Awkward but accurate; preserves the order and granting ruling. |
| `hold-sanctions-response` | `Plaintiff's Response to Motion for Sanctions` | Acceptable possessive filing role; same motion and response. |

Even an exploratory calculation crediting all these variants on the old Luna run would leave production at 38/43 readable labels versus 39/43 for its baseline. That is 88.37% production coverage and a 2.33 percentage point baseline gap, so the production strategy would still fail the original 90% coverage and two-point non-inferiority gates. This calculation does not replace the published v3 scores.

The new `synthetic-court-v4` corpus has 48 previously untested PDFs: 32 readable filings, 8 ambiguous documents, and 8 image-only scans. All cases belong to its new held-out split. Its `predeclared-alias-v1` score accepts a generated label only when it matches a case's primary label or an explicitly listed alias after case folding, whitespace collapse, and removal of a possessive suffix from a filing-party role. The match must also satisfy the case's required and forbidden material tokens. Every approved label is checked against the PDF evidence and the production label validator before evaluation. Unlisted paraphrases count as incorrect in this run, even if they look plausible afterward; they require independent review and another new held-out evaluation before they can enter a later rule. No model judges its own output. The old `synthetic-court-v3` corpus retains `exact-label-v1` scoring.

The original gates remain in force: at least 99% precision among accepted readable labels, at least 90% correct-label coverage of readable PDFs, zero material errors, and no more than two percentage points of coverage loss versus the same-model bounded baseline. With 32 readable cases, coverage needs at least 29 correct labels, accepted precision allows no incorrect label, and even one lost correct label versus baseline exceeds the two-point margin. Ambiguous abstentions and scan fallbacks are reported separately and cannot increase readable coverage. The production excerpt/prompt/validator are unchanged for this evaluation.

To reproduce the frozen v4 offline replay, run from the repository root:

```powershell
./scripts/benchmark-naming.ps1 -Corpus v4 -ReportPath build/naming-benchmark-v4-offline.json
```

The replay validates extraction, request and scoring contracts but cannot measure model accuracy.

The frozen `cbfd585` offline replay completed all 48 cases in each strategy. Each got 32/32 readable labels from simulated responses, 8/8 ambiguous fallbacks, and 8/8 scan fallbacks, with 40 calls per strategy. Complete request bytes were 22,195 for the bounded baseline and 22,150 for both targeted strategies. This is a contract check only: replay emits the preapproved label when the excerpt contains the fixture title and therefore cannot establish real model accuracy. These v4 fixtures have straightforward opening titles and should be complemented by a later fresh layout-stress set before using their result to claim general minimum-context reliability.

To reproduce the `gpt-6-luna` live evaluation with a masked key prompt and finite caps, run:

```powershell
./scripts/benchmark-naming.ps1 -Corpus v4 -Live -Provider openai -Model gpt-6-luna -MaxCalls 241 -MaxRequestBytes 1048576 -AcknowledgeCharges -ReportPath build/naming-benchmark-openai-luna6-v4-live.json
```

The 241-call cap permits at most two calls for each of the 40 text cases in all three strategies plus a final no-call scan check; the per-request 16 KiB and per-generation 96-token limits still apply. All three strategies must each contain 48 cases, with no budget exhaustion, before the quality statuses or selected strategy are meaningful. Use the saved source revision and scoring version to distinguish this evaluation from v3.

### GPT-6 Luna v4 live result (October 9, 2026)

The operator's `build/naming-benchmark-openai-luna6-v4-live.json` report used clean source revision `8dbf79c024d83c2bb4e1d953ecca7935576d199a`, the frozen `synthetic-court-v4` corpus, and `predeclared-alias-v1` scoring. Every strategy completed all 48 cases; all 8 ambiguous cases abstained and all 8 scans fell back per strategy. All 120 transmitted calls reported usage and zero reasoning tokens. No budget was exhausted.

| Strategy | Correct readable labels | Accepted-name precision | Readable fallbacks | Calls | Complete request bytes | Gate |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| Bounded 4,096-character baseline | 29/32 (90.63%) | 100% | 3 | 40 | 23,555 | Met on this synthetic run |
| Targeted 512-character excerpt | 28/32 (87.50%) | 96.55% | 3 | 40 | 23,510 | Failed |
| Targeted excerpt with expansion allowed (production) | 28/32 (87.50%) | 96.55% | 3 | 40 | 23,510 | Failed |

The benchmark selected the bounded baseline as the only strategy meeting its gates on this run. Both targeted strategies trail it by one readable case, or 3.125 percentage points, beyond the two-point margin. They saved only 45 complete request bytes (0.19%) each; the production strategy made no second calls. The targeted strategies accepted one unlisted label, `Plaintiff's Reply to Defendant's Response on Motion to Strike`, which appears consistent with that synthetic document but fails the rule frozen before the run. It cannot be credited retroactively. The baseline rejected that case as `unsafe_result`, but accepted `Notice of Removal`; the targeted strategies rejected that notice as `unsafe_result`. All strategies rejected `Entered Order Vacating Hearing` as `unsupported_result` and `Notice of Substitution of Counsel` as `unsafe_result`. The report stores no raw rejected replies, so the exact rejection rule cannot be diagnosed from this artifact.

This result supports only the bounded baseline on this straightforward-title synthetic set. It does not supersede the failed v3 layout-stress run or establish general minimum-context reliability, real court-document accuracy, or an adopted production default. The near-identical transmitted context and differing labels are also insufficient to attribute the one-case difference to excerpt size rather than model variation. The production targeted strategy remains below the declared gate.
