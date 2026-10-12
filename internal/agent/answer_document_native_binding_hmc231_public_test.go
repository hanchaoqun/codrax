package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The live 3891 rejection used S/W/M aliases with a native addition_ref; 4173
// then rejected reusing those declarations. Keep this on the public tool path.
func TestNativeBindingHMC231PublicReuseExistingNotes(t *testing.T) {
	ctx, rows := sequenceOrderHMC230Fixture(t)
	doc := sequenceOrderHMC230Doc(rows, false)
	doc.Blocks[1].Diagram.Body = "sequenceDiagram\n participant S as Storage\n participant W as Worker\n participant M as Main\n Note over W: model-owned state\n S->>W: unsupported relation\n"
	doc.Blocks[1].EdgeAnchors = nil
	bus := types.ToolBusContext(ctx, types.AgentFinalizer)
	raw, _ := json.Marshal(doc)
	res, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || res.Success {
		t.Fatalf("need rejected base: %v %+v", err, res)
	}
	installAnswerDocDiagramRelationRepairLease(ctx, ctx.Mutable, &res, false)
	lease := ctx.Mutable.AnswerDiagramRelationRepairLease()
	if lease == nil {
		t.Fatalf("need native repair lease: %+v", res)
	}
	edits := []map[string]any{}
	for _, f := range lease.Failures {
		if f.TargetCarrier == types.AnswerDiagramRelationRepairCarrierVisibleBodyEdge {
			edits = append(edits, map[string]any{"failure_ref": f.FailureRef, "action": "remove"})
		}
	}
	aliases := map[string]string{rows[0].FromNode: "S", rows[0].ToNode: "W", rows[1].ToNode: "M"}
	var schema any
	if err := json.Unmarshal((&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx), &schema); err != nil {
		t.Fatal(err)
	}
	placement := traceRelationRepairPlacement(schema)
	for _, row := range rows {
		for _, c := range lease.AllowedAdditions {
			if c.FromIdentity != row.FromIdentity || c.ToIdentity != row.ToIdentity {
				continue
			}
			edits = append(edits, map[string]any{"addition_ref": c.AdditionRef, "action": "add", "placement_ref": placement, "from_native_participant": row.FromNode, "to_native_participant": row.ToNode, "edge": map[string]string{"from_node": aliases[row.FromNode], "to_node": aliases[row.ToNode], "visible_label": "observed wakeup"}})
		}
	}
	raw, _ = json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "diagram_edge_edits": edits})
	if err := toolparam.Validate(raw, (&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx)); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"name_only", "wrong_native_side", "old_lease"} {
		var bad map[string]any
		_ = json.Unmarshal(raw, &bad)
		for _, v := range bad["diagram_edge_edits"].([]any) {
			e := v.(map[string]any)
			if e["action"] != "add" {
				continue
			}
			switch mode {
			case "name_only":
				delete(e, "from_native_participant")
				delete(e, "to_native_participant")
			case "wrong_native_side":
				e["from_native_participant"] = rows[0].ToNode
			case "old_lease":
				e["addition_ref"] = "ra1-old-generation"
			}
			break
		}
		badRaw, _ := json.Marshal(bad)
		r, err := (&tool.EmitAnswerDocumentPatch{}).Execute(bus, badRaw)
		if err != nil || r.Success || ctx.Mutable.PendingAnswerDocumentPatchBase() != nil {
			t.Fatalf("%s unexpectedly authorized/staged: %v %+v", mode, err, r)
		}
	}
	res, err = (&tool.EmitAnswerDocumentPatch{}).Execute(bus, raw)
	if err != nil || !res.Success {
		t.Fatalf("exact native alias binding must reuse existing lanes and note owner: %v %+v", err, res)
	}
	got := ctx.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks[1].EdgeAnchors) != 2 || strings.Count(got.Blocks[1].Diagram.Body, "participant ") != 3 || !strings.Contains(got.Blocks[1].Diagram.Body, "Note over W: model-owned state") {
		t.Fatalf("lanes/notes drifted: %+v", got)
	}
}

