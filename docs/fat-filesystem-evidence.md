# Issue #15 verification evidence

Implementation based on main `b3cacf9` (2026-10-12). Synthetic fixtures contain no
court/client documents and make no paid AI or court requests.

## Recorded checks (2026-10-20)

| Check | Result |
| --- | --- |
| Linux Go 1.26.9: `go test ./... -count=1 -timeout=180s` | PASS |
| Linux consuming-backend state machine: `go test ./arcourt -run TestMove -count=1 -timeout=180s` | PASS |
| Windows amd64 package cross-compilation: `GOOS=windows GOARCH=amd64 go test -c ./arcourt` | PASS; compilation only |
| Linux `go test -race ./... -count=1 -timeout=180s` | PASS |
| Desktop controller tests, race tests, and vet | PASS |
| Root `go vet ./...`, `go build ./...`, `git diff --check` | PASS |
| Native Windows NTFS tests | Pending hosted CI |
| Native Windows FAT32 VHD tests | PASS: [job 118942593987](https://github.com/tlfjar/ArcourtDownloader/actions/runs/39514286656/job/118942593987) |
| Native Windows exFAT VHD tests | PASS: [job 118942594020](https://github.com/tlfjar/ArcourtDownloader/actions/runs/39514286656/job/118942594020) |
| Physical USB/SD/external media and removal/power-loss tests | NOT RUN; no hardware available in Linux workspace |
| Interactive Windows desktop walkthrough | NOT RUN; no native GUI session available |

Native-volume results above are for implementation commit `a062974116016d848b36285165b84992c37b95ab`,
Go 1.26.9 windows/amd64, Windows Server 2025 (10.0.26100), and freshly formatted
128 MiB VHDs. Both jobs verified the actual filesystem and passed every selected
test with no skips. Additional consumed-name, naming-cancellation, sharing-handle,
and symlink-source tests are being added; their native results must be recorded
separately. These results establish the tested Windows-driver/VHD scope, not
universal removable-device or arbitrary power-loss support.

## Behavioral boundaries

NTFS retains hard-link publication and version-1 same-file-witness recovery.
FAT32/exFAT use handle-relative no-replace rename after a capability probe.
Committed repeats verify bytes; uncommitted interrupted publication is preserved
and stops for review rather than being adopted from matching hashes. The new
receipt version fails closed in older binaries. FAT32's 4 GiB file limit does not
expand the existing 1 GiB service / default 64 MiB transfer limits.

The native-volume harness and CI jobs exercise real Windows filesystem drivers
on disposable VHDs. Even a passing VHD suite cannot establish arbitrary power-loss
survival or the behavior of all removable-device controllers. File synchronization
is retained; no stronger durability promise is made.

## Dependency gate

The first root Windows vulnerability scan identified GO-2026-5024 at the newly
used `windows.NewNTUnicodeString` call in x/sys v0.42.0. The new backend already
bounds basenames to 160 bytes, below the overflow condition, but the root module
is upgraded to v0.44.0, the first fixed version. The desktop retains v0.46.0.
The license text is byte-identical; the catalog checksum/version and generated
notices are updated. See the [Go advisory](https://pkg.go.dev/vuln/GO-2026-5024).
Revalidation of the final dependency set is required; the initial failure is not
being waived or hidden.
