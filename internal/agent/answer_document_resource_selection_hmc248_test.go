package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// No attached PerfBundle: restore a real catalog, execute its current query
// calls, and carry the actual accepted bounded target profile to finalization.
func resourceSelectionPublicContextHMC248(t *testing.T) (*types.AgentContext, []types.ToolResult) {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"capture.data", "nested/capture.data"} {
		data, err := os.ReadFile(filepath.Join("../../eval/fixtures/hmosperf_catalog_objects/captures", name))
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, "captures", name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	const question = "继续 checkpoint.json 中 captures 的检查，只分析这些采集文件：10.000 到 10.050 秒内线程 101 和 202 的内存分配、释放及调用位置。"
	ctx := &types.AgentContext{Objective: question, RepoRoot: root, WorkDir: root, Language: "zh", AgentName: types.AgentAnalyzer, Stage: types.StageAnalyze,
		Mutable: types.NewMutableState(question), TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(root, ".codrax")})}
	seed, err := (&tool.TraceCatalog{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), json.RawMessage(`{"root":"captures","queries":[{"view":"resource_stack","time_start":10,"time_end":10.05},{"view":"resource_stack","pid":202,"time_start":10,"time_end":10.05}]}`))
	if err != nil || !seed.Success {
		t.Fatalf("seed catalog: %v %s", err, seed.Summary)
	}
	var page struct {
		SavedIndex string `json:"saved_index"`
	}
	if err := json.Unmarshal([]byte(seed.Summary), &page); err != nil || page.SavedIndex == "" {
		t.Fatalf("catalog snapshot: %v %s", err, seed.Summary)
	}
	ctx.Mutable = types.NewMutableState(question)
	raw := json.RawMessage(`{
		"intent":"trace","scenario":"generic","complexity":"moderate","language":"zh",
		"keywords":["resource"],"entities":["checkpoint.json","captures","101","202"],"question_kind":"mechanism","predicate_axis":"flow",
		"intent_confidence":1,"complexity_confidence":1,"kind_confidence":1,
		"predicates":{"is_scalar_answer":false,"is_role_locate_lookup":false,"is_count_question":false,"is_cross_component":false,"is_relational_lookup":false,"is_category_enumeration":false,"is_history_lookup":false,"is_diagnostic_question":false,"has_per_member_table":false},
		"diagnostic_profile":{"is_diagnostic":false,"current_risk":false,"historical_regression":false,"current_version_check":false,"confidence":1},
		"answer_role_profile":{"is_role_binding_requested":false,"confidence":1},"error_granularity_profile":{"is_granularity_question":false,"confidence":1},
		"runtime_artifact_scope_profile":{"requested_scope":"explicit_time_window","time_start":10,"time_end":10.05,"source_quote":"10.000 到 10.050 秒内线程 101 和 202","confidence":1},
		"runtime_question_profile":{"scope":"bounded_fact_set","runtime_work_relation_requested":true,"frame_causality_requested":false,"fact_families":["count_or_duration","target_scheduler_state"],"source_quote":"内存分配、释放及调用位置","confidence":1},
		"runtime_target_profile":{"declaration":"named_target","source_quote":"线程 101 和 202","confidence":1},
		"runtime_targets":[{"kind":"thread","thread":"101","source":"user_explicit","confidence":1},{"kind":"thread","thread":"202","source":"user_explicit","confidence":1}],
		"runtime_selection_profile":{"is_selection_question":false,"source_quote":"","confidence":1},"completeness_obligation":{"required":false,"source_quote":""}
	}`)
	analysis, err := (&tool.EmitAnalysis{}).Execute(types.ToolBusContext(ctx, types.AgentAnalyzer), raw)
	if err != nil || !analysis.Success {
		t.Fatalf("public analysis: %v %s", err, analysis.Summary)
	}
	ctx.AnalysisIR, err = buildAnalysisIR(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ctx.AgentName, ctx.Stage = types.AgentExplorer, types.StageExplore
	ctx.UserPinnedFiles = []string{page.SavedIndex}
	resumeArgs, _ := json.Marshal(map[string]any{"action": "resume", "root": filepath.Join(root, "captures"), "snapshot_path": page.SavedIndex})
	resumed, err := (&tool.TraceCatalog{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), resumeArgs)
	if err != nil || !resumed.Success {
		t.Fatalf("public resume: %v %s", err, resumed.Summary)
	}
	var current struct {
		Calls map[string]json.RawMessage `json:"query_calls"`
	}
	if err := json.Unmarshal([]byte(resumed.Summary), &current); err != nil || len(current.Calls) != 4 {
		t.Fatalf("current calls: %v %s", err, resumed.Summary)
	}
	keys := make([]string, 0, len(current.Calls))
	for key := range current.Calls {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	results := make([]types.ToolResult, 0, len(keys))
	for _, key := range keys {
		r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), current.Calls[key])
		if err != nil || !r.Success {
			t.Fatalf("public catalog query: %v %s", err, r.Summary)
		}
		results = append(results, r)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	ctx.AgentName, ctx.Stage = types.AgentFinalizer, types.StageFinalize
	return ctx, results
}

