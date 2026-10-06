package arcourt

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Synthetic identifiers 99ZZZ-99-1 and 99ZZZ-99-2 are invented, not live cases.
// Synthetic OPAD schema: filenames deliberately differ from document IDs, and
// one docket contains multiple attachments normally hidden in a browser menu.
const opadFixture = `{
 "caseId":"99ZZZ-99-1", "caseTitle":"Fixture v Fixture", "courtDesc":"FIXTURE",
 "caseParticipants":[
   {"name":"Fixture Division", "partyType":"JUDGE"},
   {"name":"Fixture Person", "partyType":"PLAINTIFF"}
 ],
 "caseDocuments":[{"documentFileId":"case-file", "documentName":"cover.pdf", "documentDesc":"Cover", "documentUploadDate":"2026-01-02T01:00:00Z"}],
 "caseDockets":[
   {"caseId":"99ZZZ-99-1", "docketDesc":"Order", "docketFilingDate":"2026-07-02T04:30:00Z", "docketDocuments":[
     {"documentFileId":"first", "documentName":"display-name.pdf"},
     {"documentFileId":"second", "documentName":"second.PDF"},
     {"documentFileId":"audio", "documentName":"recording.mp3"}
   ]},
   {"docketDesc":"Duplicate", "docketDocuments":[{"documentFileId":"first", "documentName":"display-name.pdf"}]}
 ]
}`

func TestOPADRoute(t *testing.T) {
	const template = "https://caseinfonew.arcourts.gov/opad/case/{case_number}"
	if got := opadCaseAPI(template, "99ZZZ-99-1"); got != "https://caseinfonew.arcourts.gov/opad/api/cases/99ZZZ-99-1" {
		t.Fatal(got)
	}
	for _, value := range []string{
		strings.ReplaceAll(template, "https:", "http:"),
		strings.ReplaceAll(template, "caseinfonew.arcourts.gov", "example.com"),
		strings.ReplaceAll(template, "caseinfonew.arcourts.gov", "caseinfonew.arcourts.gov.evil.test"),
		strings.ReplaceAll(template, "caseinfonew.arcourts.gov", "user@caseinfonew.arcourts.gov"),
		template + "?", template + "?mode=other", template + "#summary", template + "/",
		strings.ReplaceAll(template, "{case_number}", "99ZZZ-99-1"),
		strings.ReplaceAll(template, "/case/", "/%63ase/"),
	} {
		if got := opadCaseAPI(value, "99ZZZ-99-1"); got != "" {
			t.Errorf("unexpected API routing: %s", value)
		}
	}
}

func TestOPADPreviewAndDownload(t *testing.T) {
	var caseRequests, documentRequests atomic.Int32
	cfg := fixtureHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("credentials forwarded")
		}
		if strings.Contains(r.URL.Path, "/api/cases/") {
			caseRequests.Add(1)
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, opadFixture)
			return
		}
		if r.URL.Path != "/opad/api/documents/second" {
			t.Errorf("wrong file ID: %s", r.URL.Path)
		}
		documentRequests.Add(1)
		io.WriteString(w, fixturePDF)
	})
	f := fixtureFetcher(t, caseDiscovery{}, BrowserFetcherConfig{DocumentHTTP: cfg, MaxDocuments: 2})
	f.loadCase = func(ctx context.Context, number string) (caseDiscovery, error) {
		return f.readOPADCase(ctx, fixtureHost+"/opad/api/cases/"+number, number)
	}
	p, err := f.PreviewCaseDocuments(context.Background(), "99ZZZ-99-1")
	if err != nil {
		t.Fatal(err)
	}
	if p.CaseInfo.CaseTitle != "Fixture v Fixture" || p.CaseInfo.County != "FIXTURE" || p.CaseInfo.Judge != "Fixture Division" || len(p.CaseInfo.Parties) != 1 {
		t.Fatalf("metadata: %+v", p.CaseInfo)
	}
	if len(p.DocketEntries) != 2 || p.Discovery.DiscoverableDocuments != 3 || !p.Discovery.Truncated || p.Discovery.Complete || *p.Discovery.ReportedDocketRows != 2 {
		t.Fatalf("preview: %+v", p)
	}
	if p.DocketEntries[0].FilingDate != "07/01/2026" || p.DocketEntries[0].DocketDescription != "Order" || !strings.HasSuffix(p.DocketEntries[0].RequestURL, "/first") {
		t.Fatalf("entry: %+v", p.DocketEntries[0])
	}
	sink := &testSink{}
	r, err := f.DownloadCaseDocuments(context.Background(), "99ZZZ-99-1", DocumentSelection{SourceURLs: []string{p.DocketEntries[1].SourceURL}}, sink)
	if err != nil || len(r.Outcomes) != 1 || r.Outcomes[0].Status != DocumentSucceeded || caseRequests.Load() != 2 || documentRequests.Load() != 1 {
		t.Fatalf("download=%+v error=%v cases=%d documents=%d", r, err, caseRequests.Load(), documentRequests.Load())
	}
	if len(sink.writers) != 1 || !sink.writers[0].committed {
		t.Fatal("PDF was not committed")
	}
}

