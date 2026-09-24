package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func schedulerMeasurementPublicContext(t *testing.T, path string) (*types.AgentContext, types.ToolResult, *tracequery.SchedulerConcurrencyStats) {
	t.Helper()
	if path == "" {
		var err error
		path, err = filepath.Abs("../../eval/fixtures/hmosperf_scheduler_concurrency/events.systrace")
		if err != nil {
			t.Fatal(err)
		}
	}
	start, end := 1.0, 1.01
	ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("分析 1.000～1.010 秒等待 CPU 和运行的线程规模、各深度持续时间、每 3ms 变化及主要线程。"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh", PerfTrace: &types.PerfBundle{},
			RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}},
			RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 101, Thread: "render", Source: "user_request"}},
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1.000～1.010 秒"}}}}
	args, err := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": start, "time_end": end, "bucket_ms": 3})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
	if err != nil || !result.Success {
		t.Fatalf("real scheduler query failed: %v / %s", err, result.Summary)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	for _, row := range result.Observations {
		if row.Predicate != "scheduler_concurrency" {
			continue
		}
		data, err := os.ReadFile(row.SourceRef.PayloadRef)
		if err != nil {
			t.Fatal(err)
		}
		var payload tracequery.Result
		if err := json.Unmarshal(data, &payload); err != nil || payload.WindowStats == nil || payload.WindowStats.SchedulerConcurrency == nil {
			t.Fatalf("actual source payload lost scheduler account: %v", err)
		}
		return ctx, result, payload.WindowStats.SchedulerConcurrency
	}
	t.Fatal("real query published no scheduler population")
	return nil, types.ToolResult{}, nil
}

