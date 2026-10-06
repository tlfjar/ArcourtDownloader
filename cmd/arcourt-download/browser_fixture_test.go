package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tlfjar/ArcourtDownloader/arcourt"
)

// Opt-in, synthetic, local-only integration of the CLI with the unchanged real
// service, installed browser, production document client, and filesystem writer.
// The transport hooks are test-only and are never exposed as CLI options.
func TestCLIBrowserFixtures(t *testing.T) {
	if os.Getenv("ARCOURT_CLI_FIXTURES") != "1" {
		t.Skip("set ARCOURT_CLI_FIXTURES=1 to run the local CLI workflow with an installed browser")
	}
	const pdf = "%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF\n"
	const html = `<!doctype html><title>Synthetic case</title>
<h1 data-testid="case-title">Fixture v Fixture</h1><h2>Case Summary</h2>
<p>Case number: 60CV-2026-1</p><h2>Docket Entries</h2><table>
<tr><th>Date</th><th>Description</th><th>Document</th></tr>
<tr><td>01/02/2026</td><td>Synthetic order</td><td><a href="http://arcourts.gov/one.pdf">PDF</a></td></tr>
<tr><td>01/03/2026</td><td>Synthetic notice</td><td><a href="http://arcourts.gov/two.pdf">PDF</a></td></tr>
</table>`
	var transfers atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".pdf") {
			transfers.Add(1)
			w.Header().Set("Content-Type", "application/pdf")
			io.WriteString(w, pdf)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, html)
	}))
	defer server.Close()
	factory := func(cfg arcourt.BrowserFetcherConfig) (service, func() error, error) {
		cfg.DocumentHTTP.LookupIP = func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		}
		cfg.DocumentHTTP.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		}
		return browserService(cfg)
	}
	args := []string{"preview", "--case", testCase, "--case-url-template", server.URL + "/case/{case_number}", "--json", "--page-timeout=20s"}
	if browser := os.Getenv("ARCOURT_BROWSER_EXECUTABLE"); browser != "" {
		args = append(args, "--browser", browser)
	}
	code, preview, _, _ := invoke(t, factory, args...)
	if code != exitSuccess || preview.Preview == nil || len(preview.Preview.Documents) != 2 || len(preview.Warnings) == 0 {
		t.Fatalf("preview: %d %+v", code, preview)
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(base, "CLI fixture output with spaces")
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	args[0] = "download"
	args = append(args, "--output", out, "--id", preview.Preview.Documents[1].ID)
	code, downloaded, _, _ := invoke(t, factory, args...)
	if code != exitSuccess || downloaded.Download == nil || downloaded.Download.Counts.Succeeded != 1 || transfers.Load() != 1 {
		t.Fatalf("download: %d %+v transfers=%d", code, downloaded, transfers.Load())
	}
	result := downloaded.Download
	data, err := os.ReadFile(filepath.Join(result.Directory, result.Documents[0].Filename))
	if err != nil || string(data) != pdf {
		t.Fatalf("saved PDF mismatch: %v", err)
	}
	digest := sha256.Sum256(data)
	if result.Documents[0].SHA256 != hex.EncodeToString(digest[:]) || result.Documents[0].DocumentID != preview.Preview.Documents[1].ID {
		t.Fatal("saved identity/hash mismatch")
	}
	manifestData, err := os.ReadFile(result.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest arcourt.DownloadManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil || len(manifest.Documents) != 1 || manifest.CaseNumber != testCase {
		t.Fatalf("manifest: %+v %v", manifest, err)
	}
	code, repeated, _, _ := invoke(t, factory, args...)
	if code != exitSuccess || repeated.Download.Counts.Skipped != 1 || repeated.Download.Documents[0].SkipReason != arcourt.SkipVerified || transfers.Load() != 1 {
		t.Fatalf("verified repeat: %d %+v transfers=%d", code, repeated, transfers.Load())
	}
	// --all adds the other document but cannot claim a complete docket.
	args = append(args[:len(args)-2], "--all")
	code, all, _, _ := invoke(t, factory, args...)
	if code != exitFailure || all.Download.Counts.Succeeded != 1 || all.Download.Counts.Skipped != 1 || transfers.Load() != 2 || len(all.Warnings) == 0 {
		t.Fatalf("all: %d %+v transfers=%d", code, all, transfers.Load())
	}
	t.Log("CLI preview -> explicit ID -> verified repeat -> incomplete --all passed with real local PDF transfers, hashes, manifests, and an output path containing spaces")
}
