package tracequery

// rank_family_fold_rcm_test.go — RCM engine-half pins (ledger §24.7.1 /
// §24.10 / §24.12 dimension A, real_trace_campaign_20260705.md, user rulings
// 2026-07-08).
//
// Witness anchors:
//   - opendir_78 gap③ (E5/E6): two same-thread block_io_by_inode rows with
//     DIFFERENT inodes (1.136ms rank#3 + 0.462ms rank#8, disjoint segments) →
//     one 1.598ms contender whose board position is at least as good as the
//     best pre-merge member (§24.7.1 合并量参赛).
//   - cmp_78_01 dim-A (E27-E42): ×14 same-thread class_verification spans,
//     pairwise disjoint, union == Σ == 7.124ms → ONE family row.
//
// Mutation self-checks (each verified RED during development by applying the
// mutation locally):
//   M-a fold disabled (foldSameThreadTypeRankFamilies returns its input /
//       rank loop iterates spans per-span)      → TestRCMBlockIOInodeFamilyMergesIntoOneContender,
//                                                 TestRCMOpendirShapeBoardComparison,
//                                                 TestRCMSemanticFamilyRankRowCarriesFamilyTotal red.
//   M-b union guard replaced by naive Σ         → TestRCMSemanticFamilyOverlapPublishesUnionNotSum,
//                                                 TestRCMGenericFamilyIntervalUnionCaliber red.
//   M-c selected-window key dropped from the
//       merge key (cross-window discipline off) → TestRCMCrossWindowSameThreadTypeRowsNeverMerge red.
//   M-d family carriers rerouted onto the
//       display Merged* lane                    → TestTraceCausalProjectionFamilyMemberLaneIsolatedFromMergedLane
//                                                 (internal/types) red.

import (
	"reflect"
	"strings"
	"testing"
)

// --- shared fixture helpers ----------------------------------------------------

func rcmSpan(thread ThreadRef, name string, startTs, endTs float64, startLine, endLine int) TraceSpanSummary {
	return TraceSpanSummary{
		Thread:        thread,
		Kind:          "sync",
		Name:          name,
		SemanticClass: traceSpanSemanticClass(name),
		StartTs:       startTs,
		EndTs:         endTs,
		DurationMs:    (endTs - startTs) * 1000,
		StartLine:     startLine,
		EndLine:       endLine,
	}
}

func rcmRankItem(typ string, thread ThreadRef, raw float64, startTs, endTs float64, lineStart, lineEnd int) RootCauseRankItem {
	item := rootCauseItem(typ, thread, raw, 0.76, lineStart, lineEnd, "window_stats", "fixture "+typ)
	item.CumulativeImpactMs = raw
	switch typ {
	case "runnable_wait", "scheduler_latency", "fragmented_runnable_wait":
		item.DominantState = string(StateRunnable)
		item.RunnableMs = raw
	case "io_wait":
		item.DominantState = string(StateIOWait)
		item.IOWaitMs = raw
	case "d_state_or_io_wait", "fragmented_d_state_or_io_wait":
		item.DominantState = string(StateDSleep)
		item.DStateMs = raw
	}
	if rootCauseItemIsSemanticSpanWork(item) {
		item.EffectiveImpactMs = raw
	}
	item.StartTs = startTs
	item.EndTs = endTs
	item.StatsWindowStartTs = 10.0
	item.StatsWindowEndTs = 10.2
	item.Causality = "background"
	item.ChainRelevance = "background"
	return item
}

// --- M1: semantic span family fold --------------------------------------------

func TestRCMSemanticFamilyFoldDisjointSumsAndPreservesSpans(t *testing.T) {
	worker := ThreadRef{Comm: "worker", PID: 200}
	other := ThreadRef{Comm: "other", PID: 300}
	spans := []TraceSpanSummary{
		rcmSpan(worker, "VerifyClass com.example.A", 5.0010, 5.0030, 10, 11), // 2.0ms
		rcmSpan(worker, "VerifyClass com.example.B", 5.0040, 5.0045, 12, 13), // 0.5ms
		rcmSpan(worker, "VerifyClass com.example.C", 5.0050, 5.0080, 14, 15), // 3.0ms
		rcmSpan(other, "VerifyClass com.example.D", 5.0010, 5.0020, 16, 17),  // 1.0ms, other thread
	}
	before := make([]TraceSpanSummary, len(spans))
	copy(before, spans)
	families := FoldSemanticSpanFamilies(nil, spans)
	if len(families) != 2 {
		t.Fatalf("(thread, class) grouping must yield 2 families (cross-thread never merges): %+v", families)
	}
	fam := families[0]
	if fam.Thread.PID != 200 || len(fam.Members) != 3 {
		t.Fatalf("the worker family must fold exactly its 3 same-thread spans: %+v", fam)
	}
	if fam.FoldCaliber != RootCauseMemberFoldCaliberSumDisjoint {
		t.Fatalf("disjoint members carry the sum_disjoint caliber: %+v", fam)
	}
	if fam.TotalMs < 5.499 || fam.TotalMs > 5.501 {
		t.Fatalf("disjoint family total == member Σ (5.5ms): got %.3f", fam.TotalMs)
	}
	if fam.Members[0].Name != "VerifyClass com.example.C" {
		t.Fatalf("members sort largest-first (representative identity): %+v", fam.Members)
	}
	if fam.MaxMs != fam.Members[0].DurationMs || fam.MinMs < 0.499 || fam.MinMs > 0.501 {
		t.Fatalf("member range accounting wrong: %+v", fam)
	}
	// stats.TraceSpans is a VIEW input — never rewritten by the fold.
	if !reflect.DeepEqual(before, spans) {
		t.Fatalf("the fold must never mutate the span inventory")
	}
	roster := fam.MemberRosterEntries()
	if len(roster) != 3 || !strings.Contains(roster[0], "VerifyClass com.example.C") {
		t.Fatalf("roster carries the member span names (distinguishing keys never drop): %v", roster)
	}
}

