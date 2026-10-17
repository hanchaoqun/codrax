package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc257SourceDomainPayload(t *testing.T, request string) map[string]any {
	p := hmc256IdentityPayload(t, request)
	p["intent"], p["question_kind"] = "enumerate", "enumeration"
	p["predicates"].(map[string]any)["has_per_member_table"] = true
	p["current_source_explanation_profile"] = map[string]any{"is_current_source_explanation_requested": false, "confidence": .9}
	p["requested_answer_dimensions"] = map[string]any{"is_dimensioned_answer": true, "confidence": .9, "dimensions": []any{
		map[string]any{"label": "recorded callsites", "role": "observed_value", "source_quote": "recorded callsites", "required": true, "index": 1},
		map[string]any{"label": "completion status", "role": "observed_value", "source_quote": "completion status", "required": true, "index": 2},
	}}
	return p
}

func TestHMC257EmitNamedCaptureDomainPublic(t *testing.T) {
	const request = "List recorded callsites and completion status from the named captures."
	for _, policy := range []string{"default", "exclude"} {
		t.Run(policy, func(t *testing.T) {
			bus := &types.BusContext{Mutable: types.NewMutableState(request), TurnRouteHint: types.TurnRouteHint{Route: "repo", Source: "repo", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired, RequiredOutcomes: types.TurnOutcomeAnswer | types.TurnOutcomeMeasurement | types.TurnOutcomeSourceExplanation}}
			p := hmc257SourceDomainPayload(t, request)
			p["external_observation_policy"] = map[string]any{"current_source_mode": policy, "artifact_citation_mode": "external_only", "confidence": .9}
			if policy == "exclude" {
				p["external_observation_policy"].(map[string]any)["exclusion_kind"] = "explicit_user_exclusion"
				p["external_observation_policy"].(map[string]any)["current_source_exclusion_quote"] = request
			}
			if out := hmc256EmitIdentity(t, bus, p); !out.Success {
				t.Fatalf("valid named runtime question rejected: %s", out.Summary)
			}
			rm := bus.Mutable.RequestModel()
			bus.AnalysisIR = &types.AnalysisIR{RequestModel: *rm}
			agent := types.SubAgentContext(bus, &types.SubAgentRequest{SubAgent: "explorer"})
			// Stage construction supplies the accepted analysis separately from
			// the common sub-agent projection.
			agent.AnalysisIR = bus.AnalysisIR
			if rm.SourceInventoryProfile.Active() {
				t.Errorf("runtime member table synthesized source inventory: %+v", rm.SourceInventoryProfile)
			}
			for _, authority := range []types.RuntimeSourceAnswerAuthoritySnapshot{types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(bus, types.ObservationLedger{}), types.BuildRuntimeSourceAnswerAuthoritySnapshotForAgentContext(agent, types.ObservationLedger{})} {
				if authority.CurrentSourceRequired || authority.CurrentSourceSatisfied || authority.RuntimeObservationCount != 0 {
					t.Errorf("request-only domain minted source obligation/facts/satisfaction: %+v", authority)
				}
			}
			if !rm.RuntimeArtifactScopeProfile.FullArtifact() || !rm.RuntimeQuestionProfile.BoundedFactSet() || len(rm.RequestedAnswerDimensions.Dimensions) != 2 {
				t.Fatal("source domain repair damaged requested facts/scope")
			}
		})
	}
}

