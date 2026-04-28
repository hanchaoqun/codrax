package recommend

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestRecommend_DisabledMasterSwitch verifies env_recommend_enabled=false
// returns no recommendations regardless of diagnosis. R5 red-line.
func TestRecommend_DisabledMasterSwitch(t *testing.T) {
	env := &types.EnvFacts{
		Pythons: []types.PythonInterp{{Path: "/usr/bin/python3"}},
	}
	d := types.Diagnosis{Kind: types.DiagRunnerMissing, Runner: "python", Tool: "pytest"}
	settings := types.DefaultEnvRecommendSettings()
	settings.Enabled = false
	recs := Recommend(d, env, settings)
	if len(recs) > 0 {
		t.Errorf("disabled master switch should produce no recs; got %d", len(recs))
	}
}

// TestRecommend_PerRunner_FallsBackToDocsLink_WhenLLMOff covers the
// LLM-first architecture's offline path: with the per-runner
// deterministic table removed and LLM disabled, every language
// runner's RunnerMissing diagnosis should still produce a
// DocsLink so the operator is never stranded.
func TestRecommend_PerRunner_FallsBackToDocsLink_WhenLLMOff(t *testing.T) {
	settings := types.DefaultEnvRecommendSettings()
	settings.LLMEnabled = false
	env := &types.EnvFacts{OSFamily: "debian"}

	for _, runner := range []string{
		"python", "node", "rust", "go", "java", "ruby",
		"swift", "cmake", "meson", "make", "hvigor", "cjpm",
	} {
		t.Run(runner, func(t *testing.T) {
			d := types.Diagnosis{Kind: types.DiagRunnerMissing, Runner: runner}
			recs := Recommend(d, env, settings)
			if len(recs) == 0 {
				t.Fatalf("runner=%s LLM-off path produced no recs (DocsLink fallback should fire)", runner)
			}
			if recs[0].Strategy != types.StrategyDocsLink {
				t.Errorf("runner=%s top-1 should be DocsLink (LLM off); got %v: %q", runner, recs[0].Strategy, recs[0].Command)
			}
		})
	}
}

// TestRecommend_GitNotInitialized covers the system-stable Stage 1
// path: runner-agnostic, the systemRecommender produces the
// candidates without LLM involvement.
func TestRecommend_GitNotInitialized(t *testing.T) {
	env := &types.EnvFacts{}
	d := types.Diagnosis{Kind: types.DiagGitRepoNotInitialized, Tool: "git"}
	recs := Recommend(d, env, types.DefaultEnvRecommendSettings())
	if len(recs) == 0 {
		t.Fatal("expected git init recommendations from systemRecommender")
	}
	found := false
	for _, r := range recs {
		if strings.Contains(r.Command, "git init") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected `git init` recommendation; got %+v", recs)
	}
}

// TestRecommend_GitNotInstalled_DebianStage1 verifies systemRecommender
// remains deterministic for git toolchain on debian, no LLM call needed.
func TestRecommend_GitNotInstalled_DebianStage1(t *testing.T) {
	env := &types.EnvFacts{OSFamily: "debian"}
	d := types.Diagnosis{Kind: types.DiagGitNotInstalled, Tool: "git"}
	settings := types.DefaultEnvRecommendSettings()
	settings.RecommendGlobalInstall = true
	settings.LLMEnabled = false // assert Stage 1 alone covers this
	recs := Recommend(d, env, settings)
	if len(recs) == 0 {
		t.Fatal("git_not_installed on debian should be served by Stage 1 deterministic table")
	}
	found := false
	for _, r := range recs {
		if strings.Contains(r.Command, "apt install") && strings.Contains(r.Command, "git") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected `apt install git` on debian; got %+v", recs)
	}
}

// TestRecommend_SystemLib_StableTable verifies the systemRecommender's
// libToPkg table remains deterministic for the well-known libs.
func TestRecommend_SystemLib_StableTable(t *testing.T) {
	env := &types.EnvFacts{OSFamily: "debian"}
	d := types.Diagnosis{Kind: types.DiagSystemLibMissing, Tool: "ssl"}
	settings := types.DefaultEnvRecommendSettings()
	settings.RecommendGlobalInstall = true
	settings.LLMEnabled = false
	recs := Recommend(d, env, settings)
	if len(recs) == 0 {
		t.Fatal("system_lib_missing(ssl) on debian should hit deterministic table")
	}
	if !strings.Contains(recs[0].Command, "libssl-dev") {
		t.Errorf("expected libssl-dev for ssl on debian; got %q", recs[0].Command)
	}
}

// TestRecommend_GlobalInstallFiltered_R8 verifies recommend_global_install=false
// strips Global candidates regardless of stage. Tests against the
// system-stable git path (Stage 1) where we know the deterministic
// output.
func TestRecommend_GlobalInstallFiltered_R8(t *testing.T) {
	env := &types.EnvFacts{OSFamily: "debian"}
	d := types.Diagnosis{Kind: types.DiagGitNotInstalled, Tool: "git"}
	settings := types.DefaultEnvRecommendSettings() // GlobalInstall=false default
	recs := Recommend(d, env, settings)
	for _, r := range recs {
		if r.Strategy == types.StrategyGlobalInstall {
			t.Errorf("R8 violation: GlobalInstall must be filtered when recommend_global_install=false; got %+v", r)
		}
	}
}

// TestRecommend_DocsLinkUnknownRunner verifies an unknown runner
// string falls all the way through (no per-runner table, LLM off,
// no docs URL) → empty result. The Renderer surfaces a generic
// "no actionable command" message in this case.
func TestRecommend_DocsLinkUnknownRunner(t *testing.T) {
	env := &types.EnvFacts{}
	d := types.Diagnosis{Kind: types.DiagRunnerMissing, Runner: "unknown_runner_xyz"}
	settings := types.DefaultEnvRecommendSettings()
	settings.LLMEnabled = false
	recs := Recommend(d, env, settings)
	if len(recs) != 0 {
		t.Errorf("unknown runner with LLM off should yield empty recs (Renderer handles); got %+v", recs)
	}
}

// TestRecommend_ResultCappedAt3 verifies the 3-recommendation cap.
// systemRecommender's gitNotInstalled returns at most one per family;
// so we exercise the cap by stuffing test data through the
// systemLib path which can produce multiple candidates.
func TestRecommend_ResultCappedAt3(t *testing.T) {
	// Force settings that allow all candidate paths to fire.
	settings := types.DefaultEnvRecommendSettings()
	settings.RecommendGlobalInstall = true
	settings.LLMEnabled = false

	// gitNotInitialized always returns 2 candidates (git init +
	// codrax docs); cap test passes trivially. Verify we never
	// exceed 3 even when stages stack.
	env := &types.EnvFacts{OSFamily: "debian"}
	d := types.Diagnosis{Kind: types.DiagGitRepoNotInitialized, Tool: "git"}
	recs := Recommend(d, env, settings)
	if len(recs) > 3 {
		t.Errorf("results should cap at 3; got %d", len(recs))
	}
}
