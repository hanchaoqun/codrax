package render

import (
	"fmt"
	"io"
	"os"
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
		if r.analysisReady && ev.Stage != "" && string(ev.Stage) != "analyze" {
			break
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
		if r.analysisReady && ev.Stage != "" && string(ev.Stage) != "analyze" {
			break
		}
		if row := r.findRunningStageRow(string(ev.Stage), string(ev.Agent)); row != nil {
			row.endTime = ev.Timestamp
			row.errorMsg = ev.Error
			row.okFinished = ev.Error == ""
			if row == r.current {
				r.current = nil
				r.activity = activityState{kind: activityWaitingDispatch}
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
			row.pending = false
			row.paused = false
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
			topicTotal := r.countTopicSiblings()
			if row == r.current {
				r.current = nil
				r.activity = activityState{kind: activityWaitingDispatch}
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
		if r.current != nil {
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
		if r.current != nil {
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
	if !previewIsTTY() {
		return
	}
	if ev.PreviewText == "" {
		return
	}
	if r.previewArea == nil {
		r.previewArea = newTTYPreviewArea(os.Stdout)
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
	}
	if r.activity.kind == activityFinalizing {
		state.streamChars = r.streamChars
	}
	return composeDockRows(state)
}

// stageProgressForFocus is the totalStages-aware variant of
// stageProgressFor. write-mode (totalStages=3) renders 1/3, 2/3, 3/3
// with planner / coder / verifier; read-mode renders 1/6 .. 6/6 over
// the analyze→finalize chain.
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
	switch stageKeyFor(row) {
	case "analyze":
		return fmt.Sprintf("1/%d", total)
	case "explore", "evidence":
		return fmt.Sprintf("2/%d", total)
	case "validate":
		return fmt.Sprintf("3/%d", total)
	case "reconcile":
		return fmt.Sprintf("4/%d", total)
	case "extract":
		return fmt.Sprintf("5/%d", total)
	case "finalize":
		return fmt.Sprintf("6/%d", total)
	case "plan", "planner":
		return fmt.Sprintf("1/%d", total)
	case "apply", "coder":
		return fmt.Sprintf("2/%d", total)
	case "verify", "verifier":
		return fmt.Sprintf("3/%d", total)
	case "log_triage", "perf_triage":
		return "—"
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
		label = stagePhraseDoneFor(stageKeyFor(row), r.lang)
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
		if ev.Objective != "" && r.objective == "" {
			r.objective = ev.Objective
			emitNonTTYLine(fmt.Sprintf("❯ %s", ev.Objective))
		}
	case EventStageStart:
		emitNonTTYLine(fmt.Sprintf("→ %s", string(ev.Stage)))
	case EventStageEnd:
		if ev.Error != "" {
			emitNonTTYLine(fmt.Sprintf("✗ %s · %s", string(ev.Stage), ev.Error))
		} else {
			emitNonTTYLine(fmt.Sprintf("✓ %s", string(ev.Stage)))
		}
	case EventTaskNodeStart:
		emitNonTTYLine(fmt.Sprintf("→ %s · %s", ev.NodeKind, ev.NodeObjective))
	case EventTaskNodeEnd:
		if ev.Error != "" {
			emitNonTTYLine(fmt.Sprintf("✗ %s · %s", ev.NodeKind, ev.Error))
		} else {
			emitNonTTYLine(fmt.Sprintf("✓ %s", ev.NodeKind))
		}
	case EventAnalysisReady:
		if block := formatSubTopicsBlock(r.lang, ev.TaskNodes); block != "" {
			fmt.Fprint(os.Stdout, block)
			mirrorDockBlockToLog(block)
		}
	case EventAgentReasoning:
		emitNonTTYLine(formatReasoning(string(ev.Agent), ev.Iteration, ev.Reasoning))
	case EventAdapterRetry:
		emitNonTTYLine(fmt.Sprintf("⟳ retry #%d in %v · %s",
			ev.RetryAttempt, ev.RetryDelay, ev.RetryReason))
	case EventAdapterFallback:
		emitNonTTYLine(fmt.Sprintf("⟳ fallback %s → %s · %s",
			ev.FallbackFrom, ev.FallbackTo, ev.RetryReason))
	}
}

// emitNonTTYLine writes a single line to stdout AND mirrors it to
// the INFO log so non-TTY runs still produce a complete audit
// trail without the operator having to scrape stdout.
func emitNonTTYLine(line string) {
	if line == "" {
		return
	}
	fmt.Fprintln(os.Stdout, line)
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
	zh := isZh(r.lang)
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

// detectStdoutTTY is a thin wrapper used at StartSpinner time so
// the renderer doesn't have to import term inside hot paths.
func detectStdoutTTY() bool {
	return isTTY(os.Stdout)
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
