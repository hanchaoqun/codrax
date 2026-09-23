package types

import (
	"encoding/json"
	"reflect"
	"sync"
	"testing"
)

func TestToolHandoffNormalizationPublicInputImmutable(t *testing.T) {
	for _, path := range []string{"attach_result", "attach_existing", "turn_a_result", "turn_a_explicit"} {
		t.Run(path, func(t *testing.T) {
			source := handoffNormalizationPublicSource(false)
			before := handoffNormalizationPublicJSON(t, source)
			got := handoffNormalizationPublicProject(t, path, source)
			if got.Refinement == nil || !reflect.DeepEqual(got.Refinement.PreferredParams, map[string]string{"path": "capture.systrace"}) {
				t.Fatalf("normalization lost its original trimming/filtering behavior: %+v", got.Refinement)
			}
			if path != "attach_result" && (got.SupportedJSON == nil || !reflect.DeepEqual(got.SupportedJSON.AcceptedEnums, map[string][]string{"mode": {"window", "summary"}})) {
				t.Fatalf("enum normalization lost its original trimming/filtering behavior: %+v", got.SupportedJSON)
			}
			if after := handoffNormalizationPublicJSON(t, source); after != before {
				t.Fatalf("public projection mutated caller-owned input\nbefore=%s\nafter=%s", before, after)
			}
		})
	}
}

func TestToolHandoffNormalizationPublicResultsDetached(t *testing.T) {
	for _, path := range []string{"attach_result", "attach_existing", "turn_a_result", "turn_a_explicit"} {
		t.Run(path, func(t *testing.T) {
			source := handoffNormalizationPublicSource(true)
			first := handoffNormalizationPublicProject(t, path, source)
			second := handoffNormalizationPublicProject(t, path, source)
			beforeSource := handoffNormalizationPublicJSON(t, source)
			beforeSecond := handoffNormalizationPublicJSON(t, second)
			handoffNormalizationPublicMutate(&first)
			if after := handoffNormalizationPublicJSON(t, source); after != beforeSource {
				t.Errorf("writing normalized output mutated input: before=%s after=%s", beforeSource, after)
			}
			if after := handoffNormalizationPublicJSON(t, second); after != beforeSecond {
				t.Errorf("two public projections share mutable storage: before=%s after=%s", beforeSecond, after)
			}
		})
	}
}

func TestToolHandoffNormalizationPublicPlanRepairPack(t *testing.T) {
	for _, path := range []string{"direct", "json", "attach_existing", "turn_a_result", "turn_a_explicit"} {
		t.Run(path+"/input_immutable", func(t *testing.T) {
			source := handoffNormalizationPublicSource(true)
			source.Handoff.PlanRepairPack = handoffNormalizationPublicPack(false)
			before := handoffNormalizationPublicJSON(t, source)
			got := handoffNormalizationPublicProjectPack(t, path, source)
			if len(got.CurrentBytes) != 1 || got.CurrentBytes[0].Path != "main.go" || len(got.CurrentBytes[0].RelocationCandidates) != 1 || got.Metadata["origin"] != "current" || !reflect.DeepEqual(got.AcceptedEnums["kind"], []string{"replace"}) {
				t.Fatalf("pack normalization lost its original trimming/filtering behavior: %+v", got)
			}
			if after := handoffNormalizationPublicJSON(t, source); after != before {
				t.Fatalf("pack public projection mutated input\nbefore=%s\nafter=%s", before, after)
			}
		})
		t.Run(path+"/results_detached", func(t *testing.T) {
			source := handoffNormalizationPublicSource(true)
			source.Handoff.PlanRepairPack = handoffNormalizationPublicPack(true)
			first := handoffNormalizationPublicProjectPack(t, path, source)
			second := handoffNormalizationPublicProjectPack(t, path, source)
			beforeSource := handoffNormalizationPublicJSON(t, source)
			beforeSecond := handoffNormalizationPublicJSON(t, second)
			handoffNormalizationPublicMutatePack(&first)
			if after := handoffNormalizationPublicJSON(t, source); after != beforeSource {
				t.Errorf("writing normalized pack mutated input: before=%s after=%s", beforeSource, after)
			}
			if after := handoffNormalizationPublicJSON(t, second); after != beforeSecond {
				t.Errorf("pack projections share mutable storage: before=%s after=%s", beforeSecond, after)
			}
		})
	}
}

