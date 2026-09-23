package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func sequencePlacementPublicFixture(t *testing.T, body string) (*types.BusContext, *types.AnswerDocumentV2) {
	t.Helper()
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = body
	prev.Blocks[1].EdgeAnchors = nil
	mut := types.NewMutableState("sequence insertion positions")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	mut.SetAnswerDiagramRelationRepairLease(types.NewAnswerDiagramRelationRepairLease(prev, nil,
		[]types.AnswerDiagramRelationRepairCandidate{{
			BlockID: "diag", RelationKind: types.DiagramRelPrecedence,
			FromIdentity: "Analyzer", ToIdentity: "Explorer", FromNodeIDs: []string{"A"}, ToNodeIDs: []string{"B"},
			Source: "typed-test-stage-proof",
		}}))
	if mut.AnswerDiagramRelationRepairLease() == nil {
		t.Fatal("fixture requires one live addition")
	}
	return &types.BusContext{Mutable: mut}, prev
}

// Read the actual dispatch schema, not an internal locator helper. A position
// is a presentation choice; this fixture separately owns its typed relation.
func sequencePlacementPublicRef(t *testing.T, bus *types.BusContext, before string) string {
	t.Helper()
	raw := (&EmitAnswerDocumentPatch{}).parametersForContext(nil, bus.Mutable, bus)
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	var found string
	var walk func(any)
	walk = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			if v["before"] == before {
				if ref, ok := v["placement_ref"].(string); ok {
					found = ref
				}
			}
			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		case string:
			// Roster is schema description data, not an extra argument field.
			if at := strings.Index(v, "sequence_placement_choices="); at >= 0 {
				var rows []map[string]any
				if json.Unmarshal([]byte(v[at+len("sequence_placement_choices="):]), &rows) == nil {
					for _, row := range rows {
						walk(row)
					}
				}
			}
		}
	}
	walk(schema)
	if found == "" {
		t.Fatalf("actual patch schema did not publish an insertion position before %q", before)
	}
	return found
}

func TestSequencePlacementPublic_InsertBeforeReturn(t *testing.T) {
	body := "sequenceDiagram\n    participant A\n    participant B\n    participant C\n    C-->>A: 调度返回\n"
	bus, prev := sequencePlacementPublicFixture(t, body)
	ref := sequencePlacementPublicRef(t, bus, "C-->>A: 调度返回")
	var schema map[string]any
	if err := json.Unmarshal((&EmitAnswerDocumentPatch{}).parametersForContext(nil, bus.Mutable, bus), &schema); err != nil {
		t.Fatal(err)
	}
	properties := schema["properties"].(map[string]any)
	branches := properties["diagram_edge_edits"].(map[string]any)["items"].(map[string]any)["oneOf"].([]any)
	branch := branches[0].(map[string]any)
	required, _ := json.Marshal(branch["required"])
	if !strings.Contains(string(required), `"placement_ref"`) || branch["properties"].(map[string]any)["placement_ref"] == nil {
		t.Fatal("actual add branch did not require published position")
	}
	addition := bus.Mutable.AnswerDiagramRelationRepairLease().AllowedAdditions[0].AdditionRef
	params := json.RawMessage(fmt.Sprintf(`{"unchanged_block_ids":["summary"],"diagram_edge_edits":[{"addition_ref":%q,"action":"add","placement_ref":%q,"edge":{"from_node":"A","to_node":"B","visible_label":"开始探索"}}]}`, addition, ref))
	result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
	if err != nil || !result.Success {
		t.Fatalf("public patch failed: %v / %+v", err, result)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	got := doc.Blocks[1].Diagram.Body
	if strings.Index(got, "A->>B: 开始探索") > strings.Index(got, "C-->>A: 调度返回") {
		t.Fatalf("stage appended after return:\n%s", got)
	}
	if !reflect.DeepEqual(doc.Blocks[0], prev.Blocks[0]) || prev.Blocks[1].Diagram.Body != body {
		t.Fatal("unrelated block or input mutated")
	}
	if strings.Contains(got, "placement") {
		t.Fatalf("private placement bookkeeping leaked:\n%s", got)
	}
}

func TestSequencePlacementPublic_BranchesAndEmptyBody(t *testing.T) {
	for _, tc := range []struct{ name, statements, before string }{
		{"alt_first", "    alt 成功\n    C-->>A: first\n    else 失败\n    C-->>A: second\n    end\n", "C-->>A: first"},
		{"alt_second", "    alt 成功\n    C-->>A: first\n    else 失败\n    C-->>A: second\n    end\n", "C-->>A: second"},
		{"nested_parallel", "    par worker\n    loop attempt\n    C-->>A: first\n    end\n    and another\n    C-->>A: second\n    end\n", "C-->>A: first"},
		{"empty_branch", "    alt 成功\n    else 失败\n    C-->>A: second\n    end\n", "else 失败"},
		{"empty_diagram", "", ""},
		{"after_final_return", "    C-->>A: done", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := "sequenceDiagram\n    participant A\n    participant B\n    participant C\n" + tc.statements
			bus, _ := sequencePlacementPublicFixture(t, body)
			ref := sequencePlacementPublicRef(t, bus, tc.before)
			addition := bus.Mutable.AnswerDiagramRelationRepairLease().AllowedAdditions[0].AdditionRef
			params := json.RawMessage(fmt.Sprintf(`{"diagram_edge_edits":[{"addition_ref":%q,"action":"add","placement_ref":%q,"edge":{"from_node":"A","to_node":"B","visible_label":"chosen"}}]}`, addition, ref))
			result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
			if err != nil || !result.Success {
				t.Fatalf("public branch patch failed: %v / %+v", err, result)
			}
			got := bus.Mutable.AnswerDocumentV2().Blocks[1].Diagram.Body
			want := body
			if tc.before != "" {
				at := strings.Index(body, "    "+tc.before)
				want = body[:at] + "    A->>B: chosen\n" + body[at:]
			} else if strings.HasSuffix(body, "\n") {
				want += "    A->>B: chosen\n"
			} else {
				want += "\n    A->>B: chosen"
			}
			if got != want {
				t.Fatalf("placement changed unrelated order/branch bytes:\ngot=%q\nwant=%q", got, want)
			}
		})
	}
}

