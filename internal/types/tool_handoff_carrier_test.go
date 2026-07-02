package types

import (
	"strconv"
	"testing"
)

func TestToolHandoffCarrierFromToolResultUsesTypedRepairAndObservations(t *testing.T) {
	pack := NormalizePlanRepairPack(PlanRepairPack{
		ToolName:          "emit_change_plan",
		ReasonCode:        "invalid_enum",
		FailingFieldPaths: []string{"$.changes[0].edits[0].kind"},
		AcceptedEnums: map[string][]string{
			"$.changes[].edits[].kind": {"replace", "insert", "delete"},
		},
	})
	result := ToolResult{
		ToolName: "emit_change_plan",
		Success:  false,
		Repair: &ToolRepair{
			Code:   PlanRepairToolCode,
			Fields: []string{"$.changes[0].edits[0].kind"},
			Metadata: map[string]string{
				PlanRepairMetadataKey: PlanRepairPackJSON(pack),
				"reason_code":         "invalid_enum",
			},
		},
		Observations: []ObservationRecord{{
			ID:              "obs-1",
			Origin:          AnswerEvidenceOriginCurrentSource,
			Producer:        "read_file",
			SourceRef:       ObservationSourceRef{Kind: ObservationSourceCurrentSource, Path: "internal/app/main.py"},
			Span:            ObservationSpan{LineStart: 10, LineEnd: 12},
			ClaimKey:        "owner",
			Subject:         "main",
			GroundingPolicy: ClaimGroundingHard,
			GroundingStatus: GroundingGrounded,
		}},
	}

	result = AttachToolHandoffCarrier(result)
	if result.Handoff == nil {
		t.Fatal("expected handoff carrier")
	}
	if result.Handoff.ReasonCode != "invalid_enum" {
		t.Fatalf("reason code = %q", result.Handoff.ReasonCode)
	}
	if result.Handoff.PlanRepairPack == nil || result.Handoff.PlanRepairPack.ReasonCode != "invalid_enum" {
		t.Fatalf("plan repair pack not carried: %+v", result.Handoff.PlanRepairPack)
	}
	if result.Handoff.SupportedJSON == nil || len(result.Handoff.SupportedJSON.AcceptedEnums) != 1 {
		t.Fatalf("supported JSON surface not carried: %+v", result.Handoff.SupportedJSON)
	}
	if len(result.Handoff.ObservationRefs) != 1 || result.Handoff.ObservationRefs[0].Source != "internal/app/main.py" {
		t.Fatalf("observation refs not carried: %+v", result.Handoff.ObservationRefs)
	}
}

func TestToolHandoffCarrierFromToolResultUsesJSONSurfaceMetadata(t *testing.T) {
	surface := NormalizeToolJSONSurfaceDescriptor(ToolJSONSurfaceDescriptor{
		ToolName:           "emit_evidence",
		ReasonCode:         "tool_param_unknown_field",
		FailingFieldPaths:  []string{"$.items[].source"},
		AcceptedFieldPaths: []string{"$.items", "$.items[].source", "$.items[].line_start"},
		AcceptedEnums: map[string][]string{
			"$.items[].scope": {"line", "symbol"},
		},
	})
	result := AttachToolHandoffCarrier(ToolResult{
		ToolName: "emit_evidence",
		Success:  false,
		Repair: &ToolRepair{
			Code:   "tool_param_unknown_field",
			Fields: []string{"$.items[].source"},
			Metadata: map[string]string{
				ToolJSONSurfaceMetadataKey: ToolJSONSurfaceDescriptorJSON(surface),
			},
		},
	})
	if result.Handoff == nil || result.Handoff.SupportedJSON == nil {
		t.Fatalf("expected supported JSON carrier: %+v", result.Handoff)
	}
	if len(result.Handoff.SupportedJSON.AcceptedFieldPaths) != 3 ||
		len(result.Handoff.SupportedJSON.AcceptedEnums) != 1 {
		t.Fatalf("supported JSON surface mismatch: %+v", result.Handoff.SupportedJSON)
	}
}

func TestToolHandoffCarrierFromToolResultDropsNonActionableEvidenceLineRepair(t *testing.T) {
	result := AttachToolHandoffCarrier(ToolResult{
		ToolName: "emit_evidence",
		Success:  true,
		Repair: &ToolRepair{
			Code: ToolRepairCodeEvidenceLineTextRepair,
			Metadata: map[string]string{
				"repair_status": ToolRepairStatusSatisfiedOrNonActionable,
			},
		},
	})
	if result.Handoff != nil {
		t.Fatalf("non-actionable evidence-line repair must not become model-facing handoff: %+v", result.Handoff)
	}

	actionable := AttachToolHandoffCarrier(ToolResult{
		ToolName: "emit_evidence",
		Success:  true,
		Repair: &ToolRepair{
			Code: ToolRepairCodeEvidenceLineTextRepair,
			Targets: []ToolRepairTarget{{
				File:   "internal/app/main.go",
				Lines:  []int{12},
				Action: string(RepairReadFile),
			}},
			Metadata: map[string]string{
				"repair_status": ToolRepairStatusActionRequired,
			},
		},
	})
	if actionable.Handoff == nil || actionable.Handoff.RepairCode != ToolRepairCodeEvidenceLineTextRepair {
		t.Fatalf("action-required evidence-line repair should still be carried: %+v", actionable.Handoff)
	}
}

