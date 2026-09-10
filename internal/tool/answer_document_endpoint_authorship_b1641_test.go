package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

// r1051: this is the actual Logger.log -> Sink.write call, not the README's
// unrelated format_value path. The existing actor remains model-authored;
// adding a separately named method must not silently retarget it to that actor.
func b1641LoggerFixture(t *testing.T) (*types.AnswerDocumentV2, []types.EvidenceItem) {
	t.Helper()
	source := filepath.Join("..", "..", "eval", "fixtures", "cpp-sink-hierarchy", "src", "logger.cpp")
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	if len(lines) < 38 || !strings.Contains(lines[35], "sink_->write") || !strings.Contains(lines[37], "sink_->flush") {
		t.Fatal("the committed C++ call witness changed")
	}
	write := diagramEvidenceTestCall("Logger.log", "Sink.write")
	write.ID, write.Source, write.LineStart, write.LineEnd = "ev-write", source, 36, 36
	flush := diagramEvidenceTestCall("Logger.log", "Sink.flush")
	flush.ID, flush.Source, flush.LineStart, flush.LineEnd = "ev-flush", source, 38, 38
	base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "Model-authored explanation stays unchanged."},
		{ID: "diagram", Kind: types.BlockDiagram, Title: "Model-authored layout", Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramSequence, Language: "mermaid",
			Body: "sequenceDiagram\n    participant Logger\n    participant Sink\n    Logger->>Sink: flush when requested\n",
		}, EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "Logger", ToNode: "Sink", FromIdentity: flush.Subject,
			ToIdentity: flush.Object, RelationKind: types.DiagramRelCall, VisibleLabel: "flush when requested"}}},
	}}
	return base, []types.EvidenceItem{write, flush}
}

func b1641AdditionBus(t *testing.T, base *types.AnswerDocumentV2, evidence []types.EvidenceItem, selected ...int) *types.BusContext {
	t.Helper()
	var requests []preEmitStandaloneRelationRepairCandidate
	for _, i := range selected {
		ev := evidence[i]
		requests = append(requests, preEmitStandaloneRelationRepairCandidate{relation: types.DiagramRelCall,
			from: ev.Subject, to: ev.Object, evidenceID: ev.ID, source: fmt.Sprintf("%s:%d", ev.Source, ev.LineStart), blockIDs: []string{"diagram"}})
	}
	allowed := diagramRelationRepairAllowedAdditions(base, types.RequestModel{Intent: types.IntentExplain, PredicateAxis: types.AxisCall},
		evidence, nil, []string{"diagram"}, requests, 8)
	if len(allowed) != len(selected) {
		t.Fatalf("real repair producer lost selected relations: %+v", allowed)
	}
	mut := types.NewMutableState("B1641 model-owned endpoint declarations")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
	mut.SetAnswerDiagramRelationRepairLease(types.NewAnswerDiagramRelationRepairLease(base, nil, allowed))
	return &types.BusContext{Mutable: mut, EvidenceItems: evidence}
}

func b1641AddEdit(t *testing.T, bus *types.BusContext, from, to, fromNode, toNode, fromLabel, toLabel string) map[string]any {
	t.Helper()
	for _, candidate := range bus.Mutable.AnswerDiagramRelationRepairLease().AllowedAdditions {
		if candidate.FromIdentity != from || candidate.ToIdentity != to {
			continue
		}
		edit := map[string]any{"action": "add", "addition_ref": candidate.AdditionRef,
			"edge": map[string]any{"from_node": fromNode, "to_node": toNode, "visible_label": "model-selected call"}}
		if fromLabel != "" {
			edit["from_node_visible_label"] = fromLabel
		}
		if toLabel != "" {
			edit["to_node_visible_label"] = toLabel
		}
		return edit
	}
	t.Fatalf("missing exact candidate %s -> %s", from, to)
	return nil
}

func b1641SchemaBranch(t *testing.T, bus *types.BusContext, edit map[string]any) json.RawMessage {
	t.Helper()
	var schema map[string]any
	if err := json.Unmarshal((&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: bus.Mutable}), &schema); err != nil {
		t.Fatal(err)
	}
	// Validate the exact selected branch: toolparam deliberately does not turn
	// every advisory oneOf into a new execution gate.
	branches := schema["properties"].(map[string]any)["diagram_edge_edits"].(map[string]any)["items"].(map[string]any)["oneOf"].([]any)
	for _, rawBranch := range branches {
		branch := rawBranch.(map[string]any)
		props := branch["properties"].(map[string]any)
		for _, field := range []string{"addition_ref", "failure_ref"} {
			ref, ok := props[field].(map[string]any)
			if !ok || ref["enum"].([]any)[0] != edit[field] || props["action"].(map[string]any)["enum"].([]any)[0] != edit["action"] {
				continue
			}
			branchJSON, _ := json.Marshal(branch)
			return branchJSON
		}
	}
	t.Fatalf("no published schema branch for %+v", edit)
	return nil
}

