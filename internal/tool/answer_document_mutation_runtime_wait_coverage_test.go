package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func runtimeWaitCoverageTestBus() *types.BusContext {
	target := ".ugc.aweme.lite-17267"
	window := "13762.791708..13763.024898"
	ref := types.ObservationSourceRef{
		Kind:       types.ObservationSourceRuntimeArtifact,
		ArtifactID: "attached_trace",
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
			Observations: []types.ObservationRecord{blocking, ipcSet, ipcRow, blockedReason},
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
	if !materializeRuntimeTraceBlockedReasonCensusCaliberCaveat(doc, bus) {
		t.Fatal("blocked-reason record census must materialize a caliber caveat")
	}
	if doc.Blocks[0].Text != original {
		t.Fatalf("typed caveats must not scan or rewrite model prose: %q", doc.Blocks[0].Text)
	}
	got := strings.Join(doc.Caveats, "\n")
	for _, want := range []string{
		"observed_blocking_lower_bound=1.409ms",
		"observed_occurrences>=1",
		"coverage_status=lower_bound_capacity_truncated",
		"不授权全窗阻塞总量、唯一 occurrence",
		"requests=2，sync=1，oneway=1，unknown=0",
		"完整请求数仍不等于完整阻塞 occurrence 数",
		"blocked_reason_records=50",
		"caller_linked_record_census_not_scheduler_state_partition",
		"不能单独证明每一段 sleep",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed wait coverage caveat missing %q:\n%s", want, got)
		}
	}
	if materializeRuntimeTraceBlockingCoverageAuthorityCaveat(doc, bus) ||
		materializeRuntimeTraceBlockedReasonCensusCaliberCaveat(doc, bus) {
		t.Fatalf("materializers must be idempotent: %+v", doc.Caveats)
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
	got := strings.Join(persisted.Caveats, "\n")
	if !strings.Contains(got, "observed_blocking_lower_bound=1.409ms") ||
		!strings.Contains(got, "blocked_reason_records=50") {
		t.Fatalf("production persist path lost typed wait coverage caveats:\n%s", got)
	}
}

func TestRuntimeBlockedReasonCensusCaliberRejectsNonTargetRecord(t *testing.T) {
	bus := runtimeWaitCoverageTestBus()
	bus.AnalysisIR.RequestModel.RuntimeTargets[0].PID = 999
	bus.AnalysisIR.RequestModel.RuntimeTargets[0].Thread = "other-999"
	doc := &types.AnswerDocumentV2{}
	if materializeRuntimeTraceBlockedReasonCensusCaliberCaveat(doc, bus) {
		t.Fatalf("non-target record must not mint a caveat: %+v", doc.Caveats)
	}
}
