package render

import (
	"strings"
	"testing"
)

// TestRetryCountSuffix_BothLangs locks the dock terminal-summary
// suffix that communicates "we tried before failing". Surfaces three
// invariants:
//
//   - Zero retries AND zero fallbacks → empty string. Clean / one-shot
//     runs keep the terse "✗ Failed · Total elapsed Xs" shape with no
//     trailing parens.
//   - Pluralization is locale-correct: "retried 1 time" / "retried N
//     times" in en; "已重试 N 次" uniform in zh.
//   - Combined retry + fallback uses an in-paren separator (",") so a
//     "(retried 3 times, switched provider once)" reads naturally.
func TestRetryCountSuffix_BothLangs(t *testing.T) {
	cases := []struct {
		name      string
		retries   int
		fallbacks int
		zh        string
		en        string
	}{
		{"zero", 0, 0, "", ""},
		{"one retry zh", 1, 0, "(已重试 1 次)", ""},
		{"one retry en", 1, 0, "", "(retried 1 time)"},
		{"two retries en", 2, 0, "", "(retried 2 times)"},
		{"five retries zh", 5, 0, "(已重试 5 次)", ""},
		{"only fallback zh", 0, 1, "(切换 provider 1 次)", ""},
		{"only fallback en singular", 0, 1, "", "(switched provider once)"},
		{"only fallback en plural", 0, 3, "", "(switched provider 3 times)"},
		{"both en", 3, 1, "", "(retried 3 times, switched provider once)"},
		{"both zh", 3, 1, "(已重试 3 次，切换 provider 1 次)", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.zh != "" {
				got := retryCountSuffix(c.retries, c.fallbacks, true)
				if got != c.zh {
					t.Errorf("zh: got %q, want %q", got, c.zh)
				}
			}
			if c.en != "" {
				got := retryCountSuffix(c.retries, c.fallbacks, false)
				if got != c.en {
					t.Errorf("en: got %q, want %q", got, c.en)
				}
			}
			if c.zh == "" && c.en == "" {
				if got := retryCountSuffix(c.retries, c.fallbacks, true); got != "" {
					t.Errorf("zh empty case should return empty; got %q", got)
				}
				if got := retryCountSuffix(c.retries, c.fallbacks, false); got != "" {
					t.Errorf("en empty case should return empty; got %q", got)
				}
			}
		})
	}
}

// TestRendererCountsAdapterRetries pins the wiring: every
// EventAdapterRetry / EventAdapterFallback the renderer sees must
// bump the corresponding counter so the terminal-summary suffix
// gets accurate counts. Without this guard a future refactor that
// moves the increment into a TTY-only branch would break the
// non-TTY path silently.
func TestRendererCountsAdapterRetries(t *testing.T) {
	r := newTestRenderer("en")
	emit := r.Emitter()
	for i := 0; i < 3; i++ {
		emit(Event{Kind: EventAdapterRetry, RetryAttempt: i + 1, RetryDelay: 0})
	}
	emit(Event{Kind: EventAdapterFallback, FallbackFrom: "a", FallbackTo: "b"})

	if r.adapterRetryTotal != 3 {
		t.Errorf("adapterRetryTotal = %d, want 3", r.adapterRetryTotal)
	}
	if r.adapterFallbackTotal != 1 {
		t.Errorf("adapterFallbackTotal = %d, want 1", r.adapterFallbackTotal)
	}

	// Verify the suffix produced from the live state matches the
	// observable output the dock would render.
	suffix := retryCountSuffix(r.adapterRetryTotal, r.adapterFallbackTotal, false)
	if !strings.Contains(suffix, "retried 3 times") {
		t.Errorf("suffix should mention retry count; got %q", suffix)
	}
	if !strings.Contains(suffix, "switched provider once") {
		t.Errorf("suffix should mention provider switch; got %q", suffix)
	}
}
