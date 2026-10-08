# Implement ArcourtDownloader issue #8: accurate, minimum-context document naming

Work in `tlfjar/ArcourtDownloader`.
Authoritative issue: https://github.com/tlfjar/ArcourtDownloader/issues/8

## Mission and scope

Fully implement issue #8 against the actual current checkout. Deliver working production code, desktop controls, secure credential handling, synthetic regression tests, a reproducible naming-quality/context benchmark, updated documentation, and coherent commits. This is an implementation assignment, not a proposal, scaffolding exercise, or provider-interface-only task.

The sole AI feature is generating correct, short document names from a bounded review of the actual downloaded PDF. Support selectable OpenAI, xAI/Grok, Anthropic/Claude, and Google/Gemini providers with configurable model IDs and user-supplied credentials through one shared naming workflow.

Accuracy comes first. The secondary objective is the smallest TOTAL transmitted context that preserves useful naming reliability, counting instructions, schemas, metadata, excerpts, images, and every attempt. Neither tiny prompts that misname filings nor pervasive fallback disguised as accuracy satisfy the issue.

Do not implement case watching, summarization, legal analysis, chat, embeddings, RAG, agents, full-document processing, a mandatory server, an installer, or an application updater.

## 1. Inspect, plan, and continue through completion

Read the live issue and comments, applicable `AGENTS.md`/agent instructions if present, `CONTRIBUTING.md`, and relevant current documentation and code. Start with:

- `README.md`, `docs/development.md`, `docs/testing.md`;
- `docs/filesystem-download-service.md`, `docs/document-fetch-contract.md`, and `docs/desktop-architecture.md`;
- `arcourt/`, especially `DownloadService`, staging/publication, naming, receipts, manifests, and verified-repeat detection;
- `desktop/internal/app`, settings, Wails bindings, and `desktop/frontend/`;
- both Go modules, lockfiles, existing checks, and packaging scripts.

Confirm branch, HEAD, worktree changes, operating system, and available tools. Preserve unrelated work and newer implementation. Do not reset, clean, overwrite, or silently stash somebody else's changes. Use or create an appropriately scoped feature branch without disrupting active work.

Retain this prompt and a concise living implementation plan using existing repository conventions. Map every issue acceptance criterion to implementation and verification evidence. Make reasonable reversible decisions without routine approval requests. Work through implementation, tests, diagnosis, repair, integration review, and commits rather than stopping after the first milestone.

Preserve the root Go/core/CLI module and separate desktop module, its in-repository replacement, `GOWORK=off`, and all relevant lockfiles. Follow current checked-in toolchain requirements, not versions remembered from older projects. Keep network, PDF, naming, and filesystem decisions in Go; keep the Wails frontend thin. Do not introduce a frontend framework or runtime merely to access an AI SDK.

Existing documentation describing the product as having no AI processing or exclusively court-directed traffic must be deliberately revised for this optional feature. Those historical descriptions are not a reason to omit issue #8. All unrelated privacy and architecture boundaries remain intact.

## 2. Evaluate and select the connector before implementing it

Evaluate current maintained open-source unified Go connectors first. Use current official documentation, actual package source, release history, and a small compilable spike, not marketing claims or remembered APIs. Examine more than one plausible package. Genkit Go, GoAI, Jetify's Go AI SDK, and LangChainGo are possible research starting points, not preapproved choices.

Record a concise comparison covering actual support for all four requested providers; required text and optional image inputs; model configuration; usage reporting; transport injection; deadlines/cancellation; hidden retries, tracing, caching, and provider failover; license and redistribution obligations; Windows/Go compatibility; and dependency/binary footprint. Distinguish native support, supported compatibility endpoints, and missing functionality.

Prefer a suitable in-process unified package. If none meets the requirements, compare an explicitly optional open-source gateway against a small custom Go adapter layer. Explain concrete reasons before choosing custom code. Do not make a hosted intermediary or a second installed runtime the default merely for developer convenience. Any gateway must be clearly identified, deliberately configured, and subject to its own disclosure/credential boundary.

Implement real request/response paths for all four providers. Do not label four dropdown entries backed by one unverified request format as four-provider support. Verify provider-specific authentication, endpoint shape, model parameters, output limits, refusal/error handling, and usage fields. Keep the application-facing naming boundary narrow; avoid building a general AI framework.

Pin dependencies, review transitive changes, and update applicable third-party notices. Preserve core builds and ordinary downloading without AI configuration.

## 3. Build bounded local PDF inspection

Inspect the completed, locally validated PDF. A docket description is not a substitute for examining document contents. Do not perform AI work during case preview or for unselected, unavailable, incomplete, invalid, or already-verified downloads.

