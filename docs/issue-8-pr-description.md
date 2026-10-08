# PR title

Add optional bounded AI PDF document naming (issue #8)

# PR description

## Summary

- Add a reusable Go naming path after independent staged-PDF validation and before receipt/no-overwrite publication. It inspects at most two opening pages locally, sends a bounded title-centered text excerpt, and preserves the original bytes, docket description, recovery, and verified-repeat behavior.
- Support OpenAI, xAI, Anthropic, and Google through pinned GoAI request/response paths with direct HTTPS destinations, bounded complete payloads, no SDK retries, and controlled fallbacks. Image-only and unreadable PDFs keep deterministic filenames.
- Add default-off desktop controls, exact-recipient consent, model IDs, per-provider Windows Credential Manager keys, credential status/actions, and per-document naming outcomes. The CLI remains AI-free by default.
- Add synthetic extraction/provider/filesystem/desktop regressions, a reproducible offline replay benchmark and opt-in budgeted live synthetic mode, dependency/license notices, user documentation, and release payload checks.

## Verification

- Passed `./scripts/check.ps1`, `./scripts/build-desktop.ps1`, both module `check-vulnerabilities.ps1` runs, `./scripts/generate-notices.ps1 -Check`, `./scripts/benchmark-naming.ps1`, and `./scripts/package-windows.ps1` on Windows/amd64. Packaging used reviewed Go 1.26.8; development checks used Go 1.27.1.
- Passed `go test -race ./...` in both Go modules with `CGO_ENABLED=1` and MSYS2 UCRT64 GCC 16.2.0 on Windows/amd64.
- Passed `./scripts/check-fixtures.ps1 -Desktop` with installed Chrome and with default Edge on Windows 11: browser/CLI fixtures, native Wails GUI fixture, and native startup. The GUI fixture requires an actual case-header mismatch before crediting that test. Edge's former process-only compatibility-layer relaunch is handled in the Edge child environment; the startup fixture retries cleanup of its own WebView2 cache.
- Offline `synthetic-court-v3` replay: 59 PDFs (43 readable, 8 ambiguous, 8 scans; 14 held out). Baseline 43/43 readable correct in 51 calls and 29,286 serialized request bytes; small 42/43 in 51 calls and 28,581 bytes; small with expansion 43/43 in 52 calls and 29,910 bytes. All accepted replay labels were correct by deterministic fixture comparison; ambiguous and scanned cases fell back. These are replay-contract results, not live model quality.
- Operator-run OpenAI `gpt-4.1-mini` synthetic live comparison: all 59 cases completed for each strategy, with 154 transmitted calls and provider usage for each. Baseline: 17/43 readable correct, 94.44% accepted-name precision, 29,592 complete request bytes. Small: 20/43, 95.24%, 28,887 bytes. Production small plus expansion: 15/43, 88.24%, 30,222 bytes. All 8 ambiguous cases abstained and all 8 scans fell back per strategy.

## Open evidence

- Offline small loses 2.326 percentage points of readable coverage versus baseline, exceeding the declared two-point non-inferiority margin; expansion restores that label but costs 624 more serialized bytes than baseline. In the OpenAI live run, all three strategies fail the declared 99% precision and 90% readable-coverage gates; production expansion also trails baseline by 4.65 percentage points. No minimum-context winner was selected. The dominant `unsafe_result` fallback needs bounded synthetic-only diagnosis before a narrow change and fresh held-out evaluation. Other provider/model quality and performance on court/client PDFs remain unmeasured; the live run sent only generated synthetic excerpts.
- A human production-GUI walkthrough and current authorized court check were not claimed. The OpenAI live result is synthetic and does not establish reliability on court/client PDFs.

See [the naming guide](ai-document-naming.md), [connector evaluation](ai-connector-evaluation.md), and [implementation plan](issue-8-plan.md) for limits, design choices, and evidence. References #8; leave the issue open until the remaining empirical and native-environment gates are reviewed.
