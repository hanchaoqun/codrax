package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC237BoundedSelectionDoesNotDependOnCountClassification(t *testing.T) {
	for _, count := range []bool{false, true} {
		for _, complete := range []bool{false, true} {
			ctx := hmc234SelectionContext(t, "列出至少三帧的卡顿")
			ctx.AnalysisIR.RequestModel.Predicates.IsCountQuestion = count
			if complete {
				ctx.AnalysisIR.RequestModel.CompletenessObligation = &types.CompletenessObligation{Required: true, SourceQuote: "列出"}
			}
			for _, field := range []string{"appid", "start_ts", "end_ts", "jank_frames"} {
				bad := hmc234SelectionQuery(t, ctx, `[{"field":"`+field+`","op":"gte","value":"3","source_quote":"fabricated condition"}]`)
				if bad.Success || bad.Repair == nil || bad.Repair.Code != types.ToolRepairCodeTraceEventSelectionSourceRequired || len(bad.Observations) != 0 {
					t.Fatalf("count=%v complete=%v field=%s admitted an unrequested predicate: %+v", count, complete, field, bad)
				}
			}
			good := hmc234SelectionQuery(t, ctx, `[{"field":"jank_frames","op":"gte","value":"3","source_quote":"至少三帧"}]`)
			if !good.Success || !strings.Contains(good.Summary, "matched_total=3") {
				t.Fatalf("legitimate normalized condition rejected: %+v", good)
			}
		}
	}
}

func TestHMC237ReportWindowAndBusinessConditionRemainIndependent(t *testing.T) {
	const request = "请列出1到1.5秒记录的至少3帧卡顿；另按业务开始时间在1到1.5秒查询。"
	ctx := hmc234SelectionContext(t, request)
	ctx.AnalysisIR.RequestModel.Predicates.IsCountQuestion = false
	trace := strings.Join([]string{
		`writer-10 (10) [000] .... 1.000000: print: B|10|jank_event_sync: start_ts=980000000, end_ts=990000000, jank_frames=3, appid=42`,
		`writer-10 (10) [000] .... 1.150000: print: B|10|jank_event_sync: start_ts=1100000000, end_ts=1140000000, jank_frames=4, appid=43`,
		`writer-10 (10) [000] .... 1.500000: print: B|10|jank_event_sync: start_ts=1200000000, end_ts=1440000000, jank_frames=5, appid=44`,
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "capture.systrace"), []byte(trace), 0600); err != nil {
		t.Fatal(err)
	}
	start, end := 1.0, 1.5
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1到1.5秒",
	}
	// The one typed preflight source permits the ordinary request-window
	// default. No payload predicate is synthesized from that report window.
	ctx.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{Active: true, Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: filepath.Join(ctx.RepoRoot, "capture.systrace"), Carrier: "file"}}}
	query := func(bounds, filters string) types.ToolResult {
		t.Helper()
		got, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"source":"path","path":"capture.systrace","view":"event_search",`+bounds+`"event_field_filters":`+filters+`}`))
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	threshold := `{"field":"jank_frames","op":"gte","value":"3","source_quote":"至少3帧"}`
	bad := query("", `[`+threshold+`,{"field":"start_ts","op":"gte","value":"1000000000","source_quote":"start_ts >= 1000000000"}]`)
	if bad.Success || bad.Repair == nil || len(bad.Observations) != 0 {
		t.Fatalf("extra fabricated payload condition silently narrowed the report population: %+v", bad)
	}
	good := query("", `[`+threshold+`]`)
	// event_search is a closed lookup envelope; the answer's accepted scope
	// owns half-open membership. Keep that longstanding navigation contract.
	if !good.Success || !strings.Contains(good.Summary, "matched_total=3") || !strings.Contains(good.Summary, "trace_query_request_window_inherited=") {
		t.Fatalf("actual retry must keep late reports and the inherited window: %+v", good)
	}
	// An explicitly requested business endpoint condition remains legal. Its
	// bounded-selector scope does not silently intersect the report window.
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeBoundedSelector, SourceQuote: "业务开始时间在1到1.5秒",
	}
	business := query("", `[`+threshold+`,{"field":"start_ts","op":"gte","value":"1000000000","source_quote":"业务开始时间在1到1.5秒"},{"field":"start_ts","op":"lt","value":"1500000000","source_quote":"业务开始时间在1到1.5秒"}]`)
	if !business.Success || !strings.Contains(business.Summary, "matched_total=2") {
		t.Fatalf("legal business endpoint query inherited an unrelated report window: %+v", business)
	}
}
