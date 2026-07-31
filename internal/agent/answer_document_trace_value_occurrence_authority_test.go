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
