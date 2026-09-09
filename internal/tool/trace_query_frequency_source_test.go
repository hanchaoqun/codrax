package tool

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1631FrequencyTrace(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name, "policy.systrace")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	body := strings.Join([]string{
		`target-42 (42) [004] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=120`,
		`policy-10 (10) [004] .... 1.100000: cpu_frequency_limits: min=558000 max=2270000 cpu_id=4`,
		`policy-10 (10) [004] .... 1.200000: cpu_frequency_limits: min=558000 max=2100000 cpu_id=4`,
		`target-42 (42) [004] .... 1.500000: sched_switch: prev_comm=target prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120`,
		`idle-0 (0) [004] .... 2.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=120`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func b1631TargetRows(result types.ToolResult) []types.ObservationRecord {
	var out []types.ObservationRecord
	for _, record := range result.Observations {
		if record.Predicate == "target_cpu_running" {
			out = append(out, record)
		}
	}
	return out
}

func TestB1631ActualFrequencySourcesSeparateCapturesAndLineQueries(t *testing.T) {
	dir := t.TempDir()
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("frequency source matrix")}
	pathA := b1631FrequencyTrace(t, dir, "capture-A")
	pathB := b1631FrequencyTrace(t, dir, "capture-B")
	results := []types.ToolResult{
		b1631ExecuteFrequency(t, ctx, pathA, nil),
		b1631ExecuteFrequency(t, ctx, pathB, nil),
		b1631ExecuteFrequency(t, ctx, pathA, map[string]any{"line_start": 1, "line_end": 5}),
		b1631ExecuteFrequency(t, ctx, pathA, map[string]any{"line_start": 1, "line_end": 6}),
	}
	for i, result := range results {
		witness := result.TraceEvidenceAuthority.FrequencyLimitWitnesses[0]
		if types.TraceFrequencyLimitSourceKey(witness) == "" {
			t.Fatalf("result %d lacks precise source: %+v", i, witness)
		}
		if len(b1631TargetRows(result)) == 0 {
			t.Fatalf("result %d lacks target CPU control", i)
		}
		for j, other := range results {
			for _, record := range b1631TargetRows(other) {
				if got := types.TraceFrequencyLimitMatchesRecord(witness, record); got != (i == j) {
					t.Fatalf("capture/query/result source join %d→%d=%v", i, j, got)
				}
			}
		}
	}
	for _, order := range [][]int{{0, 1}, {1, 0}} {
		input := []types.TraceFrequencyLimitAuthority{results[order[0]].TraceEvidenceAuthority.FrequencyLimitWitnesses[0], results[order[1]].TraceEvidenceAuthority.FrequencyLimitWitnesses[0]}
		if got := dedupTraceQueryFrequencyLimitAuthorities(input, 8); len(got) != 2 {
			t.Fatalf("different captures collapsed in auto-window merge: %+v", got)
		}
		if got := runtimeTraceDedupFrequencyLimitWitnesses(input, 8); len(got) != 2 {
			t.Fatalf("different captures collapsed in answer roster: %+v", got)
		}
	}
	lineA := results[2].TraceEvidenceAuthority.FrequencyLimitWitnesses[0].SourceRef.QueryScopeID
	lineB := results[3].TraceEvidenceAuthority.FrequencyLimitWitnesses[0].SourceRef.QueryScopeID
	if !strings.HasSuffix(lineA, ":query_lines=1..5") || !strings.HasSuffix(lineB, ":query_lines=1..6") {
		t.Fatalf("actual query line bounds lost: %q / %q", lineA, lineB)
	}
}

func TestB1631ActualAutoWindowChildrenKeepIndependentSource(t *testing.T) {
	dir := t.TempDir()
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	path := b1631FrequencyTrace(t, dir, "capture")
	candidates := []traceQueryAutoWindowCandidate{
		{Rank: 1, Start: 1, End: 2},
		{Rank: 2, Start: 1, End: 2},
		{Rank: 3, Start: 1.15, End: 2},
	}
	result := (&TraceQuery{}).runAutoWindowCandidates(ctx, traceQueryParams{View: "window_stats", PID: 42}, path, "path", "source-regression", candidates, "")
	if !result.Success || result.TraceEvidenceAuthority == nil || len(result.TraceEvidenceAuthority.FrequencyLimitWitnesses) != 3 {
		t.Fatalf("actual child queries must retain three source-specific witnesses: %+v", result)
	}
	witnesses := result.TraceEvidenceAuthority.FrequencyLimitWitnesses
	if witnesses[0].SourceRef.PayloadRef != witnesses[1].SourceRef.PayloadRef || witnesses[0].SourceRef.QueryScopeID == witnesses[1].SourceRef.QueryScopeID {
		t.Fatalf("same-parent payload must not erase distinct children: %+v", witnesses)
	}
	for _, witness := range witnesses {
		matches := 0
		for _, record := range b1631TargetRows(result) {
			if types.TraceFrequencyLimitMatchesRecord(witness, record) {
				matches++
			}
		}
		if matches != 1 {
			t.Fatalf("one policy child must match exactly its one CPU roster row, got %d: %+v", matches, witness)
		}
	}
}

func TestB1631FrequencySourceMemoJSONAndLegacyBoundary(t *testing.T) {
	dir := t.TempDir()
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("frequency memo")}
	path := b1631FrequencyTrace(t, dir, "capture")
	first := b1631ExecuteFrequency(t, ctx, path, nil)
	second := b1631ExecuteFrequency(t, ctx, path, nil)
	if !second.ReusedFromRunMemo || !reflect.DeepEqual(first.TraceEvidenceAuthority, second.TraceEvidenceAuthority) || !reflect.DeepEqual(first.Observations, second.Observations) {
		t.Fatal("memo reuse changed the original source receipt or facts")
	}
	raw, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	var decoded types.ToolResult
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.TraceEvidenceAuthority, decoded.TraceEvidenceAuthority) {
		t.Fatal("JSON transport dropped frequency source")
	}
	w := first.TraceEvidenceAuthority.FrequencyLimitWitnesses[0]
	for _, record := range b1631TargetRows(decoded) {
		if !types.TraceFrequencyLimitMatchesRecord(w, record) {
			t.Fatal("JSON transport broke same-result target matching")
		}
	}
	engine := tracequery.Result{View: "window_stats", SourcePath: path, TimeStart: 1, TimeEnd: 2, WindowStats: &tracequery.WindowStats{CPUFrequencyLimits: []tracequery.CPUFrequencyLimit{{CPU: 4, MinFrequency: 558000, MaxFrequency: 2100000, Count: 2, Line: 3, Ts: 1.2}}}}
	legacy := traceQueryEvidenceAuthority(engine).FrequencyLimitWitnesses[0]
	noReceipt := traceQueryEvidenceAuthorityWithSource(engine, "path", "", "", "", time.Unix(1, 0), tracequery.Query{}).FrequencyLimitWitnesses[0]
	for _, unknown := range []types.TraceFrequencyLimitAuthority{legacy, noReceipt} {
		if types.TraceFrequencyLimitSourceKey(unknown) != "" || types.TraceFrequencyLimitMatchesRecord(unknown, b1631TargetRows(first)[0]) {
			t.Fatal("unknown source obtained a join credential")
		}
		if len(dedupTraceQueryFrequencyLimitAuthorities([]types.TraceFrequencyLimitAuthority{unknown, unknown}, 8)) != 2 {
			t.Fatal("unknown source was silently treated as a duplicate")
		}
	}
	if legacy.SourceRef != nil || legacy.ObservedAt != "" {
		t.Fatal("summary-only factory minted a receipt")
	}
}

