package tracequery

// rank_p3_measure_flagship_test.go — P3MEASURE-1 flagship pins (§29.169,
// 2026-07-20) on the two committed real traces / four flagship boards:
//
//	pin① A/B write-surface purity (值/榜/席位/板指纹零动): clearing the four
//	     P3M* fields and re-stamping through the production pass reproduces
//	     the published wire byte-identically, and the stamp writes NOTHING
//	     outside the p3m_* JSON keys (every other item byte and every
//	     result-level byte compares equal before/after). Combined with the
//	     tool-side face A/B (answer_document_projection_p3measure_test.go)
//	     this is the 双不可见 proof chain.
//	pin② µs identity on every measured live seat: µs(valid) + µs(invalid)
//	     reconstructs exactly, edge_witnessed ≤ 席值 (+1µs float-dust slack
//	     against the independently-published value channel).
//	pin③ tieba audit calibration (校准 pin ①): the archived
//	     onchain_segment_audit_20260718.md quadrant reproduces through the
//	     measurement's own primitives — EXACT µs numbers at the audit's
//	     legacy chain caps, and the argued HEAD-caps evolution (see the
//	     tolerance argument inline).
//	pin④ the two-dimension separation live witness: the 60555 shared-worker
//	     seat (the ruled-legal 97.8% 兼服 form) measures counterfactually
//	     VALID in full while its structural edge-witness is zero — the exact
//	     coexistence §29.169 rules (ruling on the counterfactual lane, honest
//	     structure on the audit lane, no re-litigation).

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

var p3mFlagshipBoards = []struct {
	name  string
	trace string
	q     Query
}{
	{"tieba_flag", "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace",
		Query{PID: 59566, TimeStart: 34579.450627, TimeEnd: 34579.595184,
			MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12}},
	{"tieba_trace", "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace",
		Query{PID: 59566, TimeStart: 34579.472865, TimeEnd: 34579.587805,
			TraceFlavorHint: TraceFlavorHarmonyHitrace}},
	{"donghu_2955", "../../eval/fixtures/real_traces/donghu.ftrace",
		Query{PID: 2955, TimeStart: 13762.791708, TimeEnd: 13763.024898,
			MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12}},
	{"donghu_17267", "../../eval/fixtures/real_traces/donghu.ftrace",
		Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898,
			TraceFlavorHint: TraceFlavorHarmonyHitrace}},
}

func p3mFlagshipIndex(t *testing.T, cacheMap map[string]*Index, trace string) *Index {
	t.Helper()
	if idx, ok := cacheMap[trace]; ok {
		return idx
	}
	if _, err := os.Stat(trace); err != nil {
		t.Skipf("real-trace fixture absent: %v", err)
	}
	idx, err := BuildIndex(context.Background(), trace)
	if err != nil {
		t.Fatal(err)
	}
	cacheMap[trace] = idx
	return idx
}

