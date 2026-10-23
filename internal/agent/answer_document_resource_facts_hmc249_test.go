package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func resourceFactsContextHMC249(t *testing.T) *types.AgentContext {
	t.Helper()
	ctx, _ := resourceSelectionPublicContextHMC248(t)
	var results []types.ToolResult
	for _, path := range []string{"captures/capture.data", "captures/nested/capture.data"} {
		for _, pid := range []int{101, 202} {
			args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "resource_stack", "pid": pid, "time_start": 10, "time_end": 10.05})
			result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
			if err != nil || !result.Success {
				t.Fatalf("resource query: %v %s", err, result.Summary)
			}
			results = append(results, result)
		}
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	ctx.Mutable.ResetDispatchToolResults()
	return ctx
}

func TestHMC249ResourceFactsActualFinalizer(t *testing.T) {
	ctx := resourceFactsContextHMC249(t)
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	var tables []types.RuntimeMeasurementTable
	for _, table := range view.RuntimeMeasurementContract.Choices() {
		// Preserve the original event/frame-pair oracle; HMC279 separately
		// verifies complete-query statistics in the actual Finalizer context.
		if strings.HasSuffix(table.ObservationID, "#resource_stack") && table.View != types.RuntimeMeasurementDistribution {
			tables = append(tables, table)
		}
	}
	if len(tables) != 8 {
		for _, result := range ctx.Mutable.TurnAArtifacts().ToolResults {
			for _, r := range result.Observations {
				if r.Predicate == tool.TraceResourceStackPredicate {
					_, decoded := types.DecodeRuntimeMeasurementPublication(r)
					t.Logf("resource publication decoded=%v source=%+v notes=%d", decoded, r.SourceRef, len(r.RichNotes))
				}
			}
		}
		t.Fatalf("four exact native resource queries need summary/member pairs, got %d", len(tables))
	}
	frames := 0
	for _, table := range tables {
		if table.View == types.RuntimeMeasurementMembers {
			frames += len(table.Rows)
		}
	}
	if frames != 14 {
		t.Fatalf("four queries lost original frame population: %d", frames)
	}
	before, _ := json.Marshal(ctx.Mutable.TurnAArtifacts().ToolResults)
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	after, _ := json.Marshal(ctx.Mutable.TurnAArtifacts().ToolResults)
	if string(before) != string(after) {
		t.Fatal("presentation changed accepted native observations")
	}
	if !strings.Contains(prompt, "Native resource fact choices") {
		t.Fatal("actual finalizer cannot choose shared resource facts")
	}
	if strings.Contains(prompt, types.TraceNoteKeyRuntimeMeasurement+"={") {
		t.Fatal("publication JSON duplicated in finalizer context")
	}
	for _, table := range tables {
		for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable, types.BlockDiagram} {
			ref := &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
			if !types.BindRuntimeMeasurementReceipt(ref, view.RuntimeMeasurementContract) {
				t.Fatal("native resource pair did not bind")
			}
			text := render.RenderAnswerDocument(&types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "facts", Kind: kind, RuntimeMeasurement: ref}}}, "zh")
			if !strings.Contains(text, "源事件") || kind == types.BlockDiagram && strings.Contains(text, "-->") {
				t.Fatalf("%s lost source event or fabricated an ordered edge: %s", kind, text)
			}
		}
	}
}

