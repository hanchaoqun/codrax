package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc320BusinessFinalizer(t *testing.T) (*types.AgentContext, string) {
	t.Helper()
	raw, err := os.ReadFile("../../eval/fixtures/hmosperf_marker_tree/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "events.systrace")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	start, end := 5.0, 5.012
	rm := types.RequestModel{Language: "zh", PerfTrace: &types.PerfBundle{},
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet,
			FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration, types.RuntimeQuestionFactTargetSchedulerState}},
		// HMC319's first submitted model was rejected for a non-user 900
		// target. Its accepted named-target request contains only 700.
		RuntimeTargetProfile:        &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "business-700"},
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 700, Thread: "business-700", Source: "user_explicit"}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "5.000..5.012"}}
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: t.TempDir(), Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("Explain the observed business accounts within the selected window"), AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	ctx.Mutable.SetRequestModel(rm)
	return ctx, path
}

func hmc320BusinessQuery(t *testing.T, ctx *types.AgentContext, path string, selectors map[string]any) types.ToolResult {
	t.Helper()
	params := map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": 5.0, "time_end": 5.012}
	for k, v := range selectors {
		params[k] = v
	}
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
	if err != nil || !result.Success {
		t.Fatalf("public query %s: %v %+v", raw, err, result)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
	return result
}

func TestHMC320BusinessPublicationsAreNotPhysicalInstances(t *testing.T) {
	ctx, path := hmc320BusinessFinalizer(t)
	for _, selectors := range []map[string]any{nil, {"pid": 700}, {"thread": "business-700"}} {
		hmc320BusinessQuery(t, ctx, path, selectors)
	}
	ledger := answerDocObservationLedger(ctx)
	queries, instances := map[string]bool{}, map[string]bool{}
	var rows []types.ObservationRecord
	for _, r := range ledger.Records {
		if f, ok := tool.DecodeTraceBusinessTreeFact(r); ok && f.Node.Thread.PID == 700 {
			rows = append(rows, r)
			queries[r.SourceRef.QueryScopeID] = true
			instances[f.Node.ID] = true
		}
	}
	if len(rows) != 9 || len(queries) != 3 || len(instances) != 3 {
		t.Fatalf("public prerequisites: publication rows=%d, independent queries=%d, native instances=%d", len(rows), len(queries), len(instances))
	}
	before, _ := json.Marshal(ledger)
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	t.Logf("native rows=%d, queries=%d, native instances=%d\n%s", len(rows), len(queries), len(instances), renderAnswerDocBusinessTreeFacts(ctx, types.ObservationLedger{Records: rows}))
	if !strings.Contains(prompt, "最终上下文独立展示 9 条查询发布记录、另省略 0") || strings.Contains(prompt, "最终上下文独立展示 9 个实例") {
		t.Error("public finalizer confuses independent publication count with physical instance count")
	}
	for _, r := range rows {
		if !strings.Contains(prompt, "observation_id="+quotedTreeValue(r.ID)) || !strings.Contains(prompt, "query_scope="+quotedTreeValue(r.SourceRef.QueryScopeID)) {
			t.Errorf("independent publication was erased: %s", r.ID)
		}
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if string(before) != string(after) {
		t.Fatal("display changed the source ledger")
	}
}

func TestHMC320PublishedSpanKeepsSelfDurationWhenStatesUnknown(t *testing.T) {
	ctx, path := hmc320BusinessFinalizer(t)
	hmc320BusinessQuery(t, ctx, path, map[string]any{"pid": 700})
	hmc320BusinessQuery(t, ctx, path, map[string]any{"view": "thread_timeline", "thread": "background-900"})
	ledger := answerDocObservationLedger(ctx)
	var row types.ObservationRecord
	var fact tool.TraceBusinessTreeFact
	for _, r := range ledger.Records {
		if f, ok := tool.DecodeTraceBusinessTreeFact(r); ok && f.Node.Thread.PID == 900 && r.SourceRef.QueryTargetPID == 700 {
			row, fact = r, f
		}
	}
	if row.ID == "" || fact.Node.Self == nil || fact.Node.Self.DurationMs < 5.499 || fact.Node.Self.DurationMs > 5.501 {
		t.Fatalf("public prerequisite lost independent native self account: %+v", fact)
	}
	want := tool.TraceBusinessTreeNodeText(fact.Node)
	if !strings.Contains(want, "自身耗时{5.5 毫秒") || !strings.Contains(want, "状态未能计量，不按零处理") {
		t.Fatalf("source did not independently carry known self duration and unknown state: %s", want)
	}
	before, _ := json.Marshal(ledger)
	causalBefore, _ := json.Marshal(types.CompileTraceCausalProjectionSet(ledger))
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	t.Logf("published span query=%q native fact=%s\nobservation=%s", row.SourceRef.QueryScopeID, want, row.ID)
	if !strings.Contains(prompt, want) || !strings.Contains(prompt, "observation_id="+quotedTreeValue(row.ID)) {
		t.Error("public finalizer drops self duration of the already-published same-query business interval while retaining its unknown states")
	}
	afterLedger := answerDocObservationLedger(ctx)
	after, _ := json.Marshal(afterLedger)
	causalAfter, _ := json.Marshal(types.CompileTraceCausalProjectionSet(afterLedger))
	if string(before) != string(after) || string(causalBefore) != string(causalAfter) {
		t.Fatal("supplement display changed full observations or causal authority")
	}
}

func hmc320PrincipalLedger(ctx *types.AgentContext, ledger types.ObservationLedger) types.ObservationLedger {
	principal := ledger
	principal.Records, _ = answerDocFinalizerObservationRecords(ctx, ledger.Records)
	principal.Records, _ = answerDocSelectedWindowObservationRecords(ctx, principal.Records)
	return principal
}

func hmc320PublishedBackgroundTable(t *testing.T, publication *answerDocNativePresentationReceipt) types.RuntimeMeasurementTable {
	t.Helper()
	for _, annotation := range publication.businessAnnotations {
		if annotation.Owner.PID != 900 {
			continue
		}
		p, ok := types.DecodeRuntimeMeasurementPublication(annotation.Record)
		if ok && len(p.Tables) == 1 {
			return p.Tables[0]
		}
	}
	t.Fatal("no current native background annotation")
	return types.RuntimeMeasurementTable{}
}

func TestHMC320PublishedAccountGuardBoundaries(t *testing.T) {
	for _, name := range []string{"unpublished", "partial_rows", "partial_notes", "different_table_same_selector", "json_ledger", "missing_tree_member", "missing_span_member", "same_id_changed_account", "same_name_other_query", "same_name_other_source", "query_target_changed", "query_window_changed", "source_generation_changed", "request_window_changed", "detached_annotation_owner_changed"} {
		t.Run(name, func(t *testing.T) {
			ctx, path := hmc320BusinessFinalizer(t)
			hmc320BusinessQuery(t, ctx, path, map[string]any{"pid": 700})
			ledger := answerDocObservationLedger(ctx)
			principal := hmc320PrincipalLedger(ctx, ledger)
			publication := &answerDocNativePresentationReceipt{}
			renderAnswerDocRuntimeMeasurementChoices(ctx, publication)
			if text := renderAnswerDocPublishedBusinessAccounts(ctx, ledger, principal, publication); !strings.Contains(text, "自身耗时{5.5 毫秒") {
				t.Fatalf("public positive prerequisite: %s", text)
			}
			switch name {
			case "unpublished":
				publication = &answerDocNativePresentationReceipt{businessAnnotations: publication.businessAnnotations}
			case "partial_rows", "partial_notes":
				table := hmc320PublishedBackgroundTable(t, publication)
				publication.observeMeasurementViewDetails(table, name != "partial_rows", name != "partial_notes")
			case "different_table_same_selector":
				table := hmc320PublishedBackgroundTable(t, publication)
				table.Rows[0][1] = "different native content"
				publication.measurementViews, publication.measurementViewDetails = nil, nil
				publication.observeMeasurementView(table, true)
			case "json_ledger":
				raw, _ := json.Marshal(ledger)
				ledger = types.ObservationLedger{}
				if err := json.Unmarshal(raw, &ledger); err != nil {
					t.Fatal(err)
				}
			case "missing_tree_member", "missing_span_member":
				predicate := types.TraceBusinessTreePredicate
				if name == "missing_span_member" {
					predicate = types.TraceBusinessSpanPredicate
				}
				removed := false
				var keep []types.ObservationRecord
				for _, r := range ledger.Records {
					if !removed && r.Predicate == predicate && r.Object == "DecodeRecord" {
						removed = true
						continue
					}
					keep = append(keep, r)
				}
				if !removed {
					t.Fatal("missing unrelated node prerequisite")
				}
				ledger.Records = keep
			case "same_id_changed_account":
				for i, r := range ledger.Records {
					if r.Predicate == types.TraceBusinessTreePredicate && r.Subject == "background-900" {
						ledger.Records[i].Summary += " altered after producer seal"
					}
				}
			case "same_name_other_query", "same_name_other_source":
				queryPath := path
				if name == "same_name_other_source" {
					raw, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					queryPath = filepath.Join(t.TempDir(), "same-bytes.systrace")
					if err := os.WriteFile(queryPath, raw, 0600); err != nil {
						t.Fatal(err)
					}
				}
				// The published span still belongs to the first query. Keep
				// only the second query's same-name tree to deny borrowing.
				hmc320BusinessQuery(t, ctx, queryPath, map[string]any{"thread": "business-700"})
				ledger = answerDocObservationLedger(ctx)
				var keep []types.ObservationRecord
				for _, r := range ledger.Records {
					if r.Predicate == types.TraceBusinessTreePredicate && r.SourceRef.QueryTargetThread == "" {
						continue
					}
					keep = append(keep, r)
				}
				ledger.Records = keep
			case "query_target_changed", "query_window_changed":
				for i, r := range ledger.Records {
					if r.Predicate != types.TraceBusinessTreePredicate && r.Predicate != types.TraceBusinessSpanPredicate {
						continue
					}
					if name == "query_target_changed" {
						ledger.Records[i].SourceRef.QueryTargetPID = 900
					} else {
						ledger.Records[i].SourceRef.QueryWindowEndTs = 5.013
					}
				}
			case "source_generation_changed":
				f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					t.Fatal(err)
				}
				_, writeErr := f.WriteString("\n# new source generation\n")
				closeErr := f.Close()
				if writeErr != nil || closeErr != nil {
					t.Fatalf("change generation: %v %v", writeErr, closeErr)
				}
			case "request_window_changed":
				end := 5.008
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeEnd = &end
			case "detached_annotation_owner_changed":
				for i := range publication.businessAnnotations {
					if publication.businessAnnotations[i].Owner.PID == 900 {
						publication.businessAnnotations[i].Owner.PID = 700
					}
				}
			}
			if text := renderAnswerDocPublishedBusinessAccounts(ctx, ledger, principal, publication); text != "" {
				t.Fatalf("invalid or unpublished account acquired field handoff: %s", text)
			}
		})
	}
}

func TestHMC320PublishedAccountDoesNotNeedPrincipalRows(t *testing.T) {
	ctx, path := hmc320BusinessFinalizer(t)
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 42, Thread: "other-42", Source: "user_explicit"}}
	ctx.AnalysisIR.RequestModel.RuntimeTargetProfile.SourceQuote = "other-42"
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	hmc320BusinessQuery(t, ctx, path, map[string]any{"pid": 700})
	ledger := answerDocObservationLedger(ctx)
	principal := hmc320PrincipalLedger(ctx, ledger)
	if len(principal.Records) != 0 {
		t.Fatalf("public empty-principal prerequisite retained %d rows", len(principal.Records))
	}
	text := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(text, "自身耗时{5.5 毫秒") || !strings.Contains(text, "背景事实，不改变主分析目标") {
		t.Fatal("already-published fields disappeared with an empty principal projection")
	}
	if strings.Contains(text, "最终上下文独立展示") {
		t.Fatal("field completion was counted as principal publication or new instances")
	}
}

