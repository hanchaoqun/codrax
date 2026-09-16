package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the public initial-instruction and Observe surfaces together: a
// source relation is not a dynamic execution counter in either authoring lane.
func TestB1712PublicSourceRelationTeachingDoesNotCountRuntimeOccurrences(t *testing.T) {
	for _, required := range []bool{true, false} {
		lane := "optional"
		if required {
			lane = "required"
		}
		for _, toolName := range []string{"emit_answer_document", "emit_answer_document_patch"} {
			t.Run(lane+"/"+toolName, func(t *testing.T) {
				ctx := ctxWithAnswerPatchBaseAndSequenceCallCapsule()
				ctx.AnalysisIR.RequestModel.PredicateAxis = types.AxisFlow
				ctx.AnalysisIR.AnswerContract.Diagram.Required = required
				ctx.EvidenceItems = []types.EvidenceItem{{
					ID: "source-call", Kind: types.EvidenceRelationship,
					Source: "src/dispatcher.ts", LineStart: 7,
					AnchorKind: types.AnchorCall, Subject: "Dispatcher.execute", Object: "Worker.run",
					GroundingStatus: types.GroundingGrounded,
				}}
				ctx.Mutable.SetPendingAnswerDocumentPatchBase(&types.AnswerDocumentV2{
					DocumentModel: "v2",
					Blocks: []types.AnswerBlock{
						{ID: "summary", Kind: types.BlockSummary, Text: "model-authored summary"},
						{ID: "diagram", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{
							Kind: types.DiagramSequence, Language: "mermaid",
							Body: "sequenceDiagram\n participant d as Dispatcher.execute\n participant w as Worker.run\n loop selected attempts\n d->>w: perform work\n end",
						}},
					},
				})
				before, err := json.Marshal(ctx.Mutable.PendingAnswerDocumentPatchBase())
				if err != nil {
					t.Fatal(err)
				}
				evaluator := &answerDocumentEvaluator{diagramRequired: required, mu: ctx.Mutable}
				initial := evaluator.BuildInitialInstruction(ctx, nil)
				signal := evaluator.Observe(ctx, LoopObservation{
					Phase: PhaseMidLoop,
					LastToolResult: &types.ToolResult{
						ToolName: toolName, Success: false,
						Repair: &types.ToolRepair{Code: "answer_doc_pre_emit_contract", Metadata: map[string]string{
							"violation_kinds":                       string(types.ViolDiagramCallEdgeUnproven),
							types.ToolRepairMetaOffendingBlockKinds: string(types.BlockDiagram),
						}},
					},
				})
				if !signal.HintRequested || signal.StopRequested {
					t.Fatalf("public Observe did not reach local diagram repair: %+v", signal)
				}
				for surface, prompt := range map[string]string{"initial": initial, "repair": signal.Hint} {
					for _, want := range []string{
						"Static call-site evidence proves a relation, not its runtime execution count",
						"control flow already read",
						"Do not infer execution counts, branch outcomes, or runtime argument values from evidence-row counts",
						"Repetition does not authorize a new edge or path",
						`"from_identity":"Dispatcher.execute","to_identity":"Worker.run","relation_kind":"call"`,
					} {
						if !strings.Contains(prompt, want) {
							t.Errorf("%s lacks shared source-relation boundary %q", surface, want)
						}
					}
					for _, forbidden := range []string{
						"use a recipe at most once",
						"use each exact relation recipe below at most once",
						"distinct grounded call-site row proves another occurrence",
						"repeated occurrences beyond the currently accepted typed relation evidence",
					} {
						if strings.Contains(prompt, forbidden) {
							t.Errorf("%s retains static/dynamic count contradiction %q", surface, forbidden)
						}
					}
				}
				for _, want := range []string{"emit_answer_document_patch", "unchanged_block_ids", "native", "Visible node labels, edge/message labels, and Notes remain model-authored"} {
					if !strings.Contains(signal.Hint, want) {
						t.Errorf("repair lost ownership or JSON teaching %q", want)
					}
				}
				if required {
					if !strings.Contains(signal.Hint, "Keep the required diagram") || strings.Contains(signal.Hint, "remove_block_ids") {
						t.Error("required source diagram must remain required")
					}
				} else if !strings.Contains(signal.Hint, "The diagram remains optional") || !strings.Contains(signal.Hint, "keep separate components disconnected") {
					t.Error("optional diagram choice or disconnected boundary changed")
				}
				after, err := json.Marshal(ctx.Mutable.PendingAnswerDocumentPatchBase())
				if err != nil {
					t.Fatal(err)
				}
				if string(before) != string(after) {
					t.Fatal("teaching rewrote the model-owned answer")
				}
			})
		}
	}
}

func TestB1712SourceOccurrenceTeachingLeavesExplicitWindowTraceIndependent(t *testing.T) {
	ctx := ctxWithAnswerPatchBaseAndSequenceCallCapsule()
	start, end := 5.0, 5.01
	ctx.AnalysisIR.RequestModel = types.RequestModel{
		Intent: types.IntentRootCause, Scenario: types.ScenarioRootCause,
		PredicateAxis: types.AxisCall,
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{
			RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
			TimeStart:      &start, TimeEnd: &end,
			SourceQuote: "5.000 through 5.010 seconds", Confidence: 0.99,
		},
	}
	if family := types.ResolveQuestionFamily(ctx.AnalysisIR.RequestModel); family != types.QFRootCauseTrace {
		t.Fatalf("Trace negative-control premise resolved to %s", family)
	}
	before, err := json.Marshal(ctx.AnalysisIR.RequestModel)
	if err != nil {
		t.Fatal(err)
	}
	prompt := (&answerDocumentEvaluator{mu: ctx.Mutable}).BuildInitialInstruction(ctx, nil)
	if strings.Contains(prompt, "Static call-site evidence proves a relation") || strings.Contains(prompt, "## Current-Source Mechanism Relation Authority") {
		t.Fatal("source relation-count teaching leaked into runtime Trace authority")
	}
	after, err := json.Marshal(ctx.AnalysisIR.RequestModel)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("source teaching changed the explicit Trace window")
	}
}
