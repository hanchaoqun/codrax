package render

import (
	"fmt"
	"strings"
	"time"
)

// activityKind enumerates the row-1 status word the dock displays.
// Each kind maps to a localized phrase via activityPhrase. Transitions
// are driven by EventEmitter handlers in renderer.go — see the dock
// design doc for the per-event mapping. The state machine is purely
// "the most recent meaningful event wins"; there is no implicit
// timeout or fallback (expired states only change on the next event).
//
// Why a flat enum instead of an interface: the dock is a tight
// rendering loop; switch-on-int is a single CPU op vs an interface
// dispatch. Localized phrases live in one table (activityPhrase),
// adding a new kind is a 1-line enum + 1-line phrase + 1-line emit.
type activityKind int

const (
	activityNone              activityKind = iota
	activityWaitingPipeline                // StartSpinner ~ EventObjectiveStarted
	activityWaitingDispatch                // EventObjectiveStarted ~ first stage; stage gaps
	activityWaitingNode                    // EventTaskNodeStart ~ first EventAgentThinking
	activityRequesting                     // EventAgentThinking
	activityReceiving                      // EventAgentContent (with streamTail)
	activityCallingTool                    // EventToolCallStart ~ EventToolCallEnd (with toolName)
	activityFinalizing                     // EventLivePreviewChunk (with streamTail)
	activityRetrying                       // EventAdapterRetry (attempt+delay)
	activitySwitchingProvider              // EventAdapterFallback
	activityPreparingWorktree              // applyPreHook (write mode)
	activityCapturingBaseline              // captureBaseline (write mode)
	activityAcceptanceReview               // multi-phase acceptance check (commit 44)
	activityRepoMapScanning                // local repomap index parse (with file counts)
	activityLightRoute                     // REPL light route with no LLM/pipeline task rows
	activityErrorRecoverable               // EventTaskNodeEnd with recoverable error, before requeue
	activityErrorFatal                     // terminal error before StopSpinner
	activityCancelled                      // Ctrl+C path before StopSpinner
)

// activityState carries everything row 1 needs to render. Kept narrow
// so the renderer's mu-protected state slot stays small.
type activityState struct {
	kind activityKind
	// detail carries either the streaming tail (receiving / finalizing)
	// or the active tool name (callingTool) or retry attempt info.
	// Empty for kinds that have no per-instance detail.
	detail string
	// deadline/timeoutBudget are populated by light-route operations that
	// run outside the normal pipeline task rows. They let the live dock show
	// a real countdown instead of a static "timeout 2m0s" hint.
	deadline      time.Time
	timeoutBudget time.Duration
	// retryAttempt + retryDelaySec populated only for activityRetrying;
	// rendered as "重试中（第 N 次，等 Xs）".
	retryAttempt  int
	retryDelaySec int
}

// activityPhrase produces the localized row-1 status word for the
// given state. Returns the bare status word; the dock composer
// handles the leading glyph and the trailing `▸ tail` segment.
func activityPhrase(s activityState, lang string) string {
	zh := isZh(lang)
	switch s.kind {
	case activityWaitingPipeline:
		if zh {
			return "准备流水线"
		}
		return "Preparing pipeline"
	case activityWaitingDispatch:
		if zh {
			return "等待派发"
		}
		return "Queued"
	case activityWaitingNode:
		if zh {
			return "整理上下文中"
		}
		return "Preparing context"
	case activityRequesting:
		if zh {
			return "请求模型中"
		}
		return "Calling the model"
	case activityReceiving:
		if zh {
			return "接收中"
		}
		return "Receiving response"
	case activityCallingTool:
		if zh {
			return "调用工具中"
		}
		return "Calling tool"
	case activityFinalizing:
		if zh {
			return "撰写最终答案"
		}
		return "Writing answer"
	case activityRetrying:
		// User-facing language: spell out WHAT is being retried (the
		// LLM model request — operators reading old "重试中（#N）"
		// could not tell whether it was a tool retry, a stage retry,
		// or an LLM-call retry). Drop internal "#N" / "L1" jargon —
		// those are layer-of-the-retry-stack identifiers in the
		// system log, not end-user terms.
		if zh {
			return fmt.Sprintf("正在重新请求模型（%ds 后第 %d 次重试）", s.retryDelaySec, s.retryAttempt)
		}
		return fmt.Sprintf("Retrying model request in %ds (attempt %d)", s.retryDelaySec, s.retryAttempt)
	case activitySwitchingProvider:
		if zh {
			return "正在切换 LLM 服务"
		}
		return "Switching LLM provider"
	case activityPreparingWorktree:
		if zh {
			return "准备 worktree 中"
		}
		return "Preparing worktree"
	case activityCapturingBaseline:
		if zh {
			return "抓取基准"
		}
		return "Capturing baseline"
	case activityAcceptanceReview:
		if zh {
			return "验收审查中"
		}
		return "Acceptance review"
	case activityRepoMapScanning:
		if zh {
			return "扫描仓库索引中"
		}
		return "Scanning repo index"
	case activityLightRoute:
		if strings.TrimSpace(s.detail) != "" {
			return strings.TrimSpace(s.detail)
		}
		if zh {
			return "处理中"
		}
		return "Working"
	case activityErrorRecoverable:
		if zh {
			return "错误恢复中"
		}
		return "Recovering"
	case activityErrorFatal:
		if zh {
			return "已失败"
		}
		return "Failed"
	case activityCancelled:
		if zh {
			return "已取消"
		}
		return "Cancelled"
	}
	return ""
}

