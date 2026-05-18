package render

// focusRow picks the row whose state the dock should display this
// frame. Selection rules (in priority order):
//
//  1. r.current if it's a NodeRow that's actively running and no
//     upstream evidence_t* row is still in flight.
//  2. Otherwise, the first NodeRow whose endTime is zero, pending
//     is false, AND no upstream evidence_t* is still in flight
//     (Symptom 2 fix: prevents "validate shows before evidence done").
//  3. Otherwise, the first stage row that's running (analyze /
//     log_triage / perf_triage).
//  4. Otherwise, nil — composeDockRow1 then renders the bare
//     activity word with no stage context.
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
		// paused folds into pending — a paused row is "behind another
		// in-flight dispatch", not the focus. Without this, focusRow
		// could return a paused row, liveBarPrimaryText would render
		// it as "running" (because it's not endTime-set), and the
		// live bar would lie about which row is actually executing.
		if row.pending || row.paused || !row.endTime.IsZero() {
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

// fallbackRow picks a row to render row 2 against when focusRow
// returned nil. Used to keep the stage row honest during the
// brief windows where the live focus has just ended and the next
// dispatch has not yet started — pre-fix row 2 collapsed to a
// bare "▪ —" with no stage label.
//
// Selection rules (in priority order):
//
//  1. The first NodeRow that's pending=true — "what's about to run"
//     reads cleaner than "what just ended" when the next dispatch
//     is imminent (e.g. between evidence_t0 end and evidence_t1
//     start, fallback returns evidence_t1 so row 2 shows
//     "正在探索代码并收集证据" continuously instead of flickering
//     to "已 X" between siblings).
//  2. Otherwise, the most recently finished row (max endTime) so
//     the user reads "已 X" of the just-completed step instead of
//     the previous step's stale label.
//  3. nil otherwise — composeCurrentDockRows leaves row 2 blank.
//
// liveBarPrimaryText already resolves pending rows to "待 X" and
// finished rows to "已 X" / "X 失败"; we never need to invent a
// new lifecycle slot here.
//
// Caller MUST hold r.mu.
func (r *Renderer) fallbackRow() *taskRow {
	for _, row := range r.tasks {
		if row == nil || row.isSubAgent {
			continue
		}
		if row.isNodeRow && row.pending {
			return row
		}
	}
	var newest *taskRow
	for _, row := range r.tasks {
		if row == nil || row.isSubAgent {
			continue
		}
		if row.endTime.IsZero() {
			continue
		}
		if newest == nil || row.endTime.After(newest.endTime) {
			newest = row
		}
	}
	return newest
}

// topicProgressFor returns the ordinal focus segment when the focus is
// one of multiple evidence_tN siblings. Empty otherwise.
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
	return topicProgressPhrase(idx, total, lang)
}

// liveBarPrimaryText resolves the focus's stage label via the
// existing stagePhrase localisation helper. Shares
// primaryTextLifecycle with friendlyPrimaryText (status_blocks.go)
// so the dock-row-2 label and the spinner-area / commit-line label
// can never disagree on the row's lifecycle slot — including the
// recoverable/retry case (a row that errored and is awaiting retry
// reads as "X 出错,正在重试" in BOTH places, never as one-sided
// "未能 X" / "正在重试").
//
// paused folds into pending lexically — same rule the spinner area
// uses so a node parked behind another in-flight dispatch reads as
// "queued" in BOTH the dock and the live bar instead of one-sided
// "running" / "queued" disagreement.
func liveBarPrimaryText(row *taskRow, lang string) string {
	if row == nil {
		return ""
	}
	key := stageKeyFor(row)
	state := primaryTextLifecycle(row, classifyStatusError(row))
	return stagePhrase(key, lang, state)
}

// fallbackBarPrimaryText is the dock row 2 label resolver for the
// fallbackRow path. When the dock has an active LLM call / tool call
// AND the chosen row is a pending NodeRow, the LLM is computing FOR
// that next node — render the running slot ("正在 X") instead of
// pending ("待 X") so row 2 reads consistently with row 1's
// "请求模型中" / "调用工具中" / etc. For non-pending fallback rows
// (most-recently-finished pick) and for pending rows when no activity
// is in flight, fall through to the regular liveBarPrimaryText.
func fallbackBarPrimaryText(row *taskRow, lang string, activity activityKind) string {
	if row == nil {
		return ""
	}
	if row.isNodeRow && (row.pending || row.paused) && activityIsLive(activity) {
		key := stageKeyFor(row)
		return stagePhrase(key, lang, stagePhraseRunning)
	}
	return liveBarPrimaryText(row, lang)
}

// activityIsLive reports whether the dock's current activity kind
// indicates the engine is actively working (LLM call, tool call,
// streaming response). Idle / queued / waiting states return false.
func activityIsLive(kind activityKind) bool {
	switch kind {
	case activityRequesting, activityReceiving, activityCallingTool, activityRepoMapScanning, activityFinalizing:
		return true
	}
	return false
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
