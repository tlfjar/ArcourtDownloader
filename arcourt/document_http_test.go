package arcourt

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const fixturePDF = "%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF\n"
const fixtureHost = "http://caseinfonew.arcourts.gov"

type testSink struct {
	writers                                []*testWriter
	openErr, writeErr, commitErr, abortErr error
	onWrite                                func()
	discard                                bool
}
type testWriter struct {
	ctx  context.Context
	sink *testSink
	bytes.Buffer
	committed, aborted bool
	maxWrite           int
}

func (s *testSink) OpenDocument(ctx context.Context, _ DocketEntry) (DocumentWriter, error) {
	if s.openErr != nil {
		return nil, s.openErr
	}
	w := &testWriter{ctx: ctx, sink: s}
	s.writers = append(s.writers, w)
	return w, nil
}
func (w *testWriter) Write(p []byte) (int, error) {
	if w.sink.onWrite != nil {
		w.sink.onWrite()
	}
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if w.sink.writeErr != nil {
		return 0, w.sink.writeErr
	}
	if len(p) > w.maxWrite {
		w.maxWrite = len(p)
	}
	if w.sink.discard {
		return len(p), nil
	}
	return w.Buffer.Write(p)
}
func (w *testWriter) Commit() error {
	if w.sink.commitErr != nil {
		return w.sink.commitErr
	}
	w.committed = true
	return nil
}
func (w *testWriter) Abort() error { w.aborted = true; return w.sink.abortErr }

// Exercise the REAL HTTP transport and policy, routing only an already checked
// public numeric IP to a loopback fixture. Production policy has no test bypass.
func fixtureHTTP(t *testing.T, handler http.HandlerFunc) DocumentHTTPConfig {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return DocumentHTTPConfig{
		MaxAttempts: 1, RetryDelay: time.Millisecond,
		LookupIP: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if address != "93.184.216.34:80" && address != "93.184.216.34:443" {
				return nil, fmt.Errorf("dial was not pinned: %s", address)
			}
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
	}
}

func runDownload(t *testing.T, cfg DocumentHTTPConfig, path string, sink *testSink) DocumentOutcome {
	t.Helper()
	c := newDocumentClient(cfg)
	t.Cleanup(c.http.CloseIdleConnections)
	return c.download(context.Background(), DocketEntry{SourceURL: "fixture", RequestURL: fixtureHost + path}, sink)
}

func TestDocumentDirectAndJSON(t *testing.T) {
	for _, key := range []string{"direct", "url", "downloadUrl", "documentUrl", "signedUrl", "storage", "redirect-base"} {
		t.Run(key, func(t *testing.T) {
			var requests []string
			var requestsMu sync.Mutex
			cfg := fixtureHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				requestsMu.Lock()
				requests = append(requests, r.RequestURI)
				requestsMu.Unlock()
				if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("Referer") != "" {
					t.Error("session or URL leaked")
				}
				if r.URL.Path == "/entry" && key == "redirect-base" {
					http.Redirect(w, r, "/folder/meta", 302)
					return
				}
				if key != "direct" && (r.URL.Path == "/entry" || r.URL.Path == "/folder/meta") {
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					next, jsonKey := "files/doc.pdf?z=2&a=1%2f2", key
					if key == "storage" {
						next, jsonKey = "http://cdr-prod-cmslegacy-images-bucket.s3.us-gov-west-1.amazonaws.com/doc.pdf?z=2&a=1%2f2", "signedUrl"
					}
					if key == "redirect-base" {
						jsonKey = "url"
					}
					fmt.Fprintf(w, `{"%s":%q}`, jsonKey, next)
					return
				}
				w.Header().Set("Content-Type", "application/pdf")
				io.WriteString(w, fixturePDF)
			})
			sink := &testSink{}
			path := "/entry"
			if key == "direct" {
				path += "?z=2&a=1%2f2"
			}
			out := runDownload(t, cfg, path, sink)
			digest := sha256.Sum256([]byte(fixturePDF))
			if out.Status != DocumentSucceeded || out.Bytes != int64(len(fixturePDF)) || out.SHA256 != hex.EncodeToString(digest[:]) {
				t.Fatalf("outcome: %+v", out)
			}
			if len(sink.writers) != 1 || !sink.writers[0].committed || sink.writers[0].aborted || sink.writers[0].String() != fixturePDF {
				t.Fatal("incorrect writer lifecycle or bytes")
			}
			requestsMu.Lock()
			last := requests[len(requests)-1]
			requestsMu.Unlock()
			if !strings.HasSuffix(last, "?z=2&a=1%2f2") {
				t.Fatalf("query changed: %q", last)
			}
			if key == "redirect-base" && !strings.HasPrefix(last, "/folder/files/") {
				t.Fatalf("wrong resolution base: %s", last)
			}
		})
	}
}

