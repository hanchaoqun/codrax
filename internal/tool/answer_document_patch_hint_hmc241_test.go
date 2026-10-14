package tool

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC241PatchStrictDecodeHelperAttachesCurrentSchema(t *testing.T) {
	now := time.Unix(1, 0)
	decodeErr := errors.New(`json: unknown field "unsupported"`)
	raw := []byte(`{"unsupported":true}`)
	base, _ := failStrictDecode("emit_answer_document_patch", now, decodeErr, nil, raw)
	for _, field := range []string{"first_current_branch", "second_current_branch"} {
		t.Run(field, func(t *testing.T) {
			schema := json.RawMessage(fmt.Sprintf(`{"type":"object","properties":{%q:{"type":"string"}}}`, field))
			result, err := failAnswerDocumentPatchStrictDecode("emit_answer_document_patch", now, decodeErr, nil, raw, schema)
			if err != nil || result.Success || result.Repair == nil || result.Repair.Code != base.Repair.Code || result.Summary != base.Summary || !result.Timestamp.Equal(now) {
				t.Fatalf("lost shared strict-decode failure semantics: err=%v result=%+v", err, result)
			}
			surface, ok := types.ToolJSONSurfaceDescriptorFromToolRepair(result.ToolName, result.Repair)
			if !ok || !reflect.DeepEqual(surface.AcceptedFieldPaths, []string{"$." + field}) || surface.ReasonCode != "tool_param_unknown_field" {
				t.Fatalf("repair metadata must use only this current schema, not the static patch schema: %+v", surface)
			}
		})
	}
}

func TestHMC241PatchDecodeHintUsesActualLeaseSchema(t *testing.T) {
	for _, field := range []string{"from_node", "relation_kind"} {
		t.Run(field, func(t *testing.T) {
			body := "sequenceDiagram\n participant A\n participant B\n participant C\n C-->>A: complete\n"
			bus, _ := sequencePlacementPublicFixture(t, body)
			addition := bus.Mutable.AnswerDiagramRelationRepairLease().AllowedAdditions[0].AdditionRef
			position := sequencePlacementPublicRef(t, bus, "C-->>A: complete")
			before, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
			raw := json.RawMessage(fmt.Sprintf(`{"diagram_edge_edits":[{"action":"add","addition_ref":%q,"placement_ref":%q,%q:"misplaced","edge":{"from_node":"A","to_node":"B","visible_label":"begin"}}]}`, addition, position, field))
			result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
			if err != nil || result.Success {
				t.Fatalf("invalid current branch must remain rejected: %v %+v", err, result)
			}
			if field == "from_node" && !strings.Contains(result.Summary, "diagram_edge_edits[i].edge.from_node") {
				t.Fatalf("lost exact current edge destination: %s", result.Summary)
			}
			for _, forbidden := range []string{"blocks[i].edge_anchors", "claim_uses", "diagram_edge_edits[i].match.relation_kind", "diagram_edge_edits[i].edge.relation_kind"} {
				if strings.Contains(result.Summary, forbidden) {
					t.Fatalf("hint borrowed a destination absent from this live branch: %s", result.Summary)
				}
			}
			surface, ok := types.ToolJSONSurfaceDescriptorFromToolRepair(result.ToolName, result.Repair)
			if !ok {
				t.Fatal("missing schema repair descriptor")
			}
			for _, path := range surface.AcceptedFieldPaths {
				if path == "$.diagram_edge_edits[].edge.relation_kind" || path == "$.diagram_edge_edits[].match.relation_kind" {
					t.Fatalf("metadata reintroduced a branch-hidden field: %+v", surface)
				}
			}
			if field == "relation_kind" && (len(surface.FailingFieldPaths) != 1 || strings.Contains(surface.FailingFieldPaths[0], "replace_blocks")) {
				t.Fatalf("wrong-field coordinate was attributed to another operation: %+v", surface)
			}
			after, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
			if string(before) != string(after) {
				t.Fatal("decode failure mutated the answer")
			}
		})
	}
}

func TestHMC241AdditionAndPlacementRefsRemainSeparate(t *testing.T) {
	body := "sequenceDiagram\n participant A\n participant B\n participant C\n C-->>A: complete\n"
	bus, _ := sequencePlacementPublicFixture(t, body)
	addition := bus.Mutable.AnswerDiagramRelationRepairLease().AllowedAdditions[0].AdditionRef
	position := sequencePlacementPublicRef(t, bus, "C-->>A: complete")
	if addition == position {
		t.Fatal("relation identity must not double as a display position")
	}
	params := func(ref string) json.RawMessage {
		return json.RawMessage(fmt.Sprintf(`{"diagram_edge_edits":[{"action":"add","addition_ref":%q,"placement_ref":%q,"edge":{"from_node":"A","to_node":"B","visible_label":"begin"}}]}`, addition, ref))
	}
	bad, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params(addition))
	if err != nil || bad.Success || bus.Mutable.AnswerDocumentV2().Blocks[1].Diagram.Body != body {
		t.Fatalf("wrong position changed the answer: %v %+v", err, bad)
	}
	good, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params(position))
	if err != nil || !good.Success {
		t.Fatalf("actual published position must execute after rejection: %v %+v", err, good)
	}
	got := bus.Mutable.AnswerDocumentV2().Blocks[1].Diagram.Body
	if strings.Index(got, "A->>B: begin") > strings.Index(got, "C-->>A: complete") {
		t.Fatalf("repair appended after the selected position: %s", got)
	}
}

func TestHMC241LegacyPatchHintKeepsPublishedPaths(t *testing.T) {
	bus, _ := sequencePlacementPublicFixture(t, "sequenceDiagram\n participant A\n participant B\n")
	bus.Mutable.SetAnswerDiagramRelationRepairLease(nil)
	result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{"diagram_edge_edits":[{"block_id":"diag","action":"add","from_node":"A","edge":{"to_node":"B","relation_kind":"precedence","from_identity":"Analyzer","to_identity":"Explorer","visible_label":"begin"}}]}`))
	if err != nil || result.Success || !strings.Contains(result.Summary, "diagram_edge_edits[i].edge.from_node") {
		t.Fatalf("legacy schema's real field path must stay discoverable: %v %+v", err, result)
	}
}
