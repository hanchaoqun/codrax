package tool

import (
	"context"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func reconciliationTraceRecord(id, predicate, subject, object string, notes ...string) types.ObservationRecord {
	return types.ObservationRecord{
		ID:              id,
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "sample.systrace"},
		Span:            types.ObservationSpan{LineStart: 10, LineEnd: 20},
		Predicate:       predicate,
		ClaimKey:        predicate + ":fixture",
		Subject:         subject,
		Object:          object,
		Value:           "30.000",
		Unit:            "ms",
		RichNotes:       notes,
		SupportRefs:     []string{"sample.systrace:10-20"},
		Confidence:      0.9,
	}
}

func reconciliationTraceRecords() []types.ObservationRecord {
	const selected = "selected_window=10.000000..10.100000"
	return []types.ObservationRecord{
		reconciliationTraceRecord("trace_query:q#root_cause_rank:1", "root_cause_primary", "worker-7", "priority_inversion_gated",
			selected, "rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
			"impact_ms=30.000", "cumulative_impact_ms=30.000", "effective_impact_ms=30.000",
			"type=priority_inversion_gated", "fix_direction=lock_priority", "rank_board_target=app-10"),
		reconciliationTraceRecord("trace_query:q#wakeup_chain:path", "wakeup_chain", "app-10", "worker-7 -> app-10",
			selected, "path=worker-7 -> app-10"),
		reconciliationTraceRecord("trace_query:q#target_window_states", "target_window_states", "app-10", "state_partition",
			selected, "running=20.000", "runnable=30.000", "sleep=50.000", "d_state=0.000", "io_wait=0.000", "total=100.000"),
	}
}

func reconciliationTraceBus(t *testing.T) *types.BusContext {
	t.Helper()
	mut := types.NewMutableState("trace root cause")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true, Observations: reconciliationTraceRecords(),
	}}})
	ctx := &types.BusContext{Ctx: context.Background(), Mutable: mut, Language: "zh"}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	set := types.CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) != 1 {
		t.Fatalf("fixture must compile one projection: %+v", set)
	}
	blocks := runtimeTraceCausalProjectionCluster(set.Projections[0], "zh", runtimeTraceProjUserFocus{})
	markRuntimeTraceSystemBlocks(blocks)
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: append([]types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, Text: "model summary",
	}}, blocks...)}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	return ctx
}

func TestRuntimeTraceReconciliationRowsReuseVisibleEvidenceIndex(t *testing.T) {
	ctx := reconciliationTraceBus(t)
	rows := RuntimeTraceReconciliationRows(ctx)
	if len(rows) != 2 {
		t.Fatalf("expected target-state + rank-one rows, got %+v", rows)
	}
	byKind := map[RuntimeTraceReconciliationKind]RuntimeTraceReconciliationRow{}
	for _, row := range rows {
		byKind[row.Kind] = row
	}
	account := byKind[RuntimeTraceReconciliationTargetState]
	if account.Subject != "app-10" || account.TotalMS != 100 || account.EvidenceTag == "" {
		t.Fatalf("target-state typed row missing: %+v", account)
	}
	rank := byKind[RuntimeTraceReconciliationRankOne]
	if rank.Subject != "worker-7" || rank.Rank != 1 || rank.EffectiveMS != 30 || rank.FixDirection != "lock_priority" || rank.EvidenceTag == "" {
		t.Fatalf("rank-one typed row missing: %+v", rank)
	}
	// Both tags must occur on the already-rendered projection and in its
	// evidence roster; this is the anti-dangling-reference contract.
	doc := ctx.Mutable.AnswerDocumentV2()
	all := projectionClusterText(doc.Blocks)
	for _, tag := range []string{account.EvidenceTag, rank.EvidenceTag} {
		if !strings.Contains(all, "["+tag+"]") {
			t.Fatalf("typed reconciliation tag [%s] is not visible in the projection:\n%s", tag, all)
		}
		evidence := projectionClusterBlock(doc.Blocks, runtimeTraceCausalProjectionBlockIDBase+"_evidence")
		found := false
		if evidence != nil {
			for _, item := range evidence.Items {
				found = found || item.Label == tag
			}
		}
		if !found {
			t.Fatalf("typed reconciliation tag %s has no evidence-roster entry: %+v", tag, evidence)
		}
	}
}

func TestRuntimeTraceReconciliationRowsStandDownWithoutVisibleProjection(t *testing.T) {
	ctx := reconciliationTraceBus(t)
	ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "model only"}},
	})
	if rows := RuntimeTraceReconciliationRows(ctx); len(rows) != 0 {
		t.Fatalf("an invisible projection must never receive dangling appendix E# rows: %+v", rows)
	}
}
