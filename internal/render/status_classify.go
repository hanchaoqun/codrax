package render

import (
	"strings"
)

// statusErrorKind classifies a taskRow's error/detail surface so the
// renderer can pick the right glyph + style + phrasing. The same
// raw error message can carry different semantics depending on
// whether the orchestrator is mid-retry or has given up — but at
// the renderer layer we only see the row state, so we classify by
// (a) row terminal state, (b) keywords in errorMsg, (c) keywords in
// detail, in that priority.
type statusErrorKind string

const (
	statusErrorNone        statusErrorKind = ""
	statusErrorRecoverable statusErrorKind = "recoverable"
	statusErrorFatal       statusErrorKind = "fatal"
	statusErrorCancelled   statusErrorKind = "cancelled"
)

// recoverableMarkers / fatalMarkers / cancelledMarkers are the
// substring rules. Picked from the error vocabulary observed in
// orchestrator + agent + tool packages; case-insensitive match.
//
// Recoverable: orchestrator is retrying, falling back, force-finalising,
// or filling missing evidence. The user sees progress, not failure.
//
// Fatal: stage cannot continue, no fallback wired, the run is
// effectively ended at this row.
//
// Cancelled: user-initiated. context.Canceled propagates from
// Ctrl+C / /cancel — distinct from "watchdog cancelled stalled
// stream" which is a recoverable transport blip.
var (
	recoverableMarkers = []string{
		"retrying",
		"retry",
		"fallback",
		"falling back",
		"补充",
		"修正",
		"重试",
		"降级",
		"force finalize",
		"force-finalize",
		"forced-read",
		"cgec e",                       // CGEC E1/E2/E3 violations are corrected, not fatal
		"evidence insufficient",
		"emit_answer_document rejected",
		"quote length exceeds cap",
		"citation validation rejected",
		"citation_quote_max",
		"correcting",
		"correction",
		"transient",
		"stream stalled",               // watchdog kill triggers a retry
		"stream first byte",
		"alternate path",
		"using syntax check instead",
		"finalizing with available results",
		"stream stalled (no bytes",
	}
	fatalMarkers = []string{
		"cannot continue",
		"无法继续",
		"permission denied",
		"权限不足",
		"invalid configuration",
		"invalid config",
		"配置无效",
		"required dependency",
		"缺少必要依赖",
		"apply failed",
		"apply 失败",
		"verification failed",
		"verify failed",
		"验证失败",
		"worktree provisioning failed",
		"plan stage: target",            // bare-dir refusal
		"plan stage 拒绝",
		"apply stage 拒绝",
		"plan_pre_hook",
		"apply_pre_hook",
		"verify_pre_hook",
		"runner missing",
		"runner_missing",
		"no test runner",
		"no deployments available",
		"insufficient permissions",
		"target dependency is missing",
		// Force-finalize terminal failures. By the time we see
		// these the orchestrator already tried 3 attempts —
		// further retries are not happening, so the error reads
		// as fatal even though the underlying transport is
		// transient by nature.
		"forced finalize",
		"failed to generate the final answer",
		"最终答案生成失败",
		"connection to the model dropped",
		"与模型的连接中断",
		"model response timed out",
		"模型响应超时",
		"model service is unavailable or rate-limited",
		"模型服务暂不可用或限流",
	}
	cancelledMarkers = []string{
		"interrupted by user",
		"user cancelled",
		"user canceled",
		"已取消",
		"sigint",
		"ctrl+c",
		"ctrl-c",
		"cancel hit",
		"task cancelled",
		"task canceled",
	}
)

