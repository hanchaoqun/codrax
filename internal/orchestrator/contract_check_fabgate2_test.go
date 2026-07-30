package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// FABGATE-2 factory-floor tripwire pins (contract_check.go). The pure
// core is tested with an injected poisoned plan because the honest
// pipeline can no longer mint one (FABGATE-1 + the minting-side witness
// floor); reaching the poisoned state at runtime means a regression.

func TestRuntimeGroundingWitnessFloorCheck_PoisonedPlanTrips(t *testing.T) {
	poisoned := &types.AnswerSurfacePlan{RuntimeGroundingDisposition: &types.RuntimeGroundingDisposition{
		Source:         types.RuntimeGroundingSystemDetected,
		Reason:         types.EvidenceFloorWaiverExternalTrace,
		Rationale:      "runtime trace observations were available without a required current-source lane",
		CitationPolicy: types.RuntimeGroundingCitationRuntimeObservation,
	}}
	violations := runtimeGroundingWitnessFloorViolations(poisoned, false, types.ObservationLedger{})
	if len(violations) != 1 {
		t.Fatalf("witnessless runtime-grounded plan must trip exactly once; got %d", len(violations))
	}
	v := violations[0]
	if v.Kind != types.ViolAuthorityOverreach {
		t.Errorf("kind = %v, want ViolAuthorityOverreach (reused kind, no new typed signal)", v.Kind)
	}
	if v.ClusterKey != "root:runtime_grounding_witness_floor" {
		t.Errorf("cluster key = %q", v.ClusterKey)
	}
	if !strings.Contains(v.Repair, "emit_answer_document") || !strings.Contains(v.Repair, "/htrace") {
		t.Errorf("repair must be LLM-actionable (tool name + user remedy); got %q", v.Repair)
	}
}

func TestRuntimeGroundingWitnessFloorCheck_HealthyShapesSilent(t *testing.T) {
	witnessed := types.ObservationLedger{Records: []types.ObservationRecord{{
		Origin:   types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "perf_triage",
		SourceRef: types.ObservationSourceRef{
			Kind:         types.ObservationSourceRuntimeArtifact,
			ArtifactKind: "trace",
		},
	}}}
	active := &types.AnswerSurfacePlan{RuntimeGroundingDisposition: &types.RuntimeGroundingDisposition{
		Source:         types.RuntimeGroundingSystemDetected,
		Reason:         types.EvidenceFloorWaiverExternalTrace,
		Rationale:      "runtime trace observations were available without a required current-source lane",
		CitationPolicy: types.RuntimeGroundingCitationRuntimeObservation,
	}}
	if got := runtimeGroundingWitnessFloorViolations(active, false, witnessed); len(got) != 0 {
		t.Fatalf("witnessed run must not trip: %+v", got)
	}
	if got := runtimeGroundingWitnessFloorViolations(active, true, types.ObservationLedger{}); len(got) != 0 {
		t.Fatalf("attached artifact must not trip: %+v", got)
	}
	if got := runtimeGroundingWitnessFloorViolations(nil, false, types.ObservationLedger{}); len(got) != 0 {
		t.Fatalf("nil plan must not trip: %+v", got)
	}
	if got := runtimeGroundingWitnessFloorViolations(&types.AnswerSurfacePlan{}, false, types.ObservationLedger{}); len(got) != 0 {
		t.Fatalf("no-disposition plan must not trip: %+v", got)
	}
}