func TestB1631FrequencySourceReaderAndBudget(t *testing.T) {
	dir := t.TempDir()
	path := b1631FrequencyTrace(t, dir, "capture")
	result := b1631ExecuteFrequency(t, &types.BusContext{RepoRoot: dir, WorkDir: dir}, path, nil)
	w := result.TraceEvidenceAuthority.FrequencyLimitWitnesses[0]
	for _, zh := range []bool{false, true} {
		text := runtimeTraceFrequencyLimitWitnessRoster([]types.TraceFrequencyLimitAuthority{w}, zh)
		if !strings.Contains(text, path) || !strings.Contains(text, w.SourceRef.PayloadRef) {
			t.Fatalf("reader omitted source: %s", text)
		}
		for _, token := range []string{"source_ref", "query_scope_id", "query_lines=", w.SourceRef.QueryScopeID} {
			if strings.Contains(text, token) {
				t.Fatalf("reader exposed internal query identity %q: %s", token, text)
			}
		}
		child := types.CloneTraceFrequencyLimitAuthority(w)
		child.SourceRef.QueryScopeID += ":w7"
		if strings.Contains(runtimeTraceFrequencyWitnessSource(child, zh), ":w7") {
			t.Fatal("reader exposed internal child identity")
		}
		unknown := w
		unknown.SourceRef = nil
		text = runtimeTraceFrequencyLimitWitnessRoster([]types.TraceFrequencyLimitAuthority{unknown}, zh)
		boundary := "cannot associate with other records"
		if zh {
			boundary = "不能与其他记录关联"
		}
		if !strings.Contains(text, boundary) || !strings.Contains(text, "2100000") {
			t.Fatalf("unknown source lost its independent facts/boundary: %s", text)
		}
	}
	input := []types.TraceFrequencyLimitAuthority{w, w}
	for i := 0; i < 10; i++ {
		u := w
		u.SourceRef = nil
		input = append(input, u)
	}
	for name, dedup := range map[string]func([]types.TraceFrequencyLimitAuthority, int) []types.TraceFrequencyLimitAuthority{"auto": dedupTraceQueryFrequencyLimitAuthorities, "reader": runtimeTraceDedupFrequencyLimitWitnesses} {
		if got := dedup(input, 8); len(got) != 8 {
			t.Fatalf("%s changed the eight-witness budget: %d", name, len(got))
		}
		if got := dedup([]types.TraceFrequencyLimitAuthority{w, w}, 8); len(got) != 1 {
			t.Fatalf("%s failed exact same-receipt dedup: %d", name, len(got))
		}
	}
}

