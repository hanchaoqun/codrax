package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

const b1624bAgentDerived = "/anchor/.codrax/blob/20260704-120000-000-1/grep-derived.txt"

func b1624bAgentPublishDerived(t *testing.T, m *types.MutableState, input, output string) {
	t.Helper()
	ticket, ok := m.PrepareArtifactReadNavigation(input)
	if !ok {
		t.Fatalf("missing current original-result ticket: %s", input)
	}
	ticket.OutputRef = output
	m.AppendDispatchToolResult(types.ToolResult{ToolName: "grep", Success: true, RawRef: output, ArtifactReadNavigation: ticket})
}

func b1624bAgentCall(name, path string) llm.ToolCall {
	params, _ := json.Marshal(map[string]any{"path": path, "pattern": "state_total", "line_offset": 2, "limit": 3})
	return llm.ToolCall{Name: name, Params: params}
}

func TestB1624bAgentBothRejectionsAppendOnlyReturnNavigation(t *testing.T) {
	m := types.NewMutableState("navigation gate")
	m.AppendDispatchToolResult(blobEscapeTraceQueryResult())
	b1624bAgentPublishDerived(t, m, blobEscapeRawRef, b1624bAgentDerived)
	ctx := blobEscapeObservationOnlyContext(t, m)
	control := *ctx
	control.Mutable = types.NewMutableState("navigation control")
	control.Mutable.AppendDispatchToolResult(blobEscapeTraceQueryResult())
	ticket, ok := m.PrepareArtifactReadNavigation(b1624bAgentDerived)
	if !ok {
		t.Fatal("precondition: actual dispatch must preserve the derived navigation ticket")
	}
	advice := types.ArtifactReadNavigationAdvisory(ticket)
	if advice == "" {
		t.Fatal("precondition: valid shared navigation advice is empty")
	}
	for _, gate := range []struct {
		name string
		call func(*types.AgentContext, llm.ToolCall, bool) *types.ToolResult
	}{
		{"exact_artifact", validateExplorerTraceOnlyExactArtifactToolCall},
		{"runtime_evidence", validateExplorerTraceQueryRuntimeEvidenceBoundary},
	} {
		for _, name := range []string{"read_file", "grep"} {
			t.Run(gate.name+"/"+name, func(t *testing.T) {
				call := b1624bAgentCall(name, b1624bAgentDerived)
				before, after := gate.call(&control, call, true), gate.call(ctx, call, true)
				if before == nil || after == nil || after.Success || after.Repair == nil {
					t.Fatalf("navigation must retain the original rejection: before=%+v after=%+v", before, after)
				}
				if after.Summary != before.Summary+"\n"+advice || after.Repair.Hint != before.Repair.Hint+"\n"+advice {
					t.Fatalf("rejection must append the shared original-result advice only: %+v", after)
				}
				copyResult, copyRepair := *after, *after.Repair
				copyResult.Summary, copyResult.Timestamp = before.Summary, before.Timestamp
				copyRepair.Hint = before.Repair.Hint
				copyResult.Repair = &copyRepair
				if !reflect.DeepEqual(&copyResult, before) {
					t.Fatal("navigation changed rejection metadata, refinement, or other authority")
				}
				if got := gate.call(ctx, b1624bAgentCall(name, blobEscapeRawRef), true); got != nil {
					t.Fatalf("the original published-result escape must remain available: %+v", got)
				}
				if _, allowed := m.ResolveTraceQueryBlobRef(b1624bAgentDerived); allowed {
					t.Fatal("derived navigation granted a read permission")
				}
			})
		}
	}
}

