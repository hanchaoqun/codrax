package llm

// STREAM-WAIT §29.92 matrix (2026-07-15): httptest fake providers
// exercising the two customer failure shapes end to end — no real
// network.
//
//	形A "server never speaks": first-byte watchdog defaults sized for
//	     reasoning models (180s) + keep-alive bytes count as liveness
//	     (pinned in stream_first_byte_test.go) + zero extra backoff
//	     before the retry (pinned in openai_test.go).
//	形B "server refuses": empty-stream errors carry provider evidence
//	     (HTTP status, request-id header, safe body prefix), never
//	     credentials, and are retried at L1 with jittered backoff.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// testAPIKey is deliberately ≥8 bytes so redactCredential treats it as
// a real credential.
const testAPIKey = "sk-secret-test-key-123456"

// TestStreamFirstByteDefaults_ReasoningModelSafe pins the §29.92
// default raise 40s → 180s in BOTH homes (duration form in openai.go,
// seconds form in factory.go) and their equality — the two constants
// are the same default expressed twice and must never drift.
func TestStreamFirstByteDefaults_ReasoningModelSafe(t *testing.T) {
	if defaultStreamFirstByteTimeout != 180*time.Second {
		t.Fatalf("defaultStreamFirstByteTimeout = %v, want 180s (§29.92 reasoning-model-safe default)", defaultStreamFirstByteTimeout)
	}
	if defaultStreamFirstByteTimeoutSeconds != 180 {
		t.Fatalf("defaultStreamFirstByteTimeoutSeconds = %d, want 180", defaultStreamFirstByteTimeoutSeconds)
	}
	if time.Duration(defaultStreamFirstByteTimeoutSeconds)*time.Second != defaultStreamFirstByteTimeout {
		t.Fatalf("factory seconds default (%d) and adapter duration default (%v) drifted apart",
			defaultStreamFirstByteTimeoutSeconds, defaultStreamFirstByteTimeout)
	}
}

func newStreamAdapter(t *testing.T, url string, retries int) *OpenAIAdapter {
	t.Helper()
	return NewOpenAIAdapter(testAPIKey, "m", url, AdapterOptions{
		Stream:                 true,
		RequestTimeout:         10 * time.Second,
		RetryMaxAttempts:       retries,
		StreamFirstByteTimeout: 2 * time.Second,
		StreamStallTimeout:     5 * time.Second,
	})
}

// TestStreamEmpty_ImmediateEOFCarriesProviderEvidence covers 形B shape
// "200 + immediate EOF": the error must be the typed StreamEmptyError
// carrying HTTP status and the request-id response header, so the
// operator has something concrete to escalate to the provider.
func TestStreamEmpty_ImmediateEOFCarriesProviderEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Request-Id", "req-abc-123")
		w.WriteHeader(http.StatusOK)
		// Close with zero body bytes: accepted, then silence.
	}))
	defer server.Close()

	adapter := newStreamAdapter(t, server.URL, 1)
	_, err := adapter.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, nil, ChatOptions{})
	if err == nil {
		t.Fatalf("expected empty-stream error")
	}
	if !errors.Is(err, ErrStreamEmpty) {
		t.Fatalf("expected ErrStreamEmpty in chain; got %v", err)
	}
	var esErr *StreamEmptyError
	if !errors.As(err, &esErr) {
		t.Fatalf("expected *StreamEmptyError; got %T: %v", err, err)
	}
	if esErr.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", esErr.StatusCode)
	}
	if esErr.RequestID != "x-request-id=req-abc-123" {
		t.Errorf("RequestID = %q, want x-request-id=req-abc-123", esErr.RequestID)
	}
	if esErr.CommentOnly {
		t.Errorf("immediate EOF must not classify as comment-only")
	}
	msg := err.Error()
	for _, want := range []string{"empty stream", "HTTP 200", "x-request-id=req-abc-123"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error text missing %q: %q", want, msg)
		}
	}
}

// TestStreamEmpty_CommentOnlyEOFHasDistinctWording covers 形B shape
// "200 + keep-alive comments then EOF": distinct wording from the
// immediate-EOF form — the server was breathing but never spoke.
func TestStreamEmpty_CommentOnlyEOFHasDistinctWording(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cf-Ray", "ray-777")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(": keep-alive\n\n: keep-alive\n\ndata: {}\n\n"))
	}))
	defer server.Close()

	adapter := newStreamAdapter(t, server.URL, 1)
	_, err := adapter.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, nil, ChatOptions{})
	if err == nil {
		t.Fatalf("expected empty-stream error")
	}
	var esErr *StreamEmptyError
	if !errors.As(err, &esErr) {
		t.Fatalf("expected *StreamEmptyError; got %T: %v", err, err)
	}
	if !esErr.CommentOnly {
		t.Errorf("keep-alive-only stream must classify CommentOnly=true")
	}
	msg := err.Error()
	for _, want := range []string{"empty stream", "keep-alive/comment", "HTTP 200", "cf-ray=ray-777"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error text missing %q: %q", want, msg)
		}
	}
	// gotAnyChunk semantics preserved: comments and empty JSON frames
	// count as liveness bytes but never as content, so this is an
	// empty stream, not a first-byte timeout.
	if errors.Is(err, ErrStreamFirstByteTimeout) {
		t.Errorf("comment-only EOF must classify as empty stream, not first-byte timeout: %v", err)
	}
}

