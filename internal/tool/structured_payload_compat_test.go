package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStructuredPayloadCompatRepairsUniqueNestedBooleanDuplicateGenerically(t *testing.T) {
	schema := json.RawMessage(`{
		"type":"object",
		"properties":{
			"name":{"type":"string"},
			"decision_profile":{"type":"object","properties":{"enabled":{"type":"boolean"}}}
		}
	}`)
	raw := json.RawMessage(`{"name":"x","enabled":"true","decision_profile":{"enabled":true}}`)
	got := applyStructuredPayloadCompat("synthetic_decision", raw, schema)
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(got, &obj); err != nil {
		t.Fatalf("repaired payload invalid: %v\n%s", err, got)
	}
	if _, exists := obj["enabled"]; exists {
		t.Fatalf("redundant top-level decision survived: %s", got)
	}
	if string(obj["decision_profile"]) != `{"enabled":true}` {
		t.Fatalf("canonical nested decision changed: %s", got)
	}
}

func TestStructuredPayloadCompatNestedBooleanDuplicateFailsClosedWhenAmbiguousOrConflicting(t *testing.T) {
	uniqueSchema := json.RawMessage(`{"type":"object","properties":{"decision_profile":{"type":"object","properties":{"enabled":{"type":"boolean"}}}}}`)
	ambiguousSchema := json.RawMessage(`{"type":"object","properties":{"left":{"type":"object","properties":{"enabled":{"type":"boolean"}}},"right":{"type":"object","properties":{"enabled":{"type":"boolean"}}}}}`)
	for _, tc := range []struct {
		name   string
		schema json.RawMessage
		raw    json.RawMessage
	}{
		{"conflict", uniqueSchema, json.RawMessage(`{"enabled":true,"decision_profile":{"enabled":false}}`)},
		{"missing-canonical", uniqueSchema, json.RawMessage(`{"enabled":true}`)},
		{"nested-not-native-bool", uniqueSchema, json.RawMessage(`{"enabled":"false","decision_profile":{"enabled":"false"}}`)},
		{"invalid-legacy-bool", uniqueSchema, json.RawMessage(`{"enabled":"maybe","decision_profile":{"enabled":false}}`)},
		{"ambiguous-schema-path", ambiguousSchema, json.RawMessage(`{"enabled":true,"left":{"enabled":true},"right":{"enabled":true}}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, fields, ok := repairRedundantUniqueNestedBooleanFields(tc.raw, tc.schema)
			if ok || len(fields) != 0 || string(got) != string(tc.raw) {
				t.Fatalf("ambiguous/conflicting payload must pass through for strict rejection: ok=%v fields=%v\nraw=%s\ngot=%s", ok, fields, tc.raw, got)
			}
		})
	}
}

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
			!strings.Contains(src, "applyStructuredPayloadCompatWithSelectedStringFieldRepair(") &&
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
	// LT-HYG decoder-remap hint (§29.75 立案, 2026-07-14): trace_query now
	// rides the schema-aware variant of the SAME typed-repair failure path
	// (failStrictDecodeWithErrorSchema → strictDecodeFailure), so its
	// fabricated-field rejections teach the reflected parameter list.
	if !strings.Contains(src, "failStrictDecodeWithError(") &&
		!strings.Contains(src, "failStrictDecodeWithErrorSchema(") {
		t.Fatalf("trace_query must attach typed ToolRepair metadata on JSON decode failures")
	}
}
