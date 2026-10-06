package arcourt

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// This opt-in test uses installed browsers and synthetic loopback pages only.
func TestBrowserSmoke(t *testing.T) {
	if os.Getenv("ARCOURT_BROWSER_SMOKE") != "1" {
		t.Skip("set ARCOURT_BROWSER_SMOKE=1 to launch an installed browser")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!doctype html><title>Synthetic case</title><h1 data-testid="case-title">Fixture v Fixture</h1><h2>Case Summary</h2><p>Case number: 60CV-2026-1</p><h2>Docket Entries</h2>`)
	}))
	defer server.Close()
	f, err := NewBrowserFetcher(BrowserFetcherConfig{
		CaseURLTemplate: server.URL + "/case/{case_number}",
		ExecutablePath:  os.Getenv("ARCOURT_BROWSER_EXECUTABLE"),
		Headless:        true, PageTimeout: 20 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	})
	t.Logf("selected: %+v", f.BrowserInfo())
	var profiles []string
	var browsers []*chromedp.Browser
	f.launch = func(ctx context.Context, info BrowserInfo, headless bool, profile string) (context.Context, func(), error) {
		profiles = append(profiles, profile)
		ctx, stop, err := launchChromedp(ctx, info, headless, profile)
		if err == nil {
			browsers = append(browsers, chromedp.FromContext(ctx).Browser)
		}
		return ctx, stop, err
	}
	preview, err := f.PreviewCaseDocuments(context.Background(), "60CV-2026-1")
	if err != nil {
		t.Fatal(err)
	}
	if preview.CaseInfo.CaseTitle != "Fixture v Fixture" {
		t.Fatalf("unexpected fixture preview: %+v", preview)
	}
	requireProfileRemoved(t, profiles[0])

	// Inspect actual launch flags, then shut down while this second session is live.
	ctx, cleanup, err := f.openBrowser(context.Background(), 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	var commandLine string
	if err := chromedp.Run(ctx, chromedp.Navigate("chrome://version/"), chromedp.Text("#command_line", &commandLine, chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(commandLine, "--no-sandbox") || strings.Contains(commandLine, "--ignore-certificate-errors") || !strings.Contains(commandLine, "--headless") || !strings.Contains(commandLine, profiles[1]) {
		t.Fatal("actual browser flags did not preserve headless, isolated profile, sandbox, and TLS configuration")
	}
	// An untrusted TLS fixture must fail navigation.
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "untrusted") }))
	defer tlsServer.Close()
	if err := chromedp.Run(ctx, chromedp.Navigate(tlsServer.URL)); err == nil || !strings.Contains(err.Error(), "ERR_CERT_AUTHORITY_INVALID") {
		t.Fatalf("expected TLS rejection, got %v", err)
	}
	if profiles[0] == profiles[1] {
		t.Fatal("profile was reused")
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	for _, profile := range profiles {
		requireProfileRemoved(t, profile)
	}
	for _, browser := range browsers {
		select {
		case <-browser.LostConnection:
		default:
			t.Fatal("owned browser connection is still open")
		}
	}
	t.Log("synthetic preview, isolated launches, TLS rejection, session close, application shutdown, and profile removal passed")
}
