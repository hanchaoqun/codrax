package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestEmitLogTriage_Schema_RejectsUnknownField_AtRoot verifies the
// additionalProperties:false lock on the root object. A "foo" field
// the LLM might invent to add metadata is not valid JSON Schema
// territory here — LLM providers that honour the schema will refuse
// to generate it, and any that slip through get rejected on unmarshal
// via the Go type's absence of a matching field (silent-ignore, since
// the params struct is not strict-mode).
//
// The test pins the SCHEMA itself rather than unmarshal behaviour —
// the schema is what providers validate against.
func TestEmitLogTriage_Schema_RejectsUnknownField_AtRoot(t *testing.T) {
	tool := &EmitLogTriage{}
	raw := tool.Parameters()

	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("schema unmarshal: %v", err)
	}
	if got := schema["additionalProperties"]; got != false {
		t.Errorf("root additionalProperties = %v, want false", got)
	}
	required, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("required is %T, want []any", schema["required"])
	}
	wantReq := map[string]bool{"meta": true, "errors": true}
	for _, r := range required {
		if s, ok := r.(string); ok {
			delete(wantReq, s)
		}
	}
	if len(wantReq) > 0 {
		t.Errorf("missing required: %v", wantReq)
	}
}

// TestEmitLogTriage_Schema_FrameLocksRawAndConfidence pins that
// every frame must carry raw + confidence.
func TestEmitLogTriage_Schema_FrameLocksRawAndConfidence(t *testing.T) {
	tool := &EmitLogTriage{}
	raw := tool.Parameters()
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}

	// Navigate: properties.errors.items.properties.frames.items (frame schema).
	errs := schema["properties"].(map[string]any)["errors"].(map[string]any)
	errItem := errs["items"].(map[string]any)
	framesNode := errItem["properties"].(map[string]any)["frames"].(map[string]any)
	frameItem := framesNode["items"].(map[string]any)
	if frameItem["additionalProperties"] != false {
		t.Errorf("frame additionalProperties not locked")
	}
	req, _ := frameItem["required"].([]any)
	wantReq := map[string]bool{"raw": true, "confidence": true}
	for _, r := range req {
		if s, ok := r.(string); ok {
			delete(wantReq, s)
		}
	}
	if len(wantReq) > 0 {
		t.Errorf("frame required missing: %v", wantReq)
	}
}

// TestEmitLogTriage_Schema_HasCauseRecursionUpToCap verifies the
// schema recursion depth matches LogBundleCaps.MaxCauseDepth. We walk
// into Cause > Cause > ... and count how deep the property exists.
func TestEmitLogTriage_Schema_HasCauseRecursionUpToCap(t *testing.T) {
	tool := &EmitLogTriage{}
	raw := tool.Parameters()
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	errs := schema["properties"].(map[string]any)["errors"].(map[string]any)
	cur := errs["items"].(map[string]any)

	depth := 1
	for {
		props, _ := cur["properties"].(map[string]any)
		cause, ok := props["cause"].(map[string]any)
		if !ok {
			break
		}
		depth++
		cur = cause
	}
	if depth != types.LogBundleCaps.MaxCauseDepth {
		t.Errorf("schema cause depth = %d, want %d",
			depth, types.LogBundleCaps.MaxCauseDepth)
	}
}