func TestHMC257ExcludedSourceLocationRepairPublic(t *testing.T) {
	const request = "List recorded callsites and completion status from the named captures; do not inspect current source."
	bus := &types.BusContext{Mutable: types.NewMutableState(request)}
	p := hmc257SourceDomainPayload(t, request)
	p["external_observation_policy"] = map[string]any{"current_source_mode": "exclude", "artifact_citation_mode": "external_only", "exclusion_kind": "explicit_user_exclusion", "current_source_exclusion_quote": "do not inspect current source", "confidence": .9}
	dim := p["requested_answer_dimensions"].(map[string]any)["dimensions"].([]any)[0].(map[string]any)
	dim["role"] = "source_location"
	bad := hmc256EmitIdentity(t, bus, p)
	if bad.Success || bus.Mutable.RequestModel() != nil {
		t.Fatalf("contradictory source role accepted/published: %s", bad.Summary)
	}
	for _, want := range []string{"source_location", "current_source_mode=exclude", "observed_value", "complete", "not yet accepted"} {
		if !strings.Contains(bad.Summary, want) {
			t.Errorf("missing precise neutral repair %q: %s", want, bad.Summary)
		}
	}
	if dim["role"] != "source_location" {
		t.Fatal("system rewrote model role")
	}
	dim["role"] = "observed_value"
	if good := hmc256EmitIdentity(t, bus, p); !good.Success {
		t.Fatalf("model-owned role repair rejected: %s", good.Summary)
	}
	if rm := bus.Mutable.RequestModel(); rm.SourceInventoryProfile.Active() || rm.RequestedAnswerDimensions.Dimensions[0].Role != types.RequestedAnswerDimensionObservedValue {
		t.Fatal("corrected observed callsite lost or became source inventory")
	}
}

func TestHMC257CausalScopeRepairDoesNotInventMissingDimensionPublic(t *testing.T) {
	const request = "List recorded callsites and completion status from the named captures."
	bus := &types.BusContext{Mutable: types.NewMutableState(request)}
	p := hmc257SourceDomainPayload(t, request)
	p["runtime_question_profile"].(map[string]any)["scope"] = "causal_diagnosis"
	bad := hmc256EmitIdentity(t, bus, p)
	if bad.Success || bus.Mutable.RequestModel() != nil {
		t.Fatalf("scope conflict accepted: %s", bad.Summary)
	}
	if strings.Contains(bad.Summary, "preserve both on retry instead of demoting") || !strings.Contains(bad.Summary, "no required causal dimension") {
		t.Fatalf("repair promoted rejected classification into confirmed breadth: %s", bad.Summary)
	}
	p["runtime_question_profile"].(map[string]any)["scope"] = "bounded_fact_set"
	if good := hmc256EmitIdentity(t, bus, p); !good.Success {
		t.Fatalf("finite model-owned repair rejected: %s", good.Summary)
	}
	if rm := bus.Mutable.RequestModel(); len(rm.RequestedAnswerDimensions.Dimensions) != 2 || !rm.RuntimeQuestionProfile.BoundedFactSet() {
		t.Fatal("repair added causal obligations")
	}
	// Keep the established causal branch and its full projection breadth.
	raw, causal := mixedRuntimePayload(t, "causal_attribution", true, true)
	causal["runtime_question_profile"].(map[string]any)["fact_families"] = []string{"other_observed_value"}
	cb := &types.BusContext{Mutable: types.NewMutableState(raw)}
	rejected := hmc256EmitIdentity(t, cb, causal)
	if rejected.Success || !strings.Contains(rejected.Summary, "already-typed required causal dimension") {
		t.Fatalf("existing causal breadth lost its precise repair: %s", rejected.Summary)
	}
	delete(causal["runtime_question_profile"].(map[string]any), "fact_families")
	if good := hmc256EmitIdentity(t, cb, causal); !good.Success {
		t.Fatalf("causal repair rejected: %s", good.Summary)
	}
	rm := cb.Mutable.RequestModel()
	if decided, allowed := types.RuntimeTraceReportShapeAuthority(rm); !decided || !allowed || !rm.RuntimeQuestionProfile.RuntimeWorkRelationRequested || !rm.RuntimeQuestionProfile.FrameCausalityRequested {
		b, _ := json.Marshal(rm)
		t.Fatalf("explicit causal projection was suppressed: %s", b)
	}
}
