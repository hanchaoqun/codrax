package orchestrator

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/env"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
)

// bareDirAuthorizationMessage produces the user-facing error
// message the apply pre-hook surfaces when the target directory
// is not a git repo and no auto-init authorization was granted.
//
// When env_recommend is enabled and EnvFacts is available, routes
// through env.Diagnose + env.Recommend + env.RenderRecommendations
// so the message carries `!`-prefixed copy-pastable commands +
// the three-tier authorization explanation. Falls back to the
// legacy hardcoded prose when env_recommend is disabled (R6 red
// line: byte-identical pre-feature behaviour).
func bareDirAuthorizationMessage(ctx *types.BusContext, state worktree.RepoState) string {
	legacy := fmt.Sprintf(
		"apply stage: target %s is %s — codrax can scaffold it (`git init` + empty initial commit) but needs explicit authorization. "+
			"Pass --auto-init-repo (single-shot CLI), set codrax.yaml :: write_auto_init_repo: true (deploy-wide), "+
			"or in the REPL answer y to the consent prompt that fires before /approve.",
		ctx.MainRepoRoot, state)

	// env_recommend integration. When the master switch is off or
	// EnvFacts hasn't been probed, surface the legacy prose so
	// behaviour is byte-identical with pre-env_recommend.
	if !ctx.EnvRecommendSettings.Enabled || ctx.EnvFacts == nil {
		return legacy
	}

	// Map RepoState to the env_recommend DiagKind. Both NeedsInit
	// states reach this function (RepoNotInitialized + RepoNoCommits);
	// RepoReady is filtered upstream in stage_hooks.go.
	var kind types.DiagKind
	switch state {
	case worktree.RepoNoCommits:
		kind = types.DiagGitRepoNoCommits
	default:
		kind = types.DiagGitRepoNotInitialized
	}
	d := types.Diagnosis{
		Kind:       kind,
		Tool:       "git",
		Source:     "apply_pre_hook: detected via worktree.DetectRepoState",
		Confidence: 1.0,
	}
	recs := env.Recommend(d, ctx.EnvFacts, ctx.EnvRecommendSettings)
	rendered := env.RenderRecommendations(d, recs, ctx.Language)
	return fmt.Sprintf("apply stage 拒绝:目标目录非 git 仓且未授权自动初始化。\n\n%s\n\n如需 codrax 自动 scaffold(走三层授权门):\n  - 单次 CLI:加 --auto-init-repo\n  - 持久化:codrax.yaml 设 write_auto_init_repo: true\n  - REPL 模式:下次 /approve 前会有 y/N 确认提示", rendered)
}
