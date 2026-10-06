package arcourt

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var (
	ErrDestinationRejected = errors.New("document destination rejected")
	ErrDocumentTooLarge    = errors.New("document response exceeds size limit")
	ErrInvalidPDF          = errors.New("response is not a complete PDF")
	ErrRedirectLimit       = errors.New("document redirect limit exceeded")
	ErrIndirectionLimit    = errors.New("document JSON indirection limit exceeded")
)

// DocumentHTTPConfig controls bounded HTTP transfers. Zero values use defaults.
// LookupIP and DialContext are trusted transport injection seams: production
// callers should leave them nil. DialContext receives a validated numeric IP,
// never a hostname; tests can route that IP to a synthetic loopback server.
// No proxy, cookie jar, browser credentials, or unverified TLS is used.
type DocumentHTTPConfig struct {
	MaxPDFBytes    int64         // default 64 MiB, maximum 1 GiB
	MaxJSONBytes   int64         // default 1 MiB, maximum 4 MiB
	AttemptTimeout time.Duration // default 45s, maximum 5m (includes JSON hops)
	ConnectTimeout time.Duration // default 10s, maximum 30s (includes DNS)
	ReadTimeout    time.Duration // default 15s, maximum 1m per socket read
	MaxAttempts    int           // default 3, maximum 5; 1 disables retries
	RetryDelay     time.Duration // default 250ms, maximum 5s; linear backoff
	LookupIP       func(context.Context, string) ([]net.IPAddr, error)
	DialContext    func(context.Context, string, string) (net.Conn, error)
}

type documentClient struct {
	cfg  DocumentHTTPConfig
	http *http.Client
}

func bounded[T ~int | ~int64](value, fallback, maximum T) T {
	if value <= 0 {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}

func newDocumentClient(cfg DocumentHTTPConfig) *documentClient {
	cfg.MaxPDFBytes = bounded(cfg.MaxPDFBytes, 64<<20, 1<<30)
	cfg.MaxJSONBytes = bounded(cfg.MaxJSONBytes, 1<<20, 4<<20)
	cfg.AttemptTimeout = bounded(cfg.AttemptTimeout, 45*time.Second, 5*time.Minute)
	cfg.ConnectTimeout = bounded(cfg.ConnectTimeout, 10*time.Second, 30*time.Second)
	cfg.ReadTimeout = bounded(cfg.ReadTimeout, 15*time.Second, time.Minute)
	cfg.MaxAttempts = bounded(cfg.MaxAttempts, 3, 5)
	cfg.RetryDelay = bounded(cfg.RetryDelay, 250*time.Millisecond, 5*time.Second)
	if cfg.LookupIP == nil {
		cfg.LookupIP = net.DefaultResolver.LookupIPAddr
	}
	if cfg.DialContext == nil {
		cfg.DialContext = (&net.Dialer{}).DialContext
	}
	c := &documentClient{cfg: cfg}
	transport := &http.Transport{
		// A proxy could resolve/connect to a different IP, bypassing our dialer.
		Proxy: nil, DialContext: c.dialContext,
		TLSHandshakeTimeout:    cfg.ConnectTimeout,
		ResponseHeaderTimeout:  cfg.ReadTimeout,
		MaxResponseHeaderBytes: 64 << 10,
		DisableCompression:     true,
		// Sequential transfers need no connection pool. Each new connection is
		// resolved, checked, and pinned; a rebinding cannot reuse an unchecked IP.
		DisableKeepAlives: true,
	}
	c.http = &http.Client{Transport: transport, Timeout: cfg.AttemptTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 5 {
				return ErrRedirectLimit
			}
			if _, err := validateResolvedDocumentURL(via[len(via)-1].URL.String(), req.URL.String()); err != nil {
				return err
			}
			// Do not propagate cookies, authorization, or signed Referer URLs.
			req.Header = documentHeaders()
			return nil
		},
	}
	return c
}

type readDeadlineConn struct {
	net.Conn
	timeout time.Duration
}

func (c *readDeadlineConn) Read(p []byte) (int, error) {
	if err := c.Conn.SetReadDeadline(time.Now().Add(c.timeout)); err != nil {
		return 0, err
	}
	return c.Conn.Read(p)
}

func (c *documentClient) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.ConnectTimeout)
	defer cancel()
	host, port, err := net.SplitHostPort(address)
	if err != nil || !isAllowedArcourtDocumentHost(host) {
		return nil, ErrDestinationRejected
	}
	ips, err := c.cfg.LookupIP(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, errors.New("document host has no addresses")
	}
	// Reject the entire answer if it mixes public and private/local addresses.
	for _, ip := range ips {
		if ip.Zone != "" || blockedIP(ip.IP) {
			return nil, ErrDestinationRejected
		}
	}
	var last error
	for _, ip := range ips {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		conn, err := c.cfg.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if err == nil {
			return &readDeadlineConn{Conn: conn, timeout: c.cfg.ReadTimeout}, nil
		}
		last = err
	}
	return nil, last
}

var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/96"), netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"), netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/32"), netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}