func TestSchedulerMeasurementActualFinalizerHandoffAndScope(t *testing.T) {
	ctx, result, native := schedulerMeasurementPublicContext(t, "")
	before, _ := json.Marshal(result)
	if native.Window == nil || native.Window.StartTs != 1 || native.Window.EndTs != 1.01 || native.BucketMs != 3 || len(native.Groups) != 2 {
		t.Fatalf("real query lost explicit axis: %+v", native)
	}
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	choices := view.RuntimeMeasurementContract.Choices()
	if len(choices) != 8 {
		t.Fatalf("expected two states × four source-bound views, got %d", len(choices))
	}
	states := map[string]string{}
	publications := map[string]types.RuntimeMeasurementTable{}
	for _, row := range result.Observations {
		if row.Predicate != "scheduler_concurrency" {
			continue
		}
		p, ok := types.DecodeRuntimeMeasurementPublication(row)
		if !ok || len(p.Tables) != 4 || row.Role != types.AnswerAggregateRoleSupportingCoverage ||
			!reflect.DeepEqual(p.Source, row.SourceRef) || row.SourceRef.PayloadRef == "" || row.SourceRef.QueryScopeID == "" {
			t.Fatalf("producer identity/role/publication missing: %+v", row)
		}
		start, end, known := types.TraceObservationContinuousQueryWindow(row.SourceRef)
		if !known || start != 1 || end != 1.01 {
			t.Fatalf("selected table lost continuous source window: %+v", row.SourceRef)
		}
		if states[row.ID] != "" {
			t.Fatal("duplicate scheduler observation ID")
		}
		states[row.ID] = row.Subject
		for _, table := range p.Tables {
			key := table.ObservationID + "/" + string(table.View)
			if _, exists := publications[key]; exists {
				t.Fatalf("duplicate selector %s", key)
			}
			publications[key] = table
		}
		fact := answerDocBoundedRuntimeFactAuthorityRow(row, &ctx.AnalysisIR.RequestModel, "zh")
		if strings.Contains(fact, "target_owned") || !strings.Contains(fact, "selected_window_context") {
			t.Fatalf("all-thread measurement gained user-target ownership: %s", fact)
		}
	}
	if len(states) != 2 || len(publications) != 8 {
		t.Fatalf("population census changed: states=%v tables=%d", states, len(publications))
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	seen := map[string]map[types.RuntimeMeasurementView]bool{}
	for _, table := range choices {
		state := states[table.ObservationID]
		if state != "running" && state != "runnable" {
			t.Fatalf("unpublished scheduler identity %q", table.ObservationID)
		}
		if seen[state] == nil {
			seen[state] = map[types.RuntimeMeasurementView]bool{}
		}
		seen[state][table.View] = true
		key := table.ObservationID + "/" + string(table.View)
		if !reflect.DeepEqual(table, publications[key]) {
			t.Fatalf("contract changed source-bound publication %s", key)
		}
		selector := fmt.Sprintf("observation_id=%q view=%q", table.ObservationID, table.View)
		preview := runtimeMeasurementPreviewInPrompt(t, prompt, selector)
		if !reflect.DeepEqual(preview.Columns, table.Columns) || !reflect.DeepEqual(preview.Rows, table.Rows) {
			t.Fatalf("actual finalizer lost complete small-fixture preview: %s / %+v", selector, preview)
		}
		for _, note := range table.Notes {
			if !strings.Contains(prompt, note) {
				t.Fatalf("actual finalizer lost scope/unknown/population teaching: %s / %s", selector, note)
			}
		}
		if table.View == types.RuntimeMeasurementSummary {
			want := [][]string{{"2", "0.8", "6", "8"}}
			if state == "running" {
				want = [][]string{{"2", "0.5", "4", "5"}}
			}
			if !reflect.DeepEqual(table.Rows, want) {
				t.Fatalf("wrong source-bound %s summary: %v", state, table.Rows)
			}
		}
		if table.View == types.RuntimeMeasurementDistribution {
			want := [][]string{{"0", "4", "40"}, {"1", "4", "40"}, {"2", "2", "20"}}
			if state == "running" {
				want = [][]string{{"0", "6", "60"}, {"1", "3", "30"}, {"2", "1", "10"}}
			}
			if !reflect.DeepEqual(table.Rows, want) {
				t.Fatalf("depth distribution is not wall-time weighted: %s / %v", state, table.Rows)
			}
		}
		if table.View == types.RuntimeMeasurementMembers {
			for _, row := range table.Rows {
				if strings.Contains(row[0], "104") || strings.Contains(row[0], "unfinished") {
					t.Fatal("unclosed thread was promoted into confirmed members")
				}
			}
		}
	}
	for _, state := range []string{"running", "runnable"} {
		for _, kind := range []types.RuntimeMeasurementView{types.RuntimeMeasurementSummary, types.RuntimeMeasurementMembers, types.RuntimeMeasurementDistribution, types.RuntimeMeasurementTimeline} {
			if !seen[state][kind] {
				t.Fatalf("missing %s/%s", state, kind)
			}
		}
	}
	after, _ := json.Marshal(result)
	if string(before) != string(after) || ctx.Mutable.TraceRootCauseReport() != nil || strings.Contains(prompt, types.TraceNoteKeyRuntimeMeasurement+"={") {
		t.Fatal("display mutated input, minted root authority or leaked private publication JSON")
	}
	start, end := 1.002, 1.008
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeStart = &start
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeEnd = &end
	if len(types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()) != 0 {
		t.Fatal("whole-window tables were borrowed into a narrower requested window")
	}
	changedPrompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for id := range states {
		if strings.Contains(changedPrompt, fmt.Sprintf("observation_id=%q view=", id)) {
			t.Fatal("stale window selector remained in actual finalizer instruction")
		}
	}
}

func TestSchedulerMeasurementOpenTailRemainsUnknownInFinalizer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "open.systrace")
	body := " idle-0 (0) [000] .... 0.998000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=idle next_pid=0 next_prio=120\n" +
		" waker-900 (900) [000] .... 1.001000: sched_wakeup: comm=unfinished pid=104 prio=120 target_cpu=000\n" +
		" idle-0 (0) [000] .... 1.012000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=idle next_pid=0 next_prio=120\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, _, native := schedulerMeasurementPublicContext(t, path)
	if len(native.Groups) != 1 || native.Groups[0].State != "runnable" || native.Groups[0].Values != nil || native.Groups[0].Distribution != nil || len(native.Groups[0].Buckets) != 0 || native.Groups[0].Coverage.OpenEndedIntervals != 1 {
		t.Fatalf("open population fabricated measured zeros: %+v", native)
	}
	choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
	if len(choices) != 4 {
		t.Fatalf("unknown population lost selectable disclosures: %d", len(choices))
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, table := range choices {
		selector := fmt.Sprintf("observation_id=%q view=%q", table.ObservationID, table.View)
		preview := runtimeMeasurementPreviewInPrompt(t, prompt, selector)
		if table.View == types.RuntimeMeasurementSummary {
			if !reflect.DeepEqual(preview.Rows, [][]string{{"不可用", "不可用", "不可用", "不可用"}}) {
				t.Fatalf("unknown summary became zero: %+v", preview)
			}
		} else if len(table.Rows) != 0 || len(preview.Rows) != 0 {
			t.Fatalf("open tail manufactured %s rows: %+v", table.View, table.Rows)
		}
	}
	if !strings.Contains(prompt, "未闭合 1") || !strings.Contains(prompt, "不证明系统空闲") || ctx.Mutable.TraceRootCauseReport() != nil {
		t.Fatal("unknown or causal boundary missing from actual finalizer message")
	}
}

