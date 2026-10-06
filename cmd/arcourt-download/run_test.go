package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tlfjar/ArcourtDownloader/arcourt"
)

const testCase = "60CV-2026-1"
const testTemplate = "https://court.example.invalid/case/{case_number}"

func noEnv(string) string { return "" }

func testPreview() *arcourt.CasePreview {
	return &arcourt.CasePreview{
		CaseInfo: arcourt.CaseInfo{CaseNumber: testCase, CaseTitle: "Synthetic v Fixture", County: "Fixture County", Judge: "Fixture Judge"},
		DocketEntries: []arcourt.DocketEntry{
			{SourceURL: "https://arcourts.gov/doc/one", RequestURL: "https://arcourts.gov/doc/one?signature=SECRET", FilingDate: "01/02/2026", DocketDescription: "Synthetic order"},
			{SourceURL: "https://arcourts.gov/doc/two", RequestURL: "https://arcourts.gov/doc/two?signature=SECRET", FilingDate: "01/03/2026", DocketDescription: "Synthetic notice"},
		},
		Discovery: arcourt.DiscoveryCoverage{Complete: true, DiscoverableDocuments: 2},
	}
}

type fakeService struct {
	preview                  func(context.Context, string) (*arcourt.CasePreview, error)
	download                 func(context.Context, arcourt.DownloadRequest, chan<- arcourt.DownloadEvent) (*arcourt.LocalDownloadResult, error)
	requests                 []arcourt.DownloadRequest
	previewCalls, closeCalls int
	closeErr                 error
}

func (f *fakeService) Preview(ctx context.Context, number string) (*arcourt.CasePreview, error) {
	f.previewCalls++
	if f.preview != nil {
		return f.preview(ctx, number)
	}
	return testPreview(), nil
}

func (f *fakeService) Download(ctx context.Context, req arcourt.DownloadRequest, events chan<- arcourt.DownloadEvent) (*arcourt.LocalDownloadResult, error) {
	f.requests = append(f.requests, req)
	if f.download != nil {
		return f.download(ctx, req, events)
	}
	r := &arcourt.LocalDownloadResult{CaseNumber: req.CaseNumber, Directory: req.OutputDirectory,
		Counts: arcourt.DownloadCounts{Selected: len(req.Selection), Succeeded: len(req.Selection)}}
	for _, e := range req.Selection {
		d := arcourt.LocalDocumentResult{DocumentID: arcourt.DocumentID(e.SourceURL), Description: e.DocketDescription, Status: arcourt.DocumentSucceeded, Saved: true}
		r.Documents = append(r.Documents, d)
		events <- arcourt.DownloadEvent{Kind: arcourt.DownloadDocumentDone, DocumentID: d.DocumentID, Document: d}
	}
	return r, nil
}

func (f *fakeService) factory(arcourt.BrowserFetcherConfig) (service, func() error, error) {
	return f, func() error { f.closeCalls++; return f.closeErr }, nil
}

func baseArgs(command string) []string {
	return []string{command, "--case", testCase, "--case-url-template", testTemplate, "--json"}
}

func invoke(t *testing.T, f serviceFactory, args ...string) (int, response, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), args, noEnv, &stdout, &stderr, f)
	var r response
	d := json.NewDecoder(strings.NewReader(stdout.String()))
	if err := d.Decode(&r); err != nil {
		t.Fatalf("invalid JSON: %v, stdout=%s", err, stdout.String())
	}
	if err := d.Decode(new(any)); err != io.EOF {
		t.Fatalf("stdout contained more than one JSON value: %s", stdout.String())
	}
	if r.ExitCode != code {
		t.Fatalf("exit mismatch: %d / %+v", code, r)
	}
	return code, r, stdout.String(), stderr.String()
}

