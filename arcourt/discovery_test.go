package arcourt

// Case identities in these tests are fabricated; no live case data is used.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"
)

func fixtureFetcher(t *testing.T, d caseDiscovery, cfg BrowserFetcherConfig) *BrowserFetcher {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	f := &BrowserFetcher{cfg: cfg, shutdown: ctx, cancelShutdown: cancel,
		loadCase: func(context.Context, string) (caseDiscovery, error) { return d, nil },
	}
	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	})
	return f
}

func discoveryFixture() caseDiscovery {
	return caseDiscovery{
		Info: CaseInfo{CaseNumber: "60CV-2026-1", CaseTitle: "Fixture v Fixture", County: "Fixture County", Judge: "Fixture Judge", Parties: []Party{{Name: "Fixture", Role: "Plaintiff"}}},
		Rows: []linkRow{
			{URL: fixtureHost + "/one.pdf?z=2&a=1", SourceURL: fixtureHost + "/documents/one", Description: "Order", FilingDate: "01/02/2026"},
			{URL: fixtureHost + "/two.pdf", SourceURL: fixtureHost + "/documents/two", Description: "Notice", FilingDate: "02/03/2026"},
			{URL: fixtureHost + "/three.pdf", SourceURL: fixtureHost + "/documents/three", Description: "Motion", FilingDate: "03/04/2026"},
		},
		Coverage: DiscoveryCoverage{Complete: true}, // only the injected fixture is known complete
	}
}

func TestCaseIdentityBothPaths(t *testing.T) {
	for _, tc := range []struct {
		requested, extracted string
		want                 error
	}{
		{"60CV-2026-1", "60CV-2026-2", ErrCaseNumberMismatch},
		{"60CV-2026-1", "", ErrCaseNumberMismatch},
		{"60CV-2026-1", "prefix 60CV-2026-1", ErrCaseNumberMismatch},
		{"60CV-2026-1", "60CV-2026-1 suffix", ErrCaseNumberMismatch},
		{"prefix 60CV-2026-1", "60CV-2026-1", ErrInvalidCaseNumber},
		{"60CV-2026-1 suffix", "60CV-2026-1", ErrInvalidCaseNumber},
		{"case-abc", "", ErrInvalidCaseNumber},
		{"", "60CV-2026-1", ErrInvalidCaseNumber},
		{"60CV-2026-1\n60CV-2026-2", "60CV-2026-1", ErrInvalidCaseNumber},
	} {
		t.Run(tc.requested+"/"+tc.extracted, func(t *testing.T) {
			d := discoveryFixture()
			d.Info.CaseNumber = tc.extracted
			f := fixtureFetcher(t, d, BrowserFetcherConfig{})
			preview, err := f.PreviewCaseDocuments(context.Background(), tc.requested)
			if preview != nil || !errors.Is(err, tc.want) {
				t.Fatalf("preview=%+v, err=%v", preview, err)
			}
			sink := &testSink{}
			result, err := f.DownloadCaseDocuments(context.Background(), tc.requested, DocumentSelection{SourceURLs: []string{d.Rows[0].SourceURL}}, sink)
			if !errors.Is(err, tc.want) || len(sink.writers) != 0 || len(result.Outcomes) != 1 || result.Outcomes[0].Status != DocumentFailed {
				t.Fatalf("download=%+v, err=%v", result, err)
			}
		})
	}
	for _, number := range []string{"60CV-2026-1", "99ZZZ-99-1", "60DR-24-42", " 60cv-2026-1 "} {
		if !looksLikeCaseNumber(number) {
			t.Errorf("supported example rejected: %q", number)
		}
	}
}