func TestHMC248ResourceMembersActualFinalizer(t *testing.T) {
	ctx, results := resourceSelectionPublicContextHMC248(t)
	if ctx.AttachedHitrace != "" || ctx.AnalysisIR.RequestModel.PerfTrace != nil {
		t.Fatal("test silently gained an attachment")
	}
	before, _ := json.Marshal(answerDocObservationLedger(ctx))
	native, empty := 0, 0
	for _, r := range results {
		for _, record := range r.Observations {
			if p, ok := tool.DecodeTraceResourceStack(record); ok {
				native++
				if p.TargetPID == 202 && p.Status == "available" && p.MatchedEvents == 0 {
					empty++
				}
			}
		}
	}
	if native != 4 || empty != 1 {
		t.Fatalf("native sources missing: native=%d exact empty=%d", native, empty)
	}
	prompt := dependencyObservationMessages(t, ctx)
	for _, want := range []string{"### 已观测资源事件与调用栈", "ImageCache::reserve", "DecodeCache::reserve", "UIFrame::render", "Preview::render", "匹配资源事件=0", "tid=202"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("actual finalizer lost %q", want)
		}
	}
	if !strings.Contains(prompt, "deterministic_runtime_queries=4") || strings.Contains(prompt, "deterministic_runtime_queries=0") {
		t.Error("published native queries contradict the source authority census")
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if !bytes.Equal(before, after) {
		t.Fatal("display mutated original observations")
	}
}

func resourceSelectionAllRecordHMC248(t *testing.T, results []types.ToolResult) types.ObservationRecord {
	t.Helper()
	for _, result := range results {
		for _, r := range result.Observations {
			if p, ok := tool.DecodeTraceResourceStack(r); ok && p.TargetPID == 0 && p.MatchedEvents == 4 {
				return r
			}
		}
	}
	t.Fatal("missing all-owner source")
	return types.ObservationRecord{}
}

func resourceSelectionRecordPayloadHMC248(t *testing.T, r types.ObservationRecord, p tracequery.ResourceStackResult) types.ObservationRecord {
	t.Helper()
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	r.RichNotes = []string{types.TraceNoteKeyResourceStack + "=" + string(data)}
	if _, ok := tool.DecodeTraceResourceStack(r); !ok {
		t.Fatal("fixture must preserve the native decoding contract")
	}
	return r
}