func TestPreviewToDownloadWithFakeService(t *testing.T) {
	f := &fakeService{}
	code, preview, stdout, stderr := invoke(t, f.factory, baseArgs("preview")...)
	if code != exitSuccess || len(preview.Preview.Documents) != 2 || preview.Preview.Case.Number != testCase {
		t.Fatalf("%d %+v", code, preview)
	}
	if !strings.Contains(stderr, "Discovering case") || strings.Contains(stdout, "SECRET") || strings.Contains(stdout, "https://") {
		t.Fatalf("output isolation: %s / %s", stdout, stderr)
	}
	for _, mode := range []string{"selected", "all"} {
		t.Run(mode, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "output with spaces")
			if err := os.Mkdir(out, 0700); err != nil {
				t.Fatal(err)
			}
			fresh := testPreview()
			fresh.DocketEntries[1].RequestURL = "https://arcourts.gov/doc/two?signature=REFRESHED"
			f.preview = func(context.Context, string) (*arcourt.CasePreview, error) { return fresh, nil }
			args := append(baseArgs("download"), "--output", out)
			want := fresh.DocketEntries
			if mode == "selected" {
				id := preview.Preview.Documents[1].ID
				args = append(args, "--id", id, "--id", strings.ToUpper(id))
				want = fresh.DocketEntries[1:]
			} else {
				args = append(args, "--all")
			}
			code, result, stdout, stderr := invoke(t, f.factory, args...)
			if code != exitSuccess || result.Download.Counts.Selected != len(want) {
				t.Fatalf("%d %+v", code, result)
			}
			req := f.requests[len(f.requests)-1]
			if req.OutputDirectory != out || req.CaseNumber != testCase || !reflect.DeepEqual(req.Selection, want) {
				t.Fatalf("request=%+v want=%+v", req, want)
			}
			if !strings.Contains(stderr, "succeeded") || strings.Contains(stdout, "REFRESHED") {
				t.Fatalf("bad streams: %s / %s", stdout, stderr)
			}
		})
	}
	if f.previewCalls != 3 || f.closeCalls != 3 {
		t.Fatalf("lifecycle: %+v", f)
	}
}

func TestInvalidInvocations(t *testing.T) {
	out := t.TempDir()
	id := arcourt.DocumentID(testPreview().DocketEntries[0].SourceURL)
	valid := append(baseArgs("download"), "--output", out)
	for _, tc := range []struct {
		name    string
		args    []string
		message string
	}{
		{"no command", []string{"--json"}, "expected preview or download"},
		{"unknown command", []string{"fetch", "--json"}, "expected preview or download"},
		{"no selection", valid, "requires either"},
		{"both", append(append([]string{}, valid...), "--all", "--id", id), "exclusively"},
		{"empty id", append(append([]string{}, valid...), "--id="), "64-character"},
		{"short id", append(append([]string{}, valid...), "--id", "abc"), "64-character"},
		{"url as id", append(append([]string{}, valid...), "--id", "https://arcourts.gov/doc/one"), "64-character"},
		{"missing case", []string{"preview", "--json", "--case-url-template", testTemplate}, "--case requires"},
		{"malformed case", append(baseArgs("preview"), "--case", "prefix60CV-2026-1"), "--case requires"},
		{"template required", []string{"preview", "--json", "--case", testCase}, "must contain"},
		{"template placeholder", append(baseArgs("preview"), "--case-url-template", "https://court.example.invalid/"), "must contain"},
		{"template protocol", append(baseArgs("preview"), "--case-url-template", "file:///{case_number}"), "HTTP(S)"},
		{"template host", append(baseArgs("preview"), "--case-url-template", "https://{case_number}.example.invalid/"), "HTTP(S)"},
		{"template credentials", append(baseArgs("preview"), "--case-url-template", "https://user:SECRET@court.example.invalid/{case_number}"), "without user credentials"},
		{"unknown flag before JSON", []string{"preview", "--typo", "--json"}, "flag provided"},
		{"positional", append(baseArgs("preview"), "extra"), "positional"},
		{"bool separate value", append(baseArgs("preview"), "--headless", "false"), "positional"},
		{"preview selection", append(baseArgs("preview"), "--all"), "download options"},
		{"preview output", append(baseArgs("preview"), "--output", out), "download option"},
		{"output required", append(baseArgs("download"), "--all"), "--output"},
		{"missing directory", append(baseArgs("download"), "--all", "--output", filepath.Join(out, "absent")), "existing directory"},
		{"selection cap", append(append([]string{}, valid...), "--max-documents=1", "--id", id, "--id", strings.Repeat("a", 64)), "selection exceeds"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeService{}
			code, r, stdout, _ := invoke(t, f.factory, tc.args...)
			if code != exitInvalid || !strings.Contains(r.Error, tc.message) || f.previewCalls != 0 || f.closeCalls != 0 {
				t.Fatalf("%d %+v fake=%+v", code, r, f)
			}
			if strings.Contains(stdout, "SECRET") {
				t.Fatal("configuration credential leaked")
			}
		})
	}
	for _, opt := range []string{"--timeout=0", "--timeout=25h", "--page-timeout=-1s", "--page-timeout=6m", "--document-timeout=0", "--document-timeout=6m", "--max-documents=0", "--max-documents=10001", "--max-pdf-bytes=0", "--max-pdf-bytes=1073741825", "--max-attempts=0", "--max-attempts=6"} {
		t.Run(opt, func(t *testing.T) {
			f := &fakeService{}
			code, _, _, _ := invoke(t, f.factory, append(baseArgs("preview"), opt)...)
			if code != exitInvalid || f.previewCalls != 0 {
				t.Fatal("invalid bound accepted")
			}
		})
	}
}

