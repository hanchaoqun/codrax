package types

// trace_causal_projection_ptv5_test.go — PTV5 #68 (用户裁定 2026-07-05) pins:
//
//   PTS  — on-chain bucket-cap overflow folds with a count, ZERO silent drops
//          (合成超 cap pin + 突变形态: ≤cap compiles byte-identical, no fold).
//   PTS  — a producer-side fold record (typed folded_* notes) re-materializes
//          as the fold node (MergedCount/range/roster).
//   Q4   — R1 absorb backfills the loser's Object into an EMPTY survivor
//          Object (root_evidence 类型 token 回填), while conflicting non-empty
//          Objects keep the 影响点 lane exactly as before.
//   Q4   — priority_inversion_candidate is a typed node field (exact "true"
//          note match; absorb ORs it across merged views).
//   Q3   — QueryWindows collects the DISTINCT typed selected_window pairs
//          (±1ms dedupe, ascending, display-only).

import (
	"fmt"
	"strings"
	"testing"
)

func ptv5OnChainRecord(i int, impact float64) ObservationRecord {
	return ObservationRecord{
		ID: fmt.Sprintf("trace_query:x#root_cause_ctx:%d", i), Origin: AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: ClaimGroundingHard,
		Predicate: "root_cause_context", ClaimKey: fmt.Sprintf("root_cause_context:t%d", i),
		Subject: fmt.Sprintf("worker-%d", i), Object: "runnable_wait",
		Value: fmt.Sprintf("%.3f", impact), Unit: "ms",
		Span: ObservationSpan{LineStart: 100 + i, LineEnd: 100 + i},
		SourceRef: ObservationSourceRef{
			Kind: ObservationSourceRuntimeArtifact, Path: "ptv5.systrace", ArtifactKind: "trace",
		},
		RichNotes: []string{
			"chain_relevance=on_chain",
			fmt.Sprintf("impact_ms=%.3f", impact),
		},
		Confidence: 0.8,
	}
}

