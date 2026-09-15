package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1697ActualTraceQuery(t *testing.T, name string) (*types.AgentContext, *BaseAgent, types.ToolResult, string) {
	t.Helper()
	root := t.TempDir()
	capture := filepath.Join(root, "captures", name)
	if err := os.MkdirAll(filepath.Dir(capture), 0755); err != nil {
		t.Fatal(err)
	}
	body := "# tracer: nop\n" +
		" app-17267 (17267) [000] .... 5.000000: sched_switch: prev_comm=app prev_pid=17267 prev_prio=120 prev_state=R ==> next_comm=idle next_pid=0 next_prio=120\n" +
		" limiter-4776 (4776) [000] .... 5.001000: cpu_frequency_limits: min=300000 max=1000000 cpu_id=0\n" +
		" idle-0 (0) [000] .... 5.002000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=17267 next_prio=120\n" +
		" app-17267 (17267) [000] .... 5.003000: tracing_mark_write: B|17267|work\n" +
		" app-17267 (17267) [000] .... 5.004000: tracing_mark_write: E|17267\n"
	if err := os.WriteFile(capture, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	ctx := blobEscapeObservationOnlyContext(t, types.NewMutableState("bounded raw trace read"))
	ctx.RepoRoot, ctx.WorkDir, ctx.AttachedHitrace = root, filepath.Join(root, ".codrax", "blob", "b1697"), capture
	params, _ := json.Marshal(map[string]any{"source": "path", "path": capture, "view": "event_search", "pid": 17267, "time_start": 5.0, "time_end": 5.01, "line_start": 2, "line_end": 6, "limit": 10})
	query, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !query.Success || len(query.Observations) == 0 {
		t.Fatalf("actual trace query failed: %v %+v", err, query)
	}
	ctx.Mutable.AppendDispatchToolResult(query)
	if ctx.Mutable.TraceQueryRuntimeObservationCount() == 0 {
		t.Fatal("actual query must arm the hard runtime evidence boundary")
	}
	reg := tool.NewRegistry()
	reg.Register(&tool.ReadFile{})
	reg.Register(&tool.TraceQuery{})
	return ctx, NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: reg}, nil), query, capture
}

func b1697ReadCall(path string, offset, limit int) llm.ToolCall {
	params, _ := json.Marshal(map[string]any{"path": path, "line_offset": offset, "limit": limit})
	return llm.ToolCall{Name: "read_file", Params: params}
}

func TestB1697ActualTraceQueryPermitsBoundedOriginalRead(t *testing.T) {
	for _, name := range []string{"attached_trace.txt", "capture.go"} {
		t.Run(name, func(t *testing.T) {
			ctx, base, query, capture := b1697ActualTraceQuery(t, name)
			for _, offset := range []int{2, 0} {
				read, _ := base.executeTool(ctx, b1697ReadCall(capture, offset, 2))
				if read == nil || !read.Success {
					t.Fatalf("bounded follow-up on actually queried original capture rejected: %+v", read)
				}
				if read.RuntimeArtifactRead == nil || read.RuntimeArtifactRead.TraceQueryBlob || read.RuntimeArtifactRead.Kind != "trace" || read.ReadCoverage != nil {
					t.Fatalf("original read must stay runtime-only, not query-result/source authority: %+v", read)
				}
				if span := read.RuntimeArtifactRead; span.LineStart != offset+1 || span.LineEnd != offset+2 {
					t.Fatalf("explicit bounded read expanded beyond its requested lines: %+v", span)
				}
				if offset == 2 && !strings.Contains(read.Summary, "cpu_frequency_limits") {
					t.Fatalf("raw witness must remain readable without deleting inherited target PID: %s", read.Summary)
				}
				_, readSet, ranges := extractFileCoverage([]types.ToolResult{query, *read}, ctx.RepoRoot)
				if len(readSet) != 0 || len(ranges) != 0 {
					t.Fatalf("original trace read minted source coverage: %+v %+v", readSet, ranges)
				}
			}
		})
	}
}

func TestB1697OriginalReadDoesNotBroadenToolOrPathPermission(t *testing.T) {
	ctx, base, query, capture := b1697ActualTraceQuery(t, "capture.go")
	other := filepath.Join(t.TempDir(), filepath.Base(capture))
	if err := os.WriteFile(other, []byte("package unqueried\n"), 0644); err != nil {
		t.Fatal(err)
	}
	calls := []llm.ToolCall{
		b1697ReadCall(other, 1, 2),
		b1697ReadCall(filepath.Base(capture), 1, 2),
		b1697ReadCall(capture, 0, 0),
		b1697ReadCall(capture, 1, 201),
		b1697ReadCall(capture, int(^uint(0)>>1), 2),
		{Name: "read_file", Params: json.RawMessage(`{"path":` + strconvQuoteB1697(capture) + `}`)},
		{Name: "grep", Params: json.RawMessage(`{"path":` + strconvQuoteB1697(capture) + `,"pattern":"cpu_frequency_limits"}`)},
		{Name: "exec_command", Params: json.RawMessage(`{"cmd":"true"}`)},
	}
	for _, call := range calls {
		read, _ := base.executeTool(ctx, call)
		if read == nil || read.Success {
			t.Fatalf("unregistered/unbounded/general tool escaped: %+v => %+v", call, read)
		}
	}
	read, _ := base.executeTool(ctx, b1697ReadCall(capture, 1, 2))
	if read == nil || !read.Success || read.RawRef == "" {
		t.Fatalf("bounded original read required: %+v", read)
	}
	ctx.Mutable.AppendDispatchToolResult(*read)
	if derived, _ := base.executeTool(ctx, b1697ReadCall(read.RawRef, 0, 2)); derived == nil || derived.Success {
		t.Fatalf("derived read result acquired original-source permission: %+v", derived)
	}
	if query.RawRef == "" {
		t.Fatal("actual query result must publish its normal Q5-A blob")
	}
	if blob, _ := base.executeTool(ctx, b1697ReadCall(query.RawRef, 1, 2)); blob == nil || !blob.Success || blob.RuntimeArtifactRead == nil || !blob.RuntimeArtifactRead.TraceQueryBlob {
		t.Fatalf("original Q5-A query-result lane changed: %+v", blob)
	}
}