func TestRCMSemanticFamilyOverlapPublishesUnionNotSum(t *testing.T) {
	worker := ThreadRef{Comm: "worker", PID: 200}
	spans := []TraceSpanSummary{
		rcmSpan(worker, "VerifyClass com.example.A", 5.0010, 5.0030, 10, 11), // 2.0ms
		rcmSpan(worker, "VerifyClass com.example.B", 5.0020, 5.0040, 12, 13), // 2.0ms, overlaps 1ms
	}
	families := FoldSemanticSpanFamilies(nil, spans)
	if len(families) != 1 {
		t.Fatalf("one (thread,class) group expected: %+v", families)
	}
	fam := families[0]
	if fam.FoldCaliber != RootCauseMemberFoldCaliberIntervalUnion {
		t.Fatalf("overlapping members carry the interval_union caliber: %+v", fam)
	}
	// Mutation M-b: a naive Σ would publish 4.0 — the union is 3.0.
	if fam.TotalMs < 2.999 || fam.TotalMs > 3.001 {
		t.Fatalf("overlapping same-thread segments deduplicate to the union (3.0ms), never the naive Σ (4.0ms): got %.3f", fam.TotalMs)
	}
	if fam.SumMs < 3.999 || fam.SumMs > 4.001 {
		t.Fatalf("the lossless raw Σ must stay disclosed: %+v", fam)
	}
}

func TestRCMSemanticFamilyLaneSplitsAndNeverCrossMerges(t *testing.T) {
	worker := ThreadRef{Comm: "worker", PID: 200}
	chain := &ChainResult{Nodes: []ChainNode{{
		Thread: worker,
		Window: TimeWindow{StartTs: 5.0000, EndTs: 5.0035},
	}}}
	spans := []TraceSpanSummary{
		rcmSpan(worker, "VerifyClass com.example.A", 5.0010, 5.0030, 10, 11), // overlaps node → on-chain
		rcmSpan(worker, "VerifyClass com.example.B", 5.0050, 5.0080, 12, 13), // no overlap → non-chain
	}
	families := FoldSemanticSpanFamilies(chain, spans)
	if len(families) != 2 {
		t.Fatalf("道别键: on-chain and non-chain members of one (thread,class) form TWO families: %+v", families)
	}
	var onChain, offChain int
	for _, fam := range families {
		if fam.OnChain {
			onChain++
		} else {
			offChain++
		}
	}
	if onChain != 1 || offChain != 1 {
		t.Fatalf("lane split expected exactly 1 on-chain + 1 non-chain family: %+v", families)
	}
}

func TestRCMSemanticFamilyOfOneStaysByteIdenticalSingleSpanRow(t *testing.T) {
	worker := ThreadRef{Comm: "worker", PID: 200}
	span := rcmSpan(worker, "JIT compiling void a.b", 5.0010, 5.0030, 10, 11)
	families := FoldSemanticSpanFamilies(nil, []TraceSpanSummary{span})
	if len(families) != 1 || len(families[0].Members) != 1 {
		t.Fatalf("family of one expected: %+v", families)
	}
	// 退化不变体: the pipeline routes a family of one through the single-span
	// constructor with the VERBATIM member span, so the rank row stays
	// byte-identical to the pre-RCM shape (and carries no member accounting).
	if !reflect.DeepEqual(families[0].Members[0], span) {
		t.Fatalf("the single member must be the verbatim span: %+v", families[0].Members[0])
	}
	q := Query{TimeStart: 5.0, TimeEnd: 5.01}
	item, ok := rootCauseItemFromSemanticTraceSpan(q, ChainResult{}, families[0].Members[0], false)
	if !ok {
		t.Fatalf("single-span constructor must mint")
	}
	if item.MemberCount != 0 || len(item.MemberRoster) != 0 || item.MemberFoldCaliber != "" {
		t.Fatalf("a degenerate family carries NO member accounting (byte-stable single row): %+v", item)
	}
}

