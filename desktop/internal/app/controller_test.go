package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tlfjar/ArcourtDownloader/arcourt"
)

const caseNumber = "60CV-2026-1"

func fixturePreview() *arcourt.CasePreview {
	return &arcourt.CasePreview{CaseInfo: arcourt.CaseInfo{CaseNumber: caseNumber, CaseTitle: "Synthetic v Fixture"}, DocketEntries: []arcourt.DocketEntry{
		{SourceURL: "https://arcourts.gov/one.pdf?signature=SECRET", RequestURL: "https://arcourts.gov/one.pdf?signature=SECRET", DocketDescription: "Order", FilingDate: "01/02/2026"},
		{SourceURL: "https://arcourts.gov/two.pdf", DocketDescription: "Notice", FilingDate: "01/03/2026"},
	}}
}

type fakeService struct {
	preview  func(context.Context, string) (*arcourt.CasePreview, error)
	download func(context.Context, arcourt.DownloadRequest, chan<- arcourt.DownloadEvent) (*arcourt.LocalDownloadResult, error)
}

func (s fakeService) Preview(ctx context.Context, n string) (*arcourt.CasePreview, error) {
	if s.preview != nil {
		return s.preview(ctx, n)
	}
	return fixturePreview(), nil
}
func (s fakeService) Download(ctx context.Context, r arcourt.DownloadRequest, ch chan<- arcourt.DownloadEvent) (*arcourt.LocalDownloadResult, error) {
	return s.download(ctx, r, ch)
}

