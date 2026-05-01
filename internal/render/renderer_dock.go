package render

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// dockTickInterval is the animation refresh cadence. 100 ms is the
// established CLI sweet spot for braille-spinner perceived smoothness
// without spamming the terminal. Operators can override via the
// renderer's setter — left as a future yaml knob if anyone needs it.
const dockTickInterval = 100 * time.Millisecond

// streamTailDisplayCols caps the row-1 `▸ tail` segment display
// width. ~25 cols gives the user a feel for "bytes flowing" without
// dominating the line. Stage counters and time row stay readable
// even when the tail is at full extent.
const streamTailDisplayCols = 25

// dockEmitter is the renderer's new event handler. It replaces the
// bar-based Emitter. Same signature, same locking discipline, same
// source events. The output layer differs: instead of `bar.Update`
// + scattered `fmt.Println`, every durable scrollback line goes
// through `r.dock.commitToScrollback`, and the live row content
// goes through `r.dock.paintDock(composeDockRows(...))`.
//
// Lifecycle: Renderer.Emitter() returns a closure that locks r.mu
// and dispatches here.

// isWriteTrioNodeKind reports whether a node row's nodeKind is one
// of the write-mode plan/apply/verify trio. Used by the retry-aware
// reclassification in EventTaskNodeStart to scope the "flip prior
// success to failed" pass — read-mode evidence/validate/reconcile/
// extract/finalize nodes have different kinds and must be left
// alone when a write retry fires.
func isWriteTrioNodeKind(kind string) bool {
	switch kind {
	case "plan", "apply", "verify":
		return true
	}
	return false
}