// Run this test with -race only after the sequential mutation oracles pass.
// The shared input is never changed; writes below affect only each caller's
// returned projection, just as independent downstream contexts may do.
func TestToolHandoffNormalizationPublicConcurrentReaders(t *testing.T) {
	for _, tc := range []struct {
		name      string
		canonical bool
	}{{"needs_normalization", false}, {"already_canonical", true}} {
		t.Run(tc.name, func(t *testing.T) {
			source := handoffNormalizationPublicSource(tc.canonical)
			source.Handoff.PlanRepairPack = handoffNormalizationPublicPack(tc.canonical)
			before := handoffNormalizationPublicJSON(t, source)
			paths := []string{"attach_result", "attach_existing", "turn_a_result", "turn_a_explicit"}
			var workers sync.WaitGroup
			for i := 0; i < 12; i++ {
				workers.Add(1)
				go func(path string) {
					defer workers.Done()
					for j := 0; j < 20; j++ {
						got := handoffNormalizationPublicProject(t, path, source)
						if got.Refinement.PreferredParams["path"] != "capture.systrace" {
							t.Error("another context changed the current projection")
						}
						handoffNormalizationPublicMutate(&got)
						if got.PlanRepairPack != nil {
							handoffNormalizationPublicMutatePack(got.PlanRepairPack)
						}
					}
				}(paths[i%len(paths)])
			}
			workers.Wait()
			if after := handoffNormalizationPublicJSON(t, source); after != before {
				t.Fatalf("concurrent public projections changed shared input: before=%s after=%s", before, after)
			}
		})
	}
}

func handoffNormalizationPublicPack(canonical bool) *PlanRepairPack {
	pack := &PlanRepairPack{ReasonCode: "invalid_edit", ToolName: "emit_change_plan",
		AcceptedEnums: map[string][]string{"kind": {"replace"}}, Metadata: map[string]string{"origin": "current"},
		CurrentBytes: []PlanRepairCurrentBytes{{Path: "main.go", CurrentBytes: "package main", SafeEditKinds: []string{"replace"},
			RelocationCandidates: []PlanRepairRelocationCandidate{{Path: "main.go", StartLine: 2, EndLine: 3, Source: "current"}}}},
	}
	if !canonical {
		pack.AcceptedEnums = map[string][]string{" kind ": {" replace ", "replace", ""}, "": {"discard"}}
		pack.Metadata = map[string]string{" origin ": " current ", "": "discard"}
		pack.CurrentBytes[0].Path = " main.go "
		pack.CurrentBytes[0].SafeEditKinds = []string{" replace ", "replace", ""}
		pack.CurrentBytes[0].RelocationCandidates = []PlanRepairRelocationCandidate{{}, {Path: " main.go ", StartLine: 2, EndLine: 3, Source: " current "}}
		pack.CurrentBytes = append(pack.CurrentBytes, PlanRepairCurrentBytes{})
	}
	return pack
}

func handoffNormalizationPublicProjectPack(t *testing.T, path string, source ToolResult) PlanRepairPack {
	t.Helper()
	switch path {
	case "direct":
		return NormalizePlanRepairPack(*source.Handoff.PlanRepairPack)
	case "json":
		raw := PlanRepairPackJSON(*source.Handoff.PlanRepairPack)
		var pack PlanRepairPack
		if err := json.Unmarshal([]byte(raw), &pack); err != nil {
			t.Fatal(err)
		}
		return pack
	default:
		carrier := handoffNormalizationPublicProject(t, path, source)
		if carrier.PlanRepairPack == nil {
			t.Fatal("public handoff lost its plan-repair pack")
		}
		return *carrier.PlanRepairPack
	}
}

