package tool

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func runtimeWaitCoverageTestBus() *types.BusContext {
	target := ".ugc.aweme.lite-17267"
	window := "13762.791708..13763.024898"
	ref := types.ObservationSourceRef{
		Kind:       types.ObservationSourceRuntimeArtifact,
		ArtifactID: "attached_trace",
		Path:       "/tmp/attached_trace.txt",
	}
	blocking := types.ObservationRecord{
		ID:              "root:binder",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRolePrincipalAnswer,
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef:       ref,
		Span:            types.ObservationSpan{StartTs: 13762.835861, EndTs: 13762.837270},
		ClaimKey:        "root_cause_target_self_state",
		Predicate:       "root_cause_target_self_state",
		Subject:         target,
		Object:          "binder_wait",
		Value:           "1.409",
		Unit:            "ms",
		RichNotes: []string{
			types.TraceNoteKeyTier + "=" + types.TraceCausalTierTargetSelfState,
			types.TraceNoteKeyType + "=binder_wait",
			types.TraceNoteKeySelectedWindow + "=" + window,
			types.TraceNoteKeyCapacityTruncated + "=true",
		},
	}
	ipcSet := types.ObservationRecord{
		ID:              "ipc:set",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef:       ref,
		Span:            types.ObservationSpan{StartTs: 13762.791708, EndTs: 13763.024898},
		ClaimKey:        "ipc_request_census:" + target,
		Predicate:       "ipc_request_census",
		Subject:         target,
		Value:           "2",
		Unit:            "requests",
		RichNotes: []string{
			types.TraceNoteKeySelectedWindow + "=" + window,
			types.TraceNoteKeyIPCRequestCensusStatus + "=complete",
			types.TraceNoteKeyIPCSyncRequestCount + "=1",
			types.TraceNoteKeyIPCOnewayRequestCount + "=1",
			types.TraceNoteKeyIPCUnknownRequestCount + "=0",
		},
	}
	ipcRow := types.ObservationRecord{
		ID:              "ipc:12145859",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef:       ref,
		Span:            types.ObservationSpan{StartTs: 13762.835811, EndTs: 13762.835943},
		ClaimKey:        "ipc_request_edge:" + target + ":12145859",
		Predicate:       "ipc_request_edge",
		Subject:         target,
		Object:          "binder:496_9-10961",
		Value:           "12145859",
		Unit:            "transaction_id",
		RichNotes: []string{
			types.TraceNoteKeySelectedWindow + "=" + window,
			types.TraceNoteKeyIPCTransactionID + "=12145859",
			types.TraceNoteKeyIPCCallSemantics + "=sync_request",
			types.TraceNoteKeyIPCFlags + "=0x10",
			types.TraceNoteKeyIPCFlagsKnown + "=true",
			types.TraceNoteKeyIPCCode + "=0x19",
			types.TraceNoteKeyIPCCodeKnown + "=true",
		},
	}
	count := 50
	blockedReason := types.ObservationRecord{
		ID:              "blocked:set",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef:       ref,
		ClaimKey:        "blocked_reason_census:" + target,
		Predicate:       "blocked_reason_census",
		Subject:         target,
		Object:          "blocked_reason",
		Value:           "50",
		ResultCount:     &count,
		RichNotes: []string{
			types.TraceNoteKeySelectedWindow + "=" + window,
			types.TraceNoteKeyBlockedReasonCensus + "=fscache_page_get_an×39(Σ14.756ms)/fscache_page_wait_o×11(Σ1.602ms)",
		},
	}
	targetState := types.ObservationRecord{
		ID:              "target:states",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef:       ref,
		Span:            types.ObservationSpan{StartTs: 13762.791708, EndTs: 13763.024898},
		ClaimKey:        "target_window_states:" + target,
		Predicate:       "target_window_states",
		Subject:         target,
		Object:          "state_partition",
		Value:           "233.190",
		Unit:            "ms",
		RichNotes: []string{
			types.TraceNoteKeySelectedWindow + "=" + window,
			types.TraceNoteKeyRunning + "=157.248",
			types.TraceNoteKeyRunnable + "=5.604",
			types.TraceNoteKeySleep + "=70.338",
			types.TraceNoteKeyDState + "=0.000",
			types.TraceNoteKeyIOWait + "=0.000",
			types.TraceNoteKeyTotal + "=233.190",
		},
	}
	return &types.BusContext{
		Mutable: types.NewMutableState("wait coverage"),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentRootCause,
				Scenario: types.ScenarioRootCause,
				RuntimeTargets: []types.RuntimeTarget{{
					Kind:   types.RuntimeTargetKindThread,
					PID:    17267,
					Thread: target,
					Source: "user_explicit",
				}},
			},
			AnswerContract: types.AnswerContract{Language: "zh"},
		},
		ToolResults: []types.ToolResult{{
			ToolName:     "trace_query",
			Success:      true,
			Observations: []types.ObservationRecord{blocking, ipcSet, ipcRow, blockedReason, targetState},
		}},
	}
}