// TestStreamEmpty_NonSSEBodyCarriesTruncatedPrefixAndRedactsKey covers
// the "200 + error JSON instead of SSE" refinement plus the credential
// red line: the body prefix is carried (≤512B, truncated) and the
// configured api key NEVER appears in the error text even when a
// hostile/echoing provider embeds it in the body.
func TestStreamEmpty_NonSSEBodyCarriesTruncatedPrefixAndRedactsKey(t *testing.T) {
	longTail := strings.Repeat("x", 900)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "req-json-1")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"error":{"message":"no capacity","echo":"` + testAPIKey + `","pad":"` + longTail + `"}}`))
	}))
	defer server.Close()

	adapter := newStreamAdapter(t, server.URL, 1)
	_, err := adapter.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, nil, ChatOptions{})
	if err == nil {
		t.Fatalf("expected empty-stream error")
	}
	var esErr *StreamEmptyError
	if !errors.As(err, &esErr) {
		t.Fatalf("expected *StreamEmptyError; got %T: %v", err, err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "non-SSE payload") {
		t.Errorf("non-SSE body shape must say so: %q", msg)
	}
	if !strings.Contains(msg, `"no capacity"`) {
		t.Errorf("error text must carry the provider body prefix: %q", msg)
	}
	if len(esErr.BodyPrefix) > emptyStreamBodyPrefixCap+len("...[truncated]") {
		t.Errorf("BodyPrefix length %d exceeds the %dB cap", len(esErr.BodyPrefix), emptyStreamBodyPrefixCap)
	}
	// Credential red line (negative pin): the api key was echoed in
	// the body; it must not survive into the error text.
	if strings.Contains(msg, testAPIKey) {
		t.Fatalf("CREDENTIAL LEAK: error text contains the api key: %q", msg)
	}
}

// TestAPIError_Non200CarriesStatusRequestIDAndRedactsKey covers 形B
// shape "HTTP≠200": the apiError wording keeps the load-bearing
// "status NNN" token, adds the request id, truncates a huge body for
// display, and never leaks the api key.
func TestAPIError_Non200CarriesStatusRequestIDAndRedactsKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req-503-9")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"overloaded","echo":"` + testAPIKey + `"}`))
	}))
	defer server.Close()

	adapter := newStreamAdapter(t, server.URL, 1)
	_, err := adapter.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, nil, ChatOptions{})
	if err == nil {
		t.Fatalf("expected api error")
	}
	msg := err.Error()
	for _, want := range []string{"status 503", "x-request-id=req-503-9", "overloaded"} {
		if !strings.Contains(msg, want) {
			t.Errorf("apiError text missing %q: %q", want, msg)
		}
	}
	if strings.Contains(msg, testAPIKey) {
		t.Fatalf("CREDENTIAL LEAK: apiError text contains the api key: %q", msg)
	}
}

// TestStreamEmpty_RetriedAtL1WithJitteredBackoff pins the §29.92 retry
// contract for 形B: an empty stream is provably safe to retry (zero
// callbacks fired), so L1 retries it in-adapter — first failure, then
// success — with the OnRetry notice carrying the "empty stream" reason
// and a jittered delay in (0, 1s] for attempt 0.
func TestStreamEmpty_RetriedAtL1WithJitteredBackoff(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if calls.Add(1) == 1 {
			return // empty stream on the first attempt
		}
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	adapter := newStreamAdapter(t, server.URL, 3)
	var retryReason string
	var retryDelay time.Duration
	var retryCount atomic.Int64
	resp, err := adapter.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, nil, ChatOptions{
		OnRetry: func(attempt int, delay time.Duration, reason string) {
			retryCount.Add(1)
			retryReason = reason
			retryDelay = delay
		},
	})
	if err != nil {
		t.Fatalf("empty stream must be retried at L1 and recover: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("recovered response wrong: %q", resp.Content)
	}
	if got := retryCount.Load(); got != 1 {
		t.Fatalf("expected exactly 1 retry notice, got %d", got)
	}
	if retryReason != "empty stream" {
		t.Errorf("retry reason = %q, want \"empty stream\"", retryReason)
	}
	if retryDelay <= 0 || retryDelay > time.Second {
		t.Errorf("attempt-0 jittered backoff = %v, want in (0, 1s]", retryDelay)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("expected 2 upstream attempts, got %d", got)
	}
}