Select informative text locally before transmission. Prefer the first-page title/heading and the smallest nearby passage establishing the document type and material qualifiers. Handle wrapped headings, filing stamps, repeated headers, caption tables, and noisy extraction order. Remove irrelevant caption/address/signature boilerplate without losing the filing party's role or other naming distinctions. Do not blindly take the first N characters and call that title extraction.

Create a deterministic extraction/selection pipeline with explicit page, character/byte, input-budget, time, and memory bounds. Start with a maximum of two opening pages and compare targeted excerpts around 512 and 1,024 characters against a larger bounded baseline around 4,096 characters. These are starting experiment settings, not asserted optima. Choose and document final defaults from evidence, retaining finite safety ceilings.

Never extract every page and truncate afterward. Structural PDF reads needed to locate approved pages are distinct from extracting full-document content. Bound decompression and extraction output, handle malformed/encrypted PDFs and unusable text safely, and avoid making the optional parser a new availability hazard. Prefer a portable dependency; document any native extraction requirement honestly.

Enforce transmission limits at the final application-controlled provider boundary, including SDK-added content. Count the complete input, not only the excerpt. Use an appropriate local tokenizer where available or explicitly labeled conservative estimates with independently enforced page/character/byte limits. Never present an estimate as an exact provider token count or claim a guarantee the implementation cannot enforce.

No full PDF, source URL, signed download URL, unrelated case information, document history, or other document may be attached or fetched by the provider. Do not use a remote file-upload API or URL-fetch tool.

For scanned/image-only documents, implement a bounded opening-page image or local OCR path only when it fits the chosen portable architecture. Bound page count, dimensions, encoded size, and image-token assumptions; send no whole PDF. Otherwise implement an explicit, tested `image_only`/unsupported deterministic fallback. This is an allowed outcome, but report scanned-document coverage separately and in the overall denominator. Do not present scan fallback as successful content naming.

A zero-call local-title path is permitted when an explicit title can be reliably recognized. Validate it independently and report it separately from AI-generated names; do not use its success to imply that provider quality was measured.

## 4. Generate grounded short labels with bounded calls

Use a concise stateless naming request. Ask only for a short label or an unambiguous abstention, using the smallest reliably parseable response contract. No conversation history, rationale, elaborate taxonomy, tools, web access, or general-purpose reasoning workflow. Treat document text as untrusted data, including instructions embedded in a filing.

Prefer the actual document title. Preserve supported distinctions including motion versus order, proposed versus entered order, amended and ordinal-amended filings, response versus reply, granting versus denying where stated, and filing-party role where useful. Do not infer the movant from the first party listed in the caption or rename a response after the motion it quotes.

Start with a short-label budget of approximately 64 ASCII bytes within the existing filename convention, adjusting only with documented justification. The entire basename must still satisfy the existing 120-byte sanitization limit, with room for date, document identity, collision suffix, and `.pdf`. Shortening must not silently discard a material qualifier. Prefer role labels over unnecessary personal names.

Use the smallest adequate excerpt first. Expand only when local signals or a valid insufficient-context result justify additional text. Clear-title documents should ordinarily require no more than one provider call. Limit automatic transmissions to at most two per document, including any retry or expansion; disable or account for SDK retries so nested retry loops cannot exceed the ceiling. Avoid sending conversation history with an expanded request.

Define bounded request/output limits and a total naming deadline. Respect cancellation and bounded Retry-After behavior. Do not retry invalid credentials, unsupported models, policy refusals, or unusable extraction indefinitely. Suppress repeated configuration/authentication failures for the remainder of a batch while allowing ordinary downloads to continue.

Never silently switch providers, models, intermediaries, or text/image modes outside the user's disclosed configuration. A model's confidence score is not a reliability test. Reject ambiguous, unsupported, malformed, empty, excessively long, or unsafe results. Conservative abstention retains the ordinary deterministic filename.

## 5. Preserve filesystem integrity, recovery, and rerun behavior

Integrate naming into the real download lifecycle, not an unrelated post-download rename script. Inspect the current staging, independent PDF verification, recovery receipt, hard-link publication, and manifest commit sequence before choosing the integration point.

Prefer selecting the final label before existing no-overwrite publication when that preserves failure and cancellation guarantees. If a different arrangement is necessary, explicitly design and test its recoverable state transitions. Never replace hard-link/no-overwrite publication with an ordinary overwrite-capable rename or an exposed partial copy.

