package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQuerySelectsFromBothFrameCatalogs(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, tid, first int, times []float64) string {
		var data strings.Builder
		for i, s := range times {
			fmt.Fprintf(&data, "app-%d (%d) [000] .... %.6f: tracing_mark_write: B|%d|Choreographer#doFrame %d\n", tid, tid, s, tid, first+i)
			fmt.Fprintf(&data, "app-%d (%d) [000] .... %.6f: tracing_mark_write: B|%d|draw-VRI[Main]\n", tid, tid, s+.001, tid)
			fmt.Fprintf(&data, "app-%d (%d) [000] .... %.6f: tracing_mark_write: E\n", tid, tid, s+.002)
			fmt.Fprintf(&data, "app-%d (%d) [000] .... %.6f: tracing_mark_write: E\n", tid, tid, s+.003)
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(data.String()), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	current := write("current.ftrace", 7, 10, []float64{1, 1.01, 1.10, 1.11, 1.12})
	baseline := write("baseline.ftrace", 8, 100, []float64{2, 2.01, 2.10, 2.11, 2.12})
	selector := func(path string, pid int) map[string]any {
		return map[string]any{"source": "path", "path": path, "view": "frame_catalog", "pid": pid}
	}
	args, err := json.Marshal(map[string]any{"comparison": map[string]any{
		"current": selector(current, 7), "baseline": map[string]any{"source": "path", "path": baseline, "view": "frame_catalog"}, "target_frame_vsync": 12,
	}})
	if err != nil {
		t.Fatal(err)
	}
	query := &TraceQuery{}
	if err := toolparam.Validate(args, query.Parameters()); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{Ctx: t.Context(), RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("select frame")}
	out, err := query.Execute(ctx, args)
	if err != nil || !out.Success {
		t.Fatalf("frame selection failed: %v, %s", err, out.Summary)
	}
	body, err := os.ReadFile(out.RawRef)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		tracequery.FrameCatalogMatch
		Investigation json.RawMessage `json:"investigation"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "reference_only" || result.SelectedID == "" || result.Candidates[0].Frame.Vsync != 102 ||
		result.Candidates[0].Frame.EndNS != 2103000000 {
		t.Fatalf("unexpected native result: %+v", result)
	}
	if err := toolparam.Validate(result.Investigation, query.Parameters()); err != nil {
		t.Fatalf("invalid investigation handoff: %v", err)
	}
	measured, err := query.Execute(ctx, result.Investigation)
	if err != nil || !measured.Success {
		t.Fatalf("prepared frame investigation failed: %v, %s", err, measured.Summary)
	}
	report, ok := measured.RuntimeMeasurementPair.Report()
	if !ok {
		t.Fatal("missing measurement pair receipt")
	}
	for i, want := range []struct {
		path       string
		tid        int
		start, end float64
	}{{baseline, 8, 2.1, 2.103}, {current, 7, 1.1, 1.103}} {
		var side traceQueryParams
		if err := json.Unmarshal(report.Sides[i].Request, &side); err != nil {
			t.Fatal(err)
		}
		if side.Path != want.path || side.View != "window_stats" || side.PID.Int() != want.tid ||
			side.TimeStart.Seconds() != want.start || side.TimeEnd.Seconds() != want.end || report.Sides[i].Status == "failed" {
			t.Fatalf("side %d used the wrong frame scope: %s (%s)", i, report.Sides[i].Request, report.Sides[i].Status)
		}
	}

	baselineStart, baselineEnd := 2.0, 2.102
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{Comparison: &types.RuntimeArtifactComparisonScope{
			Baseline: types.RuntimeArtifactComparisonSide{Path: baseline, RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{
				SourceQuote: baseline, TimeStart: &baselineStart, TimeEnd: &baselineEnd,
			}},
			Current: types.RuntimeArtifactComparisonSide{Path: current, RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{SourceQuote: current}},
		}},
	}}
	clipped, err := query.Execute(ctx, args)
	if err != nil || !clipped.Success || !strings.Contains(clipped.Summary, "outside its requested comparison window") ||
		strings.Contains(clipped.Summary, "prepared pair of frame windows") {
		t.Fatalf("frame selection should retain its result without preparing an out-of-scope investigation: %v, %s", err, clipped.Summary)
	}
}
