package arcourt

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestIsRetryable(t *testing.T) {
	if IsRetryable(context.Canceled) || IsRetryable(fmt.Errorf("temporary: %w", context.Canceled)) {
		t.Fatal("user cancellation must never be retryable")
	}
	if !IsRetryable(context.DeadlineExceeded) {
		t.Fatal("deadline exceeded should be retryable")
	}
	if !IsRetryable(errors.New("temporary connection reset by peer")) {
		t.Fatal("temporary network message should be retryable")
	}
	if IsRetryable(errors.New("invalid case number")) {
		t.Fatal("validation-like errors should not be retryable")
	}
}
