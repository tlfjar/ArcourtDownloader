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
- Passed `./scripts/check-fixtures.ps1 -Desktop` with `ARCOURT_BROWSER_EXECUTABLE` set to installed Chrome: browser/CLI fixtures, native Wails GUI fixture, and native startup. The GUI fixture requires an actual case-header mismatch before crediting that test.
- Offline `synthetic-court-v3` replay: 59 PDFs (43 readable, 8 ambiguous, 8 scans; 14 held out). Baseline 43/43 readable correct in 51 calls and 29,286 serialized request bytes; small 42/43 in 51 calls and 28,581 bytes; small with expansion 43/43 in 52 calls and 29,910 bytes. All accepted replay labels were correct by deterministic fixture comparison; ambiguous and scanned cases fell back. These are replay-contract results, not live model quality.

## Open evidence

- The small strategy loses 2.326 percentage points of readable coverage versus baseline, exceeding the declared two-point non-inferiority margin. Expansion restores that label but costs 624 more serialized bytes than baseline. A minimum-total-context production choice and actual provider naming quality remain unverified until explicitly authorized, budgeted live synthetic evaluations; no paid call or court/client document was used here.
- Edge headless fixtures fail at browser startup on this installation (`chrome failed to start` with empty stderr). The same native GUI workflow passed with installed Chrome. A human production-GUI walkthrough was not claimed. Race tests were unavailable (`CGO_ENABLED=0`, no GCC).

See [the naming guide](ai-document-naming.md), [connector evaluation](ai-connector-evaluation.md), and [implementation plan](issue-8-plan.md) for limits, design choices, and evidence. References #8; leave the issue open until the remaining empirical and native-environment gates are reviewed.