func lightRouteActivityWithCountdown(s activityState, now time.Time, lang string) activityState {
	if s.kind != activityLightRoute || s.deadline.IsZero() {
		return s
	}
	remaining := s.deadline.Sub(now)
	if remaining < 0 {
		remaining = 0
	}
	remainingLabel := ceilDurationSecond(remaining).String()
	budgetLabel := ""
	if s.timeoutBudget > 0 {
		budgetLabel = ceilDurationSecond(s.timeoutBudget).String()
	}
	base := strings.TrimSpace(s.detail)
	if base == "" {
		base = activityPhrase(activityState{kind: activityLightRoute}, lang)
	}
	if isZh(lang) {
		if budgetLabel != "" {
			s.detail = fmt.Sprintf("%s（剩余 %s / 超时 %s）", base, remainingLabel, budgetLabel)
		} else {
			s.detail = fmt.Sprintf("%s（剩余 %s）", base, remainingLabel)
		}
		return s
	}
	if budgetLabel != "" {
		s.detail = fmt.Sprintf("%s (remaining %s / timeout %s)", base, remainingLabel, budgetLabel)
	} else {
		s.detail = fmt.Sprintf("%s (remaining %s)", base, remainingLabel)
	}
	return s
}

func ceilDurationSecond(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	const sec = time.Second
	return ((d + sec - time.Nanosecond) / sec) * sec
}

// activityHasStreamTail reports whether this kind exposes a `▸ tail`
// segment after its status word. callingTool shows the tool name;
// receiving / finalizing show the streaming tail. Everything else
// renders as a bare status word.
func activityHasStreamTail(kind activityKind) bool {
	switch kind {
	case activityReceiving, activityFinalizing, activityCallingTool, activityRepoMapScanning:
		return true
	}
	return false
}

// activityGlyphKind picks which glyph cluster the row-1 prefix uses.
// Most kinds animate via spinnerFrames; retry / fallback freeze on ↻;
// fatal / cancelled freeze on ✗. Caller still owns the actual glyph
// string + style — this is just the policy gate.
type activityGlyphKind int

const (
	activityGlyphSpinner     activityGlyphKind = iota // animated braille frame
	activityGlyphRecoverable                          // ↻ yellow frozen
	activityGlyphFatal                                // ✗ red frozen
)

func activityGlyphFor(kind activityKind) activityGlyphKind {
	switch kind {
	case activityRetrying, activitySwitchingProvider, activityErrorRecoverable:
		return activityGlyphRecoverable
	case activityErrorFatal, activityCancelled:
		return activityGlyphFatal
	}
	return activityGlyphSpinner
}

// stagePhraseDoneFor returns the localized "已 X" phrase for the
// commit row when a stage finishes successfully. Currently delegates
// to the existing stagePhrase helper (state=stagePhraseDone) so the
// read / pre-stage / write-stage labels stay in one source.
func stagePhraseDoneFor(stageKey, lang string) string {
	return stagePhrase(stageKey, lang, stagePhraseDone)
}

// stagePhraseSkippedFor returns the localized phrase for a stage that
// completed via a short-circuit path (loaded plan or --skip-verify verify
// bypassed). Returns "" for stage
// keys that don't have a defined skipped phrase, signalling the
// caller to fall through to the regular done phrase.
//
// Only plan and verify can legitimately be skipped in production —
// /approve --plan-file skips plan, /approve --skip-verify skips
// verify. Other stages use the regular done phrase even if a future
// short-circuit fires (the fallthrough is a safety net, not a
// vocabulary expansion).
func stagePhraseSkippedFor(stageKey, lang string) string {
	zh := isZh(lang)
	switch stageKey {
	case "plan":
		if zh {
			return "已加载预审方案"
		}
		return "Pre-reviewed plan loaded"
	case "verify":
		if zh {
			return "已跳过测试验证"
		}
		return "Verify skipped"
	}
	return ""
}

// commitRowKind enumerates the shape of a permanent scrollback line
// the dock writes via commitToScrollback. Each shape has a glyph +
// fixed style and a free-form text body. The shape catalog is the
// dock's only output to scrollback — anything that wants to be
// remembered after the run goes through one of these.
type commitRowKind int

