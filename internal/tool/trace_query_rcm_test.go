package tool

// trace_query_rcm_test.go — RCM tool-half pins (ledger §24.7.1/§24.10/§24.12
// dimension A, real_trace_campaign_20260705.md, 2026-07-08): the typed
// observation channel consumes the SAME semantic family fold as the rank
// minting loop (one function, two consumers), a multi-member family publishes
// ONE record carrying the family total + member accounting, and the rank
// observation face carries the member_*/inode/dev typed notes.
//
// NOTE: key literals in this file are deliberate verbatim wire pins — do not
// replace them with the constants.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func rcmToolSpan(thread tracequery.ThreadRef, name string, startTs, endTs float64, startLine, endLine int) tracequery.TraceSpanSummary {
	return tracequery.TraceSpanSummary{
		Thread:        thread,
		Kind:          "sync",
		Name:          name,
		SemanticClass: "class_verification",
		StartTs:       startTs,
		EndTs:         endTs,
		DurationMs:    (endTs - startTs) * 1000,
		StartLine:     startLine,
		EndLine:       endLine,
	}
}

func TestRCMSemanticObservationFamilyRecordCarriesTotalAndMembers(t *testing.T) {
	worker := tracequery.ThreadRef{Comm: "verify", PID: 200}
	other := tracequery.ThreadRef{Comm: "other", PID: 300}
	stats := tracequery.WindowStats{
		Window: tracequery.TimeWindow{StartTs: 5.0, EndTs: 5.1},
		TraceSpans: []tracequery.TraceSpanSummary{
			rcmToolSpan(worker, "VerifyClass com.example.A", 5.0010, 5.0030, 10, 11), // 2.0ms
			rcmToolSpan(worker, "VerifyClass com.example.B", 5.0040, 5.0055, 12, 13), // 1.5ms
			rcmToolSpan(other, "VerifyClass com.example.D", 5.0060, 5.0070, 16, 17),  // 1.0ms
		},
	}
	records := traceQueryTypedSemanticTraceSpanObservations(tracequery.Result{}, stats,
		types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "a.systrace", ArtifactKind: "trace"},
		"scope", time.Unix(1751600000, 0).UTC().Format(time.RFC3339))
	if len(records) != 2 {
		t.Fatalf("×2 same-thread spans fold into ONE family record beside the other-thread single: %d records", len(records))
	}
	fam := records[0]
	if fam.Value != "3.500" {
		t.Fatalf("the family record Value is the window-projection total: %+v", fam)
	}
	// 两消费方同源: the record total equals the shared fold's TotalMs verbatim.
	families := tracequery.FoldSemanticSpanFamilies(nil, stats.TraceSpans)
	if fam.Value != fmt.Sprintf("%.3f", families[0].TotalMs) {
		t.Fatalf("observation Value and fold TotalMs must be one source: %q vs %.3f", fam.Value, families[0].TotalMs)
	}
	notes := strings.Join(fam.RichNotes, "\n")
	for _, want := range []string{
		"member_count=2",
		"member_max_ms=2.000",
		"member_min_ms=1.500",
		"member_fold_caliber=sum_disjoint",
		"member_roster=VerifyClass com.example.A 2.000ms | VerifyClass com.example.B 1.500ms",
		"selected_window=5.000000..5.100000",
		"span_name=VerifyClass com.example.A",
	} {
		if !strings.Contains(notes, want) {
			t.Fatalf("family record missing %q:\n%s", want, notes)
		}
	}
	if strings.Contains(notes, "member_sum_ms=") {
		t.Fatalf("disjoint members publish no member_sum disclosure (published == Σ): %s", notes)
	}
	if fam.Span.LineStart != 10 || fam.Span.LineEnd != 13 {
		t.Fatalf("the record Span is the member envelope: %+v", fam.Span)
	}
	// The other-thread single keeps the pre-RCM per-span record shape.
	single := records[1]
	if single.Value != "1.000" || strings.Contains(strings.Join(single.RichNotes, "\n"), "member_count=") {
		t.Fatalf("a family of one keeps the byte-stable single-span record: %+v", single)
	}
}

