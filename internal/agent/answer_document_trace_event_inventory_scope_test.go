package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func traceInventoryScopePublicQuery(t *testing.T, source string, start, end float64, extra map[string]any) types.ToolResult {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "scoped-business.trace")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	params := map[string]any{"source": "path", "path": path, "view": "event_search", "time_start": start, "time_end": end, "limit": 40}
	for key, value := range extra {
		params[key] = value
	}
	wire, _ := json.Marshal(params)
	result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, wire)
	if err != nil || !result.Success {
		t.Fatalf("public event_search failed: %v / %s", err, result.Summary)
	}
	return result
}

func traceInventoryScopeContext(result types.ToolResult, start, end float64) *types.AgentContext {
	ctx := traceEventInventoryPublicContext([]types.ToolResult{result})
	ctx.Stage, ctx.AgentName = types.StageFinalize, types.AgentFinalizer
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 101, Thread: "writer-101", Source: "user_explicit"}}
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{
		Scope:        types.RuntimeQuestionScopeBoundedFactSet,
		FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration, types.RuntimeQuestionFactOccurrenceTime},
	}
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end,
		SourceQuote: fmt.Sprintf("[%g,%g) seconds", start, end),
	}
	return ctx
}

func traceInventoryScopeReceipt(t *testing.T, ctx *types.AgentContext) types.ObservationRecord {
	t.Helper()
	var receipts []types.ObservationRecord
	for _, record := range answerDocObservationLedger(ctx).Records {
		if types.IsValidTraceEventSearchInventoryRecord(record) {
			receipts = append(receipts, record)
		}
	}
	if len(receipts) != 1 {
		t.Fatalf("want one accepted public receipt, got %d", len(receipts))
	}
	return receipts[0]
}

func traceInventoryScopeAuthority(t *testing.T, ctx *types.AgentContext) string {
	t.Helper()
	ledger := answerDocObservationLedger(ctx)
	wire, err := json.Marshal([]any{ledger, types.CompileTraceCausalProjectionSet(ledger), ctx.Mutable.TraceRootCauseReport()})
	if err != nil {
		t.Fatal(err)
	}
	return string(wire)
}

func traceInventoryScopeProjectionFields(t *testing.T, prompt string) map[string]json.RawMessage {
	t.Helper()
	for _, line := range strings.Split(prompt, "\n") {
		if !strings.HasPrefix(line, "- {") {
			continue
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "- ")), &object); err != nil {
			t.Fatal(err)
		}
		if raw, ok := object["prompt_scope_projection"]; ok {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			return fields
		}
	}
	t.Fatal("scope-excluded members were not disclosed separately from engine coverage")
	return nil
}

