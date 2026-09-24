package tool

import (
	"encoding/json"
	"html"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSchedulerMeasurementPublicQueryEmitPatchRender(t *testing.T) {
	result, stats, path := schedulerConcurrencyToolQuery(t, schedulerConcurrencyPublicTrace, map[string]any{"bucket_ms": 3})
	start, end := 1.0, 1.01
	rm := types.RequestModel{Language: "zh", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{},
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1–1.01秒", Confidence: 1}}
	mu := types.NewMutableState("查看线程规模、持续分布和每3ms的变化")
	mu.SetRequestModel(rm)
	mu.AppendDispatchToolResult(result)
	mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", Mutable: mu, AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}}}
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	choices := view.RuntimeMeasurementContract.Choices()
	if len(choices) != 8 || stats == nil || stats.BucketMs != 3 {
		t.Fatalf("expected two states with four source-bound views: choices=%d stats=%+v", len(choices), stats)
	}
	before, _ := json.Marshal(types.CompileTraceCausalProjectionSet(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))))
	for _, table := range choices {
		if !strings.Contains(strings.Join(table.Notes, " "), path) || len(table.Rows) == 0 {
			t.Fatalf("missing native rows or exact source: %+v", table)
		}
		if table.View == types.RuntimeMeasurementTimeline && (len(table.Rows) != 4 || table.Rows[3][0] != "1.009000" || table.Rows[3][1] != "1.010000") {
			t.Fatalf("3ms timeline lost zero/short-tail bucket: %+v", table)
		}
		for _, patch := range []bool{false, true} {
			selected := map[string]any{"id": "measure", "kind": "table", "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}}
			input := map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": "以下为已确认线程区间的统计。"}, selected}}
			schema := BuildAnswerDocumentParametersFor(view)
			if patch {
				input = map[string]any{"replace_blocks": []any{selected}, "unchanged_block_ids": []string{"lead"}}
				schema = BuildAnswerDocumentPatchParametersFor(view)
			}
			raw, _ := json.Marshal(input)
			if err := toolparam.Validate(raw, schema); err != nil {
				t.Fatalf("schema rejects own native choice: %v", err)
			}
			if out := b1659bExecuteAnswer(t, ctx, input, patch); !out.Success {
				t.Fatalf("public emit/patch failed %s/%t: %s", table.View, patch, out.Summary)
			}
			doc := mu.AnswerDocumentV2()
			b := blockByID(t, doc, "measure")
			if b.RuntimeMeasurement == nil || !b.RuntimeMeasurement.IsBound() || !reflect.DeepEqual(*b.RuntimeMeasurement.BoundTable, table) {
				t.Fatal("native table changed on binding")
			}
			visible := html.UnescapeString(render.RenderAnswerDocument(doc, "zh"))
			for _, row := range table.Rows {
				for _, cell := range row {
					if cell != "" && !strings.Contains(visible, cell) {
						t.Fatalf("bound value missing from render: %s", cell)
					}
				}
			}
		}
	}
	after, _ := json.Marshal(types.CompileTraceCausalProjectionSet(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))))
	if string(before) != string(after) || mu.TraceRootCauseReport() != nil {
		t.Fatal("measurement display mutated causal authority")
	}
}

func TestSchedulerMeasurementPublicationSourceAndAccounting(t *testing.T) {
	result, stats, _ := schedulerConcurrencyToolQuery(t, schedulerConcurrencyPublicTrace, map[string]any{"bucket_ms": 3})
	var source types.ObservationRecord
	for _, r := range result.Observations {
		if r.Predicate == "scheduler_concurrency" && r.Subject == "running" {
			source = r
		}
	}
	if _, ok := types.DecodeRuntimeMeasurementPublication(source); !ok {
		t.Fatal("public query receipt failed physical source finalization")
	}
	for _, field := range []string{"source", "window", "generation", "duplicate"} {
		t.Run(field, func(t *testing.T) {
			r := source
			r.RichNotes = append([]string(nil), source.RichNotes...)
			switch field {
			case "source":
				r.SourceRef.Path += ".other"
			case "window":
				r.SourceRef.QueryWindowEndTs += 1
			case "generation":
				r.SourceRef.QueryScopeID += ".other"
			case "duplicate":
				for _, n := range source.RichNotes {
					if strings.HasPrefix(n, types.TraceNoteKeyRuntimeMeasurement+"=") {
						r.RichNotes = append(r.RichNotes, n)
					}
				}
			}
			if _, ok := types.DecodeRuntimeMeasurementPublication(r); ok {
				t.Fatal("mismatched publication acquired display authority")
			}
		})
	}
	for _, group := range stats.Groups {
		group.AcceptedIntervalCount++
		if traceQuerySchedulerConcurrencyReceipt(source, stats, group) != "" {
			t.Fatal("contradictory witness accounting published")
		}
	}
}

func TestSchedulerMeasurementNanosecondAndLineWindow(t *testing.T) {
	body := "idle-0 (0) [001] .... 0.000000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=41 next_prio=120\n" +
		"worker-41 (41) [001] .... 0.000000001: sched_switch: prev_comm=worker prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n"
	for _, lines := range []bool{false, true} {
		args := map[string]any{"time_start": 0, "time_end": .000000010, "bucket_ms": 3}
		if lines {
			args["line_start"], args["line_end"] = 1, 2
		}
		result, _, _ := schedulerConcurrencyToolQuery(t, body, args)
		var found bool
		for _, r := range result.Observations {
			if r.Predicate != "scheduler_concurrency" || r.Subject != "running" {
				continue
			}
			publication, ok := types.DecodeRuntimeMeasurementPublication(r)
			if !ok {
				t.Fatal("valid nanosecond interval lost source-bound publication")
			}
			found = true
			for _, table := range publication.Tables {
				switch table.View {
				case types.RuntimeMeasurementMembers:
					if len(table.Rows) != 1 || table.Rows[0][3] != "0.000000" || table.Rows[0][4] != "0.000000001" {
						t.Fatalf("actual endpoints collapsed by display precision: %+v", table)
					}
					if lines && (table.Rows[0][5] != "不可用" || table.Rows[0][6] != "不可用") {
						t.Fatal("line selection invented a continuous-time contribution")
					}
				case types.RuntimeMeasurementSummary:
					if lines && table.Rows[0][0] != "不可用" {
						t.Fatal("line-only statistics became a measured zero")
					}
				case types.RuntimeMeasurementDistribution, types.RuntimeMeasurementTimeline:
					if lines && len(table.Rows) != 0 {
						t.Fatal("line selection invented a time distribution or curve")
					}
				}
			}
		}
		if !found {
			t.Fatal("missing running interval measurement")
		}
	}
}
