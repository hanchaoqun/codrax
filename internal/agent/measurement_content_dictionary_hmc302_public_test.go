package agent

import (
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
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc302MeasurementContext(t *testing.T) (*types.AgentContext, []types.ToolResult) {
	t.Helper()
	data, err := os.ReadFile("../../eval/cases/trace_query_wakeup_background_demotion.case")
	if err != nil {
		t.Fatal(err)
	}
	_, trace, found := strings.Cut(string(data), "HTRACE='")
	if !found {
		t.Fatal("missing trace fixture")
	}
	trace, _, found = strings.Cut(trace, "\n'\n")
	if !found {
		t.Fatal("unterminated trace fixture")
	}
	return hmc302MeasurementContextFromTrace(t, trace)
}

func hmc302MeasurementContextFromTrace(t *testing.T, trace string) (*types.AgentContext, []types.ToolResult) {
	t.Helper()
	return hmc302MeasurementContextFromQueries(t, trace, []float64{2.02}, 0)
}

func hmc302MeasurementContextFromQueries(t *testing.T, trace string, ends []float64, bucket float64) (*types.AgentContext, []types.ToolResult) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.ftrace")
	if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
		t.Fatal(err)
	}
	start, end := 2.0, 2.02
	rm := types.RequestModel{Language: "zh", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{},
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "[2,2.02) 秒"}}
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("说明此时间窗内各线程的状态和并行度"), AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}}}
	ctx.Mutable.SetRequestModel(rm)
	ctx.RuntimeArtifactPreflight.Artifacts = append(ctx.RuntimeArtifactPreflight.Artifacts, types.RuntimeArtifactPreflightArtifact{Kind: "trace", Source: path, Carrier: "path"})
	var results []types.ToolResult
	for _, queryEnd := range ends {
		for _, view := range []string{"window_stats", "wakeup_chain", "root_cause_rank"} {
			params := map[string]any{"source": "path", "path": path, "view": view, "pid": 100, "time_start": start, "time_end": queryEnd, "trace_flavor": "harmony_hitrace"}
			if bucket > 0 {
				params["bucket_ms"] = bucket
			}
			args, _ := json.Marshal(params)
			out, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
			if err != nil || !out.Success {
				t.Fatalf("public %s query: %v / %s", view, err, out.Summary)
			}
			results = append(results, types.AttachToolHandoffCarrier(out))
		}
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results, HandoffCarriers: types.ToolHandoffCarriersFromTurnAInputs(results, nil, nil)})
	return ctxbuilder.BuildAgentContext(types.ToolBusContext(ctx, types.AgentFinalizer), types.AgentFinalizer, types.StageFinalize), results
}

