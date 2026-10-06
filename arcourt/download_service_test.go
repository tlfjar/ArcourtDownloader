package arcourt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type localFakeFetcher struct {
	entries        []DocketEntry
	statuses       map[string]DocumentStatus
	data           map[string]string
	calls          [][]string
	previewNumber  string
	duringWrite    func(context.Context, string, DocumentWriter) error
	beforeDownload func()
}

func (f *localFakeFetcher) PreviewCaseDocuments(ctx context.Context, number string) (*CasePreview, error) {
	if f.previewNumber != "" {
		number = f.previewNumber
	}
	return &CasePreview{CaseInfo: CaseInfo{CaseNumber: number}, DocketEntries: append([]DocketEntry(nil), f.entries...)}, ctx.Err()
}

func (f *localFakeFetcher) DownloadCaseDocuments(ctx context.Context, number string, selection DocumentSelection, sink DocumentSink) (*DownloadResult, error) {
	f.calls = append(f.calls, append([]string(nil), selection.SourceURLs...))
	if f.beforeDownload != nil {
		f.beforeDownload()
	}
	result := &DownloadResult{CaseInfo: CaseInfo{CaseNumber: number}}
	var batchErr error
	for _, id := range selection.SourceURLs {
		out := DocumentOutcome{Document: DocketEntry{SourceURL: id}}
		for _, entry := range f.entries {
			if normalizeSourceURL(entry.SourceURL) == id {
				out.Document = entry
				break
			}
		}
		switch {
		case ctx.Err() != nil:
			out.Status, out.Err = DocumentCanceled, ctx.Err()
		case f.statuses[id] == DocumentUnavailable:
			out.Status, out.Err = DocumentUnavailable, ErrDocumentUnavailable
		case f.statuses[id] == DocumentSkippedLimit:
			out.Status, out.Err = DocumentSkippedLimit, ErrDocumentLimit
		case f.statuses[id] == DocumentFailed:
			out.Status, out.Err = DocumentFailed, errors.New("https://court.invalid/?token=DO-NOT-PERSIST")
		default:
			w, err := sink.OpenDocument(ctx, out.Document)
			if err == nil {
				data := fixturePDF
				if value, ok := f.data[id]; ok {
					data = value
				}
				if f.duringWrite != nil {
					err = f.duringWrite(ctx, id, w)
				} else {
					_, err = w.Write([]byte(data))
				}
				if err == nil {
					err = w.Commit()
				}
				if err != nil {
					err = errors.Join(err, w.Abort())
				}
			}
			out.Err = err
			out.Status = DocumentSucceeded
			if err != nil {
				out.Status = DocumentFailed
			}
			if ctx.Err() != nil && err != nil {
				out.Status = DocumentCanceled
			}
		}
		batchErr = errors.Join(batchErr, out.Err)
		result.Outcomes = append(result.Outcomes, out)
	}
	return result, batchErr
}

func localFixture(t *testing.T, n int) (*DownloadService, *localFakeFetcher, DownloadRequest) {
	t.Helper()
	f := &localFakeFetcher{}
	for i := 0; i < n; i++ {
		f.entries = append(f.entries, DocketEntry{SourceURL: fmt.Sprintf("https://arcourts.gov/docs/%d?token=secret", i), RequestURL: fmt.Sprintf("https://arcourts.gov/docs/%d?token=secret", i), DocketDescription: "Order", FilingDate: "2026-09-01"})
	}
	s, err := NewDownloadService(f)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s, f, DownloadRequest{CaseNumber: "60CV-2026-1", Selection: append([]DocketEntry(nil), f.entries...), OutputDirectory: dir}
}

func readDownloadManifest(t *testing.T, result *LocalDownloadResult) DownloadManifest {
	t.Helper()
	data, err := os.ReadFile(result.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "https://") || strings.Contains(string(data), "token=") || strings.Contains(string(data), "DO-NOT-PERSIST") {
		t.Fatalf("source URL or error secret persisted: %s", data)
	}
	var m DownloadManifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func assertLocalCounts(t *testing.T, r *LocalDownloadResult, want DownloadCounts) {
	t.Helper()
	if r.Counts != want {
		t.Fatalf("counts=%+v, want %+v; documents=%+v", r.Counts, want, r.Documents)
	}
	c := r.Counts
	if c.Selected != c.Succeeded+c.Failed+c.Unavailable+c.Skipped+c.Canceled {
		t.Fatal("counts do not reconcile")
	}
}

func assertNoIncompleteTemps(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("unexpected temporary file: %s", entry.Name())
		}
	}
}

