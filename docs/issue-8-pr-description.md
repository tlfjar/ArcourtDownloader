# PR title

Add optional bounded AI PDF document naming (issue #8)

# PR description

## Summary

- Add a reusable Go naming path after independent staged-PDF validation and before receipt/no-overwrite publication. It inspects at most two opening pages locally, sends a bounded title-centered text excerpt, and preserves the original bytes, docket description, recovery, and verified-repeat behavior.
- Support OpenAI, xAI, Anthropic, and Google through pinned GoAI request/response paths with direct HTTPS destinations, bounded complete payloads, no SDK retries, and controlled fallbacks. Image-only and unreadable PDFs keep deterministic filenames.
- Validate every model reply with `title-excerpt-v3`, which requires the selected heading material words to survive into the label in heading order. Anything else, including abstention, unsafe characters, a dropped material word, or a rearranged title, keeps the deterministic docket-derived filename.
- Add default-off desktop controls, exact-recipient consent, model IDs, per-provider Windows Credential Manager keys, credential status/actions, and per-document naming outcomes. The consent disclosure now states whether the selected provider and model have a completed naming-quality evaluation. The CLI remains AI-free by default.
- Add synthetic extraction/provider/filesystem/desktop regressions, a reproducible offline replay benchmark and opt-in budgeted live synthetic mode, dependency/license notices, user documentation, and release payload checks.

## Naming quality evidence

Quality gates were declared before measurement: at least 99% accepted-name precision, at least 90% correct-label coverage on nameable supported-text fixtures, zero material type/party/amendment errors, and no more than two percentage points of coverage loss against the same-model bounded baseline.

The final held-out run (`build/naming-benchmark-openai-luna6-v6-live.json`, clean source revision `90fcd94`, OpenAI `gpt-6-luna`) met every gate in all three request strategies:

| Strategy | Correct readable labels | Accepted-name precision | Material errors | Calls | Complete request bytes | Gate |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| Bounded 4,096-character baseline | 31/32 (96.88%) | 100% | 0 | 40 | 28,258 | Met |
| Targeted 512-character excerpt | 31/32 (96.88%) | 100% | 0 | 40 | 28,258 | Met |
| Targeted excerpt with expansion (production) | 31/32 (96.88%) | 100% | 0 | 40 | 28,258 | Met |

Each strategy completed all 48 cases, abstained on all 8 ambiguous documents, and fell back on all 8 image-only scans. There is no coverage spread between the baseline and the production strategy, so the two-point non-inferiority margin is satisfied. The single readable miss is identical across strategies: the model answered `ABSTAIN` on `EXHIBIT D`, so that PDF keeps its docket-derived filename. A refusal to guess is the intended behavior, not a naming error.

## How the validator reached that result

The earlier `title-excerpt-v1` validator checked only a hardcoded qualifier list, so a paraphrase could drop a material subject word and still be accepted. `title-excerpt-v2` required every material heading word to survive. Its first held-out run on `synthetic-court-v5` recorded zero material errors but exposed a real defect: two strategies accepted labels that kept every material word while rearranging them, turning `Brief in Reply to Opposition` into `Reply Brief to Opposition` and `Entered Order Confirming Sale` into `Order Confirming Sale (Entered)`. Each names a different filing than the page does.

`title-excerpt-v3` closes that gap by matching the heading material words as an ordered subsequence of the label, with articles, prepositions, and a possessive filing-party role still optional. A confirming v5 rerun raised precision to 100% in all three strategies; the reordered labels are now fallbacks.

Each time a run output motivated a validator change, that corpus was retired as held-out evidence and a new one was authored and committed before the next measurement. v4 was retired for v2, and v5 for v3. A test enforces that the v6 corpus restates no earlier corpus label.

## Provider status

| Provider | Connector | Live naming quality |
| --- | --- | --- |
| OpenAI | Implemented; injected request/response tests pass | Evaluated. `gpt-6-luna` met every gate on held-out `synthetic-court-v6`. |
| xAI | Implemented; injected request/response tests pass | Experimental: unmeasured. No API key was available. |
| Anthropic | Implemented; injected request/response tests pass | Experimental: unmeasured. No API key was available. |
| Google | Implemented; injected request/response tests pass | Experimental: unmeasured. No API key was available. |

All four connectors share the same local extraction limits, the same label validator, the same filename sanitization, and the same deterministic fallback, so an unmeasured provider cannot bypass those protections; the realistic risk of selecting one is a lower rate of useful labels, not an unsafe filename. There is no automatic cross-provider fallback. The README, the naming guide, and the desktop consent disclosure all state which provider and model have been evaluated, and the naming guide documents the procedure for qualifying another one.

## Verification

- Passed `./scripts/check.ps1`, `./scripts/build-desktop.ps1`, both module `check-vulnerabilities.ps1` runs, `./scripts/generate-notices.ps1 -Check`, `./scripts/benchmark-naming.ps1`, and `./scripts/package-windows.ps1` on Windows/amd64. Packaging used reviewed Go 1.26.8; development checks used Go 1.27.1.
- Passed `go test -race ./...` in both Go modules with `CGO_ENABLED=1` and MSYS2 UCRT64 GCC 16.2.0 on Windows/amd64.
- Passed `./scripts/check-fixtures.ps1 -Desktop` with installed Chrome and with default Edge on Windows 11: browser/CLI fixtures, native Wails GUI fixture, and native startup. The GUI fixture requires an actual case-header mismatch before crediting that test.
- Offline replays validate extraction, request-building, and scoring contracts only: `synthetic-court-v3` 43/43 readable per strategy; `synthetic-court-v5` 32/32; `synthetic-court-v6` 32/32 at 26,898 complete request bytes.
- Live synthetic runs used only generated fixtures, finite call and byte caps, and a key supplied at run time that is never written to a report or into the repository.

## Open evidence and limits

- The live results are single runs of one OpenAI model against generated fixtures. They do not establish accuracy on real court or client PDFs, and they do not transfer to other providers or model IDs. The one-case differences between strategies stay within ordinary model variation and are not evidence of an excerpt-size effect; no strategy is claimed as a general minimum-context winner, and the benchmark baseline is not an adopted production default.
- xAI, Anthropic, and Google naming quality is unmeasured because no API keys were available. Those selections ship as experimental.
- A human production-GUI walkthrough has not been performed; GUI coverage is the automated Wails fixture and startup check. No authorized live court-site check is claimed.
- Earlier `title-excerpt-v1` and `title-excerpt-v2` runs that failed or exposed defects are retained in the naming guide and plan so the evidence trail shows what changed and why.

See [the naming guide](ai-document-naming.md), [connector evaluation](ai-connector-evaluation.md), and [implementation plan](issue-8-plan.md) for limits, design choices, and the full evidence history. References #8; leave the issue open until the remaining native-environment and human-review gates are confirmed.