func TestRuntimeWaitCoverageCaveatsUseTypedAuthoritiesWithoutRewritingModelProse(t *testing.T) {
	bus := runtimeWaitCoverageTestBus()
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "模型正文保留原样：1.409ms 是唯一阻塞，全部 70.338ms 都是 fscache。",
		}},
	}
	original := doc.Blocks[0].Text
	if !materializeRuntimeTraceBlockingCoverageAuthorityCaveat(doc, bus) {
		t.Fatal("lower-bound blocking authority must materialize a caveat")
	}
	if !materializeRuntimeTraceTargetStateAuthorityBlock(doc, bus) {
		t.Fatal("target-thread state authority must materialize a principal value card")
	}
	if !materializeRuntimeTraceBlockedReasonCensusCaliberCaveat(doc, bus) {
		t.Fatal("blocked-reason record census must materialize a caliber caveat")
	}
	if doc.Blocks[len(doc.Blocks)-1].ID != "summary" || doc.Blocks[len(doc.Blocks)-1].Text != original {
		t.Fatalf("typed authorities must not scan or rewrite model prose: %+v", doc.Blocks)
	}
	var surfaces []string
	for _, block := range doc.Blocks {
		surfaces = append(surfaces, block.Title, types.AnswerBlockVisibleSurface(block))
	}
	got := strings.Join(surfaces, "\n")
	for _, want := range []string{
		"observed_blocking_lower_bound=1.409ms",
		"observed_occurrences>=1",
		"coverage_status=lower_bound_capacity_truncated",
		"不授权全窗阻塞总量、唯一 occurrence",
		"requests=2，sync=1，oneway=1，unknown=0",
		"完整请求数仍不等于完整阻塞 occurrence 数",
		"non_io_d_state=0.000ms",
		"accounted_total=233.190ms",
		"coverage_status=complete",
		"blocked_reason_records=50",
		"caller_linked_record_census_not_scheduler_state_partition",
		"不能单独证明每一段 sleep",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed wait coverage caveat missing %q:\n%s", want, got)
		}
	}
	if materializeRuntimeTraceBlockingCoverageAuthorityCaveat(doc, bus) ||
		materializeRuntimeTraceTargetStateAuthorityBlock(doc, bus) ||
		materializeRuntimeTraceBlockedReasonCensusCaliberCaveat(doc, bus) {
		t.Fatalf("materializers must be idempotent: %+v", doc.Blocks)
	}
	if len(doc.Blocks) != 4 ||
		doc.Blocks[0].ID != runtimeTraceBlockingCoverageAuthorityBlockID ||
		doc.Blocks[1].ID != runtimeTraceTargetStateAuthorityBlockID ||
		doc.Blocks[2].ID != runtimeTraceBlockedReasonCensusCaliberBlockID ||
		doc.Blocks[3].ID != "summary" {
		t.Fatalf("typed authority blocks must lead the model narrative: %+v", doc.Blocks)
	}
}

func TestRuntimeWaitCoverageAuthorityLeadsDoNotMintWithoutTypedRows(t *testing.T) {
	bus := &types.BusContext{
		Mutable: types.NewMutableState("no runtime authority rows"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentTrace,
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, Text: "model answer",
	}}}
	if materializeRuntimeTraceBlockingCoverageAuthorityCaveat(doc, bus) ||
		materializeRuntimeTraceTargetStateAuthorityBlock(doc, bus) ||
		materializeRuntimeTraceBlockedReasonCensusCaliberCaveat(doc, bus) {
		t.Fatalf("fixed authority lead text must not mint a block without typed rows: %+v", doc.Blocks)
	}
	if len(doc.Blocks) != 1 || doc.Blocks[0].ID != "summary" {
		t.Fatalf("empty authority lanes changed the document: %+v", doc.Blocks)
	}
}

