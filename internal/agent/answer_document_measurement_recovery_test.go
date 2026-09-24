package agent

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeMeasurementRecoveryRebindsCurrentPublicQuery(t *testing.T) {
	for _, lane := range []string{"snapshot", "rejected", "text"} {
		for _, state := range []string{"current", "source_changed", "window_changed", "missing"} {
			t.Run(lane+"/"+state, func(t *testing.T) {
				ctx, result, _ := ioInFlightPublicContext(t, "en", types.RuntimeQuestionScopeBoundedFactSet, "")
				view := types.BuildAnswerSemanticViewForAgentContext(ctx)
				var blocks []types.AnswerBlock
				for _, table := range view.RuntimeMeasurementContract.Choices() {
					receipt := &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
					if !types.BindRuntimeMeasurementReceipt(receipt, view.RuntimeMeasurementContract) {
						t.Fatal("real producer table could not bind")
					}
					blocks = append(blocks, types.AnswerBlock{ID: "measure-" + string(table.View) + table.ObservationID, Kind: types.BlockTable, RuntimeMeasurement: receipt})
				}
				doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: blocks}
				expected := render.RenderAnswerDocument(doc, "en")
				raw, err := json.Marshal(doc)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(raw), "BoundTable") || strings.Contains(string(raw), "Actual start (s)") {
					t.Fatal("private data crossed the saved JSON boundary")
				}
				accepted := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "accepted", Kind: types.BlockSummary, Text: "Keep existing answer."}}}
				ctx.Mutable.RewriteAcceptedAnswerDocumentV2(accepted)
				ctx.Mutable.SetRetryState(&types.RetryState{PrevEmitJSON: raw})
				if lane == "rejected" {
					ctx.Mutable.SetRetryState(nil)
					ctx.Mutable.SetLastRejectedAnswerDocumentV2(doc)
				}
				switch state {
				case "source_changed":
					for i := range result.Observations {
						result.Observations[i].SourceRef.Path += ".changed"
					}
					ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
				case "window_changed":
					start, end := 2.0, 2.01
					ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeStart = &start
					ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeEnd = &end
				case "missing":
					ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{})
					ctx.Mutable.ResetDispatchToolResults()
				}
				var got *types.AnswerDocumentV2
				var ok bool
				if lane == "text" {
					rec, recovered := tool.RecoverAnswerDocumentV2FromText(string(raw))
					if !recovered {
						t.Fatal("actual text recovery rejected selectors")
					}
					_, ok = (&answerDocumentEvaluator{language: "en"}).parseRecoveredContentAnswerDocument(ctx, rec, &StageOutput{})
					got = rec.Document
				} else {
					got, ok = recoverRetryStateAnswerDocumentV2(ctx)
				}
				if state != "current" {
					if ok {
						t.Fatal("stale or missing producer evidence was recovered")
					}
					if !reflect.DeepEqual(ctx.Mutable.AnswerDocumentV2(), accepted) {
						t.Fatal("failed recovery replaced accepted answer")
					}
					return
				}
				if !ok || got == nil {
					t.Fatal("current producer receipts did not recover")
				}
				// Recovery may attach disclosures/system sections, but every selected
				// table must retain exact producer facts in the original order.
				if len(got.Blocks) < len(blocks) {
					t.Fatal("recovery dropped selected tables")
				}
				tables := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: got.Blocks[:len(blocks)]}
				if rendered := render.RenderAnswerDocument(tables, "en"); rendered != expected {
					t.Fatalf("saved selectors changed measurements after recovery:\n%s", rendered)
				}
				got.Blocks[0].RuntimeMeasurement.BoundTable.Rows[0][0] = "edited recovered copy"
				if render.RenderAnswerDocument(doc, "en") != expected {
					t.Fatal("recovery aliased original bound data")
				}
			})
		}
	}
}
