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
	// would mislead — an interactive REPL asks its own consent
	// BEFORE dispatching a write-mode Run (repl preRunBareDirConsent),
	// so a user who actually sees this plan-stage text is in a
	// scripted/one-shot context where no prompt can ever appear.
	if zh {
		var b strings.Builder
		fmt.Fprintf(&b, "目录 %s 还不是 git 仓库。改代码前需要先把它变成 git 仓库,请任选一种授权方式:\n\n", repo)
		b.WriteString("  • 永久授权:在配置文件里设 write_auto_init_repo: true\n")
		b.WriteString("  • 仅本次:启动时加 --auto-init-repo\n")
		if stage == "apply" {
			b.WriteString("  • 仅本次 (交互):再次 /approve 时,看到 y/N 提示回 y\n")
		}
		b.WriteString("\n说明:这一步只是把目录变成 git 仓库。如果目录是空的,要从零创建新项目还需要再加一项授权 (write_scaffold_enabled 或 --allow-scaffold)。")
		b.WriteString("\n授权后请重新发送同样的请求。")
		return b.String()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Directory %s is not yet a git repo. Changing code requires turning it into one first. Pick ONE authorization:\n\n", repo)
	b.WriteString("  • permanent: set write_auto_init_repo: true in your config file\n")
	b.WriteString("  • this run only: pass --auto-init-repo at startup\n")
	if stage == "apply" {
		b.WriteString("  • this run only (interactive): answer y at the next /approve y/N prompt\n")
	}
	b.WriteString("\nNote: this only authorizes turning the directory into a git repo. If the directory is empty and you want a brand-new project created from scratch, an additional authorization is also required (write_scaffold_enabled or --allow-scaffold).")
	b.WriteString("\nThen re-send the same request.")
	return b.String()
}

// scaffoldAuthorizationMessage is the user-facing error returned by
// planPreHook when init authorization (--auto-init-repo) is in place
// but the target dir is structurally empty AND scaffold authorization
// is missing.
//
// Why this is its own gate (not folded into bareDirAuthorizationMessage):
// "you may run git init in my dir" and "you may invent files for my
// empty dir" are two distinct user permissions. Pre-2026-04-30 they
// were collapsed under a single --auto-init-repo flag, which let
// users authorize git init thinking it was harmless and then watched
// codrax stall trying to assemble a multi-file project plan from
// nothing. Splitting the gates lets each refusal carry its own
// remediation menu, and makes the implicit conflation explicit.
func scaffoldAuthorizationMessage(ctx *types.BusContext) string {
	if ctx == nil {
		return "scaffold authorization required"
	}
	zh := ctx.Language == "" || strings.HasPrefix(strings.ToLower(ctx.Language), "zh")
	repo := ctx.MainRepoRoot
	if repo == "" {
		repo = "(target dir)"
	}
	if zh {
		var b strings.Builder
		fmt.Fprintf(&b, "目录 %s 是空的。从零创建新项目需要单独授权才能允许凭空生成文件,请任选一种授权方式:\n\n", repo)
		b.WriteString("  • 永久授权:在配置文件里设 write_scaffold_enabled: true\n")
		b.WriteString("  • 仅本次:启动时加 --allow-scaffold\n")
		b.WriteString("\n这一项默认关闭,是为了避免在你没明确同意时擅自创建文件。如果你只是想把已有代码所在的目录变成 git 仓库,把源文件放进来再试;或者改用 /mode auto 先看现有内容。")
		b.WriteString("\n授权后请重新发送同样的请求。")
		return b.String()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Directory %s is empty. Creating a new project from scratch requires a separate authorization to allow files to be generated from nothing. Pick ONE:\n\n", repo)
	b.WriteString("  • permanent: set write_scaffold_enabled: true in your config file\n")
	b.WriteString("  • this run only: pass --allow-scaffold at startup\n")
	b.WriteString("\nThis is off by default so files are never fabricated without explicit consent. If you only meant to turn a directory with existing code into a git repo, drop your source files in first and retry; or switch to /mode auto to inspect what is already there.")
	b.WriteString("\nThen re-send the same request.")
	return b.String()
}