func TestTraceCausalProjectionOnChainOverflowFoldsWithCount(t *testing.T) {
	const total = 30 // > traceCausalProjectionOnChainLimit (24)
	var records []ObservationRecord
	for i := 0; i < total; i++ {
		records = append(records, ptv5OnChainRecord(i, 100.0-float64(i))) // impact-major already
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	if len(got.OnChainCauses) != traceCausalProjectionOnChainLimit+1 {
		t.Fatalf("overflow must keep %d rows + 1 fold row, got %d", traceCausalProjectionOnChainLimit, len(got.OnChainCauses))
	}
	var fold *TraceCausalProjectionNode
	for i := range got.OnChainCauses {
		if got.OnChainCauses[i].OnChainOverflowFold {
			if fold != nil {
				t.Fatalf("exactly one fold row expected")
			}
			fold = &got.OnChainCauses[i]
		}
	}
	if fold == nil {
		t.Fatalf("overflow fold row missing: %+v", got.OnChainCauses)
	}
	if fold.MergedCount != total-traceCausalProjectionOnChainLimit {
		t.Fatalf("fold must COUNT the folded rows: got %d want %d", fold.MergedCount, total-traceCausalProjectionOnChainLimit)
	}
	if fold.ChainRelevance != "on_chain" || strings.TrimSpace(fold.Subject) != "" {
		t.Fatalf("fold row must stay a subjectless on-chain row: %+v", fold)
	}
	// Value = member MAX (wall clock never sums across threads).
	if fold.ImpactMS != fold.MergedMaxMS || fold.MergedMaxMS <= 0 || fold.MergedMinMS > fold.MergedMaxMS {
		t.Fatalf("fold value must be the member MAX with a sane range: %+v", fold)
	}
	// ZERO silent drops: every record's evidence id survives on some node
	// (kept row or fold's merged evidence).
	seen := map[string]bool{}
	for _, node := range got.OnChainCauses {
		seen[node.EvidenceID] = true
		for _, id := range node.MergedEvidenceIDs {
			seen[id] = true
		}
	}
	for _, record := range records {
		if !seen[record.ID] {
			t.Fatalf("record %s silently dropped from the on-chain bucket", record.ID)
		}
	}
	// The fold roster keeps up to the merged-subject cap of member names.
	if len(fold.MergedSubjects) == 0 {
		t.Fatalf("fold row must keep member thread names: %+v", fold)
	}
}

func TestTraceCausalProjectionOnChainUnderCapHasNoFoldRow(t *testing.T) {
	// 突变形态: at exactly the cap, the bucket stays fold-free (byte-identical
	// to the plain limiter).
	var records []ObservationRecord
	for i := 0; i < traceCausalProjectionOnChainLimit; i++ {
		records = append(records, ptv5OnChainRecord(i, 100.0-float64(i)))
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	if len(got.OnChainCauses) != traceCausalProjectionOnChainLimit {
		t.Fatalf("under-cap bucket must keep every row: %d", len(got.OnChainCauses))
	}
	for _, node := range got.OnChainCauses {
		if node.OnChainOverflowFold {
			t.Fatalf("under-cap bucket must not mint a fold row: %+v", node)
		}
	}
}

func TestTraceCausalProjectionFoldedNotesRematerializeFoldNode(t *testing.T) {
	record := ptv5OnChainRecord(0, 12.0)
	record.Subject = ""
	record.ClaimKey = "wakeup_causal_impact:folded_overflow"
	record.Predicate = "wakeup_causal_impact"
	record.RichNotes = []string{
		"chain_relevance=on_chain",
		"impact=12.000",
		"folded_rows=3",
		"folded_min_ms=2.000",
		"folded_max_ms=12.000",
		"folded_subjects=of-1,of-2",
	}
	got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{record, ptv5OnChainRecord(1, 50)})
	var fold *TraceCausalProjectionNode
	for i := range got.OnChainCauses {
		if got.OnChainCauses[i].OnChainOverflowFold {
			fold = &got.OnChainCauses[i]
		}
	}
	if fold == nil {
		t.Fatalf("folded_* notes must re-materialize the fold node: %+v", got.OnChainCauses)
	}
	if fold.MergedCount != 3 || fold.MergedMinMS != 2.0 || fold.MergedMaxMS != 12.0 {
		t.Fatalf("fold accounting must ride the typed notes: %+v", fold)
	}
	if len(fold.MergedSubjects) != 2 || fold.MergedSubjects[0] != "of-1" {
		t.Fatalf("folded_subjects roster must parse: %+v", fold.MergedSubjects)
	}
}

func TestTraceCausalProjectionAbsorbBackfillsEmptyObject(t *testing.T) {
	survivor := TraceCausalProjectionNode{Subject: "w-1", Object: ""}
	loser := TraceCausalProjectionNode{Subject: "w-1", Object: "lock_contention", EvidenceID: "e-2"}
	traceCausalProjectionAbsorbSameFact(&survivor, loser, map[string]bool{}, nil)
	if survivor.Object != "lock_contention" {
		t.Fatalf("empty survivor Object must take the loser's cause token: %+v", survivor)
	}
	if len(survivor.SecondaryObjects) != 0 {
		t.Fatalf("the backfilled Object must not double as an 影响点: %+v", survivor.SecondaryObjects)
	}
	// 突变形态: conflicting non-empty Objects keep the survivor's and record
	// the loser's as an 影响点, exactly as before.
	survivor2 := TraceCausalProjectionNode{Subject: "w-1", Object: "binder_wait"}
	traceCausalProjectionAbsorbSameFact(&survivor2, loser, map[string]bool{}, nil)
	if survivor2.Object != "binder_wait" || len(survivor2.SecondaryObjects) != 1 || survivor2.SecondaryObjects[0] != "lock_contention" {
		t.Fatalf("conflicting Objects must keep the 影响点 lane: %+v", survivor2)
	}
}

func TestTraceCausalProjectionPriorityInversionCandidateTypedField(t *testing.T) {
	record := ptv5OnChainRecord(0, 5.0)
	record.Predicate = "wakeup_causal_impact"
	record.ClaimKey = "wakeup_causal_impact:worker-0"
	record.Object = "s_sleep"
	record.RichNotes = append(record.RichNotes, "priority_inversion_candidate=true")
	got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{record})
	if len(got.OnChainCauses) != 1 || !got.OnChainCauses[0].PriorityInversionCandidate {
		t.Fatalf("the typed note must set the node field: %+v", got.OnChainCauses)
	}
	// 突变形态: absence (and non-"true" values) leave the field false.
	record.RichNotes = record.RichNotes[:len(record.RichNotes)-1]
	got = TraceCausalProjectionFromObservationRecords([]ObservationRecord{record})
	if len(got.OnChainCauses) != 1 || got.OnChainCauses[0].PriorityInversionCandidate {
		t.Fatalf("no note → field stays false: %+v", got.OnChainCauses)
	}
	// Absorb ORs the candidacy across merged views of one fact.
	survivor := TraceCausalProjectionNode{Subject: "w-1"}
	traceCausalProjectionAbsorbSameFact(&survivor, TraceCausalProjectionNode{PriorityInversionCandidate: true}, map[string]bool{}, nil)
	if !survivor.PriorityInversionCandidate {
		t.Fatalf("absorb must keep the candidacy of the merged fact")
	}
}