func controller(t *testing.T, s Service, closeService func() error) *Controller {
	t.Helper()
	if closeService == nil {
		closeService = func() error { return nil }
	}
	c := New(func(Preferences) (Service, func() error, string, error) {
		return s, closeService, "Synthetic browser", nil
	}, PreferenceStore{filepath.Join(t.TempDir(), "settings.json")})
	t.Cleanup(c.Close)
	if err := c.SetCase(caseNumber); err != nil {
		t.Fatal(err)
	}
	return c
}
func waitIdle(t *testing.T, c *Controller) State {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s := c.Snapshot(); !s.Busy {
			return s
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("controller did not become idle")
	return State{}
}
func ready(t *testing.T, c *Controller) State {
	t.Helper()
	if err := c.PreviewCase(); err != nil {
		t.Fatal(err)
	}
	s := waitIdle(t, c)
	if s.Preview == nil {
		t.Fatalf("missing preview: %+v", s)
	}
	return s
}
func selected(t *testing.T, c *Controller) State {
	t.Helper()
	s := ready(t, c)
	if err := c.SavePreferences(Preferences{OutputDirectory: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if err := c.SetSelection(s.Generation, []string{s.Preview.Documents[0].ID}, true, false); err != nil {
		t.Fatal(err)
	}
	return c.Snapshot()
}

func TestStalePreviewAndLateProgress(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	c := controller(t, fakeService{preview: func(ctx context.Context, _ string) (*arcourt.CasePreview, error) {
		close(entered)
		<-release
		return fixturePreview(), nil
	}}, nil)
	if err := c.PreviewCase(); err != nil {
		t.Fatal(err)
	}
	<-entered
	old := c.Snapshot().Generation
	if err := c.SetCase("60CV-2026-2"); err != nil {
		t.Fatal(err)
	}
	if err := c.PreviewCase(); err == nil {
		t.Fatal("overlap accepted while canceled job unwinds")
	}
	c.progress(old, arcourt.DownloadEvent{Kind: arcourt.DownloadDocumentDone, Counts: arcourt.DownloadCounts{Succeeded: 99}})
	close(release)
	s := waitIdle(t, c)
	if s.Preview != nil || s.Selected != 0 || s.Result != nil || s.Progress.Succeeded != 0 || s.CaseNumber != "60CV-2026-2" {
		t.Fatalf("stale state: %+v", s)
	}
}

func TestSelectionVerificationAndChangedCase(t *testing.T) {
	var calls atomic.Int32
	c := controller(t, fakeService{download: func(context.Context, arcourt.DownloadRequest, chan<- arcourt.DownloadEvent) (*arcourt.LocalDownloadResult, error) {
		calls.Add(1)
		return nil, nil
	}}, nil)
	s := ready(t, c)
	if err := c.Download(s.Generation); err == nil {
		t.Fatal("unverified download accepted")
	}
	if err := c.SetSelection(s.Generation, []string{}, true, true); err != nil {
		t.Fatal(err)
	}
	if err := c.Download(s.Generation); err == nil {
		t.Fatal("empty selection accepted")
	}
	if err := c.SetSelection(s.Generation, []string{"unknown"}, true, false); err == nil {
		t.Fatal("unknown selection accepted")
	}
	if err := c.SetSelection(s.Generation, []string{s.Preview.Documents[1].ID}, true, false); err != nil {
		t.Fatal(err)
	}
	if c.Snapshot().Selected != 1 {
		t.Fatal("selection missing")
	}
	if err := c.SetCase("60CV-2026-2"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetSelection(s.Generation, []string{s.Preview.Documents[1].ID}, true, false); err == nil {
		t.Fatal("stale selection accepted")
	}
	if err := c.Download(s.Generation); err == nil {
		t.Fatal("stale download accepted")
	}
	if s := c.Snapshot(); s.Selected != 0 || s.Verified || s.Preview != nil || calls.Load() != 0 {
		t.Fatalf("selection leaked: %+v", s)
	}
}

func TestCancelWaitsForCloseAndKeepsResults(t *testing.T) {
	entered, unwound := make(chan struct{}), make(chan struct{})
	var closeCalls atomic.Int32
	c := controller(t, fakeService{download: func(ctx context.Context, r arcourt.DownloadRequest, ch chan<- arcourt.DownloadEvent) (*arcourt.LocalDownloadResult, error) {
		close(entered)
		<-ctx.Done()
		<-unwound
		return &arcourt.LocalDownloadResult{Counts: arcourt.DownloadCounts{Selected: 2, Succeeded: 1, Canceled: 1}, Partial: true}, ctx.Err()
	}}, func() error { closeCalls.Add(1); return nil })
	s := selected(t, c)
	if err := c.Download(s.Generation); err != nil {
		t.Fatal(err)
	}
	<-entered
	c.Cancel()
	if s := c.Snapshot(); !s.Busy || !s.Canceling {
		t.Fatal("slot released before cleanup")
	}
	closed := make(chan struct{})
	go func() { c.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("closed before operation unwound")
	case <-time.After(20 * time.Millisecond):
	}
	close(unwound)
	<-closed
	s = c.Snapshot()
	if s.Busy || s.Result == nil || s.Result.Counts.Succeeded != 1 || s.Phase != "canceled" || closeCalls.Load() < 3 {
		t.Fatalf("cancel/close: %+v closes=%d", s, closeCalls.Load())
	}
	if err := c.PreviewCase(); err == nil {
		t.Fatal("job accepted after close")
	}
}

func TestProgressTerminalResultAndCloseFailure(t *testing.T) {
	var closeFail atomic.Bool
	c := controller(t, fakeService{download: func(_ context.Context, r arcourt.DownloadRequest, ch chan<- arcourt.DownloadEvent) (*arcourt.LocalDownloadResult, error) {
		ch <- arcourt.DownloadEvent{Kind: arcourt.DownloadFinished, Counts: arcourt.DownloadCounts{Succeeded: 99}}
		return &arcourt.LocalDownloadResult{Counts: arcourt.DownloadCounts{Selected: 1, Succeeded: 1}}, nil
	}}, func() error {
		if closeFail.Load() {
			return errors.New("SECRET")
		}
		return nil
	})
	s := selected(t, c)
	closeFail.Store(true)
	if err := c.Download(s.Generation); err != nil {
		t.Fatal(err)
	}
	s = waitIdle(t, c)
	if s.Progress.Succeeded != 1 || s.Phase != "error" || s.Diagnostic == "" {
		t.Fatalf("terminal/cleanup: %+v", s)
	}
	c.progress(s.Generation, arcourt.DownloadEvent{Kind: arcourt.DownloadFinished, Counts: arcourt.DownloadCounts{Succeeded: 99}})
	if c.Snapshot().Progress.Succeeded != 1 {
		t.Fatal("late progress changed terminal result")
	}
}

func TestOutcomeMappingAndRedaction(t *testing.T) {
	for _, tc := range []struct {
		name     string
		r        *arcourt.LocalDownloadResult
		err      error
		coverage *arcourt.DiscoveryCoverage
		phase    string
	}{
		{"success", &arcourt.LocalDownloadResult{Counts: arcourt.DownloadCounts{Selected: 1, Succeeded: 1}}, nil, nil, "success"},
		{"repeat", &arcourt.LocalDownloadResult{Counts: arcourt.DownloadCounts{Selected: 1, Skipped: 1}, Documents: []arcourt.LocalDocumentResult{{Status: arcourt.DocumentSkipped, SkipReason: arcourt.SkipVerified, Saved: true}}}, nil, nil, "success"},
		{"partial", &arcourt.LocalDownloadResult{Counts: arcourt.DownloadCounts{Selected: 2, Succeeded: 1, Unavailable: 1}, Partial: true}, nil, nil, "partial"},
		{"all unknown", &arcourt.LocalDownloadResult{Counts: arcourt.DownloadCounts{Selected: 1, Succeeded: 1}}, nil, &arcourt.DiscoveryCoverage{}, "success"},
		{"missing outcome count", &arcourt.LocalDownloadResult{Counts: arcourt.DownloadCounts{Selected: 2, Succeeded: 1}}, nil, &arcourt.DiscoveryCoverage{}, "partial"},
		{"excess outcome count", &arcourt.LocalDownloadResult{Counts: arcourt.DownloadCounts{Selected: 1, Succeeded: 2}}, nil, &arcourt.DiscoveryCoverage{}, "partial"},
		{"all document failure", &arcourt.LocalDownloadResult{Counts: arcourt.DownloadCounts{Selected: 2, Succeeded: 1, Failed: 1}, Partial: true}, arcourt.ErrDocumentsIncomplete, &arcourt.DiscoveryCoverage{}, "partial"},
		{"all fetch limit", &arcourt.LocalDownloadResult{Counts: arcourt.DownloadCounts{Selected: 1, Skipped: 1}, Documents: []arcourt.LocalDocumentResult{{Status: arcourt.DocumentSkipped, SkipReason: arcourt.SkipLimit}}}, nil, &arcourt.DiscoveryCoverage{}, "partial"},
		{"cancel", nil, context.Canceled, nil, "canceled"},
		{"all cancel", &arcourt.LocalDownloadResult{Counts: arcourt.DownloadCounts{Selected: 2, Succeeded: 1, Canceled: 1}, Partial: true}, context.Canceled, &arcourt.DiscoveryCoverage{}, "canceled"},
		{"failure", nil, errors.New("https://host/?signature=SECRET"), nil, "error"},
		{"manifest", &arcourt.LocalDownloadResult{Counts: arcourt.DownloadCounts{Selected: 1, Succeeded: 1}}, arcourt.ErrManifest, nil, "partial"},
		{"all manifest", &arcourt.LocalDownloadResult{Counts: arcourt.DownloadCounts{Selected: 1, Succeeded: 1}}, arcourt.ErrManifest, &arcourt.DiscoveryCoverage{}, "partial"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			phase, msg := outcome(tc.r, tc.err, tc.coverage)
			if phase != tc.phase || strings.Contains(msg, "SECRET") {
				t.Fatalf("%s %s", phase, msg)
			}
		})
	}
	p := fixturePreview()
	p.CaseInfo.CaseTitle = "Case https://host/?signature=SECRET"
	p.DocketEntries[0].DocketDescription = "PDF https://host/?token=SECRET"
	b, _ := json.Marshal(previewView(p))
	if strings.Contains(string(b), "SECRET") || strings.Contains(string(b), "SourceURL") {
		t.Fatal(string(b))
	}
	r := resultView(&arcourt.LocalDownloadResult{Documents: []arcourt.LocalDocumentResult{{Description: "https://host/?token=SECRET", Err: errors.New("SECRET")}}})
	b, _ = json.Marshal(r)
	if strings.Contains(string(b), "SECRET") {
		t.Fatal(string(b))
	}
	if !strings.Contains(operationError(os.ErrPermission), "writable") {
		t.Fatal("permission advice missing")
	}
}

func TestOutcomeExplainsConcreteCoverageWarnings(t *testing.T) {
	result := &arcourt.LocalDownloadResult{Counts: arcourt.DownloadCounts{Selected: 2, Succeeded: 2}}
	for _, tc := range []struct {
		name     string
		coverage arcourt.DiscoveryCoverage
		message  string
	}{
		{"truncated", arcourt.DiscoveryCoverage{Truncated: true}, "additional documents were omitted"},
		{"paginated", arcourt.DiscoveryCoverage{PaginationDetected: true}, "may contain additional documents"},
		{"virtualized", arcourt.DiscoveryCoverage{VirtualizationDetected: true}, "may contain additional documents"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, complete := range []bool{false, true} {
				coverage := tc.coverage
				coverage.Complete = complete
				phase, message := outcome(result, nil, &coverage)
				if phase != "partial" || !strings.HasPrefix(message, "Download complete: 2 saved; 0 already downloaded and verified.") || !strings.Contains(message, tc.message) || !strings.Contains(message, "Select All included only the documents shown.") {
					t.Fatalf("Complete=%t: %s %q", complete, phase, message)
				}
			}
		})
	}
}

func TestSelectAllDownloadSeparatesSuccessFromCoverage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		result  *arcourt.LocalDownloadResult
		err     error
		phase   string
		message string
	}{
		{
			name: "saved",
			result: &arcourt.LocalDownloadResult{Counts: arcourt.DownloadCounts{Selected: 2, Succeeded: 2}, Documents: []arcourt.LocalDocumentResult{
				{Status: arcourt.DocumentSucceeded, Saved: true}, {Status: arcourt.DocumentSucceeded, Saved: true},
			}},
			phase: "success", message: "Download complete: 2 saved; 0 already downloaded and verified.",
		},
		{
			name: "verified existing files",
			result: &arcourt.LocalDownloadResult{Counts: arcourt.DownloadCounts{Selected: 2, Skipped: 2}, Documents: []arcourt.LocalDocumentResult{
				{Status: arcourt.DocumentSkipped, SkipReason: arcourt.SkipVerified, Saved: true},
				{Status: arcourt.DocumentSkipped, SkipReason: arcourt.SkipVerified, Saved: true},
			}},
			phase: "success", message: "Download complete: 0 saved; 2 already downloaded and verified.",
		},
		{
			name: "unavailable document",
			result: &arcourt.LocalDownloadResult{Counts: arcourt.DownloadCounts{Selected: 2, Succeeded: 1, Unavailable: 1}, Partial: true, Documents: []arcourt.LocalDocumentResult{
				{Status: arcourt.DocumentSucceeded, Saved: true}, {Status: arcourt.DocumentUnavailable, Err: arcourt.ErrDocumentUnavailable},
			}},
			err: arcourt.ErrDocumentsIncomplete, phase: "partial", message: "Some selected documents did not complete.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan arcourt.DownloadRequest, 1)
			c := controller(t, fakeService{download: func(_ context.Context, req arcourt.DownloadRequest, _ chan<- arcourt.DownloadEvent) (*arcourt.LocalDownloadResult, error) {
				requests <- req
				return tc.result, tc.err
			}}, nil)
			output := t.TempDir()
			if err := c.SavePreferences(Preferences{OutputDirectory: output}); err != nil {
				t.Fatal(err)
			}
			s := ready(t, c)
			var ids []string
			for _, doc := range s.Preview.Documents {
				ids = append(ids, doc.ID)
			}
			if err := c.SetSelection(s.Generation, ids, true, true); err != nil {
				t.Fatal(err)
			}
			if err := c.Download(s.Generation); err != nil {
				t.Fatal(err)
			}
			s = waitIdle(t, c)
			select {
			case req := <-requests:
				if req.CaseNumber != caseNumber || req.OutputDirectory != output || len(req.Selection) != len(ids) {
					t.Fatalf("incorrect Select All request: %+v", req)
				}
				for i, entry := range req.Selection {
					if entry.SourceURL != fixturePreview().DocketEntries[i].SourceURL {
						t.Fatalf("selection entry %d changed", i)
					}
				}
			default:
				t.Fatal("download request missing")
			}
			if !s.All || s.Phase != tc.phase || !strings.HasPrefix(s.Message, tc.message) || s.Result == nil || s.Result.Counts != tc.result.Counts || s.Result.Partial != tc.result.Partial {
				t.Fatalf("Select All result: %+v", s)
			}
			if len(s.Preview.Warnings) == 0 {
				t.Fatal("coverage note disappeared after download")
			}
			if tc.phase == "success" && (!strings.Contains(s.Message, "Select All included every document in this preview.") || !strings.Contains(s.Message, "Records the court does not make available online cannot be checked.")) {
				t.Fatalf("success lacks explanation of coverage: %q", s.Message)
			}
		})
	}
}