// p3mStripJSON marshals an item and removes the four p3m_* wire keys.
func p3mStripJSON(t *testing.T, item RootCauseRankItem) string {
	t.Helper()
	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"p3m_counterfactual_valid_ms", "p3m_counterfactual_invalid_ms",
		"p3m_edge_witnessed_ms", "p3m_disposition",
	} {
		delete(m, key)
	}
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// TestP3MeasureFlagshipABPurityAndIdentity — pin① + pin② on all four boards.
func TestP3MeasureFlagshipABPurityAndIdentity(t *testing.T) {
	if testing.Short() {
		t.Skip("real-trace battery")
	}
	indexes := map[string]*Index{}
	for _, board := range p3mFlagshipBoards {
		idx := p3mFlagshipIndex(t, indexes, board.trace)
		rank := BuildRootCauseRank(idx, board.q)
		if rank.p3MeasureCtx == nil {
			t.Fatalf("%s: the measurement context must ride the rank result", board.name)
		}

		// ── pin②: measured seats exist and hold the µs invariants ─────────
		measured := 0
		for i := range rank.Items {
			item := &rank.Items[i]
			switch item.P3MDisposition {
			case "":
				relationOnlySemantic := item.OnChainBasis == RootCauseOnChainBasisHostWakeupEdge &&
					rootCauseItemIsSemanticSpanWork(*item) && rootCauseEffectiveImpactMs(*item) <= 0
				if rootCauseItemIsOnChain(*item) && !relationOnlySemantic {
					t.Fatalf("%s: on-chain seat without a measurement disposition: r%d %s pid=%d basis=%q",
						board.name, item.Rank, item.Type, item.Thread.PID, item.OnChainBasis)
				}
				continue
			case p3mDispositionSegmentJoin, p3mDispositionEdgeTerminatedWindow, p3mDispositionCounterfactualOnly:
				measured++
			}
			validUs, invalidUs := p3mUs(item.P3MCounterfactualValidMs), p3mUs(item.P3MCounterfactualInvalidMs)
			witnessedUs := p3mUs(item.P3MEdgeWitnessedMs)
			if validUs < 0 || invalidUs < 0 || witnessedUs < 0 {
				t.Fatalf("%s: negative measurement on r%d %s: %+v", board.name, item.Rank, item.Type, item)
			}
			if item.P3MDisposition == p3mDispositionSelfRuled &&
				(validUs != 0 || invalidUs != 0 || witnessedUs != 0) {
				t.Fatalf("%s: self_ruled seat carries numbers (禁重诉既裁): r%d %s %+v",
					board.name, item.Rank, item.Type, item)
			}
			// edge_witnessed ≤ 席值 (+1µs float-dust slack: the value channel
			// is published through an independent float path).
			if valueUs := p3mUs(rootCauseEffectiveImpactMs(*item)); witnessedUs > valueUs+1 {
				t.Fatalf("%s: edge_witnessed %dµs exceeds 席值 %dµs on r%d %s",
					board.name, witnessedUs, valueUs, item.Rank, item.Type)
			}
		}
		if measured == 0 {
			t.Fatalf("%s: the flagship board must hold at least one measured seat (A must be meaningful)", board.name)
		}

		// ── pin①: clear + restamp through the PRODUCTION pass ─────────────
		origItems := make([]string, len(rank.Items))
		origStripped := make([]string, len(rank.Items))
		for i := range rank.Items {
			raw, err := json.Marshal(rank.Items[i])
			if err != nil {
				t.Fatal(err)
			}
			origItems[i] = string(raw)
			origStripped[i] = p3mStripJSON(t, rank.Items[i])
		}
		resultNoItems := rank
		resultNoItems.Items = nil
		origResult, err := json.Marshal(resultNoItems)
		if err != nil {
			t.Fatal(err)
		}

		cleared := rank
		cleared.Items = append([]RootCauseRankItem(nil), rank.Items...)
		for i := range cleared.Items {
			p3mClearSeat(&cleared.Items[i])
			if got := p3mStripJSON(t, cleared.Items[i]); got != origStripped[i] {
				t.Fatalf("%s: clearing the P3M fields moved a non-p3m byte on item %d:\n%s\nvs\n%s",
					board.name, i, got, origStripped[i])
			}
		}
		stampP3CounterfactualMeasure(&cleared)
		for i := range cleared.Items {
			raw, _ := json.Marshal(cleared.Items[i])
			if string(raw) != origItems[i] {
				t.Fatalf("%s: restamp must reproduce the published wire byte-identically (item %d):\n%s\nvs\n%s",
					board.name, i, raw, origItems[i])
			}
			// The stamp wrote nothing outside the p3m_* keys.
			if got := p3mStripJSON(t, cleared.Items[i]); got != origStripped[i] {
				t.Fatalf("%s: the stamp moved a non-p3m byte on item %d", board.name, i)
			}
		}
		clearedNoItems := cleared
		clearedNoItems.Items = nil
		if rawResult, _ := json.Marshal(clearedNoItems); string(rawResult) != string(origResult) {
			t.Fatalf("%s: the stamp moved a result-level byte:\n%s\nvs\n%s", board.name, rawResult, origResult)
		}
	}
}

