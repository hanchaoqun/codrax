package tool

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func structuredPayloadContractFiles() []string {
	files := []string{
		"emit_analysis.go",
		"emit_evidence.go",
		"emit_investigation_complete.go",
		"emit_log_triage.go",
		"emit_answer_document_v2.go",
		"emit_answer_document_patch.go",
		"emit_answer_symbol.go",
		"emit_hypothesis_verdict.go",
		"emit_log_segmentation.go",
		"emit_perf_trace.go",
		"emit_perf_segmentation.go",
	}
	return append(files, writeModeToolContractFiles()...)
}

func writeModeToolContractFiles() []string {
	return []string{
		"emit_write_analysis.go",
		"emit_change_plan.go",
		"emit_plan_skeleton.go",
		"emit_plan_change.go",
		"emit_write_workflow_decision.go",
		"apply_patch.go",
		"emit_test_results.go",
		"run_tests.go",
	}
}

func TestStructuredPayloadCompatCoverageForStructuredEmitTools(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(currentFile)
	for _, name := range structuredPayloadContractFiles() {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		src := string(raw)
		if !strings.Contains(src, "applyStructuredPayloadCompat(") &&
			!strings.Contains(src, "applyStructuredPayloadCompatWithLegacyStringFieldRepair(") &&
			!strings.Contains(src, "decodeStrictToolParams(") {
			t.Fatalf("%s must route structured tool payloads through applyStructuredPayloadCompat", name)
		}
	}
}

func TestStructuredEmitToolsAttachTypedDecodeRepair(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(currentFile)
	for _, name := range structuredPayloadContractFiles() {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		src := string(raw)
		if !strings.Contains(src, "failStrictDecode") &&
			!strings.Contains(src, "decodeStrictToolParams(") &&
			!strings.Contains(src, "decodeStrictNormalizedToolParams(") {
			t.Fatalf("%s must attach typed ToolRepair metadata on structured JSON decode failures", name)
		}
	}
}

func TestTraceQueryRoutesThroughCompatAndTypedDecodeRepair(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(currentFile), "trace_query.go"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	if !strings.Contains(src, "applyStructuredPayloadCompat(") {
		t.Fatalf("trace_query must route tool payloads through applyStructuredPayloadCompat")
	}
	if !strings.Contains(src, "failStrictDecodeWithError(") {
		t.Fatalf("trace_query must attach typed ToolRepair metadata on JSON decode failures")
	}
}