func TestHMC248ResourceMembersOwnerProjection(t *testing.T) {
	ctx, results := resourceSelectionPublicContextHMC248(t)
	r := resourceSelectionAllRecordHMC248(t, results)
	original, _ := tool.DecodeTraceResourceStack(r)
	for _, tc := range []struct {
		name   string
		target types.RuntimeTarget
		want   int
	}{
		{"thread_101", types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, PID: 101, Source: "user_explicit"}, 3},
		{"thread_202", types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, Thread: "202", Source: "user_explicit"}, 1},
		{"name_only", types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, Thread: "gallery-main", Source: "user_explicit"}, 3},
		{"not_seen", types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, PID: 303, Source: "user_explicit"}, 0},
		{"unknown_kind", types.RuntimeTarget{PID: 101, Source: "user_explicit"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := ctx.ShallowClone()
			ir := *ctx.AnalysisIR
			copy.AnalysisIR = &ir
			copy.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{tc.target}
			ledger := types.ObservationLedger{Records: []types.ObservationRecord{r}}
			view := renderAnswerDocScopedResourceStacks(copy, ledger, ledger)
			if got := strings.Count(view.text, "源事件物理rowid="); got != tc.want {
				t.Fatalf("display rows=%d want=%d: %s", got, tc.want, view.text)
			}
			if tc.want > 0 {
				if !strings.Contains(view.text, "原查询匹配事件=4") || !strings.Contains(view.text, "原交接省略事件=0") {
					t.Error("recast an all-owner query as a target census")
				}
				if tc.want == 3 && strings.Contains(view.text, "tid=202") || tc.want == 1 && strings.Contains(view.text, "tid=101") {
					t.Error("unrequested owner leaked")
				}
			}
			if got, _ := json.Marshal(original); !bytes.Equal(got, func() []byte { p, _ := tool.DecodeTraceResourceStack(r); b, _ := json.Marshal(p); return b }()) {
				t.Error("projection changed source payload")
			}
			// Even an other-observed-value profile must not restore the unfiltered
			// aggregate as a generic compact/RichNotes row.
			profile := *copy.AnalysisIR.RequestModel.RuntimeQuestionProfile
			profile.FactFamilies = []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactOtherObservedValue}
			copy.AnalysisIR.RequestModel.RuntimeQuestionProfile = &profile
			if got := answerDocScopeProjectedObservationRecords(copy, ledger.Records); len(got) != 0 {
				t.Fatal("raw resource aggregate re-entered generic prose rows")
			}
		})
	}
	// Same numeric TID/TGID is not interchangeable. Use distinct producer-owned
	// process identity while retaining the original event and frame carriers.
	p := original
	p.Events = append([]tracequery.ResourceStackEvent(nil), p.Events...)
	for i := range p.Events {
		p.Events[i].Source.PID = 700
	}
	processRecord := resourceSelectionRecordPayloadHMC248(t, r, p)
	for _, tc := range []struct {
		kind types.RuntimeTargetKind
		pid  int
		want int
	}{
		{types.RuntimeTargetKindProcess, 101, 0}, {types.RuntimeTargetKindThread, 700, 0}, {types.RuntimeTargetKindProcess, 700, 4},
	} {
		copy := ctx.ShallowClone()
		ir := *ctx.AnalysisIR
		copy.AnalysisIR = &ir
		copy.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: tc.kind, PID: tc.pid, Source: "user_explicit"}}
		ledger := types.ObservationLedger{Records: []types.ObservationRecord{processRecord}}
		if got := strings.Count(renderAnswerDocScopedResourceStacks(copy, ledger, ledger).text, "源事件物理rowid="); got != tc.want {
			t.Errorf("kind=%s pid=%d rows=%d want=%d", tc.kind, tc.pid, got, tc.want)
		}
	}
}