func TestLocalDownloadContentsManifestAndRepeat(t *testing.T) {
	s, f, req := localFixture(t, 2)
	original := append([]DocketEntry(nil), req.Selection...)
	events := make(chan DownloadEvent, 32)
	preview, err := s.Preview(context.Background(), req.CaseNumber)
	if err != nil || len(preview.DocketEntries) != 2 {
		t.Fatalf("preview=%+v: %v", preview, err)
	}
	r, err := s.Download(context.Background(), req, events)
	if err != nil {
		t.Fatal(err)
	}
	assertLocalCounts(t, r, DownloadCounts{Selected: 2, Succeeded: 2})
	if r.Partial {
		t.Fatal("unexpected partial result")
	}
	if !reflect.DeepEqual(req.Selection, original) || !reflect.DeepEqual(f.entries, original) {
		t.Fatal("source entries mutated")
	}
	if r.Documents[0].Filename == r.Documents[1].Filename {
		t.Fatal("duplicate descriptions collided")
	}
	hash := sha256.Sum256([]byte(fixturePDF))
	for _, doc := range r.Documents {
		data, err := os.ReadFile(filepath.Join(r.Directory, doc.Filename))
		if err != nil || string(data) != fixturePDF {
			t.Fatalf("saved content: %q %v", data, err)
		}
		if doc.SHA256 != hex.EncodeToString(hash[:]) || doc.Size != int64(len(data)) || !doc.Saved {
			t.Fatalf("bad content metadata: %+v", doc)
		}
		if doc.Description != "Order" || doc.FilingDate != "2026-09-01" || len(doc.Filename) > 120 {
			t.Fatalf("lost labels or long name: %+v", doc)
		}
	}
	m := readDownloadManifest(t, r)
	if m.Version != 1 || m.CaseNumber != req.CaseNumber || len(m.Documents) != 2 {
		t.Fatalf("bad manifest: %+v", m)
	}
	assertNoIncompleteTemps(t, r.Directory)
	seen := map[DownloadEventKind]bool{}
	close(events)
	for event := range events {
		seen[event.Kind] = true
	}
	for _, kind := range []DownloadEventKind{DownloadStarted, DownloadTransferring, DownloadDocumentDone, DownloadFinished} {
		if !seen[kind] {
			t.Errorf("missing event %s", kind)
		}
	}
	again, err := s.Download(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertLocalCounts(t, again, DownloadCounts{Selected: 2, Skipped: 2})
	if len(f.calls) != 1 {
		t.Fatalf("repeat download fetched again: %v", f.calls)
	}
	for _, doc := range again.Documents {
		if doc.SkipReason != SkipVerified || !doc.Saved {
			t.Fatalf("bad repeat: %+v", doc)
		}
	}
}

func TestLocalAlteredAndMissingRecovery(t *testing.T) {
	s, f, req := localFixture(t, 3)
	r, err := s.Download(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	altered := filepath.Join(r.Directory, r.Documents[0].Filename)
	if err := os.WriteFile(altered, []byte("user edited content"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(r.Directory, r.Documents[1].Filename)); err != nil {
		t.Fatal(err)
	}
	again, err := s.Download(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertLocalCounts(t, again, DownloadCounts{Selected: 3, Succeeded: 2, Skipped: 1})
	if len(f.calls) != 2 || len(f.calls[1]) != 2 {
		t.Fatalf("wrong retries: %v", f.calls)
	}
	data, err := os.ReadFile(altered)
	if err != nil || string(data) != "user edited content" {
		t.Fatal("modified file overwritten")
	}
	if again.Documents[0].Filename == r.Documents[0].Filename || again.Documents[1].Filename == r.Documents[1].Filename {
		t.Fatal("historical filename reused")
	}
	if len(readDownloadManifest(t, again).Documents) != 5 {
		t.Fatal("saved history lost")
	}
}

func TestLocalExistingCaseInsensitiveCollision(t *testing.T) {
	s, _, req := localFixture(t, 1)
	dir := filepath.Join(req.OutputDirectory, "case-60cv-2026-1")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	name := "2026-09-01_order-" + DocumentID(req.Selection[0].SourceURL)[:16] + ".pdf"
	unrelated := filepath.Join(dir, strings.ToUpper(name))
	if err := os.WriteFile(unrelated, []byte("unrelated"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := s.Download(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.EqualFold(r.Documents[0].Filename, name) {
		t.Fatal("case-insensitive collision reused")
	}
	data, err := os.ReadFile(unrelated)
	if err != nil || string(data) != "unrelated" {
		t.Fatal("unrelated file overwritten")
	}
}

func TestLocalPartialExplicitSkipUnavailableAndRetry(t *testing.T) {
	s, f, req := localFixture(t, 5)
	f.statuses = map[string]DocumentStatus{req.Selection[1].SourceURL: DocumentFailed, req.Selection[2].SourceURL: DocumentUnavailable, req.Selection[4].SourceURL: DocumentSkippedLimit}
	req.SkipSourceURLs = []string{req.Selection[3].SourceURL}
	req.Selection = append(req.Selection, req.Selection[0], DocketEntry{})
	r, err := s.Download(context.Background(), req, nil)
	if !errors.Is(err, ErrDocumentsIncomplete) || !r.Partial {
		t.Fatalf("partial=%v, err=%v", r.Partial, err)
	}
	assertLocalCounts(t, r, DownloadCounts{Selected: 5, Succeeded: 1, Failed: 1, Unavailable: 1, Skipped: 2})
	if len(f.calls[0]) != 4 || r.Documents[3].SkipReason != SkipExplicit || r.Documents[4].SkipReason != SkipLimit {
		t.Fatal("skip selection/accounting failed")
	}
	if len(readDownloadManifest(t, r).Documents) != 5 {
		t.Fatal("failure outcomes missing")
	}
	f.statuses = nil
	req.Selection = req.Selection[1:3]
	req.SkipSourceURLs = nil
	again, err := s.Download(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertLocalCounts(t, again, DownloadCounts{Selected: 2, Succeeded: 2})
	if len(f.calls[1]) != 2 {
		t.Fatal("independent retry fetched other documents")
	}
}

func TestLocalDeterministicDiskFailures(t *testing.T) {
	for _, op := range []string{"pdf.create", "pdf.write", "pdf.sync", "pdf.close", "receipt.write", "receipt.publish", "publish.link", "manifest.write", "manifest.rename"} {
		t.Run(op, func(t *testing.T) {
			s, _, req := localFixture(t, 1)
			injected := errors.New("injected disk failure")
			published := false
			s.diskBefore = func(operation, name string) error {
				if operation == "publish.link" && op != "publish.link" {
					published = true
				}
				if operation == op && (op != "manifest.write" || published) {
					return injected
				}
				return nil
			}
			r, err := s.Download(context.Background(), req, nil)
			if !errors.Is(err, injected) {
				t.Fatalf("lost disk error: %v", err)
			}
			assertLocalCounts(t, r, DownloadCounts{Selected: 1, Failed: 1})
			if op == "manifest.rename" || op == "manifest.write" {
				if !errors.Is(err, ErrManifest) || !r.Documents[0].Saved || !r.Partial {
					t.Fatalf("saved-but-unrecorded not reported: %+v %v", r, err)
				}
				if data, err := os.ReadFile(filepath.Join(r.Directory, r.Documents[0].Filename)); err != nil || string(data) != fixturePDF {
					t.Fatal("published file removed")
				}
			} else {
				assertNoIncompleteTemps(t, r.Directory)
			}
		})
	}
}

func TestLocalManifestFailureReconcilesWithoutRefetch(t *testing.T) {
	s, f, req := localFixture(t, 1)
	s.diskBefore = func(op, name string) error {
		if op == "manifest.rename" {
			return errors.New("injected rename failure")
		}
		return nil
	}
	r, err := s.Download(context.Background(), req, nil)
	if !errors.Is(err, ErrManifest) || !r.Documents[0].Saved {
		t.Fatalf("result=%+v error=%v", r, err)
	}
	if len(readDownloadManifest(t, r).Documents) != 0 {
		t.Fatal("failed rename changed manifest")
	}
	s.diskBefore = nil
	again, err := s.Download(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertLocalCounts(t, again, DownloadCounts{Selected: 1, Skipped: 1})
	if len(f.calls) != 1 || again.Documents[0].Filename != r.Documents[0].Filename {
		t.Fatal("recovery downloaded or renamed saved PDF")
	}
	assertNoIncompleteTemps(t, r.Directory)
	entries, _ := os.ReadDir(r.Directory)
	for _, entry := range entries {
		if receiptPattern.MatchString(entry.Name()) {
			t.Fatal("receipt was not retired")
		}
	}
}

func TestLocalCancellationMidWriteAndUnconsumedEvents(t *testing.T) {
	s, f, req := localFixture(t, 3)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.duringWrite = func(ctx context.Context, id string, w DocumentWriter) error {
		if id == req.Selection[1].SourceURL {
			if _, err := w.Write([]byte("%PDF-1.7\npartial")); err != nil {
				return err
			}
			cancel()
			_, err := w.Write([]byte("more bytes"))
			return err
		}
		_, err := w.Write([]byte(fixturePDF))
		return err
	}
	done := make(chan struct{})
	var r *LocalDownloadResult
	var err error
	go func() { r, err = s.Download(ctx, req, make(chan DownloadEvent)); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("progress subscriber deadlocked cancellation")
	}
	if !errors.Is(err, context.Canceled) || !r.Partial {
		t.Fatalf("partial=%v error=%v", r.Partial, err)
	}
	assertLocalCounts(t, r, DownloadCounts{Selected: 3, Succeeded: 1, Canceled: 2})
	data, readErr := os.ReadFile(filepath.Join(r.Directory, r.Documents[0].Filename))
	if readErr != nil || string(data) != fixturePDF {
		t.Fatal("completed PDF lost")
	}
	assertNoIncompleteTemps(t, r.Directory)
	if len(readDownloadManifest(t, r).Documents) != 3 {
		t.Fatal("canceled outcomes missing")
	}
}

func TestLocalConcurrentServicesAndBusyPreview(t *testing.T) {
	s, f, req := localFixture(t, 1)
	started, release := make(chan struct{}), make(chan struct{})
	f.beforeDownload = func() { close(started); <-release }
	done := make(chan error, 1)
	go func() { _, err := s.Download(context.Background(), req, nil); done <- err }()
	<-started
	if _, err := s.Preview(context.Background(), req.CaseNumber); !errors.Is(err, ErrDownloadBusy) {
		t.Errorf("preview busy: %v", err)
	}
	if _, err := s.Download(context.Background(), req, nil); !errors.Is(err, ErrDownloadBusy) {
		t.Errorf("download busy: %v", err)
	}
	other, _, _ := localFixture(t, 1)
	r, err := other.Download(context.Background(), req, nil)
	if !errors.Is(err, ErrOutputBusy) {
		t.Errorf("output lock: %v", err)
	}
	assertLocalCounts(t, r, DownloadCounts{Selected: 1, Failed: 1})
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := other.Download(context.Background(), req, nil); err != nil {
		t.Fatalf("lock not released: %v", err)
	}
}

func TestLocalEmptyInvalidAndCanceled(t *testing.T) {
	s, f, req := localFixture(t, 2)
	empty := req
	empty.Selection = nil
	empty.OutputDirectory = "this directory does not exist"
	r, err := s.Download(context.Background(), empty, nil)
	if err != nil || len(f.calls) != 0 {
		t.Fatalf("empty: %+v %v", r, err)
	}
	assertLocalCounts(t, r, DownloadCounts{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, err = s.Download(ctx, req, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	assertLocalCounts(t, r, DownloadCounts{Selected: 2, Canceled: 2})
	for _, number := range []string{"../outside", "60CV-2026-1/../../outside", "CON", ""} {
		req.CaseNumber = number
		if _, err := s.Download(context.Background(), req, nil); !errors.Is(err, ErrInvalidCaseNumber) {
			t.Fatalf("unsafe case %q: %v", number, err)
		}
	}
	f.previewNumber = "60CV-2026-2"
	if _, err := s.Preview(context.Background(), "60CV-2026-1"); !errors.Is(err, ErrCaseNumberMismatch) {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(req.OutputDirectory); len(entries) != 0 {
		t.Fatal("empty/invalid/canceled request wrote files")
	}
}

func TestLocalRejectsTraversalAndUntrustedManifest(t *testing.T) {
	for _, kind := range []string{"relative", "traversal", "wrong_version", "wrong_case", "traversal_record", "unrelated_manifest", "uppercase_manifest"} {
		t.Run(kind, func(t *testing.T) {
			s, f, req := localFixture(t, 1)
			dir := filepath.Join(req.OutputDirectory, "case-60cv-2026-1")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			m := DownloadManifest{Version: 1, CaseNumber: req.CaseNumber}
			switch kind {
			case "relative":
				req.OutputDirectory = "relative"
			case "traversal":
				req.OutputDirectory = req.OutputDirectory + string(filepath.Separator) + ".." + string(filepath.Separator) + filepath.Base(req.OutputDirectory)
			case "wrong_version":
				m.Version = 99
			case "wrong_case":
				m.CaseNumber = "60CV-2026-2"
			case "traversal_record":
				m.Documents = []LocalDocumentResult{{DocumentID: DocumentID(req.Selection[0].SourceURL), Timestamp: time.Now(), Status: DocumentSucceeded, Saved: true, Filename: "../outside.pdf", Size: 10, SHA256: strings.Repeat("a", 64)}}
			}
			var original []byte
			name := ManifestFilename
			if kind != "relative" && kind != "traversal" {
				original, _ = json.Marshal(m)
				if kind == "unrelated_manifest" {
					original = []byte("user data")
				}
				if kind == "uppercase_manifest" {
					name = strings.ToUpper(name)
				}
				if err := os.WriteFile(filepath.Join(dir, name), original, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.Download(context.Background(), req, nil); err == nil {
				t.Fatal("unsafe destination accepted")
			}
			if len(f.calls) != 0 {
				t.Fatal("fetch occurred before destination validation")
			}
			if original != nil {
				data, _ := os.ReadFile(filepath.Join(dir, name))
				if string(data) != string(original) {
					t.Fatal("unrelated manifest overwritten")
				}
			}
		})
	}
}

func TestLocalInvalidPDFAndFailurePreservePriorSuccess(t *testing.T) {
	s, f, req := localFixture(t, 3)
	f.data = map[string]string{req.Selection[1].SourceURL: "<html>error</html>", req.Selection[2].SourceURL: "%PDF-1.7\ntruncated"}
	r, err := s.Download(context.Background(), req, nil)
	if !errors.Is(err, ErrInvalidPDF) || !r.Partial {
		t.Fatalf("bad screening: %+v %v", r, err)
	}
	assertLocalCounts(t, r, DownloadCounts{Selected: 3, Succeeded: 1, Failed: 2})
	assertNoIncompleteTemps(t, r.Directory)
}

// A real subprocess exits between publish and manifest replacement, bypassing
// all defers. This also verifies that the OS releases the case lock on death.
func TestLocalCrashRecovery(t *testing.T) {
	if output := os.Getenv("ARCOURT_TEST_CRASH_OUTPUT"); output != "" {
		s, _, req := localFixture(t, 1)
		req.OutputDirectory = output
		published := false
		s.diskBefore = func(op, name string) error {
			if op == "publish.link" {
				published = true
			}
			if op == "manifest.create" && published {
				os.Exit(77)
			}
			return nil
		}
		_, _ = s.Download(context.Background(), req, nil)
		os.Exit(78)
	}
	s, f, req := localFixture(t, 1)
	cmd := exec.Command(os.Args[0], "-test.run=^TestLocalCrashRecovery$")
	cmd.Env = append(os.Environ(), "ARCOURT_TEST_CRASH_OUTPUT="+req.OutputDirectory)
	err := cmd.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 77 {
		t.Fatalf("crash child: %v", err)
	}
	r, err := s.Download(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertLocalCounts(t, r, DownloadCounts{Selected: 1, Skipped: 1})
	if len(f.calls) != 0 {
		t.Fatal("crash recovery refetched document")
	}
	assertNoIncompleteTemps(t, r.Directory)
}

func TestLocalCrossProcessLock(t *testing.T) {
	if output := os.Getenv("ARCOURT_TEST_LOCK_OUTPUT"); output != "" {
		s, _, req := localFixture(t, 1)
		req.OutputDirectory = output
		if _, err := s.Download(context.Background(), req, nil); !errors.Is(err, ErrOutputBusy) {
			t.Fatalf("child lock: %v", err)
		}
		return
	}
	_, _, req := localFixture(t, 1)
	dir := filepath.Join(req.OutputDirectory, "case-60cv-2026-1")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	lock, err := lockOutput(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestLocalCrossProcessLock$")
	cmd.Env = append(os.Environ(), "ARCOURT_TEST_LOCK_OUTPUT="+req.OutputDirectory)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child: %v %s", err, output)
	}
}

func TestLocalOversizedMetadataCannotPublishUnrecoverablePDF(t *testing.T) {
	s, f, req := localFixture(t, 1)
	f.entries[0].DocketDescription = strings.Repeat("x", maxManifestBytes)
	r, err := s.Download(context.Background(), req, nil)
	if !errors.Is(err, ErrManifest) || r.Documents[0].Saved {
		t.Fatalf("metadata overflow: saved=%v err=%v", r.Documents[0].Saved, err)
	}
	assertLocalCounts(t, r, DownloadCounts{Selected: 1, Failed: 1})
	assertNoIncompleteTemps(t, r.Directory)
	entries, err := os.ReadDir(r.Directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".pdf") {
			t.Fatal("unrecoverable PDF published")
		}
	}
}
