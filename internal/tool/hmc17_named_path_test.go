package tool

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Named paths are ordinary per-run inputs, not replacement attachments. All
// tests enter the public tool boundary and use the real preparation service.
func hmc17NamedPathContext(t *testing.T) (*types.BusContext, *traceinput.Coordinator, *atomic.Int32) {
	t.Helper()
	dir := t.TempDir()
	conversions := &atomic.Int32{}
	preparer := traceinput.NewCoordinator(traceinput.Options{
		RuntimeAnchor: filepath.Join(dir, ".codrax"), PreviewBytes: 1024,
		Progress: func(event hitraceconv.ProgressEvent) {
			if event.Stage == "trace_prepare" && event.Status == hitraceconv.ProgressStatusStarted {
				conversions.Add(1)
			}
		},
	})
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("inspect selected capture"), TraceInputPreparer: preparer}
	return bus, preparer, conversions
}

func hmc17NamedQuery(t *testing.T, bus *types.BusContext, params map[string]any) types.ToolResult {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&TraceQuery{}).Execute(bus, raw)
	if err != nil {
		t.Fatalf("public trace query returned an error: %v", err)
	}
	return result
}

func hmc17NamedPayload(t *testing.T, result types.ToolResult) tracequery.Result {
	t.Helper()
	if !result.Success {
		t.Fatalf("query failed: %+v", result)
	}
	payloadRef := result.RawRef
	for _, observation := range result.Observations {
		if observation.SourceRef.PayloadRef == "" {
			continue
		}
		payloadRef = observation.SourceRef.PayloadRef
		break
	}
	if payloadRef == "" {
		t.Fatalf("successful public query did not publish a typed result: %+v", result)
	}
	body, err := os.ReadFile(payloadRef)
	if err != nil {
		t.Fatal(err)
	}
	var payload tracequery.Result
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func hmc17NamedBinaryFile(t *testing.T) (string, []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "capture.sys")
	original := hmc17NamedBinaryTailFixture()
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODRAX_TRACE_STREAMER", filepath.Join(t.TempDir(), "missing-trace-streamer"))
	return path, original
}

