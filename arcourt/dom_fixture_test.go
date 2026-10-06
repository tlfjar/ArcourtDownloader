package arcourt

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// Browser-dependent parsing tests are opt-in and never contact a court. Fixtures
// are synthetic markup, with no personal documents or real signed URLs.
func TestDocketDOMFixtures(t *testing.T) {
	if os.Getenv("ARCOURT_DOM_FIXTURES") != "1" {
		t.Skip("set ARCOURT_DOM_FIXTURES=1 to run synthetic DOM parsing in an installed browser")
	}
	page, err := os.ReadFile("testdata/docket.html")
	if err != nil {
		t.Fatal(err)
	}
	var currentPage atomic.Value
	currentPage.Store(string(page))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, currentPage.Load().(string))
	}))
	defer server.Close()
	f, err := NewBrowserFetcher(BrowserFetcherConfig{CaseURLTemplate: server.URL + "/case/{case_number}", ExecutablePath: os.Getenv("ARCOURT_BROWSER_EXECUTABLE"), Headless: true, PageTimeout: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ctx, cleanup, err := f.openBrowser(context.Background(), 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	for _, tc := range []struct {
		name, replace string
		want          error
	}{
		{"metadata and links", "60CV-2026-1", nil},
		{"wrong rendered case", "60CV-2026-2", ErrCaseNumberMismatch},
		{"malformed labelled case", "prefix 60CV-2026-1 suffix", ErrCaseNumberMismatch},
		{"request URL is not identity evidence", "unavailable", ErrCaseNumberMismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			currentPage.Store(strings.ReplaceAll(string(page), "60CV-2026-1", tc.replace))
			var d caseDiscovery
			if err := chromedp.Run(ctx, chromedp.Navigate(server.URL+"/case/60CV-2026-1"), chromedp.WaitReady("body", chromedp.ByQuery),
				chromedp.Evaluate(extractCaseInfoJS, &d.Info), chromedp.Evaluate(extractDocketRowsJS, &d.Rows), chromedp.Evaluate(inspectDocketCoverageJS, &d.Coverage)); err != nil {
				t.Fatal(err)
			}
			if err := validateCaseNumberMatch(d.Info.CaseNumber, "60CV-2026-1"); !errors.Is(err, tc.want) {
				t.Fatalf("identity=%q, err=%v", d.Info.CaseNumber, err)
			}
			if d.Info.CaseTitle != "Fixture v Fixture" || d.Info.County != "Fixture County" || d.Info.Judge != "Fixture Judge" || len(d.Info.Parties) != 1 || d.Info.Parties[0].Role != "Plaintiff" {
				t.Fatalf("metadata=%+v", d.Info)
			}
			rows := normalizeLinkRows(d.Rows)
			if len(rows) != 2 || rows[0].URL != server.URL+"/files/order.pdf?z=2&a=1%2f2" || rows[0].FilingDate != "01/02/2026" || rows[0].Description != "Synthetic order" || rows[1].URL != server.URL+"/opad/api/documents/fixture%20token" {
				t.Fatalf("rows=%+v", rows)
			}
			if !d.Coverage.PaginationDetected || !d.Coverage.VirtualizationDetected || d.Coverage.ReportedDocketRows == nil || *d.Coverage.ReportedDocketRows != 50 || d.Coverage.Complete {
				t.Fatalf("coverage=%+v", d.Coverage)
			}
		})
	}
	// A body case number remains a supported fallback when there is no label.
	currentPage.Store(`<h1>Fixture v Fixture</h1><p>60CV-2026-1</p>`)
	var info CaseInfo
	if err := chromedp.Run(ctx, chromedp.Navigate(server.URL+"/case/60CV-2026-2"), chromedp.Evaluate(extractCaseInfoJS, &info)); err != nil {
		t.Fatal(err)
	}
	if info.CaseNumber != "60CV-2026-1" {
		t.Fatalf("body fallback=%q", info.CaseNumber)
	}
	currentPage.Store(`<h1>Fixture v Fixture</h1>`)
	if err := chromedp.Run(ctx, chromedp.Navigate(server.URL+"/case/60CV-2026-1"), chromedp.Evaluate(extractCaseInfoJS, &info)); err != nil {
		t.Fatal(err)
	}
	if info.CaseNumber != "" {
		t.Fatal("address bar was used as case identity")
	}
	currentPage.Store(`<title>403 Forbidden</title><h1>403 Forbidden</h1>`)
	if err := chromedp.Run(ctx, chromedp.Navigate(server.URL+"/case/60CV-2026-1")); err != nil {
		t.Fatal(err)
	}
	if err := waitForCasePageReady(ctx, "60CV-2026-1"); !errors.Is(err, ErrCaseAccessDenied) || shouldRetryCasePageLoad(err) {
		t.Fatalf("access rejection was not preserved: %v", err)
	}
}
