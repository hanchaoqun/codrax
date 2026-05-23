package render

import (
	"strings"
	"testing"
)

// TestRunTelemetrySuffix_BothLangs locks the dock terminal-summary
// suffix that communicates what kind of recovery happened. Surfaces three
// invariants:
//
//   - All counters zero → empty string. Clean / one-shot runs keep the terse
//     "✗ Failed · Total elapsed Xs" shape with no trailing parens.
//   - Transport retry, provider fallback, semantic answer rework, and
//     accept-with-boundary advisory are separate phrases.
//   - Combined counters use a locale-appropriate separator inside one compact
//     parenthesized suffix.
func TestRunTelemetrySuffix_BothLangs(t *testing.T) {
	cases := []struct {
		name             string
		retries          int
		fallbacks        int
		answerRewrites   int
		advisoryAccepted int
		zh               string
		en               string
	}{
		{name: "zero", zh: "", en: ""},
		{name: "one model retry zh", retries: 1, zh: "(模型请求重试 1 次)"},
		{name: "one model retry en", retries: 1, en: "(model request retried once)"},
		{name: "two model retries en", retries: 2, en: "(model request retried 2 times)"},
		{name: "five model retries zh", retries: 5, zh: "(模型请求重试 5 次)"},
		{name: "only provider fallback zh", fallbacks: 1, zh: "(切换 provider 1 次)"},
		{name: "only provider fallback en singular", fallbacks: 1, en: "(switched provider once)"},
		{name: "only provider fallback en plural", fallbacks: 3, en: "(switched provider 3 times)"},
		{name: "semantic rewrite zh", answerRewrites: 2, zh: "(答案返工 2 次)"},
		{name: "semantic rewrite en", answerRewrites: 2, en: "(answer reworked 2 times)"},
		{name: "accepted advisory zh", advisoryAccepted: 1, zh: "(带边界接受 1 次)"},
		{name: "accepted advisory en", advisoryAccepted: 2, en: "(accepted with boundary notes 2 times)"},
		{name: "combined en", retries: 3, fallbacks: 1, answerRewrites: 2, advisoryAccepted: 1, en: "(model request retried 3 times, switched provider once, answer reworked 2 times, accepted with boundary note once)"},
		{name: "combined zh", retries: 3, fallbacks: 1, answerRewrites: 2, advisoryAccepted: 1, zh: "(模型请求重试 3 次，切换 provider 1 次，答案返工 2 次，带边界接受 1 次)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.zh != "" {
				got := runTelemetrySuffix(c.retries, c.fallbacks, c.answerRewrites, c.advisoryAccepted, true)
				if got != c.zh {
					t.Errorf("zh: got %q, want %q", got, c.zh)
				}
			}
			if c.en != "" {
				got := runTelemetrySuffix(c.retries, c.fallbacks, c.answerRewrites, c.advisoryAccepted, false)
				if got != c.en {
					t.Errorf("en: got %q, want %q", got, c.en)
				}
			}
			if c.zh == "" && c.en == "" {
				if got := runTelemetrySuffix(c.retries, c.fallbacks, c.answerRewrites, c.advisoryAccepted, true); got != "" {
					t.Errorf("zh empty case should return empty; got %q", got)
				}
				if got := runTelemetrySuffix(c.retries, c.fallbacks, c.answerRewrites, c.advisoryAccepted, false); got != "" {
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
	if !strings.Contains(suffix, "model request retried 3 times") {
		t.Errorf("suffix should mention retry count; got %q", suffix)
	}
	if !strings.Contains(suffix, "switched provider once") {
		t.Errorf("suffix should mention provider switch; got %q", suffix)
	}
}

func TestRendererOrchestratorNoticeTelemetryBuckets(t *testing.T) {
	r := newTestRenderer("zh")
	for _, kind := range []OrchestratorNoticeKind{
		NoticeFallbackFinalizerOnly,
		NoticeFallbackBackToExtract,
		NoticeFallbackBackToExplore,
		NoticeSelfConsistencyContradictionRewriting,
		NoticeAnswerCheckRetry,
	} {
		r.recordOrchestratorNoticeTelemetry(kind)
	}
	if r.answerRewriteTotal != 5 {
		t.Fatalf("answerRewriteTotal = %d, want 5", r.answerRewriteTotal)
	}
	for _, kind := range []OrchestratorNoticeKind{
		NoticeFallbackFailLoud,
		NoticeFinalizeRepairCap,
		NoticeSelfConsistencyContradictionLogged,
		NoticeLowGrounding,
		NoticeYieldKill,
	} {
		r.recordOrchestratorNoticeTelemetry(kind)
	}
	if r.answerAdvisoryTotal != 5 {
		t.Fatalf("answerAdvisoryTotal = %d, want 5", r.answerAdvisoryTotal)
	}

	for _, kind := range []OrchestratorNoticeKind{
		NoticeRetry,
		NoticeNoToolCall,
		NoticeSelfConsistencyStart,
		NoticeSemanticQualityReviewStart,
		NoticeForcedRead,
		NoticeFinalizing,
		NoticeInvestigationReady,
		NoticeRepoMapScanProgress,
	} {
		r.recordOrchestratorNoticeTelemetry(kind)
	}
	if r.answerRewriteTotal != 5 || r.answerAdvisoryTotal != 5 {
		t.Fatalf("non-answer/advisory notices changed counters: rewrite=%d advisory=%d", r.answerRewriteTotal, r.answerAdvisoryTotal)
	}
}