func TestFreshSelectionAndCoverage(t *testing.T) {
	for _, tc := range []struct {
		name      string
		change    func(*arcourt.CasePreview)
		all       bool
		code      int
		downloads int
	}{
		{"unknown or disappeared ID", func(p *arcourt.CasePreview) { p.DocketEntries = p.DocketEntries[1:] }, false, exitInvalid, 0},
		{"changed case", func(p *arcourt.CasePreview) { p.CaseInfo.CaseNumber = "60CV-2026-2" }, true, exitFailure, 0},
		{"empty all", func(p *arcourt.CasePreview) { p.DocketEntries = nil }, true, exitInvalid, 0},
		{"incomplete selected", func(p *arcourt.CasePreview) { p.Discovery.Complete = false }, false, exitSuccess, 1},
		{"incomplete all", func(p *arcourt.CasePreview) { p.Discovery.Complete = false }, true, exitFailure, 1},
		{"truncated all", func(p *arcourt.CasePreview) { p.Discovery.Truncated = true }, true, exitFailure, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := testPreview()
			tc.change(p)
			f := &fakeService{preview: func(context.Context, string) (*arcourt.CasePreview, error) { return p, nil }}
			args := append(baseArgs("download"), "--output", t.TempDir())
			if tc.all {
				args = append(args, "--all")
			} else {
				args = append(args, "--id", arcourt.DocumentID(testPreview().DocketEntries[0].SourceURL))
			}
			code, r, _, stderr := invoke(t, f.factory, args...)
			if code != tc.code || len(f.requests) != tc.downloads || f.closeCalls != 1 {
				t.Fatalf("%d %+v fake=%+v", code, r, f)
			}
			if tc.downloads > 0 && (len(r.Warnings) == 0 || !strings.Contains(stderr, "Warning:")) {
				t.Fatal("missing coverage warning")
			}
		})
	}
}

func TestPartialAndSafeErrors(t *testing.T) {
	f := &fakeService{download: func(_ context.Context, req arcourt.DownloadRequest, _ chan<- arcourt.DownloadEvent) (*arcourt.LocalDownloadResult, error) {
		return &arcourt.LocalDownloadResult{CaseNumber: req.CaseNumber, Partial: true,
			Counts: arcourt.DownloadCounts{Selected: 2, Succeeded: 1, Failed: 1},
			Documents: []arcourt.LocalDocumentResult{
				{DocumentID: arcourt.DocumentID(req.Selection[0].SourceURL), Status: arcourt.DocumentSucceeded, Saved: true},
				{DocumentID: arcourt.DocumentID(req.Selection[1].SourceURL), Status: arcourt.DocumentFailed, Error: "document download or local persistence failed", Err: errors.New("https://example.invalid/?signature=SECRET")},
			}}, errors.New("transport request https://example.invalid/?signature=SECRET")
	}}
	code, r, stdout, stderr := invoke(t, f.factory, append(baseArgs("download"), "--output", t.TempDir(), "--all")...)
	if code != exitFailure || !r.Download.Partial || r.Download.Counts.Succeeded != 1 || r.Download.Counts.Failed != 1 {
		t.Fatalf("%d %+v", code, r)
	}
	if strings.Contains(stdout+stderr, "SECRET") {
		t.Fatal("underlying error leaked")
	}
	if f.closeCalls != 1 {
		t.Fatal("fetcher not closed")
	}
}

