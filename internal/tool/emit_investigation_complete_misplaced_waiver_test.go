package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEmitInvestigationComplete_RecoversNoDirectedPathWaiverFromStringEncodedAggregateTail(t *testing.T) {
	mut := types.NewMutableState("trace buildAnalysisIR to gate.Run")
	mut.AppendEvidence([]types.EvidenceItem{
		{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "buildAnalysisIR", Predicate: "calls", Object: "gate.RunWith", AnchorSymbol: "gate.RunWith", Source: "internal/agent/analyzer.go", LineStart: 2666, GroundingStatus: types.GroundingGrounded},
		{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "gate.Run", Predicate: "calls", Object: "gate.RunWith", AnchorSymbol: "gate.RunWith", Source: "internal/analysis/gate/gate.go", LineStart: 135, GroundingStatus: types.GroundingGrounded},
	})
	mut.EvidenceClosure().SetReadSet(map[string]bool{
		"internal/agent/analyzer.go":     true,
		"internal/analysis/gate/gate.go": true,
	})
	bus := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:                   types.IntentTrace,
		PredicateAxis:            types.AxisCall,
		CallChainEndpointProfile: orderedCallChainEndpoints("buildAnalysisIR", "gate.Run"),
		AnalyzerHints:            types.AnalyzerHints{Kind: string(types.ReqCallChain), ExactTargets: []string{"buildAnalysisIR", "gate.Run"}, MentionedEntities: []string{"buildAnalysisIR", "gate.Run"}},
	}}}
	payload, err := json.Marshal(map[string]any{
		"reason":          "the exact requested sink is a parallel wrapper endpoint",
		"confidence":      "high",
		"result_kind":     "resolved",
		"aggregate_facts": `[], "principal_span_waiver": {"reason":"no_directed_path","rationale":"buildAnalysisIR calls gate.RunWith while gate.Run independently calls RunWith"}`,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	res, err := (&EmitInvestigationComplete{}).Execute(bus, payload)
	if err != nil || !res.Success || !mut.IsInvestigationComplete() {
		t.Fatalf("string-tail no_directed_path waiver must close once: res=%+v err=%v", res, err)
	}
	if got := mut.PrincipalSpanWaiver(); got == nil || got.Reason != types.PrincipalSpanWaiverNoDirectedPath {
		t.Fatalf("recovered principal waiver = %+v", got)
	}
	if !strings.Contains(res.Summary, "typed call-chain endpoint boundary accepted") {
		t.Fatalf("completion summary did not publish typed endpoint boundary: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_MisplacedWaiverFamiliesPreserveTopLevelPriorityAndConflictChecks(t *testing.T) {
	t.Run("top level family wins", func(t *testing.T) {
		var got emitInvestigationCompleteParams
		err := json.Unmarshal([]byte(`{
			"reason":"done","confidence":"high","result_kind":"resolved",
			"principal_span_waiver":{"reason":"no_directed_path","rationale":"top-level boundary"},
			"aggregate_facts":"[], \"principal_span_waiver\": {\"reason\":\"inlined_call\",\"rationale\":\"tail must not replace top level\"}, \"clear_principal_span_waiver\": true"
		}`), &got)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.PrincipalSpanWaiver == nil || got.PrincipalSpanWaiver.Reason != "no_directed_path" || got.ClearPrincipalSpanWaiver {
			t.Fatalf("top-level waiver family lost priority: %+v", got)
		}
	})

	t.Run("tail declaration and clear remain contradictory", func(t *testing.T) {
		mut := types.NewMutableState("q")
		payload, err := json.Marshal(map[string]any{
			"reason":          "done",
			"confidence":      "high",
			"result_kind":     "resolved",
			"aggregate_facts": `[], "evidence_floor_waiver": {"reason":"external_only_log","rationale":"external runtime only"}, "clear_evidence_floor_waiver": true`,
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		res, err := (&EmitInvestigationComplete{}).Execute(&types.BusContext{Mutable: mut}, payload)
		if err != nil || res.Success || !strings.Contains(res.Summary, "clear_evidence_floor_waiver cannot be set together") {
			t.Fatalf("recovered contradictory family must use existing reject: res=%+v err=%v", res, err)
		}
	})
}

func TestEmitInvestigationComplete_MisplacedWaiverStrictDecodeRejectsUnknownNestedField(t *testing.T) {
	var got emitInvestigationCompleteParams
	err := json.Unmarshal([]byte(`{
		"reason":"done","confidence":"high","result_kind":"resolved",
		"aggregate_facts":"[], \"principal_span_waiver\": {\"reason\":\"no_directed_path\",\"rationale\":\"bounded\",\"invented\":true}"
	}`), &got)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("misplaced typed waiver must retain strict nested schema, got %v", err)
	}
}