// classifyStatusError inspects a taskRow's error / detail / state
// fields and decides which severity bucket the surface should fall
// into. Returns statusErrorNone when no error semantics are present.
//
// Priority: row-state first (a non-zero endTime + errorMsg means the
// row is terminal), then explicit cancellation markers, then
// recoverable markers (which can fire even while the row is still
// running — "retrying" is a transient-progress signal), then fatal
// markers. The order matters: a string containing both "retry" and
// "verification failed" (e.g. "retry budget exhausted; verification
// failed") needs to land on fatal because the retry is exhausted.
func classifyStatusError(row *taskRow) statusErrorKind {
	if row == nil {
		return statusErrorNone
	}
	candidate := strings.ToLower(strings.TrimSpace(row.errorMsg))
	detail := strings.ToLower(strings.TrimSpace(row.detail))

	// Cancellation: scan errorMsg only — detail might contain
	// "cancelled" as part of an internal status string that is
	// NOT user-driven (e.g. a sub-task got cancelled by a retry
	// supervisor).
	if hasMarker(candidate, cancelledMarkers) {
		return statusErrorCancelled
	}
	// Fatal first when the row has terminated AND no recovery hint
	// is in flight. retry-exhausted reads as fatal; "retrying" alone
	// reads as recoverable.
	if !row.endTime.IsZero() && candidate != "" {
		if hasMarker(candidate, fatalMarkers) && !hasMarker(candidate, []string{"retrying"}) {
			return statusErrorFatal
		}
		if hasMarker(candidate, recoverableMarkers) || hasMarker(detail, recoverableMarkers) {
			return statusErrorRecoverable
		}
		// Generic terminal failure with neither fatal nor
		// recoverable cue: default to recoverable so the user sees
		// "X 出错,正在重试" instead of a screaming ✗ that the
		// orchestrator was about to retry anyway.
		//
		// History: pre-2026-05-06 this branch returned statusErrorFatal,
		// which produced ✗ + "未能 X" on the durable scrollback line
		// for any error message that wasn't on the recoverableMarkers
		// substring whitelist. The orchestrator's actual retry
		// decision (retryReadStageDispatchError, transientRetryBudget,
		// per-stage retry loops) is structured, NOT string-keyed, so
		// the renderer cannot tell from errorMsg alone whether retry
		// will fire. Defaulting to fatal therefore lied whenever the
		// orchestrator's retry path took an error message that did
		// not happen to contain "retry" / "fallback" / etc. The
		// orchestrator emits MarkRunFatal at run end when the run
		// truly cannot continue; that path remains the only source
		// of the terminal ✗ summary. fatalMarkers above still flags
		// errors operators recognise as obviously terminal
		// (verification failed, permission denied, etc.) so the
		// list is not dead — only the unknown-error fallback flips.
		return statusErrorRecoverable
	}
	// Row is still running OR has no errorMsg. Recovery cues in
	// detail surface as recoverable (transient retry / forced-read
	// / fixing citation format).
	if hasMarker(detail, recoverableMarkers) {
		return statusErrorRecoverable
	}
	return statusErrorNone
}

// recoverableDetailPhrase maps a small set of well-known internal
// recoverable signals to a localized user-friendly sentence. The
// caller is expected to have already verified the row is in
// statusErrorRecoverable state. Returns "" when no specific phrasing
// matches; the caller falls through to the row's detail string.
func recoverableDetailPhrase(row *taskRow, lang string) string {
	if row == nil {
		return ""
	}
	zh := isZh(lang)
	probe := strings.ToLower(row.errorMsg + " " + row.detail)
	switch {
	case strings.Contains(probe, "forced-read"),
		strings.Contains(probe, "cgec e2"):
		// Force-read / CGEC E2 means specific files were skipped and
		// the orchestrator is reading them now; phrase emphasizes the
		// "filling in" semantic (we know what's missing).
		if zh {
			return "正在补充遗漏的关键信息"
		}
		return "Filling in missing key information"
	case strings.Contains(probe, "evidence insufficient"),
		strings.Contains(probe, "cgec e1"),
		strings.Contains(probe, "cgec e3"):
		// Generic evidence-insufficient signal — we don't yet know
		// which file is missing, just that the gate failed.
		if zh {
			return "证据还不够完整，正在补充确认"
		}
		return "Evidence is incomplete; collecting more confirmation"
	case strings.Contains(probe, "emit_answer_document rejected"),
		strings.Contains(probe, "citation validation rejected"):
		if zh {
			return "正在修正答案引用格式"
		}
		return "Fixing answer citation format"
	case strings.Contains(probe, "quote length exceeds cap"),
		strings.Contains(probe, "citation_quote_max"):
		if zh {
			return "正在修正引用片段格式"
		}
		return "Fixing citation snippet format"
	case strings.Contains(probe, "no deployments available"),
		strings.Contains(probe, "finalizing with available results"):
		if zh {
			return "模型服务暂不可用，正在尝试基于已有结果收尾"
		}
		return "Model service is temporarily unavailable; finalizing with available results"
	case strings.Contains(probe, "alternate path"),
		strings.Contains(probe, "fallback"),
		strings.Contains(probe, "transient"),
		strings.Contains(probe, "stream stalled"):
		if zh {
			return "工具调用失败，正在尝试替代路径"
		}
		return "Tool call failed; trying an alternate path"
	case strings.Contains(probe, "using syntax check instead"):
		if zh {
			return "测试运行器不可用，正在使用语法检查替代"
		}
		return "Test runner unavailable; using syntax check instead"
	}
	return ""
}

