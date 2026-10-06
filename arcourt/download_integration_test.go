package arcourt

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalFetcherIntegrationAndNetworkCancellation(t *testing.T) {
	started := make(chan struct{})
	cfg := fixtureHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		if r.URL.Path == "/two.pdf" {
			io.WriteString(w, "%PDF-1.7\npartial")
			w.(http.Flusher).Flush()
			close(started)
			<-r.Context().Done()
			return
		}
		io.WriteString(w, fixturePDF)
	})
	f := fixtureFetcher(t, discoveryFixture(), BrowserFetcherConfig{DocumentHTTP: cfg})
	s, err := NewDownloadService(f)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Preview(context.Background(), "60CV-2026-1")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	var r *LocalDownloadResult
	output, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		r, err = s.Download(ctx, DownloadRequest{CaseNumber: p.CaseInfo.CaseNumber, Selection: p.DocketEntries, OutputDirectory: output}, nil)
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("transfer never started")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation did not interrupt network I/O")
	}
	if !errors.Is(err, context.Canceled) || !r.Partial {
		t.Fatalf("result=%+v err=%v", r, err)
	}
	assertLocalCounts(t, r, DownloadCounts{Selected: 3, Succeeded: 1, Canceled: 2})
	data, err := os.ReadFile(filepath.Join(r.Directory, r.Documents[0].Filename))
	if err != nil || string(data) != fixturePDF {
		t.Fatal("first committed PDF lost")
	}
	assertNoIncompleteTemps(t, r.Directory)
}

func TestLocalFetcherIntegrationSelectionLimit(t *testing.T) {
	cfg := fixtureHTTP(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, fixturePDF) })
	f := fixtureFetcher(t, discoveryFixture(), BrowserFetcherConfig{DocumentHTTP: cfg, MaxDocuments: 1})
	s, err := NewDownloadService(f)
	if err != nil {
		t.Fatal(err)
	}
	var entries []DocketEntry
	for _, row := range discoveryFixture().Rows {
		entries = append(entries, entryFromRow(row))
	}
	entries = append(entries, DocketEntry{SourceURL: fixtureHost + "/missing"})
	output, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Download(context.Background(), DownloadRequest{CaseNumber: "60CV-2026-1", Selection: entries, OutputDirectory: output}, nil)
	if !errors.Is(err, ErrDocumentsIncomplete) {
		t.Fatal(err)
	}
	assertLocalCounts(t, r, DownloadCounts{Selected: 4, Succeeded: 1, Skipped: 2, Unavailable: 1})
	if r.Documents[1].SkipReason != SkipLimit || !r.Partial {
		t.Fatal("limit/partial not exposed")
	}
}
