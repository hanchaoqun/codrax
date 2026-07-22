package tracequery

// WINFLAG-1 (§29.190④, 2026-07-21; spec residuals from §29.189 AUDITFIX-C
// P3 / 残口④⑤⑥ 合并案): the single home of the RESULT-side typed start_set
// derivation. 「真重基 0 起点」 vs 「未设置回填 0」 becomes typed-distinguishable
// on the engine RESULT windows: the TimeStartSet parse flag (plus the
// normalizeQuery whole-trace backfill provenance) conducts into
// TimeWindow.StartSet at every q-window copy site via queryResultTimeWindow,
// and the three residual consumer families branch on it:
//
//	(a) the selected_window note producer (internal/tool) lifts its 0-start
//	    suppression only when the flag is set — a true rebased [0,end]
//	    window declares itself, the line-anchored unset (0,end) form stays
//	    honestly silent (the 「起止未采集」 false word dies for rebased runs);
//	(b) the chain-path / window_stats / state-account observation Spans
//	    (internal/tool) copy the q-window ts pair only when the start is
//	    determined — the unset form publishes an ABSENT (0,0) pair so
//	    evidence-index window labels never claim a whole-prefix window;
//	(c) the rank-fold family start>0 guards (rank_family_fold.go /
//	    rank_self_running_fold.go / demoteLockDominatedInversionCandidates /
//	    streamStateAccumulateDuration) read rankFoldStartUsable instead of
//	    guessing that 0 means absent — a flagged [0,end] run keeps its
//	    ts==0-starting members in interval inventories, envelopes and
//	    identity keys.
//
// Carrier decision (spec 实施轮廓 2 — two candidates evaluated): the
// AUDITFIX-C P1 precedent (window_source closed-set string on
// FrameTargetResolution) would need a new enum field on EVERY result struct
// plus R2' note-key sync; the TimeWindow.StartSet bool rides the existing
// carrier every result already embeds, serializes nowhere (`json:"-"`), and
// needs zero new note keys — the strictly smaller invasion, chosen.
// Progressive compatibility is structural: the flag defaults to false, every
// consumer's no-flag arm is byte-identical to the pre-WINFLAG behavior, and
// artifacts minted by older builds simply never carry the new positive arms.
// Query-API params keep their 0=unset sentinel untouched — the flag lives on
// results only and never flows back into Query fields.

// queryResultTimeWindow is THE constructor for a result window copied from
// the (normalized) query window — every `TimeWindow{StartTs: q.TimeStart,
// EndTs: q.TimeEnd}` result-copy site routes through it so the start_set
// flag conducts 全线. Values are verbatim; only the flag is added.
func queryResultTimeWindow(q Query) TimeWindow {
	return TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd, StartSet: queryResultWindowStartSet(q)}
}

// queryResultWindowStartSet derives the typed start_set flag from the parse
// flag and the normalizeQuery backfill provenance:
//   - TimeStartSet: the caller explicitly set time_start (including an
//     explicit 0 — the rebased [0,end] anchor form G8 exists for);
//   - TimeStart > 0: a positive start is unambiguous regardless of flags;
//   - timeStartBackfilled: normalizeQuery filled the whole-trace start from
//     idx.FirstTs — a determined index fact that is legally 0 on a rebased
//     export (the parity fix: a FirstTs>0 trace already published its
//     whole-trace window, only the FirstTs==0 rebased trace was dropped).
//
// The remaining false case is exactly the ambiguous unset-0 family: a
// line-anchored query whose TimeStart stayed at the 0=unset sentinel
// (normalizeQuery deliberately skips the backfill when LineStart>0), or an
// un-normalized unbounded query (宁漏勿假指 — conservative false).
func queryResultWindowStartSet(q Query) bool {
	return q.TimeStartSet || q.TimeStart > 0 || q.timeStartBackfilled
}

// queryWindowStartsAtDeterminedZero is the query-side twin of
// TimeWindow.StartsAtDeterminedZero for fold sites that hold q rather than a
// stamped result window: true exactly when this run's analysis window start
// is a DETERMINED real 0.
func queryWindowStartsAtDeterminedZero(q Query) bool {
	return q.TimeStart == 0 && (q.TimeStartSet || q.timeStartBackfilled)
}

// rankFoldStartUsable (WINFLAG-1 (c)) is the shared flag-aware member
// interval validity gate for the rank-fold family: a member [StartTs, EndTs)
// is usable when it has positive length and a determined start — positive,
// or 0 exactly when the analysis window's own start is a determined real 0
// (zeroStartReal): in a flagged [0,end] run ts==0 is a real timestamp
// (window-clamped members legally start at 0). Without the flag,
// member StartTs==0 keeps meaning the zero-value absence sentinel and the
// gate is byte-equivalent to the legacy `StartTs > 0 && EndTs > StartTs`
// guards (无旗零回归).
func rankFoldStartUsable(startTs, endTs float64, zeroStartReal bool) bool {
	if endTs <= startTs {
		return false
	}
	if startTs > 0 {
		return true
	}
	return zeroStartReal && startTs == 0
}