func TestRuntimeTraceAuthorityTargetRequiresTypedEntitySupplementConsensus(t *testing.T) {
	bus := runtimeWaitCoverageTestBus()
	target := bus.AnalysisIR.RequestModel.RuntimeTargets[0]
	bus.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{
		Kind: types.RuntimeTargetKindThread, PID: target.PID, Thread: target.Thread,
		Source: types.RuntimeTargetSourceExplicitToolCall, Confidence: 1,
	}}
	bus.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{target.Thread}
	bus.Mutable.SetRequestModel(bus.AnalysisIR.RequestModel)
	bus.Mutable.SetSystemTraceSupplement(types.SystemTraceSupplementMeta{
		Views:        []string{"root_cause_rank"},
		TargetPID:    target.PID,
		TargetThread: target.Thread,
		TargetSource: "cursor",
	}, []types.ToolResult{{ToolName: "trace_query", Success: true}})

	rm := runtimeTraceAuthorityRequestModel(bus)
	if rm == nil || len(rm.RuntimeTargets) != 2 ||
		rm.RuntimeTargets[1].PID != target.PID ||
		rm.RuntimeTargets[1].Source != runtimeTraceTypedTargetConsensusSource {
		t.Fatalf("typed entity + executed supplement agreement must recover private answer authority: %+v", rm)
	}
	if len(bus.AnalysisIR.RequestModel.RuntimeTargets) != 1 ||
		!types.RuntimeTargetIsExplorationCursorSource(bus.AnalysisIR.RequestModel.RuntimeTargets[0].Source) {
		t.Fatalf("answer-time consensus must not mutate the persistent request model: %+v",
			bus.AnalysisIR.RequestModel.RuntimeTargets)
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, Text: "model answer",
	}}}
	if !materializeRuntimeTraceBlockingCoverageAuthorityCaveat(doc, bus) ||
		!materializeRuntimeTraceBlockedReasonCensusCaliberCaveat(doc, bus) {
		t.Fatalf("recovered target authority must expose its matching deterministic rows: %+v", doc.Blocks)
	}

	for name, mutate := range map[string]func(){
		"cursor without analyzer entity": func() {
			bus.AnalysisIR.RequestModel.AnalyzerHints.Entities = nil
			bus.Mutable.SetRequestModel(bus.AnalysisIR.RequestModel)
		},
		"mismatched supplement pid": func() {
			bus.Mutable.SetSystemTraceSupplement(types.SystemTraceSupplementMeta{
				Views: []string{"root_cause_rank"}, TargetPID: 999, TargetSource: "cursor",
			}, []types.ToolResult{{ToolName: "trace_query", Success: true}})
		},
		"meta without executed results": func() {
			bus.Mutable.SetSystemTraceSupplement(types.SystemTraceSupplementMeta{
				Views: []string{"root_cause_rank"}, TargetPID: target.PID, TargetSource: "cursor",
			}, nil)
		},
		"failed supplement result": func() {
			bus.Mutable.SetSystemTraceSupplement(types.SystemTraceSupplementMeta{
				Views: []string{"root_cause_rank"}, TargetPID: target.PID, TargetSource: "cursor",
			}, []types.ToolResult{{ToolName: "trace_query", Success: false}})
		},
		"census-only meta without targeted view": func() {
			bus.Mutable.SetSystemTraceSupplement(types.SystemTraceSupplementMeta{
				TargetPID: target.PID, TargetSource: "cursor", CensusLite: true,
			}, []types.ToolResult{{ToolName: "trace_query", Success: true}})
		},
	} {
		// Restore the common typed entity and valid executed meta, then apply
		// one independent negative mutation.
		bus.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{target.Thread}
		bus.Mutable.SetRequestModel(bus.AnalysisIR.RequestModel)
		bus.Mutable.SetSystemTraceSupplement(types.SystemTraceSupplementMeta{
			Views: []string{"root_cause_rank"}, TargetPID: target.PID, TargetSource: "cursor",
		}, []types.ToolResult{{ToolName: "trace_query", Success: true}})
		mutate()
		if got := runtimeTraceAuthorityRequestModel(bus); got != nil {
			t.Errorf("%s must fail closed, got %+v", name, got.RuntimeTargets)
		}
	}
}