func TestB1631ActualFrequencyBoundaryKeepsModelAndSource(t *testing.T) {
	dir := t.TempDir()
	path := b1631FrequencyTrace(t, dir, "capture")
	result := b1631ExecuteFrequency(t, &types.BusContext{RepoRoot: dir, WorkDir: dir}, path, nil)
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			ctx := newBusForMutationTest()
			ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentRootCause}, AnswerContract: types.AnswerContract{Language: lang}}
			ctx.ToolResults = []types.ToolResult{result}
			model := types.AnswerBlock{ID: "model-answer", Kind: types.BlockSummary, Text: "Keep the model-authored explanation byte-identical."}
			doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{model}}
			before, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if !materializeRuntimeTraceFrequencyAuthorityCaveat(doc, ctx) || len(doc.Blocks) != 2 || !reflect.DeepEqual(doc.Blocks[0], model) {
				t.Fatalf("publication must append only the independent boundary: %+v", doc)
			}
			after, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("reader mutated a published result")
			}
			if !strings.Contains(types.AnswerBlockVisibleSurface(doc.Blocks[1]), path) {
				t.Fatal("actual boundary dropped capture source")
			}
		})
	}
}

func TestB1631PublicationScopeIncludesFullSubmittedQuery(t *testing.T) {
	result := tracequery.Result{View: "window_stats", SourcePath: "/captures/policy.systrace", TimeStart: 1, TimeEnd: 2, WindowStats: &tracequery.WindowStats{CPUFrequencyLimits: []tracequery.CPUFrequencyLimit{{CPU: 4, MinFrequency: 558000, MaxFrequency: 2100000, Count: 2, Line: 3, Ts: 1.2}}}}
	base := tracequery.Query{View: "window_stats", PID: 42, TimeStart: 1, TimeEnd: 2, LineStart: 1, LineEnd: 5}
	queries := []tracequery.Query{base, base, base}
	queries[1].Pattern = "different literal filter"
	queries[2].TargetScope = "process"
	seen := map[string]bool{}
	for _, query := range queries {
		// Identical result bytes, content-addressed receipt and publication
		// second cannot erase different submitted typed filters.
		w := traceQueryEvidenceAuthorityWithSource(result, "path", "/results/same.json", "/results/same.txt", "", time.Unix(100, 0), query).FrequencyLimitWitnesses[0]
		key := types.TraceFrequencyLimitSourceKey(w)
		if key == "" || seen[key] {
			t.Fatalf("distinct submitted query shares a source key: query=%+v witness=%+v", query, w)
		}
		seen[key] = true
		again := traceQueryEvidenceAuthorityWithSource(result, "path", "/results/same.json", "/results/same.txt", "", time.Unix(100, 0), query).FrequencyLimitWitnesses[0]
		if !reflect.DeepEqual(w, again) {
			t.Fatal("same submitted query changed its deterministic source identity")
		}
	}
	invalid := base
	invalid.MinDurationMs = math.NaN()
	if got := traceQueryPublicationScope(result, "/results/same.json", "/results/same.txt", "", invalid); got != "" {
		t.Fatalf("unserializable query must remain unknown, got %q", got)
	}
}