func TestNativeBindingHMC231PublicSchemaChoicesAndOmission(t *testing.T) {
	ctx := hmosBusinessIOFinalizerContext(t)
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{Kind: types.DiagramSequence, Required: true}
	ctx.AnalysisIR.AnswerContract.Diagram = &types.DiagramContract{Required: true, RequiredKind: types.DiagramSequence}
	full := string((&tool.EmitAnswerDocument{}).ParametersFor(ctx))
	patch := string((&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx))
	for _, schema := range []string{full, patch} {
		for _, s := range []string{"runtime_notes_v1", "LoadDocumentIndex", "native participant rt_thread_", "1.044500000", "body_line"} {
			if !strings.Contains(schema, s) {
				t.Fatalf("schema hides native choice %q", s)
			}
		}
	}
	ctx.Mutable = types.NewMutableState("no native source")
	if strings.Contains(string((&tool.EmitAnswerDocument{}).ParametersFor(ctx)), "runtime_notes_v1") {
		t.Fatal("native note schema survived missing private producer")
	}
}

func TestNativeBindingHMC231PublicBusinessNoteOrder(t *testing.T) {
	ctx, rows := sequenceOrderHMC230Fixture(t)
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	choices := view.RuntimeMeasurementContract.NativeBusinessDiagramAnnotations()
	if len(choices) == 0 {
		t.Fatal("public query supplied no native business annotation choices")
	}
	var selected types.NativeBusinessDiagramAnnotation
	for _, choice := range choices {
		if choice.Name == "LoadDocumentIndex" {
			selected = choice
		}
	}
	if selected.Selector == "" {
		t.Fatalf("worker business absent: %+v", choices)
	}
	doc := sequenceOrderHMC230Doc(rows, false)
	d := doc.Blocks[1].Diagram
	d.Body += " Note over " + rows[0].ToNode + ": old model note\n"
	d.RuntimeNotesV1 = []types.RuntimeDiagramNoteSelection{{Selector: selected.Selector, Point: "end", NodeID: rows[0].ToNode, BodyLine: len(strings.Split(strings.TrimSuffix(d.Body, "\n"), "\n"))}}
	raw, _ := json.Marshal(doc)
	bus := types.ToolBusContext(ctx, types.AgentFinalizer)
	res, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || res.Success || res.Repair == nil || !strings.Contains(res.Repair.Metadata[types.ToolRepairMetaDiagramRelationFailureIssues], "native_sequence_event_order_conflict") {
		t.Fatalf("precisely bound 1.0445 note below 1.045 wake must reject: %v %+v", err, res)
	}
	installAnswerDocDiagramRelationRepairLease(ctx, ctx.Mutable, &res, false)
	lines := strings.Split(strings.TrimSuffix(d.Body, "\n"), "\n")
	lines[5], lines[6] = lines[6], lines[5]
	d.Body = strings.Join(lines, "\n")
	d.RuntimeNotesV1[0].BodyLine = 6
	raw, _ = json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "replace_blocks": []types.AnswerBlock{doc.Blocks[1]}})
	res, err = (&tool.EmitAnswerDocumentPatch{}).Execute(bus, raw)
	if err != nil || !res.Success {
		t.Fatalf("precise note reorder repair failed: %v %+v", err, res)
	}
	got := ctx.Mutable.AnswerDocumentV2().Blocks[1]
	if !strings.Contains(got.Diagram.Body, "LoadDocumentIndex end @ 1.044500000s") || strings.Contains(got.Diagram.Body, "old model note") || len(got.EdgeAnchors) != 2 || strings.Count(got.Diagram.Body, "participant ") != 3 {
		t.Fatalf("native note did not stay on the exact three-lane graph: %+v", got)
	}
}