func TestPreviewCoverageAndMetadata(t *testing.T) {
	d := discoveryFixture()
	d.Coverage = DiscoveryCoverage{PaginationDetected: true, VirtualizationDetected: true, Limitations: []string{"fixture rendered rows only"}}
	f := fixtureFetcher(t, d, BrowserFetcherConfig{MaxDocuments: 2})
	preview, err := f.PreviewCaseDocuments(context.Background(), " 60cv-2026-1 ")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.DocketEntries) != 2 || !preview.Discovery.Truncated || preview.Discovery.Complete || preview.Discovery.TotalDocuments != nil || preview.Discovery.DiscoverableDocuments != 3 || !preview.Discovery.PaginationDetected || !preview.Discovery.VirtualizationDetected {
		t.Fatalf("preview=%+v", preview)
	}
	if !reflect.DeepEqual(preview.CaseInfo, d.Info) {
		t.Fatal("case metadata lost")
	}
	entry := preview.DocketEntries[0]
	if entry.RequestURL != d.Rows[0].URL || entry.SourceURL != d.Rows[0].SourceURL || entry.FilingDate != "01/02/2026" || entry.DocketDescription != "Order" {
		t.Fatalf("entry=%+v", entry)
	}
}

func TestSelectionEmptyAllAndCanonicalDuplicates(t *testing.T) {
	d := discoveryFixture()
	f := fixtureFetcher(t, d, BrowserFetcherConfig{})
	f.loadCase = func(context.Context, string) (caseDiscovery, error) {
		t.Fatal("empty selection loaded browser")
		return caseDiscovery{}, nil
	}
	for _, ids := range [][]string{nil, {}, {" ", ""}} {
		result, err := f.DownloadCaseDocuments(context.Background(), "60CV-2026-1", DocumentSelection{SourceURLs: ids}, nil)
		if err != nil || len(result.Outcomes) != 0 {
			t.Fatalf("empty=%+v, err=%v", result, err)
		}
	}
	if _, err := f.DownloadCaseDocuments(context.Background(), "60CV-2026-1", DocumentSelection{All: true, SourceURLs: []string{"one"}}, &testSink{}); err == nil {
		t.Fatal("ambiguous selection accepted")
	}
	f.loadCase = func(context.Context, string) (caseDiscovery, error) { return d, nil }
	f.cfg.DocumentHTTP = fixtureHTTP(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, fixturePDF) })
	result, err := f.DownloadCaseDocuments(context.Background(), "60CV-2026-1", DocumentSelection{All: true}, &testSink{})
	if err != nil || len(result.Outcomes) != 3 {
		t.Fatalf("All=%+v, err=%v", result, err)
	}
	for _, out := range result.Outcomes {
		if out.Status != DocumentSucceeded {
			t.Fatalf("outcome=%+v", out)
		}
	}
	d.Rows[0].SourceURL = fixtureHost + "/documents/one?a=1&b=2"
	result, err = f.DownloadCaseDocuments(context.Background(), "60CV-2026-1", DocumentSelection{SourceURLs: []string{
		"HTTP://CASEINFONEW.ARCOURTS.GOV:80/documents/one/?b=2&a=1#fragment",
		d.Rows[0].SourceURL,
	}}, &testSink{})
	if err != nil || len(result.Outcomes) != 1 || result.Outcomes[0].Document.RequestURL != d.Rows[0].URL {
		t.Fatalf("canonical=%+v, err=%v", result, err)
	}
}