func b1641CheckSchema(t *testing.T, bus *types.BusContext, edit map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(edit)
	if err := toolparam.Validate(raw, b1641SchemaBranch(t, bus, edit)); err != nil {
		t.Fatalf("model edit must satisfy the actual published branch: %v", err)
	}
}

func TestB1641EndpointAuthorshipAndAuthorityControls(t *testing.T) {
	for _, tc := range []struct {
		name, node, label, wantNode, errorText string
		ambiguous, schemaReject, noEvidence    bool
	}{
		{name: "unlabeled technical endpoint reuses actor", node: "Logger.log", wantNode: "Logger"},
		{name: "explicit new node even with same display text", node: "Logger.log", label: "Logger", wantNode: "Logger.log"},
		{name: "explicit existing exact replay", node: "Logger", label: "Logger", wantNode: "Logger"},
		{name: "existing label conflict", node: "Logger", label: "Logger::log", schemaReject: true, errorText: "exactly match the current explicit label"},
		{name: "unlabeled ambiguous alias", node: "Logger.log", ambiguous: true, errorText: "multiple declared sequence participants"},
		{name: "explicit new node does not select ambiguous alias", node: "Logger.log", label: "Logger::log", ambiguous: true, wantNode: "Logger.log"},
		{name: "display label cannot authorize unrelated endpoint", node: "Other.call", label: "Logger::log", errorText: "not a typed carrier"},
		{name: "explicit declaration cannot authorize missing evidence", node: "Logger.log", label: "Logger::log", noEvidence: true, errorText: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, evidence := b1641LoggerFixture(t)
			if tc.ambiguous {
				// Both aliases remain incident on an already-grounded sibling
				// edge, so an orphan-warning phase cannot mask the tested result.
				base.Blocks[1].Diagram.Body += "    participant OtherLogger as Logger\n    OtherLogger->>Sink: another flush\n"
				anchor := base.Blocks[1].EdgeAnchors[0]
				anchor.FromNode, anchor.VisibleLabel = "OtherLogger", "another flush"
				base.Blocks[1].EdgeAnchors = append(base.Blocks[1].EdgeAnchors, anchor)
			}
			bus := b1641AdditionBus(t, base, evidence, 0)
			edit := b1641AddEdit(t, bus, "Logger.log", "Sink.write", tc.node, "Sink", tc.label, "")
			raw, _ := json.Marshal(edit)
			err := toolparam.Validate(raw, b1641SchemaBranch(t, bus, edit))
			if (err != nil) != tc.schemaReject {
				t.Fatalf("actual branch label contract: %v", err)
			}
			before, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
			if tc.noEvidence {
				bus.EvidenceItems = nil
				bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, PredicateAxis: types.AxisCall}}
			}
			result, err := b1641Execute(t, bus, edit)
			if tc.wantNode == "" {
				if err != nil || result.Success || (tc.errorText != "" && !strings.Contains(result.Summary, tc.errorText)) {
					t.Fatalf("expected unchanged permission/ambiguity rejection: err=%v result=%+v", err, result)
				}
				after, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
				if !bytes.Equal(before, after) {
					t.Fatal("rejected edit became an accepted answer")
				}
				return
			}
			if err != nil || !result.Success {
				t.Fatalf("explicit presentation choice failed: err=%v result=%+v", err, result)
			}
			got := bus.Mutable.AnswerDocumentV2().Blocks[1]
			last := got.EdgeAnchors[len(got.EdgeAnchors)-1]
			if last.FromNode != tc.wantNode || last.FromIdentity != "Logger.log" || last.ToIdentity != "Sink.write" || last.VisibleLabel != "model-selected call" {
				t.Fatalf("model choice/hidden authority changed: %+v", last)
			}
		})
	}
}