// fatalDetailPhrase maps known fatal signals to a localized line.
// Same contract as recoverableDetailPhrase: returns "" when nothing
// specific fires, caller surfaces a generic "无法继续" / "Cannot
// continue" with the raw error message tail.
func fatalDetailPhrase(row *taskRow, lang string) string {
	if row == nil {
		return ""
	}
	zh := isZh(lang)
	probe := strings.ToLower(row.errorMsg + " " + row.detail)
	switch {
	case strings.Contains(probe, "verification failed"),
		strings.Contains(probe, "verify failed"),
		strings.Contains(probe, "验证失败"),
		strings.Contains(probe, "runner_missing"),
		strings.Contains(probe, "runner missing"),
		strings.Contains(probe, "no test runner"):
		if zh {
			return "验证失败：测试运行器不可用"
		}
		return "Verification failed: test runner is unavailable"
	case strings.Contains(probe, "apply failed"),
		strings.Contains(probe, "apply 失败"),
		strings.Contains(probe, "apply patch failed"):
		if zh {
			return "Apply 失败：补丁无法自动应用"
		}
		return "Apply failed: patch could not be applied automatically"
	case strings.Contains(probe, "permission denied"),
		strings.Contains(probe, "权限不足"):
		if zh {
			return "无法继续：权限不足"
		}
		return "Cannot continue: permission denied"
	case strings.Contains(probe, "invalid configuration"),
		strings.Contains(probe, "invalid config"),
		strings.Contains(probe, "配置无效"):
		if zh {
			return "无法继续：配置无效"
		}
		return "Cannot continue: invalid configuration"
	case strings.Contains(probe, "required dependency"),
		strings.Contains(probe, "缺少必要依赖"):
		if zh {
			return "无法继续：缺少必要依赖"
		}
		return "Cannot continue: required dependency is missing"
	case strings.Contains(probe, "no deployments available"),
		strings.Contains(probe, "model service is unavailable or rate-limited"),
		strings.Contains(probe, "模型服务暂不可用或限流"):
		if zh {
			return "无法继续：模型服务暂不可用或限流"
		}
		return "Cannot continue: model service is unavailable or rate-limited"
	case strings.Contains(probe, "connection to the model dropped"),
		strings.Contains(probe, "与模型的连接中断"),
		strings.Contains(probe, "unexpected eof"),
		strings.Contains(probe, "stream stalled"),
		strings.Contains(probe, "stream first byte"):
		if zh {
			return "无法继续：与模型的连接中断,可能是网络抖动或上游临时不可用"
		}
		return "Cannot continue: connection to the model dropped (transient network or upstream issue)"
	case strings.Contains(probe, "model response timed out"),
		strings.Contains(probe, "模型响应超时"),
		strings.Contains(probe, "deadline exceeded"):
		if zh {
			return "无法继续：模型响应超时"
		}
		return "Cannot continue: model response timed out"
	case strings.Contains(probe, "failed to generate the final answer"),
		strings.Contains(probe, "最终答案生成失败"),
		strings.Contains(probe, "forced finalize"):
		if zh {
			return "无法继续：最终答案生成失败"
		}
		return "Cannot continue: failed to generate the final answer"
	}
	return ""
}

// cancelledDetailPhrase is the localized "user cancelled" line used
// when classifyStatusError returns statusErrorCancelled.
func cancelledDetailPhrase(lang string) string {
	if isZh(lang) {
		return "已取消：用户中断了本次任务"
	}
	return "Cancelled: interrupted by user"
}

// classifyEventError classifies a raw EventStageEnd / EventTaskNodeEnd
// Error string using the same priority order as classifyStatusError
// (cancellation > fatal > recoverable). Used by the dock-event handlers
// (renderer_dock.go) to pick activityErrorFatal / activityCancelled /
// activityErrorRecoverable so dock row 1 reads the same severity as
// the just-committed scrollback line — pre-fix dock row 1 always went
// recoverable for ANY ev.Error while the commit line could go fatal,
// producing a momentary "⟳ + ✗" cross-talk between the two surfaces.
//
// Empty input returns statusErrorNone — caller should not have invoked
// us in that case but the no-op return keeps the call site simple.
func classifyEventError(errMsg string) statusErrorKind {
	candidate := strings.ToLower(strings.TrimSpace(errMsg))
	if candidate == "" {
		return statusErrorNone
	}
	if hasMarker(candidate, cancelledMarkers) {
		return statusErrorCancelled
	}
	if hasMarker(candidate, fatalMarkers) && !hasMarker(candidate, []string{"retrying"}) {
		return statusErrorFatal
	}
	// Default to recoverable. Mirrors classifyStatusError's
	// post-2026-05-06 default — orchestrator's retry decision is
	// structural (stage retry loops, transient budget) and the
	// renderer cannot tell from the message alone whether retry
	// will fire, so the calmer ⟳ shape avoids screaming ✗ on a
	// recoverable mid-flight error.
	return statusErrorRecoverable
}

func hasMarker(text string, markers []string) bool {
	if text == "" {
		return false
	}
	for _, m := range markers {
		if strings.Contains(text, strings.ToLower(m)) {
			return true
		}
	}
	return false
}
