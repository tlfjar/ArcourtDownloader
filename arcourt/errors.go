package arcourt

import (
	"context"
	"errors"
	"io"
	"net"
	"net/url"
	"strings"
)

// ErrBackendUnavailable indicates that the Arkansas court backend could not serve the request.
var ErrBackendUnavailable = errors.New("arcourt backend unavailable")

// ErrCaseLoadFailed indicates that the Arkansas case page did not load usable docket content.
var ErrCaseLoadFailed = errors.New("arcourt case load failed")

// ErrCaseAccessDenied indicates that the court rejected public case access.
var ErrCaseAccessDenied = errors.New("court case access denied")

// IsRetryable reports whether an Arcourt fetch error is likely temporary.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	var output *writerError
	if errors.As(err, &output) {
		return false
	}
	var transfer *transportError
	if errors.As(err, &transfer) {
		return IsRetryable(transfer.cause)
	}
	var status *HTTPStatusError
	if errors.As(err, &status) {
		return status.StatusCode == 408 || status.StatusCode == 429 || status.StatusCode == 500 || status.StatusCode == 502 || status.StatusCode == 503 || status.StatusCode == 504
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return IsRetryable(uerr.Err)
	}
	var nerr net.Error
	if errors.As(err, &nerr) {
		return nerr.Timeout() || nerr.Temporary()
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "temporary") || strings.Contains(msg, "connection reset")
}