func TestNativeBindingHMC231PublicNotesBoundaries(t *testing.T) {
	for _, mode := range []string{"alias", "alt", "par", "loop", "unknown_note", "wrong_owner", "name_only", "stale_selector", "edge_line", "duplicate_line", "different_window"} {
		t.Run(mode, func(t *testing.T) {
			ctx, rows := sequenceOrderHMC230Fixture(t)
			view := types.BuildAnswerSemanticViewForAgentContext(ctx)
			var choice types.NativeBusinessDiagramAnnotation
			for _, c := range view.RuntimeMeasurementContract.NativeBusinessDiagramAnnotations() {
				if c.Name == "LoadDocumentIndex" {
					choice = c
				}
			}
			if choice.Selector == "" {
				t.Fatal("no native business span")
			}
			if mode == "different_window" {
				ctx.Mutable = types.NewMutableState("new window")
				raw, _ := json.Marshal(map[string]any{"path": choice.Record.SourceRef.Path, "view": "wakeup_chain", "pid": 100, "time_start": 1.0, "time_end": 1.052})
				q, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
				if err != nil || !q.Success {
					t.Fatalf("new query: %v %+v", err, q)
				}
				ctx.Mutable.AppendDispatchToolResult(q)
				ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{q}})
				rows = tool.RuntimeDiagramRelations(answerDocObservationLedger(ctx), &ctx.AnalysisIR.RequestModel)
			}
			doc := sequenceOrderHMC230Doc(rows, false)
			b := &doc.Blocks[1]
			d := b.Diagram
			node := rows[0].ToNode
			if mode == "alias" {
				d.Body = strings.ReplaceAll(d.Body, node, "W")
				for i := range b.EdgeAnchors {
					if b.EdgeAnchors[i].FromNode == node {
						b.EdgeAnchors[i].FromNode = "W"
					}
					if b.EdgeAnchors[i].ToNode == node {
						b.EdgeAnchors[i].ToNode = "W"
					}
				}
				node = "W"
			}
			lines := strings.Split(strings.TrimSuffix(d.Body, "\n"), "\n")
			note := "Note over " + node + ": deliberately wrong original timestamp 999; do not scan"
			// Put a proven business end between the two proven wakes.
			d.Body = strings.Join(lines[:5], "\n") + "\n" + note + "\n" + lines[5]
			d.RuntimeNotesV1 = []types.RuntimeDiagramNoteSelection{{Selector: choice.Selector, Point: "end", NodeID: node, BodyLine: 6}}
			good := true
			switch mode {
			case "alt", "par", "loop":
				d.Body = strings.Join(lines, "\n") + "\n" + mode + " independent region\n" + note + "\nend"
				d.RuntimeNotesV1[0].BodyLine = 8
			case "unknown_note":
				d.Body = strings.Join(lines, "\n") + "\n" + note
				d.RuntimeNotesV1 = nil
			case "wrong_owner":
				d.RuntimeNotesV1[0].NodeID = rows[0].FromNode
				good = false
			case "name_only":
				d.Body += "\nparticipant W as document-worker-200"
				d.RuntimeNotesV1[0].NodeID = "W"
				good = false
			case "stale_selector":
				d.RuntimeNotesV1[0].Selector += "-old"
				good = false
			case "edge_line":
				d.RuntimeNotesV1[0].BodyLine = 5
				good = false
			case "duplicate_line":
				d.RuntimeNotesV1 = append(d.RuntimeNotesV1, d.RuntimeNotesV1[0])
				good = false
			case "different_window":
				good = false
			}
			raw, _ := json.Marshal(doc)
			res, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
			if err != nil || res.Success != good {
				t.Fatalf("want success=%t: %v %+v", good, err, res)
			}
			if good && mode != "unknown_note" {
				got := ctx.Mutable.AnswerDocumentV2().Blocks[1]
				if !strings.Contains(got.Diagram.Body, "LoadDocumentIndex end @ 1.044500000s") || len(got.EdgeAnchors) != 2 {
					t.Fatalf("bound note or wakes lost: %+v", got)
				}
			}
		})
	}
}