func TestTraceCausalProjectionQueryWindowsCollectDistinct(t *testing.T) {
	a := ptv5OnChainRecord(0, 5.0)
	a.RichNotes = append(a.RichNotes, "selected_window=100.000..101.000")
	b := ptv5OnChainRecord(1, 4.0)
	b.RichNotes = append(b.RichNotes, "selected_window=100.0004..101.0004") // ±1ms dedupe → same window
	c := ptv5OnChainRecord(2, 3.0)
	c.RichNotes = append(c.RichNotes, "selected_window=200.000..203.000")
	got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{c, a, b})
	if len(got.QueryWindows) != 2 {
		t.Fatalf("distinct windows must dedupe at ±1ms: %+v", got.QueryWindows)
	}
	if got.QueryWindows[0].StartTs != 100.0 || got.QueryWindows[1].StartTs != 200.0 {
		t.Fatalf("windows must sort ascending by start: %+v", got.QueryWindows)
	}
	// 突变形态: no parseable note → empty list.
	d := ptv5OnChainRecord(3, 2.0)
	got = TraceCausalProjectionFromObservationRecords([]ObservationRecord{d})
	if len(got.QueryWindows) != 0 {
		t.Fatalf("no selected_window notes → no query windows: %+v", got.QueryWindows)
	}
}

// --- 复核 P1 簇一 (2026-07-06) --------------------------------------------------

