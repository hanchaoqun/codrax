package recommend

import (
	"sync/atomic"
	"time"
)

// MetricsSnapshot is the readable form of the Recommend pipeline's
// accumulated counters. Returned by Snapshot() and consumed by the
// REPL `/env stats` command. Designed to be a stable schema —
// downstream telemetry / docs reference these field names.
//
// Snapshot is a value (not pointer); callers can stash and compare
// successive snapshots without coordination. All counts are
// monotonically non-decreasing within a process lifetime.
type MetricsSnapshot struct {
	// Calls is every Recommend() invocation, regardless of outcome.
	Calls uint64 `json:"calls"`

	// DisabledCalls counts invocations short-circuited by
	// Settings.Enabled=false (R5 red line).
	DisabledCalls uint64 `json:"disabled_calls"`

	// Stage1Hits counts requests served by the system-stable
	// deterministic table (git_state / system_lib_missing).
	Stage1Hits uint64 `json:"stage1_hits"`

	// CacheHits counts requests served by the disk-cache (Stage 2)
	// after a previous LLM call seeded it.
	CacheHits uint64 `json:"cache_hits"`

	// LLMCalls counts requests that reached Stage 3 (registry +
	// cache miss). Each is one network round-trip to env_recommender.
	LLMCalls uint64 `json:"llm_calls"`

	// LLMSuccess is the subset of LLMCalls that returned at least
	// one parsed recommendation.
	LLMSuccess uint64 `json:"llm_success"`

	// LLMTimeouts counts the env_recommend_llm_timeout_sec
	// triggers — observable signal for cheap-model latency drift.
	LLMTimeouts uint64 `json:"llm_timeouts"`

	// LLMErrors counts non-timeout failures (network / schema
	// invalid / wrong tool name / etc.). Helpful for distinguishing
	// model-side issues from deadline issues.
	LLMErrors uint64 `json:"llm_errors"`

	// DocsLinkFallbacks counts requests served by Stage 4 (no LLM,
	// or LLM returned empty). Each represents an operator who saw
	// only a docs URL — high count signals the LLM stage is
	// under-performing or disabled.
	DocsLinkFallbacks uint64 `json:"docslink_fallbacks"`

	// EmptyResults is the worst-case: registry miss + cache miss +
	// LLM off/empty + DocsLink miss (unknown runner). The Renderer
	// surfaces a generic "consult docs" footer in that case.
	EmptyResults uint64 `json:"empty_results"`

	// SinceUnix is when the counters were last reset. Lets
	// snapshots be expressed as rates over a known window.
	SinceUnix int64 `json:"since_unix"`
}

// metrics is the package-private counter set. Exported only via
// Snapshot() / ResetMetrics() so callers can never write directly.
var metrics struct {
	calls             atomic.Uint64
	disabledCalls     atomic.Uint64
	stage1Hits        atomic.Uint64
	cacheHits         atomic.Uint64
	llmCalls          atomic.Uint64
	llmSuccess        atomic.Uint64
	llmTimeouts       atomic.Uint64
	llmErrors         atomic.Uint64
	docsLinkFallbacks atomic.Uint64
	emptyResults      atomic.Uint64
	sinceUnix         atomic.Int64
}

func init() {
	metrics.sinceUnix.Store(time.Now().Unix())
}

// Snapshot returns a point-in-time copy of the counters. Reads are
// non-blocking; concurrent Recommend() calls during snapshot may
// see slightly inconsistent totals across fields, which is
// acceptable for an INFO-level dashboard.
func Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		Calls:             metrics.calls.Load(),
		DisabledCalls:     metrics.disabledCalls.Load(),
		Stage1Hits:        metrics.stage1Hits.Load(),
		CacheHits:         metrics.cacheHits.Load(),
		LLMCalls:          metrics.llmCalls.Load(),
		LLMSuccess:        metrics.llmSuccess.Load(),
		LLMTimeouts:       metrics.llmTimeouts.Load(),
		LLMErrors:         metrics.llmErrors.Load(),
		DocsLinkFallbacks: metrics.docsLinkFallbacks.Load(),
		EmptyResults:      metrics.emptyResults.Load(),
		SinceUnix:         metrics.sinceUnix.Load(),
	}
}

// ResetMetrics zeros all counters and updates the SinceUnix
// timestamp. Used by `/env stats reset` and by tests that want to
// observe a clean window.
func ResetMetrics() {
	metrics.calls.Store(0)
	metrics.disabledCalls.Store(0)
	metrics.stage1Hits.Store(0)
	metrics.cacheHits.Store(0)
	metrics.llmCalls.Store(0)
	metrics.llmSuccess.Store(0)
	metrics.llmTimeouts.Store(0)
	metrics.llmErrors.Store(0)
	metrics.docsLinkFallbacks.Store(0)
	metrics.emptyResults.Store(0)
	metrics.sinceUnix.Store(time.Now().Unix())
}