func TestSequencePlacementPublic_RejectsMissingStaleAndCrossBlockAtomically(t *testing.T) {
	for _, kind := range []string{"missing", "unknown", "body_reordered", "other_block"} {
		t.Run(kind, func(t *testing.T) {
			body := "sequenceDiagram\n    participant A\n    participant B\n    participant C\n    C-->>A: first\n    C-->>A: second\n"
			bus, prev := sequencePlacementPublicFixture(t, body)
			ref := sequencePlacementPublicRef(t, bus, "C-->>A: first")
			if kind == "missing" {
				ref = ""
			}
			if kind == "unknown" {
				ref = "sp1-not-current"
			}
			if kind == "body_reordered" {
				lease := bus.Mutable.AnswerDiagramRelationRepairLease()
				changed := atomicPatchTestDocument()
				changed.Blocks[1] = prev.Blocks[1]
				diagram := *prev.Blocks[1].Diagram
				diagram.Body = strings.Replace(body, "C-->>A: first\n    C-->>A: second", "C-->>A: second\n    C-->>A: first", 1)
				changed.Blocks[1].Diagram = &diagram
				bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, changed)
				bus.Mutable.SetAnswerDiagramRelationRepairLease(lease)
			}
			if kind == "other_block" {
				other, otherDoc := sequencePlacementPublicFixture(t, body)
				candidate := other.Mutable.AnswerDiagramRelationRepairLease().AllowedAdditions[0]
				otherDoc.Blocks[1].ID = "other"
				candidate.BlockID = "other"
				other.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, otherDoc)
				other.Mutable.SetAnswerDiagramRelationRepairLease(types.NewAnswerDiagramRelationRepairLease(otherDoc, nil, []types.AnswerDiagramRelationRepairCandidate{candidate}))
				ref = sequencePlacementPublicRef(t, other, "C-->>A: first")
			}
			before, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
			leaseBefore, _ := json.Marshal(bus.Mutable.AnswerDiagramRelationRepairLease())
			addition := bus.Mutable.AnswerDiagramRelationRepairLease().AllowedAdditions[0].AdditionRef
			params := json.RawMessage(fmt.Sprintf(`{"replace_blocks":[{"id":"summary","kind":"summary","text":"must roll back"}],"diagram_edge_edits":[{"addition_ref":%q,"action":"add","placement_ref":%q,"edge":{"from_node":"A","to_node":"B","visible_label":"new"}}]}`, addition, ref))
			result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
			if err != nil || result.Success {
				t.Fatalf("invalid position accepted: %v / %+v", err, result)
			}
			after, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
			leaseAfter, _ := json.Marshal(bus.Mutable.AnswerDiagramRelationRepairLease())
			if string(before) != string(after) || string(leaseBefore) != string(leaseAfter) || bus.Mutable.PendingAnswerDocumentPatchBase() != nil {
				t.Fatal("rejected transaction changed accepted answer, lease, or retry base")
			}
			if result.Repair == nil {
				t.Fatal("position rejection must retain actionable schema repair")
			}
		})
	}
}

