package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
)

// TestBareDirAuthorizationMessage_DisabledMasterSwitch is the R6
// byte-identity guard for the apply pre-hook integration. When
// env_recommend_enabled=false, the helper MUST return the legacy
// hardcoded prose verbatim — no env_recommend block, no rendered
// recommendations. Future refactors that accidentally couple
// rendered output to the disabled path break this.
func TestBareDirAuthorizationMessage_DisabledMasterSwitch(t *testing.T) {
	ctx := &types.BusContext{
		MainRepoRoot:         "/tmp/blank-dir",
		EnvRecommendSettings: types.EnvRecommendSettings{Enabled: false},
		EnvFacts:             &types.EnvFacts{OSFamily: "debian"},
	}
	got := bareDirAuthorizationMessage(ctx, worktree.RepoNotInitialized)
	if !strings.Contains(got, "apply stage: target") {
		t.Errorf("R6 violation: disabled path must include legacy `apply stage: target` lead; got %q", got)
	}
	if strings.Contains(got, "✗ 环境诊断") || strings.Contains(got, "环境诊断") {
		t.Errorf("R6 violation: env_recommend Renderer block must NOT appear when Enabled=false; got %q", got)
	}
	if !strings.Contains(got, "--auto-init-repo") {
		t.Errorf("legacy prose must mention --auto-init-repo authorization surface; got %q", got)
	}
}

// TestBareDirAuthorizationMessage_NilFactsFallsBack covers the
// partial-init path: env_recommend_enabled=true but EnvFacts is
// nil (Run started before the probe completed for whatever reason).
// Helper must return legacy verbatim — no panic on nil deref.
func TestBareDirAuthorizationMessage_NilFactsFallsBack(t *testing.T) {
	ctx := &types.BusContext{
		MainRepoRoot:         "/tmp/blank-dir",
		EnvRecommendSettings: types.EnvRecommendSettings{Enabled: true},
		EnvFacts:             nil,
	}
	got := bareDirAuthorizationMessage(ctx, worktree.RepoNotInitialized)
	if !strings.Contains(got, "apply stage: target") {
		t.Errorf("nil EnvFacts must surface legacy prose; got %q", got)
	}
	if strings.Contains(got, "✗ 环境诊断") {
		t.Errorf("nil EnvFacts path must NOT render env_recommend block; got %q", got)
	}
}

// TestBareDirAuthorizationMessage_EnabledRendersRecommendations
// covers the happy path: env_recommend on + EnvFacts probed +
// state=NotInitialized → message contains the env_recommend
// rendered block AND the three-tier authorization explanation.
func TestBareDirAuthorizationMessage_EnabledRendersRecommendations(t *testing.T) {
	ctx := &types.BusContext{
		MainRepoRoot:         "/tmp/blank-dir",
		EnvRecommendSettings: types.DefaultEnvRecommendSettings(),
		EnvFacts:             &types.EnvFacts{OSFamily: "debian"},
		Language:             "zh",
	}
	got := bareDirAuthorizationMessage(ctx, worktree.RepoNotInitialized)
	if !strings.Contains(got, "apply stage 拒绝") {
		t.Errorf("enabled path must wrap the rendered block in the bare-dir refusal message; got %q", got)
	}
	if !strings.Contains(got, "环境诊断") {
		t.Errorf("expected env_recommend rendered block; got %q", got)
	}
	if !strings.Contains(got, "--auto-init-repo") {
		t.Errorf("authorization-surface explanation must remain (3-tier gate); got %q", got)
	}
}

// TestBareDirAuthorizationMessage_NoCommitsState verifies the
// state→DiagKind mapping. RepoNoCommits should produce the
// gitNoCommits Recommender output, not gitNotInitialized.
func TestBareDirAuthorizationMessage_NoCommitsState(t *testing.T) {
	ctx := &types.BusContext{
		MainRepoRoot:         "/tmp/blank-dir",
		EnvRecommendSettings: types.DefaultEnvRecommendSettings(),
		EnvFacts:             &types.EnvFacts{OSFamily: "debian"},
		Language:             "zh",
	}
	got := bareDirAuthorizationMessage(ctx, worktree.RepoNoCommits)
	// systemRecommender's gitNoCommits surfaces `git commit --allow-empty`.
	if !strings.Contains(got, "git commit --allow-empty") {
		t.Errorf("RepoNoCommits should route to gitNoCommits recommender (allow-empty); got %q", got)
	}
}