func TestB1641MultipleMethodsAndBothSidesAreLanguageIndependent(t *testing.T) {
	// These typed identities exercise the presentation consumer, not a new
	// language parser. No extension or prose chooses the declaration behavior.
	for _, extension := range []string{"go", "py", "java", "cpp", "ts", "ets", "cj"} {
		t.Run(extension, func(t *testing.T) {
			base, evidence := b1641LoggerFixture(t)
			for i := range evidence {
				evidence[i].Source = "src/service." + extension
			}
			second := diagramEvidenceTestCall("Logger.close", "Sink.finish")
			second.ID, second.Source = "ev-close", "src/service."+extension
			evidence = append(evidence, second)
			bus := b1641AdditionBus(t, base, evidence, 0, 2)
			firstEdit := b1641AddEdit(t, bus, "Logger.log", "Sink.write", "Logger.log", "Sink.write", "Write a log", "Write to backend")
			secondEdit := b1641AddEdit(t, bus, "Logger.close", "Sink.finish", "Logger.close", "Sink.finish", "Close logger", "Finish backend")
			for _, edit := range []map[string]any{firstEdit, secondEdit} {
				b1641CheckSchema(t, bus, edit)
			}
			result, err := b1641Execute(t, bus, firstEdit, secondEdit)
			if err != nil || !result.Success {
				t.Fatalf("independent method declarations failed: err=%v result=%+v", err, result)
			}
			got := bus.Mutable.AnswerDocumentV2().Blocks[1]
			if len(got.EdgeAnchors) != 3 {
				t.Fatalf("method relations were collapsed: %+v", got.EdgeAnchors)
			}
			for _, edge := range got.EdgeAnchors[1:] {
				if edge.FromNode != edge.FromIdentity || edge.ToNode != edge.ToIdentity || !strings.Contains(got.Diagram.Body, edge.FromNode+"->>"+edge.ToNode+": model-selected call") {
					t.Fatalf("method node merged into actor: %+v\n%s", edge, got.Diagram.Body)
				}
			}
		})
	}
}

func TestB1641ReplaceUsesSameExplicitDeclarationChoice(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprint(explicit), func(t *testing.T) {
			base, evidence := b1641LoggerFixture(t)
			base.Blocks[1].Diagram.Body = strings.ReplaceAll(base.Blocks[1].Diagram.Body, "flush when requested", "old write")
			base.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{{FromNode: "Logger", ToNode: "Sink",
				FromIdentity: "Logger.log", ToIdentity: "Sink.write", RelationKind: types.DiagramRelCall, VisibleLabel: "old write"}}
			mut := types.NewMutableState("B1641 replacement endpoint choice")
			mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
			lease := types.NewAnswerDiagramRelationRepairLease(base, []types.AnswerDiagramRelationRepairFailure{{BlockID: "diagram",
				Issue: diagramParticipantComponentJoinEndpointMappingIssue, FromNode: "Logger", ToNode: "Sink", FromIdentity: "Logger.log", ToIdentity: "Sink.write",
				RelationKind: types.DiagramRelCall, BodyOccurrence: 1}}, nil)
			if lease == nil || len(lease.Failures) != 1 || !lease.Failures[0].AllowsAction("replace") {
				t.Fatalf("missing exact replacement capability: %+v", lease)
			}
			mut.SetAnswerDiagramRelationRepairLease(lease)
			bus := &types.BusContext{Mutable: mut, EvidenceItems: evidence}
			edit := map[string]any{"failure_ref": lease.Failures[0].FailureRef, "action": "replace", "edge": map[string]any{
				"from_node": "Logger.log", "to_node": "Sink.write", "visible_label": "replacement chosen by model"}}
			wantFrom, wantTo := "Logger", "Sink"
			if explicit {
				edit["from_node_visible_label"], edit["to_node_visible_label"] = "Logger::log", "Sink::write"
				wantFrom, wantTo = "Logger.log", "Sink.write"
			}
			b1641CheckSchema(t, bus, edit)
			before, _ := json.Marshal(base)
			result, err := b1641Execute(t, bus, edit)
			if err != nil || !result.Success {
				t.Fatalf("same add/replace declaration choice failed: err=%v result=%+v", err, result)
			}
			got := mut.AnswerDocumentV2().Blocks[1]
			if len(got.EdgeAnchors) != 1 || got.EdgeAnchors[0].FromNode != wantFrom || got.EdgeAnchors[0].ToNode != wantTo ||
				got.EdgeAnchors[0].FromIdentity != "Logger.log" || got.EdgeAnchors[0].ToIdentity != "Sink.write" ||
				!strings.Contains(got.Diagram.Body, wantFrom+"->>"+wantTo+": replacement chosen by model") || strings.Contains(got.Diagram.Body, "old write") {
				t.Fatalf("replacement lost visible/typed ownership: %+v", got)
			}
			after, _ := json.Marshal(base)
			if !bytes.Equal(before, after) || !reflect.DeepEqual(mut.AnswerDocumentV2().Blocks[0], base.Blocks[0]) ||
				!strings.Contains(got.Diagram.Body, "participant Logger\n") || !strings.Contains(got.Diagram.Body, "participant Sink\n") {
				t.Fatal("unselected immutable content changed")
			}
		})
	}
}