func TestDocumentFailures(t *testing.T) {
	for _, tc := range []struct {
		name, ct, body string
		status         int
		length         string
		limit          int64
		want           error
		opened         bool
	}{
		{name: "HTML", ct: "application/pdf", body: "<html>Access denied</html>", want: ErrInvalidPDF},
		{name: "JSON masquerading as PDF", ct: "application/pdf", body: `{"url":"/doc.pdf"}`, want: ErrInvalidPDF},
		{name: "empty PDF", ct: "application/pdf", want: ErrInvalidPDF},
		{name: "missing trailer", body: "%PDF-1.7\n1 0 obj", want: ErrInvalidPDF, opened: true},
		{name: "invalid JSON", ct: "application/json", body: `{"url":`, want: errors.New("invalid document endpoint JSON")},
		{name: "missing URL", ct: "application/json", body: `{}`, want: errors.New("document endpoint JSON has no download URL")},
		{name: "oversize known", body: fixturePDF, limit: 16, want: ErrDocumentTooLarge},
		{name: "short body", body: fixturePDF, length: "999", want: io.ErrUnexpectedEOF, opened: true},
		{name: "unauthorized", status: 401, want: &HTTPStatusError{401}},
		{name: "expired", status: 403, want: &HTTPStatusError{403}},
		{name: "gone", status: 410, want: &HTTPStatusError{410}},
		{name: "missing", status: 404, want: &HTTPStatusError{404}},
		{name: "server error", status: 503, want: &HTTPStatusError{503}},
		{name: "partial", status: 206, body: fixturePDF, want: &HTTPStatusError{206}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := fixtureHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				if tc.ct != "" {
					w.Header().Set("Content-Type", tc.ct)
				}
				if tc.length != "" {
					w.Header().Set("Content-Length", tc.length)
				}
				if tc.status != 0 {
					w.WriteHeader(tc.status)
				}
				io.WriteString(w, tc.body)
			})
			cfg.MaxPDFBytes = tc.limit
			sink := &testSink{}
			out := runDownload(t, cfg, "/entry?synthetic-secret=do-not-log", sink)
			if out.Status != DocumentFailed || out.Bytes != 0 || out.SHA256 != "" {
				t.Fatalf("outcome: %+v", out)
			}
			var status *HTTPStatusError
			if errors.As(tc.want, &status) {
				var got *HTTPStatusError
				if !errors.As(out.Err, &got) || got.StatusCode != status.StatusCode {
					t.Fatalf("err: %v", out.Err)
				}
			} else if !errors.Is(out.Err, tc.want) && out.Err.Error() != tc.want.Error() {
				t.Fatalf("err: %v, want %v", out.Err, tc.want)
			}
			if strings.Contains(out.Err.Error(), "do-not-log") {
				t.Fatal("secret leaked")
			}
			if tc.opened && (len(sink.writers) != 1 || !sink.writers[0].aborted || sink.writers[0].committed) {
				t.Fatal("failed attempt was not discarded")
			}
		})
	}
}

func TestDocumentChunkedAndJSONSizeLimits(t *testing.T) {
	for _, jsonBody := range []bool{false, true} {
		cfg := fixtureHTTP(t, func(w http.ResponseWriter, r *http.Request) {
			if jsonBody {
				w.Header().Set("Content-Type", "application/json")
			}
			w.(http.Flusher).Flush()
			if jsonBody {
				io.WriteString(w, `{"url":"`+strings.Repeat("x", 256)+`"}`)
			} else {
				io.WriteString(w, fixturePDF+strings.Repeat("x", 256))
			}
		})
		cfg.MaxPDFBytes, cfg.MaxJSONBytes = 100, 100
		sink := &testSink{}
		out := runDownload(t, cfg, "/large", sink)
		if !errors.Is(out.Err, ErrDocumentTooLarge) {
			t.Fatalf("outcome: %+v", out)
		}
		for _, w := range sink.writers {
			if !w.aborted || w.committed {
				t.Fatal("oversize output published")
			}
		}
	}
}