func TestSequencePlacementPublic_SameGapPreservesModelOrder(t *testing.T) {
	body := "sequenceDiagram\n    participant A\n    participant B\n    participant C\n    C-->>A: return\n"
	bus, prev := sequencePlacementPublicFixture(t, body)
	var candidates []types.AnswerDiagramRelationRepairCandidate
	for _, pair := range [][2]string{{"A", "B"}, {"B", "C"}, {"A", "C"}} {
		candidates = append(candidates, types.AnswerDiagramRelationRepairCandidate{BlockID: "diag", RelationKind: types.DiagramRelPrecedence, FromIdentity: pair[0], ToIdentity: pair[1], FromNodeIDs: []string{pair[0]}, ToNodeIDs: []string{pair[1]}, Source: "typed-test-proof"})
	}
	bus.Mutable.SetAnswerDiagramRelationRepairLease(types.NewAnswerDiagramRelationRepairLease(prev, nil, candidates))
	ref := sequencePlacementPublicRef(t, bus, "C-->>A: return")
	var edits []string
	for i, candidate := range bus.Mutable.AnswerDiagramRelationRepairLease().AllowedAdditions {
		edits = append(edits, fmt.Sprintf(`{"addition_ref":%q,"action":"add","placement_ref":%q,"edge":{"from_node":%q,"to_node":%q,"visible_label":%q}}`, candidate.AdditionRef, ref, candidate.FromIdentity, candidate.ToIdentity, fmt.Sprintf("step%d", i)))
	}
	result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{"diagram_edge_edits":[`+strings.Join(edits, ",")+`]}`))
	if err != nil || !result.Success {
		t.Fatalf("ordered batch failed: %v / %+v", err, result)
	}
	got := bus.Mutable.AnswerDocumentV2().Blocks[1].Diagram.Body
	if !(strings.Index(got, "step0") < strings.Index(got, "step1") && strings.Index(got, "step1") < strings.Index(got, "step2") && strings.Index(got, "step2") < strings.Index(got, "C-->>A: return")) {
		t.Fatalf("same-gap order changed:\n%s", got)
	}
}

func TestSequencePlacementPublic_StaleAnchorRestorationIsPositioned(t *testing.T) {
	for _, liveRef := range []bool{false, true} {
		t.Run(fmt.Sprintf("live_ref_%v", liveRef), func(t *testing.T) {
			prev := atomicPatchTestDocument()
			prev.Blocks[1].Diagram.Body = strings.Replace(prev.Blocks[1].Diagram.Body, "    A->>B: old label\n", "", 1)
			mut := types.NewMutableState("restore exact missing message")
			mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
			lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{{BlockID: "diag", Issue: diagramCallEdgeIssueAnchorWithoutBodyEdge, FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer", RelationKind: types.DiagramRelPrecedence}}, nil)
			mut.SetAnswerDiagramRelationRepairLease(lease)
			bus := &types.BusContext{Mutable: mut}
			ref := sequencePlacementPublicRef(t, bus, "B->>C: keep label")
			edit := emitAnswerDiagramEdgeEdit{BlockID: "diag", Action: "replace", PlacementRef: ref, Match: &prev.Blocks[1].EdgeAnchors[0], Edge: &types.DiagramEdgeAnchor{FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer", RelationKind: types.DiagramRelPrecedence, VisibleLabel: "restored"}}
			if liveRef {
				edit.BlockID = ""
				edit.Match = nil
				edit.FailureRef = lease.Failures[0].FailureRef
			}
			// Live refs exercise Execute; the historical no-body coordinate lane
			// is tested directly because the current local schema omits it.
			var got *types.AnswerDocumentV2
			if liveRef {
				raw, _ := json.Marshal(map[string]any{"diagram_edge_edits": []emitAnswerDiagramEdgeEdit{edit}})
				result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
				if err != nil || !result.Success {
					t.Fatalf("public stale repair failed: %v / %+v", err, result)
				}
				got = mut.AnswerDocumentV2()
			} else {
				patch := &types.AnswerDocumentV2Patch{}
				if err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{edit}, nil, lease); err != nil {
					t.Fatal(err)
				}
				var err error
				got, err = types.ApplyAnswerDocumentV2Patch(prev, patch)
				if err != nil {
					t.Fatal(err)
				}
			}
			body := got.Blocks[1].Diagram.Body
			if strings.Index(body, "A->>B: restored") > strings.Index(body, "B->>C: keep label") || !strings.Contains(body, "A->>B: restored") {
				t.Fatalf("missing-body restoration appended at end:\n%s", body)
			}
		})
	}
}

func TestSequencePlacementPublic_GapSurvivesSelectedStatementEdit(t *testing.T) {
	for _, action := range []string{"remove", "replace"} {
		for _, newFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s_new_first_%v", action, newFirst), func(t *testing.T) {
				prev := atomicPatchTestDocument()
				prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    participant A\n    participant B\n    participant C\n    A->>B: old label\n    C-->>A: return\n"
				prev.Blocks[1].EdgeAnchors = prev.Blocks[1].EdgeAnchors[:1]
				mut := types.NewMutableState("source gap remains after neighbor edit")
				mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
				issue := "semantic_relation_edge_unproven"
				if action == "replace" {
					issue = types.AnswerDiagramRelationRepairIssueParticipantEndpointMapping
				}
				lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{{BlockID: "diag", Issue: issue, FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer", RelationKind: types.DiagramRelPrecedence}}, []types.AnswerDiagramRelationRepairCandidate{{BlockID: "diag", RelationKind: types.DiagramRelPrecedence, FromIdentity: "Explorer", ToIdentity: "Extractor", FromNodeIDs: []string{"B"}, ToNodeIDs: []string{"C"}, Source: "typed-stage-proof"}})
				mut.SetAnswerDiagramRelationRepairLease(lease)
				bus := &types.BusContext{Mutable: mut}
				ref := sequencePlacementPublicRef(t, bus, "A->>B: old label")
				addition := fmt.Sprintf(`{"addition_ref":%q,"action":"add","placement_ref":%q,"edge":{"from_node":"B","to_node":"C","visible_label":"inserted"}}`, lease.AllowedAdditions[0].AdditionRef, ref)
				existing := fmt.Sprintf(`{"failure_ref":%q,"action":%q}`, lease.Failures[0].FailureRef, action)
				if action == "replace" {
					existing = fmt.Sprintf(`{"failure_ref":%q,"action":"replace","edge":{"from_node":"A","to_node":"B","visible_label":"replacement"}}`, lease.Failures[0].FailureRef)
				}
				edits := existing + "," + addition
				if newFirst {
					edits = addition + "," + existing
				}
				result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{"diagram_edge_edits":[`+edits+`]}`))
				if err != nil || !result.Success {
					t.Fatalf("combined position patch failed: %v / %+v", err, result)
				}
				body := mut.AnswerDocumentV2().Blocks[1].Diagram.Body
				if !strings.Contains(body, "B->>C: inserted") || strings.Index(body, "B->>C: inserted") > strings.Index(body, "C-->>A: return") || strings.Contains(body, "old label") || strings.Contains(body, "private-placement") {
					t.Fatalf("original source gap lost:\n%s", body)
				}
				if action == "replace" && strings.Index(body, "B->>C: inserted") > strings.Index(body, "A->>B: replacement") {
					t.Fatalf("gap followed replacement instead of original position:\n%s", body)
				}
			})
		}
	}
}

