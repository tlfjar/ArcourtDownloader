package arcourt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/zendev-sh/goai"
	"github.com/zendev-sh/goai/provider"
	"github.com/zendev-sh/goai/provider/anthropic"
	"github.com/zendev-sh/goai/provider/google"
	"github.com/zendev-sh/goai/provider/openai"
	"github.com/zendev-sh/goai/provider/xai"
)

// The transport checks the SDK's serialized request, including any content the
// SDK adds to the system instruction and excerpt. The PDF never leaves as a file.
const (
	maxProviderPromptBytes  = 2048
	maxProviderExcerptChars = 4096
	maxProviderExcerptBytes = 8192
	maxProviderRequestBytes = 16 << 10
	maxProviderReplyBytes   = 64 << 10
	providerOutputTokens    = 96
)

var providerModelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

type providerClient struct {
	request   NamingRequest
	transport http.RoundTripper
}

// newProviderClient creates a narrow, single-generation adapter. The caller
// provides the key from secure storage for this job; no environment credentials,
// court HTTP client, cookies, or request history are inherited.
func newProviderClient(config NamingRequest, transport http.RoundTripper) (namingClient, error) {
	if err := ValidateNamingRequest(config); err != nil || !providerModelPattern.MatchString(config.Model) {
		return nil, &namingError{reason: "configuration", permanent: true}
	}
	if transport == nil {
		// Use a new transport instead of the mutable process default. A caller's
		// proxy, custom dialer or TLS override must not receive the credential.
		transport = directNamingTransport()
	}
	return &providerClient{request: config, transport: transport}, nil
}

func directNamingTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 nil,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		ForceAttemptHTTP2:     true,
	}
}

func (c *providerClient) Generate(ctx context.Context, prompt, excerpt string) (string, NamingUsage, error) {
	var usage NamingUsage
	if err := ctx.Err(); err != nil {
		return "", usage, err
	}
	if prompt == "" || excerpt == "" || !utf8.ValidString(prompt) || !utf8.ValidString(excerpt) ||
		len(prompt) > maxProviderPromptBytes || len(excerpt) > maxProviderExcerptBytes ||
		utf8.RuneCountInString(excerpt) > maxProviderExcerptChars {
		return "", usage, &namingError{reason: "payload_limit"}
	}

	var requestBytes atomic.Int64
	transport := &namingTransport{base: c.transport, provider: c.request.Provider, model: c.request.Model, requestBytes: &requestBytes}
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	model := c.model(httpClient)
	options := []goai.Option{
		goai.WithSystem(prompt),
		goai.WithPrompt(excerpt),
		goai.WithMaxOutputTokens(providerOutputTokens),
		goai.WithMaxRetries(0),
		goai.WithMaxSteps(1),
		goai.WithPromptCaching(false),
	}
	// Luna defaults to medium reasoning. This short classification task has a
	// 96-token total output budget, so request direct output for these models.
	if c.request.Provider == "openai" && (c.request.Model == "gpt-6-luna" || c.request.Model == "gpt-5.6-luna") {
		options = append(options, goai.WithProviderOptions(map[string]any{"reasoning_effort": "none"}))
	}
	result, err := goai.GenerateText(ctx, model, options...)
	usage.RequestBytes = int(requestBytes.Load())
	if err != nil {
		return "", usage, classifyProviderError(ctx, err)
	}
	if result == nil || len(result.Steps) != 1 || len(result.ToolCalls) != 0 {
		return "", usage, &namingError{reason: "provider_error"}
	}
	u := result.TotalUsage
	usage.InputTokens = u.InputTokens
	usage.OutputTokens = u.OutputTokens
	usage.CacheReadTokens = u.CacheReadTokens
	usage.CacheWriteTokens = u.CacheWriteTokens
	usage.ReasoningTokens = u.ReasoningTokens
	usage.Reported = u.InputTokens > 0 || u.OutputTokens > 0 || u.CacheReadTokens > 0 || u.CacheWriteTokens > 0 || u.ReasoningTokens > 0
	if result.FinishReason == provider.FinishContentFilter || hasProviderRefusal(result.Steps[0].Content) {
		return "", usage, &namingError{reason: "provider_refusal", permanent: true}
	}
	if result.FinishReason != provider.FinishStop {
		return "", usage, &namingError{reason: "provider_error"}
	}
	return result.Text, usage, nil
}

