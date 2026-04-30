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
		Language:             "zh",
	}
	got := bareDirAuthorizationMessage(ctx, worktree.RepoNotInitialized, "apply")
	// Legacy disabled path must remain plain-prose: no env_recommend
	// rendered block, but must still describe the directory state +
	// the three-tier authorization surfaces. The exact wording
	// changed to plain language post-2026-04-29; the assertions
	// below pin the surfaces by token, not full sentences.
	if !strings.Contains(got, "/tmp/blank-dir") {
		t.Errorf("disabled path must name the offending directory path; got %q", got)
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
// Helper must return legacy plain-prose verbatim — no panic on
// nil deref, and no env_recommend rendered block.
func TestBareDirAuthorizationMessage_NilFactsFallsBack(t *testing.T) {
	ctx := &types.BusContext{
		MainRepoRoot:         "/tmp/blank-dir",
		EnvRecommendSettings: types.EnvRecommendSettings{Enabled: true},
		EnvFacts:             nil,
		Language:             "zh",
	}
	got := bareDirAuthorizationMessage(ctx, worktree.RepoNotInitialized, "apply")
	if !strings.Contains(got, "/tmp/blank-dir") {
		t.Errorf("nil EnvFacts path must name the offending directory; got %q", got)
	}
	if strings.Contains(got, "✗ 环境诊断") {
		t.Errorf("nil EnvFacts path must NOT render env_recommend block; got %q", got)
	}
	if !strings.Contains(got, "--auto-init-repo") {
		t.Errorf("legacy prose must mention --auto-init-repo authorization surface; got %q", got)
	}
}

// TestBareDirAuthorizationMessage_PlainProse pins the post-2026-04-30
// rewrite: a single clean message naming the path, the not-a-repo
// state, and the authorization options for THIS stage. No
// env_recommend recommendation block, no internal "信号: …" /
// "✗ 环境诊断" debug labels, no duplicated three-option list. The
// helper used to invoke env.Diagnose + env.Recommend +
// env.RenderRecommendations and concatenate all three with the
// suffix's own three-option list — the customer reported the
// result as 繁琐 / contradictory and (worse) it advertised the
// y/N option in PLAN stage where the user can never reach an
// /approve prompt.
func TestBareDirAuthorizationMessage_PlainProse(t *testing.T) {
	ctx := &types.BusContext{
		MainRepoRoot:         "/tmp/blank-dir",
		EnvRecommendSettings: types.DefaultEnvRecommendSettings(),
		EnvFacts:             &types.EnvFacts{OSFamily: "debian"},
		Language:             "zh",
	}
	got := bareDirAuthorizationMessage(ctx, worktree.RepoNotInitialized, "apply")
	if !strings.Contains(got, "git 仓库") {
		t.Errorf("must describe the not-a-repo state; got %q", got)
	}
	if !strings.Contains(got, "/tmp/blank-dir") {
		t.Errorf("must name the offending directory; got %q", got)
	}
	if !strings.Contains(got, "--auto-init-repo") {
		t.Errorf("must surface the CLI authorization surface; got %q", got)
	}
	if !strings.Contains(got, "write_auto_init_repo") {
		t.Errorf("must surface the yaml authorization surface; got %q", got)
	}
	// Internal debug labels MUST NOT leak into user-facing prose.
	for _, banned := range []string{"环境诊断", "信号:", "信号：", "plan_pre_hook", "apply_pre_hook", "DetectRepoState"} {
		if strings.Contains(got, banned) {
			t.Errorf("internal debug label %q leaked to user message; got %q", banned, got)
		}
	}
}

// TestBareDirAuthorizationMessage_PlanStageOmitsInteractive locks
// the dead-lock fix: in PLAN stage the message must NOT advertise
// the y/N interactive option. The user is blocked at plan
// generation; advertising "answer y before /approve" is
// unreachable advice that confuses the operator.
func TestBareDirAuthorizationMessage_PlanStageOmitsInteractive(t *testing.T) {
	ctx := &types.BusContext{
		MainRepoRoot:         "/tmp/blank-dir",
		EnvRecommendSettings: types.DefaultEnvRecommendSettings(),
		EnvFacts:             &types.EnvFacts{OSFamily: "debian"},
		Language:             "zh",
	}
	got := bareDirAuthorizationMessage(ctx, worktree.RepoNotInitialized, "plan")
	if strings.Contains(got, "/approve") || strings.Contains(got, "y/N") {
		t.Errorf("plan-stage message must NOT mention /approve y/N (user is blocked at plan, can never reach /approve); got %q", got)
	}
	// CLI + yaml authorization remain — they DO work in plan stage.
	for _, surface := range []string{"--auto-init-repo", "write_auto_init_repo"} {
		if !strings.Contains(got, surface) {
			t.Errorf("plan-stage message must keep %q (still applicable); got %q", surface, got)
		}
	}
}
