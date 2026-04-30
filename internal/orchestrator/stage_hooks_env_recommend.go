package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
)

// bareDirAuthorizationMessage produces the user-facing error
// message stage pre-hooks surface when the target directory is
// not a git repo and no auto-init authorization was granted.
//
// stage names which pipeline phase refused to dispatch ("plan"
// or "apply"). The list of authorization options is stage-aware:
// in PLAN stage there is no /approve y/N prompt to answer, so
// listing "interactive: answer y" is a dead-lock advice (the user
// can never reach an /approve prompt because plan generation is
// what is currently failing). The apply stage DOES go through
// /approve and CAN take y/N, so the third option is included only
// there.
//
// Pre-2026-04-30 this function piped its output through
// env.RenderRecommendations which produced two enumerated
// "推荐 1) … 推荐 2) …" entries that duplicated the suffix's
// three-option list AND surfaced internal diag signals like
// "信号: plan_pre_hook detected via worktree.DetectRepoState".
// Customer reported the result as 繁琐 / contradictory. The
// helper now writes one clean direct message — no env_recommend
// double-rendering, no internal signal leak.
func bareDirAuthorizationMessage(ctx *types.BusContext, state worktree.RepoState, stage string) string {
	_ = state // RepoState distinction (uninit vs. no-commits) is purely
	// internal — the user-facing fix is the same in both cases.
	if stage == "" {
		stage = "apply"
	}
	zh := ctx.Language == "" || strings.HasPrefix(strings.ToLower(ctx.Language), "zh")
	repo := ctx.MainRepoRoot
	if repo == "" {
		repo = "(target dir)"
	}

	// Stage-aware option list. The y/N interactive option is only
	// available in apply stage; advertising it during plan stage
	// would be a dead-lock (user is blocked at plan, can never
	// reach /approve to see the y/N prompt).
	if zh {
		var b strings.Builder
		fmt.Fprintf(&b, "目录 %s 不是 git 仓库,需要先初始化才能进入写模式。请任选一种授权方式:\n\n", repo)
		b.WriteString("  • 配置文件 (永久生效):在 codrax.yaml 设 write_auto_init_repo: true\n")
		b.WriteString("  • 命令行 (本次生效):启动加 --auto-init-repo\n")
		if stage == "apply" {
			b.WriteString("  • 交互 (本次生效):再次 /approve 时,看到 y/N 提示回 y\n")
		}
		b.WriteString("\n授权后再次运行同样的请求即可。")
		return b.String()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Directory %s is not a git repo. Write mode needs an initialized repo. Pick ONE authorization route:\n\n", repo)
	b.WriteString("  • config file (permanent): set write_auto_init_repo: true in codrax.yaml\n")
	b.WriteString("  • command line (this run): pass --auto-init-repo at startup\n")
	if stage == "apply" {
		b.WriteString("  • interactive (this run): answer y when /approve shows the y/N prompt\n")
	}
	b.WriteString("\nThen re-issue the same request.")
	return b.String()
}
