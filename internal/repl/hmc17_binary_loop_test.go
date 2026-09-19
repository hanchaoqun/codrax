package repl

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// These are entry-point acceptance tests: no private handler invocation, no
// seeded receipt, and no converter override. A real file enters through the
// same public constructor and command loop as scripted REPL users.
func hmc17LoadTraceThroughLoop(t *testing.T, command, path string, previewBytes int) (*materialAwareRunner, *RuntimeArtifactStore, string, string) {
	t.Helper()
	runtimeAnchor := filepath.Join(t.TempDir(), ".codrax")
	store := NewRuntimeArtifactStore(filepath.Join(runtimeAnchor, "artifacts"))
	memoryStore, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatal(err)
	}
	runner := &materialAwareRunner{}
	var output bytes.Buffer
	r := New(Config{
		Runner: runner, Store: memoryStore, Render: renderNothing,
		RepoRoot: t.TempDir(), Language: "en", RuntimeAnchor: runtimeAnchor,
		RuntimeArtifactStore: store, AttachedTraceMaxBytes: previewBytes,
		In: strings.NewReader(command + " " + path + "\ninspect the attached trace\n/exit\n"), Out: &output,
	})
	if err := r.Loop(); err != nil {
		t.Fatalf("public REPL loop failed: %v\n%s", err, output.String())
	}
	if len(runner.seenMaterials) != 1 || len(runner.seenTraces) != 1 {
		t.Fatalf("attachment command did not preserve the following request: materials=%d traces=%d\n%s", len(runner.seenMaterials), len(runner.seenTraces), output.String())
	}
	return runner, store, runtimeAnchor, output.String()
}

func hmc17AttachedToolContext(t *testing.T, runner *materialAwareRunner) *types.BusContext {
	t.Helper()
	material := runner.seenMaterials[0]
	if material == nil {
		t.Fatal("file attachment reached Run without prepared complete material")
	}
	if err := material.Validate(context.Background(), runner.seenTraces[0]); err != nil {
		t.Fatalf("dispatched material is not valid: %v", err)
	}
	dir := t.TempDir()
	bus := &types.BusContext{
		RepoRoot: dir, WorkDir: dir, AttachedHitrace: runner.seenTraces[0],
		AttachedHitraceSource: runner.seenSources[0], AttachedTraceMaterial: material,
		Mutable: types.NewMutableState("inspect complete attached trace"),
	}
	projected := types.ToolBusContext(promptctx.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore), types.AgentExplorer)
	if projected.AttachedTraceMaterial != material {
		t.Fatal("agent-to-tool projection dropped the REPL material receipt")
	}
	return projected
}

