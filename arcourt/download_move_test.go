package arcourt

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func moveFixture(t *testing.T) (*DownloadService, *localFakeFetcher, DownloadRequest) {
	t.Helper()
	s, f, req := localFixture(t, 1)
	if volume := os.Getenv("ARCOURT_TEST_VOLUME_ROOT"); volume != "" {
		dir, err := os.MkdirTemp(volume, "arcourt-test-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.RemoveAll(dir); err != nil {
				t.Error(err)
			}
		})
		req.OutputDirectory = dir
		s.diskSetup = func(d *downloadDisk) error {
			if err := d.selectPublisher(); err != nil {
				return err
			}
			if d.move == nil {
				return errors.New("volume fixture did not select the move backend")
			}
			return nil
		}
	} else {
		s.diskSetup = func(d *downloadDisk) error { d.move = testMoveNoReplace; return d.probeMove() }
	}
	return s, f, req
}

func TestMoveRepeatCollisionAndModifiedFile(t *testing.T) {
	s, f, req := moveFixture(t)
	var collision string
	s.diskBefore = func(op, name string) error {
		if op == "publish.move" && collision == "" {
			collision = filepath.Join(req.OutputDirectory, "case-60cv-2026-1", name)
			return os.WriteFile(collision, []byte("unrelated"), 0600)
		}
		return nil
	}
	r, err := s.Download(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertLocalCounts(t, r, DownloadCounts{Selected: 1, Succeeded: 1})
	if !strings.HasSuffix(r.Documents[0].Filename, "-1.pdf") {
		t.Fatal("collision suffix missing")
	}
	if data, _ := os.ReadFile(collision); string(data) != "unrelated" {
		t.Fatal("collision overwritten")
	}
	assertNoIncompleteTemps(t, r.Directory)
	repeat, err := s.Download(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertLocalCounts(t, repeat, DownloadCounts{Selected: 1, Skipped: 1})
	if len(f.calls) != 1 {
		t.Fatal("verified repeat fetched")
	}
	saved := filepath.Join(r.Directory, r.Documents[0].Filename)
	if err := os.WriteFile(saved, []byte("user modification"), 0600); err != nil {
		t.Fatal(err)
	}
	again, err := s.Download(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if again.Documents[0].Filename == r.Documents[0].Filename {
		t.Fatal("modified file reused")
	}
	if data, _ := os.ReadFile(saved); string(data) != "user modification" {
		t.Fatal("modified file overwritten")
	}
}

func TestMoveValidationCancellationAndStaging(t *testing.T) {
	for _, mode := range []string{"invalid", "canceled", "valid"} {
		t.Run(mode, func(t *testing.T) {
			s, f, req := moveFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.duringWrite = func(_ context.Context, _ string, w DocumentWriter) error {
				names, err := os.ReadDir(filepath.Join(req.OutputDirectory, "case-60cv-2026-1"))
				if err != nil {
					return err
				}
				for _, name := range names {
					if strings.HasSuffix(name.Name(), ".pdf") {
						t.Fatal("visible final before commit")
					}
				}
				data := fixturePDF
				if mode == "invalid" {
					data = "%PDF-1.7\nincomplete"
				}
				_, err = w.Write([]byte(data))
				if mode == "canceled" {
					cancel()
				}
				return err
			}
			r, err := s.Download(ctx, req, nil)
			if mode == "valid" && err != nil {
				t.Fatal(err)
			}
			if mode == "invalid" && !errors.Is(err, ErrInvalidPDF) {
				t.Fatal(err)
			}
			if mode == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if r.Documents[0].Saved != (mode == "valid") {
				t.Fatal("incorrect saved state")
			}
			assertNoIncompleteTemps(t, r.Directory)
		})
	}
}

func TestMoveNamingFallback(t *testing.T) {
	s, f, req := moveFixture(t)
	f.data = map[string]string{req.Selection[0].SourceURL: string(syntheticNamingPDF([]string{"ORDER DENYING MOTION TO DISMISS", "The motion is denied."}))}
	req.Naming = &NamingRequest{Provider: "google", Model: "gemini-test", APIKey: "TEST-SECRET"}
	client := &fakeNamingClient{fn: func(context.Context, string) (string, error) { return `..\CON.pdf`, nil }}
	s.namingClientFactory = func(NamingRequest, http.RoundTripper) (namingClient, error) { return client, nil }
	r, err := s.Download(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	d := r.Documents[0]
	if !d.Saved || d.Naming == nil || d.Naming.Source != "deterministic" || d.Naming.Reason != "unsafe_result" || !strings.Contains(d.Filename, "order-") {
		t.Fatalf("fallback: %+v", d)
	}
}

func TestMoveDiskFailures(t *testing.T) {
	for _, op := range []string{"pdf.create", "pdf.write", "pdf.sync", "pdf.close", "receipt.create", "receipt.write", "receipt.sync", "receipt.close", "receipt.publish", "publish.move", "manifest.write", "manifest.sync", "manifest.close", "manifest.rename"} {
		t.Run(op, func(t *testing.T) {
			s, _, req := moveFixture(t)
			injected := errors.New("synthetic failure")
			publishing := false
			s.diskBefore = func(operation, name string) error {
				if operation == "publish.move" {
					publishing = true
				}
				if operation == op && (!strings.HasPrefix(op, "manifest.") || publishing) {
					return injected
				}
				return nil
			}
			r, err := s.Download(context.Background(), req, nil)
			if !errors.Is(err, injected) {
				t.Fatalf("missing injected error: %v", err)
			}
			if strings.HasPrefix(op, "manifest.") {
				if !r.Documents[0].Saved || !errors.Is(err, ErrManifest) {
					t.Fatal("published file not reported")
				}
				s.diskBefore = nil
				_, err = s.Download(context.Background(), req, nil)
				if !errors.Is(err, ErrRecoveryRequired) {
					t.Fatalf("uncommitted file adopted: %v", err)
				}
			} else {
				assertNoIncompleteTemps(t, r.Directory)
			}
		})
	}
}

func TestMoveConcurrentPublication(t *testing.T) {
	s, _, req := moveFixture(t)
	root, err := openOutputDirectory(req.OutputDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	d := downloadDisk{root: root}
	if err := s.diskSetup(&d); err != nil {
		t.Fatal(err)
	}
	const count = 16
	sources := make([]string, count)
	for i := range sources {
		sources[i], err = d.writeClosed("race", []byte(fmt.Sprintf("writer %d", i)))
		if err != nil {
			t.Fatal(err)
		}
	}
	results := make(chan error, count)
	var wg sync.WaitGroup
	for _, source := range sources {
		wg.Go(func() { results <- d.publish(source, "winner.pdf") })
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		} else if !errors.Is(err, os.ErrExist) {
			t.Fatal(err)
		}
	}
	if wins != 1 {
		t.Fatalf("got %d winners", wins)
	}
	missing := 0
	for _, source := range sources {
		if _, err := root.Lstat(source); errors.Is(err, os.ErrNotExist) {
			missing++
		} else if err != nil {
			t.Fatal(err)
		}
	}
	if missing != 1 {
		t.Fatal("loser staging file consumed")
	}
}

func TestMoveConcurrentServices(t *testing.T) {
	s, f, req := moveFixture(t)
	second, f2, _ := moveFixture(t)
	f.beforeDownload = func() {
		if _, err := second.Download(context.Background(), req, nil); !errors.Is(err, ErrOutputBusy) {
			t.Fatalf("second writer: %v", err)
		}
	}
	if _, err := s.Download(context.Background(), req, nil); err != nil {
		t.Fatal(err)
	}
	if len(f2.calls) != 0 {
		t.Fatal("second writer reached fetch")
	}
}

// Real process termination bypasses cleanup at each meaningful publication stage.
func TestMoveCrashRecovery(t *testing.T) {
	if output := os.Getenv("ARCOURT_MOVE_CRASH_OUTPUT"); output != "" {
		s, _, req := moveFixture(t)
		req.OutputDirectory = output
		stage := os.Getenv("ARCOURT_MOVE_CRASH_STAGE")
		publishing := false
		s.diskBefore = func(op, name string) error {
			if op == "publish.move" {
				publishing = true
			}
			hit := op == stage && (!strings.HasPrefix(stage, "manifest.") || publishing)
			if stage == "cleanup" {
				hit = op == "remove" && receiptPattern.MatchString(name)
			}
			if hit {
				os.Exit(77)
			}
			return nil
		}
		_, _ = s.Download(context.Background(), req, nil)
		os.Exit(78)
	}
	for _, stage := range []string{"pdf.write", "pdf.sync", "receipt.publish", "publish.move", "manifest.create", "manifest.rename", "cleanup"} {
		t.Run(stage, func(t *testing.T) {
			s, f, req := moveFixture(t)
			cmd := exec.Command(os.Args[0], "-test.run=^TestMoveCrashRecovery$")
			cmd.Env = append(os.Environ(), "ARCOURT_MOVE_CRASH_OUTPUT="+req.OutputDirectory, "ARCOURT_MOVE_CRASH_STAGE="+stage)
			output, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 77 {
				t.Fatalf("child: %v %s", err, output)
			}
			dir := filepath.Join(req.OutputDirectory, "case-60cv-2026-1")
			before := snapshotFiles(t, dir)
			r, err := s.Download(context.Background(), req, nil)
			uncertain := stage == "publish.move" || stage == "manifest.create" || stage == "manifest.rename"
			if uncertain {
				if !errors.Is(err, ErrRecoveryRequired) || len(f.calls) != 0 {
					t.Fatalf("unproven crash recovery: %+v %v", r, err)
				}
				after := snapshotFiles(t, dir)
				for name, data := range before {
					if after[name] != data {
						t.Fatalf("unproven %s changed", name)
					}
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if stage == "cleanup" {
					assertLocalCounts(t, r, DownloadCounts{Selected: 1, Skipped: 1})
				} else {
					assertLocalCounts(t, r, DownloadCounts{Selected: 1, Succeeded: 1})
				}
				// Pre-receipt orphan staging is deliberately preserved, never swept.
				for name, data := range before {
					if strings.HasSuffix(name, ".tmp") {
						after, err := os.ReadFile(filepath.Join(dir, name))
						if err != nil || string(after) != data {
							t.Fatalf("orphan %s touched", name)
						}
					}
				}
			}
		})
	}
}

func snapshotFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[entry.Name()] = string(data)
	}
	return out
}

func TestMoveRecoveryPreservesIdenticalReplacement(t *testing.T) {
	s, _, req := moveFixture(t)
	s.diskBefore = func(op, name string) error {
		if op == "manifest.rename" {
			return errors.New("failure")
		}
		return nil
	}
	r, err := s.Download(context.Background(), req, nil)
	if !errors.Is(err, ErrManifest) {
		t.Fatal(err)
	}
	path := filepath.Join(r.Directory, r.Documents[0].Filename)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(fixturePDF), 0600); err != nil {
		t.Fatal(err)
	}
	before := snapshotFiles(t, r.Directory)
	s.diskBefore = nil
	if _, err := s.Download(context.Background(), req, nil); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("identical replacement adopted", err)
	}
	after := snapshotFiles(t, r.Directory)
	for name, data := range before {
		if after[name] != data {
			t.Fatalf("changed %s", name)
		}
	}
}

func TestMoveMetadataCollisions(t *testing.T) {
	for _, op := range []string{"manifest.publish", "receipt.publish"} {
		t.Run(op, func(t *testing.T) {
			s, _, req := moveFixture(t)
			var collision string
			s.diskBefore = func(operation, name string) error {
				if operation == op && collision == "" {
					collision = filepath.Join(req.OutputDirectory, "case-60cv-2026-1", name)
					return os.WriteFile(collision, []byte("unrelated metadata"), 0600)
				}
				return nil
			}
			r, err := s.Download(context.Background(), req, nil)
			if err == nil || r.Documents[0].Saved {
				t.Fatalf("metadata collision accepted: %+v %v", r, err)
			}
			data, err := os.ReadFile(collision)
			if err != nil || string(data) != "unrelated metadata" {
				t.Fatal("metadata overwritten", err)
			}
		})
	}
}

func TestMoveProbeRejectsOverwrite(t *testing.T) {
	_, _, req := moveFixture(t)
	root, err := openOutputDirectory(req.OutputDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	disk := downloadDisk{root: root, move: func(r *os.Root, a, b string) error { return r.Rename(a, b) }}
	if err := disk.probeMove(); !errors.Is(err, ErrUnsupportedFilesystem) {
		t.Fatalf("unsafe primitive accepted: %v", err)
	}
}

func TestMoveRejectsTraversal(t *testing.T) {
	s, _, req := moveFixture(t)
	root, err := openOutputDirectory(req.OutputDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	d := downloadDisk{root: root}
	if err := s.diskSetup(&d); err != nil {
		t.Fatal(err)
	}
	source, err := d.writeClosed("probe", []byte("source"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../escape.pdf", `..\escape.pdf`, "stream:escape", "trailing.", "/absolute"} {
		if err := d.publish(source, name); !errors.Is(err, ErrUnsafeDestination) {
			t.Fatalf("accepted %q: %v", name, err)
		}
	}
}

func TestMoveCancellationAfterReceipt(t *testing.T) {
	s, _, req := moveFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.diskBefore = func(op, name string) error {
		if op == "receipt.publish" {
			cancel()
		}
		return nil
	}
	r, err := s.Download(ctx, req, nil)
	if !errors.Is(err, context.Canceled) || r.Documents[0].Saved {
		t.Fatal("publication after cancellation", err)
	}
	for name := range snapshotFiles(t, r.Directory) {
		if receiptPattern.MatchString(name) || strings.HasSuffix(name, ".pdf") || strings.HasSuffix(name, ".tmp") {
			t.Fatalf("left %s", name)
		}
	}
}