// TestEmitLogTriage_Execute_HappyPath builds a repo with one real file
// + calls emit_log_triage with a bundle pointing at it and verifies:
// Mutable.LogTriage is populated, ResolvedFiles contains the real
// file, hallucinated frame is dropped from ResolvedFiles but kept in
// bundle.Errors.
func TestEmitLogTriage_Execute_HappyPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "internal", "svc", "handler.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("package svc\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	bus := &types.BusContext{
		RepoRoot: dir,
		Mutable:  types.NewMutableState("test"),
		AttachedLog: "panic: runtime error\n\n" +
			"goroutine 1 [running]:\nsvc.Handler(\"/x\")\n" +
			"\tinternal/svc/handler.go:42 +0x1e\n",
	}

	params, err := json.Marshal(emitLogTriageParams{
		Meta: emitLogTriageMeta{
			Lang:    "go",
			Signals: []string{"panic"},
			Summary: "runtime error in svc.Handler",
		},
		Errors: []emitLogTriageError{
			{
				Type: "runtime error: index out of range",
				Frames: []emitLogTriageFrame{
					{File: "internal/svc/handler.go", Line: 42,
						Func: "svc.Handler", Raw: "real frame", Confidence: 0.9},
					{File: "internal/fake/nope.go", Line: 10,
						Func: "fake.Nope", Raw: "fake frame", Confidence: 0.9},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	tool := &EmitLogTriage{}
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("success=false, summary=%s", res.Summary)
	}
	bundle := bus.Mutable.LogTriage()
	if bundle == nil {
		t.Fatal("bundle nil")
	}
	want := []string{"internal/svc/handler.go"}
	if len(bundle.ResolvedFiles) != 1 || bundle.ResolvedFiles[0] != want[0] {
		t.Errorf("ResolvedFiles = %v, want %v", bundle.ResolvedFiles, want)
	}
	if bundle.IntentHint != types.IntentRootCause {
		t.Errorf("IntentHint = %q, want root_cause", bundle.IntentHint)
	}
	// Summary should include the before→after frame count.
	if !strings.Contains(res.Summary, "frames=2→1") {
		t.Errorf("summary missing frame diff: %q", res.Summary)
	}
}

// TestEmitLogTriage_Execute_EmptyBundleRejected confirms the
// cross-field validator: errors[] empty AND unknown_chunks empty =>
// rejection.
func TestEmitLogTriage_Execute_EmptyBundleRejected(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("test")}
	params, _ := json.Marshal(emitLogTriageParams{
		Meta: emitLogTriageMeta{Lang: "go", Signals: []string{"other"}},
	})
	tool := &EmitLogTriage{}
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatal(err)
	}
	if res.Success {
		t.Error("expected rejection for empty bundle")
	}
	if !strings.Contains(res.Summary, "empty") {
		t.Errorf("summary not informative: %q", res.Summary)
	}
}

// TestEmitLogTriage_Execute_NilMutableRejected covers the sub-agent
// guard (same pattern as emit_analysis).
func TestEmitLogTriage_Execute_NilMutableRejected(t *testing.T) {
	bus := &types.BusContext{}
	tool := &EmitLogTriage{}
	res, err := tool.Execute(bus, json.RawMessage(`{"meta":{"lang":"go","signals":[]},"errors":[{"type":"X"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.Success {
		t.Error("expected rejection without Mutable")
	}
}

// TestEmitLogTriage_IsReadOnly confirms the tool cannot write to the
// filesystem (sub-agent permission contract).
func TestEmitLogTriage_IsReadOnly(t *testing.T) {
	tool := &EmitLogTriage{}
	if tool.IsWrite() {
		t.Error("emit_log_triage must be ReadOnly")
	}
	if tool.Confidence() != 0.0 {
		t.Errorf("Confidence = %v, want 0.0 (NonEvidenceTool)", tool.Confidence())
	}
}

// TestEmitLogSegmentation_SchemaLocked verifies the segmentation
// tool's schema.
func TestEmitLogSegmentation_SchemaLocked(t *testing.T) {
	tool := &EmitLogSegmentation{}
	raw := tool.Parameters()
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	if schema["additionalProperties"] != false {
		t.Error("root not locked")
	}
	props := schema["properties"].(map[string]any)
	segs := props["segments"].(map[string]any)
	item := segs["items"].(map[string]any)
	if item["additionalProperties"] != false {
		t.Error("segment item not locked")
	}
	req := item["required"].([]any)
	want := map[string]bool{"byte_start": true, "byte_end": true, "kind": true}
	for _, r := range req {
		if s, ok := r.(string); ok {
			delete(want, s)
		}
	}
	if len(want) > 0 {
		t.Errorf("segment required missing: %v", want)
	}
}

// TestEmitLogSegmentation_Execute_StoresValidSegments verifies
// coordinate validation + storage on MutableState.
func TestEmitLogSegmentation_Execute_StoresValidSegments(t *testing.T) {
	bus := &types.BusContext{
		AttachedLog: strings.Repeat("x", 200),
		Mutable:     types.NewMutableState("test"),
	}

	params, _ := json.Marshal(emitLogSegmentationParams{
		Segments: []LogSegment{
			{ByteStart: 0, ByteEnd: 100, Kind: "stack"},
			{ByteStart: 100, ByteEnd: 200, Kind: "trace"},
			{ByteStart: 300, ByteEnd: 400, Kind: "stack"}, // out of bounds → drop
			{ByteStart: 50, ByteEnd: 40, Kind: "stack"},   // reversed → drop
		},
	})

	tool := &EmitLogSegmentation{}
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("success=false, summary=%s", res.Summary)
	}
	stored := bus.Mutable.LogSegments()
	if len(stored) == 0 {
		t.Fatal("segments not stored")
	}
	var back []LogSegment
	if err := json.Unmarshal(stored, &back); err != nil {
		t.Fatal(err)
	}
	if len(back) != 2 {
		t.Errorf("kept = %d, want 2", len(back))
	}
}

func TestEmitLogSegmentation_Execute_ClampsSlightlyOutOfBoundsSegments(t *testing.T) {
	bus := &types.BusContext{
		AttachedLog: strings.Repeat("x", 200),
		Mutable:     types.NewMutableState("test"),
	}

	params, _ := json.Marshal(emitLogSegmentationParams{
		Segments: []LogSegment{
			{ByteStart: 150, ByteEnd: 205, Kind: "stack"},
		},
	})

	tool := &EmitLogSegmentation{}
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("success=false, summary=%s", res.Summary)
	}
	var back []LogSegment
	if err := json.Unmarshal(bus.Mutable.LogSegments(), &back); err != nil {
		t.Fatal(err)
	}
	if len(back) != 1 {
		t.Fatalf("kept = %d, want 1", len(back))
	}
	if back[0].ByteEnd != 200 {
		t.Fatalf("ByteEnd = %d, want 200", back[0].ByteEnd)
	}
	if !strings.Contains(res.Summary, "normalized=1") {
		t.Fatalf("expected normalized summary, got %q", res.Summary)
	}
}

func TestEmitLogSegmentation_Execute_RejectsWithCoordinateRepair(t *testing.T) {
	bus := &types.BusContext{
		AttachedLog: strings.Repeat("x", 50),
		Mutable:     types.NewMutableState("test"),
	}

	params, _ := json.Marshal(emitLogSegmentationParams{
		Segments: []LogSegment{
			{ByteStart: 75, ByteEnd: 90, Kind: "stack"},
		},
	})

	tool := &EmitLogSegmentation{}
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatal(err)
	}
	if res.Success {
		t.Fatalf("expected rejection, got success summary=%q", res.Summary)
	}
	if res.Repair == nil || res.Repair.Code != "segmentation_coordinates" {
		t.Fatalf("expected segmentation coordinate repair, got %+v", res.Repair)
	}
	if !strings.Contains(res.Summary, "attachment_bytes=50") {
		t.Fatalf("expected attachment size in summary, got %q", res.Summary)
	}
}