func (r *Renderer) handleEvent(ev Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Output strategy decision happens AFTER state mutation: tests
	// drive the renderer without ever calling StartSpinner and then
	// inspect r.tasks / r.current / r.activity directly, so the
	// state-tracking side of every event handler must run regardless
	// of whether a dock is attached. The dock primitive's nil-safe
	// methods make the output side a silent no-op when no live region
	// exists.
	if r.dock == nil && !r.dockEnabled {
		// Pre-StartSpinner test path AND production non-TTY path
		// share this branch: the non-TTY println happens after state
		// mutation below.
	}

	switch ev.Kind {
	case EventLivePreviewChunk:
		r.dockHandlePreviewChunk(ev)
		return
	case EventLivePreviewClear:
		r.dockHandlePreviewClear(ev)
		return
	case EventAgentReasoning:
		if ev.Reasoning == "" {
			return
		}
		line := formatReasoning(string(ev.Agent), ev.Iteration, ev.Reasoning)
		if !r.dockEnabled && r.dock == nil {
			fmt.Fprintln(r.outputWriter(), line)
			return
		}
		r.commitLineLocked(line)
		return
	case EventPhaseGroupStart:
		// Commit 43: TTY-mode phase block render. Mirror of
		// the EventAnalysisReady sub-topic-block path so the
		// dock area frame stays correct.
		block := formatPhaseGroupBlock(r.lang, ev.PhaseList)
		if block == "" {
			return
		}
		if !r.dockEnabled && r.dock == nil {
			fmt.Fprint(r.outputWriter(), block)
			return
		}
		r.commitMultilineLocked(block)
		return
	case EventPhaseProgress:
		// Commit 43: TTY-mode phase-status row.
		line := formatPhaseProgressLine(r.lang, ev.PhaseIndex, ev.PhaseTotal,
			ev.PhaseProgressKind, ev.PhaseDetail)
		if !r.dockEnabled && r.dock == nil {
			fmt.Fprintln(r.outputWriter(), line)
			return
		}
		r.commitLineLocked(line)
		return
	}

	switch ev.Kind {
	case EventObjectiveStarted:
		if ev.Objective != "" && r.objective == "" {
			r.objective = ev.Objective
			r.objectiveDone = false
			line := formatCommitRow(commitRow{
				kind: commitRowQuestion,
				body: statusObjective.Sprint(r.objective),
			})
			r.commitLineLocked(line)
			r.activity = activityState{kind: activityWaitingDispatch}
		}

	case EventObjectiveDone:
		r.objectiveDone = true

	case EventStageStart:
		// Read-mode post-analysisReady: most stages are owned by
		// the per-node lifecycle (EventTaskNodeStart / End). Two
		// exceptions: analyze still pre-dates the node graph (its
		// row was created before EventAnalysisReady, retry-aware
		// reclassification still applies); extract is a stage-only
		// step with no NodeType counterpart, so its row MUST come
		// from EventStageStart or row 2 sits on a bare "▪ —" for
		// the entire extract dispatch (5-30s).
		if r.analysisReady && ev.Stage != "" &&
			string(ev.Stage) != "analyze" && string(ev.Stage) != "extract" {
			break
		}
		// Retry-aware reclassification: if any prior row for the same
		// stage already has endTime != 0 + okFinished=true (i.e. was
		// rendered as "已 X" / "X done"), the fact that a new
		// EventStageStart is firing for the SAME stage proves the
		// prior attempt did not actually succeed — otherwise the
		// pipeline would not be re-dispatching the same stage. Flip
		// those prior rows to okFinished=false so their phrase
		// resolves to stagePhraseFailed ("理解问题失败" / "Request
		// understanding failed") instead of the lying "已理解问题".
		// Canonical case: runAnalyzePhase's internal retry loop —
		// dispatchStage emits EventStageEnd with Error="" when the
		// agent returned cleanly but AnalysisIR is still nil
		// (analyzer didn't call emit_analysis); runAnalyzePhase then
		// loops and re-dispatches. Without this fix the user reads
		// "已理解问题" alongside the running spinner of the retry
		// attempt — the exact lie the user reported. Read-mode-only
		// concern (analyze is the only stage with above-scheduler
		// retry); write-mode retries reuse node-level events which
		// already follow the no-end-on-requeue contract.
		stageStr := string(ev.Stage)
		for _, prev := range r.tasks {
			if prev == nil || prev.isNodeRow || prev.isSubAgent {
				continue
			}
			if prev.stage != stageStr {
				continue
			}
			if !prev.endTime.IsZero() && prev.okFinished {
				prev.okFinished = false
				if prev.errorMsg == "" {
					prev.errorMsg = "retried"
				}
			}
		}
		row := &taskRow{
			stage:       string(ev.Stage),
			agent:       string(ev.Agent),
			startTime:   ev.Timestamp,
			detailStart: ev.Timestamp,
		}
		r.tasks = append(r.tasks, row)
		r.current = row
		r.activity = activityState{kind: activityWaitingNode}
		r.streamTail = ""
		r.streamChars = 0

	case EventStageEnd:
		if r.analysisReady && ev.Stage != "" &&
			string(ev.Stage) != "analyze" && string(ev.Stage) != "extract" {
			break
		}
		if row := r.findRunningStageRow(string(ev.Stage), string(ev.Agent)); row != nil {
			row.endTime = ev.Timestamp
			row.errorMsg = ev.Error
			row.okFinished = ev.Error == ""
			if row == r.current {
				r.current = nil
				// Errored stage: hold the recoverable shape on row 1
				// so the user sees the failure before the next
				// dispatch overwrites it. Successful stage falls
				// through to the calmer waitingDispatch hue.
				if ev.Error != "" {
					r.activity = activityState{kind: activityErrorRecoverable, detail: ev.Error}
				} else {
					r.activity = activityState{kind: activityWaitingDispatch}
				}
				r.streamTail = ""
			}
			r.commitStageDoneLocked(row, 0)
		}

	case EventAnalysisReady:
		var analyzeDone *taskRow
		for _, row := range r.tasks {
			if row.isSubAgent || row.isNodeRow {
				continue
			}
			if row.stage == "analyze" && row.endTime.IsZero() {
				row.endTime = ev.Timestamp
				row.okFinished = true
				if row == r.current {
					r.current = nil
				}
				analyzeDone = row
			}
		}
		for _, n := range ev.TaskNodes {
			r.tasks = append(r.tasks, &taskRow{
				isNodeRow: true,
				nodeID:    n.ID,
				nodeKind:  n.Type,
				objective: n.Objective,
				pending:   true,
			})
		}
		r.analysisReady = true
		r.activity = activityState{kind: activityWaitingDispatch}
		r.streamTail = ""
		var batch strings.Builder
		if analyzeDone != nil {
			batch.WriteString(r.formatStageDoneLine(analyzeDone, 0))
			batch.WriteString("\n")
		}
		if subBlock := formatSubTopicsBlock(r.lang, ev.TaskNodes); subBlock != "" {
			batch.WriteString(subBlock)
		}
		if batch.Len() > 0 {
			r.commitMultilineLocked(batch.String())
		}

	case EventTaskNodeStart:
		if row := r.findNodeRow(ev.NodeID); row != nil {
			// Retry-aware reclassification for write-mode node rows.
			// When this row is firing for the SECOND+ time (i.e.
			// retry: prior endTime was set by a previous
			// EventTaskNodeEnd with okFinished=true), the scheduler
			// is rolling the whole plan→apply→verify cycle, which
			// means the OTHER write-mode node rows that completed
			// successfully in the prior iteration are now stale —
			// their "已 X" rendering misleads the user into thinking
			// those steps still hold while a new dispatch is
			// actively re-running them.
			//
			// Specifically: if the user just saw verify fail and the
			// scheduler is about to re-run plan, the apply row from
			// iter 1 is showing "已应用改动" — even though the
			// pipeline has decided that apply needs to re-execute on
			// the new plan. Flip the prior write-mode siblings'
			// okFinished=false so their phrase resolves to the
			// stagePhraseFailed branch ("应用改动失败") for the gap
			// between this start and the sibling's own next start.
			isRetry := !row.endTime.IsZero()
			row.pending = false
			row.paused = false
			if isRetry && isWriteTrioNodeKind(row.nodeKind) {
				// Retry of a write-mode node (plan / apply / verify):
				// the scheduler is rolling the whole plan→apply→verify
				// cycle, so other write-trio rows that completed in
				// the prior iteration are now stale — their "已 X"
				// rendering misleads the user about state that the
				// pipeline has decided to redo. Flip those siblings'
				// okFinished=false so their phrase resolves to the
				// failed branch ("应用改动失败") for the gap between
				// this start and the sibling's own next start.
				//
				// Read-mode evidence rows have non-trio nodeKinds
				// (evidence / validate / reconcile / extract / finalize)
				// and are NOT affected. The isWriteTrioNodeKind guard
				// is the load-bearing scope restrictor; the previous
				// `row.stage` check that lived here was dead code (node
				// rows never set the stage field) and has been removed.
				for _, sib := range r.tasks {
					if sib == nil || sib == row || !sib.isNodeRow {
						continue
					}
					if !isWriteTrioNodeKind(sib.nodeKind) {
						continue
					}
					if !sib.endTime.IsZero() && sib.okFinished {
						sib.okFinished = false
						if sib.errorMsg == "" {
							sib.errorMsg = "retried"
						}
					}
				}
			}
			const dispatchWindowGroupingMs = 750
			kindMatch := r.lastNodeStartKind != "" && r.lastNodeStartKind == row.nodeKind
			sameWindow := kindMatch &&
				!r.lastNodeStartAt.IsZero() &&
				ev.Timestamp.Sub(r.lastNodeStartAt) <= dispatchWindowGroupingMs*time.Millisecond
			if !sameWindow {
				r.dispatchGen++
				for _, other := range r.tasks {
					if other == row || !other.isNodeRow {
						continue
					}
					if other.pending || !other.endTime.IsZero() {
						continue
					}
					if other.dispatchGen < r.dispatchGen {
						other.paused = true
					}
				}
			}
			row.startTime = ev.Timestamp
			row.detailStart = ev.Timestamp
			row.endTime = time.Time{}
			row.okFinished = false
			row.errorMsg = ""
			row.dispatchGen = r.dispatchGen
			r.lastNodeStartAt = ev.Timestamp
			r.lastNodeStartKind = row.nodeKind
			r.current = row
			r.activity = activityState{kind: activityWaitingNode}
			r.streamTail = ""
			r.streamChars = 0
		}

	case EventTaskNodeEnd:
		if row := r.findNodeRow(ev.NodeID); row != nil {
			row.pending = false
			row.paused = false
			row.endTime = ev.Timestamp
			row.errorMsg = ev.Error
			row.okFinished = ev.Error == ""
			row.skipped = ev.NodeSkipped
			topicTotal := r.countTopicSiblings()
			if row == r.current {
				r.current = nil
				if ev.Error != "" {
					r.activity = activityState{kind: activityErrorRecoverable, detail: ev.Error}
				} else {
					r.activity = activityState{kind: activityWaitingDispatch}
				}
				r.streamTail = ""
			}
			r.commitStageDoneLocked(row, topicTotal)
		}

	case EventAgentThinking:
		if r.current != nil {
			r.current.iteration = ev.Iteration + 1
			r.current.detail = "thinking"
			r.current.detailDone = false
			r.current.detailStart = ev.Timestamp
			r.activity = activityState{kind: activityRequesting}
			r.streamTail = ""
			if r.current.isNodeRow {
				for _, other := range r.tasks {
					if other == r.current || !other.isNodeRow {
						continue
					}
					if other.pending || !other.endTime.IsZero() {
						continue
					}
					other.paused = true
				}
			}
		}

	case EventAgentContent:
		if r.current != nil && ev.Reasoning != "" {
			preview := stripMarkdown(ev.Reasoning)
			preview = strings.Join(strings.Fields(preview), " ")
			tail := tailByDisplayWidth(preview, streamTailDisplayCols)
			r.current.detail = "thinking: " + tail
			r.current.detailDone = false
			r.current.detailStart = ev.Timestamp
			r.activity = activityState{kind: activityReceiving}
			r.streamTail = tail
		}

	case EventToolCallStart:
		// Skip detail mutation when the active row has already
		// terminated (endTime != 0). A late tool-call event on a
		// finished row would otherwise reactivate its detail line
		// mid-failure rendering — visually jarring even though the
		// row's okFinished/errorMsg state stays correct. In normal
		// flow this branch is unreachable; the guard hardens against
		// pathological event ordering.
		if r.current != nil && r.current.endTime.IsZero() {
			detail := ev.ToolName
			if ev.ToolDetail != "" {
				detail += " " + ev.ToolDetail
			}
			r.current.detail = detail
			r.current.detailDone = false
			r.current.detailStart = ev.Timestamp
			r.activity = activityState{kind: activityCallingTool, detail: detail}
		}

	case EventToolCallEnd:
		if r.current != nil && r.current.endTime.IsZero() {
			detail := ev.ToolName
			if ev.ToolDetail != "" {
				detail += " " + ev.ToolDetail
			}
			r.current.detail = detail
			r.current.detailDone = true
			if ev.ToolOK {
				r.current.toolCount++
			}
			// Tool finished — flip activity back to "请求模型中" so the
			// dock doesn't sit on stale "调用工具中" between tool end
			// and the next thinking event.
			r.activity = activityState{kind: activityRequesting}
		}

	case EventTransition:
		if r.current != nil {
			r.current.detail = ""
			r.current.detailDone = false
			r.current.detailStart = ev.Timestamp
			r.activity = activityState{kind: activityWaitingDispatch}
			r.streamTail = ""
		}

	case EventSubAgentStart:
		row := &taskRow{
			stage:       string(ev.Stage),
			agent:       ev.SubAgentName,
			startTime:   ev.Timestamp,
			detailStart: ev.Timestamp,
			isSubAgent:  true,
			subAgentID:  ev.SubAgentID,
			subTitle:    ev.SubTaskTitle,
			subCount:    ev.SubTaskCount,
			detail:      ev.SubTaskTitle,
		}
		r.tasks = append(r.tasks, row)

	case EventSubAgentEnd:
		if row := r.findSubAgentRow(ev.SubAgentID); row != nil {
			row.endTime = ev.Timestamp
			row.errorMsg = ev.Error
			row.okFinished = ev.Error == ""
		}

	case EventAdapterRetry:
		// Adapter is sleeping in backoff. Flip dock to a frozen
		// retry state so the user sees we're waiting.
		delaySec := int(ev.RetryDelay / time.Second)
		if delaySec < 1 {
			delaySec = 1
		}
		r.activity = activityState{
			kind:          activityRetrying,
			retryAttempt:  ev.RetryAttempt,
			retryDelaySec: delaySec,
			detail:        ev.RetryReason,
		}
		// Also commit a permanent record so the run log shows the
		// retry happened — useful when the user comes back later
		// and wonders why their run took 60s.
		body := fmt.Sprintf("L1 重试 #%d · 等 %ds", ev.RetryAttempt, delaySec)
		if !isZh(r.lang) {
			body = fmt.Sprintf("L1 retry #%d · in %ds", ev.RetryAttempt, delaySec)
		}
		if ev.RetryReason != "" {
			body += " · " + ev.RetryReason
		}
		line := formatCommitRow(commitRow{
			kind: commitRowRetry,
			body: statusRecoverable.Sprint(body),
		})
		r.commitLineLocked(line)

	case EventAcceptanceReviewStart:
		// Commit 44: flip dock row 1 to "验收审查中" /
		// "acceptance review" while the orchestrator's
		// synchronous LLM Check call runs. Pre-commit-44 row 1
		// stayed at the prior verify's "请求模型中" for 5-30s,
		// indistinguishable from a stalled tool call.
		// PhaseIndex/PhaseTotal carry into detail so a 3-phase
		// run shows "审查中 ▸ phase 2/3".
		detail := ""
		if ev.PhaseTotal > 0 {
			detail = fmt.Sprintf("phase %d/%d", ev.PhaseIndex+1, ev.PhaseTotal)
		}
		r.activity = activityState{
			kind:   activityAcceptanceReview,
			detail: detail,
		}

	case EventAcceptanceReviewEnd:
		// Commit 44: revert to inter-phase pause state. The
		// per-phase verdict (accepted / rejected) lands as a
		// separate EventPhaseProgress (commit 43); this just
		// clears the review activity so the spinner doesn't
		// claim "审查中" while orchestration moves on.
		r.activity = activityState{kind: activityWaitingDispatch}

	case EventWorktreePreparingStart:
		r.activity = activityState{kind: activityPreparingWorktree}

	case EventWorktreePreparingEnd:
		r.activity = activityState{kind: activityWaitingDispatch}

	case EventBaselineCapturingStart:
		r.activity = activityState{kind: activityCapturingBaseline}

	case EventBaselineCapturingEnd:
		r.activity = activityState{kind: activityWaitingDispatch}

	case EventAdapterFallback:
		r.activity = activityState{
			kind:   activitySwitchingProvider,
			detail: ev.FallbackTo,
		}
		body := fmt.Sprintf("切换 provider · %s → %s", ev.FallbackFrom, ev.FallbackTo)
		if !isZh(r.lang) {
			body = fmt.Sprintf("provider fallback · %s → %s", ev.FallbackFrom, ev.FallbackTo)
		}
		line := formatCommitRow(commitRow{
			kind: commitRowRetry,
			body: statusRecoverable.Sprint(body),
		})
		r.commitLineLocked(line)
	}

	if !r.dockEnabled && r.dock == nil {
		// Non-TTY / log_stdout: emit the durable single-line shape.
		r.handleEventNonTTY(ev)
		return
	}
	r.paintDockLocked()
}

