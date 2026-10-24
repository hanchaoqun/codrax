package agent

import (
	"encoding/json"
	"fmt"
	"html"
	"reflect"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Fidelity regression, not a claimed production RED: the HMC287 answer failed
// despite these independently scoped facts and producer limits being present.
// Exercise real preparation/query and actual Finalizer context construction;
// do not validate or rewrite the model's free-form interpretation.
func TestHMC288ResourceQueryScopesActualFinalizerFidelity(t *testing.T) {
	ctx, _ := resourceSelectionPublicContextHMC248(t)
	ctx.AgentName, ctx.Stage = types.AgentExplorer, types.StageExplore
	ctx.Mutable.ResetDispatchToolResults()
	type query struct {
		name                           string
		start, end                     float64
		pid, events, frames, displayed int
		owners                         map[int]int
		record                         types.ObservationRecord
		payload                        tracequery.ResourceStackResult
		publication                    types.RuntimeMeasurementPublication
	}
	queries := []query{
		{name: "whole_query", start: 0, end: 60, events: 5, frames: 14, displayed: 4, owners: map[int]int{101: 4, 202: 1}},
		{name: "requested_window", start: 10, end: 10.05, events: 4, frames: 11, displayed: 4, owners: map[int]int{101: 3, 202: 1}},
		{name: "thread_101", start: 10, end: 10.05, pid: 101, events: 3, frames: 8, displayed: 3, owners: map[int]int{101: 3}},
	}
	var results []types.ToolResult
	ids, scopes := map[string]bool{}, map[string]bool{}
	var source string
	for i := range queries {
		q := &queries[i]
		args := map[string]any{"path": "captures/capture.data", "view": "resource_stack", "time_start": q.start, "time_end": q.end}
		if q.pid != 0 {
			args["pid"] = q.pid
		}
		raw, _ := json.Marshal(args)
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
		if err != nil || !result.Success {
			t.Fatalf("%s query: %v %s", q.name, err, result.Summary)
		}
		for _, record := range result.Observations {
			payload, ok := tool.DecodeTraceResourceStack(record)
			if !ok {
				continue
			}
			if q.record.ID != "" {
				t.Fatalf("%s has duplicate resource observations", q.name)
			}
			q.record, q.payload = record, payload
			q.publication, ok = types.DecodeRuntimeMeasurementPublication(record)
			if !ok {
				t.Fatalf("%s native publication missing", q.name)
			}
		}
		if q.record.ID == "" || q.record.SourceRef.QueryScopeID == "" || q.record.SourceRef.PayloadRef == "" || ids[q.record.ID] || scopes[q.record.SourceRef.QueryScopeID] {
			t.Fatalf("%s lost independent query identity: %+v", q.name, q.record.SourceRef)
		}
		ids[q.record.ID], scopes[q.record.SourceRef.QueryScopeID] = true, true
		if source == "" {
			source = q.record.SourceRef.Path
		}
		if source == "" || q.record.SourceRef.Path != source || q.payload.SourcePath != source ||
			!q.record.SourceRef.QueryWindowKnown || q.record.SourceRef.QueryWindowStartTs != q.start || q.record.SourceRef.QueryWindowEndTs != q.end ||
			q.record.SourceRef.QueryTargetScope != "thread" || q.record.SourceRef.QueryTargetPID != q.pid ||
			q.payload.Window.StartTs != q.start || q.payload.Window.EndTs != q.end || q.payload.Window.EndInclusive {
			t.Fatalf("%s source/selector/window changed: %+v / %+v", q.name, q.record.SourceRef, q.payload.Window)
		}
		owners, frames := map[int]int{}, 0
		for _, event := range q.payload.Events {
			owners[event.Source.TID]++
			frames += len(event.Frames)
			if event.OmittedFrames != 0 || !event.SourceFramesComplete || event.UnwindCompleteness != "unknown" {
				t.Fatalf("%s frame census/unknown unwind changed: %+v", q.name, event)
			}
		}
		if q.payload.MatchedEvents != q.events || len(q.payload.Events) != q.events || q.payload.OmittedEvents != 0 || frames != q.frames || !reflect.DeepEqual(owners, q.owners) {
			t.Fatalf("%s population: events=%d frames=%d owners=%v", q.name, q.payload.MatchedEvents, frames, owners)
		}
		s := q.payload.EventStatistics
		if s == nil || !s.Complete || s.EventCount != q.events || s.OmittedGroups != 0 || s.OmittedGroupEvents != 0 {
			t.Fatalf("%s original statistical population missing: %+v", q.name, s)
		}
		results = append(results, result)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	ctx.Mutable.ResetDispatchToolResults()
	ctx = ctxbuilder.BuildAgentContext(types.ToolBusContext(ctx, types.AgentFinalizer), types.AgentFinalizer, types.StageFinalize)
	before, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
	beforeProjection, _ := json.Marshal(types.TraceCausalProjectionFromObservationRecords(answerDocObservationLedger(ctx).Records))
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	for _, q := range queries {
		t.Run(q.name, func(t *testing.T) {
			section := resourceQueryPromptSectionHMC288(t, prompt, q.record.ID)
			for _, want := range []string{
				fmt.Sprintf("source=%q", source), fmt.Sprintf("query_scope=%q", q.record.SourceRef.QueryScopeID), fmt.Sprintf("payload=%q", q.record.SourceRef.PayloadRef),
				fmt.Sprintf("scope=thread，id=%d", q.pid), fmt.Sprintf("窗口=[%.9f, %.9f)秒", q.start, q.end),
				fmt.Sprintf("原查询匹配事件=%d", q.events), fmt.Sprintf("完整扫描事件=%d", q.events),
				"不以本次展示的事件明细为分母", "tid=101", "源帧载体完整=true，真实展开终点=未知",
				"缺深度数=1", "重复深度数=1", "未知符号数=1", "不按最深帧或固定倒数层指定业务根因",
				"ImageCache::reserve", "UIFrame::render", "buffer_cleanup", "未知（未提供）", "未知（有歧义）",
			} {
				if !strings.Contains(section, want) {
					t.Errorf("query-local Finalizer section lost %q", want)
				}
			}
			if strings.Count(section, "源事件物理rowid=") != q.displayed || strings.Contains(section, "10050000000ns") || strings.Contains(section, "tid=202") != (q.pid == 0) {
				t.Fatalf("query-local member/owner/right-edge mismatch: %s", section)
			}
			counts := map[types.RuntimeMeasurementView]int{}
			for _, table := range view.RuntimeMeasurementContract.Choices() {
				if table.ObservationID != q.record.ID {
					continue
				}
				counts[table.View] = len(table.Rows)
				if !table.HasNativeResourceFacts() || table.MemberSet != nil || table.DisplayPopulation != nil || table.HasCompleteNativeDisplayPopulation() || table.CoversMemberSet(&ctx.AnalysisIR.RequestModel) {
					t.Fatal("resource facts gained requested-population authority or lost native binding")
				}
				if table.View == types.RuntimeMeasurementDistribution {
					if !reflect.DeepEqual(table.Rows, q.publication.Tables[2].Rows) {
						t.Fatal("display projection recomputed the original query distribution")
					}
					continue
				}
				for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable, types.BlockDiagram} {
					ref := &types.AnswerRuntimeMeasurementReceipt{ObservationID: q.record.ID, View: table.View}
					if !types.BindRuntimeMeasurementReceipt(ref, view.RuntimeMeasurementContract) {
						t.Fatal("current exact table failed binding")
					}
					visible := html.UnescapeString(render.RenderAnswerDocument(&types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "native", Kind: kind, RuntimeMeasurement: ref}}}, "zh"))
					if strings.Contains(visible, "10050000000") || strings.Contains(visible, "线程ID=202") != (q.pid == 0) || strings.Contains(visible, "-->") {
						t.Fatalf("%s/%s changed members or invented a directed edge", kind, table.View)
					}
				}
			}
			wantFrames := 11
			if q.pid != 0 {
				wantFrames = 8
			}
			if len(counts) != 3 || counts[types.RuntimeMeasurementSummary] != q.displayed || counts[types.RuntimeMeasurementMembers] != wantFrames {
				t.Fatalf("query-specific view census: %v", counts)
			}
		})
	}
	if relations := tool.RuntimeDiagramRelations(answerDocObservationLedger(ctx), &ctx.AnalysisIR.RequestModel); len(relations) != 0 {
		t.Fatalf("raw frame depths became relation credentials: %+v", relations)
	}
	// This request does not require a relation diagram, so its optional diagram
	// teaching is not an unconditional output obligation. The native limit and
	// empty typed relation roster are the relevant invariant.
	if !strings.Contains(prompt, "不证明函数执行时长、等待或响应根因") {
		t.Error("actual Finalizer lost the native resource authority boundary")
	}
	after, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
	afterProjection, _ := json.Marshal(types.TraceCausalProjectionFromObservationRecords(answerDocObservationLedger(ctx).Records))
	if string(before) != string(after) || string(beforeProjection) != string(afterProjection) {
		t.Fatal("Finalizer presentation changed original query facts or causal projection")
	}
	for _, mode := range []string{"json_restored", "foreign_source"} {
		t.Run(mode, func(t *testing.T) {
			copy := ctx.ShallowClone()
			copy.Mutable = types.NewMutableState(ctx.Objective)
			copy.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
			var invalid []types.ToolResult
			if mode == "json_restored" {
				raw, _ := json.Marshal(results)
				if err := json.Unmarshal(raw, &invalid); err != nil {
					t.Fatal(err)
				}
			} else {
				for _, result := range results {
					result.Observations = append([]types.ObservationRecord(nil), result.Observations...)
					for i := range result.Observations {
						result.Observations[i].SourceRef.Path += ".foreign"
					}
					invalid = append(invalid, result)
				}
			}
			copy.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: invalid})
			invalidPrompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(copy, nil)
			for _, q := range queries {
				// JSON may retain current-source legacy observation prose. It
				// must not recreate the private native table/relationship seal;
				// a foreign physical source cannot keep even the scoped prose.
				if mode == "foreign_source" && strings.Contains(invalidPrompt, fmt.Sprintf("observation_id=%q；source=", q.record.ID)) {
					t.Fatal("unbound source entered the authoritative resource section")
				}
			}
			for _, table := range types.BuildAnswerSemanticViewForAgentContext(copy).RuntimeMeasurementContract.Choices() {
				if ids[table.ObservationID] && table.HasNativeResourceFacts() {
					t.Fatal("unbound source borrowed a current native table")
				}
			}
			if len(tool.RuntimeDiagramRelations(answerDocObservationLedger(copy), &copy.AnalysisIR.RequestModel)) != 0 {
				t.Fatal("restored or foreign resource rows acquired relation credentials")
			}
		})
	}
}

func resourceQueryPromptSectionHMC288(t *testing.T, prompt, id string) string {
	t.Helper()
	marker := fmt.Sprintf("observation_id=%q；source=", id)
	start := strings.Index(prompt, marker)
	if start < 0 {
		t.Fatalf("actual Finalizer lost resource query %q", id)
	}
	section := prompt[start:]
	for _, end := range []string{"\nobservation_id=", "\n资源栈展示受查询数与64KiB预算约束"} {
		if i := strings.Index(section, end); i >= 0 {
			section = section[:i]
		}
	}
	return section
}