func TestTraceEventInventoryScopeActualFinalizerKeepsNamedTargetBusinessFields(t *testing.T) {
	longName := "load-" + strings.Repeat("segment-", 85) + "-source-tail"
	result := traceInventoryScopePublicQuery(t,
		"writer-101 (101) [000] .... 9.999900: tracing_mark_write: I|101|outside-left\n"+
			"writer-101 (101) [000] .... 10.000000: tracing_mark_write: I|101|left-included\n"+
			"writer-202 (101) [000] .... 10.005000: tracing_mark_write: I|101|same-name-other-thread\n"+
			"other-303 (101) [001] .... 10.006000: tracing_mark_write: I|101|writer-101-only-in-payload\n"+
			"writer-101 (101) [000] .... 10.020000: tracing_mark_write: I|101|"+longName+"\n"+
			"writer-101 (101) [000] .... 10.049900: tracing_mark_write: I|101|inside-right\n"+
			"writer-101 (101) [000] .... 10.050000: tracing_mark_write: I|101|right-excluded\n"+
			"writer-101 (101) [000] .... 10.050100: tracing_mark_write: I|101|lookup-tolerance-only\n", 9.9999, 10.05, nil)
	ctx := traceInventoryScopeContext(result, 10, 10.05)
	original := traceInventoryScopeReceipt(t, ctx)
	if len(original.EventSearchInventory.Rows) != 8 {
		t.Fatalf("fixture must retain lookup tolerance/background rows: %+v", original.EventSearchInventory)
	}
	before := traceInventoryScopeAuthority(t, ctx)
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	views := traceEventInventoryPromptViews(t, prompt)
	if len(views) != 1 || len(views[0].Inventory.Rows) != 3 {
		t.Fatalf("named-target actual finalizer lost its three eligible business rows: views=%+v", views)
	}
	view := views[0]
	if !reflect.DeepEqual(view.Source, original.SourceRef) || !reflect.DeepEqual(view.Inventory.Query, original.EventSearchInventory.Query) ||
		!reflect.DeepEqual(view.Inventory.Coverage, original.EventSearchInventory.Coverage) {
		t.Fatal("prompt projection rewrote original source/query/coverage")
	}
	for n, row := range view.Inventory.Rows {
		wantLine := []int{2, 5, 6}[n]
		if row.Line != wantLine || row.LocalLine != wantLine || row.SourcePath != original.SourceRef.Path || row.EmitterTID != 101 || row.EmitterTIDKnown == nil || !*row.EmitterTIDKnown || row.CPU != 0 || row.CPUKnown == nil || !*row.CPUKnown {
			t.Fatalf("wrong physical member or known-zero semantics: %+v", row)
		}
	}
	longRow := view.Inventory.Rows[1]
	if !longRow.RawTruncated || strings.Contains(longRow.Raw, "-source-tail") || traceEventSemanticKnownText(longRow.Semantics, "marker.name") != longName {
		t.Fatal("parsed business identity beyond preview was lost or guessed")
	}
	for _, forbidden := range []string{"outside-left", "same-name-other-thread", "writer-101-only-in-payload", "right-excluded", "lookup-tolerance-only"} {
		for _, row := range view.Inventory.Rows {
			if strings.Contains(row.Raw, forbidden) || traceEventSemanticKnownText(row.Semantics, "marker.name") == forbidden {
				t.Fatalf("unowned/out-of-window row leaked into principal inventory: %s", forbidden)
			}
		}
	}
	fields := traceInventoryScopeProjectionFields(t, prompt)
	for name, want := range map[string]int{"input_rows": 8, "retained_rows": 3, "owner_rows_excluded": 2, "window_rows_excluded": 3} {
		var got int
		if err := json.Unmarshal(fields[name], &got); err != nil || got != want {
			t.Fatalf("scope accounting %s=%s, want %d: %v", name, fields[name], want, err)
		}
	}
	if after := traceInventoryScopeAuthority(t, ctx); before != after {
		t.Fatal("answer-writing projection mutated ledger, root report, or causal authority")
	}
}

func TestTraceEventInventoryScopeActualFinalizerZeroAndUnknownStayDistinct(t *testing.T) {
	unknown, err := tracequery.FormatCPUUnavailableTraceMark(tracequery.CPUUnavailableTraceMark{
		TimestampNS: 10020000000, TID: 101, TGID: 101, SpanPID: 101,
		Action: "B", Comm: "writer", Name: "librender.so", Reason: tracequery.TraceMarkCPUReasonUnknownStart,
	})
	if err != nil {
		t.Fatal(err)
	}
	source := "writer-101 (101) [000] .... 10.000000: tracing_mark_write: I|101|known-zero-cpu\n" + unknown + "\n"
	for _, empty := range []bool{false, true} {
		t.Run(fmt.Sprintf("empty=%t", empty), func(t *testing.T) {
			var extra map[string]any
			if empty {
				extra = map[string]any{"pattern": "absent-business-marker"}
			}
			result := traceInventoryScopePublicQuery(t, source, 10, 10.05, extra)
			ctx := traceInventoryScopeContext(result, 10, 10.05)
			original := traceInventoryScopeReceipt(t, ctx)
			before := traceInventoryScopeAuthority(t, ctx)
			views := traceEventInventoryPromptViews(t, traceEventInventoryActualFinalizerPrompt(t, ctx))
			want := 2
			if empty {
				want = 0
			}
			if len(views) != 1 || len(views[0].Inventory.Rows) != want || views[0].Inventory.Coverage.MatchedTotal != want ||
				!reflect.DeepEqual(views[0].Inventory.Coverage, original.EventSearchInventory.Coverage) {
				t.Fatalf("zero/known/unknown receipt lost: %+v", views)
			}
			if !empty {
				rows := views[0].Inventory.Rows
				if rows[0].CPU != 0 || rows[0].CPUKnown == nil || !*rows[0].CPUKnown || rows[1].CPU != -1 || rows[1].CPUKnown == nil || *rows[1].CPUKnown ||
					rows[1].CPUUnknownReason != tracequery.TraceMarkCPUReasonUnknownStart || traceEventSemanticKnownText(rows[1].Semantics, "marker.name") != "librender.so" {
					t.Fatalf("unknown CPU became zero or decoded name disappeared: %+v", rows)
				}
			}
			if before != traceInventoryScopeAuthority(t, ctx) {
				t.Fatal("zero/unknown projection changed original evidence")
			}
		})
	}
}