// TestP3MeasureTiebaAuditCalibration — pin③ + pin④ (校准 pin ①): the
// archived audit quadrant (onchain_segment_audit_20260718.md §2, measured on
// main=8a6e327a9) reproduces through the measurement's own primitives
// (anchor union + census segments + the audit-同源 edge join).
//
// Legacy chain caps (MaxBranches=8, MaxChainNodes=1 — the CHAIN-BUDGET 退化
// 恒等 caps that reproduce the audited 24-node/16-edge chain): EXACT µs
// reproduction of the archived table —
//
//	pid    anchored   ∧edge     proxy-only        (audit verbatim)
//	59843  21.242     20.823    0.419 (2.0%)      20.823 | 0.419 (2.0%)
//	60595  14.700     14.689    0.011 (0.1%)      14.689 | 0.011 (0.1%)
//	60555  19.065      0.424   18.641 (97.8%)     0.424  | 18.641 (97.8%)
//
// HEAD default caps (容差论证): CHAIN-BUDGET (§29.136) recovered the audited
// reverse-form leak, growing the anchor unions 21.242→25.790 / 14.700→18.979.
// 59843's proxy-only NUMERATOR stays byte-exact (0.419ms — the recovered
// segments all carry edges, share 2.0%→1.62%); 60595 additionally anchors
// 0.410ms of formerly-UNANCHORED no-edge time — bounded by the audit's own
// 未锚定∧无边 quadrant reading (0.469ms), i.e. the growth is time the audit
// already accounted, moved across the anchor column by the bigger union
// (0.011+0.410=0.421ms, 2.22%). 60555 is cap-invariant (its windows were
// never budget-limited): 18.641ms / 97.78% at BOTH cap sets.
func TestP3MeasureTiebaAuditCalibration(t *testing.T) {
	if testing.Short() {
		t.Skip("real-trace battery")
	}
	const trace = "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace"
	if _, err := os.Stat(trace); err != nil {
		t.Skipf("real-trace fixture absent: %v", err)
	}
	idx, err := BuildIndex(context.Background(), trace)
	if err != nil {
		t.Fatal(err)
	}

	type quad struct{ anchoredUs, witnessedUs int64 }
	quadrant := func(legacy bool) map[int]*quad {
		q := Query{PID: 59566, TimeStart: 34579.472865, TimeEnd: 34579.587805,
			TraceFlavorHint: TraceFlavorHarmonyHitrace}
		if legacy {
			q.MaxBranches = 8
			q.MaxChainNodes = 1
		}
		q = normalizeQuery(idx, q)
		q = ensureQueryFlavor(idx, q)
		cache := newChainQueryCache(idx, q.runCancel)
		chain := buildWakeupChainWithCache(idx, q, cache)
		ctx := buildP3MeasureContext(idx, cache, &chain)
		if ctx == nil {
			t.Fatalf("legacy=%v: measurement context must build", legacy)
		}
		q.chainAnchorWindowsByPID = ctx.anchors
		stats := ComputeWindowStats(idx, q)
		quads := map[int]*quad{}
		scan := func(census map[string]ThreadDuration, dio bool) {
			for _, td := range census {
				pid := td.Thread.PID
				if pid <= 0 || len(ctx.anchors[pid]) == 0 {
					continue
				}
				ivs := td.runnableIntervals
				if dio {
					ivs = td.dioIntervals
				}
				qd := quads[pid]
				if qd == nil {
					qd = &quad{}
					quads[pid] = qd
				}
				for _, iv := range ivs {
					witnessed := p3mSegmentHasEdge(ctx.edgeTs[pid], iv.start, iv.end)
					for _, w := range ctx.anchors[pid] {
						us := p3mRoundUs(p3mOverlapS(iv.start, iv.end, w.StartTs, w.EndTs))
						qd.anchoredUs += us
						if witnessed {
							qd.witnessedUs += us
						}
					}
				}
			}
		}
		scan(stats.runnableCensus, false)
		scan(stats.dstateCensus, true)
		scan(stats.iowaitCensus, true)
		return quads
	}
	expect := func(quads map[int]*quad, pid int, anchoredUs, witnessedUs int64, label string) {
		t.Helper()
		qd := quads[pid]
		if qd == nil {
			t.Fatalf("%s: pid %d holds no anchored census time", label, pid)
		}
		if qd.anchoredUs != anchoredUs || qd.witnessedUs != witnessedUs {
			t.Fatalf("%s pid %d: anchored/witnessed = %d/%dµs, want %d/%dµs (audit reproduction drifted)",
				label, pid, qd.anchoredUs, qd.witnessedUs, anchoredUs, witnessedUs)
		}
	}

	// ── legacy caps: byte-exact archive reproduction ───────────────────────
	legacyQuads := quadrant(true)
	expect(legacyQuads, 59843, 21242, 20823, "legacy") // proxy-only 0.419ms = 2.0%
	expect(legacyQuads, 60595, 14700, 14689, "legacy") // proxy-only 0.011ms = 0.1%
	expect(legacyQuads, 60555, 19065, 424, "legacy")   // proxy-only 18.641ms = 97.8%

	// ── HEAD caps: the argued evolution (numerator conservation) ───────────
	headQuads := quadrant(false)
	expect(headQuads, 59843, 25790, 25371, "head") // proxy-only 0.419ms EXACT (share 1.62%)
	expect(headQuads, 60595, 18979, 18558, "head") // proxy-only 0.421 = 0.011 + 0.410 ≤ 0.011+0.469
	expect(headQuads, 60555, 19065, 424, "head")   // cap-invariant 97.78%
	if grown := (headQuads[60595].anchoredUs - headQuads[60595].witnessedUs) -
		(legacyQuads[60595].anchoredUs - legacyQuads[60595].witnessedUs); grown > 469 {
		t.Fatalf("60595 proxy-only growth %dµs exceeds the audited 未锚定∧无边 budget 469µs — the tolerance argument no longer holds", grown)
	}

	// ── pin④: the ruled-legal 97.8% form on the LIVE published board ───────
	// The 60555 d_state seat: counterfactually VALID in full (no typed
	// periodic hit — the 兼服 form is ruled legal, §29.169 red line) while
	// its structural edge-witness is ZERO (the audit's 97.8% proxy-only
	// reading, honest on its own lane). The two dimensions coexist without
	// one re-litigating the other.
	rank := BuildRootCauseRank(idx, Query{PID: 59566, TimeStart: 34579.472865, TimeEnd: 34579.587805,
		TraceFlavorHint: TraceFlavorHarmonyHitrace})
	var shared *RootCauseRankItem
	for i := range rank.Items {
		item := &rank.Items[i]
		if item.Thread.PID == 60555 && item.Type == "d_state_or_io_wait" && item.Rank > 0 {
			shared = item
			break
		}
	}
	if shared == nil {
		t.Fatalf("fixture drift: the tieba board must seat the 60555 d_state shared-worker row")
	}
	if shared.P3MDisposition != p3mDispositionSegmentJoin {
		t.Fatalf("60555 witness: want measured_segment_join, got %q", shared.P3MDisposition)
	}
	if p3mUs(shared.P3MCounterfactualInvalidMs) != 0 || p3mUs(shared.P3MEdgeWitnessedMs) != 0 {
		t.Fatalf("60555 witness: 兼服 form = counterfactually valid in full (invalid=0) AND structurally zero-witnessed, got invalid=%.3f edgeW=%.3f",
			shared.P3MCounterfactualInvalidMs, shared.P3MEdgeWitnessedMs)
	}
	if validUs := p3mUs(shared.P3MCounterfactualValidMs); validUs != p3mUs(rootCauseEffectiveImpactMs(*shared)) {
		t.Fatalf("60555 witness: the seat's anchor time must equal its published D/IO wall clock, got %dµs vs %dµs",
			validUs, p3mUs(rootCauseEffectiveImpactMs(*shared)))
	}
	if !strings.Contains(shared.P3MDisposition, "measured_") {
		t.Fatalf("unreachable")
	}
}
