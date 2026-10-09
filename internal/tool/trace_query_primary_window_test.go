package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func primaryWindowContext(t *testing.T) *types.BusContext {
	t.Helper()
	data, err := os.ReadFile("../../eval/fixtures/hmosperf_rendering_candidates/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	return &types.BusContext{WorkDir: dir, RepoRoot: dir, Language: "zh", AttachedHitrace: string(data),
		Mutable: types.NewMutableState("1.000到1.050秒的渲染线索"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RuntimeArtifactScopeProfile: requestWindowTestProfile(1, 1.05),
			RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis, FrameCausalityRequested: true},
		}}}
}

func primaryWindowQuery(t *testing.T, ctx *types.BusContext, end float64) types.ToolResult {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"view": "rendering_candidates", "time_start": 1, "time_end": end})
	out, err := (&TraceQuery{}).Execute(ctx, raw)
	if err != nil || !out.Success {
		t.Fatalf("query: %v %s", err, out.Summary)
	}
	ctx.ToolResults = append(ctx.ToolResults, out)
	return out
}

func TestPrimaryWindowSupplementPublicMissingTarget(t *testing.T) {
	ctx := primaryWindowContext(t)
	original := primaryWindowQuery(t, ctx, 1.051)
	if path, _, ok := ctx.Mutable.ResolveTraceQueryWindowReplay(original.TraceQueryWindowReplay); !ok {
		t.Fatalf("native query did not mint current replay ticket: path=%q ticket=%+v source=%+v", path, original.TraceQueryWindowReplay, original.TraceQuerySourceRead)
	}
	path, label, _, reject := resolveReadyTraceQuerySource(ctx, traceQueryParams{})
	input := types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit)
	if calls := primaryWindowCalls(ctx, path, label, input); len(calls) == 0 {
		actual, raw, current := ctx.Mutable.ResolveTraceQueryWindowReplay(original.TraceQueryWindowReplay)
		physical, valid := traceQueryRequestWindowPathIdentity(ctx, path)
		t.Fatalf("no eligible calls: path=%q label=%q reject=%+v input=%d actual=%q current=%v raw=%s physical=%q valid=%v sourceMatch=%v", path, label, reject, len(input.ToolResults), actual, current, raw, physical, valid, traceQueryRequestWindowSourceMatches(ctx, path, label))
	}
	out := RunTraceQuerySystemSupplement(ctx)
	if len(out.Executed) != 1 || out.Executed[0] != "rendering_candidates" {
		t.Fatalf("missing primary window: %+v", out)
	}
	if len(ctx.ToolResults) != 1 || ctx.ToolResults[0].RawRef != original.RawRef {
		t.Fatal("changed model exploration")
	}
	results := ctx.Mutable.SystemTraceSupplementResults()
	if len(results) != 1 {
		t.Fatalf("results=%d", len(results))
	}
	found := false
	for _, obs := range results[0].Observations {
		p, ok := DecodeTraceRenderingCandidates(obs)
		if !ok {
			continue
		}
		found = true
		if p.Window.StartTs != 1 || p.Window.EndTs != 1.05 || p.Window.EndInclusive {
			t.Fatalf("wrong primary window: %+v", p.Window)
		}
		for _, c := range p.Candidates {
			if c.OwnerID == 900 {
				t.Fatal("right boundary leaked")
			}
		}
	}
	if !found {
		t.Fatal("no source-bound primary result")
	}
	if next := RunTraceQuerySystemSupplement(ctx); next.Attempted {
		t.Fatal("supplement repeated")
	}
}

