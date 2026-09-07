package tracediag

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestB1598EventSearchReportKeepsThreeIndependentCounts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.systrace")
	var trace strings.Builder
	for i := 0; i < 1639; i++ {
		fmt.Fprintf(&trace, "app-20 (20) [000] .... 6793224.%06d: print: B|20|needle\n", i)
	}
	if err := os.WriteFile(path, []byte(trace.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := tracequery.StreamEventSearch(context.Background(), path, tracequery.Query{View: "event_search", Pattern: "needle", Limit: 40})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(result)
	step := &Step{View: "event_search", effMaxLines: 5, windowOrigin: &WindowProvenance{}}
	body := renderStepBody(step, stepOutcome{result: &result})
	after, _ := json.Marshal(result)
	if string(before) != string(after) {
		t.Fatal("reader formatting changed query result")
	}
	if len(body.lines) > 5 || body.eventSearch == nil || body.eventSearch.emitted != 2 {
		t.Fatalf("existing bounded report floor changed: %+v", body)
	}
	report := strings.Join(body.lines, "\n")
	for _, want := range []string{"matched_total=1639 engine_emitted=40 enumeration_complete=true", "匹配统计已完成", "引擎返回 40 / 1639 条匹配记录", "本报告展示 2 / 40 条已返回记录", "统计完成不代表匹配记录已全部返回或展示"} {
		if !strings.Contains(report, want) {
			t.Errorf("three-layer report missing %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "e+06") || strings.Contains(report, "明细 event_search_coverage") {
		t.Fatalf("key-first coverage regressed:\n%s", report)
	}
}