// dockHandlePreviewChunk handles EventLivePreviewChunk under the
// dock model. Sets activity=finalizing, refreshes streamTail +
// streamChars, repaints. previewArea field reused as a sentinel
// "streaming in flight" until LivePreviewClear flips it back.
func (r *Renderer) dockHandlePreviewChunk(ev Event) {
	if !r.previewIsTTY() {
		return
	}
	if ev.PreviewText == "" {
		return
	}
	if r.previewArea == nil {
		r.previewArea = newTTYPreviewArea(r.outputWriter())
		r.previewRound++
	}
	r.previewLastChunk = ev.PreviewText
	r.streamChars = utf8.RuneCountInString(ev.PreviewText)
	flat := flattenToOneLine(ev.PreviewText)
	r.streamTail = tailByDisplayWidth(flat, streamTailDisplayCols)
	r.activity = activityState{kind: activityFinalizing}
	r.paintDockLocked()
}

// dockHandlePreviewClear handles EventLivePreviewClear. Drops the
// stream sentinels; on rejection commits a permanent notice.
func (r *Renderer) dockHandlePreviewClear(ev Event) {
	if r.previewArea == nil {
		return
	}
	r.previewArea = nil
	r.previewLastChunk = ""
	r.streamTail = ""
	r.streamChars = 0
	r.activity = activityState{kind: activityWaitingDispatch}
	if ev.PreviewRejected {
		notice := "上一份草稿被规则丢弃，正在重写"
		if !isZh(r.lang) {
			notice = "Previous draft rejected, rewriting"
		}
		line := formatCommitRow(commitRow{
			kind: commitRowNotice,
			body: statusRecoverable.Sprint(notice),
		})
		r.commitLineLocked(line)
	}
	r.paintDockLocked()
}

