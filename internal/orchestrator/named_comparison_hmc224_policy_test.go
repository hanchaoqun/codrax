package orchestrator

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The public emitter owns normalization and quote provenance. Tests retain its
// policy verbatim rather than manufacturing a source-exclusion prerequisite.
func hmc224EmitNaturalComparison(t *testing.T, bus *types.BusContext, request string, scope *types.RuntimeArtifactScopeProfile, policy string) types.RequestModel {
	t.Helper()
	p := softSourcePublicAnalysis("")
	p["intent"], p["scenario"] = "explain", "generic"
	p["keywords"], p["entities"] = []string{"measurements"}, []string{"captures"}
	p["predicates"].(map[string]any)["is_diagnostic_question"] = false
	p["diagnostic_profile"].(map[string]any)["is_diagnostic"] = false
	delete(p, "runtime_targets")
	p["runtime_target_profile"] = map[string]any{"declaration": "no_named_target", "confidence": 1}
	p["runtime_artifact_scope_profile"] = map[string]any{"requested_scope": scope.RequestedScope, "source_quote": scope.SourceQuote, "confidence": 1, "comparison": scope.Comparison}
	p["runtime_question_profile"] = map[string]any{"scope": "bounded_fact_set", "fact_families": []string{"resource_pressure"}, "runtime_work_relation_requested": false, "frame_causality_requested": false, "confidence": 1}
	p["requested_answer_dimensions"] = map[string]any{"is_dimensioned_answer": true, "confidence": 1, "dimensions": []any{map[string]any{"index": 1, "label": "original records", "role": "observed_value", "required": true, "source_quote": "original measurement records"}}}
	if policy != "nil" {
		p["external_observation_policy"] = map[string]any{"current_source_mode": policy, "artifact_citation_mode": "default", "confidence": 1, "rationale": "Runtime input acquisition does not exclude source analysis."}
	}
	if policy == "allow" {
		// This is the live classifier's route; the analyzer must not replace it
		// with an exclusion just to activate independent runtime acquisition.
		bus.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "mixed", NeedsRepoAccess: true, NeedsDataAccess: true, Confidence: 1, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired}
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	out, err := (&tool.EmitAnalysis{}).Execute(bus, raw)
	if err != nil || !out.Success {
		t.Fatalf("public comparison analysis failed: %v / %s", err, out.Summary)
	}
	rm := bus.Mutable.RequestModel()
	if rm == nil || rm.RuntimeArtifactScopeProfile == nil || !rm.RuntimeArtifactScopeProfile.Comparison.Valid() {
		t.Fatal("public analyzer lost independent comparison")
	}
	if policy == "nil" && rm.ExternalObservationPolicy != nil || policy != "nil" && (rm.ExternalObservationPolicy == nil || string(rm.ExternalObservationPolicy.CurrentSourceMode) != policy) {
		t.Fatalf("unexpected normalized policy: wanted=%s actual=%+v", policy, rm.ExternalObservationPolicy)
	}
	return *rm
}

func TestHMC224ComparisonPolicyDoesNotAuthorizeUnboundInputs(t *testing.T) {
	profile := types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: "/fixtures/converter.htrace", Carrier: "request_path"}}}
	for _, policy := range []*types.ExternalObservationPolicy{nil,
		{CurrentSourceMode: types.ExternalObservationCurrentSourceDefault},
		{CurrentSourceMode: types.ExternalObservationCurrentSourceAllow},
	} {
		for _, scope := range []*types.RuntimeArtifactScopeProfile{nil, {Comparison: &types.RuntimeArtifactComparisonScope{}}} {
			rm := types.RequestModel{ExternalObservationPolicy: policy, RuntimeArtifactScopeProfile: scope}
			if typedNamedTraceAdmissionEnabled(rm, profile, "Explain the source converter handling `/fixtures/converter.htrace`.") {
				t.Fatalf("unbound or invalid comparison acquired preparation intent: policy=%+v scope=%+v", policy, scope)
			}
		}
	}
}