func (c *providerClient) model(client *http.Client) provider.LanguageModel {
	// Explicit :443 base URLs prevent the SDK's optional *_BASE_URL environment
	// fallback from replacing a default base URL. The transport also pins host,
	// path, scheme and method before any request can leave this process.
	switch c.request.Provider {
	case "openai":
		return openai.Chat(c.request.Model, openai.WithAPIKey(c.request.APIKey), openai.WithBaseURL("https://api.openai.com:443/v1"), openai.WithHTTPClient(client))
	case "anthropic":
		return anthropic.Chat(c.request.Model, anthropic.WithAPIKey(c.request.APIKey), anthropic.WithBaseURL("https://api.anthropic.com:443"), anthropic.WithHTTPClient(client), anthropic.WithAutoStreaming(false))
	case "google":
		return google.Chat(c.request.Model, google.WithAPIKey(c.request.APIKey), google.WithBaseURL("https://generativelanguage.googleapis.com:443"), google.WithHTTPClient(client))
	default: // Validated by newProviderClient.
		return xai.Chat(c.request.Model, xai.WithAPIKey(c.request.APIKey), xai.WithBaseURL("https://api.x.ai:443/v1"), xai.WithHTTPClient(client))
	}
}

func hasProviderRefusal(content []provider.Part) bool {
	for _, part := range content {
		if refusal, _ := part.ProviderOptions["refusal"].(bool); refusal {
			return true
		}
	}
	return false
}

func classifyProviderError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var controlled *namingError
	if errors.As(err, &controlled) {
		return controlled
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var apiErr *goai.APIError
	if errors.As(err, &apiErr) {
		lower := strings.ToLower(apiErr.Message)
		if strings.Contains(lower, "refus") || strings.Contains(lower, "content_filter") || strings.Contains(lower, "safety") || strings.Contains(lower, "policy") {
			return &namingError{reason: "provider_refusal", permanent: true}
		}
		switch apiErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return &namingError{reason: "provider_auth", permanent: true}
		case http.StatusTooManyRequests:
			return &namingError{reason: "rate_limited"}
		case http.StatusNotFound:
			return &namingError{reason: "provider_model", permanent: true}
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			if strings.Contains(lower, "model") && (strings.Contains(lower, "invalid") || strings.Contains(lower, "unknown") || strings.Contains(lower, "not found") || strings.Contains(lower, "does not exist") || strings.Contains(lower, "unsupported")) {
				return &namingError{reason: "provider_model", permanent: true}
			}
		}
	}
	return &namingError{reason: "provider_error"}
}

type namingTransport struct {
	base         http.RoundTripper
	provider     string
	model        string
	requestBytes *atomic.Int64
}

func (t *namingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !t.allowedDestination(req.URL) || req.Method != http.MethodPost || req.URL.User != nil || req.URL.RawQuery != "" || req.URL.Fragment != "" {
		return nil, &namingError{reason: "provider_error"}
	}
	if req.Body == nil {
		return nil, &namingError{reason: "provider_error"}
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, maxProviderRequestBytes+1))
	_ = req.Body.Close()
	if err != nil || len(body) > maxProviderRequestBytes {
		return nil, &namingError{reason: "payload_limit"}
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal(body, &payload) != nil || payload["tools"] != nil || payload["file"] != nil || payload["files"] != nil {
		return nil, &namingError{reason: "provider_error"}
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	t.requestBytes.Store(int64(len(body)))
	resp, err := t.base.RoundTrip(req)
	if err != nil || resp == nil {
		return resp, err
	}
	if resp.Body == nil {
		return nil, &namingError{reason: "provider_error"}
	}
	reply, readErr := io.ReadAll(io.LimitReader(resp.Body, maxProviderReplyBytes+1))
	_ = resp.Body.Close()
	if readErr != nil || len(reply) > maxProviderReplyBytes {
		return nil, &namingError{reason: "provider_error"}
	}
	resp.Body = io.NopCloser(bytes.NewReader(reply))
	resp.ContentLength = int64(len(reply))
	return resp, nil
}

func (t *namingTransport) allowedDestination(u *url.URL) bool {
	if u == nil || u.Scheme != "https" || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	path := u.EscapedPath()
	switch t.provider {
	case "openai":
		return strings.EqualFold(u.Hostname(), "api.openai.com") && path == "/v1/responses"
	case "anthropic":
		return strings.EqualFold(u.Hostname(), "api.anthropic.com") && path == "/v1/messages"
	case "google":
		return strings.EqualFold(u.Hostname(), "generativelanguage.googleapis.com") && path == "/v1beta/models/"+t.model+":generateContent"
	case "xai":
		return strings.EqualFold(u.Hostname(), "api.x.ai") && path == "/v1/chat/completions"
	default:
		return false
	}
}