func TestHMC302PublicDistinctQueriesShareOnlyMeasurementContent(t *testing.T) {
	ctx, results := hmc302MeasurementContext(t)
	before, _ := json.Marshal(results)
	projection := hmc218CausalProjection(t, ctx)
	var nativeProjection types.TraceCausalProjectionSet
	if err := json.Unmarshal(projection, &nativeProjection); err != nil || len(nativeProjection.Projections) == 0 {
		t.Fatalf("public fixture must have an actual causal projection: %v", err)
	}
	choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
	var members []types.RuntimeMeasurementTable
	for _, table := range choices {
		if table.View == types.RuntimeMeasurementMembers && strings.HasSuffix(table.ObservationID, "#scheduler_concurrency:1") {
			members = append(members, table)
		}
	}
	if len(members) != 3 || len(members[0].Rows) == 0 {
		t.Fatalf("need three separate real runnable member publications, got %d", len(members))
	}
	for _, table := range members[1:] {
		if table.ObservationID == members[0].ObservationID || !reflect.DeepEqual(table.Rows, members[0].Rows) || !reflect.DeepEqual(table.Notes, members[0].Notes) {
			t.Fatal("fixture must retain distinct queries with byte-identical native member display data")
		}
	}
	messages := dependencyObservationMessages(t, ctx)
	bodyJSON := hmc302Dictionary(t, messages, "native_measurement_content")
	queryJSON := hmc302Dictionary(t, messages, "native_measurement_query")
	noteJSON := hmc302Dictionary(t, messages, "native_measurement_note")
	memberBodies, queryReceipts := map[string]bool{}, map[string]bool{}
	refs := 0
	for _, table := range members {
		selector := fmt.Sprintf("observation_id=%q view=%q", table.ObservationID, table.View)
		found := false
		for _, line := range strings.Split(messages, "\n") {
			if strings.Contains(line, selector) && strings.Contains(line, "; content_ref=") {
				found = true
				refs++
				contentRef := hmc302NavigationField(line, "content_ref")
				queryRef := hmc302NavigationField(line, "query_receipt")
				memberBodies[contentRef], queryReceipts[queryRef] = true, true
				var body struct {
					Preview  measurementHandoffPreview `json:"preview"`
					NoteRefs []string                  `json:"note_refs"`
				}
				if err := json.Unmarshal(bodyJSON[contentRef], &body); err != nil || !reflect.DeepEqual(body.Preview, runtimeMeasurementHandoffPreview(table, len(table.Rows))) {
					t.Fatalf("content dictionary lost native rows, indices, units or complete notes: %s / %v", contentRef, err)
				}
				var notes []string
				for _, ref := range body.NoteRefs {
					var note string
					if err := json.Unmarshal(noteJSON[ref], &note); err != nil {
						t.Fatal(err)
					}
					notes = append(notes, note)
				}
				if !reflect.DeepEqual(notes, table.Notes) {
					t.Fatal("complete note references changed order, text or omissions")
				}
				var query types.ObservationSourceRef
				if err := json.Unmarshal(queryJSON[queryRef], &query); err != nil {
					t.Fatal(err)
				}
				original := false
				for _, result := range results {
					for _, record := range result.Observations {
						if record.ID == table.ObservationID {
							original = reflect.DeepEqual(query, record.SourceRef)
						}
					}
				}
				if !original || !query.QueryWindowKnown || query.QueryWindowStartTs != 2 || query.QueryWindowEndTs != 2.02 || !strings.Contains(line, "; source_title=") {
					t.Fatal("shared content replaced independent source/query/window coordinates")
				}
			}
		}
		if !found {
			t.Errorf("independent query selector has no shared content reference: %s", selector)
		}
		if table.MemberSet != nil {
			raw, _ := json.Marshal(table.MemberSet)
			if !strings.Contains(messages, "  member_set="+string(raw)) {
				t.Fatal("content reuse omitted the selector's exact population")
			}
		}
	}
	if refs != 3 {
		t.Errorf("want three retained query selectors referring to one body, got %d", refs)
	}
	if len(memberBodies) != 1 || len(queryReceipts) != 3 {
		t.Fatalf("expected one body and three independent query receipts, got %d/%d", len(memberBodies), len(queryReceipts))
	}
	// Binding still materializes every explicitly selected complete original,
	// never a dictionary preview or a different query's authority.
	blocks := []any{map[string]any{"id": "summary", "kind": "summary", "text": "以下各查询保留各自线程和时间窗；同值展示不合并查询。"}}
	for i, table := range members {
		blocks = append(blocks, map[string]any{"id": fmt.Sprintf("members-%d", i), "kind": "table", "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}})
	}
	args, _ := json.Marshal(map[string]any{"blocks": blocks})
	out, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), args)
	if err != nil || !out.Success {
		t.Fatalf("full native selectors: %v / %s", err, out.Summary)
	}
	bound := 0
	for _, block := range ctx.Mutable.AnswerDocumentV2().Blocks {
		if block.RuntimeMeasurement == nil {
			continue
		}
		for _, table := range members {
			if block.RuntimeMeasurement.ObservationID == table.ObservationID {
				if !block.RuntimeMeasurement.IsBound() || !reflect.DeepEqual(block.RuntimeMeasurement.BoundTable.Rows, table.Rows) {
					t.Fatal("content reuse changed the independently bound full table")
				}
				bound++
			}
		}
	}
	if bound != 3 {
		t.Fatalf("independent table selectors lost: %d", bound)
	}
	after, _ := json.Marshal(results)
	if !bytes.Equal(before, after) || !bytes.Equal(projection, hmc218CausalProjection(t, ctx)) {
		t.Fatal("display reuse changed original facts or Trace causal projection")
	}
}