func TestTraceEventInventoryScopeActualFinalizerDoesNotBorrowAuthority(t *testing.T) {
	result := traceInventoryScopePublicQuery(t, "writer-101 (101) [000] .... 10.020000: tracing_mark_write: I|101|source-owned-value\n", 10, 10.05, nil)
	baseCtx := traceInventoryScopeContext(result, 10, 10.05)
	base := traceInventoryScopeReceipt(t, baseCtx)
	for _, tc := range []struct {
		name     string
		mutate   func(*types.ObservationRecord)
		admitted bool
	}{
		{"legacy-owner-unknown", func(r *types.ObservationRecord) { r.EventSearchInventory.Rows[0].EmitterTIDKnown = nil }, true},
		{"explicit-owner-unavailable", func(r *types.ObservationRecord) {
			known := false
			r.EventSearchInventory.Rows[0].EmitterTIDKnown = &known
			r.EventSearchInventory.Rows[0].EmitterTID = -1
		}, true},
		{"same-name-other-tid-query-claims-target", func(r *types.ObservationRecord) {
			r.EventSearchInventory.Rows[0].EmitterTID = 202
			r.EventSearchInventory.Query.PID = 101
			r.EventSearchInventory.Query.Thread = "writer-101"
			r.SourceRef.QueryTargetPID = 101
			r.SourceRef.QueryTargetThread = "writer-101"
			r.SourceRef.QueryTargetScope = "thread"
		}, true},
		{"model-inference-with-real-source-and-payload", func(r *types.ObservationRecord) {
			r.ClaimAuthority = types.ObservationClaimAuthorityModelInference
		}, false},
		{"not-query-producer", func(r *types.ObservationRecord) { r.Producer = "emit_investigation_complete" }, false},
		{"wrong-query-identity", func(r *types.ObservationRecord) { r.EventSearchInventory.QueryScopeID += "-other" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := base
			record.EventSearchInventory = types.CloneTraceEventSearchInventory(base.EventSearchInventory)
			tc.mutate(&record)
			if types.IsValidTraceEventSearchInventoryRecord(record) != tc.admitted {
				t.Fatal("fixture did not establish intended receipt admissibility")
			}
			changed := result
			changed.Observations = []types.ObservationRecord{record}
			ctx := traceInventoryScopeContext(changed, 10, 10.05)
			before := traceInventoryScopeAuthority(t, ctx)
			views := traceEventInventoryPromptViews(t, traceEventInventoryActualFinalizerPrompt(t, ctx))
			if tc.admitted && (len(views) != 1 || len(views[0].Inventory.Rows) != 0 || views[0].Inventory.Coverage.MatchedTotal != 1) {
				t.Fatalf("unproven ownership obtained a principal member or lost original count: %+v", views)
			}
			if !tc.admitted && len(views) != 0 {
				t.Fatalf("ineligible producer receipt granted display authority: %+v", views)
			}
			if before != traceInventoryScopeAuthority(t, ctx) {
				t.Fatal("negative display case rewrote accepted facts")
			}
		})
	}
}