func TestOPADRejectsInvalidResponses(t *testing.T) {
	for _, tc := range []struct {
		name, payload, contentType string
		status                     int
		want                       error
	}{
		{"forbidden", "private details", "text/html", 403, ErrCaseAccessDenied},
		{"unauthorized", "private details", "text/html", 401, ErrCaseAccessDenied},
		{"missing", "private details", "text/html", 404, ErrCaseLoadFailed},
		{"html", "<html>private details</html>", "text/html", 200, ErrCaseLoadFailed},
		{"invalid json", "{private details", "application/json", 200, ErrCaseLoadFailed},
		{"wrong case", strings.ReplaceAll(opadFixture, "99ZZZ-99-1", "99ZZZ-99-2"), "application/json", 200, ErrCaseNumberMismatch},
		{"missing case", `{}`, "application/json", 200, ErrCaseNumberMismatch},
		{"schema change", `{"caseId":"99ZZZ-99-1","caseTitle":"Fixture"}`, "application/json", 200, ErrCaseLoadFailed},
		{"wrong nested case", strings.Replace(opadFixture, `"caseId":"99ZZZ-99-1", "docketDesc"`, `"caseId":"99ZZZ-99-2", "docketDesc"`, 1), "application/json", 200, ErrCaseNumberMismatch},
		{"missing file ID", strings.ReplaceAll(opadFixture, `"documentFileId":"first"`, `"documentFileId":""`), "application/json", 200, ErrCaseLoadFailed},
		{"path in file ID", strings.ReplaceAll(opadFixture, `"documentFileId":"first"`, `"documentFileId":"../other"`), "application/json", 200, ErrCaseLoadFailed},
		{"too large", strings.Repeat(" ", 5000), "application/json", 200, ErrCaseLoadFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			cfg := fixtureHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.payload)
			})
			cfg.MaxJSONBytes, cfg.MaxAttempts = 4096, 3
			f := fixtureFetcher(t, caseDiscovery{}, BrowserFetcherConfig{DocumentHTTP: cfg})
			_, err := f.readOPADCase(context.Background(), fixtureHost+"/opad/api/cases/99ZZZ-99-1", "99ZZZ-99-1")
			if !errors.Is(err, tc.want) || requests.Load() != 1 || strings.Contains(err.Error(), "private details") {
				t.Fatalf("error=%v requests=%d", err, requests.Load())
			}
		})
	}
}

func TestOPADRetriesAndCancellation(t *testing.T) {
	var requests atomic.Int32
	cfg := fixtureHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, opadFixture)
	})
	cfg.MaxAttempts = 3
	f := fixtureFetcher(t, caseDiscovery{}, BrowserFetcherConfig{DocumentHTTP: cfg})
	d, err := f.readOPADCase(context.Background(), fixtureHost+"/opad/api/cases/99ZZZ-99-1", "99ZZZ-99-1")
	if err != nil || len(d.Rows) != 4 || requests.Load() != 2 {
		t.Fatalf("discovery=%+v err=%v requests=%d", d, err, requests.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = f.readOPADCase(ctx, fixtureHost+"/opad/api/cases/99ZZZ-99-1", "99ZZZ-99-1")
	if !errors.Is(err, context.Canceled) || requests.Load() != 2 {
		t.Fatalf("err=%v requests=%d", err, requests.Load())
	}

	blocked := make(chan struct{})
	cfg = fixtureHTTP(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done(); close(blocked) })
	f = fixtureFetcher(t, caseDiscovery{}, BrowserFetcherConfig{DocumentHTTP: cfg, PageTimeout: 100 * time.Millisecond})
	_, err = f.readOPADCase(context.Background(), fixtureHost+"/opad/api/cases/99ZZZ-99-1", "99ZZZ-99-1")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("HTTP request outlived deadline")
	}
}

func TestOPADDate(t *testing.T) {
	for value, want := range map[string]string{
		"2026-01-02T01:00:00.000Z": "01/01/2026",
		"2026-07-02T05:30:00Z":     "07/02/2026",
		"2026-01-02T05:30:00Z":     "01/01/2026",
		"invalid":                  "",
	} {
		if got := opadDate(value); got != want {
			t.Errorf("%s: got %s want %s", value, got, want)
		}
	}
}