// TestTraceCausalProjectionOnChainFoldCountsPostAggregationTruth pins the fold
// AFTER R1/R4: duplicate publications of one fact (same subject+impact+lines,
// different evidence id/object — the R1 same-fact shape) must NOT inflate the
// fold count. 26 distinct facts + 4 R1-duplicates → post-merge 26 rows → fold
// counts exactly 2 (the pre-aggregation fold would have claimed 6).
func TestTraceCausalProjectionOnChainFoldCountsPostAggregationTruth(t *testing.T) {
	const distinct = 26
	var records []ObservationRecord
	for i := 0; i < distinct; i++ {
		records = append(records, ptv5OnChainRecord(i, 100.0-float64(i)))
	}
	for i := 0; i < 4; i++ {
		dup := ptv5OnChainRecord(i, 100.0-float64(i))
		dup.ID = fmt.Sprintf("trace_query:x#root_cause_dup:%d", i)
		dup.ClaimKey = fmt.Sprintf("root_cause_context:dup%d", i)
		dup.Object = "sleep_wait" // different node key → survives dedupe; same R1 fact key → merges
		records = append(records, dup)
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	var fold *TraceCausalProjectionNode
	rows := 0
	for i := range got.OnChainCauses {
		if got.OnChainCauses[i].OnChainOverflowFold {
			fold = &got.OnChainCauses[i]
			continue
		}
		rows++
	}
	if fold == nil {
		t.Fatalf("26 post-merge rows must still overflow the 24 cap: %d rows", rows)
	}
	if fold.MergedCount != distinct-traceCausalProjectionOnChainLimit {
		t.Fatalf("fold must count the POST-AGGREGATION truth (%d), got %d — duplicates re-inflated the count",
			distinct-traceCausalProjectionOnChainLimit, fold.MergedCount)
	}
	// The duplicates' evidence ids still survive (absorbed onto their R1
	// survivors) — zero silent drops holds across the merge.
	seen := map[string]bool{}
	for _, node := range got.OnChainCauses {
		seen[node.EvidenceID] = true
		for _, id := range node.MergedEvidenceIDs {
			seen[id] = true
		}
	}
	for _, record := range records {
		if !seen[record.ID] {
			t.Fatalf("record %s silently dropped across merge+fold", record.ID)
		}
	}
}

// TestTraceCausalProjectionOnChainFoldCarriesCaliberSource pins 复核 P1 ②: a
// fold whose MAX member has no window projection publishes on the CUMULATIVE
// lane only (ImpactMS stays 0) so the display pipeline prints the C00 caliber
// word and never a window share; a window-projection max keeps the old lanes.
func TestTraceCausalProjectionOnChainFoldCarriesCaliberSource(t *testing.T) {
	build := func(n int) []TraceCausalProjectionNode {
		var nodes []TraceCausalProjectionNode
		for i := 0; i < n; i++ {
			nodes = append(nodes, TraceCausalProjectionNode{
				Subject: fmt.Sprintf("w-%d", i), Object: "runnable_wait",
				ChainRelevance: "on_chain", ImpactMS: 100 - float64(i),
				EvidenceID: fmt.Sprintf("e-%d", i), Confidence: 0.8,
			})
		}
		return nodes
	}
	// Cumulative-only overflow: the max member carries no ImpactMS.
	nodes := build(traceCausalProjectionOnChainLimit)
	nodes = append(nodes,
		TraceCausalProjectionNode{Subject: "cum-1", Object: "runnable_wait", ChainRelevance: "on_chain",
			CumulativeImpactMS: 55.0, EvidenceID: "e-cum-1", Confidence: 0.8},
		TraceCausalProjectionNode{Subject: "cum-2", Object: "runnable_wait", ChainRelevance: "on_chain",
			CumulativeImpactMS: 12.0, EvidenceID: "e-cum-2", Confidence: 0.8})
	out := traceCausalProjectionLimitNodesOnChainFold(nodes, traceCausalProjectionOnChainLimit)
	fold := out[len(out)-1]
	if !fold.OnChainOverflowFold || fold.ImpactMS != 0 || fold.CumulativeImpactMS != 55.0 {
		t.Fatalf("cumulative-only max must publish on the cumulative lane only: %+v", fold)
	}
	// 突变形态: a window-projection max keeps ImpactMS (window share legal).
	nodes2 := build(traceCausalProjectionOnChainLimit + 2)
	out2 := traceCausalProjectionLimitNodesOnChainFold(nodes2, traceCausalProjectionOnChainLimit)
	fold2 := out2[len(out2)-1]
	if fold2.ImpactMS != fold2.MergedMaxMS || fold2.ImpactMS <= 0 {
		t.Fatalf("window-projection max keeps the projection lane: %+v", fold2)
	}
}

// --- 复核 Low (2026-07-06) -------------------------------------------------------

// TestTraceCausalProjectionQueryWindowsTruncationFlag: a 9th DISTINCT window
// latches the truncated flag (display faces render ≥8); duplicates beyond the
// cap do NOT (they were never dropped).
func TestTraceCausalProjectionQueryWindowsTruncationFlag(t *testing.T) {
	var records []ObservationRecord
	for i := 0; i < 9; i++ {
		r := ptv5OnChainRecord(i, 5.0)
		r.RichNotes = append(r.RichNotes, fmt.Sprintf("selected_window=%d.000..%d.500", 100+i*10, 100+i*10))
		records = append(records, r)
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	if len(got.QueryWindows) != 8 || !got.QueryWindowsTruncated {
		t.Fatalf("a 9th distinct window must latch the truncated flag: n=%d truncated=%v",
			len(got.QueryWindows), got.QueryWindowsTruncated)
	}
	// 突变形态: a beyond-cap DUPLICATE of a listed window is not a drop.
	records = records[:8]
	dup := ptv5OnChainRecord(99, 5.0)
	dup.RichNotes = append(dup.RichNotes, "selected_window=100.000..100.500")
	records = append(records, dup)
	got = TraceCausalProjectionFromObservationRecords(records)
	if got.QueryWindowsTruncated {
		t.Fatalf("a duplicate window past the cap is not a truncation")
	}
}

// TestTraceCausalProjectionSameWindowToleranceExported: the exported ±1ms
// tolerance is the single authority display consumers read (复核 Low: no
// re-minted literals in internal/tool).
func TestTraceCausalProjectionSameWindowToleranceExported(t *testing.T) {
	if TraceCausalProjectionSameWindowToleranceS != traceCausalProjectionFullWindowSameWindowToleranceS {
		t.Fatalf("exported tolerance must alias the internal authority")
	}
	if TraceCausalProjectionSameWindowToleranceS != 0.001 {
		t.Fatalf("the same-window tolerance is ±1ms (seconds domain): %v", TraceCausalProjectionSameWindowToleranceS)
	}
}