func TestHMC17NamedBinaryPublicViewsReuseCompleteMaterial(t *testing.T) {
	path, original := hmc17NamedBinaryFile(t)
	bus, preparer, conversions := hmc17NamedPathContext(t)
	const oldAttached = "old-owner-8 (8) [000] .... 8.000000: sched_wakeup: comm=old-target pid=9 prio=120 target_cpu=000\n"
	bus.AttachedHitrace, bus.AttachedHitraceSource = oldAttached, "android_atrace"
	for _, view := range []string{"event_search", "window_stats", "event_search"} {
		params := map[string]any{"source": "path", "path": path, "view": view, "pid": 424242, "time_start": 1.0105, "time_end": 1.012, "limit": 20}
		if view == "event_search" {
			params["pattern"] = "tail-target"
		}
		result := hmc17NamedQuery(t, bus, params)
		if !result.Success || view == "event_search" && !strings.Contains(result.Summary, "tail-target") {
			t.Fatalf("named binary %s did not query its complete tail: %+v", view, result)
		}
		payload := hmc17NamedPayload(t, result)
		if payload.TimeStart != 1.0105 || payload.TimeEnd != 1.012 {
			t.Fatalf("preparation altered explicit time bounds: %g..%g", payload.TimeStart, payload.TimeEnd)
		}
		for _, record := range result.Observations {
			if record.SourceRef.Path == path {
				t.Fatalf("text line coordinates were reassigned to binary source: %+v", record.SourceRef)
			}
		}
	}
	material, err := preparer.Prepare(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	again, err := preparer.Prepare(context.Background(), path)
	if err != nil || material != again || conversions.Load() != 1 {
		t.Fatalf("same capture was prepared repeatedly: same=%t conversions=%d err=%v", material == again, conversions.Load(), err)
	}
	if material.SourcePath() != path || material.QueryPath() == path || len(material.Preview()) > 1024 || strings.Contains(material.Preview(), "tail-target") {
		t.Fatalf("expected original identity plus complete derived query and bounded preview: %+v", material)
	}
	if bus.AttachedHitrace != oldAttached || bus.AttachedHitraceSource != "android_atrace" || bus.AttachedTraceMaterial != nil {
		t.Fatal("named input preparation overwrote sticky attachment state")
	}
	if _, err := os.Stat(filepath.Join(bus.WorkDir, types.AttachedTraceBlobBasename)); !os.IsNotExist(err) {
		t.Fatalf("named path minted an unrelated attachment blob: %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("named conversion changed original binary: %v", err)
	}
	if entries, err := os.ReadDir(filepath.Dir(path)); err != nil || len(entries) != 1 {
		t.Fatalf("named conversion wrote beside the source: entries=%v err=%v", entries, err)
	}
}

func TestHMC17NamedBinaryGenerationChangesRejectWarmResult(t *testing.T) {
	for _, changed := range []string{"source", "derived"} {
		t.Run(changed, func(t *testing.T) {
			path, _ := hmc17NamedBinaryFile(t)
			bus, preparer, conversions := hmc17NamedPathContext(t)
			params := map[string]any{"source": "path", "path": path, "view": "event_search", "pattern": "tail-target", "time_start": 1.0105, "time_end": 1.012}
			first := hmc17NamedQuery(t, bus, params)
			if !first.Success || len(first.Observations) == 0 {
				t.Fatalf("initial physical result failed: %+v", first)
			}
			material, err := preparer.Prepare(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			mutationPath := path
			if changed == "derived" {
				mutationPath = material.QueryPath()
			}
			body, err := os.ReadFile(mutationPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(mutationPath, append(body, '\n'), 0600); err != nil {
				t.Fatal(err)
			}
			result := hmc17NamedQuery(t, bus, params)
			if result.Success || len(result.Observations) != 0 || conversions.Load() != 1 {
				t.Fatalf("changed %s reused old evidence or silently reconverted: conversions=%d result=%+v", changed, conversions.Load(), result)
			}
		})
	}
}

func TestHMC17NamedMissingPathNeverSubstitutesPreparedOrAttachedCapture(t *testing.T) {
	path, _ := hmc17NamedBinaryFile(t)
	bus, _, conversions := hmc17NamedPathContext(t)
	if result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "event_search", "pattern": "tail-target"}); !result.Success {
		t.Fatalf("prepare healthy named capture: %+v", result)
	}
	bus.AttachedHitrace = "old-owner-8 (8) [000] .... 8.000000: sched_wakeup: comm=old-target pid=9 prio=120 target_cpu=000\n"
	for _, source := range []string{"path", ""} {
		result := hmc17NamedQuery(t, bus, map[string]any{"source": source, "path": filepath.Join(t.TempDir(), "missing.sys"), "view": "event_search"})
		if result.Success || len(result.Observations) != 0 || conversions.Load() != 1 {
			t.Fatalf("missing explicit source fell back to another capture: %+v", result)
		}
	}
}

func TestHMC17NamedPreparedHintsDeduplicateOnlyReceiptBoundEndpoints(t *testing.T) {
	path, original := hmc17NamedBinaryFile(t)
	bus, preparer, conversions := hmc17NamedPathContext(t)
	material, err := preparer.Prepare(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{AnalyzerHints: types.AnalyzerHints{ExactTargets: []string{path, material.QueryPath()}}}}
	result := hmc17NamedQuery(t, bus, map[string]any{"view": "event_search", "pattern": "tail-target", "time_start": 1.0105, "time_end": 1.012})
	if !result.Success || !strings.Contains(result.Summary, "tail-target") || conversions.Load() != 1 {
		t.Fatalf("one capture's original/derived hints became competing sources: conversions=%d result=%+v", conversions.Load(), result)
	}
	// Byte-identical files from different physical sources are two captures.
	// A common parent, common basename, or common converter content cannot
	// grant a source-alias receipt or make automatic selection unambiguous.
	other := filepath.Join(t.TempDir(), filepath.Base(path))
	if err := os.WriteFile(other, original, 0600); err != nil {
		t.Fatal(err)
	}
	second, err := preparer.Prepare(context.Background(), other)
	if err != nil {
		t.Fatal(err)
	}
	if second == material || second.QueryPath() == material.QueryPath() {
		t.Fatal("independent captures shared preparation identity")
	}
	bus.AnalysisIR.RequestModel.AnalyzerHints.ExactTargets = []string{path, material.QueryPath(), other, second.QueryPath()}
	result = hmc17NamedQuery(t, bus, map[string]any{"view": "event_search", "pattern": "tail-target"})
	if result.Success || len(result.Observations) != 0 || conversions.Load() != 2 {
		t.Fatalf("multiple captures were silently chosen or conflated: conversions=%d result=%+v", conversions.Load(), result)
	}
	if bus.AttachedHitrace != "" || bus.AttachedTraceMaterial != nil {
		t.Fatal("hint resolution converted a named capture into a sticky attachment")
	}
}

func TestHMC17NamedBinaryColdSupplementUsesQueryReadyMaterial(t *testing.T) {
	path, original := hmc17NamedBinaryFile(t)
	bus, preparer, conversions := hmc17NamedPathContext(t)
	material, err := preparer.Prepare(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(material.QueryPath())
	if err != nil {
		t.Fatal(err)
	}
	budget := info.Size() + 1
	if int64(len(original)) <= budget {
		t.Fatalf("fixture must distinguish binary-container size from query-ready size: source=%d query=%d", len(original), info.Size())
	}
	suppCoreSetConfig(t, true, budget, 20*time.Second, 120)
	start, end := 1.0105, 1.012
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:                      types.IntentRootCause,
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 424242, Thread: "tail-target", Source: "user_explicit", Confidence: 1}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1.0105..1.012"},
		AnalyzerHints:               types.AnalyzerHints{ExactTargets: []string{path}},
	}}
	// No model query was run: the cold lane must measure/query the material
	// already accepted by preparation, not re-open the raw RMQ as trace text.
	out := RunTraceQuerySystemSupplement(bus)
	if !out.Attempted || len(out.Executed) == 0 || out.SkipReason != "" || conversions.Load() != 1 {
		t.Fatalf("cold supplement used original binary instead of query-ready capture: conversions=%d out=%+v", conversions.Load(), out)
	}
	foundTarget := false
	for _, result := range bus.Mutable.SystemTraceSupplementResults() {
		if !result.Success {
			t.Fatalf("prepared cold supplementary query failed: %+v", result)
		}
		payload := hmc17NamedPayload(t, result)
		if payload.TimeStart != start || payload.TimeEnd != end {
			t.Fatalf("cold supplement changed explicit window: %g..%g", payload.TimeStart, payload.TimeEnd)
		}
		// This RMQ fixture records wakeups but no prior blocked interval.
		// It authorizes the selected target and observed input rows, not a
		// proven blocking dependency. The separate business fixture below
		// verifies genuine completion-closed on-chain IO.
		if payload.RootCauseRank != nil {
			foundTarget = payload.RootCauseRank.Target.PID == 424242 && payload.EventCount > 0 && payload.SourcePath != path
		}
	}
	if !foundTarget {
		t.Fatal("cold supplementary rank did not consume the prepared capture for the exact target")
	}
}