const (
	commitRowSuccess        commitRowKind = iota // ✓ green
	commitRowFailure                             // ✗ red (system failure)
	commitRowCancelled                           // ⊘ gray (user-initiated stop, NOT a failure)
	commitRowRetry                               // ↻ yellow
	commitRowReasoning                           // ⋯ light cyan glyph, muted body
	commitRowQuestion                            // ❯ cyan
	commitRowFinal                               // ◆ green muted (run summary)
	commitRowFinalLight                          // ◇ green muted (light-route summary — local / chat / clarify)
	commitRowNotice                              // ↻ yellow (soft notice, e.g. "草稿被丢弃, 正在重写" — recoverable, not failure)
	commitRowSubTopicHeader                      // (no glyph; the sub-topic enumeration block is multiline)
)

// commitRow is the raw input to the row formatter. The formatter
// produces a single styled string (one '\n' at end) appropriate to
// hand to commitToScrollback for permanent display.
type commitRow struct {
	kind commitRowKind
	body string // pre-styled body; the glyph + leading indent are added by formatCommitRow
}

// formatCommitRow returns the fully-styled scrollback line for the
// given commit row, including 2-space indent + 1-column glyph +
// 1-space + body. NO trailing newline — the caller (commitToScrollback)
// decides where to put '\n' so multiline payloads stay batched.
//
// Style policy (dark mode reference):
//
//	✓ — statusSuccessMuted (green)
//	✗ — statusFatal (red) for system failure
//	⊘ — statusMeta (dark gray) for user-initiated cancellation (not failure)
//	↻ — statusRecoverable (yellow) for retry AND soft notice (recoverable)
//	⋯ — statusReasoningGlyph (light cyan marker only)
//	❯ — statusObjective (light cyan)
//	◆ — statusSuccessMuted (green)
//
// Body styling is up to the caller — formatCommitRow does NOT
// re-style the body so the caller can mix segment styles freely.
func formatCommitRow(row commitRow) string {
	var b strings.Builder
	b.WriteString("  ")
	switch row.kind {
	case commitRowSuccess:
		b.WriteString(statusSuccessMuted.Sprint(string(glyphSuccess)))
	case commitRowFailure:
		b.WriteString(statusFatal.Sprint(string(glyphFatal)))
	case commitRowCancelled:
		// User-initiated stop is NOT a system failure — distinct
		// glyph (⊘) + muted color so the dock doesn't shout "error"
		// when the user pressed Ctrl+C deliberately.
		b.WriteString(statusMeta.Sprint(string(glyphCancelled)))
	case commitRowRetry, commitRowNotice:
		// Both are "system is recovering / continuing" signals — same
		// ↻ + yellow palette. Notice is a one-shot soft message
		// (e.g. draft rewrite); Retry is an adapter-level retry. The
		// shared visual reads as "we are still working on it".
		b.WriteString(statusRecoverable.Sprint(string(glyphRecoverable)))
	case commitRowReasoning:
		b.WriteString(statusReasoningGlyph.Sprint(string(glyphReasoning)))
	case commitRowQuestion:
		b.WriteString(statusObjective.Sprint("❯"))
	case commitRowFinal:
		b.WriteString(statusSuccessMuted.Sprint("◆"))
	case commitRowFinalLight:
		b.WriteString(statusSuccessMuted.Sprint("◇"))
	case commitRowSubTopicHeader:
		// The block has its own header line + per-topic prefixes; the
		// commitRow body is the entire block. No per-line glyph.
		// Caller passes pre-formatted block as body, indent already
		// included.
		return row.body
	}
	b.WriteString(" ")
	b.WriteString(row.body)
	return b.String()
}

// FormatLightRouteSummary returns the styled scrollback line for a
// light-route reply (local / chat / clarify). Shape:
//
//	◇ <label> · <seg1> · <seg2> [· 总耗时 Xs]
//
// Glyph follows the project's "◆ vs ◇" convention — pipeline runs
// emit the filled diamond via commitRowFinal; light routes that
// bypass the full pipeline emit the hollow diamond. Same color
// family (statusSuccessMuted), same separator (statusMeta · ),
// same body color (statusPrimaryDone) so the line reads as a
// peer of "◆ 已结束 · …" rather than a foreign banner.
//
// elapsed may be empty — callers without an active wall-clock
// (clarify, which never starts a spinner) pass "" and the trailer
// segment is skipped. lang is the same locale code consumed by
// totalElapsedPhrase.
func FormatLightRouteSummary(label string, segments []string, elapsed, lang string) string {
	var body strings.Builder
	body.WriteString(label)
	for _, seg := range segments {
		if strings.TrimSpace(seg) == "" {
			continue
		}
		body.WriteString(" ")
		body.WriteString(statusMeta.Sprint("·"))
		body.WriteString(" ")
		body.WriteString(statusMeta.Sprint(seg))
	}
	if elapsed != "" {
		body.WriteString(" ")
		body.WriteString(statusMeta.Sprint("·"))
		body.WriteString(" ")
		body.WriteString(statusMeta.Sprint(totalElapsedPhrase(elapsed, lang)))
	}
	return formatCommitRow(commitRow{
		kind: commitRowFinalLight,
		body: statusPrimaryDone.Sprint(body.String()),
	})
}