func hmc302Dictionary(t *testing.T, messages, name string) map[string]json.RawMessage {
	t.Helper()
	out := map[string]json.RawMessage{}
	for _, line := range strings.Split(messages, "\n") {
		if value, ok := strings.CutPrefix(line, "- "+name+" "); ok {
			ref, raw, valid := strings.Cut(value, "=")
			if !valid || !json.Valid([]byte(raw)) || out[ref] != nil {
				t.Fatalf("ambiguous or invalid dictionary definition: %s", line)
			}
			out[ref] = json.RawMessage(raw)
		}
	}
	return out
}

func hmc302NavigationField(line, name string) string {
	_, tail, found := strings.Cut(line, "; "+name+"=")
	if !found {
		return ""
	}
	value, _, _ := strings.Cut(tail, ";")
	return value
}

func TestHMC302PublicContentSourceBoundaries(t *testing.T) {
	for _, mode := range []string{"different_source_same_bytes", "json_restored", "memo", "new_generation"} {
		t.Run(mode, func(t *testing.T) {
			ctx, results := hmc302MeasurementContext(t)
			choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
			switch mode {
			case "different_source_same_bytes":
				other, otherResults := hmc302MeasurementContext(t)
				ctx.RuntimeArtifactPreflight.Artifacts = append(ctx.RuntimeArtifactPreflight.Artifacts, other.RuntimeArtifactPreflight.Artifacts...)
				results = []types.ToolResult{results[0], otherResults[1]}
			case "json_restored":
				raw, _ := json.Marshal(results)
				results = nil
				if err := json.Unmarshal(raw, &results); err != nil {
					t.Fatal(err)
				}
			case "memo":
				for i := range results {
					results[i].ReusedFromRunMemo = true
				}
			case "new_generation":
				path := ctx.RuntimeArtifactPreflight.Artifacts[0].Source
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				replacement := path + ".replacement"
				if err := os.WriteFile(replacement, raw, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, path); err != nil {
					t.Fatal(err)
				}
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results, HandoffCarriers: types.ToolHandoffCarriersFromTurnAInputs(results, nil, nil)})
			if mode == "different_source_same_bytes" {
				choices = types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
			}
			keys := runtimeMeasurementPreviewIdentities(ctx, choices, &answerDocNativePresentationReceipt{})
			for _, w := range runtimeMeasurementContentWitnesses(ctx, choices, keys) {
				if w.key != "" {
					t.Fatal("unheld or different physical source shared content")
				}
			}
			if text := dependencyObservationMessages(t, ctx); strings.Contains(text, "- native_measurement_content ") {
				t.Fatal("actual finalizer admitted an unqualified content dictionary")
			}
		})
	}
}

func TestHMC302ContentKeyKeepsCompleteNativeCaliber(t *testing.T) {
	ctx, results := hmc302MeasurementContext(t)
	input := types.ObservationLedgerInputFromAgentContext(ctx, 0)
	var p types.RuntimeMeasurementPublication
	var source, predicate string
	for _, record := range results[0].Observations {
		if strings.HasSuffix(record.ID, "#scheduler_concurrency:1") {
			var valid bool
			p, valid = types.DecodeRuntimeMeasurementPublication(record)
			if !valid {
				t.Fatal("invalid public publication")
			}
			source, predicate = types.RuntimeObservationPresentationSourceKey(input, results[0], record), record.Predicate
		}
	}
	var original types.RuntimeMeasurementTable
	for _, table := range p.Tables {
		if table.View == types.RuntimeMeasurementMembers {
			original = table
		}
	}
	key := runtimeMeasurementContentKey(source, predicate, p, original)
	if key == "" {
		t.Fatal("public complete table missing source witness")
	}
	for _, mode := range []string{"notes", "units", "unseen_tail", "order", "population", "default", "target", "window", "physical_source"} {
		t.Run(mode, func(t *testing.T) {
			copy, publication, physical := original.Clone(), p, source
			switch mode {
			case "notes":
				copy.Notes = append(copy.Notes, "independent complete-query denominator")
			case "units":
				copy.Columns[0] += " ms"
			case "unseen_tail":
				copy.Rows[len(copy.Rows)-1][0] += " changed"
			case "order":
				copy.Rows[0], copy.Rows[1] = copy.Rows[1], copy.Rows[0]
			case "population":
				copy.DisplayPopulation = &types.RuntimeMeasurementDisplayPopulation{QueryScopeID: p.Source.QueryScopeID, MatchedRows: len(copy.Rows), OmittedRows: 1}
			case "default":
				copy.DefaultPresentation = !copy.DefaultPresentation
			case "target":
				publication.Source.QueryTargetPID++
			case "window":
				publication.Source.QueryWindowEndTs++
			case "physical_source":
				physical += " other generation"
			}
			if got := runtimeMeasurementContentKey(physical, predicate, publication, copy); got == key {
				t.Fatal("equal preview weakened complete-content or source requirements")
			}
			if mode == "unseen_tail" && !reflect.DeepEqual(runtimeMeasurementHandoffPreview(original, 1), runtimeMeasurementHandoffPreview(copy, 1)) {
				t.Fatal("negative must have equal previews and different full tables")
			}
		})
	}
	choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
	keys := runtimeMeasurementPreviewIdentities(ctx, choices, &answerDocNativePresentationReceipt{})
	witnesses := runtimeMeasurementContentWitnesses(ctx, choices, keys)
	for i, w := range witnesses {
		if w.key != "" {
			choices[i] = choices[i].Clone()
			choices[i].Notes = append(choices[i].Notes, "request-local projection")
			if runtimeMeasurementContentWitnesses(ctx, choices, keys)[i].key != "" {
				t.Fatal("modified contract table borrowed raw native authority")
			}
			return
		}
	}
	t.Fatal("no active public content witness")
}