func TestTraceEventInventoryScopeActualFinalizerLeavesUnboundedAndCausalLanesUnchanged(t *testing.T) {
	result := traceInventoryScopePublicQuery(t,
		"writer-101 (101) [000] .... 10.020000: tracing_mark_write: I|101|owner-value\n"+
			"writer-202 (202) [001] .... 10.030000: tracing_mark_write: I|202|other-thread-value\n"+
			"writer-101 (101) [000] .... 10.050000: tracing_mark_write: I|101|right-boundary-value\n", 10, 10.05, nil)
	for _, lane := range []string{"no-target", "causal", "exploration-cursor"} {
		t.Run(lane, func(t *testing.T) {
			ctx := traceInventoryScopeContext(result, 10, 10.05)
			switch lane {
			case "no-target":
				ctx.AnalysisIR.RequestModel.RuntimeTargets = nil
			case "causal":
				ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeCausalDiagnosis
			case "exploration-cursor":
				ctx.AnalysisIR.RequestModel.RuntimeTargets[0].Source = types.RuntimeTargetSourceExplicitToolCall
			}
			original := traceInventoryScopeReceipt(t, ctx)
			before := traceInventoryScopeAuthority(t, ctx)
			views := traceEventInventoryPromptViews(t, traceEventInventoryActualFinalizerPrompt(t, ctx))
			if len(views) != 1 || !reflect.DeepEqual(views[0].Inventory, original.EventSearchInventory) || !reflect.DeepEqual(views[0].Source, original.SourceRef) {
				t.Fatalf("finite named-target projection changed %s lane: %+v", lane, views)
			}
			if before != traceInventoryScopeAuthority(t, ctx) {
				t.Fatal("compatibility projection mutated evidence")
			}
		})
	}
}

func TestTraceEventInventoryScopeActualFinalizerKeepsSharedBudgetAndOmissions(t *testing.T) {
	var source strings.Builder
	for n := 0; n < 40; n++ {
		fmt.Fprintf(&source, "writer-101 (101) [000] .... 10.%03d000: tracing_mark_write: I|101|item-%02d-%s\n", n, n, strings.Repeat("载荷", 110))
	}
	result := traceInventoryScopePublicQuery(t, source.String(), 10, 10.05, nil)
	ctx := traceInventoryScopeContext(result, 10, 10.05)
	original := traceInventoryScopeReceipt(t, ctx)
	before := traceInventoryScopeAuthority(t, ctx)
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	views := traceEventInventoryPromptViews(t, prompt)
	if len(views) != 1 || len(views[0].Inventory.Rows) != 32 || views[0].PromptRowsOmitted != 8 ||
		views[0].Inventory.HandoffRowsOmitted != 8 || views[0].Inventory.RowsComplete ||
		!reflect.DeepEqual(views[0].Inventory.Coverage, original.EventSearchInventory.Coverage) || views[0].Inventory.Coverage.MatchedTotal != 40 {
		t.Fatalf("named-target inventory bypassed shared row budget or fabricated completeness: %+v", views)
	}
	fields := traceInventoryScopeProjectionFields(t, prompt)
	for name, want := range map[string]int{"input_rows": 40, "retained_rows": 40, "owner_rows_excluded": 0, "window_rows_excluded": 0, "prompt_budget_rows_omitted": 8} {
		var got int
		if err := json.Unmarshal(fields[name], &got); err != nil || got != want {
			t.Fatalf("budget/scope accounting %s=%s, want %d: %v", name, fields[name], want, err)
		}
	}
	start := strings.Index(prompt, "### Trace Event Search Inventories")
	end := strings.Index(prompt, "inventory_section_byte_limit=")
	if start < 0 || end < start {
		t.Fatal("inventory budget disclosure missing")
	}
	end += strings.Index(prompt[end:], "\n") + 1
	if end-start > 128*1024 {
		t.Fatalf("inventory section exceeds shared byte budget: %d", end-start)
	}
	if before != traceInventoryScopeAuthority(t, ctx) {
		t.Fatal("display budget changed underlying inventory or causal facts")
	}
}