func TestHMC17REPLLoopBinaryAliasesQueryCompleteTail(t *testing.T) {
	for _, test := range []struct{ command, source string }{
		{"/htrace", "harmony_hitrace"}, {"/atrace", "android_atrace"},
	} {
		t.Run(strings.TrimPrefix(test.command, "/"), func(t *testing.T) {
			sourceDir := t.TempDir()
			path := filepath.Join(sourceDir, "capture.sys")
			original := hmc17BinaryTraceTailFixture()
			if err := os.WriteFile(path, original, 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("CODRAX_TRACE_STREAMER", filepath.Join(t.TempDir(), "missing-trace-streamer"))
			runner, store, runtimeAnchor, output := hmc17LoadTraceThroughLoop(t, test.command, path, 1024)
			if runner.seenMaterials[0] == nil {
				t.Fatalf("raw binary default attachment was not prepared:\n%s", output)
			}
			material := runner.seenMaterials[0]
			if len(material.Preview()) > 1024 || strings.Contains(material.Preview(), "tail-target") || !strings.Contains(material.Preview(), "truncated") {
				t.Fatalf("expected bounded head preview, got %q", material.Preview())
			}
			if material.SourcePath() != path || material.QueryPath() == path || material.SelfContainedText() || runner.seenSources[0] != test.source {
				t.Fatalf("lost binary derivation/source: original=%q query=%q source=%q", material.SourcePath(), material.QueryPath(), runner.seenSources[0])
			}
			canonicalAnchor, err := filepath.EvalSymlinks(runtimeAnchor)
			if err != nil || !strings.HasPrefix(material.QueryPath(), canonicalAnchor+string(filepath.Separator)) {
				t.Fatalf("converted material escaped the configured runtime anchor: query=%q anchor=%q err=%v", material.QueryPath(), canonicalAnchor, err)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(material.QueryPath()), "preparation.json")); err != nil {
				t.Fatalf("conversion receipt is missing: %v", err)
			}
			if entries, err := os.ReadDir(sourceDir); err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
				t.Fatalf("automatic conversion wrote beside the original: entries=%v err=%v", entries, err)
			}
			bus := hmc17AttachedToolContext(t, runner)
			result, err := (&tool.TraceQuery{}).Execute(bus, json.RawMessage(`{"source":"attached_trace","view":"event_search","pattern":"tail-target","time_start":1.0105,"time_end":1.012,"limit":10}`))
			if err != nil || !result.Success || !strings.Contains(result.Summary, "tail-target") {
				t.Fatalf("attached_trace lost binary tail beyond the REPL preview: result=%+v err=%v", result, err)
			}
			if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, original) {
				t.Fatalf("automatic binary preparation changed the source: %v", err)
			}
			latest, err := store.LoadLatest()
			if err != nil || !latest.Trace.TraceRequiresReattach || latest.Trace.TraceOriginalPath != path || latest.Trace.SchemaVersion != runtimeArtifactPreparedSchemaVersion {
				t.Fatalf("binary preview gained durable completeness: latest=%+v err=%v", latest, err)
			}
			if body, err := store.Load(latest.Trace, 1024); err == nil || body != "" {
				t.Fatalf("binary preview restored without its original: body=%q err=%v", body, err)
			}
		})
	}
}

func TestHMC17REPLLoopPlainTextSnapshotCompleteness(t *testing.T) {
	for _, truncated := range []bool{false, true} {
		name := "complete"
		if truncated {
			name = "truncated"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "capture.sys")
			body := "worker-12 (12) [000] .... 9.000000: sched_wakeup: comm=text-tail pid=42 prio=120 target_cpu=000\n"
			if truncated {
				body = strings.Repeat("# head filler\n", 100) + body
			}
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			runner, store, _, output := hmc17LoadTraceThroughLoop(t, "/htrace", path, 512)
			material := runner.seenMaterials[0]
			if material == nil {
				t.Fatalf("plain file did not receive a producer-owned completeness receipt:\n%s", output)
			}
			if material.QueryPath() != path || material.SourcePath() != path || material.SelfContainedText() == truncated {
				t.Fatalf("wrong plain-text completeness: material=%+v truncated=%t", material, truncated)
			}
			latest, err := store.LoadLatest()
			if err != nil || latest.Trace.TraceRequiresReattach != truncated {
				t.Fatalf("durable metadata lost precise completeness: %+v err=%v", latest, err)
			}
			if truncated {
				if latest.SchemaVersion != runtimeArtifactPreparedSchemaVersion || latest.Trace.TraceOriginalPath != path {
					t.Fatalf("truncated snapshot missing reattachment provenance: %+v", latest)
				}
				if restored, err := store.Load(latest.Trace, 4096); err == nil || restored != "" {
					t.Fatalf("truncated preview restored as a full capture: %q err=%v", restored, err)
				}
				bus := hmc17AttachedToolContext(t, runner)
				result, err := (&tool.TraceQuery{}).Execute(bus, json.RawMessage(`{"source":"attached_trace","view":"event_search","pattern":"text-tail","time_start":8.9,"time_end":9.1,"limit":10}`))
				if err != nil || !result.Success || !strings.Contains(result.Summary, "text-tail") {
					t.Fatalf("truncated text lost complete physical query material: %+v err=%v", result, err)
				}
				return
			}
			if latest.SchemaVersion != runtimeArtifactStoreSchemaVersion || latest.Trace.SchemaVersion != runtimeArtifactStoreSchemaVersion {
				t.Fatalf("complete text unnecessarily requires the original: %+v", latest)
			}
			if material.Preview() != "# codrax-source: "+path+"\n"+body {
				t.Fatalf("complete text was modified: %q", material.Preview())
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if restored, err := store.Load(latest.Trace, 4096); err != nil || restored != strings.TrimSpace(material.Preview()) {
				t.Fatalf("self-contained snapshot depends on the original: %q err=%v", restored, err)
			}
		})
	}
}

