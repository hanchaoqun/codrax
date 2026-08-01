package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderAnswerDocObservationLedgerCarriesTraceValueOccurrenceAuthority(t *testing.T) {
	record := types.ObservationRecord{
		ID:              "root:binder",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRolePrincipalAnswer,
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef: types.ObservationSourceRef{
			Kind:       types.ObservationSourceRuntimeArtifact,
			ArtifactID: "attached_trace.txt",
		},
		Span:      types.ObservationSpan{StartTs: 13762.835861, EndTs: 13762.837270},
		ClaimKey:  "root_cause_target_self_state",
		Predicate: "root_cause_target_self_state",
		Subject:   ".ugc.aweme.lite-17267",
		Object:    "binder_wait",
		Value:     "1.409",
		Unit:      "ms",
		RichNotes: []string{"type=binder_wait"},
	}
	mut := types.NewMutableState("分析 trace")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName:     "trace_query",
		Success:      true,
		Observations: []types.ObservationRecord{record},
	}}})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentRootCause,
			RuntimeTargets: []types.RuntimeTarget{{
				Kind:   types.RuntimeTargetKindThread,
				PID:    17267,
				Thread: ".ugc.aweme.lite-17267",
				Source: "user_explicit",
			}},
		}},
	}
	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{
		"### Trace Value-Owner Temporal Authority",
		"type=`binder_wait`",
		"value=1.409ms",
		"temporal_status=`exact`",
		"value_owner_occurrence=`13762.835861..13762.837270`",
		"transaction phase",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("value-owner temporal authority missing %q:\n%s", want, got)
		}
	}
}

func TestRenderAnswerDocObservationLedgerDoesNotPromoteMissingWakeupToPositiveAuthority(t *testing.T) {
	record := types.ObservationRecord{
		ID:              "missing-wakeup",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef: types.ObservationSourceRef{
			Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "attached_trace.txt",
		},
		Span:      types.ObservationSpan{StartTs: 10.046416, EndTs: 10.050000},
		ClaimKey:  "root_cause_target_self_state",
		Predicate: "root_cause_target_self_state",
		Subject:   "target-100",
		Object:    "missing_wakeup",
		Value:     "3.584",
		Unit:      "ms",
		RichNotes: []string{
			"type=missing_wakeup",
			types.TraceNoteKeyTier + "=" + types.TraceCausalTierTargetSelfState,
			"selected_window=10.000000..10.050000",
		},
	}
	mut := types.NewMutableState("analyze trace")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true, Observations: []types.ObservationRecord{record},
	}}})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentRootCause,
			RuntimeTargets: []types.RuntimeTarget{{
				Kind: types.RuntimeTargetKindThread, PID: 100, Thread: "target-100", Source: "user_explicit",
			}},
		}},
	}
	got := renderAnswerDocObservationLedger(ctx)
	for _, forbidden := range []string{
		"### Trace Value-Owner Temporal Authority",
		"### Trace Target Blocking Wall-Clock Authority",
		"proven_blocking_wall_clock=3.584ms",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("missing-wakeup absence was promoted through %q:\n%s", forbidden, got)
		}
	}
}

func TestRenderAnswerDocObservationLedgerCarriesTraceBlockingWallClockAuthority(t *testing.T) {
	record := types.ObservationRecord{
		ID:              "blocking:binder",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef: types.ObservationSourceRef{
			Kind:       types.ObservationSourceRuntimeArtifact,
			ArtifactID: "attached_trace.txt",
		},
		Span:      types.ObservationSpan{StartTs: 13762.835861, EndTs: 13762.837270},
		ClaimKey:  "critical_blocking:binder_wait",
		Predicate: "critical_blocking",
		Subject:   ".ugc.aweme.lite-17267",
		Object:    "binder:496_9-10961",
		Value:     "1.409",
		Unit:      "ms",
		RichNotes: []string{
			"type=binder_wait",
			"peer=binder:496_9-10961",
			"flags=0x10",
			"blocking_candidate=true",
			"selected_window=13762.791708..13763.024898",
			types.TraceNoteKeyCapacityTruncated + "=true",
		},
	}
	mut := types.NewMutableState("分析 trace")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName:     "trace_query",
		Success:      true,
		Observations: []types.ObservationRecord{record},
	}}})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentRootCause,
			RuntimeTargets: []types.RuntimeTarget{{
				Kind:   types.RuntimeTargetKindThread,
				PID:    17267,
				Thread: ".ugc.aweme.lite-17267",
				Source: "user_explicit",
			}},
		}},
	}
	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{
		"### Trace Target Blocking Wall-Clock Authority",
		"blocking_type=`binder_wait`",
		"proven_blocking_wall_clock=1.409ms",
		"blocking_occurrences_present=`true`",
		"coverage_status=`lower_bound_capacity_truncated`",
		"interval=`13762.835861..13762.837270`",
		"peer=`binder:496_9-10961`",
		"transaction/reply latency",
		"Zero `D-state`/uninterruptible time cannot refute",
		"do not say total/all/only",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("blocking-wall-clock authority missing %q:\n%s", want, got)
		}
	}
}

