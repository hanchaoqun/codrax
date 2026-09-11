package llm

import (
	"net/http"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/config"
	"github.com/hanchaoqun/codrax/internal/types"
	"gopkg.in/yaml.v3"
)

// Exercise the actual config inheritance and adapter factory, not just the
// default constants: longer silent waits must not become a streaming age cap.
func TestTimeoutDefaults_ConfigFactoryAndExplicitOverrides(t *testing.T) {
	var cfg types.ProvidersConfig
	if err := yaml.Unmarshal([]byte(`llm:
  default:
    provider: openai
    api_key: test-key
    model: test-model
    base_url: http://example.invalid
  agents:
    finalizer:
      request_timeout_seconds: 37
      stream_first_byte_timeout_seconds: 19
      stream_stall_timeout_seconds: 11
    planner_fallback:
      request_timeout_seconds: 900
      stream_first_byte_timeout_seconds: 900
      stream_stall_timeout_seconds: 450
    verifier:
      stream: false
`), &cfg); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                  string
		request, first, stall time.Duration
		stream                bool
	}{
		{"analyzer", 10 * time.Minute, 10 * time.Minute, 5 * time.Minute, true},
		{"planner", 10 * time.Minute, 10 * time.Minute, 5 * time.Minute, true},
		{"finalizer", 37 * time.Second, 19 * time.Second, 11 * time.Second, true},
		{"planner_fallback", 15 * time.Minute, 15 * time.Minute, 450 * time.Second, true},
		{"verifier", 10 * time.Minute, 10 * time.Minute, 5 * time.Minute, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter, err := NewFromConfig(config.ResolveProvider(&cfg, tc.name))
			if err != nil {
				t.Fatal(err)
			}
			got, ok := adapter.(*OpenAIAdapter)
			if !ok {
				t.Fatalf("adapter=%T", adapter)
			}
			if got.stream != tc.stream {
				t.Errorf("stream=%v want=%v", got.stream, tc.stream)
			}
			assertTimeoutDefaults(t, got, tc.request, tc.first, tc.stall)
		})
	}
}

func TestTimeoutDefaults_DirectConstructorWatchdogFallback(t *testing.T) {
	a := NewOpenAIAdapter("test-key", "test-model", "http://example.invalid", AdapterOptions{
		Stream: true, RequestTimeout: 23 * time.Second, RetryMaxAttempts: 1,
	})
	assertTimeoutDefaults(t, a, 23*time.Second, 10*time.Minute, 5*time.Minute)
}

func assertTimeoutDefaults(t *testing.T, a *OpenAIAdapter, request, first, stall time.Duration) {
	t.Helper()
	if a.requestTimeout != request || a.httpClient.Timeout != request {
		t.Errorf("non-stream timeout: adapter=%v client=%v want=%v", a.requestTimeout, a.httpClient.Timeout, request)
	}
	if a.streamFirstByteTimeout != first || a.streamStallTimeout != stall {
		t.Errorf("watchdog timeouts: first=%v stall=%v want=%v/%v", a.streamFirstByteTimeout, a.streamStallTimeout, first, stall)
	}
	if a.streamHTTPClient.Timeout != 0 {
		t.Errorf("stream client has total age limit: %v", a.streamHTTPClient.Timeout)
	}
	transport, ok := a.streamHTTPClient.Transport.(*http.Transport)
	if !ok || transport.ResponseHeaderTimeout != first {
		t.Errorf("response header deadline must match first-response wait: transport=%T", a.streamHTTPClient.Transport)
	}
}