func TestRCMSemanticObservationOverlapFamilyPublishesUnionValueNotSum(t *testing.T) {
	// F1 (对抗复核收尾, 2026-07-08): the two-consumer same-source pin must
	// bite on the OVERLAP shape too — the disjoint shape has SumMs==TotalMs,
	// so a mutation that reroutes the observation Value onto fam.SumMs (the
	// exact caliber lie this batch exists to prevent) survived it. Overlapping
	// same-thread members: the record Value is the interval-union total
	// (3.000), NEVER the raw member Σ (4.000), and the Σ stays disclosed on
	// the typed member_sum_ms note. Mutation replay: Value→fam.SumMs reds
	// here.
	worker := tracequery.ThreadRef{Comm: "verify", PID: 200}
	stats := tracequery.WindowStats{
		Window: tracequery.TimeWindow{StartTs: 5.0, EndTs: 5.1},
		TraceSpans: []tracequery.TraceSpanSummary{
			rcmToolSpan(worker, "VerifyClass com.example.A", 5.0010, 5.0030, 10, 11), // 2.0ms
			rcmToolSpan(worker, "VerifyClass com.example.B", 5.0020, 5.0040, 12, 13), // 2.0ms, overlaps 1ms
		},
	}
	records := traceQueryTypedSemanticTraceSpanObservations(tracequery.Result{}, stats,
		types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "a.systrace", ArtifactKind: "trace"},
		"scope", time.Unix(1751600000, 0).UTC().Format(time.RFC3339))
	if len(records) != 1 {
		t.Fatalf("one family record expected: %d", len(records))
	}
	fam := records[0]
	families := tracequery.FoldSemanticSpanFamilies(nil, stats.TraceSpans)
	if len(families) != 1 || families[0].TotalMs >= families[0].SumMs {
		t.Fatalf("fixture must be a real union<sum overlap shape: %+v", families)
	}
	if fam.Value != fmt.Sprintf("%.3f", families[0].TotalMs) {
		t.Fatalf("observation Value must be the fold's union total: %q vs %.3f", fam.Value, families[0].TotalMs)
	}
	if fam.Value != "3.000" {
		t.Fatalf("union total of the overlap fixture is 3.000ms: %q", fam.Value)
	}
	if fam.Value == fmt.Sprintf("%.3f", families[0].SumMs) {
		t.Fatalf("the record Value must never be the raw member Σ (4.000): %q", fam.Value)
	}
	notes := strings.Join(fam.RichNotes, "\n")
	for _, want := range []string{
		"member_sum_ms=4.000",
		"member_fold_caliber=interval_union",
		"member_count=2",
	} {
		if !strings.Contains(notes, want) {
			t.Fatalf("overlap family record missing %q:\n%s", want, notes)
		}
	}
}

func TestRCMSemanticObservationFamilyLaneIsTheFoldLane(t *testing.T) {
	// 道别单源 pin: the tool context's target arm used to be able to call a
	// same-thread span on-chain where the engine's mint-time overlap predicate
	// said non-chain. A multi-member family record publishes the FOLD lane —
	// here the chain exists but no node/impact window overlaps the spans, so
	// the family stays off the on-chain lane (adjacent by envelope-vs-chain
	// window overlap), exactly like its rank row.
	worker := tracequery.ThreadRef{Comm: "verify", PID: 200}
	chain := &tracequery.ChainResult{
		Target: worker,
		Window: tracequery.TimeWindow{StartTs: 5.0, EndTs: 5.1},
		Nodes: []tracequery.ChainNode{{
			Thread: worker,
			Window: tracequery.TimeWindow{StartTs: 5.0900, EndTs: 5.0950},
		}},
	}
	stats := tracequery.WindowStats{
		Window: tracequery.TimeWindow{StartTs: 5.0, EndTs: 5.1},
		TraceSpans: []tracequery.TraceSpanSummary{
			rcmToolSpan(worker, "VerifyClass com.example.A", 5.0010, 5.0030, 10, 11),
			rcmToolSpan(worker, "VerifyClass com.example.B", 5.0040, 5.0055, 12, 13),
		},
	}
	records := traceQueryTypedSemanticTraceSpanObservations(tracequery.Result{WakeupChain: chain}, stats,
		types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "a.systrace", ArtifactKind: "trace"},
		"scope", time.Unix(1751600000, 0).UTC().Format(time.RFC3339))
	if len(records) != 1 {
		t.Fatalf("one family record expected: %d", len(records))
	}
	notes := strings.Join(records[0].RichNotes, "\n")
	if !strings.Contains(notes, "chain_relevance=adjacent") || strings.Contains(notes, "chain_relevance=on_chain") {
		t.Fatalf("the family record must publish the fold lane (non-chain → adjacent), never a target-arm on_chain: %s", notes)
	}
}