func TestB1641ExactDeclarationCaseMatchesSchema(t *testing.T) {
	base, evidence := b1641LoggerFixture(t)
	base.Blocks[1].Diagram.Body = strings.ReplaceAll(base.Blocks[1].Diagram.Body, "participant Logger\n", "participant logger.log as Logger\n")
	base.Blocks[1].Diagram.Body = strings.ReplaceAll(base.Blocks[1].Diagram.Body, "Logger->>", "logger.log->>")
	base.Blocks[1].EdgeAnchors[0].FromNode = "logger.log"
	bus := b1641AdditionBus(t, base, evidence, 0)
	edit := b1641AddEdit(t, bus, "Logger.log", "Sink.write", "Logger.log", "Sink", "Logger::log", "")
	b1641CheckSchema(t, bus, edit)
	result, err := b1641Execute(t, bus, edit)
	if err != nil || !result.Success {
		t.Fatalf("case-distinct new declaration borrowed old label ownership: err=%v result=%+v", err, result)
	}
	body := bus.Mutable.AnswerDocumentV2().Blocks[1].Diagram.Body
	for _, want := range []string{"participant logger.log as Logger", `participant Logger.log as "Logger::log"`, "logger.log->>Sink: flush when requested", "Logger.log->>Sink: model-selected call"} {
		if !strings.Contains(body, want) {
			t.Errorf("case-sensitive declaration lost %q:\n%s", want, body)
		}
	}
}

func TestB1641EndpointTeachingIsBoundedAndShared(t *testing.T) {
	base, evidence := b1641LoggerFixture(t)
	bus := b1641AdditionBus(t, base, evidence, 0)
	edit := b1641AddEdit(t, bus, "Logger.log", "Sink.write", "Logger.log", "Sink", "Logger::log", "")
	branch := string(b1641SchemaBranch(t, bus, edit))
	if strings.Count(branch, diagramEndpointLabelAuthorshipTeaching) != 1 {
		t.Fatal("each selected branch must state the complete rule once, not once per endpoint")
	}
	if !strings.Contains((&EmitAnswerDocumentPatch{}).Description(), diagramEndpointLabelAuthorshipTeaching) ||
		strings.Count(string((&EmitAnswerDocumentPatch{}).Parameters()), diagramEndpointLabelAuthorshipTeaching) != 1 {
		t.Fatal("broad and exact schemas diverged from the shared presentation rule")
	}
}

func b1641Execute(t *testing.T, bus *types.BusContext, edits ...map[string]any) (types.ToolResult, error) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "diagram_edge_edits": edits})
	if err != nil {
		t.Fatal(err)
	}
	return (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
}

func TestB1641LoggerExplicitMethodSurvivesPublishedSchemaAndExecution(t *testing.T) {
	base, evidence := b1641LoggerFixture(t)
	before, _ := json.Marshal(base)
	bus := b1641AdditionBus(t, base, evidence, 0)
	edit := b1641AddEdit(t, bus, "Logger.log", "Sink.write", "Logger.log", "Sink", "Logger::log", "")
	b1641CheckSchema(t, bus, edit)
	result, err := b1641Execute(t, bus, edit)
	if err != nil || !result.Success {
		t.Fatalf("schema-legal explicit method was retargeted/rejected: err=%v result=%+v", err, result)
	}
	got := bus.Mutable.AnswerDocumentV2()
	body := got.Blocks[1].Diagram.Body
	for _, want := range []string{"participant Logger\n", "Logger->>Sink: flush when requested", "Logger.log->>Sink: model-selected call", "Logger::log"} {
		if !strings.Contains(body, want) {
			t.Errorf("model-owned declaration or edge %q lost:\n%s", want, body)
		}
	}
	anchor := got.Blocks[1].EdgeAnchors[1]
	if anchor.FromNode != "Logger.log" || anchor.ToNode != "Sink" || anchor.FromIdentity != "Logger.log" || anchor.ToIdentity != "Sink.write" {
		t.Fatalf("endpoint choice or typed identities changed: %+v", anchor)
	}
	if mismatches := DiagramCallEdgeEvidenceMismatches(got, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence); len(mismatches) != 0 {
		t.Fatalf("typed evidence qualification changed: %+v", mismatches)
	}
	rendered, diagnostics := render.TryRenderMermaidBlocks("```mermaid\n" + body + "```\n")
	if len(diagnostics) > 1 || (len(diagnostics) == 1 && diagnostics[0].Outcome != render.OutcomeRendered) ||
		!strings.Contains(rendered, "Logger::log") || !strings.Contains(rendered, "```text\n") || strings.Contains(rendered, "```mermaid") {
		t.Fatalf("existing terminal shim did not preserve/render the method: %+v\n%s", diagnostics, rendered)
	}
	if got.Blocks[1].Diagram.Body != body {
		t.Fatal("terminal-only syntax adaptation changed model Mermaid source")
	}
	after, _ := json.Marshal(base)
	if !bytes.Equal(before, after) || !reflect.DeepEqual(got.Blocks[0], base.Blocks[0]) || !reflect.DeepEqual(got.Blocks[1].EdgeAnchors[0], base.Blocks[1].EdgeAnchors[0]) {
		t.Fatal("unmentioned model content or immutable input changed")
	}
}
