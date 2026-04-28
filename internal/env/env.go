// Package env is the public façade callers (run_tests, REPL, etc)
// import. It re-exports the Diagnose / Recommend / Probe entry
// points so consumers don't need to know the internal layering.
package env

import (
	"github.com/hanchaoqun/codrax/internal/env/diag"
	"github.com/hanchaoqun/codrax/internal/env/probe"
	"github.com/hanchaoqun/codrax/internal/env/recommend"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Probe runs the environment scan. Cheap, ~400ms total. Caller
// caches the result on BusContext for the lifetime of the Run.
func Probe(opts ProbeOptions) *types.EnvFacts {
	return probe.Run(probe.Options{
		RepoRoot:     opts.RepoRoot,
		ProbeNetwork: opts.ProbeNetwork,
	})
}

// ProbeOptions parameterise Probe.
type ProbeOptions struct {
	RepoRoot     string
	ProbeNetwork bool
}

// Diagnose classifies a failure into a structured Diagnosis.
// Wraps internal/env/diag.Diagnose.
func Diagnose(stderr, runner string, env *types.EnvFacts) types.Diagnosis {
	return diag.Diagnose(stderr, runner, env)
}

// Recommend produces ranked install suggestions for a Diagnosis.
// Wraps internal/env/recommend.Recommend (which dispatches to the
// per-runner table → cache → LLM → DocsLink pipeline).
func Recommend(d types.Diagnosis, env *types.EnvFacts, settings types.EnvRecommendSettings) []types.Recommendation {
	return recommend.Recommend(d, env, settings)
}

// SetLLMAdapter wires the env_recommender LLM (Stage 3 fallback)
// from cmd/root.go. nil disables the LLM stage entirely.
func SetLLMAdapter(a llm.Adapter) {
	recommend.SetLLMAdapter(a)
}

// SetCacheTTL forwards the configured TTL to the disk cache layer.
// Called from cmd/root.go after yaml settings resolution.
func SetCacheTTL(days int) {
	recommend.SetCacheTTL(days)
}

// Metrics returns a snapshot of the env_recommend pipeline counters
// (calls / stage1 hits / cache hits / LLM calls / docslink
// fallbacks / etc). Stable schema — used by REPL `/env stats` and
// any future telemetry exporter. Pure read; safe to call from any
// goroutine.
func Metrics() recommend.MetricsSnapshot {
	return recommend.Snapshot()
}

// ResetMetrics zeros the counter set and bumps SinceUnix. Called
// by REPL `/env stats reset` and by tests that want a clean window.
func ResetMetrics() {
	recommend.ResetMetrics()
}