func TestHMC249ResourceFactsActualScope(t *testing.T) {
	for _, mode := range []string{"target", "process", "window", "source", "json", "generation"} {
		t.Run(mode, func(t *testing.T) {
			ctx := resourceFactsContextHMC249(t)
			results := ctx.Mutable.TurnAArtifacts().ToolResults
			switch mode {
			case "target":
				ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 999, Source: "user_explicit"}}
			case "process":
				for i := range ctx.AnalysisIR.RequestModel.RuntimeTargets {
					ctx.AnalysisIR.RequestModel.RuntimeTargets[i].Kind = types.RuntimeTargetKindProcess
				}
			case "window":
				start, end := 11.0, 12.0
				s := ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile
				s.TimeStart, s.TimeEnd = &start, &end
				s.TimeWindows = nil
			case "source":
				for i := range results {
					for j := range results[i].Observations {
						results[i].Observations[j].SourceRef.Path += ".foreign"
					}
				}
			case "json":
				raw, _ := json.Marshal(results)
				results = nil
				if json.Unmarshal(raw, &results) != nil {
					t.Fatal("roundtrip")
				}
			case "generation":
				for _, name := range []string{"capture.data", "nested/capture.data"} {
					path := filepath.Join(ctx.RepoRoot, "captures", name)
					data, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					data[len(data)-1] ^= 1
					if err := os.WriteFile(path, data, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
			if mode == "target" || mode == "process" {
				// Current exact executions remain readable, but neither a
				// different ID nor thread/process resemblance owns the answer.
				view := types.BuildAnswerSemanticViewForAgentContext(ctx)
				choices := resourceOnlyChoicesHMC287(t, view.RuntimeMeasurementContract.Choices())
				if len(choices) != 12 || resourceOnlyDefaultsHMC287(t, ctx, view.RuntimeMeasurementContract) != 0 {
					t.Fatal("target mismatch lost execution facts or acquired default scope")
				}
				for _, table := range choices {
					if !strings.Contains(table.Label, "补充查询") || table.CoversMemberSet(&ctx.AnalysisIR.RequestModel) {
						t.Fatal("mismatched target acquired requested coverage")
					}
				}
				return
			}
			for _, table := range types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices() {
				if table.HasNativeResourceFacts() {
					t.Fatalf("%s accepted stale/out-of-request facts", mode)
				}
			}
			if strings.Contains((&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil), "Native resource fact choices") {
				t.Fatal("actual prompt exposed invalid selector")
			}
		})
	}
}

func TestHMC249ResourceFactsActualRecovery(t *testing.T) {
	for _, lane := range []string{"snapshot", "rejected", "text"} {
		ctx := resourceFactsContextHMC249(t)
		results := ctx.Mutable.TurnAArtifacts().ToolResults
		view := types.BuildAnswerSemanticViewForAgentContext(ctx)
		var table types.RuntimeMeasurementTable
		for _, c := range view.RuntimeMeasurementContract.Choices() {
			if c.HasNativeResourceFacts() && c.View == types.RuntimeMeasurementMembers && len(c.Rows) == 8 {
				table = c
				break
			}
		}
		if table.ObservationID == "" {
			t.Fatal("complete8frame native pair missing")
		}
		for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable, types.BlockDiagram} {
			for _, missing := range []bool{false, true} {
				t.Run(lane+"/"+string(kind)+map[bool]string{true: "/missing", false: "/current"}[missing], func(t *testing.T) {
					ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
					if missing {
						ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{})
						ctx.Mutable.ResetDispatchToolResults()
					}
					ref := &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
					if !types.BindRuntimeMeasurementReceipt(ref, view.RuntimeMeasurementContract) {
						t.Fatal("original bind")
					}
					doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "facts", Kind: kind, RuntimeMeasurement: ref}}}
					expected := render.RenderAnswerDocument(doc, "zh")
					raw, _ := json.Marshal(doc)
					accepted := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "accepted", Kind: types.BlockSummary, Text: "原答案"}}}
					ctx.Mutable.RewriteAcceptedAnswerDocumentV2(accepted)
					ctx.Mutable.SetRetryState(&types.RetryState{PrevEmitJSON: raw})
					ctx.Mutable.SetLastRejectedAnswerDocumentV2(nil)
					if lane == "rejected" {
						ctx.Mutable.SetRetryState(nil)
						ctx.Mutable.SetLastRejectedAnswerDocumentV2(doc)
					}
					var got *types.AnswerDocumentV2
					var ok bool
					if lane == "text" {
						rec, found := tool.RecoverAnswerDocumentV2FromText(string(raw))
						if found {
							_, ok = (&answerDocumentEvaluator{language: "zh"}).parseRecoveredContentAnswerDocument(ctx, rec, &StageOutput{})
							got = rec.Document
						}
					} else {
						got, ok = recoverRetryStateAnswerDocumentV2(ctx)
					}
					if ok == missing {
						t.Fatalf("recovery ok=%t missing=%t", ok, missing)
					}
					if missing {
						if !reflect.DeepEqual(ctx.Mutable.AnswerDocumentV2(), accepted) {
							t.Fatal("failed restore changed accepted document")
						}
						return
					}
					if got == nil || len(got.Blocks) == 0 || render.RenderAnswerDocument(&types.AnswerDocumentV2{DocumentModel: "v2", Blocks: got.Blocks[:1]}, "zh") != expected {
						t.Fatal("recovery changed native facts")
					}
				})
			}
		}
	}
}
