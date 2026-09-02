package outputdump

import (
	"encoding/json"
	"math"
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

func TestDefaultTraceRootCauseSidecarAlwaysWrites(t *testing.T) {
	for _, tc := range []struct {
		name   string
		report *types.TraceRootCauseReportV2
		reason string
	}{
		{name: "missing_selection", reason: "valid_model_root_cause_selection_unavailable"},
		{name: "no_candidates", reason: "no_selectable_typed_on_chain_candidates"},
		{name: "encode_failure", report: &types.TraceRootCauseReportV2{SchemaVersion: 2, RootCauses: []*types.TraceRootCauseItemV2{{ImpactSeconds: floatPointerForSidecarTest(math.NaN())}}}, reason: "root_cause_report_encoding_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := Args{Dir: t.TempDir(), Max: 10, HasTrace: true, Answer: "original model answer", RootCauseReport: tc.report, RootCauseUnavailableReason: tc.reason}
			result := WriteResult(a)
			body, err := os.ReadFile(result.RootCauseJSONPath)
			if err != nil {
				t.Fatalf("mandatory sidecar missing: %v", err)
			}
			var got struct {
				SchemaVersion int               `json:"schema_version"`
				RootCauses    []json.RawMessage `json:"root_causes"`
				Status        string            `json:"status"`
				Reason        string            `json:"reason_code"`
			}
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatal(err)
			}
			if got.SchemaVersion != 2 || got.RootCauses == nil || len(got.RootCauses) != 0 || got.Status != "unavailable" || got.Reason != tc.reason {
				t.Fatalf("invalid unavailable artifact: %s", body)
			}
			md, _ := os.ReadFile(result.MarkdownPath)
			if string(md) != BuildBody(a) {
				t.Fatal("sidecar changed model answer")
			}
		})
	}
}

func floatPointerForSidecarTest(v float64) *float64 { return &v }

func TestDefaultTraceRootCauseSidecarIndependentOfMarkdownFailure(t *testing.T) {
	a := Args{Dir: t.TempDir(), Max: 10, HasTrace: true, Answer: "answer", Now: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), PID: 42}
	if err := os.Mkdir(filepath.Join(a.Dir, FileName(a.Now, a.PID)), 0o755); err != nil {
		t.Fatal(err)
	}
	result := WriteResult(a)
	if result.MarkdownPath != "" {
		t.Fatal("directory should prevent markdown write")
	}
	if _, err := os.Stat(result.RootCauseJSONPath); err != nil {
		t.Fatalf("markdown failure suppressed sidecar: %v", err)
	}
}

func TestDefaultRootCauseSidecarAvailabilityAndScope(t *testing.T) {
	for _, tc := range []struct {
		name string
		args Args
		want bool
	}{
		{"explicit_empty_selection", Args{RootCauseReport: &types.TraceRootCauseReportV2{SchemaVersion: 2}}, true},
		{"typed_trace_without_attachment", Args{RequireRootCauseJSON: true}, true},
		{"source_answer_mentions_trace", Args{Request: "explain root-causes.json and trace", Answer: "trace results"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.args
			a.Dir, a.Max = t.TempDir(), 10
			result := WriteResult(a)
			if (result.RootCauseJSONPath != "") != tc.want {
				t.Fatalf("wrong scope: %+v", result)
			}
			if tc.want {
				body, err := os.ReadFile(result.RootCauseJSONPath)
				if err != nil {
					t.Fatal(err)
				}
				var got DefaultRootCauseArtifact
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatal(err)
				}
				if got.RootCauses == nil {
					t.Fatal("empty roots must be [], never null")
				}
				if tc.args.RootCauseReport != nil && (got.Status != "available" || got.ReasonCode != "") {
					t.Fatalf("explicit empty selection was relabeled: %s", body)
				}
			}
		})
	}
}

func TestMandatoryRootCauseSidecarWriteFailureIsReturned(t *testing.T) {
	a := Args{Dir: t.TempDir(), Max: 10, HasTrace: true, Answer: "model answer", Now: time.Now(), PID: 42}
	p := RootCauseJSONPathForMarkdown(filepath.Join(a.Dir, FileName(a.Now, a.PID)))
	if err := os.Mkdir(p, 0o755); err != nil {
		t.Fatal(err)
	}
	result := WriteResult(a)
	if result.RootCauseJSONPath != "" || result.RootCauseJSONError == nil || result.MarkdownPath == "" {
		t.Fatalf("must preserve answer and report file failure: %+v", result)
	}
}

func TestRootCauseOnlyRetentionKeepsFailuresAsRuns(t *testing.T) {
	a := Args{Dir: t.TempDir(), Max: 2, HasTrace: true, Now: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), PID: 42}
	first := WriteRootCauseOnly(a)
	if first.RootCauseJSONPath == "" || first.MarkdownPath != "" || first.HTMLPath != "" {
		t.Fatalf("only JSON expected: %+v", first)
	}
	// Stable mtime order without sleeps.
	if err := os.Chtimes(first.RootCauseJSONPath, a.Now, a.Now); err != nil {
		t.Fatal(err)
	}
	a.Now = a.Now.Add(time.Minute)
	second := WriteRootCauseOnly(a)
	if _, err := os.Stat(first.RootCauseJSONPath); err != nil {
		t.Fatalf("failure artifact pruned as orphan: %v", err)
	}
	a.Now = a.Now.Add(time.Minute)
	third := WriteResult(a)
	if _, err := os.Stat(first.RootCauseJSONPath); !os.IsNotExist(err) {
		t.Fatalf("oldest standalone root should honor retention: %v", err)
	}
	for _, p := range []string{second.RootCauseJSONPath, third.RootCauseJSONPath} {
		if _, err := os.Stat(p); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDefaultRootCauseSidecarRespectsExplicitDumpDisable(t *testing.T) {
	withExplicitReport(t, ExplicitReport{SuppressDefaultDir: true})
	dir := filepath.Join(t.TempDir(), "disabled")
	a := Args{Dir: dir, HasTrace: true}
	for _, result := range []Result{WriteResult(a), WriteRootCauseOnly(a), WriteRootCauseOnly(Args{HasTrace: true})} {
		if result.RootCauseJSONPath != "" || result.RootCauseJSONError != nil {
			t.Fatalf("disabled default should not write: %+v", result)
		}
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("disabled directory touched: %v", err)
	}
}