func TestHMC17NamedPathSystemSupplementPreservesExplicitWindowIOAndBusiness(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_business_io_chain", "events.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "business.systrace")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	bus, preparer, _ := hmc17NamedPathContext(t)
	start, end := 1.0, 1.05
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentRootCause, Scenario: types.ScenarioRootCause,
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Thread: "app-main", Source: "user_explicit", Confidence: 1}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1.000..1.050"},
		AnalyzerHints:               types.AnalyzerHints{ExactTargets: []string{path}},
	}}
	result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "event_search", "pid": 100, "time_start": start, "time_end": end})
	if !result.Success {
		t.Fatalf("lightweight named-path model query failed: %+v", result)
	}
	bus.ToolResults = append(bus.ToolResults, result)
	material, err := preparer.Prepare(context.Background(), path)
	if err != nil || material == nil || strings.Contains(material.Preview(), "block_rq_complete") {
		t.Fatalf("fixture must use complete material beyond its preview: material=%+v err=%v", material, err)
	}
	before, _ := json.Marshal(bus.ToolResults)
	out := RunTraceQuerySystemSupplement(bus)
	if !out.Attempted || len(out.Executed) == 0 || out.SkipReason != "" {
		t.Fatalf("named-path explicit-window automatic completion failed: %+v", out)
	}
	foundIO, foundBusiness := false, false
	for _, supplemented := range bus.Mutable.SystemTraceSupplementResults() {
		if !supplemented.Success {
			continue
		}
		payload := hmc17NamedPayload(t, supplemented)
		if payload.TimeStart != start || payload.TimeEnd != end {
			t.Fatalf("supplement replaced the requested window: %g..%g", payload.TimeStart, payload.TimeEnd)
		}
		if payload.RootCauseRank != nil {
			for _, item := range payload.RootCauseRank.Items {
				if item.Thread.PID == 900 && item.ChainRelevance == "on_chain" {
					t.Fatalf("background IO became an on-chain root: %+v", item)
				}
				foundIO = foundIO || item.Type == "io_latency" && item.Thread.PID == 200 && item.ChainRelevance == "on_chain" && item.ResourceCompletionClosure && math.Abs(item.EffectiveImpactMs-31) < 1e-6
			}
		}
		for _, record := range supplemented.Observations {
			foundBusiness = foundBusiness || strings.Contains(record.Subject, "LoadDocumentIndex") || strings.Contains(strings.Join(record.RichNotes, "\n"), "LoadDocumentIndex") || strings.Contains(record.Object, "LoadDocumentIndex")
		}
	}
	if !foundIO || !foundBusiness {
		t.Fatalf("automatic completion lost completion-closed IO or business context: IO=%t business=%t outcome=%+v", foundIO, foundBusiness, out)
	}
	after, _ := json.Marshal(bus.ToolResults)
	if !bytes.Equal(before, after) || bus.AttachedHitrace != "" || bus.AttachedTraceMaterial != nil {
		t.Fatal("system completion modified original tool results or sticky attachment")
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, body) {
		t.Fatalf("named-path query/supplement modified its source: %v", err)
	}
}

