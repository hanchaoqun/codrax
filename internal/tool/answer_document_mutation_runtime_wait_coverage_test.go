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
	if doc.Blocks[0].ID != "summary" || doc.Blocks[0].Text != original {
		t.Fatalf("typed authorities must not scan or rewrite model prose: %+v", doc.Blocks)
	}
	var surfaces []string
	for _, block := range doc.Blocks {
		surfaces = append(surfaces, block.Title, types.AnswerBlockVisibleSurface(block))
	}
	got := strings.Join(surfaces, "\n")
	for _, want := range []string{
		"当前至少观测到 1 段、合计 1.409ms",
		"结果达到容量上限",
		"不能据此给出全窗阻塞总量、唯一发生段",
		"同窗共记录 IPC 请求 2 次（同步 1、单向 1、类型未知 0）",
		"非 IO D-state 0.000ms",
		"已归账 233.190ms / 窗口 233.190ms",
		"覆盖=完整",
		"记录到 50 条 blocked_reason",
		"不是完整的线程调度状态分区",
		"不能据此认定每一段 sleep",
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
		doc.Blocks[0].ID != "summary" ||
		doc.Blocks[1].ID != runtimeTraceBlockingCoverageAuthorityBlockID ||
		doc.Blocks[2].ID != runtimeTraceTargetStateAuthorityBlockID ||
		doc.Blocks[3].ID != runtimeTraceBlockedReasonCensusCaliberBlockID {
		t.Fatalf("typed data boundaries must follow the model narrative: %+v", doc.Blocks)
	}
	for _, forbidden := range []string{"系统权威", "系统 authority", "后续模型正文", "以本块为准"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("internal authority protocol leaked through %q:\n%s", forbidden, got)
		}
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

func TestRuntimeWaitCoverageDataBoundariesUseReaderFacingEnglish(t *testing.T) {
	bus := runtimeWaitCoverageTestBus()
	bus.AnalysisIR.AnswerContract.Language = "en"
	bus.AnalysisIR.RequestModel.Language = "en"
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, Text: "model answer",
	}}}
	if !materializeRuntimeTraceBlockingCoverageAuthorityCaveat(doc, bus) ||
		!materializeRuntimeTraceTargetStateAuthorityBlock(doc, bus) ||
		!materializeRuntimeTraceBlockedReasonCensusCaliberCaveat(doc, bus) {
		t.Fatalf("reader-facing English boundaries did not materialize: %+v", doc.Blocks)
	}
	if doc.Blocks[0].ID != "summary" {
		t.Fatalf("English model narrative must remain first: %+v", doc.Blocks)
	}
	surface := answerDocumentTestVisibleSurface(doc)
	for _, want := range []string{
		"Observed scope of target blocking",
		"at least 1 interval totaling 1.409ms",
		"Target-thread states and wait details",
		"Target-thread state:",
		"How to read blocked_reason records",
		"not a complete scheduler-state partition",
	} {
		if !strings.Contains(surface, want) {
			t.Fatalf("English data boundary missing %q:\n%s", want, surface)
		}
	}
	for _, forbidden := range []string{"System authority", "later model prose", "takes precedence", "typed authority"} {
		if strings.Contains(surface, forbidden) {
			t.Fatalf("internal English authority protocol leaked through %q:\n%s", forbidden, surface)
		}
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
	if !strings.Contains(got, "当前至少观测到 1 段、合计 1.409ms") ||
		!strings.Contains(got, "记录到 50 条 blocked_reason") {
		t.Fatalf("production persist path lost typed wait coverage caveats:\n%s", got)
	}
	if len(persisted.Blocks) < 4 || persisted.Blocks[0].ID != "summary" {
		t.Fatalf("production hierarchy must keep user narrative first: %+v", persisted.Blocks)
	}
	projectionIndex := -1
	for i := range persisted.Blocks {
		if RuntimeTraceSystemBlock(persisted.Blocks[i]) &&
			runtimeTraceCausalProjectionStandaloneLeadBlockID(persisted.Blocks[i].ID) {
			projectionIndex = i
			break
		}
	}
	if projectionIndex <= 0 {
		t.Fatalf("fixture lost the causal decision surface: %+v", persisted.Blocks)
	}
	for _, id := range []string{
		runtimeTraceBlockingCoverageAuthorityBlockID,
		runtimeTraceTargetStateAuthorityBlockID,
		runtimeTraceBlockedReasonCensusCaliberBlockID,
	} {
		index := -1
		for i := range persisted.Blocks {
			if persisted.Blocks[i].ID == id {
				index = i
				break
			}
		}
		if index <= projectionIndex {
			t.Fatalf("data boundary %q must follow narrative and causal decision surfaces: %+v", id, persisted.Blocks)
		}
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
	got := types.AnswerBlockVisibleSurface(
		answerDocumentTestBlockByID(t, doc, runtimeTraceTargetStateAuthorityBlockID),
	)
	for _, want := range []string{
		"非 IO D-state 6.000ms",
		"等待明细完整，共 2 段（D-state 2",
		"墙钟合计 6.000ms",
		"已解析 caller：dma_fence_default_w",
		"第 1 段：d_sleep，13762.811000..13762.813000，2.000ms，iowait=0，caller=dma_fence_default_w",
		"第 2 段：d_sleep，13762.821000..13762.825000，4.000ms，iowait=0，caller=dma_fence_default_w",
		"逐段合计与上述 D/IO 状态账一致",
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
	consensusSurface := types.AnswerBlockVisibleSurface(
		answerDocumentTestBlockByID(t, consensusDoc, runtimeTraceTargetStateAuthorityBlockID),
	)
	if !strings.Contains(consensusSurface, "等待明细完整，共 2 段") ||
		!strings.Contains(consensusSurface, "墙钟合计 6.000ms") {
		t.Fatalf("consensus target lost complete occurrence values:\n%s", consensusSurface)
	}
}

func TestFocusedRuntimeFactPublishesTypedRosterWithoutFullCausalReport(t *testing.T) {
	bus := runtimeWaitCoverageTestBus()
	rm := &bus.AnalysisIR.RequestModel
	rm.Intent = types.IntentExplain
	rm.Scenario = types.ScenarioGeneric
	rm.AnalyzerHints.Kind = string(types.ReqMechanism)
	rm.PredicateAxis = types.AxisUnknown
	rm.Predicates.IsDiagnosticQuestion = false
	rm.DiagnosticProfile.IsDiagnostic = false

	target := ".ugc.aweme.lite-17267"
	count := 3
	ref := types.ObservationSourceRef{
		Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "attached_trace",
		Path: "/tmp/attached_trace.txt",
	}
	records := []types.ObservationRecord{{
		ID:     "trace_query:focused#target_window_wait_occurrences",
		Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
		Span:      types.ObservationSpan{StartTs: 13762.791708, EndTs: 13763.024898},
		Predicate: "target_window_wait_occurrences", Subject: target,
		Object: "complete", Value: "3", ResultCount: &count,
	}}
	for index, bounds := range [][3]float64{
		{13762.801000, 13762.801138, 0.138},
		{13762.802000, 13762.802147, 0.147},
		{13762.803000, 13762.803350, 0.350},
	} {
		records = append(records, types.ObservationRecord{
			ID: fmt.Sprintf(
				"trace_query:focused#target_window_wait_occurrence:%d",
				index+1,
			),
			Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
			Span:      types.ObservationSpan{StartTs: bounds[0], EndTs: bounds[1]},
			Predicate: "target_window_wait_occurrence", Subject: target,
			Object: "state=io_wait;iowait=1;caller=sync_buffer_read_wi",
			Value:  fmt.Sprintf("%.3f", bounds[2]), Unit: "ms",
		})
	}
	bus.ToolResults[0].Observations = append(bus.ToolResults[0].Observations, records...)

	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID: "summary", Kind: types.BlockSummary,
			Text: "model prose contains an unrelated capacity caveat",
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
		t.Fatalf("focused principal-value persist failed: result=%+v err=%v", result, err)
	}
	persisted := bus.Mutable.AnswerDocumentV2()
	if persisted == nil || len(persisted.Blocks) < 2 ||
		persisted.Blocks[0].ID != "summary" {
		t.Fatalf("focused fact must retain the user narrative lead: %+v", persisted)
	}
	if answerDocumentHasRuntimeTraceCausalProjectionBlock(persisted) {
		t.Fatalf("focused fact must not inherit the full causal projection: %+v", persisted.Blocks)
	}
	surface := types.AnswerBlockVisibleSurface(
		answerDocumentTestBlockByID(t, persisted, runtimeTraceTargetStateAuthorityBlockID),
	)
	for _, want := range []string{
		"等待明细完整，共 3 段",
		"墙钟合计 0.635ms",
		"第 1 段：io_wait，13762.801000..13762.801138，0.138ms，iowait=1，caller=sync_buffer_read_wi",
		"第 3 段：io_wait，13762.803000..13762.803350，0.350ms，iowait=1，caller=sync_buffer_read_wi",
	} {
		if !strings.Contains(surface, want) {
			t.Fatalf("focused typed principal-value card missing %q:\n%s", want, surface)
		}
	}
}

func TestRuntimeTargetWaitAuthorityListsRequestedScopeBeforeExploration(t *testing.T) {
	target := "com.baidu.tieba-59566"
	ref := types.ObservationSourceRef{
		Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "attached_trace",
		Path: "/tmp/attached_trace.txt",
	}
	makeRoster := func(scope string, start, end float64, durations []float64) []types.ObservationRecord {
		count := len(durations)
		records := []types.ObservationRecord{{
			ID:     "trace_query:" + scope + "#target_window_wait_occurrences",
			Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
			Span:      types.ObservationSpan{StartTs: start, EndTs: end},
			Predicate: "target_window_wait_occurrences", Subject: target,
			Object: "complete", Value: strconv.Itoa(count), ResultCount: &count,
		}}
		cursor := start + 0.001
		for i, duration := range durations {
			rowEnd := cursor + duration/1000
			records = append(records, types.ObservationRecord{
				ID:     fmt.Sprintf("trace_query:%s#target_window_wait_occurrence:%d", scope, i+1),
				Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
				GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
				Span:      types.ObservationSpan{StartTs: cursor, EndTs: rowEnd},
				Predicate: "target_window_wait_occurrence", Subject: target,
				Object: "state=io_wait;iowait=1;caller=sync_buffer_read_wi",
				Value:  fmt.Sprintf("%.3f", duration), Unit: "ms",
			})
			cursor = rowEnd + 0.001
		}
		return records
	}
	narrow := makeRoster("narrow", 10, 10.02, []float64{0.2})
	full := makeRoster("full", 10, 10.1, []float64{0.2, 0.3})
	profile := &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeFullArtifact,
		SourceQuote:    "这份 trace",
	}
	bus := &types.BusContext{
		Mutable: types.NewMutableState("requested trace scope"),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentRootCause,
				Scenario: types.ScenarioRootCause,
				RuntimeTargets: []types.RuntimeTarget{{
					Kind: types.RuntimeTargetKindThread, PID: 59566,
					Thread: target, Source: "user_explicit",
				}},
				RuntimeArtifactScopeProfile: profile,
			},
			AnswerContract: types.AnswerContract{Language: "zh"},
		},
		ToolResults: []types.ToolResult{{
			ToolName: "trace_query", Success: true, Observations: narrow,
		}},
	}
	bus.Mutable.SetRequestModel(bus.AnalysisIR.RequestModel)
	bus.Mutable.SetSystemTraceSupplement(types.SystemTraceSupplementMeta{
		Views:                  []string{"window_stats"},
		RequestedArtifactScope: types.RuntimeArtifactScopeFullArtifact,
	}, []types.ToolResult{{
		ToolName: "trace_query", Success: true, Observations: full,
	}})
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, Text: "model answer",
	}}}
	if !materializeRuntimeTraceTargetStateAuthorityBlock(doc, bus) {
		t.Fatal("requested-scope wait account did not materialize")
	}
	got := types.AnswerBlockVisibleSurface(
		answerDocumentTestBlockByID(t, doc, runtimeTraceTargetStateAuthorityBlockID),
	)
	principalAt := strings.Index(got, "请求主范围；工件=attached_trace.txt，窗口=10.000000..10.100000")
	supportAt := strings.Index(got, "探索子范围；工件=attached_trace.txt，窗口=10.000000..10.020000")
	if principalAt < 0 || supportAt < 0 || principalAt > supportAt {
		t.Fatalf("requested scope must lead the exploration row: principal=%d support=%d\n%s", principalAt, supportAt, got)
	}
	for _, want := range []string{
		"请求主范围先列",
		"不能替代主范围的次数、总量或清单",
		"D-state、io_wait 与 S 态 IO 等待是分开的记录类型",
		"等待明细完整，共 2 段（D-state 0、io_wait 2",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("requested-scope data card missing %q:\n%s", want, got)
		}
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
	got := types.AnswerBlockVisibleSurface(
		answerDocumentTestBlockByID(t, doc, runtimeTraceTargetStateAuthorityBlockID),
	)
	for _, want := range []string{
		"工件=donghu.ftrace",
		"已归账 231.794ms / 窗口 233.190ms",
		"覆盖=部分（存在未归账区间）",
		"另有 1.396ms 未归账",
		"窗口尾部开放 8.793ms（状态=sleep",
		"等待明细完整，共 11 段",
		"D-state 11",
		"墙钟合计 36.757ms",
		"已解析 caller：dma_fence_default_w+0x260/0x4dc[devhost.elf]",
		"逐段合计与上述 D/IO 状态账一致",
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
