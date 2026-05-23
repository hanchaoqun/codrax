package tool

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStructuredPayloadCompatCoverageForStructuredEmitTools(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(currentFile)
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
	for _, name := range files {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !strings.Contains(string(raw), "applyStructuredPayloadCompat(") {
			t.Fatalf("%s must route structured tool payloads through applyStructuredPayloadCompat", name)
		}
	}
}