func blockedIP(ip net.IP) bool {
	a, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	a = a.Unmap()
	if !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() {
		return true
	}
	for _, prefix := range nonPublicPrefixes {
		if prefix.Contains(a) {
			return true
		}
	}
	return false
}

func normalizeFetchHostname(hostname string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(hostname)), ".")
}

const arcourtStorageHost = "cdr-prod-cmslegacy-images-bucket.s3.us-gov-west-1.amazonaws.com"

func isAllowedArcourtDocumentHost(hostname string) bool {
	hostname = normalizeFetchHostname(hostname)
	return hostname == "arcourts.gov" || strings.HasSuffix(hostname, ".arcourts.gov") ||
		hostname == arcourtStorageHost
}

func validateDocumentFetchURL(rawURL string) (string, error) {
	target := strings.TrimSpace(rawURL)
	u, err := url.Parse(target)
	if err != nil || u.Opaque != "" || u.User != nil || u.Host == "" ||
		(u.Scheme != "https" && u.Scheme != "http") || !isAllowedArcourtDocumentHost(u.Hostname()) {
		return "", ErrDestinationRejected
	}
	if port := u.Port(); port != "" && !((u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80")) {
		return "", ErrDestinationRejected
	}
	return target, nil // Never re-encode the signed request query.
}

func validateResolvedDocumentURL(baseURL, rawNext string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", ErrDestinationRejected
	}
	next, err := base.Parse(strings.TrimSpace(rawNext))
	if err != nil || (base.Scheme == "https" && next.Scheme != "https") {
		return "", ErrDestinationRejected
	}
	return validateDocumentFetchURL(next.String())
}

func documentHeaders() http.Header {
	return http.Header{"User-Agent": {"ArcourtDownloader/1.0"}, "Accept": {"application/pdf, application/json"}}
}

// HTTPStatusError deliberately omits the body and URL (which may contain secrets).
type HTTPStatusError struct{ StatusCode int }

func (e *HTTPStatusError) Error() string {
	if e.StatusCode == 401 || e.StatusCode == 403 || e.StatusCode == 410 {
		return fmt.Sprintf("HTTP %d: access denied or link expired; rediscover the case", e.StatusCode)
	}
	return fmt.Sprintf("HTTP %d", e.StatusCode)
}

// transportError keeps errors.Is/As useful without exposing signed URLs.
type transportError struct{ cause error }

func (e *transportError) Error() string {
	switch {
	case errors.Is(e.cause, context.Canceled):
		return "document transfer canceled"
	case errors.Is(e.cause, context.DeadlineExceeded):
		return "document transfer deadline exceeded"
	case errors.Is(e.cause, ErrDestinationRejected):
		return ErrDestinationRejected.Error()
	case errors.Is(e.cause, ErrRedirectLimit):
		return ErrRedirectLimit.Error()
	case errors.Is(e.cause, io.ErrUnexpectedEOF):
		return "document response ended early (short read)"
	default:
		var certificate *tls.CertificateVerificationError
		if errors.As(e.cause, &certificate) {
			return "document TLS certificate verification failed"
		}
		var timeout net.Error
		if errors.As(e.cause, &timeout) && timeout.Timeout() {
			return "document transfer timed out"
		}
		return "document transport failed"
	}
}
func (e *transportError) Unwrap() error { return e.cause }

// Retain host I/O error classification without including a sink's paths/URLs in
// the display message. A host can inspect the cause with errors.Is/As.
type writerError struct {
	stage string
	cause error
}

func (e *writerError) Error() string { return "document writer " + e.stage + " failed" }
func (e *writerError) Unwrap() error { return e.cause }

type documentURLResponse struct {
	URL         string `json:"url"`
	DownloadURL string `json:"downloadUrl"`
	DocumentURL string `json:"documentUrl"`
	SignedURL   string `json:"signedUrl"`
}

func (c *documentClient) response(ctx context.Context, target string) (*http.Response, error) {
	for hop := 0; hop <= 3; hop++ {
		safeTarget, err := validateDocumentFetchURL(target)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, safeTarget, nil)
		if err != nil {
			return nil, ErrDestinationRejected
		}
		req.Header = documentHeaders()
		res, err := c.http.Do(req)
		if err != nil {
			return nil, &transportError{cause: err}
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 || res.StatusCode == http.StatusPartialContent {
			res.Body.Close()
			return nil, &HTTPStatusError{StatusCode: res.StatusCode}
		}
		ct := strings.ToLower(strings.Split(res.Header.Get("Content-Type"), ";")[0])
		if ct != "application/json" && !strings.HasSuffix(ct, "+json") {
			return res, nil
		}
		payloadBytes, readErr := io.ReadAll(io.LimitReader(res.Body, c.cfg.MaxJSONBytes+1))
		res.Body.Close()
		if readErr != nil {
			return nil, &transportError{cause: readErr}
		}
		if int64(len(payloadBytes)) > c.cfg.MaxJSONBytes {
			return nil, ErrDocumentTooLarge
		}
		if hop == 3 {
			return nil, ErrIndirectionLimit
		}
		var payload documentURLResponse
		if json.Unmarshal(payloadBytes, &payload) != nil {
			return nil, errors.New("invalid document endpoint JSON")
		}
		var next string
		for _, value := range []string{payload.URL, payload.DownloadURL, payload.DocumentURL, payload.SignedURL} {
			if strings.TrimSpace(value) != "" {
				next = value
				break
			}
		}
		if next == "" {
			return nil, errors.New("document endpoint JSON has no download URL")
		}
		// Relative targets are relative to the FINAL response URL after redirects.
		target, err = validateResolvedDocumentURL(res.Request.URL.String(), next)
		if err != nil {
			return nil, err
		}
	}
	return nil, ErrIndirectionLimit
}

