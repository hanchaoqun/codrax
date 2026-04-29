package render

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// focusRow picks the row whose state the live ticker should
// display this frame. Caller MUST hold r.mu.
//
// Selection rules (in priority order):
//   1. r.current if it's a NodeRow that's actively running and
//      no upstream evidence_t* row is still in flight
//   2. Otherwise, the first NodeRow whose endTime is zero,
//      pending is false, AND no upstream evidence_t* is still
//      in flight (Symptom 2 fix: prevents "validate shows
//      before evidence done")
//   3. Otherwise, the first stage row that's running (analyze /
//      log_triage / perf_triage)
//   4. Otherwise, nil — composeLiveBar then renders the
//      "preparing" placeholder
func (r *Renderer) focusRow() *taskRow {
	upstreamEvidenceInFlight := false
	for _, row := range r.tasks {
		if !row.isNodeRow || row.nodeKind != "evidence" {
			continue
		}
		if row.pending || !row.endTime.IsZero() {
			continue
		}
		upstreamEvidenceInFlight = true
		break
	}
	isDownstream := func(kind string) bool {
		switch kind {
		case "validate", "reconcile", "extract", "finalize":
			return true
		}
		return false
	}
	// Rule 1: prefer r.current unless it's prematurely-started
	// downstream while evidence still in flight.
	if r.current != nil && r.current.endTime.IsZero() && !r.current.pending {
		if !(r.current.isNodeRow && isDownstream(r.current.nodeKind) && upstreamEvidenceInFlight) {
			return r.current
		}
	}
	// Rule 2: scan task list for first valid running row.
	for _, row := range r.tasks {
		if !row.isNodeRow {
			continue
		}
		if row.pending || !row.endTime.IsZero() {
			continue
		}
		if isDownstream(row.nodeKind) && upstreamEvidenceInFlight {
			continue
		}
		return row
	}
	// Rule 3: most-recent running stage row (pre-task-graph).
	for i := len(r.tasks) - 1; i >= 0; i-- {
		row := r.tasks[i]
		if row.isSubAgent || row.isNodeRow {
			continue
		}
		if !row.endTime.IsZero() {
			continue
		}
		return row
	}
	return nil
}

// stageProgressFor returns "K/N" stage progress for the focus.
// "—" for pre-stages. Empty when stage is unknown.
func stageProgressFor(row *taskRow) string {
	if row == nil {
		return ""
	}
	const totalStages = 6
	switch stageKeyFor(row) {
	case "analyze":
		return fmt.Sprintf("1/%d", totalStages)
	case "explore", "evidence":
		return fmt.Sprintf("2/%d", totalStages)
	case "validate":
		return fmt.Sprintf("3/%d", totalStages)
	case "reconcile":
		return fmt.Sprintf("4/%d", totalStages)
	case "extract":
		return fmt.Sprintf("5/%d", totalStages)
	case "finalize":
		return fmt.Sprintf("6/%d", totalStages)
	case "log_triage", "perf_triage":
		return "—"
	}
	return ""
}

// topicProgressFor returns "关注点 K/M" when the focus is one of
// multiple evidence_tN siblings. Empty otherwise.
func (r *Renderer) topicProgressFor(focus *taskRow, lang string) string {
	if focus == nil || !focus.isNodeRow || focus.nodeKind != "evidence" {
		return ""
	}
	idx, ok := topicIndexFromNodeID(focus.nodeID)
	if !ok {
		return ""
	}
	total := 0
	for _, row := range r.tasks {
		if !row.isNodeRow || row.nodeKind != "evidence" {
			continue
		}
		if _, has := topicIndexFromNodeID(row.nodeID); has {
			total++
		}
	}
	if total < 2 {
		return ""
	}
	if isZh(lang) {
		return fmt.Sprintf("关注点 %d/%d", idx+1, total)
	}
	return fmt.Sprintf("focus %d/%d", idx+1, total)
}

// printSubTopicsToScrollback prints the sub-topic enumeration
// once when EventAnalysisReady carries multiple evidence_tN
// nodes. Each topic gets a circled-digit prefix (① ② ③) in
// LightCyan and the body in default text. PERMANENT scrollback
// record — the user can scroll back even after the live bar has
// cycled through stages.
func printSubTopicsToScrollback(w io.Writer, lang string, taskNodes []TaskNodeInfo) {
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
		return
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
	fmt.Fprintf(w, "\n  %s\n", statusObjective.Sprint(header))
	circles := []string{"①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨", "⑩"}
	for _, t := range topics {
		mark := fmt.Sprintf("%d.", t.idx+1)
		if t.idx >= 0 && t.idx < len(circles) {
			mark = circles[t.idx]
		}
		obj := strings.TrimSpace(t.objective)
		if obj == "" {
			continue
		}
		fmt.Fprintf(w, "    %s %s\n", statusObjective.Sprint(mark), statusDetail.Sprint(obj))
	}
	fmt.Fprintln(w)
}

// printStageCompletionToScrollback emits a durable single line
// for each stage that finishes. Format:
//
//   ✓ K/N 已 X · elapsed [· 第 N 轮] [· M 工具] [· 关注点 K/M]
//
// Errors surface as ✗ red instead. topicTotal > 1 enables the
// "关注点 K/M" suffix for multi-topic evidence rows.
func printStageCompletionToScrollback(w io.Writer, lang string, row *taskRow, topicTotal int) {
	if row == nil {
		return
	}
	errKind := classifyStatusError(row)
	var glyph, label string
	var glyphStyle, labelStyle = statusSuccessMuted, statusPrimaryDone
	switch errKind {
	case statusErrorFatal, statusErrorCancelled:
		glyph = string(glyphFatal)
		glyphStyle = statusFatal
		labelStyle = statusFatal
		label = friendlyPrimaryText(row, lang)
	case statusErrorRecoverable:
		glyph = string(glyphRecoverable)
		glyphStyle = statusRecoverable
		labelStyle = statusRecoverable
		label = friendlyPrimaryText(row, lang)
	default:
		if row.endTime.IsZero() || !row.okFinished {
			return
		}
		glyph = string(glyphSuccess)
		key := stageKeyFor(row)
		label = stagePhrase(key, lang, stagePhraseDone)
	}
	progress := stageProgressFor(row)
	elapsed := ""
	if !row.endTime.IsZero() {
		elapsed = row.endTime.Sub(row.startTime).Truncate(time.Second).String()
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
	if elapsed != "" {
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint("·"))
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint(elapsed))
	}
	if row.iteration > 0 {
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint("·"))
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint(metaRoundPhrase(row.iteration, lang)))
	}
	if row.toolCount > 0 {
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint("·"))
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint(metaToolCountPhrase(row.toolCount, lang)))
	}
	if topicTotal > 1 && row.isNodeRow && row.nodeKind == "evidence" {
		if idx, ok := topicIndexFromNodeID(row.nodeID); ok {
			tag := fmt.Sprintf("关注点 %d/%d", idx+1, topicTotal)
			if !isZh(lang) {
				tag = fmt.Sprintf("focus %d/%d", idx+1, topicTotal)
			}
			b.WriteString(" ")
			b.WriteString(statusMeta.Sprint("·"))
			b.WriteString(" ")
			b.WriteString(statusObjective.Sprint(tag))
		}
	}
	fmt.Fprintln(w, b.String())
}
