package env

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestRenderRecommendations_BangPrefix locks the user contract:
// every command in the rendered output starts with `!` so
// copy-paste into the codrax prompt works directly.
func TestRenderRecommendations_BangPrefix(t *testing.T) {
	d := types.Diagnosis{Kind: types.DiagRunnerMissing, Runner: "python", Tool: "pytest"}
	recs := []types.Recommendation{
		{Strategy: types.StrategyVenvInstall, Command: "!python3 -m venv .venv", Why: "x"},
		{Strategy: types.StrategyUserInstall, Command: "!pip install --user pytest", Why: "y"},
	}
	out := RenderRecommendations(d, recs, "zh")
	for _, r := range recs {
		if !strings.Contains(out, r.Command) {
			t.Errorf("zh render missing command %q in: %s", r.Command, out)
		}
	}
	if !strings.Contains(out, "!") {
		t.Errorf("expected bang prefix in render: %s", out)
	}
}

// TestRenderRecommendations_BilingualBranching covers zh + en
// produce different headings.
func TestRenderRecommendations_BilingualBranching(t *testing.T) {
	d := types.Diagnosis{Kind: types.DiagGitRepoNotInitialized, Tool: "git"}
	recs := []types.Recommendation{{Strategy: types.StrategyProjectInstall, Command: "!git init"}}
	zh := RenderRecommendations(d, recs, "zh")
	en := RenderRecommendations(d, recs, "en")
	if !strings.Contains(zh, "环境诊断") {
		t.Errorf("zh missing zh header")
	}
	if !strings.Contains(en, "Environment diagnosis") {
		t.Errorf("en missing en header")
	}
}

// TestRenderRecommendations_EmptyRecsStillRenders covers the
// "no actionable recommendation" path: caller still gets useful
// text, never an empty string.
func TestRenderRecommendations_EmptyRecsStillRenders(t *testing.T) {
	d := types.Diagnosis{Kind: types.DiagUnknown}
	out := RenderRecommendations(d, nil, "zh")
	if strings.TrimSpace(out) == "" {
		t.Error("empty recs should still produce a render")
	}
}