func TestHMC302PublicDistinctContentBudgetAndReceipts(t *testing.T) {
	var trace strings.Builder
	trace.WriteString("# tracer: nop\napp-100 (100) [001] .... 2.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120\n")
	for i := 0; i < 150; i++ {
		id, ts := 200+i, 2.0001+float64(i)*.00008
		fmt.Fprintf(&trace, "worker-%d (100) [002] .... %.6f: sched_switch: prev_comm=worker prev_pid=%d prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120\n", id, ts, id)
		fmt.Fprintf(&trace, "irq-2 (2) [002] .... %.6f: sched_wakeup: comm=worker pid=%d prio=20 target_cpu=002\n", ts+.00002, id)
		fmt.Fprintf(&trace, "worker-%d (100) [002] .... %.6f: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=%d next_prio=20\n", id, ts+.00004, id)
	}
	trace.WriteString("worker-349 (100) [002] .... 2.020000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001\napp-100 (100) [001] .... 2.020020: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52\n")
	ctx, _ := hmc302MeasurementContextFromQueries(t, trace.String(), []float64{2.017, 2.018, 2.019, 2.02}, .1)
	choices, _ := runtimeMeasurementHandoffRoster(types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices(), 32)
	keys := runtimeMeasurementPreviewIdentities(ctx, choices, &answerDocNativePresentationReceipt{})
	witnesses := runtimeMeasurementContentWitnesses(ctx, choices, keys)
	budgetKeys := append([]string(nil), keys...)
	for i, w := range witnesses {
		if w.key != "" {
			budgetKeys[i] = w.key
		}
	}
	counts := runtimeMeasurementDistinctPreviewRows(choices, budgetKeys)
	charged, distinct, partial := 0, map[string]bool{}, 0
	allocated := map[string]int{}
	for i, w := range witnesses {
		charged += counts[i]
		if w.key != "" {
			if distinct[w.key] && counts[i] != 0 {
				t.Fatal("duplicate content consumed preview rows")
			}
			if !distinct[w.key] {
				allocated[w.key] = counts[i]
			}
			distinct[w.key] = true
			if counts[i] > 0 && counts[i] < len(choices[i].Rows) {
				partial++
			}
		}
	}
	if charged > 128 || partial == 0 {
		t.Fatalf("need bounded partial native previews: rows=%d partial=%d", charged, partial)
	}
	for _, mode := range []string{"normal", "navigation_exhausted", "detail_exhausted"} {
		t.Run(mode, func(t *testing.T) {
			p := &answerDocNativePresentationReceipt{}
			if mode == "navigation_exhausted" {
				p.navigationBytes = nativePresentationNavigationByteLimit
			}
			if mode == "detail_exhausted" {
				p.presentationLimit = 1
			}
			text := renderAnswerDocRuntimeMeasurementChoices(ctx, p)
			bodies := hmc302Dictionary(t, text, "native_measurement_content")
			if mode != "normal" && len(bodies) != 0 {
				t.Fatal("exhausted atomic navigation/detail published a body")
			}
			shared := 0
			for i, w := range witnesses {
				if w.key == "" {
					continue
				}
				shared++
				detail := p.measurementViewPresentation(choices[i])
				if mode != "normal" && (detail.rowsComplete || detail.notesComplete) {
					t.Fatal("unwritten content signed complete")
				}
				selector := fmt.Sprintf("observation_id=%q view=%q", choices[i].ObservationID, choices[i].View)
				shown := false
				for _, line := range strings.Split(text, "\n") {
					if strings.Contains(line, selector) && strings.Contains(line, "; content_ref=") {
						shown = true
					}
				}
				if mode == "normal" && shown && allocated[w.key] < len(choices[i].Rows) {
					if detail.rowsComplete || !detail.notesComplete {
						t.Fatal("partial rows/full notes publication boundaries mixed")
					}
				}
				if !shown && (detail.rowsComplete || detail.notesComplete) {
					t.Fatal("navigation-omitted selector inherited another body's publication")
				}
			}
			if shared == 0 || mode == "normal" && len(bodies) == 0 {
				t.Fatal("public fixture did not exercise shared content within actual budgets")
			}
			actualRows := 0
			for _, raw := range bodies {
				var body struct {
					Preview measurementHandoffPreview `json:"preview"`
				}
				if err := json.Unmarshal(raw, &body); err != nil {
					t.Fatal(err)
				}
				actualRows += len(body.Preview.Rows)
				if len(body.Preview.SourceRowIndices) > 0 && len(body.Preview.Rows) != len(body.Preview.SourceRowIndices) {
					t.Fatal("body lost timeline indices")
				}
			}
			if actualRows > 128 {
				t.Fatalf("dictionary exceeded distinct-body row budget: %d", actualRows)
			}
		})
	}
}

