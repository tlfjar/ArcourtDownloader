package arcourt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type namingRoundTripFunc func(*http.Request) (*http.Response, error)

func (f namingRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func namingReply(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestNamingProviderPathsAndUsage(t *testing.T) {
	// Environment base URLs must not redirect a paid request or its credential.
	t.Setenv("OPENAI_BASE_URL", "https://unexpected.invalid")
	t.Setenv("ANTHROPIC_BASE_URL", "https://unexpected.invalid")
	t.Setenv("GOOGLE_BASE_URL", "https://unexpected.invalid")
	t.Setenv("XAI_BASE_URL", "https://unexpected.invalid")
	for _, tc := range []struct {
		name, model, host, path, authHeader, authValue, limitKey, reply string
	}{
		{
			name: "openai", model: "gpt-4o", host: "api.openai.com", path: "/v1/responses",
			authHeader: "Authorization", authValue: "Bearer test-secret", limitKey: "max_output_tokens",
			reply: `{"id":"resp-1","model":"gpt-4o","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"Motion to Dismiss"}]}],"usage":{"input_tokens":10,"output_tokens":5}}`,
		},
		{
			name: "anthropic", model: "claude-test", host: "api.anthropic.com", path: "/v1/messages",
			authHeader: "x-api-key", authValue: "test-secret", limitKey: "max_tokens",
			reply: `{"id":"msg_1","model":"claude-test","type":"message","content":[{"type":"text","text":"Motion to Dismiss"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`,
		},
		{
			name: "google", model: "gemini-test", host: "generativelanguage.googleapis.com", path: "/v1beta/models/gemini-test:generateContent",
			authHeader: "x-goog-api-key", authValue: "test-secret", limitKey: "maxOutputTokens",
			reply: `{"candidates":[{"content":{"parts":[{"text":"Motion to Dismiss"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}`,
		},
		{
			name: "xai", model: "grok-test", host: "api.x.ai", path: "/v1/chat/completions",
			authHeader: "Authorization", authValue: "Bearer test-secret", limitKey: "max_tokens",
			reply: `{"id":"chatcmpl-123","model":"grok-test","choices":[{"message":{"role":"assistant","content":"Motion to Dismiss"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls int
			var sentBytes int
			transport := namingRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != http.MethodPost || r.URL.Scheme != "https" || r.URL.Hostname() != tc.host || r.URL.Port() != "443" || r.URL.Path != tc.path || r.URL.RawQuery != "" {
					t.Fatalf("wrong destination: %s %s", r.Method, r.URL.Redacted())
				}
				if got := r.Header.Get(tc.authHeader); got != tc.authValue {
					t.Fatalf("wrong auth header for %s", tc.name)
				}
				payload, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				sentBytes = len(payload)
				if bytes.Contains(payload, []byte("test-secret")) || bytes.Contains(payload, []byte("tools")) ||
					!bytes.Contains(payload, []byte("System naming instruction")) || !bytes.Contains(payload, []byte("motion to dismiss")) {
					t.Fatal("request lost instruction or excerpt, or included forbidden content")
				}
				var body map[string]any
				if err := json.Unmarshal(payload, &body); err != nil {
					t.Fatal(err)
				}
				limits := body
				if tc.name == "google" {
					limits, _ = body["generationConfig"].(map[string]any)
				}
				if limits[tc.limitKey] != float64(providerOutputTokens) {
					t.Fatalf("output bound missing from %s request: %s", tc.name, payload)
				}
				return namingReply(http.StatusOK, tc.reply), nil
			})
			client, err := newProviderClient(NamingRequest{Provider: tc.name, Model: tc.model, APIKey: "test-secret"}, transport)
			if err != nil {
				t.Fatal(err)
			}
			label, usage, err := client.Generate(t.Context(), "System naming instruction", "motion to dismiss")
			if err != nil || label != "Motion to Dismiss" || calls != 1 {
				t.Fatalf("label=%q calls=%d error=%v", label, calls, err)
			}
			if !usage.Reported || usage.InputTokens != 10 || usage.OutputTokens != 5 || usage.RequestBytes != sentBytes || sentBytes == 0 {
				t.Fatalf("incorrect usage: %+v, sentBytes=%d", usage, sentBytes)
			}
		})
	}
}

func TestNamingProviderControlledErrorsAndNoHiddenRetries(t *testing.T) {
	for _, tc := range []struct {
		name, model string
	}{
		{"openai", "gpt-4o"}, {"anthropic", "claude-test"}, {"google", "gemini-test"}, {"xai", "grok-test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, status := range []struct {
				code      int
				body      string
				reason    string
				permanent bool
			}{
				{401, `{"error":{"message":"invalid API key"}}`, "provider_auth", true},
				{404, `{"error":{"message":"model not found"}}`, "provider_model", true},
				{429, `{"error":{"message":"rate limited"}}`, "rate_limited", false},
			} {
				var calls int
				transport := namingRoundTripFunc(func(*http.Request) (*http.Response, error) {
					calls++
					return namingReply(status.code, status.body), nil
				})
				client, err := newProviderClient(NamingRequest{Provider: tc.name, Model: tc.model, APIKey: "test-secret"}, transport)
				if err != nil {
					t.Fatal(err)
				}
				_, usage, err := client.Generate(t.Context(), "instruction", "motion to dismiss")
				reason, permanent := namingFailure(err)
				if calls != 1 || reason != status.reason || permanent != status.permanent || usage.Reported || usage.RequestBytes == 0 {
					t.Fatalf("status=%d calls=%d reason=%q permanent=%t usage=%+v err=%v", status.code, calls, reason, permanent, usage, err)
				}
			}
		})
	}
}

func TestNamingProviderRefusalsDoNotBecomeLabels(t *testing.T) {
	for _, tc := range []struct {
		name, model, reply string
	}{
		{"openai", "gpt-4o", `{"id":"resp-1","model":"gpt-4o","status":"incomplete","incomplete_details":{"reason":"content_filter"},"output":[{"type":"message","content":[{"type":"output_text","text":"bad label"}]}],"usage":{"input_tokens":10,"output_tokens":5}}`},
		{"anthropic", "claude-test", `{"id":"msg_1","model":"claude-test","type":"message","content":[{"type":"text","text":"bad label"}],"stop_reason":"refusal","usage":{"input_tokens":10,"output_tokens":5}}`},
		{"google", "gemini-test", `{"candidates":[{"content":{"parts":[{"text":"bad label"}]},"finishReason":"SAFETY"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}`},
		{"xai", "grok-test", `{"id":"chatcmpl-123","model":"grok-test","choices":[{"message":{"role":"assistant","content":"bad label"},"finish_reason":"content_filter"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls int
			transport := namingRoundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return namingReply(http.StatusOK, tc.reply), nil
			})
			client, err := newProviderClient(NamingRequest{Provider: tc.name, Model: tc.model, APIKey: "test-secret"}, transport)
			if err != nil {
				t.Fatal(err)
			}
			label, usage, err := client.Generate(t.Context(), "instruction", "motion to dismiss")
			if reason, permanent := namingFailure(err); reason != "provider_refusal" || !permanent || label != "" || calls != 1 || !usage.Reported {
				t.Fatalf("refusal: label=%q reason=%s permanent=%t calls=%d usage=%+v", label, reason, permanent, calls, usage)
			}
		})
	}
}

func TestNamingProviderBoundsAndDestination(t *testing.T) {
	var calls atomic.Int32
	transport := namingRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return namingReply(http.StatusOK, strings.Repeat("x", maxProviderReplyBytes+1)), nil
	})
	client, err := newProviderClient(NamingRequest{Provider: "openai", Model: "gpt-4o", APIKey: "test-secret"}, transport)
	if err != nil {
		t.Fatal(err)
	}
	_, usage, err := client.Generate(t.Context(), "instruction", "motion to dismiss")
	if reason, _ := namingFailure(err); reason != "provider_error" || usage.RequestBytes == 0 || calls.Load() != 1 {
		t.Fatalf("oversized reply: reason=%s usage=%+v calls=%d", reason, usage, calls.Load())
	}
	_, usage, err = client.Generate(t.Context(), "instruction", strings.Repeat("x", maxProviderExcerptChars+1))
	if reason, _ := namingFailure(err); reason != "payload_limit" || usage.RequestBytes != 0 || calls.Load() != 1 {
		t.Fatalf("oversized excerpt: reason=%s usage=%+v calls=%d", reason, usage, calls.Load())
	}
	for _, destination := range []string{"https://unexpected.invalid/v1/responses", "http://api.openai.com/v1/responses", "https://api.openai.com/v1/files"} {
		r, err := http.NewRequest(http.MethodPost, destination, strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		gate := &namingTransport{base: transport, provider: "openai", model: "gpt-4o", requestBytes: new(atomic.Int64)}
		_, err = gate.RoundTrip(r)
		if reason, _ := namingFailure(err); reason != "provider_error" || calls.Load() != 1 {
			t.Fatalf("destination %s: reason=%s calls=%d", destination, reason, calls.Load())
		}
	}
	oversized, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/responses", strings.NewReader(`{"input":"`+strings.Repeat("x", maxProviderRequestBytes)+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	gate := &namingTransport{base: transport, provider: "openai", model: "gpt-4o", requestBytes: new(atomic.Int64)}
	_, err = gate.RoundTrip(oversized)
	if reason, _ := namingFailure(err); reason != "payload_limit" || calls.Load() != 1 || gate.requestBytes.Load() != 0 {
		t.Fatalf("serialized payload limit: reason=%s calls=%d bytes=%d", reason, calls.Load(), gate.requestBytes.Load())
	}
	if _, err := newProviderClient(NamingRequest{Provider: "google", Model: "../other", APIKey: "test-secret"}, transport); err == nil {
		t.Fatal("path-like model accepted")
	}
}

func TestNamingProviderDoesNotFollowRedirect(t *testing.T) {
	var calls int
	transport := namingRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		response := namingReply(http.StatusFound, "")
		response.Header.Set("Location", "https://unexpected.invalid/collect")
		return response, nil
	})
	client, err := newProviderClient(NamingRequest{Provider: "openai", Model: "gpt-4o", APIKey: "test-secret"}, transport)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = client.Generate(t.Context(), "instruction", "motion to dismiss")
	if reason, _ := namingFailure(err); reason != "provider_error" || calls != 1 {
		t.Fatalf("redirect: reason=%s calls=%d", reason, calls)
	}
}

func TestNamingProviderDefaultTransportHasNoAmbientProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://unexpected.invalid:8080")
	t.Setenv("HTTP_PROXY", "http://unexpected.invalid:8080")
	client, err := newProviderClient(NamingRequest{Provider: "openai", Model: "gpt-4o", APIKey: "test-secret"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	direct, ok := client.(*providerClient).transport.(*http.Transport)
	if !ok || direct.Proxy != nil {
		t.Fatal("provider transport inherited an ambient proxy")
	}
}

func TestNamingProviderDoesNotInheritMutableDefaultTransport(t *testing.T) {
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	// A host application may replace DefaultTransport outright. This optional
	// feature must neither panic nor route a credential through that transport.
	http.DefaultTransport = namingRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("process default transport received naming request")
		return nil, nil
	})
	client, err := newProviderClient(NamingRequest{Provider: "openai", Model: "gpt-4o", APIKey: "test-secret"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	direct, ok := client.(*providerClient).transport.(*http.Transport)
	if !ok || direct.Proxy != nil || direct.DialContext != nil || direct.TLSClientConfig != nil {
		t.Fatalf("naming inherited the process transport: %#v", client.(*providerClient).transport)
	}
}

func TestNamingProviderUnknownUsageAfterTransportFailure(t *testing.T) {
	var calls int
	transport := namingRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("simulated connection failure")
	})
	client, err := newProviderClient(NamingRequest{Provider: "google", Model: "gemini-test", APIKey: "test-secret"}, transport)
	if err != nil {
		t.Fatal(err)
	}
	_, usage, err := client.Generate(t.Context(), "instruction", "motion to dismiss")
	if reason, _ := namingFailure(err); reason != "provider_error" || calls != 1 || usage.RequestBytes == 0 || usage.Reported {
		t.Fatalf("transport failure: reason=%s calls=%d usage=%+v", reason, calls, usage)
	}
}

func TestNamingProviderCancellation(t *testing.T) {
	var calls atomic.Int32
	transport := namingRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	client, err := newProviderClient(NamingRequest{Provider: "anthropic", Model: "claude-test", APIKey: "test-secret"}, transport)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(10*time.Millisecond, cancel)
	_, usage, err := client.Generate(ctx, "instruction", "motion to dismiss")
	if !errors.Is(err, context.Canceled) || calls.Load() != 1 || usage.RequestBytes == 0 || usage.Reported {
		t.Fatalf("cancel: err=%v calls=%d usage=%+v", err, calls.Load(), usage)
	}
}
