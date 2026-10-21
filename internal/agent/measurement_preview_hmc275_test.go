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

func hmc275IOContext(t *testing.T, paths []string, params []map[string]any) (*types.AgentContext, []types.ToolResult) {
	t.Helper()
	start, end := 1.0, 1.01
	rm := types.RequestModel{Language: "zh", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{},
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "[1,1.01) 秒"}}
	dir := t.TempDir()
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("列出这个窗口内各层请求的在途数量及变化"), AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}}}
	ctx.Mutable.SetRequestModel(rm)
	for _, path := range paths {
		ctx.RuntimeArtifactPreflight.Artifacts = append(ctx.RuntimeArtifactPreflight.Artifacts, types.RuntimeArtifactPreflightArtifact{Kind: "trace", Source: path, Carrier: "path"})
	}
	var results []types.ToolResult
	for i, args := range params {
		args["source"], args["path"], args["view"] = "path", paths[i], "window_stats"
		raw, _ := json.Marshal(args)
		out, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
		if err != nil || !out.Success {
			t.Fatalf("public query %d: %v / %s", i, err, out.Summary)
		}
		results = append(results, types.AttachToolHandoffCarrier(out))
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results, HandoffCarriers: types.ToolHandoffCarriersFromTurnAInputs(results, nil, nil)})
	return ctxbuilder.BuildAgentContext(types.ToolBusContext(ctx, types.AgentFinalizer), types.AgentFinalizer, types.StageFinalize), results
}

