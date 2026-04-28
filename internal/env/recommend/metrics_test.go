package recommend

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestMetrics_TracksDisabled covers the R5 short-circuit path:
// Recommend bumps both Calls and DisabledCalls when settings off.
func TestMetrics_TracksDisabled(t *testing.T) {
	ResetMetrics()
	settings := types.DefaultEnvRecommendSettings()
	settings.Enabled = false

	for i := 0; i < 3; i++ {
		Recommend(types.Diagnosis{Kind: types.DiagRunnerMissing, Runner: "python"}, &types.EnvFacts{}, settings)
	}
	s := Snapshot()
	if s.Calls != 3 {
		t.Errorf("Calls = %d, want 3", s.Calls)
	}
	if s.DisabledCalls != 3 {
		t.Errorf("DisabledCalls = %d, want 3", s.DisabledCalls)
	}
}

// TestMetrics_TracksStage1Hit verifies the system-stable Stage 1
// (git_state) increments only Stage1Hits, not LLM/cache/docslink.
func TestMetrics_TracksStage1Hit(t *testing.T) {
	ResetMetrics()
	env := &types.EnvFacts{OSFamily: "debian"}
	settings := types.DefaultEnvRecommendSettings()
	settings.RecommendGlobalInstall = true
	settings.LLMEnabled = false

	d := types.Diagnosis{Kind: types.DiagGitNotInstalled, Tool: "git"}
	recs := Recommend(d, env, settings)
	if len(recs) == 0 {
		t.Fatal("git_not_installed should produce Stage 1 hits")
	}

	s := Snapshot()
	if s.Stage1Hits != 1 {
		t.Errorf("Stage1Hits = %d, want 1", s.Stage1Hits)
	}
	if s.LLMCalls != 0 {
		t.Errorf("LLMCalls = %d, want 0 (LLM should not fire)", s.LLMCalls)
	}
	if s.DocsLinkFallbacks != 0 {
		t.Errorf("DocsLinkFallbacks = %d, want 0", s.DocsLinkFallbacks)
	}
}

// TestMetrics_TracksDocsLinkFallback verifies the LLM-off path
// for a per-runner DiagKind increments DocsLinkFallbacks.
func TestMetrics_TracksDocsLinkFallback(t *testing.T) {
	ResetMetrics()
	settings := types.DefaultEnvRecommendSettings()
	settings.LLMEnabled = false

	d := types.Diagnosis{Kind: types.DiagRunnerMissing, Runner: "python"}
	recs := Recommend(d, &types.EnvFacts{}, settings)
	if len(recs) == 0 {
		t.Fatal("expected DocsLink fallback for python LLM-off")
	}

	s := Snapshot()
	if s.DocsLinkFallbacks != 1 {
		t.Errorf("DocsLinkFallbacks = %d, want 1", s.DocsLinkFallbacks)
	}
	if s.LLMCalls != 0 {
		t.Errorf("LLMCalls = %d, want 0", s.LLMCalls)
	}
	if s.EmptyResults != 0 {
		t.Errorf("EmptyResults = %d, want 0", s.EmptyResults)
	}
}

// TestMetrics_TracksEmptyResults verifies that an unknown runner
// with LLM off (no DocsLink fallback) correctly counts EmptyResults.
func TestMetrics_TracksEmptyResults(t *testing.T) {
	ResetMetrics()
	settings := types.DefaultEnvRecommendSettings()
	settings.LLMEnabled = false

	d := types.Diagnosis{Kind: types.DiagRunnerMissing, Runner: "xyz_unknown"}
	recs := Recommend(d, &types.EnvFacts{}, settings)
	if len(recs) != 0 {
		t.Fatalf("unknown runner LLM-off should be empty; got %+v", recs)
	}

	s := Snapshot()
	if s.EmptyResults != 1 {
		t.Errorf("EmptyResults = %d, want 1", s.EmptyResults)
	}
	if s.DocsLinkFallbacks != 0 {
		t.Errorf("DocsLinkFallbacks = %d, want 0 (no URL for unknown runner)", s.DocsLinkFallbacks)
	}
}

// TestMetrics_ResetClearsAndUpdatesSince verifies ResetMetrics
// zeros all counters and bumps the SinceUnix to a new timestamp.
func TestMetrics_ResetClearsAndUpdatesSince(t *testing.T) {
	settings := types.DefaultEnvRecommendSettings()
	settings.LLMEnabled = false

	// Generate non-zero state.
	Recommend(types.Diagnosis{Kind: types.DiagRunnerMissing, Runner: "python"},
		&types.EnvFacts{}, settings)
	pre := Snapshot()
	if pre.Calls == 0 {
		t.Fatal("setup: pre-reset Calls should be > 0")
	}

	ResetMetrics()
	post := Snapshot()
	if post.Calls != 0 || post.DocsLinkFallbacks != 0 {
		t.Errorf("ResetMetrics did not clear: %+v", post)
	}
	if post.SinceUnix < pre.SinceUnix {
		t.Errorf("SinceUnix went backwards: pre=%d post=%d", pre.SinceUnix, post.SinceUnix)
	}
}
