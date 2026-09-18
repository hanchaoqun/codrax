package tool

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryPreparedMaterialSurvivesProjectionAndIgnoresOldPreviewBlob(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "complete.sys")
	body := strings.Repeat("# preview filler\n", 100) + "waker-10 (10) [001] .... 8.100000: sched_wakeup: comm=tail_business pid=20 prio=53 target_cpu=001\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := traceinput.Prepare(context.Background(), traceinput.Options{InputPath: path, PreviewBytes: 512})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(m.Preview(), "tail_business") {
		t.Fatal("fixture tail is visible in preview")
	}
	blob := filepath.Join(dir, promptctx.AttachedTraceBlobName)
	if err := os.WriteFile(blob, []byte("# old unrelated preview\n"), 0600); err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, AttachedHitrace: m.Preview(), AttachedTraceMaterial: m, Mutable: types.NewMutableState("query trace tail")}
	ac := promptctx.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
	projected := types.ToolBusContext(ac, types.AgentExplorer)
	if projected.AttachedTraceMaterial != m {
		t.Fatal("dispatch lost prepared receipt")
	}
	for _, source := range []string{"attached_trace", "path"} {
		params, _ := json.Marshal(map[string]any{"source": source, "path": path, "view": "event_search", "time_start": 8.0, "time_end": 8.2, "limit": 10})
		res, err := (&TraceQuery{}).Execute(projected, params)
		if err != nil || !res.Success || !strings.Contains(res.Summary, "tail_business") {
			t.Fatalf("full material query failed: %+v err=%v", res, err)
		}
	}
	if got, err := os.ReadFile(blob); err != nil || string(got) != "# old unrelated preview\n" {
		t.Fatalf("query overwrote old blob: %q %v", got, err)
	}
	// Drift must reject before recording call windows, even with a usable old blob.
	if err := os.WriteFile(path, []byte(body+"# changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	projected.Mutable = types.NewMutableState("stale prepared source")
	params := json.RawMessage(`{"source":"attached_trace","view":"event_search","time_start":8,"time_end":9}`)
	res, err := (&TraceQuery{}).Execute(projected, params)
	if err != nil || res.Success || res.Repair == nil || res.Repair.Code != tracequery.TraceInputAdmissionCodeSourceUnavailable || len(projected.Mutable.TraceQueryCallWindows()) != 0 {
		t.Fatalf("stale source fell back: %+v err=%v", res, err)
	}
	// A separately named trace is not blocked by an unrelated stale attachment.
	other := filepath.Join(dir, "other.systrace")
	if err := os.WriteFile(other, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	params, _ = json.Marshal(map[string]any{"source": "path", "path": other, "view": "event_search", "time_start": 8, "time_end": 9})
	res, err = (&TraceQuery{}).Execute(projected, params)
	if err != nil || !res.Success {
		t.Fatalf("unrelated explicit path blocked: %+v %v", res, err)
	}
}

func TestTraceQueryPreparedMaterialCancellationIsNotFileFailure(t *testing.T) {
	for _, warm := range []bool{false, true} {
		for _, deadline := range []bool{false, true} {
			bus, path := pureMemoFixtureCtx(t)
			m, err := traceinput.Prepare(context.Background(), traceinput.Options{InputPath: path, PreviewBytes: 512})
			if err != nil {
				t.Fatal(err)
			}
			bus.AttachedHitrace, bus.AttachedTraceMaterial = m.Preview(), m
			params := pureMemoFixtureParams(t, map[string]any{"source": "attached_trace", "path": ""})
			if warm {
				res, err := (&TraceQuery{}).Execute(bus, params)
				if err != nil || !res.Success {
					t.Fatalf("warmup: %+v %v", res, err)
				}
			}
			cancelCtx, cancel := context.WithCancel(context.Background())
			cancel()
			want := "canceled"
			if deadline {
				cancelCtx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				cancel()
				want = "deadline_exceeded"
			}
			bus.Ctx = cancelCtx
			res, err := (&TraceQuery{}).Execute(bus, params)
			if err != nil || res.Success || res.Repair != nil || res.TraceViewCancellation == nil || res.TraceViewCancellation.Reason != want || res.ReusedFromRunMemo {
				t.Fatalf("warm=%v deadline=%v cancellation misclassified: %+v %v", warm, deadline, res, err)
			}
		}
	}
}

func TestTraceQueryPreparedMaterialCanonicalAliasIsSameSource(t *testing.T) {
	dir := t.TempDir()
	physical := filepath.Join(dir, "physical.systrace")
	alias := filepath.Join(dir, "attached.sys")
	if err := os.WriteFile(physical, []byte(pureMemoFixtureTrace), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(physical, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	m, err := traceinput.Prepare(context.Background(), traceinput.Options{InputPath: alias, PreviewBytes: 512})
	if err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, AttachedHitrace: m.Preview(), AttachedTraceMaterial: m}
	p := traceQueryParams{Source: "path", Path: physical, View: "event_search"}
	path, source, reject := resolveTraceQuerySource(bus, p)
	if reject != nil || source != "attached_trace" || path != m.QueryPath() {
		t.Fatalf("canonical alias split capture: %s %s %+v", path, source, reject)
	}
}

// The new transport must retain existing explicit-window causal analysis,
// not merely make preview-tail event search possible.
func TestTraceQueryPreparedMaterialKeepsOnChainIOAndBusinessWindow(t *testing.T) {
	path, err := filepath.Abs(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_business_io_chain", "events.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := traceinput.Prepare(context.Background(), traceinput.Options{InputPath: path, PreviewBytes: 512})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(m.Preview(), "block_rq_complete") {
		t.Fatal("fixture completion must remain outside the preview")
	}
	bus := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), AttachedHitrace: m.Preview(), AttachedTraceMaterial: m, Mutable: types.NewMutableState("explain business IO")}
	bus = types.ToolBusContext(promptctx.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore), types.AgentExplorer)
	for _, view := range []string{"window_stats", "root_cause_rank"} {
		params, _ := json.Marshal(map[string]any{"source": "attached_trace", "view": view, "pid": 100, "time_start": 1.0, "time_end": 1.05, "max_depth": 4, "limit": 32})
		result, err := (&TraceQuery{}).Execute(bus, params)
		if err != nil || !result.Success {
			t.Fatalf("prepared %s failed: %v %s", view, err, result.Summary)
		}
		payloadRef := ""
		for _, observation := range result.Observations {
			if observation.SourceRef.PayloadRef != "" {
				payloadRef = observation.SourceRef.PayloadRef
				break
			}
		}
		if payloadRef == "" {
			t.Fatal("query did not publish a typed JSON payload reference")
		}
		data, err := os.ReadFile(payloadRef)
		if err != nil {
			t.Fatal(err)
		}
		var payload tracequery.Result
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatal(err)
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
				t.Fatalf("prepared transport lost IO rulers or business spans: %+v", payload.WindowStats)
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
				t.Fatalf("prepared transport lost completion-closed S-state IO root: %+v", payload.RootCauseRank.Items)
			}
		}
	}
}