// commitLineLocked commits a single fully-styled scrollback line.
// Nil-safe: when no dock is attached (test fixtures, non-TTY runs)
// the call is a silent no-op so state-mutation paths can call it
// unconditionally. Mirrors the line (ANSI-stripped) to logging.Info
// so a post-run audit of the log file reads back the same screen
// timeline the user saw.
//
// Caller MUST hold r.mu.
func (r *Renderer) commitLineLocked(line string) {
	if line == "" {
		return
	}
	mirrorDockLineToLog(line)
	if r.dock == nil {
		return
	}
	rows := r.composeCurrentDockRows()
	r.dock.commitToScrollback(line+"\n", rows)
}

// commitMultilineLocked commits a multi-line scrollback batch in
// one paintDock cycle. body MAY contain its own '\n' separators.
// Nil-safe (see commitLineLocked). Each line is independently
// mirrored to logging.Info with ANSI stripped so the log carries
// the structured block (sub-topic enumeration, multi-line notices)
// in readable form.
//
// Caller MUST hold r.mu.
func (r *Renderer) commitMultilineLocked(body string) {
	if body == "" {
		return
	}
	mirrorDockBlockToLog(body)
	if r.dock == nil {
		return
	}
	rows := r.composeCurrentDockRows()
	r.dock.commitToScrollback(body, rows)
}

// mirrorDockLineToLog writes one ANSI-stripped scrollback line to
// the INFO log. Empty / whitespace-only lines are skipped so the
// log doesn't accumulate blank rows from layout padding.
func mirrorDockLineToLog(line string) {
	plain := strings.TrimRight(stripDockAnsi(line), "\n")
	plain = strings.TrimRight(plain, " ")
	if strings.TrimSpace(plain) == "" {
		return
	}
	logging.Info("[render] %s", plain)
}

// mirrorDockBlockToLog writes a multi-line block by splitting on
// '\n' and mirroring each non-empty row. Used by sub-topic
// enumeration + analyze-done batch commits where one
// commitToScrollback call carries N lines.
func mirrorDockBlockToLog(body string) {
	for _, raw := range strings.Split(body, "\n") {
		mirrorDockLineToLog(raw)
	}
}

// dockAnsiPattern matches ANSI CSI sequences for the strip-to-log
// helper. Distinct from reAnsi because we want a tighter regex
// scoped to the dock log path; reAnsi is shared with the truncator
// and might evolve independently.
var dockAnsiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

// stripDockAnsi removes ANSI CSI sequences so the mirrored log
// line reads as plain text. Operates on the rune-level so multi-
// byte CJK content (sub-topic objectives, stage labels) survives.
func stripDockAnsi(s string) string {
	return dockAnsiPattern.ReplaceAllString(s, "")
}

