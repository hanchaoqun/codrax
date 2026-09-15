package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1697ToolNativeSource(t *testing.T) (*types.BusContext, string, json.RawMessage, types.ToolResult) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "capture.go")
	data := "# tracer: nop\n" +
		" app-100 (100) [000] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n" +
		" idle-0 (0) [000] .... 5.003000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120\n"
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: root, WorkDir: filepath.Join(root, ".codrax", "blob", "source"), Mutable: types.NewMutableState("native raw read")}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "time_start": 5.0, "time_end": 5.01, "limit": 10})
	result, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !result.Success || result.TraceQuerySourceRead.Path() == "" {
		t.Fatalf("native query must stamp original source identity: %v %+v", err, result)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	return ctx, path, params, result
}

func TestB1697SinglePhysicalCoordinatesRejectVirtualManifest(t *testing.T) {
	for _, tc := range []struct {
		name      string
		artifacts []tracequery.TraceArtifactSource
		want      bool
	}{
		{"direct", []tracequery.TraceArtifactSource{{SourcePath: "/capture/raw", VirtualLineBase: 0}}, true},
		{"canonical", []tracequery.TraceArtifactSource{{SourcePath: "/capture/./raw", VirtualLineBase: 0}}, true},
		{"no_artifacts", nil, false},
		{"virtual", []tracequery.TraceArtifactSource{{SourcePath: "/capture/raw", VirtualLineBase: 100}}, false},
		{"manifest", []tracequery.TraceArtifactSource{{SourcePath: "/capture/member.systrace", VirtualLineBase: 0}}, false},
		{"composite", []tracequery.TraceArtifactSource{{SourcePath: "/capture/raw"}, {SourcePath: "/capture/other"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ref := traceQuerySourceReadCandidate(tracequery.Result{SourcePath: "/capture/raw", TraceArtifacts: tc.artifacts})
			if got := ref != (types.TraceQuerySourceReadRef{}); got != tc.want {
				t.Fatalf("physical coordinate candidate=%v want=%v", got, tc.want)
			}
			if ref.Path() != "" {
				t.Fatal("an unstamped physical candidate must not act as read permission")
			}
		})
	}
}

func TestB1697NativeReaderBoundsAndQueryBlobCollision(t *testing.T) {
	ctx, path, _, _ := b1697ToolNativeSource(t)
	// Bounds constrain the new agent escape, not the old directly-invoked
	// reader's whole-file/paging contract. Both roles remain runtime-only.
	for _, limit := range []int{0, 1} {
		params, _ := json.Marshal(map[string]any{"path": path, "limit": limit})
		result, _ := (&ReadFile{}).Execute(ctx, params)
		if !result.Success || result.ReadCoverage != nil || result.RuntimeArtifactRead == nil || result.RuntimeArtifactRead.Kind != "trace" || result.RuntimeArtifactRead.LineEnd != result.RuntimeArtifactRead.TotalLines {
			t.Fatalf("direct native reader changed its original full/small-file read contract: %+v", result)
		}
	}
	bound := ctx.ShallowClone()
	ref, ok := ctx.Mutable.ResolveTraceQuerySourceRead(path)
	if !ok {
		t.Fatal("native query publication must provide the gate's receipt")
	}
	BindTraceQuerySourceRead(bound, ref)
	for _, params := range []json.RawMessage{
		json.RawMessage(`{"path":` + quoteB1697(path) + `}`),
		json.RawMessage(`{"path":` + quoteB1697(path) + `,"limit":0}`),
		json.RawMessage(`{"path":` + quoteB1697(path) + `,"limit":201}`),
	} {
		if TraceQuerySourceReadAllowed(ctx, params) {
			t.Fatal("unbounded original capture acquired agent escape")
		}
		if result, _ := (&ReadFile{}).Execute(bound, params); result.Success || result.ReadCoverage != nil {
			t.Fatalf("held agent receipt accepted unbounded capture read: %+v", result)
		}
	}
	// Existing Q5-A basename compatibility chooses the published result even
	// when a queried original exists at a same-basename repository path.
	blob := filepath.Join(ctx.WorkDir, filepath.Base(path))
	if err := os.WriteFile(blob, []byte("published query result\nrow 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx.Mutable.RegisterTraceQueryBlobRef(blob)
	params := json.RawMessage(`{"path":"capture.go","line_offset":1,"limit":1}`)
	if TraceQuerySourceReadAllowed(ctx, params) {
		t.Fatal("original capture permission overrode the Q5-A resolved result")
	}
	result, _ := (&ReadFile{}).Execute(ctx, params)
	if !result.Success || result.RuntimeArtifactRead == nil || !result.RuntimeArtifactRead.TraceQueryBlob || result.ReadCoverage != nil || !strings.Contains(result.Summary, "row 2") {
		t.Fatalf("Q5-A blob role/path changed under source basename collision: %+v", result)
	}
}

func quoteB1697(s string) string { b, _ := json.Marshal(s); return string(b) }

func TestB1697MemoHitCannotGrantReplacedCapture(t *testing.T) {
	ctx, path, _, _ := b1697ToolNativeSource(t)
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 100, "time_start": 5.0, "time_end": 5.01})
	first, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !first.Success || first.ReusedFromRunMemo || first.TraceQuerySourceRead.Path() == "" {
		t.Fatalf("actual non-streamed query required to populate existing memo: %v %+v", err, first)
	}
	ctx.Mutable.AppendDispatchToolResult(first)
	cached, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !cached.Success || !cached.ReusedFromRunMemo {
		t.Fatalf("unchanged query must exercise normal memo reuse: %v %+v", err, cached)
	}
	ctx.Mutable.AppendDispatchToolResult(cached)
	if _, ok := ctx.Mutable.ResolveTraceQuerySourceRead(path); !ok {
		t.Fatal("memo reuse revoked already published original permission")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(ctx.RepoRoot, "replacement")
	if err := os.WriteFile(replacement, []byte(strings.ReplaceAll(string(data), "120", "121")), info.Mode()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(replacement, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	result, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !result.Success || !result.ReusedFromRunMemo {
		t.Fatalf("existing memo must be exercised without changing its key: %v %+v", err, result)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	if result.TraceQuerySourceRead.Path() != "" {
		t.Fatal("memo result stamped a new capture identity without reading it")
	}
	if _, allowed := ctx.Mutable.ResolveTraceQuerySourceRead(path); allowed {
		t.Fatal("memo reauthorized a replaced, unqueried physical capture")
	}
}