func hmc275IOPath(t *testing.T) string {
	t.Helper()
	source, err := os.ReadFile("../../eval/fixtures/hmosperf_io_inflight/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "capture.systrace")
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func hmc275MessagePreviews(t *testing.T, messages string) map[string]measurementHandoffPreview {
	t.Helper()
	out := map[string]measurementHandoffPreview{}
	for _, line := range strings.Split(messages, "\n") {
		prefix, data, found := strings.Cut(line, "; preview=")
		if !found {
			continue
		}
		var p measurementHandoffPreview
		if err := json.Unmarshal([]byte(data), &p); err != nil {
			t.Fatal(err)
		}
		out[prefix] = p
	}
	return out
}

func TestHMC275IOTimelineActualFinalizerBoundaryAndReuse(t *testing.T) {
	path := hmc275IOPath(t)
	ctx, results := hmc275IOContext(t, []string{path, path}, []map[string]any{{"time_start": 1.0, "time_end": 1.01}, {}})
	before, _ := json.Marshal(results)
	if !strings.Contains(results[1].Summary, "trace_query_request_window_inherited=") {
		t.Fatal("public alias fixture did not exercise the same admitted inherited window")
	}
	projection := hmc218CausalProjection(t, ctx)
	choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
	messages := dependencyObservationMessages(t, ctx)
	previews := hmc275MessagePreviews(t, messages)
	rows, timelines, boundarySamples, writePreviews := 0, 0, 0, 0
	for _, p := range previews {
		rows += len(p.Rows)
	}
	if rows > 128 {
		t.Fatalf("shared row budget expanded: %d", rows)
	}
	for _, table := range choices {
		if table.View != types.RuntimeMeasurementTimeline || !strings.Contains(table.ObservationID, "#io_inflight:") {
			continue
		}
		if table.DefaultPresentation {
			t.Fatal("fixture no longer exercises non-default temporal views")
		}
		timelines++
		selector := fmt.Sprintf("observation_id=%q view=%q", table.ObservationID, table.View)
		for header, p := range previews {
			if !strings.Contains(header, selector) {
				continue
			}
			if len(p.Rows) != len(p.SourceRowIndices) || p.RowIndexBase == nil || *p.RowIndexBase != 0 {
				t.Errorf("actual Finalizer lost typed timeline source indices: %s %+v", selector, p)
			}
			for i, index := range p.SourceRowIndices {
				if index < 0 || index >= len(table.Rows) || !reflect.DeepEqual(table.Rows[index], p.Rows[i]) || i > 0 && p.SourceRowIndices[i-1] >= index {
					t.Fatal("preview changed producer rows or ordering")
				}
			}
			if len(p.Rows) >= 2 {
				boundarySamples++
				if !reflect.DeepEqual(p.Rows[0], table.Rows[0]) || !reflect.DeepEqual(p.Rows[len(p.Rows)-1], table.Rows[len(table.Rows)-1]) {
					t.Errorf("timeline tail missing: %s: %+v", selector, p)
				}
			}
			if strings.HasSuffix(table.ObservationID, "#io_inflight:3") {
				writePreviews++
				if len(table.Rows) != 5 || len(p.Rows) < 2 || !reflect.DeepEqual(p.Rows[len(p.Rows)-1], []string{"1.009000", "1.010000", "0"}) {
					t.Errorf("freed duplicate budget still loses final idle millisecond: %+v", p)
				}
			}
		}
	}
	if timelines != 8 || boundarySamples == 0 {
		t.Errorf("public pair did not retain two independent query rosters: timelines=%d boundarySamples=%d", timelines, boundarySamples)
	}
	if !strings.Contains(messages, "native_table_reuse") {
		t.Error("identical current-source previews still consume the shared budget twice")
	}
	if writePreviews != 1 {
		t.Errorf("identical write timeline must be supplied once, got %d", writePreviews)
	}
	// Every alias still selects its complete original table, not sampled rows
	// and not a different query's selector. Default output intent is unchanged.
	blocks := []any{map[string]any{"id": "summary", "kind": "summary", "text": "以下按各原生查询分别列出请求在途时序，不将请求驻留当成线程等待。"}}
	for _, table := range choices {
		if table.View == types.RuntimeMeasurementTimeline && strings.Contains(table.ObservationID, "#io_inflight:") {
			blocks = append(blocks, map[string]any{"id": fmt.Sprintf("timeline-%d", len(blocks)), "kind": "table", "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}})
		}
	}
	args, _ := json.Marshal(map[string]any{"blocks": blocks})
	out, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), args)
	if err != nil || !out.Success {
		t.Fatalf("full source-bound alias output: %v / %s", err, out.Summary)
	}
	bound := 0
	for _, block := range ctx.Mutable.AnswerDocumentV2().Blocks {
		if block.RuntimeMeasurement == nil {
			continue
		}
		for _, table := range choices {
			if table.ObservationID == block.RuntimeMeasurement.ObservationID && table.View == block.RuntimeMeasurement.View {
				if !block.RuntimeMeasurement.IsBound() || !reflect.DeepEqual(block.RuntimeMeasurement.BoundTable.Rows, table.Rows) {
					t.Fatal("output used preview instead of original producer rows")
				}
				bound++
			}
		}
	}
	if bound != 8 {
		t.Fatalf("alias suppressed an independent explicit output selector: %d", bound)
	}
	after, _ := json.Marshal(results)
	if !bytes.Equal(before, after) || !bytes.Equal(projection, hmc218CausalProjection(t, ctx)) {
		t.Fatal("presentation changed original results or Trace causal projection")
	}
}

func TestHMC275PublicPreviewSourceAndQueryBoundaries(t *testing.T) {
	for _, mode := range []string{"different_source_same_bytes", "different_window", "different_filter", "json_restored", "memo", "new_generation"} {
		t.Run(mode, func(t *testing.T) {
			path := hmc275IOPath(t)
			other := path
			args := map[string]any{"time_start": 1.0, "time_end": 1.01}
			switch mode {
			case "different_source_same_bytes":
				other = hmc275IOPath(t)
			case "different_window":
				args["time_end"] = 1.009
			case "different_filter":
				args = map[string]any{"pattern": "independent-query-filter"}
			default:
				args = map[string]any{}
			}
			ctx, results := hmc275IOContext(t, []string{path, other}, []map[string]any{{"time_start": 1.0, "time_end": 1.01}, args})
			choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
			if len(choices) == 0 {
				t.Fatal("no native table witnesses")
			}
			switch mode {
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
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				replacement := path + ".replacement"
				if err := os.WriteFile(replacement, data, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, path); err != nil {
					t.Fatal(err)
				}
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results, HandoffCarriers: types.ToolHandoffCarriersFromTurnAInputs(results, nil, nil)})
			keys := runtimeMeasurementPreviewIdentities(ctx, choices, &answerDocNativePresentationReceipt{})
			seen := map[string]bool{}
			for _, key := range keys {
				if key == "" {
					continue
				}
				if seen[key] {
					t.Fatal("independent or unheld native source reused a preview")
				}
				seen[key] = true
			}
			if mode == "json_restored" || mode == "memo" || mode == "new_generation" {
				if len(seen) != 0 {
					t.Fatal("unheld physical source retained display equivalence")
				}
			} else if len(seen) < 2 {
				t.Fatal("public negative did not preserve independent current queries")
			}
			messages := dependencyObservationMessages(t, ctx)
			if strings.Contains(messages, "native_table_reuse") {
				t.Fatal("actual Finalizer borrowed another query's preview")
			}
		})
	}
}

func TestHMC275PreviewEquivalenceRequiresEntirePublication(t *testing.T) {
	path := hmc275IOPath(t)
	ctx, results := hmc275IOContext(t, []string{path, path}, []map[string]any{{"time_start": 1.0, "time_end": 1.01}, {}})
	input := types.ObservationLedgerInputFromAgentContext(ctx, 0)
	var original types.RuntimeMeasurementPublication
	var source, predicate string
	for _, record := range results[0].Observations {
		if record.Predicate == "io_inflight" {
			var ok bool
			original, ok = types.DecodeRuntimeMeasurementPublication(record)
			if !ok {
				t.Fatal("invalid public IO publication")
			}
			source, predicate = types.RuntimeObservationPresentationSourceKey(input, results[0], record), record.Predicate
			break
		}
	}
	key := runtimeMeasurementPreviewPublicationKey(source, predicate, original)
	if key == "" {
		t.Fatal("positive public native source is unqualified")
	}
	for _, mode := range []string{"source", "window", "filter", "unit", "member", "notes", "sibling", "ordering", "default", "population", "empty_source"} {
		t.Run(mode, func(t *testing.T) {
			raw, _ := json.Marshal(original)
			var other types.RuntimeMeasurementPublication
			if err := json.Unmarshal(raw, &other); err != nil {
				t.Fatal(err)
			}
			otherSource := source
			switch mode {
			case "source":
				other.Source.Path += ".other"
			case "window":
				other.Source.QueryWindowEndTs += 1
			case "filter":
				other.Source.QueryScopeID += ":filter=independent"
			case "unit":
				other.Tables[0].Columns[0] += " bytes"
			case "member":
				other.Tables[1].MemberSet.RowIDs[0] += "other"
			case "notes":
				other.Tables[0].Notes = append(other.Tables[0].Notes, "independent scope")
			case "sibling":
				other.Tables[0].Rows[0][0] = "999"
			case "ordering":
				other.Tables[1].Rows[0], other.Tables[1].Rows[1] = other.Tables[1].Rows[1], other.Tables[1].Rows[0]
			case "default":
				other.Tables[0].DefaultPresentation = !other.Tables[0].DefaultPresentation
			case "population":
				other.Tables[1].MemberSet.TotalRows++
			case "empty_source":
				otherSource = ""
			}
			if got := runtimeMeasurementPreviewPublicationKey(otherSource, predicate, other); got == key {
				t.Fatal("different complete native publication aliased")
			}
		})
	}
	choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
	keys := runtimeMeasurementPreviewIdentities(ctx, choices, &answerDocNativePresentationReceipt{})
	changed := false
	for i, key := range keys {
		if key == "" {
			continue
		}
		choices[i] = choices[i].Clone()
		choices[i].Notes = append(choices[i].Notes, "request-local scope projection")
		if got := runtimeMeasurementPreviewIdentities(ctx, choices, &answerDocNativePresentationReceipt{})[i]; got != "" {
			t.Fatal("modified contract table borrowed raw publication authority")
		}
		changed = true
		break
	}
	if !changed {
		t.Fatal("no actual contract/publication match")
	}
}

func TestHMC275PreviewAliasPublishedCompletenessAndBudget(t *testing.T) {
	path := hmc275IOPath(t)
	ctx, _ := hmc275IOContext(t, []string{path, path}, []map[string]any{{"time_start": 1.0, "time_end": 1.01}, {}})
	choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
	for _, exhausted := range []bool{false, true} {
		receipt := &answerDocNativePresentationReceipt{}
		if exhausted {
			receipt.navigationBytes = 16 << 10
		}
		text := renderAnswerDocRuntimeMeasurementChoices(ctx, receipt)
		keys := runtimeMeasurementPreviewIdentities(ctx, choices, receipt)
		counts := runtimeMeasurementDistinctPreviewRows(choices, keys)
		seen := map[string]int{}
		aliases, complete, partial, rows := 0, 0, 0, 0
		for i, table := range choices {
			rows += counts[i]
			key := keys[i]
			if key == "" {
				continue
			}
			if prior, found := seen[key]; found {
				aliases++
				if counts[i] != 0 {
					t.Fatal("duplicate preview charged row budget")
				}
				firstComplete := receipt.measurementViews[nativeFactHandoffViewKey(choices[prior])]
				aliasComplete := receipt.measurementViews[nativeFactHandoffViewKey(table)]
				shown := strings.Contains(text, fmt.Sprintf("native_table_reuse label=%q observation_id=%q view=%q:", table.Label, table.ObservationID, table.View))
				if aliasComplete != (firstComplete && shown) {
					t.Fatalf("alias claimed completeness beyond actual representative/navigation: exhausted=%t shown=%t first=%t alias=%t selector=%s/%s", exhausted, shown, firstComplete, aliasComplete, table.ObservationID, table.View)
				}
				if firstComplete {
					complete++
				} else {
					partial++
				}
			} else {
				seen[key] = i
			}
		}
		if aliases == 0 || complete == 0 || partial == 0 || rows > 128 {
			t.Fatalf("inadequate native budget boundary witness: aliases=%d complete=%d partial=%d rows=%d", aliases, complete, partial, rows)
		}
		if exhausted && strings.Contains(text, "native_table_reuse") {
			t.Fatal("alias survived exhausted atomic navigation budget")
		}
	}
}

func TestHMC275PublicPartialTimelineAliasNeverClaimsComplete(t *testing.T) {
	var trace strings.Builder
	trace.WriteString("# tracer: nop\n")
	for round := 0; round < 10; round++ {
		for device := 0; device < 8; device++ {
			start := 1 + float64(round)*.0008 + float64(device)*.00002
			sector := 1000 + round*100 + device*8
			fmt.Fprintf(&trace, "reader-40 (40) [001] .... %.6f: block_rq_issue: 8,%d R 4096 () %d + 8 [reader]\n", start, device, sector)
			fmt.Fprintf(&trace, "irq-80 (2) [001] .... %.6f: block_rq_complete: 8,%d R () %d + 8 [0]\n", start+.00001, device, sector)
		}
	}
	path := filepath.Join(t.TempDir(), "many-requests.systrace")
	if err := os.WriteFile(path, []byte(trace.String()), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, _ := hmc275IOContext(t, []string{path, path}, []map[string]any{{"time_start": 1.0, "time_end": 1.01}, {}})
	choices, _ := runtimeMeasurementHandoffRoster(types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices(), 32)
	receipt := &answerDocNativePresentationReceipt{}
	text := renderAnswerDocRuntimeMeasurementChoices(ctx, receipt)
	keys := runtimeMeasurementPreviewIdentities(ctx, choices, receipt)
	counts := runtimeMeasurementDistinctPreviewRows(choices, keys)
	previews := hmc275MessagePreviews(t, dependencyObservationMessages(t, ctx))
	seen := map[string]int{}
	partialAliases := 0
	for i, table := range choices {
		key := keys[i]
		if key == "" {
			continue
		}
		if prior, found := seen[key]; found {
			if table.View != types.RuntimeMeasurementTimeline || counts[prior] >= len(table.Rows) {
				continue
			}
			shown := strings.Contains(text, fmt.Sprintf("native_table_reuse label=%q observation_id=%q view=%q:", table.Label, table.ObservationID, table.View))
			if !shown {
				continue
			}
			partialAliases++
			if receipt.measurementViews[nativeFactHandoffViewKey(table)] || receipt.measurementViews[nativeFactHandoffViewKey(choices[prior])] {
				t.Fatal("partial representative or alias signed complete")
			}
			selector := fmt.Sprintf("observation_id=%q view=%q", choices[prior].ObservationID, table.View)
			found := false
			for header, p := range previews {
				if !strings.Contains(header, selector) {
					continue
				}
				found = true
				if len(p.Rows) >= len(table.Rows) || len(p.SourceRowIndices) != len(p.Rows) || p.SeriesBoundaryCoverage != "not_claimed" {
					t.Fatal("actual Finalizer lost explicit partial preview status")
				}
				if len(p.Rows) == 1 && p.Selection != "first_retained_row" {
					t.Fatal("one-row preview falsely claims both boundaries")
				}
				if len(p.Rows) > 1 && p.SourceRowIndices[len(p.Rows)-1] != len(table.Rows)-1 {
					t.Fatal("partial preview lost retained tail")
				}
			}
			if !found {
				t.Fatal("alias representative missing from actual Finalizer messages")
			}
		} else {
			seen[key] = i
		}
	}
	if partialAliases == 0 {
		t.Fatal("public dense fixture did not exercise a partial published alias")
	}
}