func TestHMC248ResourceMembersEmptyPartialAndLegacy(t *testing.T) {
	ctx, results := resourceSelectionPublicContextHMC248(t)
	r := resourceSelectionAllRecordHMC248(t, results)
	p, _ := tool.DecodeTraceResourceStack(r)
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 202, Source: "user_explicit"}}
	for _, name := range []string{"exact_empty", "exact_unavailable", "all_empty", "retained_other_partial", "retained_target_partial"} {
		t.Run(name, func(t *testing.T) {
			copyP := p
			copyR := r
			switch name {
			case "exact_empty", "exact_unavailable", "all_empty":
				copyP.MatchedEvents = 0
				copyP.OmittedEvents = 0
				copyP.Events = nil
				if name != "all_empty" {
					copyP.TargetPID = 202
					copyR.SourceRef.QueryTargetPID = 202
				}
				if name == "exact_unavailable" {
					copyP.Status = "unavailable"
					copyP.Reason = "no_resource_stack_observations"
					copyR.Object = "unavailable"
				}
			default:
				copyP.Events = nil
				for _, e := range p.Events {
					if (name == "retained_target_partial") == (e.Source.TID == 202) {
						copyP.Events = append(copyP.Events, e)
					}
				}
				copyP.OmittedEvents = p.MatchedEvents - len(copyP.Events)
			}
			copyR = resourceSelectionRecordPayloadHMC248(t, copyR, copyP)
			ledger := types.ObservationLedger{Records: []types.ObservationRecord{copyR}}
			view := renderAnswerDocScopedResourceStacks(ctx, ledger, ledger)
			want := name != "all_empty" && name != "retained_other_partial"
			if (view.text != "") != want {
				t.Fatalf("visible=%t want=%t", view.text != "", want)
			}
			if name == "exact_empty" && (!strings.Contains(view.text, "id=202") || !strings.Contains(view.text, "匹配资源事件=0")) {
				t.Fatal("exact empty lost its query identity")
			}
			if name == "exact_unavailable" && !strings.Contains(view.text, "不是没有资源活动") {
				t.Fatal("unavailable promoted to zero activity")
			}
			if name == "retained_target_partial" && (!strings.Contains(view.text, "原查询匹配事件=4") || !strings.Contains(view.text, "原交接省略事件=3")) {
				t.Fatal("partial source census lost")
			}
		})
	}
	// Unnamed and causal diagnosis preserve the original display lane.
	for _, causal := range []bool{false, true} {
		copy := ctx.ShallowClone()
		ir := *ctx.AnalysisIR
		copy.AnalysisIR = &ir
		if causal {
			profile := *ir.RequestModel.RuntimeQuestionProfile
			profile.Scope = types.RuntimeQuestionScopeCausalDiagnosis
			ir.RequestModel.RuntimeQuestionProfile = &profile
		} else {
			ir.RequestModel.RuntimeTargets = nil
		}
		ledger := types.ObservationLedger{Records: []types.ObservationRecord{r}}
		if got := renderAnswerDocScopedResourceStacks(copy, ledger, ledger).text; got != renderAnswerDocResourceStacks(copy, ledger) || strings.Count(got, "源事件物理rowid=") != 4 {
			t.Fatal("legacy/causal resource display changed")
		}
	}
}