func handoffNormalizationPublicMutatePack(pack *PlanRepairPack) {
	pack.AcceptedEnums["kind"][0] = "changed"
	pack.AcceptedEnums["new"] = []string{"changed"}
	pack.Metadata["origin"] = "changed"
	pack.CurrentBytes[0].Path = "changed"
	pack.CurrentBytes[0].SafeEditKinds[0] = "changed"
	pack.CurrentBytes[0].RelocationCandidates[0].Path = "changed"
}

func handoffNormalizationPublicSource(canonical bool) ToolResult {
	params := map[string]string{" path ": " capture.systrace ", "": "discard"}
	enums := map[string][]string{" mode ": {" window ", "summary", "window", ""}, "": {"discard"}}
	if canonical {
		params = map[string]string{"path": "capture.systrace"}
		enums = map[string][]string{"mode": {"window", "summary"}}
	}
	refinement := &ToolRefinementHint{
		ReasonCode: "query_refinement", PreferredNextTool: "trace_query", PreferredParams: params,
		RequiredFields: []string{"path"}, ExcludedRoots: []string{"archive"},
		ParamNarrowingSuggestions: []ToolParamNarrowingSuggestion{{Param: "time_start", Suggested: "1", Priority: 1}},
	}
	return ToolResult{ToolName: "trace_query", Success: true, Refinement: refinement, Handoff: &ToolHandoffCarrier{
		Version: ToolHandoffCarrierVersion, ToolName: "trace_query", ReasonCode: "query_refinement", Refinement: refinement,
		SupportedJSON: &ToolJSONSurfaceDescriptor{ToolName: "trace_query", ReasonCode: "query_refinement", AcceptedEnums: enums,
			FailingFieldPaths: []string{"time_start"}, AcceptedFieldPaths: []string{"path", "time_start"}},
		AcceptedEvidence: []AcceptedEvidenceRef{{ID: "e1", Source: "capture.systrace", LineStart: 1}},
		ObservationRefs:  []ToolObservationRef{{ID: "o1", Source: "capture.systrace", LineStart: 1}},
	}}
}

func handoffNormalizationPublicProject(t *testing.T, path string, source ToolResult) ToolHandoffCarrier {
	t.Helper()
	var carriers []ToolHandoffCarrier
	switch path {
	case "attach_result":
		source.Handoff = nil
		fallthrough
	case "attach_existing":
		result := AttachToolHandoffCarrier(source)
		if result.Handoff == nil {
			t.Fatal("public attach dropped a nonempty typed carrier")
		}
		return *result.Handoff
	case "turn_a_result":
		carriers = ToolHandoffCarriersFromTurnAInputs([]ToolResult{source, source}, nil, nil)
	case "turn_a_explicit":
		carriers = ToolHandoffCarriersFromTurnAInputs(nil, nil, []ToolHandoffCarrier{*source.Handoff, *source.Handoff})
	}
	if len(carriers) != 1 {
		t.Fatalf("equivalent handoffs should preserve existing merge semantics: %+v", carriers)
	}
	return carriers[0]
}

func handoffNormalizationPublicMutate(carrier *ToolHandoffCarrier) {
	carrier.Refinement.PreferredParams["path"] = "changed"
	carrier.Refinement.RequiredFields[0] = "changed"
	carrier.Refinement.ExcludedRoots[0] = "changed"
	carrier.Refinement.ParamNarrowingSuggestions[0].Suggested = "changed"
	if carrier.SupportedJSON != nil {
		carrier.SupportedJSON.AcceptedEnums["mode"][0] = "changed"
		carrier.SupportedJSON.AcceptedEnums["new"] = []string{"changed"}
		carrier.SupportedJSON.FailingFieldPaths[0] = "changed"
		carrier.SupportedJSON.AcceptedFieldPaths[0] = "changed"
		carrier.AcceptedEvidence[0].Source = "changed"
		carrier.ObservationRefs[0].Source = "changed"
	}
}

func handoffNormalizationPublicJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
