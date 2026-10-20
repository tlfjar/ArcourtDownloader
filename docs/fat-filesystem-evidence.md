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
| Desktop `go test ./internal/app -count=1 -timeout=180s` | PASS |
| Root `go vet ./...`, `go build ./...`, `git diff --check` | PASS |
| Native Windows NTFS tests | Pending hosted CI |
| Native Windows FAT32/exFAT VHD tests | Pending hosted CI; required jobs added |
| Physical USB/SD/external media and removal/power-loss tests | NOT RUN; no hardware available in Linux workspace |
| Interactive Windows desktop walkthrough | NOT RUN; no native GUI session available |

The implementation is a candidate pending native-volume evidence. A cross-build
or Unix test result must not be described as verified FAT32/exFAT support.

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