func TestToolHandoffCarrierFromToolResultUsesTypedRefinement(t *testing.T) {
	result := AttachToolHandoffCarrier(ToolResult{
		ToolName: "grep",
		Success:  true,
		Refinement: &ToolRefinementHint{
			ReasonCode:             "grep_result_truncated",
			ResultTruncated:        true,
			UniverseExcludedReason: "configured_search_exclude_roots",
			ExcludedRoots:          []string{".codrax", "dist"},
			PreferredNextTool:      "grep",
			PreferredParams: map[string]string{
				"path":          "internal/app/main.py",
				"context_lines": "3",
			},
			RequiredFields: []string{"pattern"},
		},
	})
	if result.Handoff == nil || result.Handoff.Refinement == nil {
		t.Fatalf("expected typed refinement carrier: %+v", result.Handoff)
	}
	if result.Handoff.ReasonCode != "grep_result_truncated" {
		t.Fatalf("reason code = %q", result.Handoff.ReasonCode)
	}
	refinement := result.Handoff.Refinement
	if !refinement.ResultTruncated || refinement.PreferredNextTool != "grep" {
		t.Fatalf("refinement not carried: %+v", refinement)
	}
	if refinement.UniverseExcludedReason != "configured_search_exclude_roots" ||
		len(refinement.ExcludedRoots) != 2 {
		t.Fatalf("excluded universe refinement not carried: %+v", refinement)
	}
	if got := refinement.PreferredParams["path"]; got != "internal/app/main.py" {
		t.Fatalf("preferred path = %q", got)
	}
	if len(refinement.RequiredFields) != 1 || refinement.RequiredFields[0] != "pattern" {
		t.Fatalf("required fields = %+v", refinement.RequiredFields)
	}
}