func TestHMC248ResourceMembersSourceWindowAndBudget(t *testing.T) {
	ctx, results := resourceSelectionPublicContextHMC248(t)
	r := resourceSelectionAllRecordHMC248(t, results)
	start, end := 10.0, 10.05
	window := types.RuntimeArtifactTimeWindow{TimeStart: &start, TimeEnd: &end, SourceQuote: "10.000 到 10.050 秒"}
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
		Comparison: &types.RuntimeArtifactComparisonScope{Baseline: types.RuntimeArtifactComparisonSide{Path: filepath.Join(ctx.RepoRoot, "captures/capture.data"), RuntimeArtifactTimeWindow: window}, Current: types.RuntimeArtifactComparisonSide{Path: filepath.Join(ctx.RepoRoot, "captures/nested/capture.data"), RuntimeArtifactTimeWindow: window}}}
	ledger := answerDocObservationLedger(ctx)
	view := renderAnswerDocScopedResourceStacks(ctx, ledger, types.ObservationLedger{})
	if len(view.published) != 4 {
		t.Fatalf("current original-source scope lost: queries=%d", len(view.published))
	}
	for _, name := range []string{"source", "clock", "offset", "window", "payload", "JSON"} {
		t.Run(name, func(t *testing.T) {
			copy := ledger
			copy.Records = nil
			for _, record := range ledger.Records {
				p, ok := tool.DecodeTraceResourceStack(record)
				if !ok {
					continue
				}
				switch name {
				case "source":
					record.SourceRef.Path += ".foreign"
					p.SourcePath = record.SourceRef.Path
					record = resourceSelectionRecordPayloadHMC248(t, record, p)
				case "clock":
					record.SourceRef.TimeDomain = "different-clock"
				case "offset":
					offset := 1.0
					record.SourceRef.ClockOffsetSec = &offset
				case "window":
					record.SourceRef.QueryWindowEndTs = 10.06
					record.Span.EndTs = 10.06
					p.Window.EndTs = 10.06
					record = resourceSelectionRecordPayloadHMC248(t, record, p)
				case "payload":
					record.SourceRef.PayloadRef += ".foreign"
				}
				copy.Records = append(copy.Records, record)
			}
			if name == "JSON" {
				data, _ := json.Marshal(copy)
				copy = types.ObservationLedger{}
				if err := json.Unmarshal(data, &copy); err != nil {
					t.Fatal(err)
				}
			}
			if got := renderAnswerDocScopedResourceStacks(ctx, copy, types.ObservationLedger{}); got.text != "" || len(got.published) != 0 {
				t.Fatalf("%s borrowed original source/window scope", name)
			}
		})
	}
	// Five individually eligible queries exceed the unchanged four-query cap.
	// Only actually published queries participate in the source-authority census.
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "10..10.05"}
	p, _ := tool.DecodeTraceResourceStack(r)
	budget := types.ObservationLedger{}
	for i := 0; i < 5; i++ {
		copy := r
		copy.ID = fmt.Sprintf("query-%d", i)
		copy.SourceRef.QueryScopeID = fmt.Sprintf("scope-%d", i)
		budget.Records = append(budget.Records, copy)
	}
	view = renderAnswerDocScopedResourceStacks(ctx, budget, types.ObservationLedger{})
	if len(view.published) != 4 || !strings.Contains(view.text, "另省略查询=1") || !strings.Contains(renderAnswerDocRuntimeSourceAuthority(ctx, view.authorityLedger(types.ObservationLedger{})), "deterministic_runtime_queries=4") {
		t.Fatal("query cap or budget-admitted authority drifted")
	}
	// Existing byte budget still drops whole queries, not complete symbol tails.
	for i := range p.Events {
		for j := range p.Events[i].Frames {
			f := &p.Events[i].Frames[j]
			if f.Symbol.Status == "known" {
				f.Symbol.Value = strings.Repeat("S", 2048) + "::WHOLE"
			}
			if f.Library.Status == "known" {
				f.Library.Value = strings.Repeat("L", 2048) + "/WHOLE"
			}
		}
	}
	for i := range budget.Records {
		budget.Records[i] = resourceSelectionRecordPayloadHMC248(t, budget.Records[i], p)
	}
	view = renderAnswerDocScopedResourceStacks(ctx, budget, types.ObservationLedger{})
	if len(view.text) > 64<<10 || len(view.published) == 0 || len(view.published) >= 4 || !strings.Contains(view.text, strings.Repeat("S", 2048)+"::WHOLE") {
		t.Fatalf("byte budget did not preserve whole queries: bytes=%d queries=%d", len(view.text), len(view.published))
	}
	if !strings.Contains(renderAnswerDocRuntimeSourceAuthority(ctx, view.authorityLedger(types.ObservationLedger{})), fmt.Sprintf("deterministic_runtime_queries=%d", len(view.published))) {
		t.Fatal("omitted queries still counted as published")
	}
}

func TestHMC248ResourceMembersActualPromptDoesNotLeakUnselectedOwner(t *testing.T) {
	ctx, results := resourceSelectionPublicContextHMC248(t)
	r := resourceSelectionAllRecordHMC248(t, results)
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 101, Source: "user_explicit"}}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: []types.ObservationRecord{r}}}})
	prompt := dependencyObservationMessages(t, ctx)
	if !strings.Contains(prompt, "ImageCache::reserve") || !strings.Contains(prompt, "原查询匹配事件=4") || !strings.Contains(prompt, "展示成员=3") {
		t.Fatal("actual member projection absent")
	}
	if strings.Contains(prompt, "线程=background") || strings.Contains(prompt, `"tid":202`) || strings.Contains(prompt, "tid=202，pid=202") {
		t.Fatal("unselected owner leaked through a generic row or raw carrier")
	}
	if !strings.Contains(prompt, "deterministic_runtime_queries=1") {
		t.Fatal("actual source authority omitted displayed query")
	}
}