func TestSettingsRoundtripAndInvalidation(t *testing.T) {
	c := controller(t, fakeService{}, nil)
	s := ready(t, c)
	p := Preferences{OutputDirectory: t.TempDir(), CaseURLTemplate: "https://example.invalid/case/{case_number}"}
	if err := c.SavePreferences(p); err != nil {
		t.Fatal(err)
	}
	got, err := c.store.Load()
	if err != nil || got != p {
		t.Fatalf("%+v %v", got, err)
	}
	if now := c.Snapshot(); now.Preview != nil || now.Generation <= s.Generation {
		t.Fatal("settings did not invalidate preview")
	}
	for _, template := range []string{"bad", "https://user:secret@host/{case_number}", "https://host/{case_number}?X-Amz-Signature=secret"} {
		p.CaseURLTemplate = template
		if err := c.SavePreferences(p); err == nil {
			t.Fatal("accepted credential/invalid template")
		}
	}
	if err := os.WriteFile(c.store.Path, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	bad := New(c.factory, c.store)
	defer bad.Close()
	if bad.Snapshot().Diagnostic == "" {
		t.Fatal("corrupt settings not reported")
	}
}

func TestPreviewCancellationAndHeaderMismatch(t *testing.T) {
	for _, kind := range []string{"cancel", "mismatch"} {
		t.Run(kind, func(t *testing.T) {
			entered := make(chan struct{})
			c := controller(t, fakeService{preview: func(ctx context.Context, _ string) (*arcourt.CasePreview, error) {
				close(entered)
				if kind == "cancel" {
					<-ctx.Done()
					return nil, ctx.Err()
				}
				p := fixturePreview()
				p.CaseInfo.CaseNumber = "60CV-2026-2"
				return p, nil
			}}, nil)
			if err := c.PreviewCase(); err != nil {
				t.Fatal(err)
			}
			<-entered
			if kind == "cancel" {
				c.Cancel()
			}
			s := waitIdle(t, c)
			if s.Preview != nil || s.Selected != 0 || s.Message == "" {
				t.Fatalf("invalid preview exposed: %+v", s)
			}
			if kind == "cancel" && s.Phase != "canceled" {
				t.Fatal(s.Phase)
			}
		})
	}
}

func TestDownloadFrozenSelectionAndStaleResult(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var request arcourt.DownloadRequest
	c := controller(t, fakeService{download: func(_ context.Context, r arcourt.DownloadRequest, events chan<- arcourt.DownloadEvent) (*arcourt.LocalDownloadResult, error) {
		request = r
		close(entered)
		<-release
		events <- arcourt.DownloadEvent{Kind: arcourt.DownloadDocumentDone, Counts: arcourt.DownloadCounts{Succeeded: 1}}
		return &arcourt.LocalDownloadResult{Counts: arcourt.DownloadCounts{Selected: 1, Succeeded: 1}}, nil
	}}, nil)
	s := selected(t, c)
	if err := c.Download(s.Generation); err != nil {
		t.Fatal(err)
	}
	<-entered
	if len(request.Selection) != 1 || request.Selection[0].SourceURL != fixturePreview().DocketEntries[0].SourceURL || request.OutputDirectory != s.Preferences.OutputDirectory {
		t.Fatalf("wrong request: %+v", request)
	}
	if err := c.SetSelection(s.Generation, nil, true, false); err == nil {
		t.Fatal("active selection mutated")
	}
	if err := c.SavePreferences(Preferences{}); err == nil {
		t.Fatal("active settings mutated")
	}
	if err := c.Download(s.Generation); err == nil {
		t.Fatal("overlap accepted")
	}
	if err := c.SetCase("60CV-2026-2"); err != nil {
		t.Fatal(err)
	}
	close(release)
	s = waitIdle(t, c)
	if s.Result != nil || s.Progress.Succeeded != 0 || s.Phase != "idle" {
		t.Fatalf("old download changed new case: %+v", s)
	}
}

func TestPreviewCoverageAndSnapshotIsolation(t *testing.T) {
	p := fixturePreview()
	p.Discovery.Truncated = true
	p.Discovery.PaginationDetected = true
	c := controller(t, fakeService{preview: func(context.Context, string) (*arcourt.CasePreview, error) { return p, nil }}, nil)
	s := ready(t, c)
	if len(s.Preview.Warnings) != 3 {
		t.Fatalf("coverage warnings: %+v", s.Preview.Warnings)
	}
	if note := s.Preview.Warnings[0]; !strings.Contains(note, "sealed, withheld") || !strings.Contains(note, "site does not provide") || !strings.Contains(note, "does not indicate a download failure") {
		t.Fatalf("unclear coverage note: %q", note)
	}
	id := s.Preview.Documents[0].ID
	s.Preview.Documents[0].ID = "changed by caller"
	if c.Snapshot().Preview.Documents[0].ID != id {
		t.Fatal("snapshot aliases controller state")
	}
}