func TestPersistMergedAnswerDocumentWiresTypedWaitCoverageCaveats(t *testing.T) {
	bus := runtimeWaitCoverageTestBus()
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "保持模型正文。",
		}},
	}
	result, err := ApplyAndPersistMutation(
		bus,
		"test_emit",
		types.NewReplaceAllMutation(doc),
		nil,
		time.Now(),
	)
	if err != nil || !result.Success {
		t.Fatalf("persist failed: result=%+v err=%v", result, err)
	}
	persisted := bus.Mutable.AnswerDocumentV2()
	if persisted == nil {
		t.Fatal("missing persisted answer document")
	}
	var surfaces []string
	for _, block := range persisted.Blocks {
		surfaces = append(surfaces, block.Title, types.AnswerBlockVisibleSurface(block))
	}
	got := strings.Join(surfaces, "\n")
	if !strings.Contains(got, "observed_blocking_lower_bound=1.409ms") ||
		!strings.Contains(got, "blocked_reason_records=50") {
		t.Fatalf("production persist path lost typed wait coverage caveats:\n%s", got)
	}
	if len(persisted.Blocks) < 4 ||
		persisted.Blocks[0].ID != runtimeTraceBlockingCoverageAuthorityBlockID ||
		persisted.Blocks[1].ID != runtimeTraceTargetStateAuthorityBlockID ||
		persisted.Blocks[2].ID != runtimeTraceBlockedReasonCensusCaliberBlockID ||
		persisted.Blocks[3].ID != "summary" {
		t.Fatalf("production hierarchy must keep typed principal authorities before model prose: %+v", persisted.Blocks)
	}
}

func TestRuntimeTargetStateAuthorityPublishesCompleteOccurrenceSummary(t *testing.T) {
	bus := runtimeWaitCoverageTestBus()
	window := "13762.791708..13763.024898"
	target := ".ugc.aweme.lite-17267"
	for resultIndex := range bus.ToolResults {
		for recordIndex := range bus.ToolResults[resultIndex].Observations {
			record := &bus.ToolResults[resultIndex].Observations[recordIndex]
			if record.Predicate != "target_window_states" {
				continue
			}
			record.RichNotes = []string{
				types.TraceNoteKeySelectedWindow + "=" + window,
				types.TraceNoteKeyRunning + "=151.248",
				types.TraceNoteKeyRunnable + "=5.604",
				types.TraceNoteKeySleep + "=70.338",
				types.TraceNoteKeyDState + "=6.000",
				types.TraceNoteKeyIOWait + "=0.000",
				types.TraceNoteKeyTotal + "=233.190",
			}
		}
	}
	count := 2
	ref := types.ObservationSourceRef{
		Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "attached_trace",
		Path: "/tmp/attached_trace.txt",
	}
	records := []types.ObservationRecord{{
		ID: "trace_query:waits#target_window_wait_occurrences", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
		Span:      types.ObservationSpan{StartTs: 13762.791708, EndTs: 13763.024898},
		Predicate: "target_window_wait_occurrences", Subject: target, Object: "complete",
		Value: "2", ResultCount: &count,
	}}
	for index, bounds := range [][3]float64{
		{13762.811000, 13762.813000, 2},
		{13762.821000, 13762.825000, 4},
	} {
		records = append(records, types.ObservationRecord{
			ID:     "trace_query:waits#target_window_wait_occurrence:" + strconv.Itoa(index+1),
			Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
			Span:      types.ObservationSpan{StartTs: bounds[0], EndTs: bounds[1]},
			Predicate: "target_window_wait_occurrence", Subject: target,
			Object: "state=d_sleep;iowait=0;caller=dma_fence_default_w",
			Value:  fmt.Sprintf("%.3f", bounds[2]), Unit: "ms",
		})
	}
	bus.ToolResults[0].Observations = append(bus.ToolResults[0].Observations, records...)
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, Text: "model text",
	}}}
	if !materializeRuntimeTraceTargetStateAuthorityBlock(doc, bus) {
		t.Fatal("complete target occurrence roster must materialize")
	}
	got := types.AnswerBlockVisibleSurface(doc.Blocks[0])
	for _, want := range []string{
		"non_io_d_state=6.000ms",
		"target_wait_occurrence_roster=complete",
		"occurrences=2",
		"d_state_occurrences=2",
		"occurrence_wall_clock_sum=6.000ms",
		"wait_callers=dma_fence_default_w",
		"与上述 D/IO 配对状态账一致",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("target-state authority missing %q:\n%s", want, got)
		}
	}

	// Analyzer-emission-gap twin: the durable request model contains only the
	// model cursor, while its typed entity and the executed supplement agree
	// on the same PID. The complete occurrence roster must remain available
	// to the principal card without promoting the cursor globally.
	bus.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{
		Kind: types.RuntimeTargetKindThread, PID: 17267, Thread: target,
		Source: types.RuntimeTargetSourceExplicitToolCall, Confidence: 1,
	}}
	bus.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{target}
	bus.Mutable.SetRequestModel(bus.AnalysisIR.RequestModel)
	bus.Mutable.SetSystemTraceSupplement(types.SystemTraceSupplementMeta{
		Views: []string{"root_cause_rank"}, TargetPID: 17267,
		TargetThread: target, TargetSource: "cursor",
	}, []types.ToolResult{{ToolName: "trace_query", Success: true}})
	consensusDoc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, Text: "model text",
	}}}
	if !materializeRuntimeTraceTargetStateAuthorityBlock(consensusDoc, bus) {
		t.Fatal("typed entity/supplement consensus must recover target wait authority")
	}
	consensusSurface := types.AnswerBlockVisibleSurface(consensusDoc.Blocks[0])
	if !strings.Contains(consensusSurface, "target_wait_occurrence_roster=complete") ||
		!strings.Contains(consensusSurface, "occurrences=2") ||
		!strings.Contains(consensusSurface, "occurrence_wall_clock_sum=6.000ms") {
		t.Fatalf("consensus target lost complete occurrence values:\n%s", consensusSurface)
	}
}

