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

func TestHMC243SamplingFactRecoverySurfaceAuthority(t *testing.T) {
	for _, lane := range []string{"snapshot", "rejected", "text"} {
		for _, family := range []string{"native", "legacy"} {
			for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable, types.BlockDiagram} {
				for _, missing := range []bool{false, true} {
					t.Run(lane+"/"+family+"/"+string(kind)+map[bool]string{false: "/current", true: "/missing"}[missing], func(t *testing.T) {
						ctx, result := filteredSamplingContextHMC242(t, false)
						if family == "legacy" {
							ctx, result, _ = ioInFlightPublicContext(t, "en", types.RuntimeQuestionScopeBoundedFactSet, "")
						}
						ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
						view := types.BuildAnswerSemanticViewForAgentContext(ctx)
						var receipt *types.AnswerRuntimeMeasurementReceipt
						for _, table := range view.RuntimeMeasurementContract.Choices() {
							if (family == "native") != table.HasNativeSamplingFacts() || table.EventData != nil || family == "native" && (table.View != types.RuntimeMeasurementSummary || !strings.Contains(table.Label, "/app/A.so")) {
								continue
							}
							receipt = &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
							if !types.BindRuntimeMeasurementReceipt(receipt, view.RuntimeMeasurementContract) {
								t.Fatal("current receipt did not bind")
							}
							break
						}
						if receipt == nil {
							t.Fatal("native/legacy public fixture missing")
						}
						doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "lead", Kind: types.BlockSummary, Text: "Observed facts follow."}, {ID: "facts", Kind: kind, RuntimeMeasurement: receipt}}}
						expected := render.RenderAnswerDocument(doc, "en")
						raw, _ := json.Marshal(doc)
						accepted := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "accepted", Kind: types.BlockSummary, Text: "Keep accepted answer."}}}
						ctx.Mutable.RewriteAcceptedAnswerDocumentV2(accepted)
						ctx.Mutable.SetRetryState(&types.RetryState{PrevEmitJSON: raw})
						if lane == "rejected" {
							ctx.Mutable.SetRetryState(nil)
							ctx.Mutable.SetLastRejectedAnswerDocumentV2(doc)
						}
						if missing {
							ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{})
							ctx.Mutable.ResetDispatchToolResults()
						}
						var got *types.AnswerDocumentV2
						var ok bool
						if lane == "text" {
							rec, recovered := tool.RecoverAnswerDocumentV2FromText(string(raw))
							if recovered {
								_, ok = (&answerDocumentEvaluator{language: "en"}).parseRecoveredContentAnswerDocument(ctx, rec, &StageOutput{})
								got = rec.Document
							}
						} else {
							got, ok = recoverRetryStateAnswerDocumentV2(ctx)
						}
						want := !missing && (family == "native" || kind == types.BlockTable)
						if ok != want {
							t.Fatalf("recovered=%t want=%t", ok, want)
						}
						if !want {
							if !reflect.DeepEqual(ctx.Mutable.AnswerDocumentV2(), accepted) {
								t.Fatal("rejected recovery mutated accepted answer")
							}
							return
						}
						if got == nil || len(got.Blocks) < 2 || render.RenderAnswerDocument(&types.AnswerDocumentV2{DocumentModel: "v2", Blocks: got.Blocks[:2]}, "en") != expected {
							t.Fatal("current recovery silently lost native measurements")
						}
					})
				}
			}
		}
	}
}

func TestHMC243SamplingMeasurementActualFinalizer(t *testing.T) {
	for _, direct := range []bool{false, true} {
		for _, supplement := range []bool{false, true} {
			ctx, result := filteredSamplingContextHMC242(t, direct)
			if supplement {
				ctx.Mutable.SetSystemTraceSupplement(types.SystemTraceSupplementMeta{}, []types.ToolResult{result})
			} else {
				ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			}
			for _, r := range result.Observations {
				if r.Predicate == types.RuntimeSamplingMeasurementPredicate {
					ctx.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{Active: true, Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: r.SourceRef.Path, Carrier: "attachment"}}}
					break
				}
			}
			before, _ := json.Marshal(result.Observations)
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			_, part, _ := strings.Cut(prompt, "### 已核对的测量表 / Verified measurement tables")
			for _, want := range []string{"Native sampling fact choices", "15/35", "100/100", "7/7", "查询总体记录数", "质量维度", "总体内样本记录数", "不是热点子集质量", "查询窗口 [1.000000, 2.000000)", "symbol=work"} {
				if !strings.Contains(part, want) {
					t.Errorf("direct=%t supplement=%t actual prompt missing %q", direct, supplement, want)
				}
			}
			if strings.Contains(prompt, types.TraceNoteKeyRuntimeMeasurement+"={") {
				t.Fatal("raw publication JSON repeated in finalizer context")
			}
			after, _ := json.Marshal(result.Observations)
			if string(before) != string(after) {
				t.Fatal("display changed native observations")
			}
			if decided, allowed := types.RuntimeTraceReportShapeAuthority(&ctx.AnalysisIR.RequestModel); !decided || allowed {
				t.Fatal("fact display enabled causal authority")
			}
		}
	}
}

func TestHMC243SamplingMeasurementRequestScope(t *testing.T) {
	for _, mode := range []string{"target", "process", "window", "source"} {
		t.Run(mode, func(t *testing.T) {
			ctx, result := filteredSamplingContextHMC242(t, false)
			switch mode {
			case "target":
				ctx.AnalysisIR.RequestModel.RuntimeTargets[0].PID = 12
			case "process":
				ctx.AnalysisIR.RequestModel.RuntimeTargets[0].Kind = types.RuntimeTargetKindProcess
			case "window":
				start, end := 3.0, 4.0
				s := ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile
				s.TimeStart, s.TimeEnd = &start, &end
				s.TimeWindows = nil
			case "source":
				for i := range result.Observations {
					if result.Observations[i].Predicate == types.RuntimeSamplingMeasurementPredicate {
						result.Observations[i].SourceRef.Path += ".other"
					}
				}
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			view := types.BuildAnswerSemanticViewForAgentContext(ctx)
			for _, table := range view.RuntimeMeasurementContract.Choices() {
				if table.HasNativeSamplingFacts() {
					t.Fatalf("%s admitted unrequested sampling facts", mode)
				}
			}
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			if strings.Contains(prompt, "Native sampling fact choices") {
				t.Fatal("actual finalizer exposed disallowed selector")
			}
		})
	}
}
