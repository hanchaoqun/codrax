package render

import (
	"fmt"
	"strings"
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
	activityNone               activityKind = iota
	activityWaitingPipeline                 // StartSpinner ~ EventObjectiveStarted
	activityWaitingDispatch                 // EventObjectiveStarted ~ first stage; stage gaps
	activityWaitingNode                     // EventTaskNodeStart ~ first EventAgentThinking
	activityRequesting                      // EventAgentThinking
	activityReceiving                       // EventAgentContent (with streamTail)
	activityCallingTool                     // EventToolCallStart ~ EventToolCallEnd (with toolName)
	activityFinalizing                      // EventLivePreviewChunk (with streamTail)
	activityRetrying                        // EventAdapterRetry (attempt+delay)
	activitySwitchingProvider               // EventAdapterFallback
	activityPreparingWorktree               // applyPreHook (write mode)
	activityCapturingBaseline               // captureBaseline (write mode)
	activityAcceptanceReview                // multi-phase acceptance check (commit 44)
	activityErrorRecoverable                // EventTaskNodeEnd with recoverable error, before requeue
	activityErrorFatal                      // terminal error before StopSpinner
	activityCancelled                       // Ctrl+C path before StopSpinner
)

// activityState carries everything row 1 needs to render. Kept narrow
// so the renderer's mu-protected state slot stays small.
type activityState struct {
	kind activityKind
	// detail carries either the streaming tail (receiving / finalizing)
	// or the active tool name (callingTool) or retry attempt info.
	// Empty for kinds that have no per-instance detail.
	detail string
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
		return "preparing pipeline"
	case activityWaitingDispatch:
		if zh {
			return "等待派发"
		}
		return "awaiting dispatch"
	case activityWaitingNode:
		if zh {
			return "等待开始"
		}
		return "starting"
	case activityRequesting:
		if zh {
			return "请求模型中"
		}
		return "requesting model"
	case activityReceiving:
		if zh {
			return "接收中"
		}
		return "receiving"
	case activityCallingTool:
		if zh {
			return "调用工具中"
		}
		return "calling tool"
	case activityFinalizing:
		if zh {
			return "撰写最终答案"
		}
		return "writing answer"
	case activityRetrying:
		if zh {
			return fmt.Sprintf("重试中（第 %d 次，等 %ds）", s.retryAttempt, s.retryDelaySec)
		}
		return fmt.Sprintf("retrying (#%d, in %ds)", s.retryAttempt, s.retryDelaySec)
	case activitySwitchingProvider:
		if zh {
			return "切换 provider 中"
		}
		return "switching provider"
	case activityPreparingWorktree:
		if zh {
			return "准备 worktree 中"
		}
		return "preparing worktree"
	case activityCapturingBaseline:
		if zh {
			return "抓取基准"
		}
		return "capturing baseline"
	case activityAcceptanceReview:
		if zh {
			return "验收审查中"
		}
		return "acceptance review"
	case activityErrorRecoverable:
		if zh {
			return "错误恢复中"
		}
		return "recovering"
	case activityErrorFatal:
		if zh {
			return "已失败"
		}
		return "failed"
	case activityCancelled:
		if zh {
			return "已取消"
		}
		return "cancelled"
	}
	return ""
}

// activityHasStreamTail reports whether this kind exposes a `▸ tail`
// segment after its status word. callingTool shows the tool name;
// receiving / finalizing show the streaming tail. Everything else
// renders as a bare status word.
func activityHasStreamTail(kind activityKind) bool {
	switch kind {
	case activityReceiving, activityFinalizing, activityCallingTool:
		return true
	}
	return false
}

// activityGlyphKind picks which glyph cluster the row-1 prefix uses.
// Most kinds animate via spinnerFrames; retry / fallback freeze on ⟳;
// fatal / cancelled freeze on ✗. Caller still owns the actual glyph
// string + style — this is just the policy gate.
type activityGlyphKind int

const (
	activityGlyphSpinner     activityGlyphKind = iota // animated braille frame
	activityGlyphRecoverable                          // ⟳ yellow frozen
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
// 6-stage / pre-stage / write-stage labels stay in one source.
func stagePhraseDoneFor(stageKey, lang string) string {
	return stagePhrase(stageKey, lang, stagePhraseDone)
}

// commitRowKind enumerates the shape of a permanent scrollback line
// the dock writes via commitToScrollback. Each shape has a glyph +
// fixed style and a free-form text body. The shape catalog is the
// dock's only output to scrollback — anything that wants to be
// remembered after the run goes through one of these.
type commitRowKind int

const (
	commitRowSuccess commitRowKind = iota // ✓ green
	commitRowFailure                      // ✗ red
	commitRowCancelled                    // ✗ red, "已取消"
	commitRowRetry                        // ⟳ yellow
	commitRowReasoning                    // 💭 dark gray
	commitRowQuestion                     // ❯ cyan
	commitRowFinal                        // ◆ green muted (run summary)
	commitRowFinalLight                   // ◇ green muted (light-route summary — local / chat / clarify)
	commitRowNotice                       // ✗ yellow (e.g. "草稿被丢弃")
	commitRowSubTopicHeader               // (no glyph; the sub-topic enumeration block is multiline)
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
//   ✓ — statusSuccessMuted (green)
//   ✗ — statusFatal (red) for failure / cancelled / notice
//   ⟳ — statusRecoverable (yellow)
//   💭 — statusMeta (dark gray)
//   ❯ — statusObjective (light cyan)
//   ◆ — statusSuccessMuted (green)
//
// Body styling is up to the caller — formatCommitRow does NOT
// re-style the body so the caller can mix segment styles freely.
func formatCommitRow(row commitRow) string {
	var b strings.Builder
	b.WriteString("  ")
	switch row.kind {
	case commitRowSuccess:
		b.WriteString(statusSuccessMuted.Sprint(string(glyphSuccess)))
	case commitRowFailure, commitRowCancelled, commitRowNotice:
		b.WriteString(statusFatal.Sprint(string(glyphFatal)))
	case commitRowRetry:
		b.WriteString(statusRecoverable.Sprint(string(glyphRecoverable)))
	case commitRowReasoning:
		b.WriteString(statusMeta.Sprint("💭"))
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
