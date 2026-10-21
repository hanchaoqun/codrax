package agent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Real public queries intentionally have the same source/measurement window
// but different index witnesses. The file exceeds the public windowed-parse
// threshold; comments add bytes, not fake scheduler or dependency evidence.
func TestHMC272NativeTimelineBoundaryActualFinalizerMessages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.systrace")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := bufio.NewWriter(f)
	fmt.Fprintln(w, "idle-0 (0) [000] .... 1.980000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120")
	fmt.Fprintln(w, "target-41 (41) [000] .... 1.990000: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120")
	padding := "# " + strings.Repeat("x", 1020) + "\n"
	for i := 0; i < 66000; i++ {
		if _, err := w.WriteString(padding); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 100; i++ {
		ts := 2.001 + float64(i)*0.0008
		fmt.Fprintf(w, "worker-52 (52) [000] .... %.6f: cpu_frequency: state=%d cpu_id=0\n", ts-0.000001, 1000000+i*1000)
		fmt.Fprintf(w, "worker-52 (52) [001] .... %.6f: cpu_frequency: state=%d cpu_id=1\n", ts-0.000001, 1500000+i*1000)
		fmt.Fprintf(w, "worker-52 (52) [000] .... %.6f: sched_wakeup: comm=target pid=41 prio=120 target_cpu=0\n", ts)
		fmt.Fprintf(w, "idle-0 (0) [000] .... %.6f: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120\n", ts+0.00001)
		fmt.Fprintf(w, "target-41 (41) [000] .... %.6f: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n", ts+0.00002)
	}
	// An unrelated event proves coverage to the requested end without closing
	// target-41's sleep. The independently padded chain query sees its closure.
	fmt.Fprintln(w, "worker-52 (52) [001] .... 2.100000: cpu_frequency: state=1800000 cpu_id=1")
	fmt.Fprintln(w, "worker-52 (52) [000] .... 2.400000: sched_wakeup: comm=target pid=41 prio=120 target_cpu=0")
	fmt.Fprintln(w, "idle-0 (0) [000] .... 2.410000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120")
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	start, end := 2.0, 2.1
	rm := types.RequestModel{Language: "zh", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{},
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 41, Thread: "target-41", Source: "user_explicit"}},
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState, types.RuntimeQuestionFactDirectWaker}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end}}
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("线程在此窗口的状态及唤醒者"), AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}},
		TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, ".codrax")})}
	ctx.Mutable.SetRequestModel(rm)
	var results []types.ToolResult
	for _, view := range []string{"thread_timeline", "wakeup_chain"} {
		params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "pid": 41, "time_start": start, "time_end": end})
		out, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
		if err != nil || !out.Success {
			t.Fatalf("public %s: %v / %s", view, err, out.Summary)
		}
		results = append(results, types.AttachToolHandoffCarrier(out))
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results, HandoffCarriers: types.ToolHandoffCarriersFromTurnAInputs(results, nil, nil)})
	ctx = ctxbuilder.BuildAgentContext(types.ToolBusContext(ctx, types.AgentFinalizer), types.AgentFinalizer, types.StageFinalize)
	before, _ := json.Marshal(results)
	projectionBefore := hmc218CausalProjection(t, ctx)
	choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
	messages := dependencyObservationMessages(t, ctx)
	previews := hmc218MeasurementMessagePreviews(t, messages)
	threadTables, frequencyTables, open, closed, previewRows := 0, 0, false, false, 0
	metadata := map[string]measurementHandoffPreview{}
	for _, line := range strings.Split(messages, "\n") {
		if pos := strings.Index(line, "; preview="); pos >= 0 {
			var p measurementHandoffPreview
			if err := json.Unmarshal([]byte(line[pos+len("; preview="):]), &p); err != nil {
				t.Fatal(err)
			}
			for _, table := range choices {
				if strings.Contains(line[:pos], fmt.Sprintf("observation_id=%q view=%q", table.ObservationID, table.View)) {
					metadata[table.ObservationID+"/"+string(table.View)] = p
				}
			}
		}
	}
	for _, p := range previews {
		previewRows += len(p.Rows)
	}
	if previewRows > 128 {
		t.Fatalf("preview budget expanded: %d", previewRows)
	}
	for _, table := range choices {
		if table.DefaultPresentation && table.View == types.RuntimeMeasurementTimeline {
			p := metadata[table.ObservationID+"/"+string(table.View)]
			if len(p.Rows) != len(p.SourceRowIndices) || len(p.Rows) == 0 || p.Selection != "table_boundary_sample" || p.SeriesBoundaryCoverage != "not_claimed" {
				t.Fatalf("native partial preview lost index/coverage disclosure: %+v", p)
			}
			for i, index := range p.SourceRowIndices {
				if index < 0 || index >= len(table.Rows) || !reflect.DeepEqual(p.Rows[i], table.Rows[index]) || i > 0 && p.SourceRowIndices[i-1] >= index {
					t.Fatal("native preview reordered producer groups or rewrote rows")
				}
			}
			if strings.HasSuffix(table.ObservationID, "#cpu_frequency_timeline_observation") {
				frequencyTables++
				if table.Rows[0][0] != "CPU0" || table.Rows[len(table.Rows)-1][0] != "CPU1" || p.SourceRowIndices[len(p.Rows)-1] != len(table.Rows)-1 {
					t.Fatal("multi-CPU public fixture/last-row preservation failed")
				}
			}
		}
		if !strings.HasSuffix(table.ObservationID, "#thread_timeline_observation") {
			continue
		}
		threadTables++
		last := table.Rows[len(table.Rows)-1]
		open = open || last[7] == "未知"
		closed = closed || last[7] == "2.400000"
		preview := previews[table.ObservationID+"/"+string(table.View)]
		if len(preview.Rows) >= len(table.Rows) {
			t.Fatal("fixture failed to exercise bounded preview")
		}
		found := false
		for _, row := range preview.Rows {
			if reflect.DeepEqual(row, last) {
				found = true
			}
		}
		if !found {
			t.Errorf("actual finalizer lost source-bound boundary row for %s: %v", table.ObservationID, last)
		}
	}
	if threadTables != 2 || frequencyTables != 1 || !open || !closed {
		t.Fatalf("independent query witnesses missing: threads=%d frequency=%d open=%t closed=%t", threadTables, frequencyTables, open, closed)
	}
	for _, want := range []string{"source_row_indices", "本次查询的索引配置时间范围", "2.400000", "query-local", "state_account_observation_id=", "末端证据行也可能只是等待位置引用"} {
		if !strings.Contains(messages, want) {
			t.Errorf("actual finalizer boundary handoff missing %q", want)
		}
	}
	states := types.BuildTraceTargetStateScopeAuthorities(types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx)))
	if len(states) != 1 || states[0].EvidenceID == "" {
		t.Fatalf("expected one unchanged selected state account: %+v", states)
	}
	if !strings.Contains(messages, fmt.Sprintf("state_account_observation_id=%q", states[0].EvidenceID)) {
		t.Fatal("principal recap lost the exact selected account identity")
	}
	// Native final output still binds every original row of each independent
	// selector. The preview's non-contiguous sample must never replace it.
	params, _ := json.Marshal(map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": "各查询的原生状态和频率记录如下；不推断根因。"}}})
	out, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), params)
	if err != nil || !out.Success {
		t.Fatalf("native display: %v / %s", err, out.Summary)
	}
	seen := map[string]bool{}
	for _, block := range ctx.Mutable.AnswerDocumentV2().Blocks {
		if block.RuntimeMeasurement == nil {
			continue
		}
		for _, table := range choices {
			if table.DefaultPresentation && block.RuntimeMeasurement.ObservationID == table.ObservationID && block.RuntimeMeasurement.View == table.View {
				if seen[table.ObservationID] || !block.RuntimeMeasurement.IsBound() || !reflect.DeepEqual(block.RuntimeMeasurement.BoundTable.Rows, table.Rows) {
					t.Fatal("bounded preview replaced, merged or duplicated native output")
				}
				seen[table.ObservationID] = true
			}
		}
	}
	if len(seen) != 3 {
		t.Fatalf("lost independent thread timelines or multi-CPU frequency table: %d", len(seen))
	}
	after, _ := json.Marshal(results)
	if !bytes.Equal(before, after) || !bytes.Equal(projectionBefore, hmc218CausalProjection(t, ctx)) {
		t.Fatal("presentation changed native results or causal authority")
	}
}