The AI result is a label, never a path. Reuse and, only where necessary, strengthen existing filename sanitization and case-insensitive collision handling. Cover traversal, absolute/UNC/device paths, separators, Windows reserved names, alternate data streams, trailing dots/spaces, control characters, deceptive Unicode, unexpected extensions, and length exhaustion. Preserve date/identity conventions and `.pdf`.

Original PDF bytes must remain unchanged. Naming errors/timeouts are separate from download errors: a successfully saved document with a fallback name remains successfully saved. Expose a controlled naming status/reason without turning optional enrichment failure into `ErrDocumentsIncomplete`.

Cancellation during naming must promptly stop the AI request and pending work, preserve already published PDFs, and not discard an otherwise complete validated PDF merely because its optional label is unfinished. Document and test the exact publication/cancellation boundary, maintaining bounded local finalization and accurate overall cancellation reporting. Genuine disk/manifest failures must remain genuine failures.

Persist only the minimal naming provenance needed for behavior and diagnostics, such as source/method, chosen label, controlled fallback reason, strategy version, and usage metadata where justified. Do not persist excerpts, images, prompts, raw provider replies/errors, secrets, or request URLs. Preserve original docket descriptions rather than overwriting them with the AI label.

Handle older manifests/receipts explicitly and non-destructively. Do not silently rewrite an incompatible schema. Recovery must restore the correct filename, integrity information, and naming outcome without another naming call.

Verified reruns, including files previously saved with fallback names, must skip both downloading and paid naming. Changing AI settings must not automatically rename historical files or bill for them again. Do not deduplicate by label or weaken existing identity/hash verification. Test collisions, modified files, duplicate selections, manifest failures, and interruption windows. Document any unavoidable uncertain remote-completion window honestly; do not claim exactly-once billing or blindly replay uncertain requests.

## 6. Deliver usable settings, secure credentials, and clear disclosure

Ship the feature in the desktop application and reusable Go core. Keep CLI behavior backward compatible and AI-free by default. AI naming must be off by default, and preview/download must remain usable with no keys or configuration.

Add a clear enable/disable control, the four provider choices, editable model ID, credential configured/missing status, save/replace/remove credential actions, and concise explanation of limits/fallback. Validate configuration without requiring a paid request. Any connection-test action must be explicit, use synthetic input, and explain potential charges.

Use a suitable Windows OS-backed secret store, such as Credential Manager or a properly implemented current-user DPAPI design, after verifying current platform guidance. Ordinary settings may contain only non-secret configuration and a credential reference. Do not persist keys in preferences, manifests, logs, source, frontend storage, exported diagnostics, or command-line arguments.

Provide a narrow backend credential-write method. Never return a stored key through normal Wails snapshots or getters. Clear entered secrets from UI state after submission. Use an injectable secret-store boundary for tests; unavailable secure storage must not trigger plaintext persistence. Do not discover or reuse credentials from unrelated projects or browser profiles.

Before first transmission, explain the selected provider and any intermediary, the bounded document material leaving the computer, possible sensitive content despite truncation, API charges, and fallback behavior. Do not promise confidentiality, retention settings, or no-training treatment that the application cannot establish. Reflect supported provider privacy controls accurately. Endpoint/intermediary changes must not inherit consent for a different recipient.

AI transport must not inherit court cookies, browser sessions, signed URLs, or authentication. Constrain destination and redirect behavior and prevent credential forwarding to a different host. Do not weaken court-download network protections to accommodate an AI client. Disable optional SDK telemetry, content tracing, automatic routing, and content caching that conflicts with the disclosed flow.

Snapshot non-secret naming configuration at job start so settings changes cannot retarget an in-flight batch. Preserve current generation, busy-state, shutdown, and stale-event protections. Render provider/document text safely. Display useful non-sensitive naming outcomes without flooding ordinary download results with raw errors.

## 7. Prove context efficiency without disguising coverage loss

Build valid synthetic PDFs with actual extractable text and valid image-only examples, not only PDF-header screening fixtures. Include motions, responses, replies, orders, proposed orders, amended pleadings, affidavits, briefs, notices, exhibits, long captions, overlapping/quoted titles, first-page noise, second-page headings, missing titles, deliberately ambiguous documents, malformed/encrypted files, scans, and embedded prompt-injection text. Use enough independent examples and layout variants to make aggregate percentages meaningful, with a held-out set not used to tune rules.

Create one reproducible benchmark entry point with offline fixture/replay mode and separately opt-in live synthetic mode. Exercise the production extractor, request construction, normalization, and fallback decisions, rather than a benchmark-only approximation. Keep expected labels/acceptable variants and material-error definitions explicit. Avoid a live LLM judge when deterministic labels and human-readable failure review suffice.