// TestToolRefinementParamNarrowingSuggestionsSixSpotSync pins the R2'
// six-spot sync for the ParamNarrowingSuggestions typed field: normalize
// (trim/drop/dedupe/sort/bound/deep-copy), Empty(), merge dedupe-by-Param
// keeping the lower Priority, and toolRefinementKey coverage.
func TestToolRefinementParamNarrowingSuggestionsSixSpotSync(t *testing.T) {
	raw := []ToolParamNarrowingSuggestion{
		{Param: "  pattern ", Priority: 3, Suggested: " needle ", ReasonCode: " entries_over_threshold "},
		{Param: "", Priority: 1, Suggested: "dropped", ReasonCode: "entries_over_threshold"},
		{Param: "path", Priority: 1, Suggested: "internal/tool", ReasonCode: "entries_over_threshold"},
		{Param: "path", Priority: 4, Suggested: "stale-dup", ReasonCode: "entries_over_threshold"},
	}
	rawCopy := append([]ToolParamNarrowingSuggestion(nil), raw...)
	hint := NormalizeToolRefinementHint(ToolRefinementHint{ParamNarrowingSuggestions: raw})
	if len(hint.ParamNarrowingSuggestions) != 2 {
		t.Fatalf("normalize should trim/drop/dedupe to 2 rows: %+v", hint.ParamNarrowingSuggestions)
	}
	if hint.ParamNarrowingSuggestions[0].Param != "path" || hint.ParamNarrowingSuggestions[0].Suggested != "internal/tool" {
		t.Fatalf("dedupe must keep the lower-priority row and sort it first: %+v", hint.ParamNarrowingSuggestions)
	}
	if hint.ParamNarrowingSuggestions[1].Param != "pattern" || hint.ParamNarrowingSuggestions[1].Suggested != "needle" ||
		hint.ParamNarrowingSuggestions[1].ReasonCode != "entries_over_threshold" {
		t.Fatalf("normalize must trim fields: %+v", hint.ParamNarrowingSuggestions[1])
	}
	// Defensive slice copy: mutating the normalized output must not write
	// through to the caller's slice (deep-clone contract of
	// cloneToolRefinementHintPtr).
	hint.ParamNarrowingSuggestions[0].Suggested = "mutated"
	for i := range raw {
		if raw[i] != rawCopy[i] {
			t.Fatalf("normalize must not alias the caller slice: %+v", raw)
		}
	}

	// Empty(): a hint carrying only suggestions is not empty.
	onlySuggestions := ToolRefinementHint{ParamNarrowingSuggestions: []ToolParamNarrowingSuggestion{{Param: "scope", Priority: 1}}}
	if onlySuggestions.Empty() {
		t.Fatal("hint with only ParamNarrowingSuggestions must not be Empty")
	}

	// Merge: dedupe by Param keeping the lower Priority; receiving hint's
	// row wins ties.
	merged := mergeToolRefinementHint(
		ToolRefinementHint{
			ReasonCode: "grep_result_truncated",
			ParamNarrowingSuggestions: []ToolParamNarrowingSuggestion{
				{Param: "scope", Priority: 2, Suggested: "internal/tool", ReasonCode: "entries_over_threshold"},
				{Param: "roles", Priority: 3, Suggested: "function", ReasonCode: "entries_over_threshold"},
			},
		},
		ToolRefinementHint{
			ReasonCode: "source_inventory_candidate_budget_truncated",
			ParamNarrowingSuggestions: []ToolParamNarrowingSuggestion{
				{Param: "scope", Priority: 1, Suggested: "internal/types", ReasonCode: "candidate_budget_truncated"},
				{Param: "roles", Priority: 3, Suggested: "method", ReasonCode: "candidate_budget_truncated"},
			},
		},
	)
	if len(merged.ParamNarrowingSuggestions) != 2 {
		t.Fatalf("merge must dedupe by Param: %+v", merged.ParamNarrowingSuggestions)
	}
	if merged.ParamNarrowingSuggestions[0].Param != "scope" || merged.ParamNarrowingSuggestions[0].Suggested != "internal/types" {
		t.Fatalf("merge must keep the lower-priority scope row: %+v", merged.ParamNarrowingSuggestions)
	}
	if merged.ParamNarrowingSuggestions[1].Param != "roles" || merged.ParamNarrowingSuggestions[1].Suggested != "function" {
		t.Fatalf("merge priority tie must keep the receiving hint's row: %+v", merged.ParamNarrowingSuggestions)
	}

	// Bound: more than the cap collapses to the cap after dedupe.
	var many []ToolParamNarrowingSuggestion
	for i := 0; i < toolHandoffMaxParamNarrowingSuggestions+3; i++ {
		many = append(many, ToolParamNarrowingSuggestion{Param: "p" + strconv.Itoa(i), Priority: i + 1})
	}
	bounded := NormalizeToolRefinementHint(ToolRefinementHint{ParamNarrowingSuggestions: many})
	if len(bounded.ParamNarrowingSuggestions) != toolHandoffMaxParamNarrowingSuggestions {
		t.Fatalf("suggestion count must be bounded to %d: %d", toolHandoffMaxParamNarrowingSuggestions, len(bounded.ParamNarrowingSuggestions))
	}

	// toolRefinementKey: hints differing only in suggestions must not
	// dedup-collide in the carrier key.
	base := ToolRefinementHint{ReasonCode: "grep_result_truncated", ResultTruncated: true}
	withSuggestion := base
	withSuggestion.ParamNarrowingSuggestions = []ToolParamNarrowingSuggestion{{Param: "path", Priority: 1, Suggested: "internal/tool", ReasonCode: "entries_over_threshold"}}
	if toolRefinementKey(base) == toolRefinementKey(withSuggestion) {
		t.Fatal("toolRefinementKey must cover ParamNarrowingSuggestions")
	}
}

func TestNormalizeToolHandoffCarriersPreservesActionableRefinementUnderBudget(t *testing.T) {
	carriers := []ToolHandoffCarrier{{
		Version:    ToolHandoffCarrierVersion,
		ToolName:   "grep",
		ReasonCode: "grep_result_truncated",
		Refinement: &ToolRefinementHint{
			ReasonCode:        "grep_result_truncated",
			ResultTruncated:   true,
			PreferredNextTool: "repo_map",
			PreferredParams: map[string]string{
				"query": "Owner",
				"view":  "task_map",
			},
		},
	}}
	for i := 0; i < toolHandoffMaxCarriers+12; i++ {
		carriers = append(carriers, ToolHandoffCarrier{
			Version:    ToolHandoffCarrierVersion,
			ToolName:   "trace_query",
			ReasonCode: "tool_observation_handoff_" + strconv.Itoa(i),
			ObservationRefs: []ToolObservationRef{{
				ID:       "obs-" + strconv.Itoa(i),
				Producer: "trace_query",
				ClaimKey: "runtime",
			}},
		})
	}

	got := NormalizeToolHandoffCarriers(carriers)
	if len(got) != toolHandoffMaxCarriers {
		t.Fatalf("normalized carrier budget = %d, want %d", len(got), toolHandoffMaxCarriers)
	}
	var refinement *ToolHandoffCarrier
	for i := range got {
		if got[i].Refinement != nil && got[i].Refinement.ReasonCode == "grep_result_truncated" {
			refinement = &got[i]
			break
		}
	}
	if refinement == nil {
		t.Fatalf("actionable refinement was dropped behind later plain observations: %+v", got)
	}
	if refinement.Refinement.PreferredNextTool != "repo_map" ||
		refinement.Refinement.PreferredParams["view"] != "task_map" {
		t.Fatalf("refinement payload changed: %+v", refinement.Refinement)
	}
}

