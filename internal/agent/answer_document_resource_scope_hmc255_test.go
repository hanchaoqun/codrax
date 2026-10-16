package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func resourceScopeContextHMC255(t *testing.T) *types.AgentContext {
	t.Helper()
	ctx := resourceFactsContextHMC249(t) // four actual, exact, native queries
	// Only the accepted current target survives. No rejected IR or original
	// request parsing supplies additional target authority.
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 101, Source: "user_explicit"}}
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	return ctx
}

func TestHMC255ExecutedResourceFactsSurviveTargetHandoff(t *testing.T) {
	ctx := resourceScopeContextHMC255(t)
	before, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
	ledgerBefore, _ := json.Marshal(answerDocObservationLedger(ctx))
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	choices := view.RuntimeMeasurementContract.Choices()
	if len(choices) != 8 {
		t.Fatalf("four current executed queries need eight independent native views, got %d", len(choices))
	}
	defaults := types.RuntimeNativeFactDisplaySelections(&ctx.AnalysisIR.RequestModel, view.RuntimeMeasurementContract)
	if len(defaults) != 2 {
		t.Fatalf("accepted thread 101 must own exactly two defaults, got %d", len(defaults))
	}
	frames, supplementary := 0, 0
	for _, table := range choices {
		if table.View == types.RuntimeMeasurementMembers {
			frames += len(table.Rows)
		}
		if strings.Contains(table.Label, "补充查询") {
			supplementary++
			if table.DefaultPresentation || table.HasCompleteNativeDisplayPopulation() || table.CoversMemberSet(&ctx.AnalysisIR.RequestModel) {
				t.Fatal("supplement gained requested population authority")
			}
			for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable, types.BlockDiagram} {
				r := &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
				if !types.BindRuntimeMeasurementReceipt(r, view.RuntimeMeasurementContract) {
					t.Fatal("current supplementary native view cannot bind")
				}
				visible := render.RenderAnswerDocument(&types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "facts", Kind: kind, RuntimeMeasurement: r}}}, "zh")
				if !strings.Contains(visible, "补充查询") || strings.Contains(visible, "-->") {
					t.Fatal("same-source scope lost or invented stack order")
				}
			}
		}
	}
	if frames != 14 || supplementary != 4 {
		t.Fatalf("lost full physical frame rows or query scope: frames=%d supplementary=%d", frames, supplementary)
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{"补充查询", "tid=202", "10015000000", "128", "单位未在该载体声明", "原深度", "匹配资源事件=0", "deterministic_runtime_queries=4"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("actual finalizer lost %q", want)
		}
	}
	after, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
	ledgerAfter, _ := json.Marshal(answerDocObservationLedger(ctx))
	if string(before) != string(after) || string(ledgerBefore) != string(ledgerAfter) {
		t.Fatal("display repaired user targets or changed causal ledger")
	}
}

func TestHMC255SupplementaryResourceSourceAndWindowBoundaries(t *testing.T) {
	for _, mode := range []string{"json", "stale", "source", "window", "causal", "relation", "memo", "failed"} {
		t.Run(mode, func(t *testing.T) {
			ctx := resourceScopeContextHMC255(t)
			results := ctx.Mutable.TurnAArtifacts().ToolResults
			switch mode {
			case "json":
				b, _ := json.Marshal(results)
				results = nil
				if err := json.Unmarshal(b, &results); err != nil {
					t.Fatal(err)
				}
			case "stale":
				for _, rel := range []string{"captures/capture.data", "captures/nested/capture.data"} {
					p := filepath.Join(ctx.RepoRoot, rel)
					b, err := os.ReadFile(p)
					if err != nil {
						t.Fatal(err)
					}
					b[len(b)-1] ^= 1
					if err := os.WriteFile(p, b, 0600); err != nil {
						t.Fatal(err)
					}
				}
			case "source":
				for i := range results {
					for j := range results[i].Observations {
						results[i].Observations[j].SourceRef.Path += ".copy"
					}
				}
			case "window":
				start, end := 10.02, 10.04
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "10.020..10.040"}
			case "causal":
				ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeCausalDiagnosis
			case "relation":
				ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeRelationAnalysis
			case "memo":
				for i := range results {
					results[i].ReusedFromRunMemo = true
				}
			case "failed":
				for i := range results {
					results[i].Success = false
				}
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
			ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
			view := types.BuildAnswerSemanticViewForAgentContext(ctx)
			for _, table := range view.RuntimeMeasurementContract.Choices() {
				if strings.Contains(table.Label, "补充查询") {
					t.Fatalf("%s gained supplementary query display", mode)
				}
			}
		})
	}
}

func TestHMC255BroadQueryKeepsExactMemberProjection(t *testing.T) {
	ctx, results := resourceSelectionPublicContextHMC248(t)
	r := resourceSelectionAllRecordHMC248(t, results)
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 101, Source: "user_explicit"}}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: []types.ObservationRecord{r}}}})
	prompt := dependencyObservationMessages(t, ctx)
	if !strings.Contains(prompt, "展示成员=3") || strings.Contains(prompt, "线程=background") || strings.Contains(prompt, "补充查询") {
		t.Fatal("broad query became an independent exact query or leaked unselected owners")
	}
	if !reflect.DeepEqual(ctx.AnalysisIR.RequestModel.RuntimeTargets, []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 101, Source: "user_explicit"}}) {
		t.Fatal("display changed accepted targets")
	}
}