// commitStageDoneLocked formats and commits the durable stage-done
// line for the given row. topicTotal > 1 surfaces the "关注点 K/M"
// suffix. On error the line uses ✗ + red palette.
//
// Caller MUST hold r.mu.
func (r *Renderer) commitStageDoneLocked(row *taskRow, topicTotal int) {
	if row == nil {
		return
	}
	line := r.formatStageDoneLine(row, topicTotal)
	if line == "" {
		return
	}
	r.commitLineLocked(line)
}

// paintDockLocked renders the current 3 dock rows.
//
// Caller MUST hold r.mu.
func (r *Renderer) paintDockLocked() {
	if r.dock == nil {
		return
	}
	rows := r.composeCurrentDockRows()
	r.dock.paintDock(rows)
}

// composeCurrentDockRows builds the three styled row strings from
// the renderer's current state. Pure function over r.* fields.
//
// Caller MUST hold r.mu.
func (r *Renderer) composeCurrentDockRows() [dockRowCount]string {
	state := dockRowState{
		activity:   r.activity,
		streamTail: r.streamTail,
		frame:      spinnerFrames[r.animFrame%len(spinnerFrames)],
		lang:       r.lang,
		cancelHint: r.cancelHint,
	}
	now := time.Now()
	if !r.startTime.IsZero() {
		state.totalElapsed = truncDurationToString(now.Sub(r.startTime))
	}
	focus := r.focusRow()
	if focus != nil {
		state.stageKey = stageKeyFor(focus)
		state.stageProgress = r.stageProgressForFocus(focus)
		state.stageLabel = liveBarPrimaryText(focus, r.lang)
		state.topicProgress = r.topicProgressFor(focus, r.lang)
		state.iteration = focus.iteration
		state.toolCount = focus.toolCount
		if !focus.startTime.IsZero() {
			start := focus.startTime
			if !focus.detailStart.IsZero() && focus.detailStart.After(start) {
				start = focus.detailStart
			}
			state.stageElapsed = truncDurationToString(now.Sub(start))
		}
	} else if fallback := r.fallbackRow(); fallback != nil {
		// focus=nil time windows: post-EventTaskNodeEnd before next
		// EventTaskNodeStart, post-EventAnalysisReady before first
		// node, multi-topic dispatch gaps, mid-extract on rare
		// stage-only paths. Without a fallback row 2 collapses to
		// "▪ —" with no stage word at all. Pick the most recently
		// finished row when one exists, otherwise the first pending
		// row, and render it through the existing label/progress
		// helpers — same vocabulary, same K/N denominator, just
		// resolved to a done/pending lifecycle slot instead of
		// running.
		state.stageKey = stageKeyFor(fallback)
		state.stageProgress = r.stageProgressForFocus(fallback)
		state.stageLabel = liveBarPrimaryText(fallback, r.lang)
		state.topicProgress = r.topicProgressFor(fallback, r.lang)
	} else if r.routeSummary != nil {
		// Light routes (local / chitchat) have no taskRows — the
		// pipeline never ran. Without this branch row 2 sits on a
		// bare "▪ —" with no label for the entire dispatch. Borrow
		// the routeSummary label (already armed by SetRouteSummary
		// for the shutdown line) so row 2 reads as "▪ — 闲聊回复" /
		// "▪ — local reply". Reuses statusPrimary styling via the
		// stageLabel field — no new vocabulary or palette.
		state.stageLabel = r.routeSummary.label
	}
	if r.activity.kind == activityFinalizing {
		state.streamChars = r.streamChars
	}
	return composeDockRows(state)
}

// stageProgressForFocus returns the K/N progress string for the dock
// row 2 stage column. The value of K depends on totalStages — which
// the REPL sets per mode (read=6, ModeApply=4, ModePlan=2,
// ModeVerify=2, write trio=3) — because the same stage key maps to
// a different sequence position depending on which stages actually
// run in this Run.
//
// Examples:
//
//	totalStages=6 (read) — analyze=1/6, evidence=2/6, ..., finalize=6/6
//	totalStages=4 (apply) — analyze=1/4, plan=2/4, apply=3/4, verify=4/4
//	totalStages=3 (write trio, no analyze) — plan=1/3, apply=2/3, verify=3/3
//	totalStages=2 (plan-only) — analyze=1/2, plan=2/2
//	totalStages=2 (verify-only) — analyze=1/2, verify=2/2
//
// Pre-fix the switch hard-coded plan=1/N, apply=2/N, verify=3/N which
// (a) collided with analyze=1/N in ModeApply / ModePlan, (b) overflowed
// the denominator in ModeVerify (verify=3/2 rendered as a literal
// "3/2" string), and (c) never reached 4/N in ModeApply.
//
// log_triage / perf_triage / write_analyze are pre-stages — visible
// in row 2 as labels but rendered with "—" in the K/N slot so the
// counter only advances over the user-relevant stages of this mode.
//
// Caller MUST hold r.mu.
func (r *Renderer) stageProgressForFocus(row *taskRow) string {
	if row == nil {
		return ""
	}
	total := r.totalStages
	if total <= 0 {
		total = 6
	}
	key := stageKeyFor(row)
	switch key {
	case "log_triage", "perf_triage", "write_analyze":
		return "—"
	}
	switch total {
	case 6: // read mode
		switch key {
		case "analyze":
			return "1/6"
		case "explore", "evidence":
			return "2/6"
		case "validate":
			return "3/6"
		case "reconcile":
			return "4/6"
		case "extract":
			return "5/6"
		case "finalize":
			return "6/6"
		}
	case 4: // ModeApply: analyze + plan + apply + verify
		switch key {
		case "analyze":
			return "1/4"
		case "plan", "planner":
			return "2/4"
		case "apply", "coder":
			return "3/4"
		case "verify", "verifier":
			return "4/4"
		}
	case 3: // write trio (no analyze counted)
		switch key {
		case "plan", "planner":
			return "1/3"
		case "apply", "coder":
			return "2/3"
		case "verify", "verifier":
			return "3/3"
		}
	case 2: // ModePlan or ModeVerify (analyze + one write stage)
		switch key {
		case "analyze":
			return "1/2"
		case "plan", "planner", "verify", "verifier":
			return "2/2"
		}
	}
	return ""
}

