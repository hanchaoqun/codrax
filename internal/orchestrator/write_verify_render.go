package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// write_verify_render.go holds the write-mode apply/verify Result
// wording — the user-facing landing guidance and verification verdict
// sentences. Split from orchestrator.go (IR delivery hot-file ratchet)
// so the wording single-point lives in one concern-specific file
// (WRITEFIX-1, eval-audit 20260719 GAP-1/GAP-2/G3).

// renderApplySummary formats the apply-stage Result message. Used
// by the REPL / single-shot renderer to show the user what landed
// in the worktree. Intentionally markdown-adjacent so the content
// survives glamour rendering without structural mangling.
//
// Bilingual: lang follows the BusContext.Language convention
// (zh-default, en-only flips to English). Pre-fix this rendered in
// English regardless of --lang; the customer's lang=zh terminal
// showed mixed Chinese / English depending on the rendering layer
// (REPL chrome was zh, orchestrator-injected sections were en).
// sessionAppliedRefs is the in-order list of recovery refs the whole
// session pinned so far (including this plan's). A multi-plan session
// must list every ref: cherry-picking only the newest ref lands only
// the newest plan's diff (eval-audit 20260719 G3).
func renderApplySummary(plan *types.ChangePlan, applied map[string]bool, worktreePath, recoveryRef string, willPreserve bool, lang string, sessionAppliedRefs []string) string {
	zh := isLangZh(lang)
	if plan == nil {
		if zh {
			return "apply 阶段:没有可用的 ChangePlan"
		}
		return "apply phase: no ChangePlan available"
	}
	deliveryBroken := plan.ApplyCheckpoint.DeliveryBroken()
	checkpointErr := ""
	if deliveryBroken {
		checkpointErr = strings.TrimSpace(plan.ApplyCheckpoint.CommitError)
		if checkpointErr == "" {
			checkpointErr = strings.TrimSpace(plan.ApplyCheckpoint.TagError)
		}
	}
	multiRefLine := renderSessionAppliedRefsLine(sessionAppliedRefs, zh)
	var b strings.Builder
	if zh {
		fmt.Fprintf(&b, "## Apply 结果:%s\n\n", plan.ID)
		fmt.Fprintf(&b, "已在 worktree `%s` 中 apply **%d** 个变更。\n\n", worktreePath, len(applied))
		if len(plan.Changes) > 0 {
			b.WriteString("**变更列表**:\n\n")
			for i, c := range plan.Changes {
				status := "✓"
				if !applied[c.Path] {
					status = "✗"
				}
				fmt.Fprintf(&b, "%d. %s **%s** (`%s`)\n", i+1, status, c.Kind, c.Path)
			}
			b.WriteString("\n")
		}
		if deliveryBroken {
			// Typed disclosure: the durable ref is NOT usable — never
			// hand out cherry-pick guidance pointing at it. The
			// checkpoint is rebuilt by REPLAYING the apply (`/approve
			// --retry`), never by /verify (rework P2-2 ①). And the
			// worktree channel may only be promised when the worktree
			// actually survives this Run (rework P2-2 ②).
			fmt.Fprintf(&b, "⚠ checkpoint 提交失败,recovery ref `%s` **不可用**:%s\n\n", recoveryRef, checkpointErr)
			if willPreserve {
				fmt.Fprintf(&b, "本次已 apply 的字节目前只存在于 worktree `%s`(已保留);落地走 `/merge`(worktree 通道,不依赖该 ref),或先修复上述 git 错误再 `/approve --retry` 重放 apply 重建 checkpoint。\n",
					worktreePath)
			} else {
				b.WriteString("本次已 apply 的字节只存在于 worktree,而 worktree 会在进程退出时销毁——之后字节不可恢复。请先修复上述 git 错误,再用 `/approve --retry` 重放本 plan。\n")
			}
			return b.String()
		}
		if willPreserve {
			fmt.Fprintf(&b, "worktree 已保留。落地:在提示符里输入 `/merge`,或直接粘贴 `!git cherry-pick %s` 回车执行(`!` 前缀执行系统命令)。\n",
				recoveryRef)
		} else {
			fmt.Fprintf(&b, "worktree 进程退出时销毁,但 apply commit 已 pin 到主仓 ref `%s`,bytes 可恢复:\n\n"+
				"- 在提示符里: `/merge`(主仓有未提交跟踪改动时先 commit 或 stash;未跟踪文件不影响)\n"+
				"- 复制粘贴执行: `!git cherry-pick %s`(`!` 前缀执行系统命令)\n\n"+
				"想保留 worktree 直接审阅,在配置文件里设 `pipeline_keep_worktree_on_success: true`。\n",
				recoveryRef, recoveryRef)
		}
		if multiRefLine != "" {
			b.WriteString(multiRefLine)
		}
		return b.String()
	}
	fmt.Fprintf(&b, "## Apply result: %s\n\n", plan.ID)
	fmt.Fprintf(&b, "Applied **%d** change(s) into worktree `%s`.\n\n", len(applied), worktreePath)
	if len(plan.Changes) > 0 {
		b.WriteString("**Changes**:\n\n")
		for i, c := range plan.Changes {
			status := "✓"
			if !applied[c.Path] {
				status = "✗"
			}
			fmt.Fprintf(&b, "%d. %s **%s** (`%s`)\n", i+1, status, c.Kind, c.Path)
		}
		b.WriteString("\n")
	}
	if deliveryBroken {
		// Same contract as the zh arm: checkpoint rebuild = replay via
		// `/approve --retry` (not /verify), and the worktree channel is
		// only real when the worktree survives this Run.
		fmt.Fprintf(&b, "⚠ The checkpoint commit failed, so recovery ref `%s` is **NOT usable**: %s\n\n", recoveryRef, checkpointErr)
		if willPreserve {
			fmt.Fprintf(&b, "The applied bytes currently exist only in worktree `%s` (preserved); land them via `/merge` (worktree surface, independent of the ref), or fix the git error above and replay the apply with `/approve --retry` to rebuild the checkpoint.\n",
				worktreePath)
		} else {
			b.WriteString("The applied bytes exist only in the worktree, which is discarded on process exit — they are NOT recoverable afterwards. Fix the git error above, then replay this plan with `/approve --retry`.\n")
		}
		return b.String()
	}
	if willPreserve {
		fmt.Fprintf(&b, "Worktree preserved. Land via `/merge` at the prompt, or paste `!git cherry-pick %s` and press enter — the `!` prefix runs system commands inline.\n",
			recoveryRef)
	} else {
		fmt.Fprintf(&b, "The worktree dir is discarded on Run exit, but the apply commit is pinned in the main repo at ref `%s` so the bytes are recoverable:\n\n"+
			"- at the prompt: `/merge` (commit or stash any modified tracked files in main first; untracked files do not block)\n"+
			"- copy-paste: `!git cherry-pick %s` (`!` prefix runs the command inline)\n\n"+
			"To keep the worktree for direct inspection, set `pipeline_keep_worktree_on_success: true` in your config file.\n",
			recoveryRef, recoveryRef)
	}
	if multiRefLine != "" {
		b.WriteString(multiRefLine)
	}
	return b.String()
}