func TestSchedulerMeasurementRecoveryRebindsCurrentPublicQuery(t *testing.T) {
	for _, lane := range []string{"snapshot", "rejected", "text"} {
		for _, state := range []string{"current", "source_changed", "window_changed", "missing"} {
			t.Run(lane+"/"+state, func(t *testing.T) {
				ctx, _, native := schedulerMeasurementPublicContext(t, "")
				view := types.BuildAnswerSemanticViewForAgentContext(ctx)
				choices := view.RuntimeMeasurementContract.Choices()
				if len(choices) != 8 {
					t.Fatalf("need all eight real table choices, got %d", len(choices))
				}
				var blocks []types.AnswerBlock
				for i, table := range choices {
					receipt := &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
					if !types.BindRuntimeMeasurementReceipt(receipt, view.RuntimeMeasurementContract) {
						t.Fatal("real source-bound table failed to bind")
					}
					blocks = append(blocks, types.AnswerBlock{ID: fmt.Sprintf("measurement-%d", i), Kind: types.BlockTable, RuntimeMeasurement: receipt})
				}
				doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: blocks}
				expected := render.RenderAnswerDocument(doc, "zh")
				raw, err := json.Marshal(doc)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(raw), "BoundTable") || strings.Contains(string(raw), `"rows"`) || strings.Contains(string(raw), `"notes"`) || strings.Contains(string(raw), `"columns"`) {
					t.Fatal("private measurements crossed the saved selector JSON boundary")
				}
				accepted := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "accepted", Kind: types.BlockSummary, Text: "保留已经接受的回答。"}}}
				ctx.Mutable.RewriteAcceptedAnswerDocumentV2(accepted)
				ctx.Mutable.SetRetryState(&types.RetryState{PrevEmitJSON: raw})
				if lane == "rejected" {
					ctx.Mutable.SetRetryState(nil)
					ctx.Mutable.SetLastRejectedAnswerDocumentV2(doc)
				}
				switch state {
				case "source_changed":
					// Re-query identical bytes at a distinct physical source. This
					// must not authorize the old source's saved selectors.
					body, err := os.ReadFile(native.Groups[0].SourcePath)
					if err != nil {
						t.Fatal(err)
					}
					path := filepath.Join(t.TempDir(), "other.systrace")
					if err := os.WriteFile(path, body, 0600); err != nil {
						t.Fatal(err)
					}
					_, changed, _ := schedulerMeasurementPublicContext(t, path)
					ctx.Mutable.ResetDispatchToolResults()
					ctx.Mutable.AppendDispatchToolResult(changed)
					ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{changed}})
				case "window_changed":
					start, end := 1.002, 1.008
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
						t.Fatal("text recovery dropped selector-only tables")
					}
					_, ok = (&answerDocumentEvaluator{language: "zh"}).parseRecoveredContentAnswerDocument(ctx, rec, &StageOutput{})
					got = rec.Document
				} else {
					got, ok = recoverRetryStateAnswerDocumentV2(ctx)
				}
				if state != "current" {
					if ok {
						t.Fatal("stale or absent current evidence authorized saved tables")
					}
					if !reflect.DeepEqual(ctx.Mutable.AnswerDocumentV2(), accepted) {
						t.Fatal("failed recovery overwrote the accepted answer")
					}
					return
				}
				if !ok || got == nil || len(got.Blocks) < len(blocks) {
					t.Fatal("current source-bound selectors failed to recover")
				}
				tables := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: got.Blocks[:len(blocks)]}
				if rendered := render.RenderAnswerDocument(tables, "zh"); rendered != expected {
					t.Fatalf("recovery changed trusted values/member order/axis:\n%s", rendered)
				}
				got.Blocks[0].RuntimeMeasurement.BoundTable.Rows[0][0] = "修改恢复副本"
				if render.RenderAnswerDocument(doc, "zh") != expected || !reflect.DeepEqual(choices, view.RuntimeMeasurementContract.Choices()) {
					t.Fatal("recovered tables alias original document or producer contract")
				}
				if ctx.Mutable.TraceRootCauseReport() != nil {
					t.Fatal("measurement recovery minted root-cause authority")
				}
			})
		}
	}
}