func TestSignedURLRotationKeepsSelectableID(t *testing.T) {
	p := testPreview()
	p.DocketEntries = p.DocketEntries[:1]
	p.DocketEntries[0].SourceURL = "https://cdr-prod-cmslegacy-images-bucket.s3.us-gov-west-1.amazonaws.com/fixture.pdf?versionId=1&X-Amz-Signature=OLD"
	p.DocketEntries[0].RequestURL = p.DocketEntries[0].SourceURL
	f := &fakeService{preview: func(context.Context, string) (*arcourt.CasePreview, error) { return p, nil }}
	code, preview, stdout, _ := invoke(t, f.factory, baseArgs("preview")...)
	if code != exitSuccess || strings.Contains(stdout, "OLD") {
		t.Fatal("preview failed or exposed signed URL")
	}
	id := preview.Preview.Documents[0].ID
	p.DocketEntries[0].SourceURL = strings.ReplaceAll(p.DocketEntries[0].SourceURL, "OLD", "NEW")
	p.DocketEntries[0].RequestURL = p.DocketEntries[0].SourceURL
	code, result, _, _ := invoke(t, f.factory, append(baseArgs("download"), "--output", t.TempDir(), "--id", id)...)
	if code != exitSuccess || result.Download.Documents[0].DocumentID != id || !strings.Contains(f.requests[0].Selection[0].RequestURL, "NEW") {
		t.Fatal("stable ID did not resolve to fresh signed URL")
	}
}

func TestHumanReadablePreviewAndDownload(t *testing.T) {
	p := testPreview()
	p.Discovery.Complete = false
	f := &fakeService{preview: func(context.Context, string) (*arcourt.CasePreview, error) { return p, nil }}
	var stdout, stderr bytes.Buffer
	id := arcourt.DocumentID(p.DocketEntries[0].SourceURL)
	args := append(baseArgs("download"), "--output", t.TempDir(), "--id", id, "--json=false")
	if code := run(context.Background(), args, noEnv, &stdout, &stderr, f.factory); code != exitSuccess {
		t.Fatalf("code=%d: %s", code, stderr.String())
	}
	for _, expected := range []string{testCase, "Synthetic v Fixture", id, "01/02/2026", "Synthetic order", "Selected 1; succeeded 1"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("text output missing %q: %s", expected, stdout.String())
		}
	}
	if !strings.Contains(stderr.String(), "Discovery is incomplete") || strings.Contains(stdout.String()+stderr.String(), "SECRET") {
		t.Fatal("warning missing or request URL leaked")
	}
}

