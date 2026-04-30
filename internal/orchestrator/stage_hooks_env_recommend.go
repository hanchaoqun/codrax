package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/env"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
)

// bareDirAuthorizationMessage produces the user-facing error
// message stage pre-hooks surface when the target directory is
// not a git repo and no auto-init authorization was granted.
//
// stage names which pipeline phase refused to dispatch (currently
// "plan" or "apply"). Threading the label through the message keeps
// the surfaced text accurate — pre-2026-04-29 the function was
// only called from applyPreHook so the prose hardcoded "apply
// stage"; once planPreHook started calling it for the plan-only
// path the stage name needed to track the actual gate that fired.
//
// When env_recommend is enabled and EnvFacts is available, routes
// through env.Diagnose + env.Recommend + env.RenderRecommendations
// so the message carries `!`-prefixed copy-pastable commands +
// the three-tier authorization explanation. Falls back to the
// legacy hardcoded prose when env_recommend is disabled (R6 red
// line: byte-identical pre-feature behaviour for the apply path —
// the plan path is new and has no R6 byte-identity to preserve).
func bareDirAuthorizationMessage(ctx *types.BusContext, state worktree.RepoState, stage string) string {
	if stage == "" {
		stage = "apply"
	}
	zh := ctx.Language == "" || strings.HasPrefix(strings.ToLower(ctx.Language), "zh")
	var legacy string
	if zh {
		legacy = fmt.Sprintf(
			"目录 %s 还不是一个 git 仓库,需要先把它准备好才能改代码。我可以帮你准备(等同于 git init 并打一个空的初始提交),但这一步需要你明确同意。\n\n"+
				"同意的方式:\n"+
				"  - 命令行:加 --auto-init-repo\n"+
				"  - 配置文件:在 codrax.yaml 里设 write_auto_init_repo: true(对所有运行生效)\n"+
				"  - 交互模式:批准计划前会有 y/N 的提示,回 y 即可",
			ctx.MainRepoRoot)
	} else {
		legacy = fmt.Sprintf(
			"directory %s is not a git repo yet, and it needs to be prepared before any code change can land. I can prepare it for you (the equivalent of git init plus an empty initial commit), but only if you explicitly say so.\n\n"+
				"Ways to say so:\n"+
				"  - command line: add --auto-init-repo\n"+
				"  - config file: set write_auto_init_repo: true in codrax.yaml (applies to every run)\n"+
				"  - interactive mode: answer y to the y/N prompt shown before approving a plan",
			ctx.MainRepoRoot)
	}

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
		Source:     fmt.Sprintf("%s_pre_hook: detected via worktree.DetectRepoState", stage),
		Confidence: 1.0,
	}
	recs := env.Recommend(d, ctx.EnvFacts, ctx.EnvRecommendSettings)
	rendered := env.RenderRecommendations(d, recs, ctx.Language)
	if zh {
		return fmt.Sprintf("目录还不是 git 仓库,且没有授权我自动准备它,所以无法开始改代码。\n\n%s\n\n要让我自动准备(三种任选其一):\n  - 命令行:加 --auto-init-repo\n  - 配置文件:在 codrax.yaml 里设 write_auto_init_repo: true\n  - 交互模式:批准计划前回 y", rendered)
	}
	return fmt.Sprintf("the target directory is not a git repo yet, and I haven't been told it's OK to prepare it, so the work cannot start.\n\n%s\n\nTo let me prepare it (any one of):\n  - command line: add the --auto-init-repo flag\n  - config file: set write_auto_init_repo: true in codrax.yaml\n  - interactive mode: answer y before approving the plan", rendered)
}