func TestSequencePlacementPublic_RepeatedMessagesAndSourceBytes(t *testing.T) {
	body := "%%{init: {\"theme\":\"base\"}}%%\r\nsequenceDiagram\r\n    participant A\r\n    participant B\r\n    participant X@{\r\n        \"type\": \"queue\"\r\n    }\r\n    participant C\r\n    %% codrax-private-placement:keep\r\n    C-->>A: repeat\r\n    C-->>A: repeat\r\n"
	bus, _ := sequencePlacementPublicFixture(t, body)
	ref := sequencePlacementPublicRef(t, bus, "C-->>A: repeat") // the last exact repeated occurrence
	positions := sequenceInsertionPositions(bus.Mutable.AnswerDocumentV2().Blocks[1])
	for _, position := range positions {
		if strings.Contains(position.Before, "\"type\"") || position.Before == "}" {
			t.Fatal("published a gap inside a declaration")
		}
	}
	addition := bus.Mutable.AnswerDiagramRelationRepairLease().AllowedAdditions[0].AdditionRef
	params := fmt.Sprintf(`{"diagram_edge_edits":[{"addition_ref":%q,"action":"add","placement_ref":%q,"edge":{"from_node":"A","to_node":"B","visible_label":"inserted"}}]}`, addition, ref)
	result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(params))
	if err != nil || !result.Success {
		t.Fatalf("repeated-message insertion failed: %v / %+v", err, result)
	}
	got := bus.Mutable.AnswerDocumentV2().Blocks[1].Diagram.Body
	at := strings.LastIndex(body, "    C-->>A: repeat")
	want := body[:at] + "    A->>B: inserted\n" + body[at:]
	if got != want {
		t.Fatalf("unrelated source bytes or occurrence changed:\ngot=%q\nwant=%q", got, want)
	}
}