Compare at least the larger bounded baseline, targeted small excerpts, and targeted excerpts with bounded expansion. Keep provider/model and generation settings fixed within each comparison. Record source revision, corpus/strategy version, configuration, and date.

For each strategy and tested provider/model, report counts and denominators for correct useful labels, incorrect labels, accepted-name precision, correct-label coverage, fallbacks by reason, local-title versus AI routes, and first-call versus expanded results. Report supported-readable, ambiguous, scanned, and overall cohorts separately. Include total input tokens per document across ALL attempts, prompt/schema/metadata overhead, image usage where applicable, output tokens, call counts, and latency. Distinguish exact provider-reported usage, local estimates, cached-token categories, and missing usage on failed requests. Unknown usage is not zero.

Declare quality targets before measuring. Suggested initial floors are at least 99% accepted-name precision and 90% correct-label coverage on nameable supported-text fixtures, with no material type/party/amendment errors in the clear-title regression set. Define a narrow non-inferiority margin against the larger baseline before selecting the smaller strategy. These are engineering targets, not claimed measurements or universal guarantees. Do not lower targets after failure simply to mark the issue complete; fix the approach or report the failed gate. Report sample sizes and do not overstate statistical certainty.

Select the lowest aggregate-input strategy that meets the declared quality/coverage targets. Account for the cost of unsuccessful first attempts and expansion. A strategy that saves tokens per request but uses more total input per document has not met the optimization objective. Provider/model settings not actually measured must not inherit another model's quality claim.

Ordinary tests and CI must make zero live paid calls and use no court/client documents. Live synthetic evaluation requires applicable explicit authorization and intentionally supplied credentials, plus finite call/token or monetary limits enforced by the harness. This prompt does not invent a spending allowance. Keys merely existing on the machine are not permission.

If authorized live access is unavailable, finish every production integration and the runnable benchmark, execute offline checks, and report empirical provider-quality/context selection as pending. Mock outputs prove contracts, not real model accuracy. Missing live access is not permission to leave provider stubs, fabricate measurements, or abandon implementation.

## 8. Test, repair, document, and commit

Add focused offline tests for all four real provider paths through injected transports: authentication/request shape, bounded payloads, response parsing, usage, refusals, bad keys/models, rate limits, timeout, cancellation, oversized responses, hidden retries, and no cross-provider fallback. Assert that secrets, excluded PDF material, and unselected documents never reach provider payloads or diagnostics.

Add extraction, budget, naming, adversarial-output, scan-fallback, collision, byte-integrity, legacy-state, recovery, repeat-run, and cancellation regression tests. Verify zero provider calls when disabled, during preview, on verified repeats, and when safe extraction fails. Test desktop consent/settings/secret-state behavior, frontend rendering, and successful download reporting with naming fallback. Preserve current CLI regression coverage.

Run the repository's current applicable checks and fix introduced failures. Expect to use `scripts/check.ps1`, relevant browser/desktop fixtures, `scripts/build-desktop.ps1`, race checks where supported, vulnerability checks for both Go modules, and local packaging checks affected by new dependencies. Root `go test ./...` alone does not cover the desktop module. Record exact commands, versions, results, and skips. Do not substitute cross-compilation for native Windows execution or claim a human GUI walkthrough occurred when it did not. Do not weaken checks to make the feature pass.

Update README, contribution/privacy descriptions, desktop and filesystem contracts, testing instructions, relevant CLI documentation, and third-party notices. Add focused feature documentation covering dependency selection, data flow, credentials, limits, scan support, cancellation, naming statuses, manifests/recovery, model configuration, benchmark commands, measured results, and outstanding evidence. Keep documentation proportional and consistent; do not create a separate governance framework.

Perform a fresh review of the integrated diff, independently when available. Prioritize unexpected data disclosure, credential leakage, lost/overwritten files, manifest inconsistency, hidden paid retries, excessive fallback, and unsupported quality claims. Repair findings and rerun affected checks.

Commit coherent, scoped changes and retain the implementation prompt/plan. Do not force-push, bypass protection, merge, publish a release, alter signing policy, or close issue #8. Unless separately authorized, leave remote writes to the operator and prepare a ready-to-use PR description referencing #8. Do not claim full acceptance while empirical or native verification gates remain pending.

Finish with the implemented user workflow; connector/dependency decision; actual limits and scan behavior; commands/results and benchmark evidence; commits and remaining worktree changes; an acceptance-criterion matrix marked verified, implemented-but-unverified, or blocked; and exact remaining operator commands for any unavailable gate. Distinguish complete software implementation from evidence still requiring authorized live access or an interactive Windows session.