func TestHMC320SameNameInstancesKeepExactPublishedAccounts(t *testing.T) {
	ctx, path := hmc320BusinessFinalizer(t)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// A second closed instance on the same physical thread has a different
	// start/end pair. Equal business labels cannot replace that native pair.
	needle := "business-700 (600) [003] .... 5.010000:"
	insert := "background-900 (900) [004] .... 5.009100: tracing_mark_write: B|900|OpenDocument\n" +
		"background-900 (900) [004] .... 5.009700: tracing_mark_write: E|900\n"
	if strings.Count(string(raw), needle) != 1 {
		t.Fatal("native fixture insertion point changed")
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(raw), needle, insert+needle, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	hmc320BusinessQuery(t, ctx, path, map[string]any{"pid": 700})
	ledger := answerDocObservationLedger(ctx)
	var records []types.ObservationRecord
	for _, r := range ledger.Records {
		if f, ok := tool.DecodeTraceBusinessTreeFact(r); ok && f.Node.Thread.PID == 900 {
			records = append(records, r)
		}
	}
	if len(records) != 2 || records[0].ID == records[1].ID || records[0].Span.LineStart == records[1].Span.LineStart {
		t.Fatal("two physical same-name instances were not independently observed")
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, r := range records {
		f, _ := tool.DecodeTraceBusinessTreeFact(r)
		if !strings.Contains(prompt, tool.TraceBusinessTreeNodeText(f.Node)) || !strings.Contains(prompt, "observation_id="+quotedTreeValue(r.ID)) {
			t.Errorf("same-name instance lost its exact account: %s", r.ID)
		}
	}
	if !strings.Contains(prompt, "自身耗时{5.5 毫秒") || !strings.Contains(prompt, "自身耗时{0.6 毫秒") {
		t.Fatal("different physical pairs borrowed the other instance's self duration")
	}
}