// renderSessionAppliedRefsLine renders the complete-landing guidance for
// a session that pinned multiple applied plans. Checkpoint commits chain
// inside one worktree, so cherry-picking the refs IN ORDER reproduces
// the full delivery; a single-ref cherry-pick lands only that plan's
// diff. Empty for zero/one ref.
func renderSessionAppliedRefsLine(refs []string, zh bool) string {
	if len(refs) < 2 {
		return ""
	}
	quoted := make([]string, 0, len(refs))
	for _, ref := range refs {
		quoted = append(quoted, "`"+ref+"`")
	}
	if zh {
		return fmt.Sprintf("\n本会话共落地 **%d** 个 plan。完整取回全部改动需按顺序 cherry-pick:%s(`/merge` 使用保留的 worktree 时已包含全部)。\n",
			len(refs), strings.Join(quoted, " → "))
	}
	return fmt.Sprintf("\nThis session landed **%d** plans. To recover the FULL change set, cherry-pick the refs in order: %s (a `/merge` from the preserved worktree already contains everything).\n",
		len(refs), strings.Join(quoted, " → "))
}

// renderVerifySuccess renders a short "all good" note appended to
// the apply summary. Kept compact so it pairs visually with the
// apply-phase output already in Mutable.Result. Bilingual.
//
// Header wording mirrors renderVerifyFailure's "测试未通过" /
// "Tests did not pass" so the success and failure surfaces use the
// SAME vocabulary axis ("测试通过" ↔ "测试未通过"); previously this
// function used "Verify PASSED" / "Verify 通过" which leaked the
// internal stage name.
func renderVerifySuccess(report *types.ChangeReport, lang string) string {
	zh := isLangZh(lang)
	if report == nil {
		if zh {
			return "\n## 测试通过\n\n本次未产出测试报告,验证阶段已完成但没有可显示的细节。\n"
		}
		return "\n## Tests verified\n\nNo report produced; the verify step completed but emitted no details.\n"
	}
	total := len(report.TestResults)
	if zh {
		return fmt.Sprintf("\n## 测试通过\n\n%d 个测试通过。报告已存到 .codrax/plans/%s.report.json。\n",
			total, report.PlanID)
	}
	return fmt.Sprintf("\n## Tests verified\n\n%d test(s) passed. Report saved to .codrax/plans/%s.report.json.\n",
		total, report.PlanID)
}