func strconvQuoteB1697(s string) string { b, _ := json.Marshal(s); return string(b) }

func TestB1697OriginalReadLifetimeAndCanonicalPath(t *testing.T) {
	ctx, base, query, capture := b1697ActualTraceQuery(t, "capture.go")
	alias := filepath.Join(ctx.RepoRoot, "capture-alias.go")
	if err := os.Symlink(capture, alias); err != nil {
		t.Fatal(err)
	}
	ctx.Mutable.ResetDispatchToolResults()
	for _, path := range []string{capture, filepath.Join(filepath.Dir(capture), ".", filepath.Base(capture)), "captures/capture.go", alias} {
		if read, _ := base.executeTool(ctx, b1697ReadCall(path, 1, 2)); read == nil || !read.Success || read.RuntimeArtifactRead == nil || read.ReadCoverage != nil {
			t.Fatalf("dispatch reset/canonical exact file read failed for %q: %+v", path, read)
		}
	}
	fork := ctx.Mutable.ForkForExploreDispatch()
	parent := ctx.Mutable
	ctx.Mutable = fork
	if read, _ := base.executeTool(ctx, b1697ReadCall(capture, 1, 2)); read == nil || !read.Success {
		t.Fatalf("fork lost original permission: %+v", read)
	}
	ctx.Mutable = parent
	parent.ResetTurnAArtifacts()
	parent.MergeExploreFork(fork)
	parent.AppendDispatchToolResult(query) // same-process stale result replay
	if read, _ := base.executeTool(ctx, b1697ReadCall(capture, 1, 2)); read == nil || read.Success {
		t.Fatalf("late fork or prior-turn result resurrected raw permission: %+v", read)
	}
}

func TestB1697OriginalReadHonorsTerminalAdmission(t *testing.T) {
	ctx, base, _, capture := b1697ActualTraceQuery(t, "capture.go")
	armTraceQueryTerminalAdmissionForTest(t, ctx.Mutable, "trace_input_empty", false)
	ctx.Mutable.ResetDispatchToolResults()
	if read, _ := base.executeTool(ctx, b1697ReadCall(capture, 1, 2)); read == nil || read.Success || read.Repair == nil || read.Repair.Metadata["policy"] != "trace_input_admission_terminal" {
		t.Fatalf("registered raw capture escaped terminal admission: %+v", read)
	}
}

// Run a real native reader after the shared agent gate, with a deterministic
// mutation exactly at the handoff. This pins the otherwise tiny TOCTOU window.
type b1697HandoffRead struct {
	tool.ReadFile
	before func()
}

func (r *b1697HandoffRead) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	r.before()
	return r.ReadFile.Execute(ctx, params)
}

func TestB1697GateToReaderCannotLoseOriginalRole(t *testing.T) {
	for _, change := range []string{"replace", "reset", "symlink_retarget", "restored_mtime_rewrite"} {
		t.Run(change, func(t *testing.T) {
			ctx, _, _, capture := b1697ActualTraceQuery(t, "capture.go")
			requested := capture
			if change == "symlink_retarget" {
				requested = filepath.Join(ctx.RepoRoot, "alias.go")
				if err := os.Symlink(capture, requested); err != nil {
					t.Fatal(err)
				}
			}
			called := false
			reg := tool.NewRegistry()
			reg.Register(&tool.TraceQuery{})
			reg.Register(&b1697HandoffRead{before: func() {
				called = true
				if change == "reset" {
					ctx.Mutable.ResetTurnAArtifacts()
					return
				}
				if change == "restored_mtime_rewrite" {
					info, err := os.Stat(capture)
					if err != nil {
						t.Fatal(err)
					}
					data, err := os.ReadFile(capture)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(capture, []byte(strings.ReplaceAll(string(data), "120", "121")), info.Mode()); err != nil {
						t.Fatal(err)
					}
					if err := os.Chtimes(capture, info.ModTime(), info.ModTime()); err != nil {
						t.Fatal(err)
					}
					return
				}
				replacement := filepath.Join(ctx.RepoRoot, "unqueried.go")
				if err := os.WriteFile(replacement, []byte("package unqueried\nfunc Secret() {}\n"), 0644); err != nil {
					t.Fatal(err)
				}
				if change == "replace" {
					if err := os.Rename(replacement, capture); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := os.Remove(requested); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(replacement, requested); err != nil {
						t.Fatal(err)
					}
				}
			}})
			base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: reg}, nil)
			read, _ := base.executeTool(ctx, b1697ReadCall(requested, 1, 2))
			if !called || read == nil || read.Success || read.ReadCoverage != nil || len(read.Observations) != 0 || strings.Contains(read.Summary, "func Secret") {
				t.Fatalf("gate-to-reader %s substituted source bytes/authority: called=%v result=%+v", change, called, read)
			}
		})
	}
}
