package tool

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeDiagramMissingEvidenceTeachingPublicEmit(t *testing.T) {
	ctx, _, rows := runtimeWakeFixture(t)
	ctx.Mutable = types.NewMutableState("show the observed timeline")
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_sleep_summary/events.systrace")
	result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "thread_timeline", "pid": 41, "time_start": 10.001, "time_end": 10.012})
	ctx.Mutable.AppendDispatchToolResult(result)
	if len(runtimeDiagramRelationsForContext(ctx)) != 0 {
		t.Fatal("timeline alone unexpectedly issued relation credentials")
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	initial := RenderRuntimeDiagramRelationRecipes(ledger, &ctx.AnalysisIR.RequestModel)
	for _, anchored := range []bool{false, true} {
		doc := runtimeNestingDoc(rows[0], anchored)
		doc.Blocks[1].Diagram.Kind = types.DiagramSequence
		doc.Blocks[1].Diagram.Body = "sequenceDiagram\n participant A as loader-2\n participant B as target-41\n A->>B: 唤醒\n Note over B: 已观察休眠区间\n"
		if anchored {
			doc.Blocks[1].EdgeAnchors[0].FromNode = "A"
			doc.Blocks[1].EdgeAnchors[0].ToNode = "B"
		}
		raw, _ := json.Marshal(doc)
		res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
		if err != nil || res.Success {
			t.Fatalf("timeline/foreign event credential must not authorize arrow: %v %+v", err, res)
		}
		for name, teaching := range map[string]string{"initial": initial, "public rejection": res.Summary} {
			for _, want := range []string{"does not turn runtime events into source calls", "missing current relation credentials", "requested half-open window", "only answer/patch tools", "state intervals as notes", "Source call still requires"} {
				if !strings.Contains(teaching, want) {
					t.Errorf("%s missing %q: %s", name, want, teaching)
				}
			}
			if strings.Contains(teaching, "edge_anchor={") || strings.Contains(teaching, rows[0].FromIdentity) && name == "initial" {
				t.Errorf("%s minted a runtime recipe from measured states", name)
			}
		}
	}
}

func TestRuntimeDiagramRepairTeachingKeepsSourceAndRuntimeAuthority(t *testing.T) {
	ctx, _, rows := runtimeWakeFixture(t)
	for _, mode := range []string{"missing_anchor", "wrong_window", "source_call"} {
		t.Run(mode, func(t *testing.T) {
			doc := runtimeNestingDoc(rows[0], mode != "missing_anchor")
			if mode == "source_call" {
				doc.Blocks[1].EdgeAnchors[0].RelationKind = types.DiagramRelCall
			}
			if mode == "wrong_window" {
				start, end := 9.0, 9.1
				copyIR := *ctx.AnalysisIR
				copyIR.RequestModel = ctx.AnalysisIR.RequestModel
				copyIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "9.0 to 9.1"}
				original := ctx.AnalysisIR
				ctx.AnalysisIR = &copyIR
				defer func() { ctx.AnalysisIR = original }()
			}
			raw, _ := json.Marshal(doc)
			res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
			if err != nil || res.Success {
				t.Fatalf("teaching changed hard authority: %v %+v", err, res)
			}
			if !strings.Contains(res.Summary, "does not turn runtime events into source calls") || !strings.Contains(res.Summary, "does not establish chain membership") {
				t.Fatalf("runtime repair confused namespaces: %s", res.Summary)
			}
			if mode == "wrong_window" && !strings.Contains(res.Summary, "No eligible runtime relation pair") {
				t.Fatal("out-of-window events advertised as eligible")
			}
		})
	}
}

func TestRuntimeDiagramRepairTeachingRequiresProducerObservationNotAttachmentOrProse(t *testing.T) {
	ctx := &types.BusContext{Mutable: types.NewMutableState("sched_wakeup trace diagram"), AttachedHitrace: "a.systrace"}
	if got := runtimeDiagramRelationRepairTeaching(ctx); got != "" {
		t.Fatalf("attachment/request selected runtime evidence: %s", got)
	}
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{Success: true, Observations: []types.ObservationRecord{{
		Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "model", GroundingPolicy: types.ClaimGroundingHard,
		SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "a.systrace", QueryScopeID: "q", PayloadRef: "p"},
		Summary:   "trace_query wakeup_chain",
	}}})
	if got := runtimeDiagramRelationRepairTeaching(ctx); got != "" {
		t.Fatalf("model prose selected runtime evidence: %s", got)
	}
	doc := runtimeNestingDoc(RuntimeDiagramRelation{FromNode: "A", ToNode: "B", FromLabel: "A", ToLabel: "B"}, false)
	hints := preCheckDiagramCallEdgeEvidenceAlignment(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, newPreEmitCheckContext(ctx))
	if len(hints) == 0 || !strings.Contains(hints[0].ExpectedShape, "citable typed call-edge EvidenceItem") {
		t.Fatalf("ordinary source guidance changed: %+v", hints)
	}
}