func TestTraceEventInventoryScopeActualFinalizerUsesObservedProcessAndExactNameOnly(t *testing.T) {
	result := traceInventoryScopePublicQuery(t,
		"writer-101 (201) [000] .... 10.010000: tracing_mark_write: I|999|first-process-member\n"+
			"worker-202 (201) [001] .... 10.020000: tracing_mark_write: I|999|second-process-member\n"+
			"other-303 (999) [001] .... 10.030000: tracing_mark_write: I|201|process-only-in-payload\n"+
			"writer-404 (999) [002] .... 10.040000: tracing_mark_write: I|201|same-name-other-process\n", 10, 10.05, nil)
	for _, tc := range []struct {
		name   string
		target types.RuntimeTarget
		lines  []int
	}{
		{"header-process", types.RuntimeTarget{Kind: types.RuntimeTargetKindProcess, PID: 201, Source: "user_explicit"}, []int{1, 2}},
		{"exact-observed-name-not-unique-thread", types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, Thread: "writer", Source: "user_explicit"}, []int{1, 4}},
		{"name-substring-is-not-owner", types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, Thread: "write", Source: "user_explicit"}, []int{}},
		{"contradictory-explicit-thread-ids", types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, PID: 101, Thread: "writer-404", Source: "user_explicit"}, []int{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := traceInventoryScopeContext(result, 10, 10.05)
			ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{tc.target}
			before := traceInventoryScopeAuthority(t, ctx)
			views := traceEventInventoryPromptViews(t, traceEventInventoryActualFinalizerPrompt(t, ctx))
			if len(views) != 1 || len(views[0].Inventory.Rows) != len(tc.lines) || views[0].Inventory.Coverage.MatchedTotal != 4 {
				t.Fatalf("observed process/name scope changed: %+v", views)
			}
			for n, line := range tc.lines {
				if views[0].Inventory.Rows[n].Line != line {
					t.Fatalf("wrong source member: %+v", views[0].Inventory.Rows[n])
				}
			}
			if before != traceInventoryScopeAuthority(t, ctx) {
				t.Fatal("name/process projection inferred a different authority")
			}
		})
	}
}

func TestTraceEventInventoryScopeActualFinalizerKeepsZeroWindowAndIndependentSources(t *testing.T) {
	source := "writer-101 (101) [000] .... 0.000000: tracing_mark_write: I|101|zero-origin\n" +
		"writer-101 (101) [000] .... 0.020000: tracing_mark_write: I|101|inside-window\n" +
		"writer-101 (101) [000] .... 0.050000: tracing_mark_write: I|101|right-edge\n"
	first := traceInventoryScopePublicQuery(t, source, 0, .05, nil)
	second := traceInventoryScopePublicQuery(t, source, 0, .05, nil)
	ctx := traceInventoryScopeContext(first, 0, .05)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{first, second}})
	before := traceInventoryScopeAuthority(t, ctx)
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	views := traceEventInventoryPromptViews(t, prompt)
	if len(views) != 2 || views[0].Source.Path == views[1].Source.Path || views[0].Source.QueryScopeID == views[1].Source.QueryScopeID {
		t.Fatalf("independent physical sources were collapsed: %+v", views)
	}
	for _, view := range views {
		if len(view.Inventory.Rows) != 2 || view.Inventory.Rows[0].TraceTimeSeconds != 0 || view.Inventory.Coverage.MatchedTotal != 3 {
			t.Fatalf("zero-origin/right-open scope lost exact count: %+v", view)
		}
		for _, row := range view.Inventory.Rows {
			if row.SourcePath != view.Source.Path || row.LocalLine != row.Line || !row.SourceTimeKnown {
				t.Fatal("row coordinates borrowed another capture")
			}
		}
	}
	if !strings.Contains(prompt, "prompt_member_rows=4/32") {
		t.Fatal("identical row bytes in different captures shared an object")
	}
	if before != traceInventoryScopeAuthority(t, ctx) {
		t.Fatal("projection changed independent source identities")
	}
}