func TestPrimaryWindowSupplementNoDuplicateOrHistoricalReplay(t *testing.T) {
	for _, name := range []string{"already queried", "historical JSON", "changed source", "new run", "same path rewritten"} {
		t.Run(name, func(t *testing.T) {
			ctx := primaryWindowContext(t)
			original := primaryWindowQuery(t, ctx, 1.051)
			switch name {
			case "already queried":
				primaryWindowQuery(t, ctx, 1.05)
			case "historical JSON":
				data, _ := json.Marshal(ctx.ToolResults)
				ctx.ToolResults = nil
				if err := json.Unmarshal(data, &ctx.ToolResults); err != nil {
					t.Fatal(err)
				}
			case "changed source":
				if err := os.WriteFile(filepath.Join(ctx.WorkDir, types.AttachedTraceBlobBasename), []byte(suppCoreTrace), 0600); err != nil {
					t.Fatal(err)
				}
			case "new run":
				ctx.Mutable = types.NewMutableState("new turn")
			case "same path rewritten":
				path, _, ok := ctx.Mutable.ResolveTraceQueryWindowReplay(original.TraceQueryWindowReplay)
				if !ok {
					t.Fatal("missing native receipt")
				}
				if err := os.WriteFile(path, []byte(suppCoreTrace), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if out := RunTraceQuerySystemSupplement(ctx); len(out.Executed) != 0 {
				t.Fatalf("unexpected replay: %+v", out)
			}
		})
	}
}

func TestPrimaryWindowReplayPreservesFiltersAndHintAuthority(t *testing.T) {
	raw := json.RawMessage(`{"view":"rendering_candidates","source":"attached_trace","time_start":1,"time_end":1.051,"pid":100,"target_scope":"process","pattern":"Draw","match_mode":"literal","limit":3,"core_topology":"0-3:little,4-7:big"}`)
	var p traceQueryParams
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	params := traceQueryWindowReplayParams(p, "/prepared/capture", "attached_trace", raw)
	var got, want map[string]any
	_ = json.Unmarshal(params, &got)
	_ = json.Unmarshal(raw, &want)
	for key, expected := range want {
		if !reflect.DeepEqual(got[key], expected) {
			t.Fatalf("%s changed: got=%v want=%v", key, got[key], expected)
		}
	}
	if _, ok := got["platform"]; ok {
		t.Fatal("promoted attached platform to explicit override")
	}
	if _, ok := got["trace_flavor"]; ok {
		t.Fatal("promoted attached flavor to explicit override")
	}
	for _, field := range []string{"line_start", "line_end", "span_name", "business_span_ref", "recipe_name", "interaction_direction", "via_thread"} {
		var restricted map[string]any
		_ = json.Unmarshal(raw, &restricted)
		if field == "line_start" || field == "line_end" {
			restricted[field] = 1
		} else {
			restricted[field] = "selected"
		}
		encoded, _ := json.Marshal(restricted)
		var q traceQueryParams
		if err := json.Unmarshal(encoded, &q); err != nil {
			t.Fatal(err)
		}
		if len(traceQueryWindowReplayParams(q, "/prepared/capture", "path", encoded)) != 0 {
			t.Fatalf("replayed context-dependent %s", field)
		}
	}
}

func TestPrimaryWindowSupplementMultiWindowBudgetAndEmptyCompletion(t *testing.T) {
	ctx := primaryWindowContext(t)
	primaryWindowQuery(t, ctx, 1.051)
	var windows []types.RuntimeArtifactTimeWindow
	for _, bounds := range [][2]float64{{2, 2.1}, {3, 3.1}, {2, 2.1}, {4, 4.1}} {
		start, end := bounds[0], bounds[1]
		windows = append(windows, types.RuntimeArtifactTimeWindow{TimeStart: &start, TimeEnd: &end, SourceQuote: "explicit window"})
	}
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeWindows: windows}
	out := RunTraceQuerySystemSupplement(ctx)
	if len(out.Executed) != 2 || len(ctx.Mutable.SystemTraceSupplementResults()) != 2 {
		t.Fatalf("budget or empty result completion: %+v", out)
	}
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil || len(meta.MemberWindows) != 4 {
		t.Fatalf("lost windows: %+v", meta)
	}
	if !reflect.DeepEqual(meta.MemberWindows[0], meta.MemberWindows[2]) {
		t.Fatal("duplicate window reran or lost receipt")
	}
	if meta.MemberWindows[3].SkipReason != types.TraceSupplementReasonQueryBudgetExceeded || len(meta.MemberWindows[3].SkippedViews) != 1 {
		t.Fatalf("missing budget disclosure: %+v", meta)
	}
}

func TestStableAttachedTraceBlobPreservesVersionAndRepairsChangedContent(t *testing.T) {
	dir := t.TempDir()
	body := "source content\n"
	path := storeStableTraceBlob(dir, "attached_trace.txt", body)
	fixed := time.Unix(1234567890, 0)
	if err := os.Chtimes(path, fixed, fixed); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if next := storeStableTraceBlob(dir, "attached_trace.txt", body); next != path {
		t.Fatal("changed path")
	}
	after, _ := os.Stat(path)
	if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("same bytes invalidated source receipt")
	}
	if err := os.WriteFile(path, []byte("tamper content\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if next := storeStableTraceBlob(dir, "attached_trace.txt", body); next != path {
		t.Fatal("changed content-addressed name")
	}
	data, _ := os.ReadFile(path)
	if string(data) != body {
		t.Fatal("trusted short filename hash without checking bytes")
	}
}