func TestB1631ActualFullArtifactCoverageSharesQueryReceipt(t *testing.T) {
	for _, view := range []string{"window_stats", "event_search"} {
		t.Run(view, func(t *testing.T) {
			dir := t.TempDir()
			path := b1631FrequencyTrace(t, dir, "capture")
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(body), "prev_state=S", "prev_state=D")), 0o600); err != nil {
				t.Fatal(err)
			}
			params := map[string]any{"source": "path", "path": path, "view": view}
			if view == "window_stats" {
				params["pid"] = 42
			}
			raw, err := json.Marshal(params)
			if err != nil {
				t.Fatal(err)
			}
			result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, raw)
			if err != nil || !result.Success {
				t.Fatalf("actual full-artifact Execute: err=%v result=%+v", err, result)
			}
			var coverage *types.ObservationRecord
			for i := range result.Observations {
				if result.Observations[i].Predicate == types.RuntimeArtifactScopeCoveragePredicate {
					coverage = &result.Observations[i]
				}
			}
			if coverage == nil || len(result.Observations) < 2 {
				t.Fatal("fixture lacks full-artifact coverage plus ordinary rows")
			}
			for _, record := range result.Observations {
				if !types.TraceRuntimeAccountRecordsSameResult(record, *coverage) {
					t.Fatalf("same-result full-artifact coverage lost query scope: coverage=%+v row=%+v", coverage.SourceRef, record.SourceRef)
				}
			}
			if view == "window_stats" {
				rm := types.RequestModel{RuntimeTargets: []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 42, Thread: "target-42"}}, RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeFullArtifact, SourceQuote: "whole capture"}}
				got := types.BuildTraceTargetWaitSummaryAuthorities(types.ObservationLedger{Records: result.Observations}, &rm)
				if len(got) != 1 || !got[0].IsRequestedScopePrincipal() || got[0].Count != 1 || got[0].DStateOccurrences != 1 {
					t.Fatalf("full-artifact D-state account lost its original principal scope: %+v", got)
				}
			}
		})
	}
}

func b1631ExecuteFrequency(t *testing.T, ctx *types.BusContext, path string, extra map[string]any) types.ToolResult {
	t.Helper()
	params := map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 42, "time_start": 1, "time_end": 2}
	for key, value := range extra {
		params[key] = value
	}
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&TraceQuery{}).Execute(ctx, raw)
	if err != nil || !result.Success || result.TraceEvidenceAuthority == nil || len(result.TraceEvidenceAuthority.FrequencyLimitWitnesses) != 1 {
		t.Fatalf("actual Execute: err=%v result=%+v", err, result)
	}
	return result
}

// The first red checks the actual publication wire without requiring the new
// metadata fields to exist in the Go type yet. A schema-less helper-only pass
// cannot satisfy this assertion.
func TestB1631TraceQueryExecutePublishesFrequencyWitnessSource(t *testing.T) {
	dir := t.TempDir()
	path := b1631FrequencyTrace(t, dir, "capture-A")
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("frequency source")}
	result := b1631ExecuteFrequency(t, ctx, path, nil)
	witness := result.TraceEvidenceAuthority.FrequencyLimitWitnesses[0]
	if witness.MaxFrequencyKHz != 2100000 || witness.MinFrequencyKHz != 558000 || witness.LimitRowCount != 2 || witness.WitnessLine != 3 || witness.WitnessTs != 1.2 || witness.WindowStartTs != 1 || witness.WindowEndTs != 2 {
		t.Fatalf("original frequency facts changed: %+v", witness)
	}
	wire, err := json.Marshal(witness)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		SourceRef  map[string]any `json:"source_ref"`
		ObservedAt string         `json:"observed_at"`
	}
	if err := json.Unmarshal(wire, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.SourceRef == nil || envelope.SourceRef["path"] != path || envelope.SourceRef["query_scope_id"] == nil || envelope.SourceRef["payload_ref"] == nil || envelope.ObservedAt == "" {
		t.Fatalf("actual frequency witness lost capture/query/result source: %s", wire)
	}
	var targetSeen bool
	for _, record := range result.Observations {
		if record.Predicate != "target_cpu_running" {
			continue
		}
		targetSeen = true
		sourceWire, err := json.Marshal(record.SourceRef)
		if err != nil {
			t.Fatal(err)
		}
		var source map[string]any
		if err := json.Unmarshal(sourceWire, &source); err != nil {
			t.Fatal(err)
		}
		if source["query_scope_id"] != envelope.SourceRef["query_scope_id"] || source["payload_ref"] != envelope.SourceRef["payload_ref"] || source["path"] != path || record.ObservedAt != envelope.ObservedAt {
			t.Fatalf("policy and target source cohorts differ: witness=%s target=%+v", wire, record)
		}
	}
	if !targetSeen {
		t.Fatal("fixture lacks actual target CPU observation")
	}
}