func TestTraceEventInventoryScopeActualFinalizerPreservesSeparateWindowsWithLookupTolerance(t *testing.T) {
	source := "writer-101 (101) [000] .... 10.000000: tracing_mark_write: I|101|first-window-start\n" +
		"writer-101 (101) [000] .... 10.010000: tracing_mark_write: I|101|first-right-edge\n" +
		"writer-101 (101) [000] .... 10.015000: tracing_mark_write: I|101|between-windows\n" +
		"writer-101 (101) [000] .... 10.019900: tracing_mark_write: I|101|second-left-tolerance\n" +
		"writer-101 (101) [000] .... 10.020000: tracing_mark_write: I|101|second-window-start\n" +
		"writer-101 (101) [000] .... 10.029900: tracing_mark_write: I|101|second-window-inside\n" +
		"writer-101 (101) [000] .... 10.030000: tracing_mark_write: I|101|second-right-edge\n" +
		"writer-101 (101) [000] .... 10.030100: tracing_mark_write: I|101|second-right-tolerance\n"
	firstStart, firstEnd, secondStart, secondEnd := 10.0, 10.01, 10.02, 10.03
	first := traceInventoryScopePublicQuery(t, source, firstStart, firstEnd, nil)
	ctx := traceInventoryScopeContext(first, firstStart, firstEnd)
	path := traceInventoryScopeReceipt(t, ctx).SourceRef.Path
	second := traceInventoryScopePublicQuery(t, source, secondStart, secondEnd, map[string]any{"path": path})
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{first, second}})
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
		TimeWindows: []types.RuntimeArtifactTimeWindow{
			{TimeStart: &firstStart, TimeEnd: &firstEnd, SourceQuote: "[10,10.01) seconds"},
			{TimeStart: &secondStart, TimeEnd: &secondEnd, SourceQuote: "[10.02,10.03) seconds"},
		},
	}
	originals := make(map[string]types.ObservationRecord)
	for _, record := range answerDocObservationLedger(ctx).Records {
		if types.IsValidTraceEventSearchInventoryRecord(record) {
			originals[record.SourceRef.QueryScopeID] = record
		}
	}
	if len(originals) != 2 {
		t.Fatalf("public queries did not retain separate identities: %d", len(originals))
	}
	before := traceInventoryScopeAuthority(t, ctx)
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	views := traceEventInventoryPromptViews(t, prompt)
	if len(views) != 2 {
		t.Fatalf("lookup tolerance incorrectly erased a complete query receipt: %+v", views)
	}
	gotLines := []int{}
	for _, view := range views {
		original := originals[view.Source.QueryScopeID]
		if original.EventSearchInventory == nil || !reflect.DeepEqual(view.Source, original.SourceRef) ||
			!reflect.DeepEqual(view.Inventory.Query, original.EventSearchInventory.Query) || !reflect.DeepEqual(view.Inventory.Coverage, original.EventSearchInventory.Coverage) {
			t.Fatal("separate query sources, matching windows, or original counts were combined")
		}
		for _, row := range view.Inventory.Rows {
			gotLines = append(gotLines, row.Line)
			if row.SourcePath != path {
				t.Fatal("window used another source")
			}
		}
	}
	if !reflect.DeepEqual(gotLines, []int{1, 5, 6}) {
		t.Fatalf("window union filled its gap or admitted a right edge: %v", gotLines)
	}
	if before != traceInventoryScopeAuthority(t, ctx) {
		t.Fatal("multi-window prompt changed original evidence or causal scope")
	}
}
