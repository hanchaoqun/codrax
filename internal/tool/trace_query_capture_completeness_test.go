package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryCaptureCompletenessIsHeadPreviewSafe(t *testing.T) {
	capture := "tracebundle_trace_db_coverage family=capture_completeness table=stat role=capture_completeness found=true capture_state=parser_self_audit_degraded capture_scope=trace_streamer_parser_self_audit capture_absence_policy=require_quality_caveat capture_positive_evidence=preserve capture_data_lost=2"
	result := tracequery.Result{
		View:       "window_stats",
		SourcePath: "/trace/capture.tracebundle.json",
		Caveats:    []string{capture},
	}
	for i := 0; i < 300; i++ {
		result.Caveats = append(result.Caveats, fmt.Sprintf("large_following_caveat_%03d=%s", i, strings.Repeat("x", 256)))
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "window_stats"}, "path", "payload.json")
	if strings.Count(summary, capture) != 1 {
		t.Fatalf("capture caveat must be hoisted exactly once: count=%d", strings.Count(summary, capture))
	}
	preview, ref := StoreBlob(&types.BusContext{WorkDir: t.TempDir()}, "trace_query", summary)
	if ref == "" {
		t.Fatal("fixture did not cross the blob preview boundary")
	}
	if !strings.Contains(preview, "capture_completeness="+capture) {
		t.Fatalf("capture completeness fell into the hidden preview middle:\n%s", preview)
	}
}

func TestTraceQueryExecutePublishesCaptureCompletenessWithoutDroppingEvents(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "capture.systrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	if err := os.WriteFile(systrace, []byte(`app-20 (20) [001] .... 10.000000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001
`), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "version":"test",
  "systrace":"capture.systrace",
  "artifacts":[{"type":"systrace","path":"capture.systrace"}],
  "trace_db_coverage":[{
    "family":"capture_completeness","table":"stat","role":"capture_completeness","found":true,"rows_read":5,
    "capture_completeness":{
      "state":"parser_self_audit_degraded","rows_accepted":5,"received":1,"not_match":1,"warn_issues":1,
      "nonzero_issue_rows":1,
      "issues":[{"event_name":"sched_wakeup","stat_type":"not_match","count":1,"source":"trace","severity":"warn"}]
    }
  }]
}`
	writeToolTraceBundleV2Fixture(t, bundle, []byte(manifest))
	params, err := json.Marshal(map[string]any{
		"source":     "path",
		"path":       bundle,
		"view":       "window_stats",
		"time_start": 9.999,
		"time_end":   10.001,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("trace_query failed: %s", result.Summary)
	}
	for _, want := range []string{
		"capture_completeness=tracebundle_trace_db_coverage family=capture_completeness table=stat role=capture_completeness",
		"capture_state=parser_self_audit_degraded",
		"capture_not_match=1",
		"capture_positive_evidence=preserve",
		"parsed_events=1",
	} {
		if !strings.Contains(result.Summary, want) {
			t.Fatalf("tool result missing %q:\n%s", want, result.Summary)
		}
	}
}
