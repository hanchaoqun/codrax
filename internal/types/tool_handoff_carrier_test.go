package types

import "testing"

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
	if len(out) != 2 {
		t.Fatalf("expected newest plus preserved carrier result, got %d: %+v", len(out), out)
	}
	if out[0].Handoff == nil || len(out[0].Handoff.AcceptedEvidence) != 1 {
		t.Fatalf("carrier result not preserved: %+v", out)
	}
}