// formatStageDoneLine renders the durable "✓ K/N 已 X · ..." line
// for a completed row. Returns empty for rows that haven't actually
// ended (called defensively from EventAnalysisReady's analyzeDone
// path which is guaranteed non-nil but the contract is shared).
//
// Caller MUST hold r.mu.
func (r *Renderer) formatStageDoneLine(row *taskRow, topicTotal int) string {
	if row == nil {
		return ""
	}
	totalElapsed := r.totalElapsedString()
	errKind := classifyStatusError(row)
	zh := isZh(r.lang)
	var glyph string
	var glyphStyle, labelStyle = statusSuccessMuted, statusPrimaryDone
	var label string
	switch errKind {
	case statusErrorFatal, statusErrorCancelled:
		glyph = string(glyphFatal)
		glyphStyle = statusFatal
		labelStyle = statusFatal
		label = friendlyPrimaryText(row, r.lang)
	case statusErrorRecoverable:
		glyph = string(glyphRecoverable)
		glyphStyle = statusRecoverable
		labelStyle = statusRecoverable
		label = friendlyPrimaryText(row, r.lang)
	default:
		if row.endTime.IsZero() || !row.okFinished {
			return ""
		}
		glyph = string(glyphSuccess)
		// SkipOnFirstVisit / --skip-verify short-circuit: the LLM
		// didn't run, so the regular "已 X" phrase would lie. Phrase
		// table carries dedicated skipped variants ("已加载预审方案"
		// / "已跳过测试验证") for the two stages that can be skipped
		// in production (plan, verify); other keys fall through to
		// the regular done phrase so this branch is no-op for them.
		if row.skipped {
			label = stagePhraseSkippedFor(stageKeyFor(row), r.lang)
			if label == "" {
				label = stagePhraseDoneFor(stageKeyFor(row), r.lang)
			}
		} else {
			label = stagePhraseDoneFor(stageKeyFor(row), r.lang)
		}
	}
	progress := r.stageProgressForFocus(row)
	stageElapsed := ""
	if !row.endTime.IsZero() {
		stageElapsed = row.endTime.Sub(row.startTime).Truncate(time.Second).String()
	}
	var b strings.Builder
	b.WriteString("  ")
	b.WriteString(glyphStyle.Sprint(glyph))
	if progress != "" {
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint(progress))
	}
	b.WriteString(" ")
	b.WriteString(labelStyle.Sprint(label))
	if row.iteration > 0 {
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint("·"))
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint(metaRoundPhrase(row.iteration, r.lang)))
	}
	if row.toolCount > 0 {
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint("·"))
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint(metaToolCountPhrase(row.toolCount, r.lang)))
	}
	if stageElapsed != "" {
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint("·"))
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint(stageElapsedPhrase(stageElapsed, r.lang)))
	}
	if totalElapsed != "" {
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint("·"))
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint(totalElapsedPhrase(totalElapsed, r.lang)))
	}
	if topicTotal > 1 && row.isNodeRow && row.nodeKind == "evidence" {
		if idx, ok := topicIndexFromNodeID(row.nodeID); ok {
			tag := fmt.Sprintf("关注点 %d/%d", idx+1, topicTotal)
			if !zh {
				tag = fmt.Sprintf("focus %d/%d", idx+1, topicTotal)
			}
			b.WriteString(" ")
			b.WriteString(statusMeta.Sprint("·"))
			b.WriteString(" ")
			b.WriteString(statusObjective.Sprint(tag))
		}
	}
	return b.String()
}