// reportUntriedRunnableCandidate returns the highest-ranked typed
// test-surface candidate whose real verification never actually ran,
// or nil. Two precise shapes count as "untried":
//
//  1. No command executed at the candidate's runner@working_dir at all.
//  2. runner=make with a detected MakeTarget, and no executed make
//     command at that working_dir ran THAT target — the eval-audit
//     20260719 GAP-1/G3 shape: `make make` ran (a fabricated target),
//     `make check` (the surface's own candidate) was never tried, yet
//     the verdict wording claimed the environment lacked the runner.
//
// The returned candidate feeds a STRONG user-facing claim — "the
// verification environment is NOT missing" — so the Outcome rows at
// the candidate's key are classified into three typed lanes
// (rework P1-1, eval-audit 20260719 review):
//
//   - ran: the command genuinely executed (possibly with an unusable
//     result). These count as having tried the candidate.
//   - environment refusal (runner_missing / not_configured): the
//     runner itself is absent or unconfigured at that key. The
//     candidate is DISQUALIFIED from the untried claim — pre-fix these
//     rows were silently dropped, the candidate read as "untried", and
//     the wording asserted the environment was fine while the runner
//     binary was genuinely missing (the reverse lie).
//   - anything else (synthetic rows, skips, and any future Outcome
//     value): did not try the candidate. Only synthetic_no_tests is
//     known to imply nothing about the environment and keeps the
//     candidate untried-eligible; every OTHER value conservatively
//     disqualifies the candidate — an unknown outcome must never be
//     the basis for asserting the environment is intact.
//
// Every input is typed (surface candidates, executed command rows) —
// safe for the hard wording precondition below.
func reportUntriedRunnableCandidate(report *types.ChangeReport) *types.TestSurfaceCandidate {
	if report == nil || report.TestSurface == nil {
		return nil
	}
	ranByKey := map[string][]types.ExecutedCommand{}
	disqualifiedByKey := map[string]bool{}
	for _, cmd := range report.ExecutedCommands {
		wd := strings.TrimSpace(cmd.WorkingDir)
		if wd == "" {
			wd = "."
		}
		key := cmd.Runner + "@" + wd
		switch cmd.Outcome {
		case "", "executed", "parser_error", "timeout", "oom", "cpu_limit", "zero_tests":
			ranByKey[key] = append(ranByKey[key], cmd)
		case "synthetic_no_tests":
			// Explicit non-execution that says nothing about the
			// environment; the candidate stays untried-eligible.
		case "runner_missing", "not_configured":
			// Typed environment refusal: the "environment is not
			// missing" claim would be a lie for this key.
			disqualifiedByKey[key] = true
		default:
			// Unknown / other typed outcomes (suite_skipped,
			// probe_config_error, future values...): conservative —
			// not a try, and not a licence to claim the environment
			// is intact either.
			disqualifiedByKey[key] = true
		}
	}
	for i := range report.TestSurface.Candidates {
		cand := &report.TestSurface.Candidates[i]
		if !cand.HasTestSignal {
			continue
		}
		wd := strings.TrimSpace(cand.WorkingDir)
		if wd == "" {
			wd = "."
		}
		if disqualifiedByKey[cand.Runner+"@"+wd] {
			continue
		}
		ran := ranByKey[cand.Runner+"@"+wd]
		if len(ran) == 0 {
			return cand
		}
		if cand.Runner == "make" && strings.TrimSpace(cand.MakeTarget) != "" {
			targetTried := false
			for _, cmd := range ran {
				if executedMakeCommandTarget(cmd.Command) == strings.TrimSpace(cand.MakeTarget) {
					targetTried = true
					break
				}
			}
			if !targetTried {
				return cand
			}
		}
	}
	return nil
}

// executedMakeCommandTarget extracts the target from an executed
// `make <target>` command line; empty for non-make commands.
func executedMakeCommandTarget(cmdStr string) string {
	fields := strings.Fields(strings.TrimSpace(cmdStr))
	if len(fields) < 2 || fields[0] != "make" {
		return ""
	}
	for _, f := range fields[1:] {
		if strings.HasPrefix(f, "-") {
			continue
		}
		return f
	}
	return ""
}