// TestStreamEmpty_ExhaustionWrapsAllRetries pins the layering contract:
// when every attempt returns an empty stream, the terminal error wears
// ErrAllRetriesExhausted (so outer layers do not duplicate coverage)
// while ErrStreamEmpty and the provider evidence stay reachable.
func TestStreamEmpty_ExhaustionWrapsAllRetries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req-dead-7")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := newStreamAdapter(t, server.URL, 2)
	_, err := adapter.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, nil, ChatOptions{})
	if err == nil {
		t.Fatalf("expected terminal empty-stream error")
	}
	if !errors.Is(err, ErrAllRetriesExhausted) {
		t.Fatalf("expected ErrAllRetriesExhausted wrap; got %v", err)
	}
	if !errors.Is(err, ErrStreamEmpty) {
		t.Fatalf("ErrStreamEmpty must stay reachable through the wrap; got %v", err)
	}
	if !strings.Contains(err.Error(), "x-request-id=req-dead-7") {
		t.Errorf("provider evidence must survive the exhaustion wrap: %q", err.Error())
	}
	// L4 must not re-retry an L1-exhausted error; a bare (single)
	// occurrence remains stream-level retryable for outer layers.
	if IsStreamLevelRetryable(err) {
		t.Errorf("L1-exhausted empty stream must not be stream-level retryable")
	}
	if !IsStreamLevelRetryable(&StreamEmptyError{StatusCode: 200}) {
		t.Errorf("bare empty stream must be stream-level retryable")
	}
	if !IsRetryableDispatchError(&StreamEmptyError{StatusCode: 200}) {
		t.Errorf("bare empty stream must be dispatch-retryable")
	}
	if !IsFallbackEligible(err) {
		t.Errorf("exhausted empty stream must remain fallback-eligible (fresh provider, fresh quota)")
	}
}

// TestRequestTelemetry_CarriesFirstByteCeiling pins the §29.92 件4
// plumbing: the adapter's resolved first-byte ceiling reaches
// RequestTelemetry (heartbeat "已 Xs / 首字节上限 Ys" consumes it), and
// both wrapper adapters delegate instead of hiding it.
func TestRequestTelemetry_CarriesFirstByteCeiling(t *testing.T) {
	adapter := NewOpenAIAdapter(testAPIKey, "m", "http://example.test", AdapterOptions{
		Stream:                 true,
		RequestTimeout:         10 * time.Second,
		RetryMaxAttempts:       1,
		StreamFirstByteTimeout: 3 * time.Minute,
	})
	if got := BuildRequestTelemetry(adapter, nil, nil).StreamFirstByteTimeout; got != 3*time.Minute {
		t.Fatalf("telemetry StreamFirstByteTimeout = %v, want 3m", got)
	}
	wrapped := NewTelemetryAdapter(adapter, func(RequestTelemetry) {})
	if got := BuildRequestTelemetry(wrapped, nil, nil).StreamFirstByteTimeout; got != 3*time.Minute {
		t.Fatalf("TelemetryAdapter must delegate the first-byte ceiling; got %v", got)
	}
	fb := NewFallbackAdapter(adapter)
	if got := BuildRequestTelemetry(fb, nil, nil).StreamFirstByteTimeout; got != 3*time.Minute {
		t.Fatalf("FallbackAdapter must delegate the head adapter's first-byte ceiling; got %v", got)
	}
}

// TestRedactCredential_Bounds pins the scrub helper: real credentials
// are replaced everywhere; empty / toy-length secrets are no-ops so
// ordinary prose is never mangled.
func TestRedactCredential_Bounds(t *testing.T) {
	if got := redactCredential("body "+testAPIKey+" tail "+testAPIKey, testAPIKey); strings.Contains(got, testAPIKey) {
		t.Fatalf("redactCredential left the secret in place: %q", got)
	} else if strings.Count(got, "[REDACTED]") != 2 {
		t.Fatalf("expected both occurrences redacted: %q", got)
	}
	if got := redactCredential("keep k intact", "k"); got != "keep k intact" {
		t.Fatalf("toy-length secret must be a no-op, got %q", got)
	}
	if got := redactCredential("unchanged", ""); got != "unchanged" {
		t.Fatalf("empty secret must be a no-op, got %q", got)
	}
}
