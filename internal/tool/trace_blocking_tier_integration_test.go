package tool

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The full producer path matters: a hand-built legacy
// root_cause_target_self_state predicate masks the publication mismatch.
func TestB1602DonghuBlockingTierThroughRealProducer(t *testing.T) {
	const source = "../../eval/fixtures/real_traces/donghu.ftrace"
	idx, err := tracequery.BuildIndex(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	var records []types.ObservationRecord
	var rankRecord types.ObservationRecord
	var criticalRecord types.ObservationRecord
	for _, view := range []string{"root_cause_rank", "critical_blocking_calls"} {
		limit := 12
		if view == "critical_blocking_calls" {
			limit = 20
		}
		result := tracequery.Run(idx, tracequery.Query{View: view, PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898, MaxDepth: 4, Limit: limit})
		if result.RootCauseRank != nil {
			for _, item := range result.RootCauseRank.Items {
				if item.Type == "binder_wait" && (item.Rank != 0 || item.Tier != tracequery.RootCauseTierTargetSelfState || traceQueryRootCauseCanBePrincipal(item)) {
					t.Fatalf("a measured target wait must not become a root-cause rank seat: %+v", item)
				}
			}
		}
		before, _ := json.Marshal(result)
		obs := traceQueryTypedObservations(result, "donghu.ftrace", "b1602-"+view, "raw", "", time.Unix(1751600000, 0).UTC())
		after, _ := json.Marshal(result)
		if string(before) != string(after) {
			t.Fatal("observation publication changed engine measurements")
		}
		for _, record := range obs {
			if record.Predicate == "root_cause_context_only" && record.Object == "binder_wait" {
				rankRecord = record
			}
			if record.Predicate == "critical_blocking" && record.ClaimKey == "critical_blocking:binder_wait" {
				criticalRecord = record
			}
		}
		records = append(records, obs...)
	}
	if rankRecord.ID == "" || rankRecord.Role != types.AnswerAggregateRoleSupportingCoverage ||
		!strings.Contains(strings.Join(rankRecord.RichNotes, "\n"), "tier=target_self_state") {
		t.Fatalf("fixture must exercise the real non-principal target-self publisher: %+v", rankRecord)
	}
	rm := types.RequestModel{Intent: types.IntentTrace, RuntimeTargets: []types.RuntimeTarget{{
		Kind: types.RuntimeTargetKindThread, PID: 17267, Thread: ".ugc.aweme.lite-17267", Source: "user_explicit",
	}}}
	ledger := types.ObservationLedger{Records: records}
	var found *types.TraceBlockingWallClockAuthority
	for _, authority := range types.BuildTraceBlockingWallClockAuthorities(ledger, &rm) {
		t.Logf("published type=%s value=%.6fms occurrences=%d coverage=%s", authority.Type, authority.ObservedMS, len(authority.Occurrences), authority.CoverageStatus)
		if authority.Type == "pacing_idle" {
			t.Fatal("context-only pacing must not acquire blocking-wall-clock authority")
		}
		if authority.Type == "binder_wait" {
			copy := authority
			found = &copy
		}
	}
	if found == nil || found.CoverageStatus != "lower_bound_capacity_truncated" || len(found.Occurrences) != 1 || math.Abs(found.ObservedMS-1.409) > 0.000001 {
		t.Fatalf("real target-self wait must survive as a lower bound: %+v", found)
	}
	occurrence := found.Occurrences[0]
	if occurrence.StartTs != 13762.835861 || occurrence.EndTs != 13762.837270 || len(occurrence.RecordIDs) != 2 || criticalRecord.ID == "" ||
		!containsB1603RecordID(occurrence.RecordIDs, rankRecord.ID) || !containsB1603RecordID(occurrence.RecordIDs, criticalRecord.ID) || occurrence.Peer != criticalRecord.Object {
		t.Fatalf("same exact sleep published by rank and repaired critical must fold once and retain the critical peer: %+v", occurrence)
	}
	// B1603 repairs the source interval; B1602's original rejection of an
	// independently wider request envelope remains required.
	wrong := criticalRecord
	wrong.ID += "-wide-transaction"
	wrong.Span.StartTs = 13762.835811
	if got := types.BuildTraceBlockingWallClockAuthorities(types.ObservationLedger{Records: []types.ObservationRecord{wrong}}, &rm); len(got) != 0 {
		t.Fatalf("wider request envelope must remain inadmissible: %+v", got)
	}
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			bus := runtimeWaitCoverageTestBus()
			bus.AnalysisIR.RequestModel = rm
			bus.AnalysisIR.RequestModel.Language = lang
			bus.AnalysisIR.AnswerContract.Language = lang
			bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: records}}
			modelText := "model-owned conclusion stays unchanged"
			doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: modelText}}}
			before, _ := json.Marshal(bus.ToolResults)
			if !materializeRuntimeTraceBlockingCoverageAuthorityCaveat(doc, bus) {
				t.Fatal("real reader authority was not materialized")
			}
			var surface string
			for _, block := range doc.Blocks {
				surface += types.AnswerBlockVisibleSurface(block)
			}
			want := "type=binder wait; at least 1 interval totaling 1.409ms"
			if lang == "zh" {
				want = "类型=binder等待；当前至少观测到 1 段、合计 1.409ms"
			}
			if !strings.Contains(surface, want) || doc.Blocks[0].Text != modelText {
				t.Fatalf("reader lower bound or model ownership lost: %s", surface)
			}
			after, _ := json.Marshal(bus.ToolResults)
			if string(before) != string(after) {
				t.Fatal("reader display changed evidence/qualification")
			}
		})
	}
}

func containsB1603RecordID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