func TestRCMRankObservationCarriesMemberAndInodeNotes(t *testing.T) {
	result := tracequery.Result{RootCauseRank: &tracequery.RootCauseRankResult{
		Window: tracequery.TimeWindow{StartTs: 10.0, EndTs: 10.2},
		Items: []tracequery.RootCauseRankItem{{
			Rank: 3, Tier: "tertiary", Type: "block_io_by_inode",
			Thread:   tracequery.ThreadRef{Comm: "RxComputationT", PID: 16816},
			ImpactMs: 1.598, CumulativeImpactMs: 1.598, EffectiveImpactMs: 1.598,
			Score: 1.3, Confidence: 0.76, LineStart: 44183, LineEnd: 77131,
			Source: "window_stats.block_io_by_inode", Causality: "background", ChainRelevance: "background",
			MemberCount: 2, MemberMaxMs: 1.136, MemberMinMs: 0.462,
			MemberFoldCaliber: tracequery.RootCauseMemberFoldCaliberSumDisjoint,
			MemberRoster:      []string{"inode=286395 dev=254:2 1.136ms", "inode=300123 dev=254:2 0.462ms"},
			Dev:               "254:2",
			Summary:           "block IO family",
		}, {
			Rank: 4, Tier: "tertiary", Type: "block_io_by_inode",
			Thread:   tracequery.ThreadRef{Comm: "solo", PID: 400},
			ImpactMs: 0.9, CumulativeImpactMs: 0.9, EffectiveImpactMs: 0.9,
			Score: 0.6, Confidence: 0.76, LineStart: 100, LineEnd: 101,
			Source: "window_stats.block_io_by_inode", Causality: "background", ChainRelevance: "background",
			Inode:   "555001",
			Dev:     "254:3",
			Summary: "single block IO row",
		}},
	}}
	records := traceQueryTypedObservations(result, "a.systrace", "payload", "", "", time.Unix(1751600000, 0).UTC())
	var famNotes, soloNotes string
	for _, record := range records {
		if record.Predicate != "root_cause_tertiary" && record.Predicate != "root_cause_background" {
			continue
		}
		joined := strings.Join(record.RichNotes, "\n")
		if strings.Contains(joined, "member_count=2") {
			famNotes = joined
		} else if strings.Contains(joined, "inode=555001") {
			soloNotes = joined
		}
	}
	if famNotes == "" {
		t.Fatalf("the merged rank observation must carry member_count: %+v", records)
	}
	for _, want := range []string{
		"member_count=2",
		"member_max_ms=1.136",
		"member_min_ms=0.462",
		"member_fold_caliber=sum_disjoint",
		"member_roster=inode=286395 dev=254:2 1.136ms | inode=300123 dev=254:2 0.462ms",
		"dev=254:2",
	} {
		if !strings.Contains(famNotes, want) {
			t.Fatalf("merged rank notes missing %q:\n%s", want, famNotes)
		}
	}
	if strings.Contains(famNotes, "folded_rows=") {
		t.Fatalf("isolation: the family lane never rides the folded_* cross-thread wire-cap keys: %s", famNotes)
	}
	if soloNotes == "" {
		t.Fatalf("the unmerged row must still publish its typed inode/dev identity: %+v", records)
	}
	if !strings.Contains(soloNotes, "dev=254:3") || strings.Contains(soloNotes, "member_count=") {
		t.Fatalf("unmerged row notes: %s", soloNotes)
	}
}