func TestRenderAnswerDocObservationLedgerCarriesTraceIPCRequestCensusAuthority(t *testing.T) {
	set := types.ObservationRecord{
		ID:              "ipc:set",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef: types.ObservationSourceRef{
			Kind:       types.ObservationSourceRuntimeArtifact,
			ArtifactID: "attached_trace",
		},
		Span:      types.ObservationSpan{StartTs: 13762.791708, EndTs: 13763.024898},
		ClaimKey:  "ipc_request_census:.ugc.aweme.lite-17267",
		Predicate: "ipc_request_census",
		Subject:   ".ugc.aweme.lite-17267",
		Value:     "15",
		Unit:      "requests",
		RichNotes: []string{
			types.TraceNoteKeySelectedWindow + "=13762.791708..13763.024898",
			types.TraceNoteKeyIPCRequestCensusStatus + "=complete",
			types.TraceNoteKeyIPCSyncRequestCount + "=5",
			types.TraceNoteKeyIPCOnewayRequestCount + "=10",
			types.TraceNoteKeyIPCUnknownRequestCount + "=0",
		},
	}
	row := set
	row.ID = "ipc:12145859"
	row.ClaimKey = "ipc_request_edge:.ugc.aweme.lite-17267:12145859"
	row.Predicate = "ipc_request_edge"
	row.Object = "binder:496_9-10961"
	row.Value = "12145859"
	row.Unit = "transaction_id"
	row.Span = types.ObservationSpan{StartTs: 13762.835811, EndTs: 13762.835943}
	row.RichNotes = []string{
		types.TraceNoteKeySelectedWindow + "=13762.791708..13763.024898",
		types.TraceNoteKeyIPCTransactionID + "=12145859",
		types.TraceNoteKeyIPCCallSemantics + "=sync_request",
		types.TraceNoteKeyIPCFlags + "=0x10",
		types.TraceNoteKeyIPCFlagsKnown + "=true",
		types.TraceNoteKeyIPCCode + "=0x19",
		types.TraceNoteKeyIPCCodeKnown + "=true",
		types.TraceNoteKeyIPCReceiverSource + "=matched_receive",
	}
	rows := []types.ObservationRecord{set}
	for i := 0; i < 5; i++ {
		copy := row
		copy.ID = row.ID + string(rune('a'+i))
		copy.RichNotes = append([]string(nil), row.RichNotes...)
		copy.RichNotes[1] = types.TraceNoteKeyIPCTransactionID + "=" + string(rune('1'+i))
		copy.Span.StartTs += float64(i)
		copy.Span.EndTs += float64(i)
		rows = append(rows, copy)
	}
	rows[1] = row

	mut := types.NewMutableState("分析 trace")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName:     "trace_query",
		Success:      true,
		Observations: rows,
	}}})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentRootCause,
			RuntimeTargets: []types.RuntimeTarget{{
				Kind:   types.RuntimeTargetKindThread,
				PID:    17267,
				Thread: ".ugc.aweme.lite-17267",
				Source: "user_explicit",
			}},
		}},
	}
	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{
		"### Trace IPC Request Census Authority",
		"requests=15",
		"sync_request=5",
		"oneway_request=10",
		"transaction=12145859",
		"code=`0x19`",
		"request counts and target blocking-occurrence counts are separate",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("IPC request census authority missing %q:\n%s", want, got)
		}
	}
}