func TestDocumentDestinationsAndRedirectBounds(t *testing.T) {
	for _, target := range []string{"http://localhost/x", "http://127.0.0.1/x", "http://10.0.0.1/x", "http://[::1]/x", "http://attacker.example/x", "http://arcourts.gov.attacker.example/x", "http://user:pass@arcourts.gov/x", "http://arcourts.gov:8080/x", "file:///tmp/doc"} {
		for _, mode := range []string{"initial", "redirect", "json"} {
			t.Run(mode+target, func(t *testing.T) {
				var calls atomic.Int32
				cfg := fixtureHTTP(t, func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if mode == "redirect" {
						http.Redirect(w, r, target, 302)
					} else {
						w.Header().Set("Content-Type", "application/json")
						fmt.Fprintf(w, `{"url":%q}`, target)
					}
				})
				c := newDocumentClient(cfg)
				request := fixtureHost + "/entry"
				if mode == "initial" {
					request = target
				}
				out := c.download(context.Background(), DocketEntry{RequestURL: request}, &testSink{})
				if !errors.Is(out.Err, ErrDestinationRejected) {
					t.Fatalf("outcome: %+v", out)
				}
				want := int32(1)
				if mode == "initial" {
					want = 0
				}
				if calls.Load() != want {
					t.Fatal("rejected destination connected")
				}
			})
		}
	}
	for _, mode := range []string{"redirect", "json"} {
		var calls atomic.Int32
		cfg := fixtureHTTP(t, func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			if mode == "redirect" {
				http.Redirect(w, r, "/loop", 302)
			} else {
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"url":"/loop"}`)
			}
		})
		out := runDownload(t, cfg, "/loop", &testSink{})
		if mode == "redirect" && (!errors.Is(out.Err, ErrRedirectLimit) || calls.Load() != 6) {
			t.Fatalf("redirect bound: %+v, %d", out, calls.Load())
		}
		if mode == "json" && (!errors.Is(out.Err, ErrIndirectionLimit) || calls.Load() != 4) {
			t.Fatalf("JSON bound: %+v, %d", out, calls.Load())
		}
	}
	if _, err := validateResolvedDocumentURL("https://arcourts.gov/a", "http://arcourts.gov/b"); !errors.Is(err, ErrDestinationRejected) {
		t.Fatal("HTTPS downgrade allowed")
	}
}

func TestDocumentDNSPolicyAndPinning(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.0.1", "169.254.169.254", "0.0.0.0", "::1", "::ffff:127.0.0.1", "fc00::1", "fe80::1", "224.0.0.1", "100.64.0.1", "198.18.0.1", "240.0.0.1", "64:ff9b::7f00:1", "2002:7f00:1::"} {
		t.Run(ip, func(t *testing.T) {
			dialed := false
			c := newDocumentClient(DocumentHTTPConfig{MaxAttempts: 1,
				LookupIP: func(context.Context, string) ([]net.IPAddr, error) {
					return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP(ip)}}, nil
				},
				DialContext: func(context.Context, string, string) (net.Conn, error) {
					dialed = true
					return nil, errors.New("unexpected dial")
				},
			})
			out := c.download(context.Background(), DocketEntry{RequestURL: fixtureHost + "/doc"}, &testSink{})
			if !errors.Is(out.Err, ErrDestinationRejected) || dialed {
				t.Fatalf("DNS bypass: %+v, dialed=%t", out, dialed)
			}
		})
	}
	// A second resolution after a redirect now returns private space.
	var lookups atomic.Int32
	cfg := fixtureHTTP(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/again", 302) })
	cfg.LookupIP = func(context.Context, string) ([]net.IPAddr, error) {
		if lookups.Add(1) == 1 {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	out := runDownload(t, cfg, "/rebind", &testSink{})
	if !errors.Is(out.Err, ErrDestinationRejected) || lookups.Load() != 2 {
		t.Fatalf("rebind: %+v", out)
	}
}

func TestDocumentRetriesAndWriterFailures(t *testing.T) {
	for _, failure := range []string{"503", "429", "short"} {
		t.Run(failure, func(t *testing.T) {
			var calls atomic.Int32
			cfg := fixtureHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					switch failure {
					case "503":
						w.WriteHeader(503)
					case "429":
						w.WriteHeader(429)
					case "short":
						w.Header().Set("Content-Length", "999")
						io.WriteString(w, fixturePDF)
					}
					return
				}
				io.WriteString(w, fixturePDF)
			})
			cfg.MaxAttempts = 3
			sink := &testSink{}
			out := runDownload(t, cfg, "/entry", sink)
			if out.Status != DocumentSucceeded || out.Attempts != 2 || calls.Load() != 2 {
				t.Fatalf("retry: %+v", out)
			}
			if failure == "short" && (len(sink.writers) != 2 || !sink.writers[0].aborted || sink.writers[1].String() != fixturePDF) {
				t.Fatal("retry appended to partial output")
			}
		})
	}
	for _, stage := range []string{"open", "write", "commit", "abort"} {
		t.Run(stage, func(t *testing.T) {
			cfg := fixtureHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				if stage == "abort" {
					w.Header().Set("Content-Length", "999")
				}
				io.WriteString(w, fixturePDF)
			})
			cfg.MaxAttempts = 3
			sink := &testSink{}
			injected := errors.New("temporary injected writer failure")
			switch stage {
			case "open":
				sink.openErr = injected
			case "write":
				sink.writeErr = injected
			case "commit":
				sink.commitErr = injected
			case "abort":
				sink.abortErr = injected
			}
			out := runDownload(t, cfg, "/entry", sink)
			if out.Status != DocumentFailed || out.Attempts != 1 {
				t.Fatalf("writer retry: %+v", out)
			}
			if !errors.Is(out.Err, injected) {
				t.Fatal("writer failure lost its cause")
			}
			if IsRetryable(out.Err) {
				t.Fatal("writer/cleanup failure must not invite retries")
			}
			if stage != "open" && (len(sink.writers) != 1 || !sink.writers[0].aborted || sink.writers[0].committed) {
				t.Fatal("writer cleanup failed")
			}
		})
	}
}

func TestDocumentCancellationAndTimeout(t *testing.T) {
	for _, mode := range []string{"before", "read", "backoff", "timeout", "read-timeout", "connect-timeout"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var calls atomic.Int32
			cfg := fixtureHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if mode == "backoff" {
					w.WriteHeader(503)
					cancel()
					return
				}
				w.Header().Set("Content-Type", "application/pdf")
				io.WriteString(w, "%PDF-1.7\n")
				w.(http.Flusher).Flush()
				if mode == "read" {
					cancel()
				}
				<-r.Context().Done()
			})
			cfg.MaxAttempts = 3
			cfg.RetryDelay = time.Millisecond
			if mode == "before" {
				cancel()
			}
			if mode == "timeout" {
				cfg.AttemptTimeout = 30 * time.Millisecond
			}
			if mode == "read-timeout" {
				cfg.ReadTimeout = 30 * time.Millisecond
			}
			if mode == "connect-timeout" {
				cfg.ConnectTimeout = 30 * time.Millisecond
				cfg.LookupIP = func(ctx context.Context, _ string) ([]net.IPAddr, error) { <-ctx.Done(); return nil, ctx.Err() }
			}
			c := newDocumentClient(cfg)
			sink := &testSink{}
			out := c.download(ctx, DocketEntry{RequestURL: fixtureHost + "/entry?secret=synthetic"}, sink)
			if strings.Contains(out.Err.Error(), "synthetic") {
				t.Fatal("URL leaked")
			}
			if strings.Contains(mode, "timeout") {
				if out.Status != DocumentFailed || out.Attempts != 3 {
					t.Fatalf("timeout: %+v", out)
				}
			} else if out.Status != DocumentCanceled || out.Attempts > 1 || !errors.Is(out.Err, context.Canceled) {
				t.Fatalf("cancel: %+v", out)
			}
			for _, w := range sink.writers {
				if !w.aborted || w.committed {
					t.Fatal("cancellation published bytes")
				}
			}
		})
	}
}

func TestDocumentStreamingBound(t *testing.T) {
	const payloadBytes = 8 << 20
	cfg := fixtureHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "%PDF-1.7\n")
		chunk := bytes.Repeat([]byte("x"), 8192)
		for i := 0; i < payloadBytes/len(chunk); i++ {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
		io.WriteString(w, "\n%%EOF\n")
	})
	sink := &testSink{discard: true}
	out := runDownload(t, cfg, "/large", sink)
	if out.Status != DocumentSucceeded || out.Bytes != payloadBytes+16 || len(sink.writers) != 1 || sink.writers[0].maxWrite > 32<<10 {
		t.Fatalf("streaming: %+v", out)
	}
}

func TestDocumentTLSVerification(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, fixturePDF) }))
	defer server.Close()
	c := newDocumentClient(DocumentHTTPConfig{MaxAttempts: 1,
		LookupIP: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
	})
	out := c.download(context.Background(), DocketEntry{RequestURL: "https://arcourts.gov/doc"}, &testSink{})
	if out.Status != DocumentFailed || out.Attempts != 1 || IsRetryable(out.Err) {
		t.Fatalf("TLS failure: %+v", out)
	}
}

func TestAllowedStorageRedirectHasNoSessionOrReferer(t *testing.T) {
	var calls atomic.Int32
	cfg := fixtureHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/start" {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "synthetic", Path: "/"})
			http.Redirect(w, r, "http://"+arcourtStorageHost+"/final?b=2&a=1", 302)
			return
		}
		if r.Host != arcourtStorageHost || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("Referer") != "" || r.URL.RawQuery != "b=2&a=1" {
			t.Error("redirect changed query or forwarded session/Referer")
		}
		io.WriteString(w, fixturePDF)
	})
	out := runDownload(t, cfg, "/start?secret=synthetic", &testSink{})
	if out.Status != DocumentSucceeded || calls.Load() != 2 {
		t.Fatalf("redirect=%+v", out)
	}
}

func TestResolvedHostsCannotResolvePrivate(t *testing.T) {
	for _, mode := range []string{"redirect", "json"} {
		var calls atomic.Int32
		cfg := fixtureHTTP(t, func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			target := "http://static.arcourts.gov/document.pdf"
			if mode == "redirect" {
				http.Redirect(w, r, target, 302)
			} else {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"url":%q}`, target)
			}
		})
		cfg.LookupIP = func(ctx context.Context, host string) ([]net.IPAddr, error) {
			ip := "93.184.216.34"
			if host == "static.arcourts.gov" {
				ip = "10.0.0.1"
			}
			return []net.IPAddr{{IP: net.ParseIP(ip)}}, nil
		}
		out := runDownload(t, cfg, "/entry", &testSink{})
		if !errors.Is(out.Err, ErrDestinationRejected) || calls.Load() != 1 {
			t.Fatalf("%s private resolution: %+v", mode, out)
		}
	}
}

func TestCancellationInterruptsRetryBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	responded := make(chan struct{})
	cfg := fixtureHTTP(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503); close(responded) })
	cfg.MaxAttempts = 3
	cfg.RetryDelay = 5 * time.Second
	c := newDocumentClient(cfg)
	done := make(chan DocumentOutcome, 1)
	go func() { done <- c.download(ctx, DocketEntry{RequestURL: fixtureHost + "/entry"}, &testSink{}) }()
	<-responded
	// Let the HTTP response enter the much longer retry delay.
	time.AfterFunc(30*time.Millisecond, cancel)
	select {
	case out := <-done:
		if out.Status != DocumentCanceled || out.Attempts != 1 {
			t.Fatalf("backoff=%+v", out)
		}
	case <-time.After(time.Second):
		t.Fatal("retry delay ignored cancellation")
	}
}

func TestPermanentFailuresAreNotRetried(t *testing.T) {
	for _, status := range []int{401, 403, 404, 410, 400} {
		cfg := fixtureHTTP(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) })
		cfg.MaxAttempts = 3
		out := runDownload(t, cfg, "/entry", &testSink{})
		if out.Attempts != 1 || out.Status != DocumentFailed {
			t.Fatalf("permanent %d=%+v", status, out)
		}
	}
}