func TestHMC302NativePartialBodyKeepsPublicationBounds(t *testing.T) {
	ctx, _ := hmc302MeasurementContext(t)
	choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
	keys := runtimeMeasurementPreviewIdentities(ctx, choices, &answerDocNativePresentationReceipt{})
	witnesses := runtimeMeasurementContentWitnesses(ctx, choices, keys)
	var indices []int
	for i, table := range choices {
		if table.View == types.RuntimeMeasurementMembers && strings.HasSuffix(table.ObservationID, "#scheduler_concurrency:1") && witnesses[i].key != "" {
			indices = append(indices, i)
		}
	}
	if len(indices) != 3 {
		t.Fatal("missing current native independent member selectors")
	}
	for _, rows := range []int{0, 1} {
		t.Run(fmt.Sprintf("shown_rows_%d", rows), func(t *testing.T) {
			d := runtimeMeasurementContentDictionary{}
			notes := runtimeMeasurementNoteNavigation{}
			p := &answerDocNativePresentationReceipt{}
			var b strings.Builder
			for position, i := range indices {
				allocation := 0
				if position == 0 {
					allocation = rows
				}
				shown, actual, charged := d.write(&b, witnesses[i], choices[i], allocation, "", "", notes, p)
				if !shown || actual != rows || position > 0 && charged != 0 {
					t.Fatal("alias changed or recharged the actual partial preview")
				}
				detail := p.measurementViewPresentation(choices[i])
				if detail.rowsComplete || !detail.notesComplete {
					t.Fatal("partial or zero-row body promoted to complete native data")
				}
			}
			bodies := hmc302Dictionary(t, b.String(), "native_measurement_content")
			if len(bodies) != 1 {
				t.Fatal("partial aliases rewrote or lost the defined body")
			}
			if len(notes) == 0 {
				t.Fatal("complete native notes absent")
			}
		})
	}
	// A rejected first body must not register note IDs which were never sent.
	d, notes, p := runtimeMeasurementContentDictionary{}, runtimeMeasurementNoteNavigation{}, &answerDocNativePresentationReceipt{presentationLimit: 1}
	var b strings.Builder
	i := indices[0]
	if shown, _, _ := d.write(&b, witnesses[i], choices[i], 1, "", "", notes, p); shown || len(notes) != 0 || len(d.bodies) != 0 || len(d.queries) != 0 {
		t.Fatal("failed staged publication committed unseen definitions")
	}
}