func TestSelectionUnavailableLimitAndPartialFailure(t *testing.T) {
	d := discoveryFixture()
	var requested []string
	var requestedMu sync.Mutex
	cfg := fixtureHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		requestedMu.Lock()
		requested = append(requested, r.URL.Path)
		requestedMu.Unlock()
		if r.URL.Path == "/two.pdf" {
			w.WriteHeader(403)
			return
		}
		io.WriteString(w, fixturePDF)
	})
	f := fixtureFetcher(t, d, BrowserFetcherConfig{MaxDocuments: 3, DocumentHTTP: cfg})
	ids := []string{d.Rows[0].SourceURL, d.Rows[1].SourceURL, fixtureHost + "/missing", d.Rows[2].SourceURL}
	result, err := f.DownloadCaseDocuments(context.Background(), "60CV-2026-1", DocumentSelection{SourceURLs: ids}, &testSink{})
	if !errors.Is(err, ErrDocumentsIncomplete) || len(result.Outcomes) != 4 {
		t.Fatalf("partial=%+v, err=%v", result, err)
	}
	want := []DocumentStatus{DocumentSucceeded, DocumentFailed, DocumentUnavailable, DocumentSucceeded}
	for i, out := range result.Outcomes {
		if out.Status != want[i] {
			t.Fatalf("outcome %d=%+v", i, out)
		}
	}
	requestedMu.Lock()
	actualRequests := append([]string(nil), requested...)
	requestedMu.Unlock()
	if !reflect.DeepEqual(actualRequests, []string{"/one.pdf", "/two.pdf", "/three.pdf"}) {
		t.Fatalf("requests=%v", actualRequests)
	}
	f.cfg.MaxDocuments = 1
	result, err = f.DownloadCaseDocuments(context.Background(), "60CV-2026-1", DocumentSelection{SourceURLs: ids}, &testSink{})
	if !errors.Is(err, ErrDocumentsIncomplete) || result.Outcomes[1].Status != DocumentSkippedLimit || result.Outcomes[2].Status != DocumentUnavailable || result.Outcomes[3].Status != DocumentSkippedLimit {
		t.Fatalf("limit=%+v, err=%v", result, err)
	}
	// Fresh discovery still finds an explicit selection beyond preview's cap.
	result, err = f.DownloadCaseDocuments(context.Background(), "60CV-2026-1", DocumentSelection{SourceURLs: []string{d.Rows[2].SourceURL}}, &testSink{})
	if err != nil || result.Outcomes[0].Status != DocumentSucceeded {
		t.Fatalf("beyond preview=%+v, err=%v", result, err)
	}
	d.Coverage.Complete = false
	f.loadCase = func(context.Context, string) (caseDiscovery, error) { return d, nil }
	result, err = f.DownloadCaseDocuments(context.Background(), "60CV-2026-1", DocumentSelection{All: true}, &testSink{})
	if !errors.Is(err, ErrIncompleteDiscovery) || !errors.Is(err, ErrDocumentsIncomplete) || len(result.Outcomes) != 3 || result.Outcomes[0].Status != DocumentSucceeded || result.Outcomes[2].Status != DocumentSkippedLimit {
		t.Fatalf("incomplete All=%+v, err=%v", result, err)
	}
}

func TestBatchCancellationPreservesEarlierSuccess(t *testing.T) {
	d := discoveryFixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requests := 0
	cfg := fixtureHTTP(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, fixturePDF) })
	f := fixtureFetcher(t, d, BrowserFetcherConfig{DocumentHTTP: cfg})
	sink := &testSink{onWrite: func() {
		requests++
		if requests == 2 {
			cancel()
		}
	}}
	result, err := f.DownloadCaseDocuments(ctx, "60CV-2026-1", DocumentSelection{All: true}, sink)
	if !errors.Is(err, context.Canceled) || len(result.Outcomes) != 3 || result.Outcomes[0].Status != DocumentSucceeded || result.Outcomes[1].Status != DocumentCanceled || result.Outcomes[2].Status != DocumentCanceled {
		t.Fatalf("cancellation=%+v, err=%v", result, err)
	}
	if len(sink.writers) != 2 || !sink.writers[0].committed || !sink.writers[1].aborted {
		t.Fatal("cancellation lost success or published failure")
	}
}

func TestCloseCancelsDocumentTransfer(t *testing.T) {
	started := make(chan struct{})
	cfg := fixtureHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "%PDF-1.7\n")
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	})
	f := fixtureFetcher(t, discoveryFixture(), BrowserFetcherConfig{DocumentHTTP: cfg})
	done := make(chan error, 1)
	go func() {
		_, err := f.DownloadCaseDocuments(context.Background(), "60CV-2026-1", DocumentSelection{All: true}, &testSink{})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("transfer not started")
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close left transfer running")
	}
}

func TestCanonicalRowDeduplicationPreservesRequest(t *testing.T) {
	rows := normalizeLinkRows([]linkRow{
		{URL: fixtureHost + "/one.pdf?z=2&a=1", SourceURL: fixtureHost + "/documents/one?b=2&a=1"},
		{URL: fixtureHost + "/one.pdf?a=1&z=2", SourceURL: fixtureHost + "/documents/one?a=1&b=2", Description: "Order", FilingDate: "01/01/2026"},
	})
	if len(rows) != 1 || rows[0].URL != fixtureHost+"/one.pdf?z=2&a=1" || rows[0].Description != "Order" || rows[0].FilingDate != "01/01/2026" {
		t.Fatalf("rows=%+v", rows)
	}
}
