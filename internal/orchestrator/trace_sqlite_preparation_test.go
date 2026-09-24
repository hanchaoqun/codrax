package orchestrator

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRunNamedSQLiteUsesSharedPreparationAndPublicQuery(t *testing.T) {
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	original, err := os.ReadFile("../../eval/fixtures/hmosperf_existing_sqlite/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repo, "named trace.htrace")
	if err := os.WriteFile(path, original, 0o400); err != nil {
		t.Fatal(err)
	}
	analyzerCalls, otherCalls := 0, 0
	var events []render.Event
	o := newTypedNamedTraceAdmissionTestOrchestrator([]string{path}, &analyzerCalls, &otherCalls, &events)
	o.SetTraceRuntimeAnchor(t.TempDir())
	bus, _ := o.Run("只分析 `"+path+"` 中的业务打点，不分析代码", repo, "main")
	if bus == nil || bus.TraceInputPreparer == nil || !hasTraceAdmissionEventKind(events, render.EventAnalysisReady) {
		t.Fatalf("named SQLite failed typed admission: bus=%v events=%v", bus != nil, events)
	}
	materials := bus.TraceInputPreparer.PreparedMaterials()
	canonicalPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(materials) != 1 || materials[0].SourcePath() != canonicalPath || bus.AttachedTraceMaterial != nil || bus.AttachedHitrace != "" {
		t.Fatal("named SQLite missing, duplicated or became sticky attachment")
	}
	// The real query tool uses the same process-local preparer, not a second
	// conversion or a byte-limited analyzer preview. Use a live output directory
	// because this fixture orchestrator already returned from its stub pipeline.
	bus.WorkDir = t.TempDir()
	params, err := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "pattern": "tail-business-marker", "time_start": 2.98, "time_end": 3.0, "limit": 10})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&tool.TraceQuery{}).Execute(bus, params)
	if err != nil || !result.Success || !strings.Contains(result.Summary, "tail-business-marker") {
		t.Fatalf("named public query failed: %+v %v", result, err)
	}
	if after := bus.TraceInputPreparer.PreparedMaterials(); len(after) != 1 || after[0] != materials[0] {
		t.Fatal("query did not reuse typed pre-entry material")
	}
	idx, err := tracequery.BuildIndex(t.Context(), materials[0].QueryPath())
	if err != nil {
		t.Fatal(err)
	}
	spans, _ := tracequery.FindSpanWindows(idx, tracequery.Query{SpanName: "OpenDocument"}, 10)
	if len(spans) != 1 || spans[0].StartTs != 1 || spans[0].EndTs != 1.05 {
		t.Fatalf("business window lost in SQLite normalization: %+v", spans)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, original) {
		t.Fatal("original SQLite changed", err)
	}
}

func TestTypedNamedSQLiteNonTraceSuffixAdmission(t *testing.T) {
	data, err := os.ReadFile("../../eval/fixtures/hmosperf_existing_sqlite/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "named.capture")
	if err := os.WriteFile(path, data, 0o400); err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{
		TraceInputPreparer:       traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: t.TempDir()}),
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: path, Carrier: "request_path"}}},
		AnalysisIR:               &types.AnalysisIR{RequestModel: types.RequestModel{ExternalObservationPolicy: &types.ExternalObservationPolicy{CurrentSourceMode: types.ExternalObservationCurrentSourceExclude, ExclusionKind: types.ExternalObservationSourceExclusionExplicitUserBoundary, SourceQuotes: []string{"only the trace"}}}},
	}
	if err := validateTypedNamedTraceInputsBeforeExploration(t.Context(), bus, ""); err != nil {
		t.Fatal(err)
	}
	canonicalPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	if materials := bus.TraceInputPreparer.PreparedMaterials(); len(materials) != 1 || materials[0].SourcePath() != canonicalPath {
		t.Fatal("precise trace identity did not prepare content-recognized DB")
	}
}
