# Security reporting

Report suspected vulnerabilities privately through
[Report a vulnerability](https://github.com/tlfjar/ArcourtDownloader/security/advisories/new)
in the planned public repository, `tlfjar/ArcourtDownloader`. This route requires
GitHub private vulnerability reporting to be enabled. If it is unavailable, do
not post vulnerability details publicly; a public issue may request that the
maintainer enable private reporting without describing the vulnerability.
No private email address or response-time guarantee is provided.

Include the affected source revision or build identity, Windows/browser/runtime
versions, expected behavior, impact, and a minimal synthetic reproduction.
Use fabricated data and loopback fixtures. Do not send court PDFs, client
documents, credentials, signed URLs, browser profiles, or unredacted client
screenshots, even through the private reporting form.

Public issues must exclude all of those materials. Ordinary software bugs and
usage questions belong in [support](SUPPORT.md). Please coordinate disclosure
privately while a report is investigated and a fix can be prepared.

There is no published supported release series yet. Reports against current
source are welcome; older development snapshots have no maintenance commitment.
The app does not bypass court access controls, import personal browser sessions,
or guarantee the completeness of online records. See the
[download contract](docs/document-fetch-contract.md) for its actual boundaries.