func TestHMC17REPLLoopPreparedExplicitWindowKeepsOnChainIOAndBusiness(t *testing.T) {
	path, err := filepath.Abs(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_business_io_chain", "events.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	runner, _, _, output := hmc17LoadTraceThroughLoop(t, "/atrace", path, 512)
	if runner.seenMaterials[0] == nil {
		t.Fatalf("explicit-window fixture was not prepared through the REPL:\n%s", output)
	}
	if strings.Contains(runner.seenTraces[0], "block_rq_complete") {
		t.Fatal("fixture completion must remain beyond the preview")
	}
	bus := hmc17AttachedToolContext(t, runner)
	for _, view := range []string{"window_stats", "root_cause_rank"} {
		params, err := json.Marshal(map[string]any{"source": "attached_trace", "view": view, "pid": 100, "time_start": 1.0, "time_end": 1.05, "max_depth": 4, "limit": 32})
		if err != nil {
			t.Fatal(err)
		}
		result, err := (&tool.TraceQuery{}).Execute(bus, params)
		if err != nil || !result.Success {
			t.Fatalf("REPL prepared %s failed: %+v err=%v", view, result, err)
		}
		payloadRef := ""
		for _, observation := range result.Observations {
			if observation.SourceRef.PayloadRef != "" {
				payloadRef = observation.SourceRef.PayloadRef
				break
			}
		}
		if payloadRef == "" {
			t.Fatal("public trace query did not publish its typed result")
		}
		data, err := os.ReadFile(payloadRef)
		if err != nil {
			t.Fatal(err)
		}
		var payload tracequery.Result
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.TimeStart != 1.0 || payload.TimeEnd != 1.05 {
			t.Fatalf("prepared transport altered the explicit window: %g..%g", payload.TimeStart, payload.TimeEnd)
		}
		found := false
		if view == "window_stats" {
			if payload.WindowStats == nil {
				t.Fatal("missing explicit-window statistics")
			}
			for _, io := range payload.WindowStats.IOLatencies {
				found = found || io.IssueThread.PID == 200 && io.CompletionWokeIssuer && math.Abs(io.DurationMs-35) < 1e-6 && math.Abs(io.IssuerBlockedMs-31) < 1e-6
			}
			if !found || len(payload.WindowStats.TraceSpans) != 2 {
				t.Fatalf("REPL attachment lost non-additive IO rulers or business spans: %+v", payload.WindowStats)
			}
		} else {
			if payload.RootCauseRank == nil {
				t.Fatal("missing root-cause ranking")
			}
			for _, item := range payload.RootCauseRank.Items {
				if item.Thread.PID == 900 && item.ChainRelevance == "on_chain" {
					t.Fatalf("background IO promoted onto response chain: %+v", item)
				}
				found = found || item.Type == "io_latency" && item.Thread.PID == 200 && item.ChainRelevance == "on_chain" && item.ResourceCompletionClosure && math.Abs(item.EffectiveImpactMs-31) < 1e-6
			}
			if !found {
				t.Fatalf("REPL attachment lost completion-closed IO root: %+v", payload.RootCauseRank.Items)
			}
		}
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("REPL preparation/query modified the established fixture: %v", err)
	}
}

// Same real RMQ envelope as cmd.cliBinaryTraceTailFixture: twelve physical
// pages put the unique final wakeup beyond both raw and converted previews.
func hmc17BinaryTraceTailFixture() []byte {
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