func TestRuntimeTargetStateAuthorityRealDonghuPublishesElevenOccurrencesAndPartialCoverage(t *testing.T) {
	const trace = "../../eval/fixtures/real_traces/donghu.ftrace"
	idx, err := tracequery.BuildIndex(context.Background(), trace)
	if err != nil {
		t.Fatal(err)
	}
	start, end := 13762.791708, 13763.024898
	result := tracequery.Run(idx, tracequery.Query{
		View: "root_cause_rank", PID: 2955, TimeStart: start, TimeEnd: end,
		TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace, Limit: 12,
	})
	records := traceQueryTypedObservations(
		result, "attached_trace", "payload-root", "raw-root", "",
		time.Unix(1751600000, 0).UTC(),
	)
	bus := &types.BusContext{
		Mutable: types.NewMutableState("donghu state authority"),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentTrace,
				RuntimeTargets: []types.RuntimeTarget{{
					Kind: types.RuntimeTargetKindThread, PID: 2955,
					Thread: "CompThread_0-2955", Source: "user_explicit",
				}},
				RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{
					RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
					TimeStart:      &start, TimeEnd: &end, SourceQuote: "typed-window",
				},
			},
			AnswerContract: types.AnswerContract{Language: "zh"},
		},
		ToolResults: []types.ToolResult{{
			ToolName: "trace_query", Success: true, Observations: records,
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, Text: "model text",
	}}}
	if !materializeRuntimeTraceTargetStateAuthorityBlock(doc, bus) {
		t.Fatal("real donghu target-state authority did not materialize")
	}
	got := types.AnswerBlockVisibleSurface(doc.Blocks[0])
	for _, want := range []string{
		"artifact=donghu.ftrace",
		"accounted_total=231.794ms",
		"window=233.190ms",
		"coverage_status=partial_unaccounted",
		"unaccounted=1.396ms",
		"tail_open=8.793ms(state=sleep",
		"target_wait_occurrence_roster=complete",
		"occurrences=11",
		"d_state_occurrences=11",
		"occurrence_wall_clock_sum=36.757ms",
		"wait_callers=dma_fence_default_w+0x260/0x4dc[devhost.elf]",
		"与上述 D/IO 配对状态账一致",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("real target-state authority missing %q:\n%s", want, got)
		}
	}
}

func TestRuntimeBlockedReasonCensusCaliberRejectsNonTargetRecord(t *testing.T) {
	bus := runtimeWaitCoverageTestBus()
	bus.AnalysisIR.RequestModel.RuntimeTargets[0].PID = 999
	bus.AnalysisIR.RequestModel.RuntimeTargets[0].Thread = "other-999"
	doc := &types.AnswerDocumentV2{}
	if materializeRuntimeTraceBlockedReasonCensusCaliberCaveat(doc, bus) {
		t.Fatalf("non-target record must not mint a caveat: %+v", doc.Blocks)
	}
}