func TestB1624bAgentUnknownAndStaleNavigationKeepOriginalRejections(t *testing.T) {
	for _, scenario := range []string{"unpublished", "basename", "different_directory", "source", "forged_summary", "reset", "conflict", "nonreader"} {
		t.Run(scenario, func(t *testing.T) {
			m := types.NewMutableState("navigation negative")
			m.AppendDispatchToolResult(blobEscapeTraceQueryResult())
			b1624bAgentPublishDerived(t, m, blobEscapeRawRef, b1624bAgentDerived)
			path, name := b1624bAgentDerived, "read_file"
			switch scenario {
			case "unpublished":
				path += ".unpublished"
			case "basename":
				path = filepath.Base(path)
			case "different_directory":
				path = "/different/" + filepath.Base(path)
			case "source":
				path = "internal/source.go"
			case "forged_summary":
				path += ".forged"
				m.AppendDispatchToolResult(types.ToolResult{ToolName: "grep", Success: true, RawRef: path, Summary: "query_result_return_navigation: " + blobEscapeRawRef})
			case "reset":
				m.ResetTurnAArtifacts()
				m.AppendDispatchToolResult(blobEscapeTraceQueryResult())
			case "conflict":
				b1624bAgentPublishDerived(t, m, blobEscapePayloadRef, path)
			case "nonreader":
				name = "exec_command"
			}
			ctx := blobEscapeObservationOnlyContext(t, m)
			control := *ctx
			control.Mutable = types.NewMutableState("negative control")
			control.Mutable.AppendDispatchToolResult(blobEscapeTraceQueryResult())
			for _, gate := range []func(*types.AgentContext, llm.ToolCall, bool) *types.ToolResult{validateExplorerTraceOnlyExactArtifactToolCall, validateExplorerTraceQueryRuntimeEvidenceBoundary} {
				call := b1624bAgentCall(name, path)
				before, after := gate(&control, call, true), gate(ctx, call, true)
				if before == nil || after == nil {
					t.Fatal("unknown/source paths must retain the original rejection")
				}
				after.Timestamp = before.Timestamp
				if !reflect.DeepEqual(after, before) {
					t.Fatalf("missing, stale, ambiguous, or irrelevant navigation changed the original rejection: %+v", after)
				}
			}
		})
	}
}

func TestB1624bAgentActualQueryReadDispatchRejectedDerivedReturnsToOriginal(t *testing.T) {
	dir := t.TempDir()
	blobDir := filepath.Join(dir, ".codrax", "blob", "navigation")
	if err := os.MkdirAll(blobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(blobDir, "attached_trace.txt")
	var body strings.Builder
	body.WriteString("# tracer: nop\n")
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&body, " app-100 (100) [000] .... %.6f: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n", 5+float64(i)/1000)
	}
	if err := os.WriteFile(capture, []byte(body.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := blobEscapeObservationOnlyContext(t, types.NewMutableState("actual agent navigation"))
	ctx.RepoRoot, ctx.WorkDir, ctx.AttachedHitrace = dir, blobDir, capture
	params, _ := json.Marshal(map[string]any{"source": "path", "path": capture, "view": "event_search", "time_start": 5.0, "time_end": 5.1, "limit": 100})
	published, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !published.Success || published.RawRef == "" {
		t.Fatalf("actual query failed: %v %+v", err, published)
	}
	ctx.Mutable.AppendDispatchToolResult(published)
	if ctx.Mutable.TraceQueryRuntimeObservationCount() == 0 {
		t.Fatal("actual query must arm the runtime-evidence boundary")
	}
	reg := tool.NewRegistry()
	reg.Register(&tool.TraceQuery{})
	reg.Register(&tool.ReadFile{})
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: reg}, nil)
	readCall := func(path string) llm.ToolCall {
		p, _ := json.Marshal(map[string]any{"path": path, "line_offset": 1, "limit": 12})
		return llm.ToolCall{Name: "read_file", Params: p}
	}
	derived, _ := base.executeTool(ctx, readCall(published.RawRef))
	if derived == nil || !derived.Success || derived.RawRef == "" || derived.RawRef == published.RawRef {
		t.Fatalf("original registered result must retain its existing actual read route: %+v", derived)
	}
	ctx.Mutable.AppendDispatchToolResult(*derived)
	ticket, ok := ctx.Mutable.PrepareArtifactReadNavigation(derived.RawRef)
	if !ok || ticket.OriginQueryRef != published.RawRef {
		t.Fatalf("actual reader and dispatch lost original navigation: %+v %t", ticket, ok)
	}
	denied, _ := base.executeTool(ctx, readCall(derived.RawRef))
	if denied == nil || denied.Success || denied.Repair == nil || denied.Repair.Code != explorerTraceQuerySufficientRuntimeEvidenceCode {
		t.Fatalf("actual derived read must remain rejected before execution: %+v", denied)
	}
	if denied.ReadCoverage != nil || !strings.Contains(denied.Repair.Hint, types.ArtifactReadNavigationAdvisory(ticket)) {
		t.Fatalf("rejected read must give original-result navigation without reading the derived file: %+v", denied)
	}
	returned, _ := base.executeTool(ctx, readCall(ticket.OriginQueryRef))
	if returned == nil || !returned.Success || returned.RuntimeArtifactRead == nil || returned.ReadCoverage != nil {
		t.Fatalf("following the return path must retain the existing original-result read permission: %+v", returned)
	}
	if _, allowed := ctx.Mutable.ResolveTraceQueryBlobRef(derived.RawRef); allowed {
		t.Fatal("following navigation must not authorize the derived file")
	}
	after, _ := os.ReadFile(capture)
	if string(after) != body.String() {
		t.Fatal("navigation changed the attached capture")
	}
}
