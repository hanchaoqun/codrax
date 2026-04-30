package render

import (
	"fmt"
)

// focusRow picks the row whose state the dock should display this
// frame. Selection rules (in priority order):
//
//   1. r.current if it's a NodeRow that's actively running and no
//      upstream evidence_t* row is still in flight.
//   2. Otherwise, the first NodeRow whose endTime is zero, pending
//      is false, AND no upstream evidence_t* is still in flight
//      (Symptom 2 fix: prevents "validate shows before evidence done").
//   3. Otherwise, the first stage row that's running (analyze /
//      log_triage / perf_triage).
//   4. Otherwise, nil — composeDockRow1 then renders the bare
//      activity word with no stage context.
//
// Caller MUST hold r.mu.
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
	if r.current != nil && r.current.endTime.IsZero() && !r.current.pending {
		if !(r.current.isNodeRow && isDownstream(r.current.nodeKind) && upstreamEvidenceInFlight) {
			return r.current
		}
	}
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

// topicProgressFor returns "关注点 K/M" / "focus K/M" when the focus
// is one of multiple evidence_tN siblings. Empty otherwise.
func (r *Renderer) topicProgressFor(focus *taskRow, lang string) string {
	if focus == nil || !focus.isNodeRow || focus.nodeKind != "evidence" {
		return ""
	}
	idx, ok := topicIndexFromNodeID(focus.nodeID)
	if !ok {
		return ""
	}
	total := r.countTopicSiblings()
	if total < 2 {
		return ""
	}
	if isZh(lang) {
		return fmt.Sprintf("关注点 %d/%d", idx+1, total)
	}
	return fmt.Sprintf("focus %d/%d", idx+1, total)
}

// liveBarPrimaryText resolves the focus's stage label via the
// existing stagePhrase localisation helper.
func liveBarPrimaryText(row *taskRow, lang string) string {
	if row == nil {
		return ""
	}
	key := stageKeyFor(row)
	state := stagePhraseRunning
	switch {
	case row.isNodeRow && row.pending:
		state = stagePhrasePending
	case !row.endTime.IsZero():
		state = stagePhraseDone
	}
	return stagePhrase(key, lang, state)
}

// stageElapsedPhrase / totalElapsedPhrase localise the per-stage and
// cumulative "本 5s · 总 45s" trailers. Empty input returns empty.
func stageElapsedPhrase(elapsed, lang string) string {
	if elapsed == "" {
		return ""
	}
	if isZh(lang) {
		return "本 " + elapsed
	}
	return "stage " + elapsed
}

func totalElapsedPhrase(elapsed, lang string) string {
	if elapsed == "" {
		return ""
	}
	if isZh(lang) {
		return "总 " + elapsed
	}
	return "total " + elapsed
}