func TestRCMSemanticFamilyRankRowCarriesFamilyTotal(t *testing.T) {
	// cmp_78_01 dim-A shape (scaled): same-thread disjoint class_verification
	// spans must publish ONE rank row whose value channels carry the family
	// total and whose roster names every member.
	idx := buildTraceIndex(t, "rcm_semantic_family.systrace", `
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=R ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 5.000400: tracing_mark_write: B|200|VerifyClass com.example.A
     worker-200 (200) [002] .... 5.001400: tracing_mark_write: E|200
     worker-200 (200) [002] .... 5.002000: tracing_mark_write: B|200|VerifyClass com.example.B
     worker-200 (200) [002] .... 5.002500: tracing_mark_write: E|200
     worker-200 (200) [002] .... 5.003000: tracing_mark_write: B|200|VerifyClass com.example.C
     worker-200 (200) [002] .... 5.005000: tracing_mark_write: E|200
        app-100 (100) [001] .... 5.006500: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.007, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	var semanticRows []RootCauseRankItem
	for _, item := range rank.Items {
		if item.Type == "class_verification" {
			semanticRows = append(semanticRows, item)
		}
	}
	if len(semanticRows) != 1 {
		t.Fatalf("×3 same-thread spans must participate as ONE family row (mutation M-a bites here): %+v", rank.Items)
	}
	fam := semanticRows[0]
	if fam.MemberCount != 3 || len(fam.MemberRoster) != 3 {
		t.Fatalf("family accounting must ride the typed member carriers: %+v", fam)
	}
	// 1.0 + 0.5 + 2.0 = 3.5ms disjoint sum.
	if fam.ImpactMs < 3.499 || fam.ImpactMs > 3.501 {
		t.Fatalf("the participation value is the window-projection family total: %+v", fam)
	}
	if fam.MemberFoldCaliber != RootCauseMemberFoldCaliberSumDisjoint {
		t.Fatalf("disjoint members publish sum_disjoint: %+v", fam)
	}
	if fam.SpanName != "VerifyClass com.example.C" {
		t.Fatalf("the family representative is the largest member: %+v", fam)
	}
	if fam.MemberMaxMs < 1.999 || fam.MemberMaxMs > 2.001 || fam.MemberMinMs < 0.499 || fam.MemberMinMs > 0.501 {
		t.Fatalf("member range accounting wrong: %+v", fam)
	}
}

func TestRCMCmpShapeBoardComparisonFamilyOutranksBestSingle(t *testing.T) {
	// cmp_78 witness-shaped 对照 (M4): the same pool ranked per-span (pre-RCM
	// board) vs family-merged (post-RCM board) — the family's board position
	// must be at least as good as the best single member's, and the two dumps
	// are logged as the before/after record.
	worker := ThreadRef{Comm: "verify", PID: 200}
	q := Query{TimeStart: 5.0, TimeEnd: 5.1}
	var spans []TraceSpanSummary
	starts := 5.0010
	for i := 0; i < 14; i++ {
		spans = append(spans, rcmSpan(worker, "VerifyClass com.example.Klass", starts, starts+0.0005, 100+i*2, 101+i*2)) // 0.5ms each
		starts += 0.0010
	}
	competitors := []RootCauseRankItem{
		rcmRankItem("io_latency", ThreadRef{Comm: "io", PID: 300}, 3.0, 10.01, 10.013, 300, 301),
		rcmRankItem("runnable_wait", ThreadRef{Comm: "bg", PID: 400}, 5.0, 10.02, 10.025, 400, 401),
	}
	boardPosition := func(items []RootCauseRankItem) (best int, dump []string) {
		normalizeRootCauseCumulativeImpact(items)
		normalizeRootCauseEffectiveImpact(items)
		sortRootCauseRankItems(items, false)
		best = -1
		for i, item := range items {
			dump = append(dump, item.Type)
			if item.Type == "class_verification" && best == -1 {
				best = i
			}
		}
		return best, dump
	}
	// BEFORE: per-span participation (the pre-§24.10 shape).
	var before []RootCauseRankItem
	for _, span := range spans {
		if item, ok := rootCauseItemFromSemanticTraceSpan(q, ChainResult{}, span, false); ok {
			before = append(before, item)
		}
	}
	before = append(before, competitors[0], competitors[1])
	beforePos, beforeDump := boardPosition(before)
	// AFTER: family participation.
	var after []RootCauseRankItem
	for _, fam := range FoldSemanticSpanFamilies(nil, spans) {
		item, ok := rootCauseItemFromSemanticSpanFamily(q, fam, false)
		if !ok {
			t.Fatalf("family mint failed: %+v", fam)
		}
		after = append(after, item)
	}
	if len(after) != 1 {
		t.Fatalf("×14 same-thread spans form ONE family: %d", len(after))
	}
	if after[0].ImpactMs < 6.999 || after[0].ImpactMs > 7.001 {
		t.Fatalf("family total must be the 7.0ms disjoint Σ (witness form ×14 union==Σ): %+v", after[0])
	}
	after = append(after, competitors[0], competitors[1])
	afterPos, afterDump := boardPosition(after)
	t.Logf("board BEFORE (per-span): pos=%d %v", beforePos, beforeDump)
	t.Logf("board AFTER  (family):  pos=%d %v", afterPos, afterDump)
	if beforePos < 0 || afterPos < 0 {
		t.Fatalf("both boards must seat the semantic contender: before=%d after=%d", beforePos, afterPos)
	}
	if afterPos > beforePos {
		t.Fatalf("合并量参赛: the family position (%d) must be at least as good as the best single member position (%d)", afterPos, beforePos)
	}
	if afterPos != 0 {
		t.Fatalf("witness direction: the 7.0ms family outranks the 5.0/3.0 competitors, got position %d (%v)", afterPos, afterDump)
	}
}

// --- M2: generic same-(thread,type) families -----------------------------------

func TestRCMBlockIOInodeFamilyMergesIntoOneContender(t *testing.T) {
	// opendir_78 E5/E6 witness shape: same thread, same type, two different
	// inodes, disjoint segments — 1.136 + 0.462 = 1.598 single contender.
	thread := ThreadRef{Comm: "RxComputationT", PID: 16816}
	e5 := rcmRankItem("block_io_by_inode", thread, 1.136, 10.050, 10.070, 59099, 77131)
	e5.Inode = "286395"
	e5.Dev = "254:2"
	e5.MemberKey = "inode=286395 dev=254:2"
	e6 := rcmRankItem("block_io_by_inode", thread, 0.462, 10.010, 10.020, 44183, 44638)
	e6.Inode = "300123"
	e6.Dev = "254:2"
	e6.MemberKey = "inode=300123 dev=254:2"
	other := rcmRankItem("runnable_wait", ThreadRef{Comm: "bg", PID: 400}, 0.9, 10.03, 10.031, 100, 101)
	items := foldSameThreadTypeRankFamilies(Query{TimeStart: 10.0, TimeEnd: 10.2}, false, []RootCauseRankItem{e5, other, e6})
	var merged *RootCauseRankItem
	blockRows := 0
	for i := range items {
		if items[i].Type == "block_io_by_inode" {
			blockRows++
			merged = &items[i]
		}
	}
	if blockRows != 1 {
		t.Fatalf("two same-(thread,type) rows must merge into ONE contender (mutation M-a bites here): %+v", items)
	}
	if merged.CumulativeImpactMs < 1.597 || merged.CumulativeImpactMs > 1.599 {
		t.Fatalf("disjoint segments sum: want 1.598, got %+v", merged)
	}
	if merged.MemberCount != 2 || merged.MemberFoldCaliber != RootCauseMemberFoldCaliberSumDisjoint {
		t.Fatalf("family accounting: %+v", merged)
	}
	if merged.MemberMaxMs != 1.136 || merged.MemberMinMs != 0.462 {
		t.Fatalf("member range: %+v", merged)
	}
	// §24.7.1 ①: the real distinguishing keys must never be dropped — the
	// merged row clears the ambiguous Inode and the roster carries BOTH.
	if merged.Inode != "" {
		t.Fatalf("differing member inodes must clear the merged Inode field: %+v", merged)
	}
	if merged.Dev != "254:2" {
		t.Fatalf("an agreeing member dev stays on the merged row: %+v", merged)
	}
	roster := strings.Join(merged.MemberRoster, " | ")
	if !strings.Contains(roster, "inode=286395") || !strings.Contains(roster, "inode=300123") {
		t.Fatalf("roster must carry every member's distinguishing key: %v", merged.MemberRoster)
	}
	if merged.LineStart != 44183 || merged.LineEnd != 77131 {
		t.Fatalf("evidence envelope: %+v", merged)
	}
}

func TestRCMOpendirShapeBoardComparison(t *testing.T) {
	// opendir_78 board 对照 (M4): pre-merge the io_latency family splits into
	// 6 seats (max single 0.568) and block_io holds #(top) with 1.136 while
	// also occupying a second seat with 0.462; post-merge the io family
	// (union = 2.547: Σ2.946 − 0.399 overlap dedup) outranks the block family
	// (1.598) — the audit's
	// "弱化排序存在且双向" correction, both directions healed by one fold.
	ioThread := ThreadRef{Comm: "RxComputationT", PID: 16816}
	mkIO := func(raw float64, start float64, l1, l2 int) RootCauseRankItem {
		it := rcmRankItem("io_latency", ioThread, raw, start, start+raw/1000, l1, l2)
		it.MemberKey = "dev=254:2 op=R"
		return it
	}
	// One overlapping pair (0.474 ∩ 0.499 shape) — union < Σ.
	pool := []RootCauseRankItem{
		mkIO(0.568, 10.010, 100, 101),
		mkIO(0.500, 10.020, 102, 103),
		mkIO(0.499, 10.0300, 104, 105),
		mkIO(0.474, 10.0301, 106, 107), // overlaps the 0.499 segment
		mkIO(0.462, 10.040, 108, 109),
		mkIO(0.443, 10.050, 110, 111),
	}
	blockA := rcmRankItem("block_io_by_inode", ioThread, 1.136, 10.060, 10.062, 200, 201)
	blockA.Inode = "286395"
	blockB := rcmRankItem("block_io_by_inode", ioThread, 0.462, 10.070, 10.071, 202, 203)
	blockB.Inode = "300123"
	pool = append(pool, blockA, blockB)
	dumpBoard := func(items []RootCauseRankItem) []string {
		normalizeRootCauseCumulativeImpact(items)
		normalizeRootCauseEffectiveImpact(items)
		sortRootCauseRankItems(items, false)
		var out []string
		for _, item := range items {
			out = append(out, item.Type)
		}
		return out
	}
	beforeItems := make([]RootCauseRankItem, len(pool))
	copy(beforeItems, pool)
	beforeBoard := dumpBoard(beforeItems)
	afterItems := foldSameThreadTypeRankFamilies(Query{TimeStart: 10.0, TimeEnd: 10.2}, false, pool)
	afterBoard := dumpBoard(afterItems)
	t.Logf("board BEFORE (split): %v", beforeBoard)
	t.Logf("board AFTER  (family): %v", afterBoard)
	if beforeBoard[0] != "block_io_by_inode" {
		t.Fatalf("pre-merge the split io family loses the top seat to the 1.136 block row: %v", beforeBoard)
	}
	if len(afterBoard) != 2 {
		t.Fatalf("post-merge exactly two family contenders remain: %v", afterBoard)
	}
	if afterBoard[0] != "io_latency" {
		t.Fatalf("§24.7.1 direction: the io family total must outrank the block family (audit-corrected order): %v", afterBoard)
	}
	var ioFam *RootCauseRankItem
	for i := range afterItems {
		if afterItems[i].Type == "io_latency" {
			ioFam = &afterItems[i]
		}
	}
	if ioFam.MemberFoldCaliber != RootCauseMemberFoldCaliberIntervalUnion {
		t.Fatalf("the overlapping pair forces the interval_union caliber (mutation M-b bites here): %+v", ioFam)
	}
	if ioFam.MemberSumMs == 0 || ioFam.CumulativeImpactMs >= ioFam.MemberSumMs {
		t.Fatalf("union < Σ must stay disclosed via MemberSumMs: %+v", ioFam)
	}
}

func TestRCMGenericFamilyIntervalUnionCaliber(t *testing.T) {
	thread := ThreadRef{Comm: "io", PID: 300}
	a := rcmRankItem("io_latency", thread, 2.0, 10.010, 10.012, 100, 101)
	b := rcmRankItem("io_latency", thread, 2.0, 10.011, 10.013, 102, 103) // overlaps 1ms
	items := foldSameThreadTypeRankFamilies(Query{TimeStart: 10.0, TimeEnd: 10.2}, false, []RootCauseRankItem{a, b})
	if len(items) != 1 {
		t.Fatalf("merge expected: %+v", items)
	}
	got := items[0]
	if got.MemberFoldCaliber != RootCauseMemberFoldCaliberIntervalUnion {
		t.Fatalf("value==interval-length members with overlap ride the union caliber: %+v", got)
	}
	if got.CumulativeImpactMs < 2.999 || got.CumulativeImpactMs > 3.001 {
		t.Fatalf("union deduplicates the overlap (3.0ms, never the naive 4.0 Σ — mutation M-b): %+v", got)
	}
	if got.MemberSumMs < 3.999 || got.MemberSumMs > 4.001 {
		t.Fatalf("raw Σ disclosure: %+v", got)
	}
}

func TestRCMStateFamilyIntervalUnionKeepsEffectiveScoreAndIdempotence(t *testing.T) {
	thread := ThreadRef{Comm: "worker", PID: 300}
	q := Query{TimeStart: 10.0, TimeEnd: 10.2}
	tests := []struct {
		name string
		typ  string
	}{
		{name: "runnable", typ: "runnable_wait"},
		{name: "D IO", typ: "d_state_or_io_wait"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := rcmRankItem(tc.typ, thread, 2.0, 10.010, 10.012, 100, 101)
			b := rcmRankItem(tc.typ, thread, 2.0, 10.011, 10.013, 102, 103)
			once := foldSameThreadTypeRankFamilies(q, false, []RootCauseRankItem{a, b})
			if len(once) != 1 || once[0].MemberFoldCaliber != RootCauseMemberFoldCaliberIntervalUnion {
				t.Fatalf("overlapping state members must publish one union family: %+v", once)
			}
			if got := rootCauseEffectiveImpactMs(once[0]); !near(got, 3, 0.000001) {
				t.Fatalf("union effective must remain authoritative after state scalars clear, got %.6f: %+v", got, once[0])
			}
			if once[0].Score <= 0 {
				t.Fatalf("positive union effective must derive a positive score: %+v", once[0])
			}
			twice := foldSameThreadTypeRankFamilies(q, false, append([]RootCauseRankItem(nil), once...))
			if !reflect.DeepEqual(once, twice) {
				t.Fatalf("state union family must remain idempotent:\nonce=%+v\ntwice=%+v", once, twice)
			}
		})
	}
}

func TestRCMBackgroundSingletonAndFamilyUseSamePublishedCap(t *testing.T) {
	q := Query{TimeStart: 10.0, TimeEnd: 10.1}
	thread := ThreadRef{PID: 300, Comm: "background"}
	single := rcmRankItem("runnable_wait", thread, 80, 10.010, 10.090, 100, 103)
	single.ImpactMs = backgroundImpactMs(q, 80, true, false)

	a := rcmRankItem("runnable_wait", thread, 40, 10.010, 10.050, 100, 101)
	b := rcmRankItem("runnable_wait", thread, 40, 10.050, 10.090, 102, 103)
	a.ImpactMs = backgroundImpactMs(q, 40, true, false)
	b.ImpactMs = backgroundImpactMs(q, 40, true, false)
	family := foldSameThreadTypeRankFamilies(q, true, []RootCauseRankItem{a, b})
	if len(family) != 1 {
		t.Fatalf("fixture must form one background family: %+v", family)
	}
	want := backgroundImpactMs(q, 80, true, false)
	if got := rootCauseEffectiveImpactMs(single); !near(got, want, 0.000001) {
		t.Fatalf("singleton background effective must honor published cap: got %.3f want %.3f %+v", got, want, single)
	}
	if got := rootCauseEffectiveImpactMs(family[0]); !near(got, want, 0.000001) {
		t.Fatalf("family and singleton must be representation-invariant: got %.3f want %.3f %+v", got, want, family[0])
	}
}

func TestRCMGenericFamilyCompositeOverlapFallsBackToMax(t *testing.T) {
	// block_io values are composite scores, NOT interval lengths — an overlap
	// without a usable union deduction publishes the member MAX (honest lower
	// bound), never a naive Σ.
	thread := ThreadRef{Comm: "io", PID: 300}
	a := rcmRankItem("block_io_by_inode", thread, 5.0, 10.010, 10.012, 100, 101) // value 5.0 ≠ 2ms interval
	b := rcmRankItem("block_io_by_inode", thread, 3.0, 10.011, 10.013, 102, 103)
	items := foldSameThreadTypeRankFamilies(Query{TimeStart: 10.0, TimeEnd: 10.2}, false, []RootCauseRankItem{a, b})
	if len(items) != 1 {
		t.Fatalf("merge expected: %+v", items)
	}
	got := items[0]
	if got.MemberFoldCaliber != RootCauseMemberFoldCaliberMaxOverlapFallback {
		t.Fatalf("composite overlap → max fallback: %+v", got)
	}
	if got.CumulativeImpactMs != 5.0 || got.MemberSumMs != 8.0 {
		t.Fatalf("published MAX with Σ disclosure: %+v", got)
	}
}

func TestRCMCountClassFamilySumsRegardlessOfOverlap(t *testing.T) {
	thread := ThreadRef{Comm: "cache", PID: 300}
	a := rcmRankItem("page_cache_churn", thread, 3.0, 10.010, 10.012, 100, 101)
	b := rcmRankItem("page_cache_churn", thread, 2.0, 10.011, 10.013, 102, 103) // overlap irrelevant for counts
	items := foldSameThreadTypeRankFamilies(Query{TimeStart: 10.0, TimeEnd: 10.2}, false, []RootCauseRankItem{a, b})
	if len(items) != 1 {
		t.Fatalf("merge expected: %+v", items)
	}
	got := items[0]
	if got.MemberFoldCaliber != RootCauseMemberFoldCaliberCountSum || got.CumulativeImpactMs != 5.0 {
		t.Fatalf("count-class members always Σ (count_sum): %+v", got)
	}
}

func TestRCMCrossWindowSameThreadTypeRowsNeverMerge(t *testing.T) {
	// M3 跨窗纪律 (mutation M-c bites here): same (thread,type), DIFFERENT
	// typed selected windows — the rows stay separate lines (RN-12 伪包含
	// same-type defense: cross-window re-measurements never double-sum).
	thread := ThreadRef{Comm: "io", PID: 300}
	a := rcmRankItem("io_latency", thread, 2.0, 10.010, 10.012, 100, 101)
	b := rcmRankItem("io_latency", thread, 2.0, 10.050, 10.052, 102, 103)
	b.StatsWindowStartTs, b.StatsWindowEndTs = 11.0, 11.2
	items := foldSameThreadTypeRankFamilies(Query{TimeStart: 10.0, TimeEnd: 10.2}, false, []RootCauseRankItem{a, b})
	if len(items) != 2 {
		t.Fatalf("cross-window rows must never merge: %+v", items)
	}
	for _, item := range items {
		if item.MemberCount != 0 {
			t.Fatalf("no family accounting on unmerged rows: %+v", item)
		}
	}
}

func TestRCMLaneNeverCrossMerges(t *testing.T) {
	thread := ThreadRef{Comm: "io", PID: 300}
	a := rcmRankItem("io_latency", thread, 2.0, 10.010, 10.012, 100, 101)
	a.Causality = "on_wakeup_chain"
	a.ChainRelevance = "on_chain"
	b := rcmRankItem("io_latency", thread, 2.0, 10.050, 10.052, 102, 103)
	items := foldSameThreadTypeRankFamilies(Query{TimeStart: 10.0, TimeEnd: 10.2}, false, []RootCauseRankItem{a, b})
	if len(items) != 2 {
		t.Fatalf("道别键: on-chain and background rows of one (thread,type) never merge: %+v", items)
	}
}

func TestRCMCrossThreadSameTypeRowsNeverMerge(t *testing.T) {
	a := rcmRankItem("io_latency", ThreadRef{Comm: "io1", PID: 300}, 2.0, 10.010, 10.012, 100, 101)
	b := rcmRankItem("io_latency", ThreadRef{Comm: "io2", PID: 301}, 2.0, 10.050, 10.052, 102, 103)
	items := foldSameThreadTypeRankFamilies(Query{TimeStart: 10.0, TimeEnd: 10.2}, false, []RootCauseRankItem{a, b})
	if len(items) != 2 {
		t.Fatalf("墙钟红线: wall clock never sums across threads — cross-thread rows never merge: %+v", items)
	}
}

func TestRCMFamilyFoldIdempotent(t *testing.T) {
	thread := ThreadRef{Comm: "io", PID: 300}
	a := rcmRankItem("io_latency", thread, 2.0, 10.010, 10.012, 100, 101)
	b := rcmRankItem("io_latency", thread, 1.0, 10.020, 10.021, 102, 103)
	q := Query{TimeStart: 10.0, TimeEnd: 10.2}
	once := foldSameThreadTypeRankFamilies(q, false, []RootCauseRankItem{a, b})
	twice := foldSameThreadTypeRankFamilies(q, false, append([]RootCauseRankItem(nil), once...))
	if !reflect.DeepEqual(once, twice) {
		t.Fatalf("the enrich pass re-folds — the pass must be idempotent:\nonce=%+v\ntwice=%+v", once, twice)
	}
}

func TestRCMFamilyFoldRegistryLaneGolden(t *testing.T) {
	// Registry declaration golden (§24.7.1 普查 + §24.10): exactly these
	// tokens fold, on exactly these lanes. A new per-instance rank family must
	// be adjudicated into this declaration — never hardcoded at a fold site.
	golden := map[string]CausalTokenFamilyFold{
		"io_latency":         CausalFamilyFoldSameThreadType,
		"block_io_by_inode":  CausalFamilyFoldSameThreadType,
		"file_io_hot_inode":  CausalFamilyFoldSameThreadType,
		"page_cache_churn":   CausalFamilyFoldSameThreadType,
		"io_burst_episode":   CausalFamilyFoldSameThreadType,
		"workqueue_activity": CausalFamilyFoldSameThreadType,
		"dma_fence_activity": CausalFamilyFoldSameThreadType,
		"scheduler_latency":  CausalFamilyFoldSameThreadType,
		// EVOLUTION RECORD (ORD, ledger §29.11 补充 cap2 观察① + §24.7.1
		// 通用规则, 2026-07-10): the off-CPU top families joined the lane —
		// the former "one-row-per-thread" exclusion premise was falsified by
		// the cap2 production witness (computeOffCPUStats buckets keyed
		// (pid, comm, CPU): one thread held seats #1/#2/#3 as runnable_wait
		// cpu=1/3/2). The CPU rides the member roster as the 区分键; the
		// runnable inversion retype folds on the same lane.
		"runnable_wait":                    CausalFamilyFoldSameThreadType,
		"priority_inversion_runnable_wait": CausalFamilyFoldSameThreadType,
		"sleep_wait":                       CausalFamilyFoldSameThreadType,
		"io_wait":                          CausalFamilyFoldSameThreadType,
		"d_state_or_io_wait":               CausalFamilyFoldSameThreadType,
		"jit_compile":                      CausalFamilyFoldSemanticClass,
		"class_verification":               CausalFamilyFoldSemanticClass,
		"shader_compile":                   CausalFamilyFoldSemanticClass,
		"runtime_compile":                  CausalFamilyFoldSemanticClass,
		"gc_pause":                         CausalFamilyFoldSemanticClass,
		// TEX (§28.1, 2026-07-09): fifth semantic class, same fold lane.
		"texture_upload": CausalFamilyFoldSemanticClass,
	}
	for _, token := range CausalTokenUniverse() {
		want := golden[token]
		if got := CausalTokenFamilyFoldLane(token); got != want {
			t.Fatalf("family-fold lane drift for %q: got %q want %q (read the §24.7.1/§24.10 rulings before moving a lane)", token, got, want)
		}
	}
	for token := range golden {
		if _, ok := CausalTokenSpecFor(token); !ok {
			t.Fatalf("family-fold declaration for unregistered token %q", token)
		}
	}
}

func TestRCMHuadongShapeSchedulerBoardComparison(t *testing.T) {
	// Third specimen-shaped 对照 (M4, huadong ladder form): a thread whose
	// scheduling pressure is split across many small scheduler_latency
	// segments loses the board to a single mid-size IO row pre-merge; the
	// family total restores the honest order. Direction pin: the family
	// position is at least as good as the best single member's.
	app := ThreadRef{Comm: "app", PID: 100}
	mkSched := func(raw, start float64, l1, l2 int) RootCauseRankItem {
		return rcmRankItem("scheduler_latency", app, raw, start, start+raw/1000, l1, l2)
	}
	segments := []RootCauseRankItem{
		mkSched(0.8, 10.010, 100, 101),
		mkSched(0.7, 10.020, 102, 103),
		mkSched(0.6, 10.030, 104, 105),
	}
	competitor := rcmRankItem("io_latency", ThreadRef{Comm: "io", PID: 300}, 1.5, 10.040, 10.0415, 200, 201)
	dumpBoard := func(items []RootCauseRankItem) (int, []string) {
		normalizeRootCauseCumulativeImpact(items)
		normalizeRootCauseEffectiveImpact(items)
		sortRootCauseRankItems(items, false)
		best := -1
		var out []string
		for i, item := range items {
			out = append(out, item.Type)
			if item.Type == "scheduler_latency" && best == -1 {
				best = i
			}
		}
		return best, out
	}
	before := append(append([]RootCauseRankItem(nil), segments...), competitor)
	beforePos, beforeBoard := dumpBoard(before)
	after := foldSameThreadTypeRankFamilies(Query{TimeStart: 10.0, TimeEnd: 10.2}, false,
		append(append([]RootCauseRankItem(nil), segments...), competitor))
	afterPos, afterBoard := dumpBoard(after)
	t.Logf("board BEFORE (split): pos=%d %v", beforePos, beforeBoard)
	t.Logf("board AFTER  (family): pos=%d %v", afterPos, afterBoard)
	if beforePos == 0 {
		t.Fatalf("specimen shape requires the split segments to LOSE the top seat pre-merge: %v", beforeBoard)
	}
	if afterPos > beforePos {
		t.Fatalf("合并量参赛: family position (%d) must be at least as good as the best split position (%d)", afterPos, beforePos)
	}
	if afterPos != 0 {
		t.Fatalf("the 2.1ms family must outrank the 1.5ms competitor: %v", afterBoard)
	}
}

func TestRCMSchedulerLatencyFamilyMergesInEnrichShape(t *testing.T) {
	// The enrich pass mints multiple scheduler_latency segments per thread —
	// same fold, same key discipline.
	thread := ThreadRef{Comm: "app", PID: 100}
	a := rcmRankItem("scheduler_latency", thread, 1.2, 10.010, 10.0112, 100, 101)
	b := rcmRankItem("scheduler_latency", thread, 0.8, 10.020, 10.0208, 102, 103)
	items := foldSameThreadTypeRankFamilies(Query{TimeStart: 10.0, TimeEnd: 10.2}, false, []RootCauseRankItem{a, b})
	if len(items) != 1 || items[0].MemberCount != 2 {
		t.Fatalf("scheduler_latency segments of one thread merge: %+v", items)
	}
	if items[0].CumulativeImpactMs < 1.999 || items[0].CumulativeImpactMs > 2.001 {
		t.Fatalf("disjoint segments sum: %+v", items[0])
	}
}

func TestRCMMergedFamilyStateScalarsFollowCaliber(t *testing.T) {
	// F3 (对抗复核收尾, 2026-07-08): the merged row's per-state scalar
	// channels and DominantState re-derive under the SAME caliber ruler as the
	// published value — the base member's slice must never survive under a Σ
	// total ("cumulative=2.0 while runnable=1.2" self-contradiction, the D-3
	// fold-inheritance class).
	thread := ThreadRef{Comm: "app", PID: 100}
	q := Query{TimeStart: 10.0, TimeEnd: 10.2}

	// (a) sum_disjoint: channels Σ; agreeing dominant word survives.
	a := rcmRankItem("scheduler_latency", thread, 1.2, 10.010, 10.0112, 100, 101)
	a.DominantState = string(StateRunnable)
	a.RunnableMs = 1.2
	b := rcmRankItem("scheduler_latency", thread, 0.8, 10.020, 10.0208, 102, 103)
	b.DominantState = string(StateRunnable)
	b.RunnableMs = 0.8
	merged := foldSameThreadTypeRankFamilies(q, false, []RootCauseRankItem{a, b})
	if len(merged) != 1 || merged[0].MemberFoldCaliber != RootCauseMemberFoldCaliberSumDisjoint {
		t.Fatalf("disjoint fixture expected: %+v", merged)
	}
	if merged[0].RunnableMs < 1.999 || merged[0].RunnableMs > 2.001 {
		t.Fatalf("sum_disjoint channels must Σ with the published value (2.0), not inherit the base slice (1.2): %+v", merged[0])
	}
	if merged[0].DominantState != string(StateRunnable) {
		t.Fatalf("an agreeing dominant word survives: %+v", merged[0])
	}

	// (b) interval_union: channels zero (per-channel union not derivable).
	c := rcmRankItem("io_latency", thread, 2.0, 10.010, 10.012, 100, 101)
	c.DominantState = string(StateIOWait)
	c.IOWaitMs = 2.0
	d := rcmRankItem("io_latency", thread, 2.0, 10.011, 10.013, 102, 103)
	d.DominantState = string(StateIOWait)
	d.IOWaitMs = 2.0
	merged = foldSameThreadTypeRankFamilies(q, false, []RootCauseRankItem{c, d})
	if len(merged) != 1 || merged[0].MemberFoldCaliber != RootCauseMemberFoldCaliberIntervalUnion {
		t.Fatalf("union fixture expected: %+v", merged)
	}
	if merged[0].IOWaitMs != 0 || merged[0].RunnableMs != 0 {
		t.Fatalf("interval_union channels must zero — absence over a fabricated per-channel number: %+v", merged[0])
	}

	// (c) max_overlap_fallback: the base member's own channels stay (the
	// published value IS that measurement); disagreeing dominant words clear.
	e := rcmRankItem("block_io_by_inode", thread, 5.0, 10.010, 10.012, 100, 101)
	e.DominantState = string(StateDSleep)
	e.DStateMs = 1.5
	f := rcmRankItem("block_io_by_inode", thread, 3.0, 10.011, 10.013, 102, 103)
	f.DominantState = string(StateIOWait)
	merged = foldSameThreadTypeRankFamilies(q, false, []RootCauseRankItem{e, f})
	if len(merged) != 1 || merged[0].MemberFoldCaliber != RootCauseMemberFoldCaliberMaxOverlapFallback {
		t.Fatalf("max fallback fixture expected: %+v", merged)
	}
	if merged[0].DStateMs != 1.5 {
		t.Fatalf("max fallback keeps the base member's own channels: %+v", merged[0])
	}
	if merged[0].DominantState != "" {
		t.Fatalf("disagreeing dominant words must clear on the merged row: %+v", merged[0])
	}
}

// --- E3/E4 interactions ---------------------------------------------------------

func TestRCMFailLoudCaveatCountsFamilies(t *testing.T) {
	worker := ThreadRef{Comm: "worker", PID: 200}
	other := ThreadRef{Comm: "other", PID: 300}
	stats := WindowStats{TraceSpans: []TraceSpanSummary{
		rcmSpan(worker, "VerifyClass com.example.A", 5.0010, 5.0030, 10, 11),
		rcmSpan(worker, "VerifyClass com.example.B", 5.0040, 5.0045, 12, 13),
		rcmSpan(other, "JIT compiling void a.b", 5.0050, 5.0080, 14, 15),
	}}
	caveat, ok := semanticSpanRankFailLoudCaveat(stats, nil)
	if !ok {
		t.Fatalf("classified spans + zero published semantic rows must fail loud")
	}
	if !strings.Contains(caveat, "3 classified semantic optimization span(s) across 2 same-thread famil") {
		t.Fatalf("the fail-loud count is family-grained (§24.12): %q", caveat)
	}
	published := []RootCauseRankItem{{Type: "class_verification"}}
	if _, ok := semanticSpanRankFailLoudCaveat(stats, published); ok {
		t.Fatalf("any published semantic family silences the caveat")
	}
}

func TestRCMSemanticCapExactlyFullDisclosesLowerBound(t *testing.T) {
	worker := ThreadRef{Comm: "worker", PID: 200}
	var spans []TraceSpanSummary
	start := 5.0010
	for i := 0; i < traceMarkSemanticSpanCap; i++ {
		spans = append(spans, rcmSpan(worker, "VerifyClass com.example.Klass", start, start+0.0005, 100+i, 100+i))
		start += 0.0010
	}
	caveat, ok := semanticSpanCapLowerBoundCaveat(WindowStats{TraceSpans: spans})
	if !ok || !strings.Contains(caveat, "lower bound") {
		t.Fatalf("an exactly-full semantic cap must disclose the ≥ lower-bound semantics: %q ok=%v", caveat, ok)
	}
	if _, ok := semanticSpanCapLowerBoundCaveat(WindowStats{TraceSpans: spans[:traceMarkSemanticSpanCap-1]}); ok {
		t.Fatalf("below the cap there is nothing to disclose")
	}
}

func TestRCMBackgroundRankCountsFamilyOnce(t *testing.T) {
	// E6 mention-gate interaction: the typed background board position counts
	// a merged family as ONE row — ×N members never inflate the non-chain
	// positions below them (§24.10: merged rows occupy fewer, larger board
	// positions; the background_rank<=3 gate semantics follow the family).
	fam := rcmRankItem("class_verification", ThreadRef{Comm: "verify", PID: 200}, 7.124, 10.0, 10.09, 300, 340)
	fam.MemberCount = 14
	fam.SpanName = "VerifyClass com.example.Klass"
	fam.SemanticClass = "class_verification"
	bg1 := rcmRankItem("runnable_wait", ThreadRef{Comm: "bg1", PID: 401}, 9.0, 10.0, 10.009, 100, 101)
	bg2 := rcmRankItem("io_latency", ThreadRef{Comm: "bg2", PID: 402}, 5.0, 10.0, 10.005, 102, 103)
	items := []RootCauseRankItem{bg1, fam, bg2}
	assignRootCauseRanksAndTiers(items)
	if items[1].BackgroundRank != 2 {
		t.Fatalf("the ×14 family occupies exactly ONE background position (#2 behind bg1): %+v", items[1])
	}
	if items[0].BackgroundRank != 0 || items[2].BackgroundRank != 0 {
		t.Fatalf("the field stays semantic-row-scoped: %+v", items)
	}
}

func TestRCMTruncationDoesNotReserveFamilySeat(t *testing.T) {
	// A folded semantic family remains one contender, but it receives no
	// type-specific capacity reservation. The strict effective prefix decides.
	var items []RootCauseRankItem
	for i := 0; i < 12; i++ {
		items = append(items, rcmRankItem("runnable_wait", ThreadRef{Comm: "bg", PID: 400 + i}, 10.0-float64(i)*0.1, 10.0, 10.01, 100+i, 100+i))
	}
	fam := rcmRankItem("class_verification", ThreadRef{Comm: "verify", PID: 200}, 7.124, 10.0, 10.09, 300, 340)
	fam.MemberCount = 14
	fam.MemberFoldCaliber = RootCauseMemberFoldCaliberSumDisjoint
	fam.SpanName = "VerifyClass com.example.Klass"
	fam.SemanticClass = "class_verification"
	items = append(items, fam)
	for i := range items {
		if items[i].Type == "runnable_wait" {
			items[i].RunnableMs = items[i].ImpactMs
		}
		if items[i].Type == "class_verification" {
			items[i].EffectiveImpactMs = items[i].ImpactMs
		}
	}
	sortRootCauseRankItems(items, false)
	kept := truncateRootCauseRankItemsStrict(items, 12)
	if len(kept) != 12 {
		t.Fatalf("capacity holds: %d", len(kept))
	}
	seated := 0
	for _, item := range kept {
		if item.Type == "class_verification" {
			seated++
			if item.MemberCount != 14 {
				t.Fatalf("the seated row is the family row: %+v", item)
			}
		}
	}
	if seated != 0 {
		t.Fatalf("the below-cut ×14 family must not displace a larger effective cause: %d", seated)
	}
}
