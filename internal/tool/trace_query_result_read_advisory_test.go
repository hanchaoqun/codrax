package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/width"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Use the real producer and dispatch publication chokepoint: neither a file
// name nor a string in its body grants the result-reader role.
func b1624PublishedResult(t *testing.T) (*types.BusContext, types.ToolResult, []string) {
	t.Helper()
	previous := MaxInlineBytes
	MaxInlineBytes = 2048
	t.Cleanup(func() { MaxInlineBytes = previous })
	dir := t.TempDir()
	blobDir := filepath.Join(dir, ".codrax", "blob", "current-session")
	if err := os.MkdirAll(blobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(blobDir, "attached_trace.txt")
	var body strings.Builder
	body.WriteString("# tracer: nop\n")
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&body, " app-100 (100) [000] .... %.6f: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n", 5+float64(i)/1000)
		fmt.Fprintf(&body, " app-100 (100) [000] .... %.6f: tracing_mark_write: B|100|quoted-result-span\n", 5+float64(i)/1000+.0001)
	}
	if err := os.WriteFile(capture, []byte(body.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: blobDir, AttachedHitrace: capture, Mutable: types.NewMutableState("result-reader")}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": capture, "view": "event_search", "time_start": 5.0, "time_end": 5.1, "limit": 256})
	result, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !result.Success {
		t.Fatalf("real query: %v %+v", err, result)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	refs := []string{result.RawRef}
	for _, observation := range result.Observations {
		if p := observation.SourceRef.PayloadRef; p != "" {
			refs = append(refs, p)
			break
		}
	}
	if len(refs) != 2 || refs[0] == "" || filepath.Ext(refs[1]) != ".json" {
		t.Fatalf("real producer did not publish text and JSON: %v", refs)
	}
	for _, ref := range refs {
		if actual, ok := ctx.Mutable.ResolveTraceQueryBlobRef(ref); !ok || actual != ref {
			t.Fatalf("dispatch failed to register %s", ref)
		}
	}
	return ctx, result, refs
}

func b1624AssertResultNavigation(t *testing.T, result types.ToolResult) {
	t.Helper()
	if !result.Success {
		t.Fatalf("reader failed: %s", result.Summary)
	}
	for _, forbidden := range []string{"trace_query_required_soft_advisory=", "Next call: trace_query(", "Use trace_query(", "prefer trace_query with", "Switch to trace_query(", "use trace_query with", "repo_map("} {
		if strings.Contains(result.Summary, forbidden) {
			t.Errorf("result bytes were mistaken for a capture/source: %q\n%s", forbidden, result.Summary)
		}
	}
	if hint := result.Refinement; hint != nil && hint.PreferredNextTool != "grep" && hint.PreferredNextTool != "read_file" {
		t.Errorf("result refinement must stay on result reading: %+v", hint)
	}
}

func TestB1624RealPublishedResultGrepAndReadFile(t *testing.T) {
	ctx, original, refs := b1624PublishedResult(t)
	before, _ := json.Marshal(original)
	for _, ref := range refs {
		content, err := os.ReadFile(ref)
		if err != nil {
			t.Fatal(err)
		}
		for _, requested := range []string{ref, filepath.Base(ref)} {
			t.Run(filepath.Ext(ref)+"/"+requested, func(t *testing.T) {
				for _, shape := range []struct {
					name, pattern     string
					window, filesOnly bool
				}{
					{"success", "B|100|quoted-result-span", true, false},
					{"zero", `E|100|quoted-result-span`, true, false},
					{"native-broad", "", true, false},
					{"streamed-broad", "", false, false},
					{"files-only", "sched_switch", false, true},
				} {
					t.Run(shape.name, func(t *testing.T) {
						p := map[string]any{"path": requested, "pattern": shape.pattern, "fixed_string": true, "context_lines": 3, "files_only": shape.filesOnly}
						if shape.pattern == "" {
							p["pattern"] = "."
							p["fixed_string"] = false
						}
						if shape.window {
							p["line_start"] = 1
							p["line_end"] = 10000
						}
						if shape.name == "success" {
							for i, line := range strings.Split(string(content), "\n") {
								if strings.Contains(line, shape.pattern) {
									p["line_start"] = i + 1
									p["line_end"] = i + 1
									break
								}
							}
						}
						params, _ := json.Marshal(p)
						result, err := (&GrepTool{}).Execute(ctx, params)
						if err != nil {
							t.Fatal(err)
						}
						b1624AssertResultNavigation(t, result)
						if shape.name == "success" && (!strings.Contains(result.Summary, shape.pattern) || strings.Contains(result.Summary, "decision=broad_result_compacted")) {
							t.Errorf("small successful read was not exercised: %+v", result)
						}
						if strings.Contains(shape.name, "broad") && (result.RawRef == "" || !strings.Contains(result.Summary, "decision=broad_result_compacted")) {
							t.Errorf("broad-result budget/refinement lost: %+v", result)
						}
						// The existing streamed branch may supply only its bounded
						// preview to the refinement threshold. Do not turn this role
						// repair into a change to that independent legacy behavior.
						if strings.Contains(shape.name, "broad") && result.Refinement != nil && !result.Refinement.ResultTruncated {
							t.Errorf("existing broad refinement lost truncation: %+v", result.Refinement)
						}
						if shape.name == "zero" && (result.Refinement == nil || result.Refinement.PreferredNextTool != "grep") {
							t.Errorf("zero-match did not retain result grep: %+v", result.Refinement)
						}
					})
				}
				for _, offset := range []int{0, 1} {
					params, _ := json.Marshal(map[string]any{"path": requested, "line_offset": offset})
					result, err := (&ReadFile{}).Execute(ctx, params)
					if err != nil {
						t.Fatal(err)
					}
					b1624AssertResultNavigation(t, result)
					want := "grep"
					if offset > 0 {
						want = "read_file"
					}
					if result.Refinement == nil || result.Refinement.PreferredNextTool != want || !result.Refinement.ResultTruncated {
						t.Errorf("paging budget/refinement lost: %+v", result.Refinement)
					}
					if result.Refinement != nil {
						resolved, reject := resolveReadFilePath(ctx, result.Refinement.PreferredParams["path"])
						if reject != nil || resolved != ref {
							t.Errorf("next read lost result identity: %s %+v", resolved, reject)
						}
						if offset > 0 && (result.Refinement.NextCursor == "" || result.Refinement.NextCursor != result.Refinement.PreferredParams["line_offset"]) {
							t.Errorf("next page lost cursor: %+v", result.Refinement)
						}
					}
					if result.RuntimeArtifactRead == nil || !result.RuntimeArtifactRead.TraceQueryBlob || result.ReadCoverage != nil || len(result.Observations) != 0 {
						t.Errorf("result acquired current-source authority: %+v", result)
					}
				}
			})
		}
		after, _ := os.ReadFile(ref)
		if !reflect.DeepEqual(content, after) {
			t.Fatal("reader changed original result bytes")
		}
	}
	after, _ := json.Marshal(original)
	if string(before) != string(after) {
		t.Fatal("reader changed published source fields")
	}
}

func TestB1624ResultWholeReadWallKeepsRoleAndRefusal(t *testing.T) {
	// Sparse fixture exercises the real pre-allocation refusal without changing
	// the global byte wall or inflating/mutating a real trace-query result.
	dir := t.TempDir()
	blobDir := filepath.Join(dir, ".codrax", "blob", "session")
	if err := os.MkdirAll(blobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sparseFile(t, blobDir, "opaque.data", width.Current().ReadFile.MaxWholeReadBytes+1)
	ref := filepath.Join(blobDir, "opaque.data")
	ctx := &types.BusContext{RepoRoot: dir, Mutable: types.NewMutableState("large result")}
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{ToolName: "trace_query", Success: true, RawRef: ref})
	params, _ := json.Marshal(map[string]any{"path": ref})
	result, err := (&ReadFile{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success || result.Repair == nil || result.Repair.Code != "read_file_too_large" {
		t.Fatalf("whole-read wall changed: %+v", result)
	}
	if strings.Contains(result.Summary+result.Repair.Hint, "trace_query") || !strings.Contains(result.Summary+result.Repair.Hint, "grep") {
		t.Errorf("oversized result treated as a capture: %+v", result)
	}
}

func TestB1624PublishedResultRoleLifetimeAndRawCapture(t *testing.T) {
	base, published, refs := b1624PublishedResult(t)
	for _, ref := range refs {
		for _, tc := range []struct {
			name  string
			setup func(*types.MutableState)
		}{
			{"new-session", func(m *types.MutableState) {}},
			{"failed-query", func(m *types.MutableState) { r := published; r.Success = false; m.AppendDispatchToolResult(r) }},
			{"other-tool", func(m *types.MutableState) { r := published; r.ToolName = "read_file"; m.AppendDispatchToolResult(r) }},
			{"summary-only", func(m *types.MutableState) {
				m.AppendDispatchToolResult(types.ToolResult{ToolName: "trace_query", Success: true, Summary: "payload_ref=" + ref})
			}},
			{"reset-cycle", func(m *types.MutableState) { m.AppendDispatchToolResult(published); m.ResetTurnAArtifacts() }},
		} {
			t.Run(filepath.Ext(ref)+"/"+tc.name, func(t *testing.T) {
				ctx := base.ShallowClone()
				ctx.Mutable = types.NewMutableState(tc.name)
				tc.setup(ctx.Mutable)
				if _, ok := resolveTraceQueryBlobRefPath(ctx, ref); ok {
					t.Fatal("fixture unexpectedly registered stale/failed ref")
				}
				if traceQueryResultReadTarget(ctx, ref, ref) || traceQueryResultReadAdvisory(ctx, ref) != "" {
					t.Fatal("unpublished ref acquired result role")
				}
				params, _ := json.Marshal(map[string]any{"path": ref, "pattern": "E|100|quoted-result-span", "fixed_string": true, "line_start": 1, "line_end": 10000})
				result, err := (&GrepTool{}).Execute(ctx, params)
				if err != nil {
					t.Fatal(err)
				}
				if !result.Success || strings.Contains(result.Summary, "query_result_read_advisory=") {
					t.Fatalf("unpublished file took result escape: %+v", result)
				}
			})
		}
	}
	// The actual input is in the same session blob directory, but is not a
	// published query output. Preserve its raw-capture navigation and budgets.
	params, _ := json.Marshal(map[string]any{"path": base.AttachedHitrace, "pattern": "E|100|quoted-result-span", "fixed_string": true, "context_lines": 3})
	result, err := (&GrepTool{}).Execute(base, params)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success || !strings.Contains(result.Summary, "trace_query_required_soft_advisory=") || !strings.Contains(result.Summary, "trace_marker_span_end_shape=") || result.Refinement == nil || result.Refinement.PreferredNextTool != "trace_query" {
		t.Fatalf("real capture lost raw-trace navigation: %+v", result)
	}
	params, _ = json.Marshal(map[string]any{"path": base.AttachedHitrace})
	read, err := (&ReadFile{}).Execute(base, params)
	if err != nil {
		t.Fatal(err)
	}
	if !read.Success || read.Refinement == nil || read.Refinement.PreferredNextTool != "trace_query" {
		t.Fatalf("capture truncation no longer points back to query: %+v", read)
	}
	// A name resemblance grants nothing. Conversely, the old basename alias
	// route really reads the registered result, not the separately named file.
	unpublished := filepath.Join(base.WorkDir, "trace_query-not-published.txt")
	if err := os.WriteFile(unpublished, []byte("sched_switch: raw unregistered input\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if traceQueryResultReadTarget(base, unpublished, unpublished) {
		t.Fatal("prefix gave an unpublished file a result role")
	}
	alias := filepath.Join(t.TempDir(), filepath.Base(refs[0]))
	if err := os.WriteFile(alias, []byte("different raw input\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, reject := resolveReadFilePath(base, alias)
	if reject != nil || resolved != refs[0] {
		t.Fatalf("existing alias route changed: %s %+v", resolved, reject)
	}
	if !traceQueryResultReadTarget(base, alias, resolved) || traceQueryResultReadTarget(base, alias, alias) {
		t.Fatal("role did not bind the actual resolved target")
	}
}

func TestB1624ResultAdviceBranchesUsePublishedRole(t *testing.T) {
	ctx, _, refs := b1624PublishedResult(t)
	for _, ref := range refs {
		contextLines := 3
		p := grepToolParams{Path: ref, Pattern: `E|100|quoted-result-span`, FixedString: true, ContextLines: &contextLines}
		if advice := grepRuntimeArtifactParamAdvisory(ctx, p); strings.Contains(advice, "trace_query") || !strings.Contains(advice, "read_file") {
			t.Errorf("param advice: %s", advice)
		}
		quoted := ref + ":5: app-100 (100) [000] .... 5.0: tracing_mark_write: B|100|quoted-result-span\n"
		if advice := grepRuntimeArtifactTraceSpanAdvisory(ctx, p, quoted); advice != "" {
			t.Errorf("result quotation became raw-span navigation: %s", advice)
		}
		if got := grepNoMatchBody(ctx, grepToolParams{Path: ref, Pattern: `foo\.bar`, FixedString: true}); !strings.Contains(got, "fixed_string") || !strings.Contains(got, "query_result_read_advisory=") {
			t.Errorf("fixed-string teaching/result role lost: %s", got)
		}
		// files_only cannot naturally exceed a multi-file entry threshold for
		// one file. Exercise its actual shared compaction/refinement entry with
		// an oversized backend path list, retaining the ordinary threshold.
		p.FilesOnly = true
		raw := strings.Repeat(ref+"\n", grepWidthFileEntryThreshold()+1)
		hint := grepBroadResultRefinement(ctx, p, raw)
		if hint == nil || hint.PreferredNextTool != "read_file" || hint.PreferredParams["path"] != ref {
			t.Errorf("files_only result promoted to source navigation: %+v", hint)
		}
	}
}