// The ordinary causal wait-owner route, not a finite timeline-only request,
// supplies the measured pre-wakeup edge capsule. Check its shared guidance at
// both the public producer and actual finalizer adapter without new authority.
func TestHMC272WakeupWaitScopePublicHandoff(t *testing.T) {
	data, err := os.ReadFile("../../eval/cases/trace_query_wakeup_causal_io_chain.case")
	if err != nil {
		t.Fatal(err)
	}
	_, trace, ok := strings.Cut(string(data), "HTRACE='")
	if !ok {
		t.Fatal("trace absent")
	}
	trace, _, ok = strings.Cut(trace, "\n'\n")
	if !ok {
		t.Fatal("trace not closed")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "chain.ftrace")
	if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
		t.Fatal(err)
	}
	start, end := 2.0, 2.020
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "pid": 100, "time_start": start, "time_end": end, "trace_flavor": "harmony_hitrace"})
	result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil || !result.Success {
		t.Fatalf("public wait-owner query: %v %s", err, result.Summary)
	}
	ctx := tracePrincipalValueAuthorityTestContext("app-100", 100, result.Observations)
	ctx.RepoRoot, ctx.WorkDir, ctx.Language = dir, dir, "zh"
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis}
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end}
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	ctx = ctxbuilder.BuildAgentContext(types.ToolBusContext(ctx, types.AgentFinalizer), types.AgentFinalizer, types.StageFinalize)
	before, _ := json.Marshal(result)
	projectionBefore := hmc218CausalProjection(t, ctx)
	messages := dependencyObservationMessages(t, ctx)
	for face, text := range map[string]string{"tool": result.Summary, "actual_finalizer": messages} {
		for _, want := range []string{types.TraceWakeupPreWaitGuidance, "wakee=app-100 pre_wakeup_wait=20.000ms"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s lost measured owner/value or shared clipped-start guidance: %q", face, want)
			}
		}
	}
	if strings.Contains(messages, "pre-wakeup wait is sleep/blocking start → sched_wakeup") {
		t.Fatal("unqualified duplicate teaching survived shared guidance")
	}
	after, _ := json.Marshal(result)
	if !bytes.Equal(before, after) || !bytes.Equal(projectionBefore, hmc218CausalProjection(t, ctx)) {
		t.Fatal("teaching changed native values or causal authority")
	}
}