func (c *documentClient) download(ctx context.Context, entry DocketEntry, sink DocumentSink) DocumentOutcome {
	out := DocumentOutcome{Document: entry, Status: DocumentFailed}
	for attempt := 1; attempt <= c.cfg.MaxAttempts; attempt++ {
		if ctx.Err() != nil {
			out.Err = ctx.Err()
			break
		}
		out.Attempts = attempt
		attemptCtx, cancel := context.WithTimeout(ctx, c.cfg.AttemptTimeout)
		n, digest, retry, err := c.attempt(attemptCtx, entry, sink)
		cancel()
		if err == nil {
			out.Status, out.Bytes, out.SHA256 = DocumentSucceeded, n, digest
			return out
		}
		out.Err = err
		if ctx.Err() != nil {
			out.Err = ctx.Err()
			break
		}
		if !retry || attempt == c.cfg.MaxAttempts {
			break
		}
		timer := time.NewTimer(time.Duration(attempt) * c.cfg.RetryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			out.Err = ctx.Err()
		case <-timer.C:
		}
	}
	if ctx.Err() != nil {
		out.Status, out.Err = DocumentCanceled, ctx.Err()
	}
	return out
}

func (c *documentClient) attempt(ctx context.Context, entry DocketEntry, sink DocumentSink) (n int64, digest string, retry bool, err error) {
	res, err := c.response(ctx, entry.RequestURL)
	if err != nil {
		return 0, "", IsRetryable(err), err
	}
	defer res.Body.Close()
	if res.ContentLength > c.cfg.MaxPDFBytes {
		return 0, "", false, ErrDocumentTooLarge
	}
	reader := bufio.NewReaderSize(io.LimitReader(res.Body, c.cfg.MaxPDFBytes+1), 32<<10)
	header, err := reader.Peek(8)
	if err != nil {
		if err == io.EOF {
			return 0, "", false, ErrInvalidPDF
		}
		return 0, "", IsRetryable(err), &transportError{cause: err}
	}
	if !bytes.HasPrefix(header, []byte("%PDF-")) || (header[5] != '1' && header[5] != '2') || header[6] != '.' || header[7] < '0' || header[7] > '9' {
		return 0, "", false, ErrInvalidPDF
	}
	w, err := sink.OpenDocument(ctx, entry)
	if err != nil {
		return 0, "", false, &writerError{stage: "open", cause: err}
	}
	if w == nil {
		return 0, "", false, errors.New("document sink returned no writer")
	}
	committed := false
	defer func() {
		if !committed {
			if abortErr := w.Abort(); abortErr != nil {
				err = errors.Join(err, &writerError{stage: "abort", cause: abortErr})
				retry = false
			}
		}
	}()
	hash := sha256.New()
	tail := make([]byte, 0, 1024)
	buffer := make([]byte, 32<<10)
	for {
		if ctx.Err() != nil {
			return 0, "", false, ctx.Err()
		}
		read, readErr := reader.Read(buffer)
		if read > 0 {
			n += int64(read)
			if n > c.cfg.MaxPDFBytes {
				return 0, "", false, ErrDocumentTooLarge
			}
			chunk := buffer[:read]
			written, writeErr := w.Write(chunk)
			if writeErr == nil && written != read {
				writeErr = io.ErrShortWrite
			}
			if writeErr != nil || written != read {
				return 0, "", false, &writerError{stage: "write", cause: writeErr}
			}
			hash.Write(chunk)
			// Fixed-size trailer window; no PDF or batch-sized in-memory buffer.
			if len(chunk) >= cap(tail) {
				tail = append(tail[:0], chunk[len(chunk)-cap(tail):]...)
			} else {
				drop := max(0, len(tail)+len(chunk)-cap(tail))
				copy(tail, tail[drop:])
				tail = append(tail[:len(tail)-drop], chunk...)
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				return 0, "", IsRetryable(readErr), &transportError{cause: readErr}
			}
			break
		}
	}
	if res.ContentLength >= 0 && n != res.ContentLength {
		return 0, "", true, &transportError{cause: io.ErrUnexpectedEOF}
	}
	if !bytes.HasSuffix(bytes.TrimSpace(tail), []byte("%%EOF")) {
		return 0, "", false, ErrInvalidPDF
	}
	if ctx.Err() != nil {
		return 0, "", false, ctx.Err()
	}
	if err := w.Commit(); err != nil {
		return 0, "", false, &writerError{stage: "commit", cause: err}
	}
	committed = true
	return n, hex.EncodeToString(hash.Sum(nil)), false, nil
}
