package tool

import (
	"encoding/json"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The raw totals are not wrong: each belongs to the original state segments
// selected by one recursive query, not to one state in the outer query window.
// Keep the real query and answer entry points in the regression so an isolated
// formatter test cannot hide a lost window on the publication/projection path.
func TestHMC313PublicActualCaliberScope(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			bus := &types.BusContext{Language: lang, RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("native actual caliber")}
			result := businessRefTestQuery(t, bus, map[string]any{"path": path, "view": "root_cause_rank", "pid": 100, "time_start": .999, "time_end": 1.051})
			if !result.Success {
				t.Fatal(result.Summary)
			}
			native := businessSpanSchedulerPublicPayload(t, result)
			if native.WakeupChain == nil || native.RootCauseRank == nil {
				t.Fatal("original chain and four ranked items must remain")
			}
			var ranked []string
			for _, item := range native.RootCauseRank.Items {
				if item.Rank > 0 {
					ranked = append(ranked, item.Type)
				}
			}
			if !reflect.DeepEqual(ranked, []string{"io_latency", "scheduler_latency", "runnable_wait", "runnable_wait"}) {
				t.Fatalf("original four ranked items changed: %v", ranked)
			}
			found := 0
			for _, impact := range native.WakeupChain.CausalImpacts {
				wantTotal, wantImpact, wantStart, wantEnd := 0.0, 0.0, 0.0, 0.0
				if impact.Thread.PID == 100 && impact.DominantState == "runnable" {
					wantTotal, wantImpact, wantStart, wantEnd = 6, 1, 1.045, 1.051
				} else if impact.Thread.PID == 200 && impact.DominantState == "s_sleep" {
					wantTotal, wantImpact, wantStart, wantEnd = 41.5, 31, 1.004, 1.0455
				} else {
					continue
				}
				found++
				sum := impact.ActualRunningMs + impact.ActualRunnableMs + impact.ActualSleepMs + impact.ActualDStateMs + impact.ActualIOWaitMs
				if math.Abs(impact.ActualTotalMs-wantTotal) > 1e-6 || math.Abs(impact.ActualImpactMs-wantImpact) > 1e-6 || math.Abs(sum-wantTotal) > 1e-6 ||
					impact.ActualWindow.StartTs != wantStart || impact.ActualWindow.EndTs != wantEnd {
					t.Fatalf("raw original-state accounting changed: %+v sum=%v", impact, sum)
				}
				t.Logf("native pid=%d projected=%+v original=%+v actual_impact=%.3f actual_total=%.3f actual_state_sum=%.3f", impact.Thread.PID, impact.Window, impact.ActualWindow, impact.ActualImpactMs, impact.ActualTotalMs, sum)
			}
			if found != 2 {
				t.Fatalf("expected both original accounts, got %d", found)
			}
			requestFound, backgroundFound := false, false
			for _, request := range native.WindowStats.IOLatencies {
				requestFound = requestFound || (math.Abs(request.DurationMs-35) < 1e-6 && math.Abs(request.IssuerBlockedMs-31) < 1e-6)
				backgroundFound = backgroundFound || math.Abs(request.DurationMs-47) < 1e-6
			}
			if !requestFound || !backgroundFound {
				t.Fatal("request residence, closed wait and background must retain independent rulers")
			}
			bus.ToolResults = []types.ToolResult{result}
			bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}, AnswerContract: types.AnswerContract{Language: lang}}
			ledger := types.ObservationLedger{Records: result.Observations}
			contract, err := tracefinding.CompileCandidateContract(ledger, types.CompileTraceCausalProjectionSet(ledger), tracefinding.SeatFrameCausalityAuthority{})
			if err != nil {
				t.Fatal(err)
			}
			selected := ""
			for _, candidate := range contract.Candidates {
				if candidate.PrimaryEligible && candidate.Decision.Token.Token == "io_latency" && math.Abs(candidate.Decision.Magnitude.Value-31) < 1e-6 {
					selected = candidate.Decision.CandidateID
				}
			}
			if selected == "" {
				t.Fatal("closed 31ms wait lost its rank eligibility")
			}
			contract.RootCauseReportEnabled = true
			bus.Mutable.SetTraceFindingContract(contract)
			const prose = "Independent native measurements remain unchanged."
			raw, _ := json.Marshal(map[string]any{
				"blocks":            []map[string]any{{"id": "lead", "kind": "summary", "surface_role": "principal", "trace_causal_claim_caliber": "no_causal_conclusion", "text": prose}},
				"trace_root_causes": map[string]any{"schema_version": 2, "root_causes": []map[string]any{{"candidate_id": selected, "description": prose}}},
			})
			emitted, err := (&EmitAnswerDocument{}).Execute(bus, raw)
			if err != nil || !emitted.Success {
				t.Fatalf("public emit: %v %s", err, emitted.Summary)
			}
			doc, report := bus.Mutable.AnswerDocumentV2(), bus.Mutable.TraceRootCauseReport()
			visible := render.RenderAnswerDocument(doc, lang)
			t.Logf("public rendered answer:\n%s", visible)
			wanted := []string{"1.045000..1.051000s", "1.004000..1.045500s", "selected original-state segments total 6.000ms", "selected original-state segments total 41.500ms", "does not by itself establish a same-state, business-window or whole-query-window total", "does not establish continuous execution"}
			if lang == "zh" {
				wanted = []string{"1.045000..1.051000s", "1.004000..1.045500s", "所选原始状态段合计 6.000ms", "所选原始状态段合计 41.500ms", "不能据此替代同状态、业务窗或全查询窗统计", "不证明连续执行", "小计 2.000ms(区间互斥)"}
			}
			for _, want := range wanted {
				if !strings.Contains(visible, want) {
					t.Errorf("actual-caliber disclosure missing %q", want)
				}
			}
			if report == nil || len(report.RootCauses) != 1 || report.RootCauses[0].ImpactSeconds == nil || math.Abs(*report.RootCauses[0].ImpactSeconds-.031) > 1e-9 {
				t.Fatalf("display must not change selected root: %+v", report)
			}
			patched, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{"unchanged_block_ids":["lead"]}`))
			if err != nil || !patched.Success || !reflect.DeepEqual(doc, bus.Mutable.AnswerDocumentV2()) || !reflect.DeepEqual(report, bus.Mutable.TraceRootCauseReport()) {
				t.Fatalf("no-op patch changed native projection: %v %s", err, patched.Summary)
			}
		})
	}
}
