# Issue #15 verification evidence

Implementation based on main `b3cacf9` (2026-10-12). Synthetic fixtures contain no
court/client documents and make no paid AI or court requests.

## Recorded checks (2026-10-20)

The tested implementation revision is
`92714fc1a569b5180046dc2200d26b78ab5187d4`. Subsequent evidence/CLI-guide edits do
not change executable code, tests, dependencies, or workflows.

| Check | Result |
| --- | --- |
| Linux Go 1.26.9: `go test ./... -count=1 -timeout=180s` | PASS |
| Linux `go test -race ./... -count=1 -timeout=180s` | PASS |
| Linux consuming-backend state machine: `go test ./arcourt -run TestMove -count=1 -timeout=180s` | PASS |
| Desktop controller tests, race tests, and vet | PASS |
| Root `go mod verify`, `go vet ./...`, `go build ./...`, `git diff --check` | PASS |
| Windows amd64 package cross-compilation: `GOOS=windows GOARCH=amd64 go test -c ./arcourt` | PASS; compilation only |
| Native Windows NTFS/core/CLI/frontend/controller checks | PASS: [job 118943873642](https://github.com/tlfjar/ArcourtDownloader/actions/runs/39514678214/job/118943873642) |
| Native Windows FAT32 VHD tests | PASS: [job 118943873618](https://github.com/tlfjar/ArcourtDownloader/actions/runs/39514678214/job/118943873618) |
| Native Windows exFAT VHD tests | PASS: [job 118943873515](https://github.com/tlfjar/ArcourtDownloader/actions/runs/39514678214/job/118943873515) |
| Native Windows browser DOM/CLI fixtures | PASS: [job 118943873576](https://github.com/tlfjar/ArcourtDownloader/actions/runs/39514678214/job/118943873576) |
| Windows-targeted vulnerability scans, root and desktop | PASS: [root](https://github.com/tlfjar/ArcourtDownloader/actions/runs/39514678214/job/118943873606), [desktop](https://github.com/tlfjar/ArcourtDownloader/actions/runs/39514678214/job/118943873471) |
| Windows local unsigned package, rejection checks, and isolated tagged-release rehearsal | PASS: [packaging job](https://github.com/tlfjar/ArcourtDownloader/actions/runs/39514678214/job/118943873199) |
| Workflow policy/rehearsals | PASS: [job 118943873721](https://github.com/tlfjar/ArcourtDownloader/actions/runs/39514678214/job/118943873721) |
| Physical USB/SD/external media and removal/power-loss tests | NOT RUN; no physical hardware available |
| Interactive Windows 11 desktop walkthrough | NOT RUN; no native GUI session available |

Native-volume tests used Go 1.26.9 windows/amd64, Windows Server 2025
(10.0.26100), and freshly formatted 128 MiB VHDs. Both jobs verified the actual
filesystem and reported 42 passing test/subtest entries. Each explicitly skipped
`TestMoveWindowsSymlinkSource` because FAT32/exFAT cannot create the symlink
fixture (`Incorrect function`). Traversal rejection tests ran; the ordinary
Windows suite also retains existing NTFS symlink/junction defenses.

The suites cover staging, PDF verification, cancellation during writing and after
receipt creation, deterministic naming fallback and naming cancellation, PDF and
metadata collisions, concurrent publishers/services, repeat verification,
modified/unrelated files, disk errors, and subprocess termination before/after
publication and around manifest persistence. They also check that a file newly
created at a consumed staging basename is preserved and that an explicitly
non-delete-sharing Windows handle produces an access/sharing error rather than
an unsupported-filesystem diagnosis.

These results establish the tested Windows-driver/VHD scope, not universal
removable-device or arbitrary power-loss support. Logs are retained with the
linked CI jobs and their `filesystem-FAT32-*` / `filesystem-exFAT-*` artifacts.

## Behavioral boundaries

NTFS retains hard-link publication and version-1 same-file-witness recovery.
FAT32/exFAT use handle-relative no-replace rename after a capability probe.
Committed repeats verify bytes; uncommitted interrupted publication is preserved
and stops for review rather than being adopted from matching hashes. The new
receipt version fails closed in older binaries. FAT32's 4 GiB file limit does not
expand the existing 1 GiB service / default 64 MiB transfer limits.

File synchronization is retained; no stronger durability promise is made.
Windows 11 and physical-media walkthroughs remain useful release acceptance
checks; disconnecting media or power-loss survival is outside the existing
ordinary-I/O/process-interruption contract.

## Corrections made during verification

The first root Windows vulnerability scan identified
[GO-2026-5024](https://pkg.go.dev/vuln/GO-2026-5024) at the newly used
`windows.NewNTUnicodeString` call in x/sys v0.42.0. The backend bounds basenames
to 160 bytes, below the overflow condition, but the root module was upgraded to
v0.44.0, the first fixed version. The desktop retains v0.46.0. The license text
is byte-identical; the catalog checksum/version and generated notices were
updated and packaging checked. Final vulnerability scans pass; the initial
failure was not waived.

An added sharing-conflict test initially used `os.Root.Open`, which permits
delete sharing on this Go version and therefore did not block rename. It now
opens an explicit Windows handle denying delete sharing. The corrected test
passes on both native FAT32 and exFAT volumes; no production relaxation was made.