// formatSubTopicsBlock returns the multi-line "分析识别到 N 个关注点：" enumeration
// or empty when fewer than 2 evidence_tN nodes were emitted. Output
// includes leading/trailing blank lines so commitToScrollback writes
// it as a visually framed block.
func formatSubTopicsBlock(lang string, taskNodes []TaskNodeInfo) string {
	type topic struct {
		idx       int
		objective string
	}
	var topics []topic
	for _, n := range taskNodes {
		if n.Type != "evidence" {
			continue
		}
		i, ok := topicIndexFromNodeID(n.ID)
		if !ok {
			continue
		}
		topics = append(topics, topic{idx: i, objective: n.Objective})
	}
	if len(topics) < 2 {
		return ""
	}
	for i := 0; i < len(topics); i++ {
		for j := i + 1; j < len(topics); j++ {
			if topics[j].idx < topics[i].idx {
				topics[i], topics[j] = topics[j], topics[i]
			}
		}
	}
	header := fmt.Sprintf("分析识别到 %d 个关注点：", len(topics))
	if !isZh(lang) {
		header = fmt.Sprintf("Analyzer identified %d focus areas:", len(topics))
	}
	circles := []string{"①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨", "⑩"}
	var b strings.Builder
	b.WriteString("\n  ")
	b.WriteString(statusObjective.Sprint(header))
	b.WriteString("\n")
	for _, t := range topics {
		mark := fmt.Sprintf("%d.", t.idx+1)
		if t.idx >= 0 && t.idx < len(circles) {
			mark = circles[t.idx]
		}
		obj := strings.TrimSpace(t.objective)
		if obj == "" {
			continue
		}
		b.WriteString("    ")
		b.WriteString(statusObjective.Sprint(mark))
		b.WriteString(" ")
		b.WriteString(statusDetail.Sprint(obj))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
}

// formatPhaseGroupBlock returns the multi-phase enumeration block
// the dock prints once at runPhaseGroup entry. Visually parallel to
// formatSubTopicsBlock — header line + N indented rows with circle
// markers — so multi-phase write-mode runs feel like the read-mode
// analyzer's sub-topic enumeration. Empty PhaseList returns "" so
// the caller can degrade silently.
//
// Layout:
//
//	  多阶段方案识别到 3 个 phase：
//	    ① 添加迁移
//	    ② 更新 ORM
//	    ③ 弃用旧字段
func formatPhaseGroupBlock(lang string, phases []PhaseInfo) string {
	if len(phases) == 0 {
		return ""
	}
	header := fmt.Sprintf("多阶段方案识别到 %d 个 phase：", len(phases))
	if !isZh(lang) {
		header = fmt.Sprintf("Multi-phase plan with %d phases:", len(phases))
	}
	circles := []string{"①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨", "⑩", "⑪", "⑫"}
	var b strings.Builder
	b.WriteString("\n  ")
	b.WriteString(statusObjective.Sprint(header))
	b.WriteString("\n")
	for _, p := range phases {
		mark := fmt.Sprintf("%d.", p.Index+1)
		if p.Index >= 0 && p.Index < len(circles) {
			mark = circles[p.Index]
		}
		goal := strings.TrimSpace(p.Goal)
		if goal == "" {
			continue
		}
		b.WriteString("    ")
		b.WriteString(statusObjective.Sprint(mark))
		b.WriteString(" ")
		b.WriteString(statusDetail.Sprint(goal))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
}

// formatPhaseProgressLine returns the single-line dock entry for
// an EventPhaseProgress. Pre-commit-43 these entries went through
// the generic EventAgentReasoning path and rendered with the
// "💭 [orchestrator-1] ..." LLM-thinking icon — semantically wrong
// for what is structural progression, not LLM reasoning. Now they
// render with status-typed icons (▶ start / ✓ accepted / ✗ rejected)
// + same color palette as the sub-topic block.
func formatPhaseProgressLine(lang string, idx, total int, kind PhaseProgressKind, detail string) string {
	zh := isZh(lang)
	icon := "▶"
	switch kind {
	case PhaseProgressAccepted:
		icon = "✓"
	case PhaseProgressRejected:
		icon = "✗"
	}
	header := fmt.Sprintf("%s Phase %d/%d", icon, idx+1, total)
	suffix := ""
	switch kind {
	case PhaseProgressStart:
		if zh {
			suffix = " 启动"
		} else {
			suffix = " starting"
		}
	case PhaseProgressAccepted:
		if zh {
			suffix = " 已接受"
		} else {
			suffix = " accepted"
		}
	case PhaseProgressRejected:
		if zh {
			suffix = " 被拒绝"
		} else {
			suffix = " rejected"
		}
	}
	out := "  " + statusObjective.Sprint(header+suffix)
	if d := strings.TrimSpace(detail); d != "" {
		out += statusDetail.Sprint(": " + d)
	}
	return out
}

// countTopicSiblings returns how many evidence_tN nodes are present
// in r.tasks. Used by commit to decide whether the "关注点 K/M"
// suffix should appear on the completion line.
//
// Caller MUST hold r.mu.
func (r *Renderer) countTopicSiblings() int {
	total := 0
	for _, sibling := range r.tasks {
		if sibling.isNodeRow && sibling.nodeKind == "evidence" {
			if _, ok := topicIndexFromNodeID(sibling.nodeID); ok {
				total++
			}
		}
	}
	return total
}

// handleEventNonTTY handles events when the dock is disabled (non-
// TTY stdout, --log-stdout in TTY mode, etc.). Each event maps to
// at most one Println-style line so CI logs and piped output remain
// readable, and each line is mirrored to logging.Info so a post-run
// audit can replay the event timeline from the log file alone.
//
// emitNonTTYLine is the central writer: it prints to stdout AND
// mirrors to INFO log. Both writes carry the same plain-text body
// (no ANSI escapes either side, since the non-TTY path never
// styles).
func (r *Renderer) handleEventNonTTY(ev Event) {
	switch ev.Kind {
	case EventObjectiveStarted:
		if ev.Objective != "" {
			r.emitNonTTYLine(fmt.Sprintf("❯ %s", ev.Objective))
		}
	case EventStageStart:
		r.emitNonTTYLine(fmt.Sprintf("→ %s", string(ev.Stage)))
	case EventStageEnd:
		if ev.Error != "" {
			r.emitNonTTYLine(fmt.Sprintf("✗ %s · %s", string(ev.Stage), ev.Error))
		} else {
			r.emitNonTTYLine(fmt.Sprintf("✓ %s", string(ev.Stage)))
		}
	case EventTaskNodeStart:
		r.emitNonTTYLine(fmt.Sprintf("→ %s · %s", ev.NodeKind, ev.NodeObjective))
	case EventTaskNodeEnd:
		if ev.Error != "" {
			r.emitNonTTYLine(fmt.Sprintf("✗ %s · %s", ev.NodeKind, ev.Error))
		} else {
			r.emitNonTTYLine(fmt.Sprintf("✓ %s", ev.NodeKind))
		}
	case EventAnalysisReady:
		if block := formatSubTopicsBlock(r.lang, ev.TaskNodes); block != "" {
			fmt.Fprint(r.outputWriter(), block)
			mirrorDockBlockToLog(block)
		}
	case EventAgentReasoning:
		r.emitNonTTYLine(formatReasoning(string(ev.Agent), ev.Iteration, ev.Reasoning))
	case EventAdapterRetry:
		r.emitNonTTYLine(fmt.Sprintf("⟳ retry #%d in %v · %s",
			ev.RetryAttempt, ev.RetryDelay, ev.RetryReason))
	case EventAdapterFallback:
		r.emitNonTTYLine(fmt.Sprintf("⟳ fallback %s → %s · %s",
			ev.FallbackFrom, ev.FallbackTo, ev.RetryReason))
	case EventPhaseGroupStart:
		// Commit 43: render the full phase enumeration once
		// at group entry so operators see the workflow shape
		// before stages start. Mirror of EventAnalysisReady's
		// sub-topic block.
		if block := formatPhaseGroupBlock(r.lang, ev.PhaseList); block != "" {
			fmt.Fprint(r.outputWriter(), block)
			mirrorDockBlockToLog(block)
		}
	case EventPhaseProgress:
		// Commit 43: per-phase status row with structured
		// icon ▶/✓/✗ instead of the 💭 thought-bubble used
		// by EventAgentReasoning. Detail surfaces phase goal
		// (start) or rejection reasoning (rejected).
		r.emitNonTTYLine(formatPhaseProgressLine(r.lang, ev.PhaseIndex, ev.PhaseTotal,
			ev.PhaseProgressKind, ev.PhaseDetail))
	}
}

// emitNonTTYLine writes a single line to stdout AND mirrors it to
// the INFO log so non-TTY runs still produce a complete audit
// trail without the operator having to scrape stdout.
func (r *Renderer) emitNonTTYLine(line string) {
	if line == "" {
		return
	}
	fmt.Fprintln(r.outputWriter(), line)
	mirrorDockLineToLog(line)
}

// commitDockShutdownLocked prints the closing run-summary line and
// clears the dock. Used by StopSpinner.
//
// Caller MUST hold r.mu.
func (r *Renderer) commitDockShutdownLocked() {
	if r.dock == nil {
		return
	}
	// Light-route override: when the REPL (local / chat) armed a
	// route summary before StopSpinner, swap the default
	// "◆ 已结束 · 4 阶段 · 总耗时 7s · …" line for the route-specific
	// "◇ <label> · <segments> · 总耗时 Xs" shape. The pipeline path
	// has nothing to count (0 stages, 0 tools, 0 iterations) so the
	// default shape would render misleading zeros; the override
	// names what actually happened. Single-shot consumption — clear
	// the cached summary so a later Run on the same Renderer falls
	// back to the default.
	zh := isZh(r.lang)
	// Terminal-state override (must precede the routeSummary path
	// so a cancelled / failed light route shows the ✗ summary
	// instead of the cheerful "◇ 本地回复 · …" — the user's last
	// signal was Ctrl+C / an error, the dock should agree).
	// MarkRunFatal / MarkRunCancelled arm the kind before
	// StopSpinner; we swap the regular "◆ 已结束 · …" summary for
	// "✗ 已失败 · 总耗时 Xs" / "✗ 已取消 · 总耗时 Xs" using the
	// existing commitRowFailure / commitRowCancelled shapes (red
	// ✗ glyph + statusFatal palette — no new style introduced).
	if r.activity.kind == activityErrorFatal || r.activity.kind == activityCancelled {
		totalElapsed := r.totalElapsedString()
		if totalElapsed == "" {
			totalElapsed = "0s"
		}
		var head string
		var rowKind commitRowKind
		switch {
		case r.activity.kind == activityCancelled:
			rowKind = commitRowCancelled
			if zh {
				head = "已取消"
			} else {
				head = "cancelled"
			}
		default:
			rowKind = commitRowFailure
			if zh {
				head = "已失败"
			} else {
				head = "failed"
			}
		}
		var body strings.Builder
		body.WriteString(head)
		body.WriteString(" ")
		body.WriteString(statusMeta.Sprint("·"))
		body.WriteString(" ")
		body.WriteString(statusMeta.Sprint(totalElapsedPhrase(totalElapsed, r.lang)))
		line := formatCommitRow(commitRow{
			kind: rowKind,
			body: statusFatal.Sprint(body.String()),
		})
		r.commitLineLocked(line)
		r.dock.clearDock()
		r.routeSummary = nil
		return
	}
	if r.routeSummary != nil {
		elapsed := r.totalElapsedString()
		line := FormatLightRouteSummary(r.routeSummary.label, r.routeSummary.segments, elapsed, r.lang)
		r.commitLineLocked(line)
		r.dock.clearDock()
		r.routeSummary = nil
		return
	}
	completed := 0
	totalTools := 0
	totalIters := 0
	for _, row := range r.tasks {
		if row == nil {
			continue
		}
		if !row.endTime.IsZero() {
			completed++
		}
		totalTools += row.toolCount
		if row.iteration > totalIters {
			totalIters = row.iteration
		}
	}
	totalElapsed := r.totalElapsedString()
	if totalElapsed == "" {
		totalElapsed = "0s"
	}
	var body strings.Builder
	if zh {
		body.WriteString("已结束")
	} else {
		body.WriteString("done")
	}
	if completed > 0 {
		body.WriteString(" ")
		body.WriteString(statusMeta.Sprint("·"))
		body.WriteString(" ")
		if zh {
			body.WriteString(statusMeta.Sprint(fmt.Sprintf("%d 阶段", completed)))
		} else {
			body.WriteString(statusMeta.Sprint(fmt.Sprintf("%d stages", completed)))
		}
	}
	body.WriteString(" ")
	body.WriteString(statusMeta.Sprint("·"))
	body.WriteString(" ")
	body.WriteString(statusMeta.Sprint(totalElapsedPhrase(totalElapsed, r.lang)))
	if totalTools > 0 {
		body.WriteString(" ")
		body.WriteString(statusMeta.Sprint("·"))
		body.WriteString(" ")
		body.WriteString(statusMeta.Sprint(metaToolCountPhrase(totalTools, r.lang)))
	}
	if totalIters > 0 {
		body.WriteString(" ")
		body.WriteString(statusMeta.Sprint("·"))
		body.WriteString(" ")
		if zh {
			body.WriteString(statusMeta.Sprint(fmt.Sprintf("%d 轮 LLM 对话", totalIters)))
		} else {
			body.WriteString(statusMeta.Sprint(fmt.Sprintf("%d LLM rounds", totalIters)))
		}
	}
	line := formatCommitRow(commitRow{
		kind: commitRowFinal,
		body: statusPrimaryDone.Sprint(body.String()),
	})
	r.commitLineLocked(line)
	r.dock.clearDock()
}

// dockEventEmitter returns the dock-driven emitter callback. Used
// by Renderer.Emitter() once dock mode is enabled.
func (r *Renderer) dockEventEmitter() EventEmitter {
	return func(ev Event) { r.handleEvent(ev) }
}

// _silence is a discard sink that satisfies io.Writer for any
// fallback writes that might have been routed to a removed surface.
var _silence io.Writer = io.Discard

// reAnsiCheck guards the ANSI-aware truncation in dock rows.
// Provided so the row composer can be tested without circular import.
var reAnsiCheck = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)