// renderVerifyUnverified is the "change applied, but local verification did
// not produce a validating assertion" message. The plan's bytes are on disk in
// the worktree, but no local test verdict proved they work. Deliberately worded
// so the operator immediately understands this is NOT a code failure and NOT a
// verified success — it's an explicit middle state.
//
// Wording precondition (eval-audit 20260719 G3/GAP-5): the "environment
// is missing the runner/dependencies" sentence is only allowed when NO
// runnable typed-surface candidate remains untried. When a candidate
// exists, the honest wording is "the verification command failed:
// <real error>" — the zod sessions told the user their environment
// lacked a test runner while `make` and the `check` target both
// existed and had simply never been tried with the right target.
func renderVerifyUnverified(report *types.ChangeReport, lang string) string {
	zh := isLangZh(lang)
	planID := ""
	reasonZH := "本地验证器没有产出结构化验证报告"
	reasonEN := "the local verifier did not produce a typed verification report"
	if report != nil {
		planID = report.PlanID
		if reportIndicatesVerificationUnavailable(report) {
			summary := strings.TrimSpace(report.FailureSummary)
			if cand := reportUntriedRunnableCandidate(report); cand != nil {
				reasonZH = "验证命令失败"
				reasonEN = "the verification command failed"
				if summary != "" {
					reasonZH += ":" + summary
					reasonEN += ": " + summary
				}
				reasonZH += fmt.Sprintf("(本地测试面仍有未尝试的候选 %s,验证环境并未缺失)", cand.ID)
				reasonEN += fmt.Sprintf(" (typed test surface still has the untried candidate %s; the environment itself is not missing)", cand.ID)
			} else {
				reasonZH = "本地验证环境缺少测试运行器或依赖"
				reasonEN = "the local verification environment is missing the test runner or dependencies"
				if summary != "" {
					reasonZH += ": " + summary
					reasonEN += ": " + summary
				}
			}
		} else if runners := strings.Join(report.NoTestsRunners, ", "); runners != "" {
			reasonZH = fmt.Sprintf("测试运行器 (%s) 没有发现任何测试", runners)
			reasonEN = fmt.Sprintf("the test runner (%s) discovered zero tests for the changed code", runners)
		}
	}
	// The "install the environment" suggestion is only honest when the
	// environment is actually what failed; for the untried-candidate
	// shape the right next step is re-running verification.
	envStepZH := "- 补齐本地验证环境后 /verify,或\n"
	envStepEN := "- install the local verification environment and /verify, or\n"
	if reportUntriedRunnableCandidate(report) != nil {
		envStepZH = "- 直接 /verify 重试(按 typed 测试面候选执行),或\n"
		envStepEN = "- /verify again (it runs the typed test-surface candidate), or\n"
	}
	passedChecks, failedChecks := reportVerificationResultCounts(report)
	if zh {
		if passedChecks > 0 {
			return fmt.Sprintf("\n## 未完全验证 (unverified)\n\n"+
				"代码改动已落到 worktree；已有 %d 项本地检查通过、%d 项失败，但%s。已通过检查作为局部证据保留，不能替代尚不可用的必要验证，因此本次改动仍不能标记为“已验证”。\n\n"+
				"建议:\n"+
				"%s"+
				"- /merge 接受当前局部证据与未验证风险,或\n"+
				"- /reject 退回到 plan。\n\n"+
				"报告已存到 .codrax/plans/%s.report.json。\n",
				passedChecks, failedChecks, reasonZH, envStepZH, planID)
		}
		return fmt.Sprintf("\n## 未验证 (unverified)\n\n"+
			"代码改动已落到 worktree,但%s,所以没有断言验证过这次改动。\n\n"+
			"建议:\n"+
			"- 添加测试覆盖再 /verify,或\n"+
			"%s"+
			"- 直接 /merge 接受这次未验证改动,或\n"+
			"- /reject 退回到 plan。\n\n"+
			"报告已存到 .codrax/plans/%s.report.json。\n",
			reasonZH, envStepZH, planID)
	}
	if passedChecks > 0 {
		return fmt.Sprintf("\n## Partially verified (unverified)\n\n"+
			"The change applied to the worktree; %d local check(s) passed and %d failed, but %s. The passing checks remain useful partial evidence, but they do not replace the required verification that was unavailable, so this change is still not verified.\n\n"+
			"Next steps:\n"+
			"%s"+
			"- /merge while accepting the partial evidence and unverified risk, or\n"+
			"- /reject to roll back.\n\n"+
			"Report saved to .codrax/plans/%s.report.json.\n",
			passedChecks, failedChecks, reasonEN, envStepEN, planID)
	}
	return fmt.Sprintf("\n## Unverified\n\n"+
		"The change applied to the worktree, but %s, so no assertion verified the change.\n\n"+
		"Next steps:\n"+
		"- add test coverage and /verify, or\n"+
		"%s"+
		"- /merge to accept the unverified change as-is, or\n"+
		"- /reject to roll back.\n\n"+
		"Report saved to .codrax/plans/%s.report.json.\n",
		reasonEN, envStepEN, planID)
}

func reportVerificationResultCounts(report *types.ChangeReport) (passed, failed int) {
	if report == nil {
		return 0, 0
	}
	for _, result := range report.TestResults {
		if result.Passed {
			passed++
		} else {
			failed++
		}
	}
	return passed, failed
}
