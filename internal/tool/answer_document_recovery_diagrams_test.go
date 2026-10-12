package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC227RecoveryPreservesCurrentValidatedGraphBesideRejectedGraph(t *testing.T) {
	ctx, _, rows := runtimeNestingFixture(t)
	doc := runtimeNestingDoc(rows[0], true)
	raw, _ := json.Marshal(doc)
	res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || !res.Success {
		t.Fatalf("normal accepted path: %v %+v", err, res)
	}
	good := doc.Blocks[1]
	bad := runtimeNestingDoc(rows[1], true).Blocks[1]
	bad.ID = "bad"
	bad.EdgeAnchors[0].ToIdentity += "_foreign"
	doc.Blocks = append(doc.Blocks, bad)
	raw, _ = json.Marshal(doc)
	res, err = (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || res.Success || res.Repair == nil {
		t.Fatalf("mixed actual reject: %v %+v", err, res)
	}
	recovered := ctx.Mutable.LastRejectedAnswerDocumentV2()
	_, isolated := IsolateRecoveredAnswerDiagrams(ctx, recovered, nil, "en")
	if isolated != 1 || recovered.Blocks[1].Diagram == nil || !reflect.DeepEqual(recovered.Blocks[1].Diagram, good.Diagram) || recovered.Blocks[3].Diagram != nil {
		t.Fatalf("isolation must be graph-local, not all-or-nothing: isolated=%d %+v", isolated, recovered.Blocks)
	}
	if ctx.Mutable.LastRejectedAnswerDocumentV2().Blocks[3].Diagram == nil {
		t.Fatal("raw failed draft mutated")
	}
	// Same aliases and visible names cannot retain authority after the actual
	// current producer disappears. No remembered success or source string signs it.
	ctx.Mutable = types.NewMutableState("new generation, no measurements")
	_, isolated = IsolateRecoveredAnswerDiagrams(ctx, recovered, nil, "en")
	if isolated != 1 || recovered.Blocks[1].Diagram != nil {
		t.Fatal("stale native graph survived")
	}
}

func TestHMC227RecoveryTableFailureDoesNotEraseProvedGraph(t *testing.T) {
	ctx, _, rows := runtimeNestingFixture(t)
	doc := runtimeNestingDoc(rows[0], true)
	doc.Blocks[2].RuntimeMeasurement = &types.AnswerRuntimeMeasurementReceipt{ObservationID: "absent", View: "absent"}
	raw, _ := json.Marshal(doc)
	res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || res.Success {
		t.Fatalf("table must fail: %v %+v", err, res)
	}
	before := *doc.Blocks[1].Diagram
	_, isolated := IsolateRecoveredAnswerDiagrams(ctx, doc, nil, "en")
	if isolated != 0 || doc.Blocks[1].Diagram == nil || *doc.Blocks[1].Diagram != before {
		t.Fatal("independent table failure erased verified relationship")
	}
}

func TestHMC227RecoveryUncheckedAndPrivateSystemBoundary(t *testing.T) {
	ctx, _, rows := runtimeNestingFixture(t)
	for _, name := range []string{"missing_context", "missing_owners", "wrong_relation", "partial_edge_ownership"} {
		t.Run(name, func(t *testing.T) {
			doc := runtimeNestingDoc(rows[0], true)
			use := ctx
			switch name {
			case "missing_context":
				use = nil
			case "missing_owners":
				doc.Blocks[1].EdgeAnchors = nil
			case "wrong_relation":
				doc.Blocks[1].EdgeAnchors[0].RelationKind = types.DiagramRelCall
			case "partial_edge_ownership":
				doc.Blocks[1].Diagram.Body += "\n X --> Y"
			}
			_, isolated := IsolateRecoveredAnswerDiagrams(use, doc, nil, "en")
			if isolated != 1 || doc.Blocks[1].Diagram != nil {
				t.Fatalf("unchecked graph retained: %s", name)
			}
		})
	}
	doc := runtimeNestingDoc(rows[0], false)
	doc.Blocks[1].SystemGeneratedKind = types.AnswerSystemGeneratedRuntimeTrace
	_, isolated := IsolateRecoveredAnswerDiagrams(nil, doc, nil, "en")
	if isolated != 0 || doc.Blocks[1].Diagram == nil {
		t.Fatal("private causal projection removed")
	}
	raw, _ := json.Marshal(doc)
	var forged types.AnswerDocumentV2
	if err := json.Unmarshal(raw, &forged); err != nil {
		t.Fatal(err)
	}
	_, isolated = IsolateRecoveredAnswerDiagrams(nil, &forged, nil, "en")
	if isolated != 1 {
		t.Fatal("serialized system-looking diagram gained authority")
	}
}

func TestHMC227RecoveryFencesAndAttachmentsShareIsolation(t *testing.T) {
	for _, fence := range []string{"```mermaid\nflowchart TD\n A --> B\n```", "````mermaid\nflowchart TD\n A --> B\n````", "~~~mermaid\nflowchart TD\n A --> B\n~~~", "```text\nflowchart TD\n A --> B\n```", "```sequenceDiagram\n A->>B: call\n```", "```mermaid\nflowchart TD\n A --> B"} {
		got, n := IsolateUnvalidatedDiagramFences("before\n"+fence+"\nafter", "en")
		if n != 1 || strings.Contains(got, "A --> B") || strings.Contains(got, "A->>B") || !strings.Contains(got, "before") {
			t.Fatalf("fence escaped: %q => %q (%d)", fence, got, n)
		}
	}
	code := "```go\n// flowchart is an ordinary comment\n```\nregular flowchart prose"
	if got, n := IsolateUnvalidatedDiagramFences(code, "en"); n != 0 || got != code {
		t.Fatal("ordinary code/prose changed")
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "section", Kind: types.BlockSection, Text: "keep\n```mermaid\nflowchart TD\n A --> B\n```"}}}
	attachments := []types.AnswerDisplayAttachment{
		{Kind: types.AnswerDisplayAttachmentDiagram, Body: "flowchart TD\n A --> B", Source: "emit_answer_document.retry_state"},
		{Kind: types.AnswerDisplayAttachmentDiagram, Body: "flowchart TD\n A --> B", Source: "orchestrator.system_crosscheck_appendix"},
		{Kind: types.AnswerDisplayAttachmentMarkdown, Body: "attachment text\n```mermaid\nflowchart TD\n A --> B\n```"},
	}
	kept, n := IsolateRecoveredAnswerDiagrams(nil, doc, attachments, "en")
	if n != 4 || len(kept) != 1 || strings.Contains(kept[0].Body, "A --> B") || !strings.Contains(kept[0].Body, "attachment text") || strings.Contains(doc.Blocks[0].Text, "A --> B") {
		t.Fatalf("diagram resurrected through another carrier: %d %+v %+v", n, kept, doc)
	}
}

func TestHMC227RecoveryUsesExistingStructuralReplyAuthority(t *testing.T) {
	for _, paired := range []bool{true, false} {
		doc := diagramEvidenceTestDoc("A", "B")
		doc.Blocks[0].Diagram.Body += "  B-->>A: result\n"
		if !paired {
			doc.Blocks[0].Diagram.Body = "sequenceDiagram\n  participant A as Alpha.Run\n  participant B as Beta.Run\n  B-->>A: result\n"
			doc.Blocks[0].EdgeAnchors = nil
		}
		ctx := &types.BusContext{Mutable: types.NewMutableState("reply"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain, PredicateAxis: types.AxisFlow}}, EvidenceItems: []types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")}}
		_, n := IsolateRecoveredAnswerDiagrams(ctx, doc, nil, "en")
		if (n == 0) != paired {
			t.Fatalf("paired=%v isolated=%d doc=%+v", paired, n, doc)
		}
	}
}
