//go:build desktopfixture && !bindings

package main

// This server and its transport seams are excluded from normal builds.
// Never distribute the separately named fixture executable.
import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tlfjar/ArcourtDownloader/arcourt"
	"github.com/tlfjar/ArcourtDownloader/desktop/internal/app"
)

func configure(_ app.PreferenceStore) (app.Factory, app.PreferenceStore, func()) {
	dir := os.Getenv("ARCOURT_DESKTOP_FIXTURE_DIR")
	if !filepath.IsAbs(dir) {
		panic("fixture build requires ARCOURT_DESKTOP_FIXTURE_DIR")
	}
	var mu sync.Mutex
	loads := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveFixtureUI(w, r) {
			return
		}
		if strings.HasSuffix(r.URL.Path, ".pdf") {
			w.Header().Set("Content-Type", "application/pdf")
			if strings.HasSuffix(r.URL.Path, "slow.pdf") {
				io.WriteString(w, "%PDF-1.7\npartial")
				w.(http.Flusher).Flush()
				_ = os.WriteFile(filepath.Join(dir, "slow-started"), []byte("started"), 0600)
				<-r.Context().Done()
				return
			}
			io.WriteString(w, "%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF\n")
			return
		}
		number := strings.TrimPrefix(r.URL.Path, "/case/")
		mu.Lock()
		loads[number]++
		count := loads[number]
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		if number == "60CV-2026-5" {
			// Deliberately disagree with the requested case for the identity gate.
			number = "60CV-2026-999"
		}
		fmt.Fprintf(w, `<!doctype html><title>Synthetic case</title><h1 data-testid="case-title">Fixture v Fixture</h1><h2>Case Summary</h2><p>Case number: %s</p><h2>Docket Entries</h2><table><tr><th>Date</th><th>Description</th><th>Document</th></tr><tr><td>01/02/2026</td><td>Synthetic order</td><td><a href="http://arcourts.gov/one.pdf?signature=DO_NOT_DISPLAY">PDF</a></td></tr>`, number)
		if number != "60CV-2026-2" || count == 1 {
			second := "two.pdf"
			if number == "60CV-2026-3" {
				second = "slow.pdf"
			}
			fmt.Fprintf(w, `<tr><td>01/03/2026</td><td>Synthetic notice</td><td><a href="http://arcourts.gov/%s">PDF</a></td></tr>`, second)
		}
		io.WriteString(w, "</table>")
	}))
	if err := os.WriteFile(filepath.Join(dir, "ui-address"), []byte(server.URL), 0600); err != nil {
		panic(err)
	}
	store := app.PreferenceStore{Path: filepath.Join(dir, "settings.json")}
	if err := store.Save(app.Preferences{CaseURLTemplate: server.URL + "/case/{case_number}"}); err != nil {
		panic(err)
	}
	factory := func(p app.Preferences) (app.Service, func() error, string, error) {
		f, err := arcourt.NewBrowserFetcher(arcourt.BrowserFetcherConfig{CaseURLTemplate: p.CaseURLTemplate, ExecutablePath: p.BrowserOverride, Headless: true, DocumentHTTP: arcourt.DocumentHTTPConfig{
			LookupIP: func(context.Context, string) ([]net.IPAddr, error) {
				return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
			},
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
			},
		}})
		if err != nil {
			return nil, nil, "", err
		}
		s, err := arcourt.NewDownloadService(f)
		if err != nil {
			_ = f.Close()
			return nil, nil, "", err
		}
		b := f.BrowserInfo()
		return s, f.Close, b.Name + " · " + b.ExecutablePath, nil
	}
	return factory, store, server.Close
}

func webviewDirectory(_ string, store app.PreferenceStore) string {
	return filepath.Join(filepath.Dir(store.Path), "WebView2")
}
