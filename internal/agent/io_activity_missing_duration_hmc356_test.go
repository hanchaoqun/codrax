package agent

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC356IOActivityDoesNotPublishUnaccountedDurationsAsZero(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			ctx, result, inFlight := ioInFlightPublicContext(t, lang, types.RuntimeQuestionScopeBoundedFactSet, "")
			if inFlight == nil || len(inFlight.Groups) != 4 {
				t.Fatal("the authored source must retain all four exact in-flight accounts")
			}
			var pressure *types.ObservationRecord
			for i := range result.Observations {
				if strings.HasSuffix(result.Observations[i].ID, "#io_pressure:1") {
					pressure = &result.Observations[i]
					break
				}
			}
			if pressure == nil || pressure.Value != "10.000" || pressure.Unit != types.TraceObservationUnitCompositeScore {
				t.Fatalf("the original positive latency contribution and index must survive: %+v", pressure)
			}
			body, err := os.ReadFile(pressure.SourceRef.PayloadRef)
			if err != nil {
				t.Fatal(err)
			}
			var native tracequery.Result
			if err := json.Unmarshal(body, &native); err != nil || native.WindowStats == nil || native.WindowStats.IOPressureSummary == nil {
				t.Fatal("actual native activity payload missing", err)
			}
			activity := native.WindowStats.IOPressureSummary
			if len(native.WindowStats.DStateTop) != 0 || len(native.WindowStats.IOWaitTop) != 0 || activity.Score != activity.BlockMaxLatencyMs || activity.Score < 9.99 || activity.Score > 10.01 {
				t.Fatal("the negative control has no state duration account but does have admitted IO latency")
			}
			before, _ := json.Marshal(types.CompileTraceCausalProjection(answerDocObservationLedger(ctx)))
			// The unmodified full ledger can leave an optional background row
			// unselected. Exercise that actual published row as the one current
			// supporting result, preserving all of its source/query coordinates.
			selected := ctx.ShallowClone()
			selected.Mutable = types.NewMutableState("Explain this observed IO activity")
			// This subcase selects an already queried background fact. Do not
			// fabricate an attached-source match to force a bounded selection.
			selected.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Language: lang, PerfTrace: &types.PerfBundle{}}}
			selected.Mutable.SetRequestModel(selected.AnalysisIR.RequestModel)
			selected.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
				ToolName: "trace_query", Success: true, RawRef: pressure.SourceRef.RawRef, Observations: []types.ObservationRecord{*pressure},
			}}})
			selectedLedger := answerDocObservationLedger(selected)
			coverage := types.BuildTraceObservationCoverage(selectedLedger)
			if len(coverage.TopObservations) != 1 || coverage.TopObservations[0].ID != pressure.ID {
				t.Fatalf("harness did not retain selected result in coverage; records=%d top=%d", len(selectedLedger.Records), len(coverage.TopObservations))
			}
			qualifier := types.FormatObservationValueContexts(pressure.ValueContext)
			// Use the real Finalizer context under both the ordinary and whole-
			// section budget paths. Matching is a regression assertion only.
			for _, budgeted := range []bool{false, true} {
				eval := &answerDocumentEvaluator{}
				var prompt string
				if budgeted {
					prompt = eval.BuildInitialInstructionWithBudget(selected, nil, func() *finalizerPromptBudget {
						return &finalizerPromptBudget{RequestAllocationBytes: 700000}
					})
				} else {
					prompt = eval.BuildInitialInstruction(selected, nil)
				}
				for face, text := range map[string]string{"native": activity.Summary, "tool": result.Summary, "observation": pressure.Summary + " " + strings.Join(pressure.RichNotes, " "), "finalizer": prompt} {
					for _, bad := range []string{"d_state=0.000", "io_wait=0.000"} {
						if strings.Contains(text, bad) {
							t.Errorf("budgeted=%t %s publishes unsupported state duration %s", budgeted, face, bad)
						}
					}
				}
				if !strings.Contains(prompt, pressure.ID) || !strings.Contains(prompt, "block_max=10.000") {
					t.Error("repair cannot simply hide the valid positive activity account")
				}
				if qualifier == "" || !strings.Contains(prompt, qualifier) || !strings.Contains(prompt, "d_state=not_accounted") || !strings.Contains(prompt, "io_wait=not_accounted") {
					t.Error("the published activity must retain its complete qualification at the consumer")
				}
				var top string
				for _, line := range strings.Split(prompt, "\n") {
					if strings.HasPrefix(line, "- top[") && strings.Contains(line, "id=`"+pressure.ID+"`") {
						top = line
						break
					}
				}
				if top == "" || !strings.Contains(top, qualifier) {
					t.Error("soft coverage preview independently dropped the operand qualification")
				}
			}
			after, _ := json.Marshal(types.CompileTraceCausalProjection(answerDocObservationLedger(ctx)))
			if string(before) != string(after) {
				t.Fatal("rendering changed causal projection state")
			}
		})
	}
}
