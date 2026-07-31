package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderAnswerDocObservationLedger_IncludesCrossArtifactRelationAuthority(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{
			answerDocRuntimeArtifactPairTestRecord("a", "a.systrace"),
			answerDocRuntimeArtifactPairTestRecord("b", "b.systrace"),
		},
	}}})
	got := renderAnswerDocObservationLedger(&types.AgentContext{Mutable: mu})
	for _, want := range []string{
		"### Typed Cross-Artifact Relation Authority",
		"left=`a.systrace`; right=`b.systrace`",
		"shared_clock_origin=`unproven`",
		"direct_time_alignment=`unproven`",
		"shared_device=`unproven`",
		"shared_capture_session=`unproven`",
		"same_time_domain_label=true",
		"local_identity_only=true",
		"A subtraction of local timestamps is only a numeric offset",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("cross-artifact relation prompt missing %q:\n%s", want, got)
		}
	}
}

func TestRenderAnswerDocObservationLedger_OmitsCrossArtifactRelationForSingleArtifact(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName:     "trace_query",
		Success:      true,
		Observations: []types.ObservationRecord{answerDocRuntimeArtifactPairTestRecord("a", "a.systrace")},
	}}})
	got := renderAnswerDocObservationLedger(&types.AgentContext{Mutable: mu})
	if strings.Contains(got, "Typed Cross-Artifact Relation Authority") {
		t.Fatalf("single artifact must not publish pair authority:\n%s", got)
	}
}

func answerDocRuntimeArtifactPairTestRecord(id, path string) types.ObservationRecord {
	return types.ObservationRecord{
		ID:              "trace_query:" + id,
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef: types.ObservationSourceRef{
			Kind:                types.ObservationSourceRuntimeArtifact,
			ArtifactID:          id,
			ArtifactKind:        "trace",
			Path:                path,
			TimeDomain:          "trace_seconds",
			CanonicalTimeDomain: "trace_seconds",
			ClockAlignment:      "identity",
		},
		ClaimKey: "artifact_coverage:" + id,
	}
}
