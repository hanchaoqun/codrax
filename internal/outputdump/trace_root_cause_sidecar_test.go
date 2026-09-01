package outputdump

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestWriteResultWritesSeparateRootCauseJSONWithoutChangingMarkdown(t *testing.T) {
	dir := t.TempDir()
	answer := "# 完整分析\n\n这是原本的长答案。\n"
	impactSeconds := 0.0124
	report := &types.TraceRootCauseReportV2{
		SchemaVersion: types.TraceRootCauseReportSchemaVersion,
		RootCauses: []*types.TraceRootCauseItemV2{{
			Rank:          1,
			Category:      types.TraceRootCauseCPUSchedulingDelay,
			ThreadName:    "RenderThread",
			ImpactSeconds: &impactSeconds,
			Summary:       "RenderThread线程CPU调度延迟",
			Evidence:      []string{"runnable 12.4 ms，期间未获得 CPU"},
		}},
	}
	result := WriteResult(Args{
		Dir: dir, Max: 10, Language: "zh", Request: "分析 trace", Answer: answer,
		RootCauseReport: report, Now: time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local), PID: 42,
	})
	if result.MarkdownPath == "" || result.RootCauseJSONPath == "" {
		t.Fatalf("missing output paths: %+v", result)
	}
	markdown, err := os.ReadFile(result.MarkdownPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(markdown), answer) || strings.Contains(string(markdown), "root_causes") {
		t.Fatalf("JSON leaked into or replaced the full answer:\n%s", markdown)
	}
	if filepath.Dir(result.MarkdownPath) != filepath.Dir(result.RootCauseJSONPath) {
		t.Fatalf("sidecar is not next to markdown: %+v", result)
	}
	encoded, err := os.ReadFile(result.RootCauseJSONPath)
	if err != nil {
		t.Fatal(err)
	}
	var got types.TraceRootCauseReportV2
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("sidecar is not valid JSON: %v", err)
	}
	if len(got.RootCauses) != 1 || got.RootCauses[0].Rank != 1 || got.RootCauses[0].Summary != "RenderThread线程CPU调度延迟" ||
		got.RootCauses[0].ImpactSeconds == nil || *got.RootCauses[0].ImpactSeconds != impactSeconds {
		t.Fatalf("unexpected sidecar: %#v", got)
	}
}