func TestCancellationAndTimeout(t *testing.T) {
	for _, stage := range []string{"preview", "download", "timeout"} {
		t.Run(stage, func(t *testing.T) {
			started := make(chan struct{})
			f := &fakeService{}
			if stage == "download" {
				f.download = func(ctx context.Context, req arcourt.DownloadRequest, events chan<- arcourt.DownloadEvent) (*arcourt.LocalDownloadResult, error) {
					events <- arcourt.DownloadEvent{Kind: arcourt.DownloadStarted, DocumentID: arcourt.DocumentID(req.Selection[0].SourceURL)}
					close(started)
					<-ctx.Done()
					return &arcourt.LocalDownloadResult{CaseNumber: req.CaseNumber, Counts: arcourt.DownloadCounts{Selected: 2, Canceled: 2}}, ctx.Err()
				}
			} else {
				f.preview = func(ctx context.Context, _ string) (*arcourt.CasePreview, error) {
					close(started)
					<-ctx.Done()
					return nil, ctx.Err()
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			args := append(baseArgs("download"), "--output", t.TempDir(), "--all")
			if stage == "timeout" {
				args = append(args, "--timeout=100ms")
			}
			var stdout, stderr bytes.Buffer
			done := make(chan int, 1)
			go func() { done <- run(ctx, args, noEnv, &stdout, &stderr, f.factory) }()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("operation never started")
			}
			if stage != "timeout" {
				cancel()
			}
			select {
			case code := <-done:
				want := exitCanceled
				if stage == "timeout" {
					want = exitFailure
				}
				var r response
				if err := json.Unmarshal(stdout.Bytes(), &r); err != nil {
					t.Fatal(err)
				}
				if code != want || r.ExitCode != want || f.closeCalls != 1 {
					t.Fatalf("%d %+v fake=%+v", code, r, f)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("cancellation did not finish")
			}
		})
	}
}

func TestConfigurationPrecedence(t *testing.T) {
	out := t.TempDir()
	env := map[string]string{"ARCOURT_CASE_URL_TEMPLATE": testTemplate, "ARCOURT_BROWSER": "environment browser", "ARCOURT_OUTPUT": out}
	getenv := func(key string) string { return env[key] }
	o, err := parseOptions([]string{"download", "--case", " 60cv-2026-1 ", "--all"}, getenv)
	if err != nil || o.browser.CaseURLTemplate != testTemplate || o.browser.ExecutablePath != env["ARCOURT_BROWSER"] || o.output != out || !o.browser.Headless || o.caseNumber != testCase {
		t.Fatalf("%+v %v", o, err)
	}
	other := t.TempDir()
	o, err = parseOptions([]string{"download", "--case", testCase, "--all", "--case-url-template", "https://other.example.invalid?case={case_number}", "--browser", "", "--output", other, "--headless=false", "--timeout=1m", "--page-timeout=30s", "--document-timeout=20s", "--max-documents=2", "--max-pdf-bytes=1024", "--max-attempts=1"}, getenv)
	if err != nil || o.browser.ExecutablePath != "" || o.output != other || o.browser.Headless || o.timeout != time.Minute || o.browser.PageTimeout != 30*time.Second || o.browser.DocumentHTTP.AttemptTimeout != 20*time.Second || o.browser.MaxDocuments != 2 || o.browser.DocumentHTTP.MaxPDFBytes != 1024 || o.browser.DocumentHTTP.MaxAttempts != 1 {
		t.Fatalf("%+v %v", o, err)
	}
	_, err = parseOptions([]string{"preview", "--case", testCase, "--case-url-template="}, getenv)
	if err == nil {
		t.Fatal("empty flag failed to override environment")
	}
	o, err = parseOptions(append(baseArgs("download"), "--all", "--output", "."), noEnv)
	if err != nil || !filepath.IsAbs(o.output) {
		t.Fatalf("relative output not resolved: %+v %v", o, err)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("closed pipe") }

func TestHelpCleanupAndOutputFailure(t *testing.T) {
	f := &fakeService{}
	code, r, _, _ := invoke(t, f.factory, "preview", "--help", "--json")
	if code != exitSuccess || r.Help == "" || f.closeCalls != 0 {
		t.Fatal("help constructed a browser")
	}
	for _, mode := range []string{"json", "text"} {
		args := baseArgs("preview")
		if mode == "text" {
			args = append(args, "--json=false")
		}
		if code := run(context.Background(), args, noEnv, failingWriter{}, io.Discard, f.factory); code != exitFailure {
			t.Fatal("output failure reported success")
		}
	}
	f.closeErr = errors.New("SECRET")
	code, r, stdout, stderr := invoke(t, f.factory, baseArgs("preview")...)
	if code != exitFailure || !strings.Contains(r.Error, "cleanup failed") || strings.Contains(stdout+stderr, "SECRET") {
		t.Fatalf("%d %+v", code, r)
	}
	var textOut bytes.Buffer
	f.closeErr = nil
	if code := run(context.Background(), []string{"--help"}, noEnv, &textOut, io.Discard, f.factory); code != 0 || !strings.Contains(textOut.String(), "Usage:") {
		t.Fatal("text help failed")
	}
}