func TestToolHandoffCarrierProjectionRankOrdersActionability(t *testing.T) {
	cases := []struct {
		name    string
		carrier ToolHandoffCarrier
		want    int
	}{
		{
			name: "repair",
			carrier: ToolHandoffCarrier{
				Repair: &ToolRepair{Code: "schema_repair"},
			},
			want: 0,
		},
		{
			name: "accepted evidence",
			carrier: ToolHandoffCarrier{
				AcceptedEvidence: []AcceptedEvidenceRef{{ID: "ev-1"}},
			},
			want: 1,
		},
		{
			name: "refinement",
			carrier: ToolHandoffCarrier{
				Refinement: &ToolRefinementHint{ReasonCode: "grep_result_truncated", ResultTruncated: true},
			},
			want: 2,
		},
		{
			name: "observation",
			carrier: ToolHandoffCarrier{
				ObservationRefs: []ToolObservationRef{{ID: "obs-1"}},
			},
			want: 3,
		},
		{name: "empty", carrier: ToolHandoffCarrier{}, want: 4},
	}
	for _, tc := range cases {
		if got := ToolHandoffCarrierProjectionRank(tc.carrier); got != tc.want {
			t.Fatalf("%s rank = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestSetTurnAArtifactsProjectsAcceptedEvidenceCarrier(t *testing.T) {
	mut := NewMutableState("q")
	mut.SetTurnAArtifacts(TurnAArtifacts{
		ToolResults: []ToolResult{{
			ToolName: "grep",
			Success:  true,
			Summary:  "ok",
		}},
		EvidenceItems: []EvidenceItem{{
			ID:              "ev-1",
			Kind:            EvidenceDirect,
			Scope:           ScopeLine,
			Source:          "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
			LineStart:       7,
			LineEnd:         9,
			Subject:         "EntryComponent",
			OwnerSymbol:     "EntryComponent",
			AnchorSymbol:    "build",
			GroundingStatus: GroundingGrounded,
		}},
	})

	turnA := mut.TurnAArtifacts()
	if turnA == nil {
		t.Fatal("expected TurnAArtifacts")
	}
	var found *ToolHandoffCarrier
	for i := range turnA.HandoffCarriers {
		if turnA.HandoffCarriers[i].ToolName == "emit_evidence" {
			found = &turnA.HandoffCarriers[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected accepted evidence carrier, got %+v", turnA.HandoffCarriers)
	}
	if len(found.AcceptedEvidence) != 1 {
		t.Fatalf("accepted evidence refs = %+v", found.AcceptedEvidence)
	}
	ref := found.AcceptedEvidence[0]
	if ref.SourcePathRole != SourcePathRoleThirdParty {
		t.Fatalf("source path role = %q", ref.SourcePathRole)
	}
	if ref.OwnerSymbol != "EntryComponent" || ref.LineStart != 7 {
		t.Fatalf("accepted evidence ref mismatch: %+v", ref)
	}
}

func TestBoundTurnAToolResultsPreservesCarrierPayload(t *testing.T) {
	in := []ToolResult{
		{
			ToolName: "emit_evidence",
			Success:  false,
			Handoff: &ToolHandoffCarrier{
				Version:    ToolHandoffCarrierVersion,
				ToolName:   "emit_evidence",
				ReasonCode: "accepted_evidence_handoff",
				AcceptedEvidence: []AcceptedEvidenceRef{{
					ID:          "ev-1",
					Source:      "src/a.ts",
					LineStart:   1,
					LineEnd:     1,
					Subject:     "A",
					OwnerSymbol: "A",
				}},
			},
		},
		{ToolName: "read_file", Success: false, Summary: "x"},
		{ToolName: "repo_map", Success: false, Summary: "y"},
	}

	out := BoundTurnAToolResults(in, 1, 512, PreserveSuccessfulToolResultWithPayload)
	if len(out) != 1 {
		t.Fatalf("expected the preserved carrier result to win the count cap, got %d: %+v", len(out), out)
	}
	if out[0].Handoff == nil || len(out[0].Handoff.AcceptedEvidence) != 1 {
		t.Fatalf("carrier result not preserved: %+v", out)
	}
}