// Real RMQ: twelve physical pages put the last typed wakeup beyond the
// bounded preview. This is the same wire layout as the CLI/REPL acceptance
// fixture, not a converter substitute or prebuilt material receipt.
func hmc17NamedBinaryTailFixture() []byte {
	var out bytes.Buffer
	header := make([]byte, 12)
	binary.LittleEndian.PutUint16(header[0:2], 0x0ace)
	header[2] = 1
	binary.LittleEndian.PutUint16(header[4:6], 1)
	binary.LittleEndian.PutUint32(header[8:12], 2)
	out.Write(header)
	segment := func(kind uint32, data []byte) {
		var hdr [8]byte
		binary.LittleEndian.PutUint32(hdr[:4], kind)
		binary.LittleEndian.PutUint32(hdr[4:], uint32(len(data)))
		out.Write(hdr[:])
		out.Write(data)
	}
	format := "name: sched_wakeup\nID: 10\nformat:\n" +
		"\tfield:unsigned short common_type;\toffset:0;\tsize:2;\tsigned:0;\n" +
		"\tfield:unsigned char common_flags;\toffset:2;\tsize:1;\tsigned:0;\n" +
		"\tfield:unsigned char common_preempt_count;\toffset:3;\tsize:1;\tsigned:0;\n" +
		"\tfield:int common_pid;\toffset:4;\tsize:4;\tsigned:1;\n" +
		"\tfield:char comm[16];\toffset:8;\tsize:16;\tsigned:0;\n" +
		"\tfield:int pid;\toffset:24;\tsize:4;\tsigned:1;\n" +
		"\tfield:int prio;\toffset:28;\tsize:4;\tsigned:1;\n" +
		"\tfield:int target_cpu;\toffset:32;\tsize:4;\tsigned:1;\n" +
		"print fmt: \"comm=%s pid=%d prio=%d target_cpu=%03d\"\n"
	segment(1, []byte(format))
	segment(2, []byte("12 worker\n"))
	segment(3, []byte("12 12\n"))
	var pages bytes.Buffer
	for i := 0; i < 12; i++ {
		page := make([]byte, 4096)
		binary.LittleEndian.PutUint64(page[0:8], uint64(1_000_000_000+i*1_000_000))
		binary.LittleEndian.PutUint64(page[8:16], 42)
		binary.LittleEndian.PutUint16(page[21:23], 36)
		payload := page[23:59]
		binary.LittleEndian.PutUint16(payload[:2], 10)
		binary.LittleEndian.PutUint32(payload[4:8], 12)
		name, pid := "head-worker", uint32(42)
		if i == 11 {
			name, pid = "tail-target", 424242
		}
		copy(payload[8:24], name)
		binary.LittleEndian.PutUint32(payload[24:28], pid)
		binary.LittleEndian.PutUint32(payload[28:32], 120)
		pages.Write(page)
	}
	segment(4, pages.Bytes())
	return out.Bytes()
}
