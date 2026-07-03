package render

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/pterm/pterm"
)

// dockTickInterval is the animation refresh cadence. 100 ms is the
// established CLI sweet spot for braille-spinner perceived smoothness
// without spamming the terminal. Operators can override via the
// renderer's setter — left as a future yaml knob if anyone needs it.
const dockTickInterval = 100 * time.Millisecond

// streamTailDisplayCols caps the row-1 `▸ tail` segment display
// width. ~50 cols keeps a little more live text before the received-size
// counter to understand what is arriving, while composeDockRows still
// applies the terminal-width cap so narrow panes never wrap.
const streamTailDisplayCols = 50

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
		if lines := r.answerDraftPreviewLinesOnPreviewClear(ev); len(lines) > 0 {
			if !r.dockEnabled && r.dock == nil {
				r.emitNonTTYLines(lines)
			} else {
				r.commitScrollbackLinesLocked(lines)
			}
		}
		r.dockHandlePreviewClear(ev)
		return
	case EventAgentReasoning:
		if ev.Reasoning == "" {
			return
		}
		// Same-round duplicate echo skip (see lastReasoningEchoKey on
		// the Renderer struct): when the content channel repeats the
		// reasoning channel verbatim (after display-layer collapse +
		// trim) within one round, the second echo adds zero
		// information — drop it before any formatting. Both branches
		// below (TTY commit and handleEventNonTTY replay) are covered
		// because each recomputes from ev.Reasoning.
		echoKey := fmt.Sprintf("%s|%s|%d|%s", ev.Agent, ev.Stage, ev.Iteration, ev.ParallelUnitID)
		echoText := strings.TrimSpace(collapseRepeatedSegments(ev.Reasoning, r.lang))
		if echoKey == r.lastReasoningEchoKey && echoText == r.lastReasoningEchoText {
			return
		}
		r.lastReasoningEchoKey, r.lastReasoningEchoText = echoKey, echoText
		lines := formatReasoningLinesWithParallel(string(ev.Agent), ev.Stage, ev.Iteration, ev.Reasoning, r.thinkingTruncate, r.lang, r.activityTraceUnitLabelLocked(ev))
		if len(lines) == 0 {
			return
		}
		r.bindLiveEventStageLocked(string(ev.Stage))
		if !r.dockEnabled && r.dock == nil {
			r.handleEventNonTTY(ev)
			return
		}
		r.commitScrollbackLinesLocked(lines)
		return
	case EventAgentToolCallBatch:
		line := formatUnavailableToolCallBatchWithParallel(string(ev.Agent), ev.Stage, ev.Iteration, ev.ToolNames,
			ev.ToolCallCount, ev.ToolName, ev.ToolDetail, r.lang, r.activityTraceUnitLabelLocked(ev), ev.UnavailableToolNames)
		if line == "" {
			return
		}
		r.bindLiveEventStageLocked(string(ev.Stage))
		detail := ev.ToolName
		if detail == "" && len(ev.ToolNames) > 0 {
			detail = strings.Join(ev.ToolNames, ", ")
		}
		if ev.ToolDetail != "" {
			if detail != "" {
				detail += " "
			}
			detail += ev.ToolDetail
		}
		r.activity = activityState{kind: activityCallingTool, detail: detail}
		if !r.dockEnabled && r.dock == nil {
			r.handleEventNonTTY(ev)
			return
		}
		r.commitLineLocked(line)
		return
	case EventAgentLLMWaiting:
		// Slow-LLM-request heartbeat (2026-07-03 berlin.systrace dead-
		// silence session: one 14m35s model response with the last
		// visible line frozen on the preceding trace_query call). The
		// live dock needs no state mutation here — its animation ticker
		// already repaints row 1 ("请求模型中" + elapsed) while the
		// request is in flight; what was missing is the DURABLE record
		// in scrollback and on non-TTY / --log-stdout runs. Durable
		// lines are rate-limited to power-of-two WaitTicks (precise
		// integer gate — hard requirement) so a very long request costs
		// O(log n) lines; this is the single gate site for both the
		// TTY and non-TTY branches.
		if !llmWaitHeartbeatDurable(ev.WaitTick) {
			return
		}
		if !r.dockEnabled && r.dock == nil {
			r.handleEventNonTTY(ev)
			return
		}
		r.commitLineLocked(formatLLMWaitHeartbeatLine(string(ev.Agent), ev.Stage, ev.Iteration,
			ev.WaitElapsed, ev.ModelID, r.lang, r.activityTraceUnitLabelLocked(ev)))
		return
	case EventOrchestratorNotice:
		// Distinct from EventAgentReasoning: no thinking glyph,
		// no stage-round trace label — the user must be able to tell at a
		// glance whether a line is the LLM thinking aloud or the
		// orchestrator announcing a control-flow decision.
		scanProgressOnly := ev.NoticeKind == NoticeRepoMapScanProgress
		scanDone := ev.NoticeKind == NoticeRepoMapScanOK || ev.NoticeKind == NoticeRepoMapScanFail
		if ev.Stage != "" && !scanDone {
			r.bindLiveEventStageLocked(string(ev.Stage))
		}
		line := r.formatOrchestratorNoticeLocked(ev.NoticeKind, ev.Reasoning, string(ev.Stage))
		if line == "" {
			return
		}
		r.recordOrchestratorNoticeTelemetry(ev.NoticeKind)
		if ev.NoticeKind == NoticeFinalizing {
			r.activity = activityState{kind: activityFinalizing}
			r.streamTail = ""
			r.streamChars = 0
		} else if ev.NoticeKind == NoticeRepoMapScanStart || ev.NoticeKind == NoticeRepoMapScanProgress {
			_, body := peelGlyphPrefix(ev.Reasoning)
			if body == "" {
				body = ev.Reasoning
			}
			r.activity = activityState{kind: activityRepoMapScanning}
			r.streamTail = strings.TrimSpace(body)
		} else if ev.NoticeKind == NoticeRepoMapScanOK || ev.NoticeKind == NoticeRepoMapScanFail {
			if r.activity.kind == activityRepoMapScanning {
				r.activity = activityState{kind: activityWaitingNode}
				r.streamTail = ""
			}
			if localStageKeyMatchesStageKey(r.localStageKey, string(ev.Stage)) {
				r.localStageKey = ""
			}
		}
		if !r.dockEnabled && r.dock == nil {
			r.handleEventNonTTY(ev)
			return
		}
		if scanProgressOnly {
			r.paintDockLocked()
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
			r.handleEventNonTTY(ev)
			return
		}
		r.commitMultilineLocked(block)
		return
	case EventPhaseProgress:
		// Commit 43: TTY-mode phase-status row.
		line := formatPhaseProgressLine(r.lang, ev.PhaseIndex, ev.PhaseTotal,
			ev.PhaseProgressKind, ev.PhaseDetail)
		if !r.dockEnabled && r.dock == nil {
			r.handleEventNonTTY(ev)
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
		r.localStageKey = ""
		r.clearParallelIfStageKeyOutsideLocked(canonicalStageKey(string(ev.Stage)))
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
			firstStart:  ev.Timestamp,
			detailStart: ev.Timestamp,
		}
		r.tasks = append(r.tasks, row)
		r.current = row
		r.activity = activityState{kind: activityWaitingNode}
		r.streamTail = ""
		r.streamChars = 0

	case EventStageEnd:
		if canonicalStageKey(string(ev.Stage)) == r.localStageKey {
			r.localStageKey = ""
		}
		if canonicalStageKey(string(ev.Stage)) == "explore" {
			r.parallel = nil
		}
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
				// Errored stage: hold the matching activity shape on
				// row 1 so the user sees the failure before the next
				// dispatch overwrites it. classifyEventError uses the
				// same fatalMarkers / cancelledMarkers / recoverable
				// priority as classifyStatusError → dock row 1 reads
				// the same severity as the just-committed scrollback
				// line (no more ↻ on row 1 + ✗ in scrollback for the
				// same event).
				if ev.Error != "" {
					switch classifyEventError(ev.Error) {
					case statusErrorFatal:
						r.activity = activityState{kind: activityErrorFatal, detail: ev.Error}
					case statusErrorCancelled:
						r.activity = activityState{kind: activityCancelled, detail: ev.Error}
					default:
						r.activity = activityState{kind: activityErrorRecoverable, detail: ev.Error}
					}
				} else {
					r.activity = activityState{kind: activityWaitingDispatch}
				}
				r.streamTail = ""
			}
			r.commitStageDoneLocked(row, 0)
		}

	case EventAnalysisReady:
		r.localStageKey = ""
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
				isNodeRow:            true,
				nodeID:               n.ID,
				nodeKind:             n.Type,
				objective:            n.Objective,
				hasInvestigationUnit: n.HasInvestigationUnit,
				investigationUnit:    n.InvestigationUnit,
				pending:              true,
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
		if analysisBlock := formatAnalysisBreakdownBlock(r.lang, ev.TaskNodes, ev.AnswerDimensions); analysisBlock != "" {
			batch.WriteString(analysisBlock)
		}
		if batch.Len() > 0 {
			r.commitMultilineLocked(batch.String())
		}

	case EventTaskNodeStart:
		r.localStageKey = ""
		if row := r.findNodeRow(ev.NodeID); row != nil {
			r.clearParallelIfStageKeyOutsideLocked(stageKeyFor(row))
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
			if row.firstStart.IsZero() {
				row.firstStart = ev.Timestamp
			}
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
		if r.current == nil {
			r.localStageKey = ""
		}
		if row := r.findNodeRow(ev.NodeID); row != nil {
			if localStageKeyMatchesStageKey(r.localStageKey, stageKeyFor(row)) {
				r.localStageKey = ""
			}
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
					switch classifyEventError(ev.Error) {
					case statusErrorFatal:
						r.activity = activityState{kind: activityErrorFatal, detail: ev.Error}
					case statusErrorCancelled:
						r.activity = activityState{kind: activityCancelled, detail: ev.Error}
					default:
						r.activity = activityState{kind: activityErrorRecoverable, detail: ev.Error}
					}
				} else {
					r.activity = activityState{kind: activityWaitingDispatch}
				}
				r.streamTail = ""
			}
			r.commitStageDoneLocked(row, topicTotal)
		}

	case EventParallelDispatchStart:
		r.parallel = newParallelActivity(ev)
		r.activity = activityState{kind: activityWaitingNode}
		r.streamTail = ""

	case EventParallelDispatchUnitStart:
		if r.parallel != nil {
			r.parallel.startUnit(ev)
			r.activity = activityState{kind: activityWaitingNode}
			r.streamTail = ""
		}

	case EventParallelDispatchUnitEnd:
		if r.parallel != nil {
			r.parallel.endUnit(ev)
		}

	case EventParallelDispatchEnd:
		r.parallel = nil
		r.activity = activityState{kind: activityWaitingDispatch}
		r.streamTail = ""

	case EventAgentThinking:
		// A new LLM request is starting: reset the reasoning→content
		// echo-dedupe state so its scope is precisely "the two
		// emissions of ONE response". Without this reset the state
		// lived forever, and any later response with the same (agent,
		// stage, iteration, unit) key and same text — e.g. the
		// chit-chat path, where Iteration is always 0 across turns —
		// had its REAL reasoning silently swallowed (2026-07-03
		// adversarial review WF2). EventAgentThinking is emitted by
		// BaseAgent before every LLM call, so it is the per-request
		// boundary event.
		r.lastReasoningEchoKey, r.lastReasoningEchoText = "", ""
		r.bindLiveEventStageLocked(string(ev.Stage))
		r.recordLLMRequestTelemetry(ev)
		if r.parallel != nil {
			r.parallel.setUnitActivity(ev, activityRequesting, "", "")
		}
		r.activity = activityState{kind: activityRequesting}
		r.streamTail = ""
		if r.current != nil {
			r.current.iteration = ev.Iteration + 1
			r.current.detail = "thinking"
			r.current.detailDone = false
			r.current.detailStart = ev.Timestamp
			if r.current.isNodeRow && !(r.parallel != nil && r.parallel.matches(ev)) {
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

	case EventLLMRequestStart:
		// Same per-request echo-dedupe reset as EventAgentThinking:
		// direct dispatches that signal request start through this
		// event get the same "one response" dedupe scope.
		r.lastReasoningEchoKey, r.lastReasoningEchoText = "", ""
		r.bindLiveEventStageLocked(string(ev.Stage))
		r.recordLLMRequestTelemetry(ev)
		if r.parallel != nil {
			r.parallel.setUnitActivity(ev, activityRequesting, "", "")
		}
		r.activity = activityState{kind: activityRequesting}
		r.streamTail = ""
		if r.current != nil && r.current.endTime.IsZero() && r.current.detail == "" {
			r.current.detail = "thinking"
			r.current.detailDone = false
			r.current.detailStart = ev.Timestamp
		}

	case EventAgentResponse:
		r.bindLiveEventStageLocked(string(ev.Stage))
		// The LLM call has returned; any remaining work before the
		// next visible tool/stage event is local parsing, validation, or
		// context assembly. Do not leave row 1 claiming "请求模型中"
		// during CPU-bound post-processing.
		if r.parallel != nil {
			r.parallel.setUnitActivity(ev, activityWaitingNode, "", "")
		}
		r.activity = activityState{kind: activityWaitingNode}
		r.streamTail = ""

	case EventLocalWorkStart:
		// Scheduler / validator local work is not an LLM request. Keep
		// the dock honest without committing a noisy scrollback line for
		// every fast local phase.
		r.localStageKey = canonicalStageKey(string(ev.Stage))
		r.clearParallelIfStageKeyOutsideLocked(r.localStageKey)
		r.activity = activityState{kind: activityWaitingNode}
		r.streamTail = ""

	case EventAgentContent:
		if ev.Reasoning != "" {
			r.bindLiveEventStageLocked(string(ev.Stage))
			preview := stripMarkdown(ev.Reasoning)
			preview = strings.Join(strings.Fields(preview), " ")
			tail := tailByDisplayWidth(preview, streamTailDisplayCols)
			if r.parallel != nil {
				r.parallel.setUnitActivity(ev, activityReceiving, "", tail)
			}
			if r.current != nil {
				r.current.detail = "thinking: " + tail
				r.current.detailDone = false
				r.current.detailStart = ev.Timestamp
			}
			r.activity = activityState{kind: activityReceiving}
			r.streamTail = tail
		}

	case EventToolCallStart:
		r.bindLiveEventStageLocked(string(ev.Stage))
		// Skip detail mutation when the active row has already
		// terminated (endTime != 0). A late tool-call event on a
		// finished row would otherwise reactivate its detail line
		// mid-failure rendering — visually jarring even though the
		// row's okFinished/errorMsg state stays correct. In normal
		// flow this branch is unreachable; the guard hardens against
		// pathological event ordering.
		detail := ev.ToolName
		if ev.ToolDetail != "" {
			detail += " " + ev.ToolDetail
		}
		if r.parallel != nil {
			r.parallel.setUnitActivity(ev, activityCallingTool, detail, "")
		}
		if r.current != nil && r.current.endTime.IsZero() {
			r.current.detail = detail
			r.current.detailDone = false
			r.current.detailStart = ev.Timestamp
			r.activity = activityState{kind: activityCallingTool, detail: detail}
		}

	case EventToolCallEnd:
		r.bindLiveEventStageLocked(string(ev.Stage))
		if r.parallel != nil {
			r.parallel.setUnitActivity(ev, activityRequesting, "", "")
		}
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
		if r.dockEnabled || r.dock != nil {
			if lines := r.answerDraftPreviewLinesOnToolEnd(ev); len(lines) > 0 {
				r.commitScrollbackLinesLocked(lines)
			}
			if shouldRenderStructuredToolSummary(ev) {
				if block := formatStructuredToolResultSummary(ev.ToolName, ev.ToolParamsJSON, ev.ToolResultSummary, r.lang, ev.EvidenceTotal); block != "" {
					r.commitMultilineLocked(block)
				}
			}
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
			firstStart:  ev.Timestamp,
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
		if r.parallel != nil {
			r.parallel.setUnitActivity(ev, activityRetrying, ev.RetryReason, "")
		}
		r.adapterRetryTotal++
		// Also commit a permanent record so the run log shows the
		// retry happened — useful when the user comes back later
		// and wonders why their run took 60s. User-facing language:
		// SPELL OUT what is being retried (LLM model request) so the
		// operator does not have to guess; drop internal "L1" layer-
		// name and "#N" issue-tracker syntax (those are for the
		// [llm] log lines). RetryReason is localised through
		// localizeRetryReason so a zh user does not see English
		// "rate limit" mixed into Chinese prose.
		var body string
		if isZh(r.lang) {
			body = fmt.Sprintf("已重新请求模型 (第 %d 次,等 %ds)", ev.RetryAttempt, delaySec)
		} else {
			body = fmt.Sprintf("retried model request (attempt %d, after %ds)", ev.RetryAttempt, delaySec)
		}
		if ev.RetryReason != "" {
			body += " · " + localizeRetryReason(ev.RetryReason, r.lang)
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
		// run shows "审查中 ▸ 阶段 2/3" (zh) / "review ▸ phase 2/3"
		// (en). Pre-2026-05-06 detail was hardcoded "phase N/M" in
		// English regardless of language — Chinese users saw a
		// mixed-language "审查中 ▸ phase 2/3" tail.
		detail := ""
		if ev.PhaseTotal > 0 {
			if isZh(r.lang) {
				detail = fmt.Sprintf("阶段 %d/%d", ev.PhaseIndex+1, ev.PhaseTotal)
			} else {
				detail = fmt.Sprintf("phase %d/%d", ev.PhaseIndex+1, ev.PhaseTotal)
			}
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
		if ev.FallbackTo != "" {
			r.requestModelID = ev.FallbackTo
		}
		r.activity = activityState{
			kind:   activitySwitchingProvider,
			detail: ev.FallbackTo,
		}
		if r.parallel != nil {
			r.parallel.setUnitActivity(ev, activitySwitchingProvider, ev.FallbackTo, "")
		}
		r.adapterFallbackTotal++
		body := fmt.Sprintf("切换 LLM 服务 %s → %s", ev.FallbackFrom, ev.FallbackTo)
		if !isZh(r.lang) {
			body = fmt.Sprintf("switched LLM provider %s → %s", ev.FallbackFrom, ev.FallbackTo)
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
// Consecutive duplicate suppression: the orchestrator can emit the
// same soft notice twice in a row (e.g. two forced-read passes both
// firing "↻ 正在补充关键信息" within the same dispatch). Compare the
// new line byte-identically to the immediately-prior committed line
// and skip when they match — legitimate non-notice commits (per-stage
// success / failure / question rows) carry distinguishing K/N or
// stage labels, so a real byte-identical duplicate only happens on
// the soft-notice path.
//
// Caller MUST hold r.mu.
func (r *Renderer) commitLineLocked(line string) {
	if line == "" {
		return
	}
	if r.dock == nil {
		return
	}
	if line == r.lastCommittedLine {
		return
	}
	r.lastCommittedLine = line
	mirrorDockLineToLog(line)
	rows := r.composeCurrentDockRows()
	r.dock.commitToScrollback(line+"\n", rows)
}

func (r *Renderer) commitScrollbackLinesLocked(lines []scrollbackLine) {
	if len(lines) == 0 {
		return
	}
	if len(lines) == 1 && !lines[0].visual {
		r.commitLineLocked(lines[0].text)
		return
	}
	if r.dock == nil {
		return
	}
	body := formatScrollbackBody(lines, isTTY(r.outputWriter()))
	if body == r.lastCommittedLine {
		return
	}
	r.lastCommittedLine = body
	mirrorDockBlockToLog(body)
	rows := r.composeCurrentDockRows()
	r.dock.commitToScrollback(body, rows)
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
	if r.dock == nil {
		return
	}
	mirrorDockBlockToLog(body)
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

func formatScrollbackBody(lines []scrollbackLine, disableVisualAutoWrap bool) string {
	if len(lines) == 0 {
		return ""
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		text := line.text
		if line.visual && disableVisualAutoWrap {
			text = "\x1b[?7l" + text + "\x1b[?7h"
		}
		out = append(out, text)
	}
	return strings.Join(out, "\n")
}

// dockAnsiPattern matches ANSI CSI sequences for the strip-to-log
// helper. Distinct from reAnsi because we want a tighter regex
// scoped to the dock log path; reAnsi is shared with the truncator
// and might evolve independently.
var dockAnsiPattern = regexp.MustCompile(`\x1b\[[0-9;:?]*[A-Za-z]`)

// stripDockAnsi removes ANSI CSI sequences so the mirrored log
// line reads as plain text. Operates on the rune-level so multi-
// byte CJK content (sub-topic objectives, stage labels) survives.
func stripDockAnsi(s string) string {
	return dockAnsiPattern.ReplaceAllString(s, "")
}

// commitStageDoneLocked formats and commits the durable stage-done
// line for the given row. topicTotal > 1 surfaces the ordinal focus
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

// recordLLMRequestTelemetry stores the latest request-level model /
// context signal independently of any task row. Some LLM calls are
// direct reviewer dispatches (no BaseAgent row), and some dock frames
// render fallback rows or soft-notice windows; the telemetry should
// remain visible whenever it describes the current or most-recent LLM
// request instead of disappearing with row focus churn.
//
// Caller MUST hold r.mu.
func (r *Renderer) recordLLMRequestTelemetry(ev Event) {
	if strings.TrimSpace(ev.ModelID) != "" {
		r.requestModelID = ev.ModelID
	}
	if ev.ContextTokensEstimate > 0 {
		r.requestContextTokensEstimate = ev.ContextTokensEstimate
	}
	if ev.ContextWindowTokens > 0 {
		r.requestContextWindowTokens = ev.ContextWindowTokens
	}
}

// composeCurrentDockRows builds the three styled row strings from
// the renderer's current state. Pure function over r.* fields.
//
// Caller MUST hold r.mu.
func (r *Renderer) composeCurrentDockRows() [dockRowCount]string {
	now := time.Now()
	state := dockRowState{
		activity:   lightRouteActivityWithCountdown(r.activity, now, r.lang),
		streamTail: r.streamTail,
		frame:      spinnerFrames[r.animFrame%len(spinnerFrames)],
		lang:       r.lang,
		cancelHint: r.cancelHint,
		parallel:   r.parallel.snapshot(),
	}
	if !r.startTime.IsZero() {
		state.totalElapsed = truncDurationToString(now.Sub(r.startTime))
	}
	focus := r.focusRow()
	localKey := r.localStagePresentationKeyLocked()
	if key, label, parallelFocus := r.parallelStagePresentationLocked(focus, localKey); key != "" {
		r.applyDockStagePresentationLocked(&state, key, label)
		// Parallel dispatch is an active stage owner, not a stale
		// repair/fallback row. Keep its visible K/N on the stage it
		// actually belongs to so exploration cannot read as extract's
		// 3/4 while fan-out workers are still running.
		if progress := progressForStageKey(key, normalizedTotalStages(r.totalStages)); progress != "" {
			state.stageProgress = progress
		}
		if parallelFocus != nil {
			state.topicProgress = r.topicProgressFor(parallelFocus, r.lang)
			state.iteration = parallelFocus.iteration
			state.toolCount = parallelFocus.toolCount
			if !parallelFocus.startTime.IsZero() {
				start := parallelFocus.startTime
				if !parallelFocus.detailStart.IsZero() && parallelFocus.detailStart.After(start) {
					start = parallelFocus.detailStart
				}
				state.stageElapsed = truncDurationToString(now.Sub(start))
			}
		}
	} else if localKey != "" && r.localStageOverridesFocusLocked(focus, localKey) {
		r.applyDockStagePresentationLocked(&state, localKey, syntheticStageLabelForKey(localKey, r.lang, r.activity.kind))
	} else if focus != nil {
		key := stageKeyFor(focus)
		r.applyDockStagePresentationLocked(&state, key, liveBarPrimaryText(focus, r.lang))
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
	} else if localKey != "" {
		r.applyDockStagePresentationLocked(&state, localKey, syntheticStageLabelForKey(localKey, r.lang, r.activity.kind))
	} else if key := r.topologyNextStageKeyLocked(); key != "" {
		r.applyDockStagePresentationLocked(&state, key, syntheticStageLabelForKey(key, r.lang, r.activity.kind))
	} else if fallback := r.fallbackRow(); fallback != nil {
		// focus=nil time windows: post-EventTaskNodeEnd before next
		// EventTaskNodeStart, post-EventAnalysisReady before first
		// node, multi-topic dispatch gaps, mid-extract on rare
		// stage-only paths. Without a fallback row 2 collapses to
		// "▪ —" with no stage word at all. Pick the most recently
		// finished row when one exists, otherwise the first pending
		// row, and render it through the existing label/progress
		// helpers — same vocabulary, same K/N denominator.
		//
		// Lifecycle slot: when the fallback row is a pending
		// NodeRow AND the dock activity is an active LLM call /
		// tool call, the LLM is computing FOR that next node — show
		// the running slot ("正在 X") instead of pending ("待 X")
		// so row 2 doesn't read as contradictory with row 1's
		// "请求模型中" / "调用工具中". Pending nodes still render
		// as "待 X" when no activity is in flight (genuinely queued).
		key := stageKeyFor(fallback)
		r.applyDockStagePresentationLocked(&state, key, fallbackBarPrimaryText(fallback, r.lang, r.activity.kind))
		state.topicProgress = r.topicProgressFor(fallback, r.lang)
		state.iteration = fallback.iteration
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
	state.modelID = r.requestModelID
	state.contextTokensEstimate = r.requestContextTokensEstimate
	state.contextWindowTokens = r.requestContextWindowTokens
	if r.activity.kind == activityFinalizing {
		state.streamChars = r.streamChars
	}
	return composeDockRows(state)
}

// stageProgressForFocus returns the K/N progress string for the dock
// row 2 stage column. The value of K depends on totalStages — which
// the REPL sets per mode (read=4, ModeApply=4, ModePlan=2,
// ModeVerify=2, write trio=3) — because the same stage key maps to
// a different sequence position depending on which stages actually
// run in this Run.
//
// Examples:
//
//	totalStages=4 (read) — analyze=1/4, explore-family=2/4, extract=3/4, finalize=4/4
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
		// Default matches read mode: 4 dispatch boundaries
		// (analyze + explore + extract + finalize). evidence /
		// validate / reconcile fold under explore — they share a
		// single dispatch with one start/end and have no observable
		// transition for the dock to advance through. CLI / non-TTY
		// callers that never call SetTotalStages get this default.
		total = 4
	}
	key := stageKeyFor(row)
	switch key {
	case "log_triage", "perf_triage", "write_analyze":
		return "—"
	}
	switch total {
	case 4:
		// totalStages=4 covers both:
		//   read mode      — analyze + explore + extract + finalize
		//   ModeApply mode — analyze + plan + apply + verify
		// The two modes use disjoint stage keys so a shared case is
		// unambiguous. evidence / validate / reconcile fold under
		// "explore=2/4" because they run inside the SAME explore
		// dispatch with a shared start/end — the dock has no
		// observable transition between them, so giving them
		// separate slots would be a slot count the user can never
		// see advance.
		switch key {
		case "analyze":
			return "1/4"
		// Read-mode explore-family slots collapse into 2/4.
		case "explore", "evidence", "validate", "reconcile":
			return "2/4"
		case "extract":
			return "3/4"
		case "finalize":
			return "4/4"
		// Write-mode ModeApply slots.
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

// applyDockStagePresentationLocked writes the row-2 stage fields for
// an actual execution key while keeping the visible K/N monotonic.
// When a finalizer contract failure legitimately sends work back to
// extract/explore, the user should not see the primary progress bar
// jump from 4/4 to 2/4. Instead row 2 keeps the highest reached slot
// and names the active repair sub-stage: "4/4 · 修复中：正在探索...".
//
// Caller MUST hold r.mu.
func (r *Renderer) applyDockStagePresentationLocked(state *dockRowState, key, label string) {
	if state == nil {
		return
	}
	state.stageKey = key
	progress, repairing := r.displayProgressForStageKeyLocked(key)
	state.stageProgress = progress
	if repairing && strings.TrimSpace(label) != "" {
		label = repairStageLabel(label, r.lang)
	}
	state.stageLabel = label
}

// displayProgressForStageKeyLocked returns the visible K/N for key.
// The underlying stage may move backwards during repair, but the
// visible primary progress is high-water monotonic; the label carries
// the repair detail instead.
//
// Caller MUST hold r.mu.
func (r *Renderer) displayProgressForStageKeyLocked(key string) (string, bool) {
	total := normalizedTotalStages(r.totalStages)
	slot := stageSlotForKey(key, total)
	if slot <= 0 {
		return progressForStageKey(key, total), false
	}
	maxSlot := r.maxStartedStageSlotLocked()
	if maxSlot > slot {
		return progressForSlot(maxSlot, total), true
	}
	return progressForSlot(slot, total), false
}

func syntheticStageLabelForKey(key, lang string, activity activityKind) string {
	state := stagePhrasePending
	if activityIsLive(activity) || activity == activityWaitingNode {
		state = stagePhraseRunning
	}
	return stagePhrase(key, lang, state)
}

// parallelStagePresentationLocked returns the stage surface that owns
// an active bounded fan-out. This is intentionally stronger than the
// ordinary focus/local/fallback ordering: while explore parallelism is
// still in flight, a pending extract/finalize row or high-water repair
// cursor must not make the live dock look like a sequential downstream
// stage. Later typed stage events still clear stale parallel telemetry
// via clearParallelIfStageKeyOutsideLocked.
//
// Caller MUST hold r.mu.
func (r *Renderer) parallelStagePresentationLocked(focus *taskRow, localKey string) (string, string, *taskRow) {
	if r == nil || r.parallel == nil {
		return "", "", nil
	}
	key := parallelPresentationStageKey(string(r.parallel.stage))
	if key == "" {
		return "", "", nil
	}
	label := parallelStageLabelForKey(key, r.lang)
	if focus != nil {
		focusKey := stageKeyFor(focus)
		if r.parallel.appliesToStageKey(focusKey) {
			return focusKey, liveBarPrimaryText(focus, r.lang), focus
		}
	}
	if localKey != "" && r.parallel.appliesToStageKey(localKey) {
		key = localKey
		label = parallelStageLabelForKey(key, r.lang)
	}
	return key, label, nil
}

func parallelStageLabelForKey(key, lang string) string {
	return stagePhrase(key, lang, stagePhraseRunning)
}

func parallelPresentationStageKey(stageKey string) string {
	key := canonicalStageKey(stageKey)
	if key == "explore" {
		return "evidence"
	}
	return key
}

// clearParallelIfStageKeyOutsideLocked enforces the display ownership
// boundary for parallel telemetry. Parallel dispatch state belongs to the
// stage that declared it (currently the explore family); when a later
// typed stage/node/local-work event moves the dock to another family, stale
// parallel counters must not be appended to that new stage row.
//
// Caller MUST hold r.mu.
func (r *Renderer) clearParallelIfStageKeyOutsideLocked(stageKey string) {
	if r == nil || r.parallel == nil {
		return
	}
	if r.parallel.appliesToStageKey(stageKey) {
		return
	}
	r.parallel = nil
}

func repairStageLabel(label, lang string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	if isZh(lang) {
		if label == "正在交叉验证证据" {
			return "修复中：正在校验证据和答案约束"
		}
		return "修复中：" + label
	}
	if label == "Cross-validating evidence" {
		return "Repairing: validating evidence and answer constraints"
	}
	return "Repairing: " + label
}

func normalizedTotalStages(total int) int {
	if total <= 0 {
		return 4
	}
	return total
}

func stageSlotForKey(key string, total int) int {
	key = canonicalStageKey(key)
	switch total {
	case 4:
		switch key {
		case "analyze":
			return 1
		case "explore", "evidence", "validate", "reconcile", "plan", "planner":
			return 2
		case "extract", "apply", "coder":
			return 3
		case "finalize", "verify", "verifier":
			return 4
		}
	case 3:
		switch key {
		case "plan", "planner":
			return 1
		case "apply", "coder":
			return 2
		case "verify", "verifier":
			return 3
		}
	case 2:
		switch key {
		case "analyze":
			return 1
		case "plan", "planner", "verify", "verifier":
			return 2
		}
	}
	return 0
}

func progressForSlot(slot, total int) string {
	if slot <= 0 || total <= 0 || slot > total {
		return ""
	}
	return fmt.Sprintf("%d/%d", slot, total)
}

// maxStartedStageSlotLocked returns the highest main-pipeline slot
// that has actually started. Pending TaskNode rows declared by
// EventAnalysisReady are excluded; they are future topology, not
// completed progress.
//
// Caller MUST hold r.mu.
func (r *Renderer) maxStartedStageSlotLocked() int {
	total := normalizedTotalStages(r.totalStages)
	maxSlot := 0
	for _, row := range r.tasks {
		if row == nil || row.isSubAgent {
			continue
		}
		if row.isNodeRow && row.pending && rowFirstStart(row).IsZero() && row.endTime.IsZero() {
			continue
		}
		if rowFirstStart(row).IsZero() && row.endTime.IsZero() {
			continue
		}
		if slot := stageSlotForKey(stageKeyFor(row), total); slot > maxSlot {
			maxSlot = slot
		}
	}
	return maxSlot
}

func (r *Renderer) localStagePresentationKeyLocked() string {
	key := canonicalStageKey(r.localStageKey)
	if key == "" {
		return ""
	}
	if key == "explore" && r.hasStageKeyLocked("evidence") {
		key = "evidence"
	}
	total := normalizedTotalStages(r.totalStages)
	localSlot := stageSlotForKey(key, total)
	if upstream := r.pendingUpstreamStageKeyLocked(localSlot, total); upstream != "" {
		return upstream
	}
	maxSlot := r.maxStartedStageSlotLocked()
	// Some pre-finalize local gates are emitted as StageFinalize even
	// before the mandatory stage-only extract dispatch has run. Do not
	// let such local bookkeeping leapfrog the topology on the live
	// status row; the next required user-visible stage owns the label.
	if localSlot > 0 && maxSlot > 0 && localSlot > maxSlot+1 {
		return ""
	}
	return key
}

// pendingUpstreamStageKeyLocked returns the earliest declared but not-yet-started
// node whose visible stage slot is before localSlot. Local scheduler work can be
// tagged with a downstream stage while it is preparing context for the next graph
// window; the dock must not present that downstream slot before an upstream node
// has even started. This is structural topology state only: no model/user prose is
// inspected.
//
// Caller MUST hold r.mu.
func (r *Renderer) pendingUpstreamStageKeyLocked(localSlot, total int) string {
	if localSlot <= 0 {
		return ""
	}
	bestSlot := 0
	bestKey := ""
	for _, row := range r.tasks {
		if row == nil || row.isSubAgent || !row.isNodeRow {
			continue
		}
		if !row.pending || !rowFirstStart(row).IsZero() || !row.endTime.IsZero() {
			continue
		}
		key := stageKeyFor(row)
		slot := stageSlotForKey(key, total)
		if slot <= 0 || slot >= localSlot {
			continue
		}
		if bestSlot == 0 || slot < bestSlot {
			bestSlot = slot
			bestKey = key
		}
	}
	return bestKey
}

// bindLiveEventStageLocked makes row 2 follow the typed stage carried
// by live LLM/tool/progress events. This is deliberately structural:
// prefer the event stage, and when stream/tool deltas omit it, fall
// back to the active task row. Never infer stage ownership from prose
// tails such as "接收中 ▸ ...", because those tails are noisy model/output
// content. A live explore event may arrive while the topology fallback
// already points at extract; in that window the status line should
// still name the work that is actually producing bytes right now.
//
// Caller MUST hold r.mu.
func (r *Renderer) bindLiveEventStageLocked(stage string) {
	key := canonicalStageKey(stage)
	if key == "" && r.current != nil && r.current.endTime.IsZero() && !r.current.pending {
		key = stageKeyFor(r.current)
	}
	if key == "" {
		return
	}
	r.localStageKey = key
	r.clearParallelIfStageKeyOutsideLocked(key)
}

func localStageKeyMatchesStageKey(localKey, stageKey string) bool {
	localKey = canonicalStageKey(localKey)
	stageKey = canonicalStageKey(stageKey)
	if localKey == "" || stageKey == "" {
		return false
	}
	if localKey == stageKey {
		return true
	}
	return isExploreFamilyStageKey(localKey) && isExploreFamilyStageKey(stageKey)
}

func (r *Renderer) localStageOverridesFocusLocked(focus *taskRow, localKey string) bool {
	if focus == nil || localKey == "" {
		return true
	}
	if localKey == "operation" || localKey == "data" {
		return true
	}
	total := normalizedTotalStages(r.totalStages)
	localSlot := stageSlotForKey(localKey, total)
	focusSlot := stageSlotForKey(stageKeyFor(focus), total)
	return localSlot > 0 && focusSlot > 0 && localSlot < focusSlot
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
	var glyph string
	var glyphStyle, labelStyle = statusSuccessMuted, statusPrimaryDone
	var label string
	switch errKind {
	case statusErrorFatal:
		glyph = string(glyphFatal)
		glyphStyle = statusFatal
		labelStyle = statusFatal
		label = friendlyPrimaryText(row, errKind, r.lang)
	case statusErrorCancelled:
		// User-initiated stop is NOT a system failure — same ⊘ glyph
		// + dim-gray palette as commitRowCancelled (run-end summary)
		// and statusIcon for the live spinner area, so mid-flight
		// cancelled rows do not visually mimic system failure.
		glyph = string(glyphCancelled)
		glyphStyle = statusMeta
		labelStyle = statusMeta
		label = friendlyPrimaryText(row, errKind, r.lang)
	case statusErrorRecoverable:
		glyph = string(glyphRecoverable)
		glyphStyle = statusRecoverable
		// Body intentionally dimmed (statusPrimaryDone gray) — the
		// glyph + N/K progress carry the "still working, retrying"
		// signal in yellow, while the prose body recedes to a
		// muted hue so a row of retries doesn't fill the dock with
		// loud yellow text. Pre-fix the whole label was yellow and
		// drowned out neighbouring success / objective rows.
		labelStyle = statusPrimaryDone
		label = friendlyPrimaryText(row, errKind, r.lang)
	default:
		if row.endTime.IsZero() || !row.okFinished {
			return ""
		}
		glyph = string(glyphSuccess)
		// Loaded-plan / --skip-verify short-circuit: the LLM didn't
		// run, so the regular "已 X" phrase would lie. Phrase
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
	stageElapsed := r.stageDoneElapsedString(row)
	// Topic ordinals are intentionally not rendered in the stage-
	// completion row. They read like completion progress when shown
	// beside K/N stage state, while the topic block below already
	// carries the stable per-focus labels.
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
	return b.String()
}

func (r *Renderer) stageDoneElapsedString(row *taskRow) string {
	if row == nil || row.endTime.IsZero() {
		return ""
	}
	start := rowFirstStart(row)
	if row.isNodeRow {
		progress := r.stageProgressForFocus(row)
		for _, other := range r.tasks {
			if other == nil || !other.isNodeRow {
				continue
			}
			if r.stageProgressForFocus(other) != progress {
				continue
			}
			otherStart := rowFirstStart(other)
			if otherStart.IsZero() {
				continue
			}
			if start.IsZero() || otherStart.Before(start) {
				start = otherStart
			}
		}
	}
	if start.IsZero() || row.endTime.Before(start) {
		return ""
	}
	return row.endTime.Sub(start).Truncate(time.Second).String()
}

func rowFirstStart(row *taskRow) time.Time {
	if row == nil {
		return time.Time{}
	}
	if !row.firstStart.IsZero() {
		return row.firstStart
	}
	return row.startTime
}

// formatSubTopicsBlock returns the multi-line analyzer investigation-unit
// enumeration or empty when fewer than 2 displayable units were emitted.
// Structured InvestigationUnit metadata is authoritative; the historical
// "_tN" evidence-node suffix is only a fallback for older event payloads.
// Output includes leading/trailing blank lines so commitToScrollback writes it
// as a visually framed block.
type renderedInvestigationTopic struct {
	idx     int
	text    string
	unit    types.InvestigationUnit
	hasUnit bool
}

func formatAnalysisBreakdownBlock(lang string, taskNodes []TaskNodeInfo, dimensions []AnswerDimensionInfo) string {
	var b strings.Builder
	if subBlock := formatSubTopicsBlock(lang, taskNodes); subBlock != "" {
		b.WriteString(subBlock)
	}
	if dimBlock := formatAnswerDimensionsBlock(lang, dimensions); dimBlock != "" {
		b.WriteString(dimBlock)
	}
	return b.String()
}

func formatSubTopicsBlock(lang string, taskNodes []TaskNodeInfo) string {
	var topics []renderedInvestigationTopic
	seen := map[string]bool{}
	for _, n := range taskNodes {
		if n.Type != "evidence" {
			continue
		}
		idx := len(topics)
		key := strings.TrimSpace(n.ID)
		if n.HasInvestigationUnit {
			if n.InvestigationUnit.Index > 0 {
				idx = n.InvestigationUnit.Index - 1
			} else if i, ok := topicIndexFromNodeID(n.ID); ok {
				idx = i
			}
			if unitID := strings.TrimSpace(n.InvestigationUnit.ID); unitID != "" {
				key = unitID
			} else {
				key = fmt.Sprintf("unit-%d", idx+1)
			}
		} else if i, ok := topicIndexFromNodeID(n.ID); ok {
			idx = i
		} else {
			continue
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		text := strings.TrimSpace(n.Objective)
		if n.HasInvestigationUnit {
			text = investigationUnitDisplayText(n.InvestigationUnit, text)
		}
		topics = append(topics, renderedInvestigationTopic{
			idx:     idx,
			text:    text,
			unit:    n.InvestigationUnit,
			hasUnit: n.HasInvestigationUnit,
		})
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
	header := fmt.Sprintf("分析拆分为 %d 个调查单元：", len(topics))
	if !isZh(lang) {
		header = fmt.Sprintf("Analyzer split the request into %d investigation units:", len(topics))
	}
	if allTopicsAreUserBuckets(topics) {
		header = fmt.Sprintf("分析保留了 %d 个用户分区：", len(topics))
		if !isZh(lang) {
			header = fmt.Sprintf("Analyzer kept %d user partitions:", len(topics))
		}
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
		obj := strings.TrimSpace(t.text)
		if obj == "" {
			continue
		}
		b.WriteString("    ")
		b.WriteString(statusTopicLabel.Sprint(mark))
		b.WriteString(" ")
		b.WriteString(statusTopicText.Sprint(obj))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
}

func formatAnswerDimensionsBlock(lang string, dimensions []AnswerDimensionInfo) string {
	var clean []AnswerDimensionInfo
	for _, dim := range dimensions {
		label := strings.TrimSpace(dim.Label)
		if label == "" {
			continue
		}
		if dim.Index <= 0 {
			dim.Index = len(clean) + 1
		}
		dim.Label = label
		clean = append(clean, dim)
	}
	if len(clean) == 0 {
		return ""
	}
	for i := 0; i < len(clean); i++ {
		for j := i + 1; j < len(clean); j++ {
			if clean[j].Index < clean[i].Index {
				clean[i], clean[j] = clean[j], clean[i]
			}
		}
	}
	header := fmt.Sprintf("分析拆分为 %d 个答案维度：", len(clean))
	if !isZh(lang) {
		if len(clean) == 1 {
			header = "Analyzer split the answer into 1 answer dimension:"
		} else {
			header = fmt.Sprintf("Analyzer split the answer into %d answer dimensions:", len(clean))
		}
	}
	circles := []string{"①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨", "⑩"}
	var b strings.Builder
	b.WriteString("\n  ")
	b.WriteString(statusObjective.Sprint(header))
	b.WriteString("\n")
	for i, dim := range clean {
		mark := fmt.Sprintf("%d.", i+1)
		if i >= 0 && i < len(circles) {
			mark = circles[i]
		}
		b.WriteString("    ")
		b.WriteString(statusTopicLabel.Sprint(mark))
		b.WriteString(" ")
		b.WriteString(statusTopicText.Sprint(dim.Label))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
}

func investigationUnitDisplayText(unit types.InvestigationUnit, fallback string) string {
	label := strings.TrimSpace(unit.Label)
	summary := strings.TrimSpace(unit.Summary)
	switch {
	case label != "" && summary != "" && !strings.EqualFold(label, summary):
		return label + " — " + summary
	case summary != "":
		return summary
	case label != "":
		return label
	default:
		return strings.TrimSpace(fallback)
	}
}

func allTopicsAreUserBuckets(topics []renderedInvestigationTopic) bool {
	if len(topics) == 0 {
		return false
	}
	for _, t := range topics {
		if !t.hasUnit || t.unit.AnswerPartition != types.InvestigationAnswerPartitionUserBucket {
			return false
		}
	}
	return true
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
//	多阶段方案识别到 3 个 phase：
//	  ① 添加迁移
//	  ② 更新 ORM
//	  ③ 弃用旧字段
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
// "⋯ [orchestrator-1] ..." LLM-thinking marker — semantically wrong
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
	// "Phase" → "阶段" so a zh user does not see "▶ Phase 2/3 启动"
	// — pre-fix the noun was hardcoded English regardless of language.
	var header string
	if zh {
		header = fmt.Sprintf("%s 阶段 %d/%d", icon, idx+1, total)
	} else {
		header = fmt.Sprintf("%s Phase %d/%d", icon, idx+1, total)
	}
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

// retryCountSuffix preserves the historical adapter-only helper for callers
// and tests that only know about model-request retry / provider fallback.
// runTelemetrySuffix is the newer run-summary surface that also carries typed
// answer-rework and accept-with-boundary advisory counts.
func retryCountSuffix(retries, fallbacks int, zh bool) string {
	return runTelemetrySuffix(retries, fallbacks, 0, 0, zh)
}

func runTelemetrySuffix(modelRetries, providerFallbacks, answerRewrites, advisoryAccepts int, zh bool) string {
	if modelRetries == 0 && providerFallbacks == 0 && answerRewrites == 0 && advisoryAccepts == 0 {
		return ""
	}
	parts := make([]string, 0, 4)
	if zh {
		if modelRetries > 0 {
			parts = append(parts, fmt.Sprintf("模型请求重试 %d 次", modelRetries))
		}
		if providerFallbacks > 0 {
			parts = append(parts, fmt.Sprintf("切换 provider %d 次", providerFallbacks))
		}
		if answerRewrites > 0 {
			parts = append(parts, fmt.Sprintf("答案返工 %d 次", answerRewrites))
		}
		if advisoryAccepts > 0 {
			parts = append(parts, fmt.Sprintf("带边界接受 %d 次", advisoryAccepts))
		}
		return "(" + strings.Join(parts, "，") + ")"
	}
	if modelRetries > 0 {
		if modelRetries == 1 {
			parts = append(parts, "model request retried once")
		} else {
			parts = append(parts, fmt.Sprintf("model request retried %d times", modelRetries))
		}
	}
	if providerFallbacks > 0 {
		if providerFallbacks == 1 {
			parts = append(parts, "switched provider once")
		} else {
			parts = append(parts, fmt.Sprintf("switched provider %d times", providerFallbacks))
		}
	}
	if answerRewrites > 0 {
		if answerRewrites == 1 {
			parts = append(parts, "answer reworked once")
		} else {
			parts = append(parts, fmt.Sprintf("answer reworked %d times", answerRewrites))
		}
	}
	if advisoryAccepts > 0 {
		if advisoryAccepts == 1 {
			parts = append(parts, "accepted with boundary note once")
		} else {
			parts = append(parts, fmt.Sprintf("accepted with boundary notes %d times", advisoryAccepts))
		}
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func (r *Renderer) recordOrchestratorNoticeTelemetry(kind OrchestratorNoticeKind) {
	switch kind {
	case NoticeAnswerCheckRetry,
		NoticeFallbackFinalizerOnly,
		NoticeFallbackBackToExtract,
		NoticeFallbackBackToExplore,
		NoticeSelfConsistencyContradictionRewriting:
		r.answerRewriteTotal++
	case NoticeFallbackFailLoud,
		NoticeFinalizeRepairCap,
		NoticeSelfConsistencyContradictionLogged,
		NoticeLowGrounding,
		NoticeYieldKill:
		r.answerAdvisoryTotal++
	}
}

// orchestratorNoticeStyle returns the pterm style the dock should
// use for an EventOrchestratorNotice's GLYPH based on its NoticeKind.
// Body styling is picked separately by softNoticeBodyStyle so retry-
// class notices can dim their prose body to a muted hue while keeping
// the bucket signal on the glyph + K/N progress.
//
// Buckets:
//
//	retry-class            → statusRecoverable (yellow) — actual
//	                         retry / re-run / rewrite
//	fallback-terminal-class → statusWarningMuted (yellow) — retry budget
//	                         exhausted; the answer ships with what we
//	                         have. Distinct GLYPH (·) from retry-class
//	                         (↻) so the visual still discriminates
//	                         "trying again" vs "giving up gracefully".
//	info-class             → statusMeta (dark gray) — quiet informational
//	progress-class         → statusObjective (cyan) — forward milestone
//	                         or active non-retry work
//
// Default falls through to statusMeta so a future kind added without
// a switch update is visible (gray) but not loud (no false retry hue).
func orchestratorNoticeStyle(kind OrchestratorNoticeKind) *pterm.Style {
	switch kind {
	case NoticeRetry,
		NoticeAnswerCheckRetry,
		NoticeFallbackFinalizerOnly,
		NoticeFallbackBackToExtract,
		NoticeFallbackBackToExplore,
		NoticeSelfConsistencyContradictionRewriting,
		NoticeNoToolCall:
		return statusRecoverable
	case NoticeFinalizeRepairCap:
		// P6/P7: hard-cap reached but the answer DID ship — pure
		// info-class (gray ·), not the warning yellow used by
		// FailLoud / YieldKill where the retry stopped without
		// producing a result.
		return statusMeta
	case NoticeYieldKill, NoticeFallbackFailLoud, NoticeLowGrounding:
		// Warning-muted: noteworthy but not fatal. Yellow on the glyph
		// signals "pay attention" without screaming red — the body and
		// surrounding answer/logs carry the actual content.
		return statusWarningMuted
	case NoticeForcedRead,
		NoticeFinalizing,
		NoticeSelfConsistencyStart,
		NoticeSemanticQualityReviewStart,
		NoticeProceedingWithoutExtract,
		NoticeInvestigationReady,
		NoticeRepoMapScanStart,
		NoticeRepoMapScanProgress:
		return statusObjective
	case NoticeInvestigationSupplement:
		// Missing-facts continuation is expected read-loop progress,
		// not a transport/model retry. Keep it muted so the active
		// forced-read / stage-start row owns the user's attention.
		return statusMeta
	case NoticeMultiRepoScanOK, NoticeRepoMapScanOK:
		// Match the canonical glyphSuccess (✓) / statusSuccessMuted
		// (FgGreen) pairing already used by the dock state row's
		// "stage finished OK" tick — the operator sees the same
		// green ✓ for both a completed stage and a completed sub-
		// repo scan, so the success palette stays consistent across
		// the dock chrome.
		return statusSuccessMuted
	case NoticeMultiRepoScanFail, NoticeRepoMapScanFail:
		// Match the canonical glyphFatal (✗) / statusFatal (FgRed)
		// pairing used by the dock state row's "stage finished with
		// fatal error" marker. A failed sub-repo scan is genuinely
		// fatal for that sub-repo's evidence — surfacing the red
		// keeps the colour vocabulary coherent across the dock.
		return statusFatal
	}
	return statusMeta
}

// formatOrchestratorNoticeLocked returns the dock scrollback line for
// an EventOrchestratorNotice with the current K/N stage progress
// inserted between the glyph and the body when available.
//
// Caller MUST hold r.mu.
func (r *Renderer) formatOrchestratorNoticeLocked(kind OrchestratorNoticeKind, text string, stage string) string {
	progress := ""
	if key := canonicalStageKey(stage); key != "" {
		progress = progressForStageKey(key, normalizedTotalStages(r.totalStages))
	}
	if progress == "" {
		progress = r.softNoticeStageProgressLocked()
	}
	return formatOrchestratorNoticeWithProgress(kind, text, progress)
}

// formatOrchestratorNotice keeps the old test/non-context entry point.
// Equivalent to formatOrchestratorNoticeWithProgress with empty K/N
// progress.
func formatOrchestratorNotice(kind OrchestratorNoticeKind, text string) string {
	return formatOrchestratorNoticeWithProgress(kind, text, "")
}

// formatOrchestratorNoticeWithProgress is the underlying formatter.
// Layout policy:
//
//   - 2-space leading indent (mirrors formatReasoning's "  " indent
//     so the column-1 alignment with surrounding rows is preserved)
//   - The message body carries its own kind glyph (↻ / › / · / ⊘) chosen
//     by internal/orchestrator/user_messages.go. The glyph keeps the
//     bucket colour (yellow / gray / cyan); for retry-class notices
//     we ALSO splice the current K/N stage progress between glyph
//     and body and dim the prose body to statusPrimaryDone so a
//     row of retries doesn't fill the dock with loud yellow text.
//
// Empty text → empty output so the caller can degrade silently.
func formatOrchestratorNoticeWithProgress(kind OrchestratorNoticeKind, text, progress string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	style := orchestratorNoticeStyle(kind)
	// Split the leading kind glyph (single rune followed by a space)
	// from the prose body. user_messages.go produces "↻ <text>" /
	// "› <text>" / "· <text>" / "⊘ <text>" for soft notices. When the head is a
	// single rune + space, peel it so we can colour the glyph + the
	// optional K/N progress in the bucket palette but dim the prose.
	glyph, body := peelGlyphPrefix(text)
	if glyph == "" {
		// No recognisable glyph head — fall back to pre-split styling.
		return "  " + style.Sprint(text)
	}
	bodyStyle := softNoticeBodyStyle(kind)
	var b strings.Builder
	b.WriteString("  ")
	b.WriteString(style.Sprint(glyph))
	if progress != "" {
		// K/N progress shares the body palette (gray) — only the
		// glyph carries the bucket colour. Pre-fix the progress was
		// styled with the bucket palette and read as a second
		// loud-yellow segment alongside the glyph.
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint(progress))
	}
	if body != "" {
		b.WriteString(" ")
		b.WriteString(bodyStyle.Sprint(body))
	}
	return b.String()
}

// peelGlyphPrefix splits a soft-notice text into its leading glyph
// (one rune, one of ↻ / › / · / ⊘ / ✓ / ✗) and the rest of the body. The
// glyph and trailing space are stripped from the body. Returns
// ("", text) when the head is not a recognised glyph + space.
func peelGlyphPrefix(text string) (glyph, body string) {
	if text == "" {
		return "", ""
	}
	// Walk one rune.
	for _, r := range text {
		switch r {
		case glyphRecoverable, glyphProgress, glyphCancelled, glyphWarning,
			glyphSuccess, glyphFatal:
			rest := strings.TrimPrefix(text, string(r))
			rest = strings.TrimPrefix(rest, " ")
			return string(r), rest
		}
		break
	}
	return "", text
}

// softNoticeBodyStyle picks the prose-body palette for an orchestrator
// soft notice. Retry-class buckets (the yellow ones) dim their body
// to statusPrimaryDone so a stack of retries does not fill the dock
// with loud yellow text — the glyph + K/N progress carry the bucket
// signal, the body recedes. Fallback-terminal-class keeps the body
// at statusMeta (the prior default) so only the glyph picks up the
// new yellow tint. info-class and progress-class buckets keep the
// bucket palette throughout (gray and cyan are already mild).
func softNoticeBodyStyle(kind OrchestratorNoticeKind) *pterm.Style {
	switch kind {
	case NoticeRetry,
		NoticeAnswerCheckRetry,
		NoticeFallbackFinalizerOnly,
		NoticeFallbackBackToExtract,
		NoticeFallbackBackToExplore,
		NoticeSelfConsistencyContradictionRewriting,
		NoticeNoToolCall:
		return statusPrimaryDone
	case NoticeYieldKill, NoticeFallbackFailLoud, NoticeLowGrounding:
		// Warning-muted: glyph picks up the yellow tint; the prose body
		// stays at statusMeta so the row nudges the eye without
		// overwhelming ordinary progress.
		return statusMeta
	}
	return orchestratorNoticeStyle(kind)
}

// softNoticeStageProgressLocked returns the K/N stage progress label
// for the row best representing the current dispatch — falls back to
// focusRow / fallbackRow / current. Empty when no row is identifiable
// (e.g. light routes that bypass the pipeline).
//
// Caller MUST hold r.mu.
func (r *Renderer) softNoticeStageProgressLocked() string {
	if row := r.focusRow(); row != nil {
		progress, _ := r.displayProgressForStageKeyLocked(stageKeyFor(row))
		return progress
	}
	if key := r.localStagePresentationKeyLocked(); key != "" {
		progress, _ := r.displayProgressForStageKeyLocked(key)
		return progress
	}
	// Soft-notice gap handling: between two sequential stage
	// dispatches there is a brief window where r.current is nil and
	// no NodeRow is in flight. The generic fallbackRow path then
	// returns the FIRST pending NodeRow — but read mode's TaskGraph
	// has no `extract` NodeRow (extract is a stage-only step), so
	// after explore ends the first-pending NodeRow is `finalize` and
	// the soft notice K/N would render as 4/4. Architecturally the
	// imminent next dispatch is extract → 3/4. Compute K via the
	// stage-topology cursor instead so the notice does not leapfrog
	// stages that have no NodeRow representation.
	if progress := r.topologyNextStageProgressLocked(); progress != "" {
		return progress
	}
	if row := r.fallbackRow(); row != nil {
		progress, _ := r.displayProgressForStageKeyLocked(stageKeyFor(row))
		return progress
	}
	if r.current != nil {
		progress, _ := r.displayProgressForStageKeyLocked(stageKeyFor(r.current))
		return progress
	}
	return ""
}

// topologyNextStageProgressLocked returns the visible K/N progress
// label for the topologically-next stage. See topologyNextStageKeyLocked.
//
// Caller MUST hold r.mu.
func (r *Renderer) topologyNextStageProgressLocked() string {
	next := r.topologyNextStageKeyLocked()
	if next == "" {
		return ""
	}
	progress, _ := r.displayProgressForStageKeyLocked(next)
	return progress
}

// topologyNextStageKeyLocked returns the next required stage key from
// the pipeline topology, not from the first pending TaskNode. This is
// load-bearing for read mode because extract is stage-only: after the
// evidence nodes complete, the only pending TaskNode may be finalize,
// but the real next dispatch is extract. Picking pending finalize here
// makes the live dock jump 2/4 → 4/4 → 3/4.
//
// Caller MUST hold r.mu.
func (r *Renderer) topologyNextStageKeyLocked() string {
	total := normalizedTotalStages(r.totalStages)
	maxSlot := r.maxStartedStageSlotLocked()
	if maxSlot <= 0 {
		if latest := r.latestEndedPreStageKeyLocked(); latest == "log_triage" || latest == "perf_triage" {
			return "analyze"
		}
		return ""
	}
	nextSlot := maxSlot + 1
	if nextSlot > total {
		return ""
	}
	return r.stageKeyForTopologySlotLocked(nextSlot)
}

func (r *Renderer) latestEndedPreStageKeyLocked() string {
	var newestKey string
	var newestTime time.Time
	for _, row := range r.tasks {
		if row == nil || row.isSubAgent || row.isNodeRow || row.endTime.IsZero() {
			continue
		}
		key := canonicalStageKey(row.stage)
		if key != "log_triage" && key != "perf_triage" {
			continue
		}
		if row.endTime.After(newestTime) {
			newestKey = key
			newestTime = row.endTime
		}
	}
	return newestKey
}

func (r *Renderer) stageKeyForTopologySlotLocked(slot int) string {
	total := normalizedTotalStages(r.totalStages)
	if r.usesWriteModeSlotsLocked() {
		switch total {
		case 4:
			switch slot {
			case 1:
				return "analyze"
			case 2:
				return "plan"
			case 3:
				return "apply"
			case 4:
				return "verify"
			}
		case 3:
			switch slot {
			case 1:
				return "plan"
			case 2:
				return "apply"
			case 3:
				return "verify"
			}
		case 2:
			switch slot {
			case 1:
				return "analyze"
			case 2:
				if r.hasStageKeyLocked("verify") {
					return "verify"
				}
				return "plan"
			}
		}
		return ""
	}
	switch total {
	case 4:
		switch slot {
		case 1:
			return "analyze"
		case 2:
			return "explore"
		case 3:
			return "extract"
		case 4:
			return "finalize"
		}
	}
	return ""
}

func (r *Renderer) usesWriteModeSlotsLocked() bool {
	if normalizedTotalStages(r.totalStages) == 3 {
		return true
	}
	for _, row := range r.tasks {
		key := stageKeyFor(row)
		switch key {
		case "plan", "apply", "verify":
			return true
		}
	}
	return false
}

func (r *Renderer) hasStageKeyLocked(want string) bool {
	want = canonicalStageKey(want)
	for _, row := range r.tasks {
		if stageKeyFor(row) == want {
			return true
		}
	}
	return false
}

// progressForStageKey returns the K/N string for a stage key using
// the same slot mapping as stageProgressForFocus. Empty when the
// key is not in the slot table for the given total. Read mode uses
// total==4 with the analyze/explore/extract/finalize layout; write
// mode (ModeApply) also uses total==4 with analyze/plan/apply/verify.
func progressForStageKey(key string, total int) string {
	switch total {
	case 4:
		switch key {
		case "analyze":
			return "1/4"
		case "explore", "evidence", "validate", "reconcile", "plan":
			// `plan` shares slot 2 in ModeApply (analyze=1/4, plan=2/4).
			// Read mode never sees plan and write mode never sees
			// explore-family keys, so the unified case is unambiguous.
			return "2/4"
		case "extract", "apply":
			return "3/4"
		case "finalize", "verify":
			return "4/4"
		}
	case 3:
		// Write trio (skip analyze) — plan/apply/verify only.
		switch key {
		case "plan":
			return "1/3"
		case "apply":
			return "2/3"
		case "verify":
			return "3/3"
		}
	case 2:
		// Plan-only / verify-only (analyze + 1 stage).
		switch key {
		case "analyze":
			return "1/2"
		case "plan", "verify":
			return "2/2"
		}
	}
	return ""
}

// countTopicSiblings returns how many evidence_tN nodes are present
// in r.tasks. Used by commit to decide whether the ordinal focus
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

func (r *Renderer) parallelUnitTraceLabelLocked(ev Event) string {
	if r == nil || r.parallel == nil || !r.parallel.matches(ev) {
		return ""
	}
	ordinal, _, ok := r.parallel.unitOrdinal(ev.ParallelUnitID)
	if !ok || ordinal <= 0 {
		return ""
	}
	lane := parallelLaneLabelsInlinePhrase(r.parallel.unitLaneLabels(ev.ParallelUnitID), r.lang)
	if isZh(r.lang) {
		if lane != "" {
			return fmt.Sprintf("第 %d 路（%s）", ordinal, lane)
		}
		return fmt.Sprintf("第 %d 路", ordinal)
	}
	if lane != "" {
		return fmt.Sprintf("Lane %d (%s)", ordinal, lane)
	}
	return fmt.Sprintf("Lane %d", ordinal)
}

func (r *Renderer) activityTraceUnitLabelLocked(ev Event) string {
	if label := r.parallelUnitTraceLabelLocked(ev); label != "" {
		return label
	}
	return focusedExploreDispatchTraceLabel(ev, r.lang)
}

func focusedExploreDispatchTraceLabel(ev Event, lang string) string {
	if ev.Stage != types.StageExplore || ev.Agent != types.AgentExplorer {
		return ""
	}
	switch types.TaskNodeType(strings.TrimSpace(ev.DispatchKind)) {
	case types.NodeReconcile:
		if isZh(lang) {
			return "汇总"
		}
		return "Reconcile"
	case types.NodeValidate:
		if isZh(lang) {
			return "校验"
		}
		return "Validate"
	case types.NodeProbe:
		if isZh(lang) {
			return "探查"
		}
		return "Probe"
	default:
		return ""
	}
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
		if r.nonTTYStageOwnedByTaskNode(ev) {
			break
		}
		r.emitNonTTYLine(fmt.Sprintf("→ %s", r.nonTTYStageLabel(string(ev.Stage), stagePhraseRunning)))
	case EventStageEnd:
		if r.nonTTYStageOwnedByTaskNode(ev) {
			break
		}
		if ev.Error != "" {
			glyph, state := nonTTYEndErrorShape(ev.Error)
			r.emitNonTTYLine(fmt.Sprintf("%s %s · %s", glyph, r.nonTTYStageLabel(string(ev.Stage), state), ev.Error))
		} else {
			r.emitNonTTYLine(fmt.Sprintf("✓ %s", r.nonTTYStageLabel(string(ev.Stage), stagePhraseDone)))
		}
	case EventTaskNodeStart:
		r.emitNonTTYLine(fmt.Sprintf("→ %s", r.nonTTYTaskNodeLabel(ev, stagePhraseRunning)))
	case EventTaskNodeEnd:
		if ev.Error != "" {
			glyph, state := nonTTYEndErrorShape(ev.Error)
			r.emitNonTTYLine(fmt.Sprintf("%s %s · %s", glyph, r.nonTTYTaskNodeLabel(ev, state), ev.Error))
		} else {
			r.emitNonTTYLine(fmt.Sprintf("✓ %s", r.nonTTYTaskNodeLabel(ev, stagePhraseDone)))
		}
	case EventAnalysisReady:
		if row := r.findFinishedStageRowAt("analyze", ev.Timestamp); row != nil {
			r.emitNonTTYLine(fmt.Sprintf("✓ %s", r.nonTTYStageLabel(row.stage, stagePhraseDone)))
		}
		if block := formatAnalysisBreakdownBlock(r.lang, ev.TaskNodes, ev.AnswerDimensions); block != "" {
			fmt.Fprint(r.outputWriter(), block)
			mirrorDockBlockToLog(block)
		}
	case EventAgentReasoning:
		r.emitNonTTYLines(formatReasoningLinesWithParallel(string(ev.Agent), ev.Stage, ev.Iteration, ev.Reasoning, r.thinkingTruncate, r.lang, r.activityTraceUnitLabelLocked(ev)))
	case EventAgentToolCallBatch:
		if line := formatUnavailableToolCallBatchWithParallel(string(ev.Agent), ev.Stage, ev.Iteration, ev.ToolNames,
			ev.ToolCallCount, ev.ToolName, ev.ToolDetail, r.lang, r.activityTraceUnitLabelLocked(ev), ev.UnavailableToolNames); line != "" {
			r.emitNonTTYLine(line)
		}
	case EventToolCallEnd:
		if lines := r.answerDraftPreviewLinesOnToolEnd(ev); len(lines) > 0 {
			r.emitNonTTYLines(lines)
		}
		if shouldRenderStructuredToolSummary(ev) {
			if block := formatStructuredToolResultSummary(ev.ToolName, ev.ToolParamsJSON, ev.ToolResultSummary, r.lang, ev.EvidenceTotal); block != "" {
				fmt.Fprint(r.outputWriter(), stripAnsiEscapes(block))
				mirrorDockBlockToLog(block)
			}
		}
	case EventOrchestratorNotice:
		// Mirror of the TTY branch above: render WITHOUT the
		// LLM-thinking prefix or stage-round trace label so log scrapers and
		// CI operators read the same "this is the orchestrator
		// talking, not the LLM" cue.
		if line := r.formatOrchestratorNoticeLocked(ev.NoticeKind, ev.Reasoning, string(ev.Stage)); line != "" {
			r.emitNonTTYLine(line)
		}
	case EventAgentLLMWaiting:
		// Non-TTY runs have no live dock at all, so these durable
		// heartbeat lines are the ONLY in-flight signal that a slow
		// model request is still alive — exactly the surface that was
		// fully silent in the 2026-07-03 customer session. The
		// power-of-two WaitTick rate limit already fired in
		// handleEvent (single gate site) before this branch is
		// reached.
		r.emitNonTTYLine(stripAnsiEscapes(formatLLMWaitHeartbeatLine(string(ev.Agent), ev.Stage, ev.Iteration,
			ev.WaitElapsed, ev.ModelID, r.lang, r.activityTraceUnitLabelLocked(ev))))
	case EventAdapterRetry:
		// Non-TTY mode (CI / piped stdout) — same user-facing
		// language as the dock activity row + permanent commit so
		// log scrapers and operators see consistent phrasing.
		// "重新请求模型" / "retrying model request" makes the
		// retry SUBJECT explicit (the LLM call, not a tool retry
		// or stage retry).
		if isZh(r.lang) {
			r.emitNonTTYLine(fmt.Sprintf("%s 正在重新请求模型 (第 %d 次,等 %v) · %s",
				RecoverableNoticeGlyph, ev.RetryAttempt, ev.RetryDelay, localizeRetryReason(ev.RetryReason, r.lang)))
		} else {
			r.emitNonTTYLine(fmt.Sprintf("%s retrying model request (attempt %d, in %v) · %s",
				RecoverableNoticeGlyph, ev.RetryAttempt, ev.RetryDelay, localizeRetryReason(ev.RetryReason, r.lang)))
		}
	case EventAdapterFallback:
		if isZh(r.lang) {
			r.emitNonTTYLine(fmt.Sprintf("%s 切换 LLM 服务 %s → %s · %s",
				RecoverableNoticeGlyph, ev.FallbackFrom, ev.FallbackTo, localizeRetryReason(ev.RetryReason, r.lang)))
		} else {
			r.emitNonTTYLine(fmt.Sprintf("%s switching LLM provider %s → %s · %s",
				RecoverableNoticeGlyph, ev.FallbackFrom, ev.FallbackTo, localizeRetryReason(ev.RetryReason, r.lang)))
		}
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
		// icon ▶/✓/✗ instead of the thinking glyph used
		// by EventAgentReasoning. Detail surfaces phase goal
		// (start) or rejection reasoning (rejected).
		r.emitNonTTYLine(formatPhaseProgressLine(r.lang, ev.PhaseIndex, ev.PhaseTotal,
			ev.PhaseProgressKind, ev.PhaseDetail))
	}
}

func shouldRenderStructuredToolSummary(ev Event) bool {
	if ev.ToolOK {
		return true
	}
	switch strings.TrimSpace(ev.ToolName) {
	case "emit_answer_document", "emit_answer_document_patch":
		return true
	default:
		return false
	}
}

func (r *Renderer) nonTTYStageLabel(stage string, state stagePhraseState) string {
	return stagePhrase(canonicalStageKey(stage), r.lang, state)
}

// nonTTYEndErrorShape picks the glyph + phrase column for an
// EventStageEnd / EventTaskNodeEnd that carries an Error on the
// non-TTY surface. Severity comes from classifyEventError — the SAME
// classifier the TTY dock uses for the same two events (EventStageEnd
// / EventTaskNodeEnd handlers in handleEvent + classifyStatusError on
// the commit line) — so the two surfaces can never disagree on
// whether an error reads as terminal.
//
// Pre-fix this path hardcoded "✗ <failed phrase>" for EVERY error:
// a transient analyze timeout printed "✗ 未能理解问题 · context
// deadline exceeded" immediately followed by the orchestrator's
// "↻ … 连接/流式响应异常，正在重试模型请求" notice — the ✗ read as a
// terminal failure even though the very next line said the system was
// retrying (berlin.systrace customer session, audit H21). Per the
// contract documented on classifyStatusError, dispatch-level errors
// whose message carries no fatal/cancelled marker default to the
// recoverable shape (↻ + stagePhraseRetry) because the orchestrator's
// retry decision is structural, not string-keyed; the terminal ✗ for
// a run that truly cannot continue still arrives via the run-end
// failure surface (MarkRunFatal / CLI error print), which is the only
// authoritative "no retry budget left" signal.
func nonTTYEndErrorShape(errMsg string) (glyph string, state stagePhraseState) {
	switch classifyEventError(errMsg) {
	case statusErrorFatal:
		return string(glyphFatal), stagePhraseFailed
	case statusErrorCancelled:
		// User-initiated stop is NOT a system failure — same ⊘ the
		// TTY dock uses (statusIcon / commitRowCancelled). Label keeps
		// the failed column: "未能 X" + ⊘ agree on "did not finish".
		return string(glyphCancelled), stagePhraseFailed
	default:
		return string(glyphRecoverable), stagePhraseRetry
	}
}

func (r *Renderer) nonTTYStageOwnedByTaskNode(ev Event) bool {
	if !r.analysisReady || ev.Stage == "" {
		return false
	}
	stage := string(ev.Stage)
	return stage != "analyze" && stage != "extract"
}

func (r *Renderer) nonTTYTaskNodeLabel(ev Event, state stagePhraseState) string {
	kind := strings.TrimSpace(ev.NodeKind)
	if kind == "" && ev.NodeID != "" {
		if row := r.findNodeRow(ev.NodeID); row != nil {
			kind = row.nodeKind
		}
	}
	return stagePhrase(canonicalStageKey(kind), r.lang, state)
}

func (r *Renderer) findFinishedStageRowAt(stage string, ts time.Time) *taskRow {
	for i := len(r.tasks) - 1; i >= 0; i-- {
		row := r.tasks[i]
		if row == nil || row.isNodeRow || row.isSubAgent {
			continue
		}
		if row.stage != stage {
			continue
		}
		if row.endTime.Equal(ts) {
			return row
		}
	}
	return nil
}

// emitNonTTYLine writes a single line to stdout AND mirrors it to
// the INFO log so non-TTY runs still produce a complete audit
// trail without the operator having to scrape stdout.
func (r *Renderer) emitNonTTYLine(line string) {
	if line == "" {
		return
	}
	if line == r.lastCommittedLine {
		return
	}
	r.lastCommittedLine = line
	fmt.Fprintln(r.outputWriter(), line)
	mirrorDockLineToLog(line)
}

func (r *Renderer) emitNonTTYLines(lines []scrollbackLine) {
	for _, line := range lines {
		r.emitNonTTYLine(stripAnsiEscapes(line.text))
	}
}

func (r *Renderer) answerDraftPreviewLinesOnToolEnd(ev Event) []scrollbackLine {
	toolName := strings.TrimSpace(ev.ToolName)
	if r.answerDraftPreviewEmitted {
		return nil
	}
	if toolName == "emit_answer_document_patch" {
		return r.takePendingAnswerDraftPreview()
	}
	if toolName != "emit_answer_document" {
		return nil
	}
	if len(r.pendingAnswerDraftPreview) > 0 {
		return r.takePendingAnswerDraftPreview()
	}
	lines := formatAnswerDocumentDraftPreviewLines(ev.ToolParamsJSON, r.lang)
	if len(lines) == 0 {
		return nil
	}
	r.pendingAnswerDraftPreview = cloneScrollbackLines(lines)
	if ev.ToolOK {
		return nil
	}
	return r.takePendingAnswerDraftPreview()
}

func (r *Renderer) answerDraftPreviewLinesOnPreviewClear(ev Event) []scrollbackLine {
	if ev.PreviewRejected {
		return r.takePendingAnswerDraftPreview()
	}
	r.pendingAnswerDraftPreview = nil
	return nil
}

func (r *Renderer) takePendingAnswerDraftPreview() []scrollbackLine {
	if r.answerDraftPreviewEmitted || len(r.pendingAnswerDraftPreview) == 0 {
		return nil
	}
	lines := cloneScrollbackLines(r.pendingAnswerDraftPreview)
	r.pendingAnswerDraftPreview = nil
	r.answerDraftPreviewEmitted = true
	return lines
}

func cloneScrollbackLines(in []scrollbackLine) []scrollbackLine {
	if len(in) == 0 {
		return nil
	}
	out := make([]scrollbackLine, len(in))
	copy(out, in)
	return out
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
		var bodyStyle = statusFatal
		switch {
		case r.activity.kind == activityCancelled:
			rowKind = commitRowCancelled
			// User-initiated stop is not a system failure — paint the
			// summary head in muted gray so it visually reads as "we
			// stopped because you asked", not "something went wrong".
			bodyStyle = statusMeta
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
				head = "Failed"
			}
		}
		var body strings.Builder
		body.WriteString(head)
		body.WriteString(" ")
		body.WriteString(statusMeta.Sprint("·"))
		body.WriteString(" ")
		body.WriteString(statusMeta.Sprint(totalElapsedPhrase(totalElapsed, r.lang)))
		// Append a compact typed telemetry suffix when the run had model
		// request retries, provider fallback, semantic answer rework, or
		// accept-with-boundary advisory outcomes. These buckets are separate:
		// transport backoff is not answer rewrite, and advisory acceptance is
		// not a retry. The suffix is dim gray so it does not compete with the
		// summary head.
		if suffix := runTelemetrySuffix(r.adapterRetryTotal, r.adapterFallbackTotal, r.answerRewriteTotal, r.answerAdvisoryTotal, zh); suffix != "" {
			body.WriteString(" ")
			body.WriteString(statusMeta.Sprint(suffix))
		}
		line := formatCommitRow(commitRow{
			kind: rowKind,
			body: bodyStyle.Sprint(body.String()),
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
	// completed counts STAGE rows only (analyze / explore / extract /
	// finalize / plan / apply / verify). Sub-node rows (isNodeRow,
	// emitted via EventAnalysisReady's TaskNodes payload — probe /
	// evidence / validate / chain) and sub-agent rows (isSubAgent —
	// EventSubAgentStart) are dock-UX artefacts of the topology DAG,
	// not independent pipeline stages, so counting them here reads
	// back to the operator as "this Run had 6 stages" when there are
	// only 4 real stages plus 2 explore-family sub-nodes. The other
	// loops in this file (e.g. line 240) already filter via
	// "row.isSubAgent || row.isNodeRow continue" — this counter was
	// the last hold-out.
	//
	// totalTools (sum) and totalIters (max) intentionally aggregate
	// across ALL rows: each tool call increments exactly one current
	// row's toolCount (sum = unique-call count), and per-row
	// iteration tracks each row's ReAct depth so max(iter) reads as
	// "deepest ReAct chain seen in this Run" — both metrics stay
	// semantically correct.
	completed := 0
	totalTools := 0
	totalIters := 0
	for _, row := range r.tasks {
		if row == nil {
			continue
		}
		if !row.isNodeRow && !row.isSubAgent && !row.endTime.IsZero() {
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
var reAnsiCheck = regexp.MustCompile(`\x1b\[[0-9;:?]*[A-Za-z]`)
