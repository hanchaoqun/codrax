package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderingCandidatesActualFinalizerNavigation(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_rendering_candidates/events.systrace")
	start, end := 1.0, 1.05
	ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("渲染框架和线程角色"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh", PerfTrace: &types.PerfBundle{},
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1.000到1.050"}}}}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "rendering_candidates", "time_start": start, "time_end": end})
	r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !r.Success {
		t.Fatalf("query: %v %+v", err, r)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
	before, _ := json.Marshal(answerDocObservationLedger(ctx))
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{"### 渲染框架与线程角色候选", "Flutter", "ArkUI", "仅供后续排查导航", "PID=600", "RN", "KMP"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("lost actual finalizer content %q", want)
		}
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if string(before) != string(after) {
		t.Fatal("display modified original ledger")
	}
	ledger := answerDocObservationLedger(ctx)
	start, end = 1.01, 1.04
	if got := renderAnswerDocRenderingCandidates(ctx, ledger); got != "" {
		t.Fatal("wider signature query entered narrowed requested window")
	}
	start, end = 1, 1.05
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindProcess, PID: 100, Source: "user_explicit"}}
	if got := renderAnswerDocRenderingCandidates(ctx, ledger); !strings.Contains(got, "框架候选=ArkUI") || strings.Contains(got, "框架候选=Flutter") || !strings.Contains(got, "当前展示=1") {
		t.Fatalf("broad query lost the matching whole process candidate or leaked other owners: %s", got)
	}
	ctx.AnalysisIR.RequestModel.RuntimeTargets = nil
	for _, record := range r.Observations {
		if record.Predicate != tool.TraceRenderingCandidatesPredicate {
			continue
		}
		one := types.ObservationLedger{Records: []types.ObservationRecord{record, record}}
		if strings.Count(renderAnswerDocRenderingCandidates(ctx, one), "查询记录=") != 1 {
			t.Fatal("identical query repeated")
		}
		p, _ := tool.DecodeTraceRenderingCandidates(record)
		p.Caveats = append(append([]string(nil), p.Caveats...), "conflicting payload")
		data, _ := json.Marshal(p)
		one.Records[1].RichNotes = []string{types.TraceNoteKeyRenderingCandidates + "=" + string(data)}
		if got := renderAnswerDocRenderingCandidates(ctx, one); got != "" {
			t.Fatal("conflicting query selected by order")
		}
	}
}

func TestRenderingCandidatesActualFinalizerWholeCandidateTargetScope(t *testing.T) {
	// The first four Flutter examples are on TID 11. Its seventh occurrence is
	// on TID 12 in the same process: a bounded preview is not a thread census.
	var data strings.Builder
	data.WriteString("# tracer: nop\n")
	for i := 0; i < 7; i++ {
		tid, comm := 11, "render"
		if i == 6 {
			tid, comm = 12, "other"
		}
		fmt.Fprintf(&data, "%s-%d (10) [000] .... 1.%06d: tracing_mark_write: B|10|flutter::Draw\n", comm, tid, 1000+i*1000)
	}
	data.WriteString("render-21 (20) [000] .... 1.008000: tracing_mark_write: B|20|FlushMessages\n")
	// Marker pid 30 is not an emitter process header; process ownership stays unknown.
	data.WriteString("render-31 [000] .... 1.009000: tracing_mark_write: B|30|RNView::Draw\n")
	path := filepath.Join(t.TempDir(), "scope.systrace")
	if err := os.WriteFile(path, []byte(data.String()), 0600); err != nil {
		t.Fatal(err)
	}
	thread := func(id int, name string) types.RuntimeTarget {
		return types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, PID: id, Thread: name, Source: "user_explicit"}
	}
	process := func(id int) types.RuntimeTarget {
		return types.RuntimeTarget{Kind: types.RuntimeTargetKindProcess, PID: id, Source: "user_explicit"}
	}
	for _, tc := range []struct {
		name   string
		query  map[string]any
		target types.RuntimeTarget
		want   string
	}{
		{"known_process_with_omitted_examples", nil, process(10), "Flutter"},
		{"thread_hidden_other_owner", nil, thread(11, ""), ""},
		{"name_hidden_other_owner", nil, thread(0, "render"), "ArkUI"},
		{"same_process_query_not_thread_proof", map[string]any{"pid": 10, "target_scope": "process"}, thread(11, ""), ""},
		{"complete_thread_examples", nil, thread(21, ""), "ArkUI"},
		{"unknown_process_not_marker_pid", nil, process(30), ""},
		{"unknown_process_has_exact_thread", nil, thread(31, ""), "React Native"},
		{"explicit_tid_wins_over_name", nil, thread(99, "render"), ""},
		{"exact_thread_query", map[string]any{"pid": 11}, thread(11, ""), "Flutter"},
		{"exact_name_query", map[string]any{"thread": "render"}, thread(0, "render"), "Flutter"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start, end := 1.0, 1.05
			ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
				Mutable: types.NewMutableState("渲染候选"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh", PerfTrace: &types.PerfBundle{},
					RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1到1.05"}}}}
			params := map[string]any{"source": "path", "path": path, "view": "rendering_candidates", "time_start": start, "time_end": end}
			for k, v := range tc.query {
				params[k] = v
			}
			raw, _ := json.Marshal(params)
			r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
			if err != nil || !r.Success {
				t.Fatalf("query: %v %+v", err, r)
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
			ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{tc.target}
			ledger := answerDocObservationLedger(ctx)
			before, _ := json.Marshal(ledger)
			var original tracequery.RenderingCandidatesResult
			for _, record := range ledger.Records {
				if p, ok := tool.DecodeTraceRenderingCandidates(record); ok {
					original = p
				}
			}
			if original.TotalCandidates == 0 {
				t.Fatal("public query had no accepted typed candidates")
			}
			got := renderAnswerDocRenderingCandidates(ctx, ledger)
			if tc.want == "" {
				if got != "" {
					t.Fatalf("unproven target scope published: %s", got)
				}
			} else {
				if !strings.Contains(got, "框架候选="+tc.want) {
					t.Fatalf("whole matching candidate missing: %s", got)
				}
				if strings.Contains(got, "框架候选=Flutter") && tc.want != "Flutter" {
					t.Fatalf("unshown examples treated as a complete thread census: %s", got)
				}
				if !strings.Contains(got, fmt.Sprintf("候选组合总数=%d", original.TotalCandidates)) {
					t.Fatalf("producer population was relabeled as target population: %s", got)
				}
				if tc.name == "known_process_with_omitted_examples" && !strings.Contains(got, "匹配行次数=7（不是线程数或帧数）；示例省略=3") {
					t.Fatalf("whole-candidate count/examples changed: %s", got)
				}
				if (tc.name == "exact_thread_query" || tc.name == "exact_name_query") && !strings.Contains(got, "匹配行次数=6（不是线程数或帧数）；示例省略=2") {
					t.Fatalf("producer's exact query scope did not preserve the complete count: %s", got)
				}
				prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				if !strings.Contains(prompt, "框架候选="+tc.want) {
					t.Fatalf("actual finalizer lost scoped navigation")
				}
			}
			after, _ := json.Marshal(answerDocObservationLedger(ctx))
			if string(before) != string(after) {
				t.Fatal("scope projection changed original ledger")
			}
		})
	}
}
