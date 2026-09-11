package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The relation lease comes from an actual read -> grounded evidence -> full
// emit rejection. Only dispatcher installation is explicit in this tool test;
// no relation, required-kind failure, or repair candidate is fabricated.
func b1663RequiredSummaryLease(t *testing.T, missingPath ...bool) (*types.BusContext, *types.AnswerDocumentV2, *types.AnswerDiagramRelationRepairLease) {
	t.Helper()
	bus, _ := b1649ActualCall(t)
	definition, err := (&EmitEvidence{}).Execute(bus, json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"direct","subject":"p.Caller","source":"calls.go","line_start":3,"anchor_kind":"definition","anchor_symbol":"Caller"}]}`))
	if err != nil || !definition.Success {
		t.Fatalf("actual declaration evidence: %v %+v", err, definition)
	}
	var definitionID string
	for _, ev := range bus.Mutable.EmittedEvidence() {
		if ev.Source == "calls.go" && ev.LineStart == 3 && ev.GroundingStatus == types.GroundingGrounded && types.ClaimFormOf(ev) == types.ClaimDefinitionFact {
			definitionID = ev.ID
			bus.EvidenceItems = append(bus.EvidenceItems, ev)
		}
	}
	if definitionID == "" {
		t.Fatal("grounded declaration missing")
	}
	doc := b1649Diagram("p.Caller", "Callee")
	doc.Blocks = doc.Blocks[1:]
	wantDeficit := 1
	if len(missingPath) > 0 && missingPath[0] {
		wantDeficit++
	} else {
		doc.Blocks = append(doc.Blocks, b1663Path(definitionID))
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := (&EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || initial.Success || initial.Repair == nil || !strings.Contains(initial.Summary, "kind=summary") {
		t.Fatalf("public missing-summary/relation premise: err=%v result=%+v", err, initial)
	}
	var delta types.AnswerDiagramRelationRepairDelta
	if err := json.Unmarshal([]byte(initial.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta); err != nil {
		t.Fatalf("public relation delta: %v %+v", err, initial.Repair)
	}
	lease := types.NewAnswerDiagramRelationRepairLease(doc, delta.Failures, delta.AllowedAdditions)
	if lease == nil || len(lease.Failures) != 1 || len(lease.AllowedAdditions) != 1 || !lease.Failures[0].AllowsAction("attach") {
		t.Fatalf("public source must permit the exact attach: %+v", lease)
	}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "previous", Kind: types.BlockSummary, Text: "Previously accepted model answer."}},
	})
	bus.Mutable.SetPendingAnswerDocumentPatchBase(doc)
	bus.Mutable.SetAnswerDiagramRelationRepairLease(lease)
	view := types.BuildAnswerSemanticViewForBusContext(bus)
	if deficit := requiredAnswerBlockDeficit(doc.Blocks, view); deficit != wantDeficit {
		t.Fatalf("expected %d missing required carriers, deficit=%d view=%+v", wantDeficit, deficit, view)
	}
	return bus, doc, lease
}

func b1663Path(definitionID string) types.AnswerBlock {
	return types.AnswerBlock{
		ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
		FacetIDs:  []string{string(types.FacetCurrentCodePath)},
		ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimDefinitionFact}},
		Items:     []types.AnswerBlockItem{{ID: "hop", Label: "p.Caller", Text: "The caller is declared here.", EvidenceIDs: []string{definitionID}}},
	}
}

func b1663Summary(id string) map[string]any {
	return map[string]any{"id": id, "kind": "summary", "text": "The caller invokes the callee.", "surface_role": "principal"}
}

func TestB1663PublicMixedAdditionNamesActualFailure(t *testing.T) {
	for _, badKind := range []string{"existing_diagram", "optional_caveat", "duplicate_summary"} {
		for _, badFirst := range []bool{false, true} {
			if badKind == "duplicate_summary" && badFirst {
				continue // Either order is legal until the second summary closes no deficit.
			}
			t.Run(fmt.Sprintf("%s/badFirst=%t", badKind, badFirst), func(t *testing.T) {
				bus, doc, lease := b1663RequiredSummaryLease(t)
				var bad any
				badID := "extra"
				switch badKind {
				case "existing_diagram":
					bad, badID = doc.Blocks[0], doc.Blocks[0].ID
				case "optional_caveat":
					bad = map[string]any{"id": badID, "kind": "caveat", "text": "Optional model context."}
				case "duplicate_summary":
					bad = b1663Summary(badID)
				}
				additions := []any{b1663Summary("needed-summary"), bad}
				if badFirst {
					additions[0], additions[1] = additions[1], additions[0]
				}
				params, _ := json.Marshal(map[string]any{"add_blocks": additions, "unchanged_block_ids": []string{"diagram", "path"}})
				beforeBase, _ := json.Marshal(bus.Mutable.PendingAnswerDocumentPatchBase())
				beforeAccepted, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
				beforeLease, _ := json.Marshal(lease)
				schemaBefore := (&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: bus.Mutable, AnalysisIR: bus.AnalysisIR, EvidenceItems: bus.EvidenceItems})
				result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
				if err != nil || result.Success || result.Repair == nil || result.Repair.Code != types.ToolRepairCodeAnswerDocRelationRepairScope || result.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome] != types.AnswerDocumentPatchOutcomeNotStaged {
					t.Fatalf("mixed invalid patch must remain atomically rejected: err=%v result=%+v", err, result)
				}
				afterBase, _ := json.Marshal(bus.Mutable.PendingAnswerDocumentPatchBase())
				afterAccepted, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
				afterLease, _ := json.Marshal(bus.Mutable.AnswerDiagramRelationRepairLease())
				schemaAfter := (&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: bus.Mutable, AnalysisIR: bus.AnalysisIR, EvidenceItems: bus.EvidenceItems})
				if string(beforeBase) != string(afterBase) || string(beforeAccepted) != string(afterAccepted) || string(beforeLease) != string(afterLease) || string(schemaBefore) != string(schemaAfter) {
					t.Fatal("rejected patch changed base, accepted model document, lease, or schema")
				}
				if !strings.Contains(result.Summary, "block="+fmt.Sprintf("%q", badID)+"; whole-block operation=whole_add_not_authorized") || strings.Contains(result.Summary, `block="needed-summary"`) {
					t.Fatalf("diagnostic must identify the actual non-reducing addition, not a preceding legal summary: bad=%s result=%s", badID, result.Summary)
				}
				if !reflect.DeepEqual(result.Repair.Fields, []string{fmt.Sprintf("blocks[%q].kind:whole_add_not_authorized", badID)}) {
					t.Fatalf("typed repair coordinate disagrees with the actual failed addition: %+v", result.Repair.Fields)
				}
			})
		}
	}
}

func TestB1663PublicMultipleRequiredKindsRemainLegal(t *testing.T) {
	for _, pathFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("pathFirst=%t", pathFirst), func(t *testing.T) {
			bus, doc, lease := b1663RequiredSummaryLease(t, true)
			var definitionID string
			for _, ev := range bus.EvidenceItems {
				if types.ClaimFormOf(ev) == types.ClaimDefinitionFact {
					definitionID = ev.ID
				}
			}
			additions := []any{b1663Summary("needed-summary"), b1663Path(definitionID)}
			if pathFirst {
				additions[0], additions[1] = additions[1], additions[0]
			}
			params, _ := json.Marshal(map[string]any{
				"add_blocks": additions,
				"diagram_edge_edits": []any{map[string]any{
					"action": "attach", "failure_ref": lease.Failures[0].FailureRef, "addition_ref": lease.AllowedAdditions[0].AdditionRef,
					"edge": map[string]string{"from_node": "A", "to_node": "B", "visible_label": "invoke callee"},
				}},
			})
			before, _ := json.Marshal(doc)
			result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
			if err != nil || !result.Success {
				t.Fatalf("multiple independently missing required kinds must remain legal: %v %+v", err, result)
			}
			got := bus.Mutable.AnswerDocumentV2()
			if got == nil || len(got.Blocks) < 3 || !reflect.DeepEqual(got.Blocks[0].Diagram, doc.Blocks[0].Diagram) {
				t.Fatalf("model diagram changed: %+v", got)
			}
			wantIDs := []string{"diagram", "needed-summary", "path"}
			if pathFirst {
				wantIDs[1], wantIDs[2] = wantIDs[2], wantIDs[1]
			}
			for i, id := range wantIDs {
				if got.Blocks[i].ID != id {
					t.Fatalf("model-chosen addition order changed: %+v", got.Blocks)
				}
			}
			summaryIndex := 1
			if pathFirst {
				summaryIndex = 2
			}
			if got.Blocks[summaryIndex].Text != "The caller invokes the callee." {
				t.Fatal("model summary changed")
			}
			after, _ := json.Marshal(doc)
			if string(before) != string(after) {
				t.Fatal("accepted multi-kind patch changed its immutable input")
			}
		})
	}
}

func TestB1663RequiredAdditionAuthorizationBoundaries(t *testing.T) {
	base := &types.AnswerDocumentV2{DocumentModel: "v2"}
	summaryView := &types.AnswerSemanticView{RequiredBlocks: []types.BlockRequirement{{Kind: types.BlockSummary, MinCount: 1, Required: true}}}
	multiView := &types.AnswerSemanticView{RequiredBlocks: []types.BlockRequirement{
		{Kind: types.BlockSummary, MinCount: 1, Required: true},
		{Kind: types.BlockOrderedList, AlternativeKinds: []types.AnswerBlockKind{types.BlockTable}, MinCount: 1, Required: true, FacetIDs: []string{"member_set"}},
	}}
	summary := emitAnswerBlockV2{ID: "summary", Kind: "summary", Text: "model text"}
	list := emitAnswerBlockV2{ID: "list", Kind: "ordered_list", FacetIDs: []string{"member_set"}}
	claimFacet := emitAnswerBlockV2{ID: "table", Kind: "table", ClaimUses: []types.RenderedClaimUse{{FacetID: "member_set"}}}
	wrongFacet := emitAnswerBlockV2{ID: "other", Kind: "ordered_list", FacetIDs: []string{"current_code_path"}}
	for _, tc := range []struct {
		name  string
		base  *types.AnswerDocumentV2
		view  *types.AnswerSemanticView
		add   []emitAnswerBlockV2
		ok    bool
		index int
	}{
		{"nil base", nil, summaryView, []emitAnswerBlockV2{summary}, false, 0},
		{"nil view", base, nil, []emitAnswerBlockV2{summary}, false, 0},
		{"nil additions", base, summaryView, nil, false, 0},
		{"empty additions", base, summaryView, []emitAnswerBlockV2{}, false, 0},
		{"no requirement", base, &types.AnswerSemanticView{}, []emitAnswerBlockV2{summary}, false, 0},
		{"no deficit", &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{Kind: types.BlockSummary}}}, summaryView, []emitAnswerBlockV2{summary}, false, 0},
		{"one required", base, summaryView, []emitAnswerBlockV2{summary}, true, -1},
		{"second summary", base, summaryView, []emitAnswerBlockV2{summary, summary}, false, 1},
		{"multiple kinds", base, multiView, []emitAnswerBlockV2{summary, list}, true, -1},
		{"multiple reversed", base, multiView, []emitAnswerBlockV2{list, summary}, true, -1},
		{"alternative claim facet", base, multiView, []emitAnswerBlockV2{summary, claimFacet}, true, -1},
		{"wrong facet later", base, multiView, []emitAnswerBlockV2{summary, wrongFacet}, false, 1},
		{"wrong facet first", base, multiView, []emitAnswerBlockV2{wrongFacet, summary}, false, 0},
		{"unknown kind", base, summaryView, []emitAnswerBlockV2{{ID: "unknown", Kind: "not_a_kind"}}, false, 0},
		// This checker owns required-kind deficits only; duplicate IDs remain
		// the unchanged downstream structural gate's responsibility.
		{"same ID different kinds", base, multiView, []emitAnswerBlockV2{summary, {ID: "summary", Kind: "ordered_list", FacetIDs: []string{"member_set"}}}, true, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, _ := json.Marshal([]any{tc.base, tc.view, tc.add})
			ok, index := resolveRequiredAnswerBlockAdditionsAuthorization(tc.base, tc.view, tc.add)
			if ok != tc.ok || index != tc.index || requiredAnswerBlockAdditionsAuthorized(tc.base, tc.view, tc.add) != tc.ok {
				t.Fatalf("authorization/coordinate/wrapper changed: got %v/%d want %v/%d", ok, index, tc.ok, tc.index)
			}
			after, _ := json.Marshal([]any{tc.base, tc.view, tc.add})
			if string(before) != string(after) {
				t.Fatal("authorization changed its inputs")
			}
		})
	}
}

func TestB1663PublicRequiredSummaryAndExactAttachRemainLegal(t *testing.T) {
	bus, doc, lease := b1663RequiredSummaryLease(t)
	params, _ := json.Marshal(map[string]any{
		"add_blocks":          []any{b1663Summary("needed-summary")},
		"unchanged_block_ids": []string{"path"},
		"diagram_edge_edits": []any{map[string]any{
			"action": "attach", "failure_ref": lease.Failures[0].FailureRef, "addition_ref": lease.AllowedAdditions[0].AdditionRef,
			"edge": map[string]string{"from_node": "A", "to_node": "B", "visible_label": "invoke callee"},
		}},
	})
	before, _ := json.Marshal(doc)
	result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
	if err != nil || !result.Success {
		t.Fatalf("existing legal summary + source-backed attach channel must pass: err=%v result=%+v", err, result)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks) < 3 || got.Blocks[0].Diagram == nil || !reflect.DeepEqual(got.Blocks[0].Diagram, doc.Blocks[0].Diagram) || got.Blocks[2].ID != "needed-summary" || got.Blocks[2].Text != "The caller invokes the callee." {
		t.Fatalf("accepted patch changed model content: %+v", got)
	}
	after, _ := json.Marshal(doc)
	if string(before) != string(after) {
		t.Fatal("accepted patch mutated its immutable input")
	}
}
