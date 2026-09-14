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
	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1680IndependentActualQueryReadHintPreservesAuthority(t *testing.T) {
	root := t.TempDir()
	blobDir := filepath.Join(root, ".codrax", "blob", "independent-handoff")
	if err := os.MkdirAll(blobDir, 0755); err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(blobDir, "capture.systrace")
	var body strings.Builder
	body.WriteString("# tracer: nop\n")
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&body, " app-100 (100) [000] .... %.6f: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n", 5+float64(i)/1000)
	}
	if err := os.WriteFile(capture, []byte(body.String()), 0644); err != nil {
		t.Fatal(err)
	}
	ctx := blobEscapeObservationOnlyContext(t, types.NewMutableState("independent runtime handoff"))
	ctx.RepoRoot, ctx.WorkDir, ctx.AttachedHitrace = root, blobDir, capture
	params, _ := json.Marshal(map[string]any{"source": "path", "path": capture, "view": "event_search", "time_start": 5.0, "time_end": 5.1, "limit": 100})
	published, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !published.Success || published.RawRef == "" {
		t.Fatalf("actual query: %v %+v", err, published)
	}
	ctx.Mutable.AppendDispatchToolResult(published)
	if ctx.Mutable.TraceQueryRuntimeObservationCount() == 0 {
		t.Fatal("public query must establish the actual runtime boundary")
	}
	reg := tool.NewRegistry()
	reg.Register(&tool.ReadFile{})
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: reg}, nil)
	readCall := func(path string, offset int) llm.ToolCall {
		p, _ := json.Marshal(map[string]any{"path": path, "line_offset": offset, "limit": 3})
		return llm.ToolCall{Name: "read_file", Params: p}
	}
	results := []types.ToolResult{published}
	for _, offset := range []int{1, 5} {
		read, _ := base.executeTool(ctx, readCall(published.RawRef, offset))
		if read == nil || !read.Success || read.RuntimeArtifactRead == nil || !read.RuntimeArtifactRead.TraceQueryBlob || read.ReadCoverage != nil {
			t.Fatalf("actual blob reader must remain source-inert: %+v", read)
		}
		results = append(results, *read)
		ctx.Mutable.AppendDispatchToolResult(*read)
	}
	derived := results[len(results)-1].RawRef
	if derived == "" || derived == published.RawRef {
		t.Fatal("actual read must produce a distinct navigation-only ref")
	}
	deniedBefore, _ := base.executeTool(ctx, readCall(derived, 1))
	if deniedBefore == nil || deniedBefore.Success || deniedBefore.Repair == nil {
		t.Fatalf("derived reference must already be denied: %+v", deniedBefore)
	}
	ctx.Mutable.EvidenceClosure().AddPendingRead(types.PendingRead{File: "still-needed.go", Origin: "phase1_unread"})
	pendingBefore, _ := json.Marshal(ctx.Mutable.EvidenceClosure().PendingReads())
	dispatchBefore, _ := json.Marshal(ctx.Mutable.DispatchToolResults())
	resultsBefore, _ := json.Marshal(results)
	observationsBefore := ctx.Mutable.TraceQueryRuntimeObservationCount()
	eval := &explorerEvaluator{phase: 1, mutable: ctx.Mutable, analysisIR: ctx.AnalysisIR, searchResult: &keywordSearchResult{Graph: &repomap.Graph{}}}
	schemas := []llm.ToolSchema{{Name: "read_file"}, {Name: "trace_query"}, {Name: "emit_evidence"}, {Name: "emit_investigation_complete"}}
	surfaceBefore := eval.FilterToolSchemas(ctx, schemas)
	sig := eval.Observe(ctx, LoopObservation{Phase: PhaseMidLoop, Iteration: 3, AllToolResults: results, LastToolResult: &results[len(results)-1]})
	if !sig.HintRequested || !strings.Contains(sig.HintKey, "read-without-emit") || !strings.Contains(sig.Hint, "artifact-only read backlog") || !strings.Contains(sig.Hint, "`trace_query`") {
		t.Errorf("public query result reads need artifact handoff, not source citation teaching: %+v", sig)
	}
	if sig.StopRequested || eval.investigationComplete || !reflect.DeepEqual(surfaceBefore, eval.FilterToolSchemas(ctx, schemas)) {
		t.Fatal("soft read handoff changed terminal state or tool permissions")
	}
	_, readSet, readRanges := extractFileCoverage(results, root)
	if len(readSet) != 0 || len(readRanges) != 0 {
		t.Fatalf("runtime hints must not mint source read coverage: %+v %+v", readSet, readRanges)
	}
	pendingAfter, _ := json.Marshal(ctx.Mutable.EvidenceClosure().PendingReads())
	dispatchAfter, _ := json.Marshal(ctx.Mutable.DispatchToolResults())
	resultsAfter, _ := json.Marshal(results)
	if string(pendingBefore) != string(pendingAfter) || string(dispatchBefore) != string(dispatchAfter) || string(resultsBefore) != string(resultsAfter) || observationsBefore != ctx.Mutable.TraceQueryRuntimeObservationCount() {
		t.Fatal("soft hint mutated pending source obligations, original results, or runtime observations")
	}
	deniedAfter, _ := base.executeTool(ctx, readCall(derived, 1))
	if deniedAfter == nil {
		t.Fatal("derived reference denial disappeared")
	}
	deniedAfter.Timestamp = deniedBefore.Timestamp
	if !reflect.DeepEqual(deniedBefore, deniedAfter) {
		t.Fatalf("hint changed precise derived-reference rejection: before=%+v after=%+v", deniedBefore, deniedAfter)
	}
	allowed, _ := base.executeTool(ctx, readCall(published.RawRef, 1))
	if allowed == nil || !allowed.Success || allowed.ReadCoverage != nil || allowed.RuntimeArtifactRead == nil {
		t.Fatalf("original query ref lost its existing runtime-only read permission: %+v", allowed)
	}
	if _, granted := ctx.Mutable.ResolveTraceQueryBlobRef(derived); granted {
		t.Fatal("hint promoted a derived reference to readable query authority")
	}
	if after, err := os.ReadFile(capture); err != nil || string(after) != body.String() {
		t.Fatal("read-only handoff changed the capture")
	}
}

func TestB1680IndependentActualSourceCoverageSurvivesMixedHint(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("package example\nfunc Value() int { return 7 }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "capture.log"), []byte("runtime observation\nanother event\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{RepoRoot: root, WorkDir: root, Mutable: types.NewMutableState("mixed source/runtime handoff")}
	var results []types.ToolResult
	for _, path := range []string{"capture.log", "source.go"} {
		params, _ := json.Marshal(map[string]any{"path": path})
		read, err := (&tool.ReadFile{}).Execute(bus, params)
		if err != nil || !read.Success {
			t.Fatalf("actual %s read: %v %+v", path, err, read)
		}
		results = append(results, read)
	}
	if results[0].RuntimeArtifactRead == nil || results[0].ReadCoverage != nil || results[1].RuntimeArtifactRead != nil || results[1].ReadCoverage == nil {
		t.Fatal("mixed producer preconditions not established")
	}
	_, readSetBefore, rangesBefore := extractFileCoverage(results, root)
	if len(readSetBefore) != 1 || !readSetBefore["source.go"] {
		t.Fatalf("actual source read must still enter the exact source coverage lane: %+v", readSetBefore)
	}
	ctx := &types.AgentContext{Stage: types.StageExplore, RepoRoot: root, WorkDir: root, Mutable: bus.Mutable}
	eval := &explorerEvaluator{phase: 1, mutable: bus.Mutable, searchResult: &keywordSearchResult{Graph: &repomap.Graph{}}}
	sig := eval.Observe(ctx, LoopObservation{Phase: PhaseMidLoop, Iteration: 3, AllToolResults: results, LastToolResult: &results[1]})
	if !sig.HintRequested || strings.Contains(sig.Hint, "artifact-only read backlog") || !strings.Contains(sig.Hint, "current-source") || !strings.Contains(sig.Hint, "aggregate_facts") {
		t.Errorf("mixed backlog must preserve both source and artifact landing obligations: %+v", sig)
	}
	_, readSetAfter, rangesAfter := extractFileCoverage(results, root)
	if !reflect.DeepEqual(readSetBefore, readSetAfter) || !reflect.DeepEqual(rangesBefore, rangesAfter) || sig.StopRequested {
		t.Fatal("mixed hint changed current-source coverage or requested completion")
	}
}

// A new single-large-read advisory must not create a source-materialization
// gate where the old source-coverage-only trigger did not fire. In particular,
// missing request-origin classification is not proof that artifact bytes are
// current source. The existing source escalation path is tested elsewhere.
func TestB1680IndependentLargeRuntimeHintDoesNotCreateSourceOnlySurface(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".codrax", "blob", "runtime-observations.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("runtime observation\n", 1100)), 0644); err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{RepoRoot: root, WorkDir: root, Mutable: types.NewMutableState("unknown request origin")}
	params, _ := json.Marshal(map[string]any{"path": path, "line_offset": 400, "limit": 120})
	read, err := (&tool.ReadFile{}).Execute(bus, params)
	if err != nil || !read.Success || read.RuntimeArtifactRead == nil || read.ReadCoverage != nil || read.RuntimeArtifactRead.LineEnd-read.RuntimeArtifactRead.LineStart+1 != 120 {
		t.Fatalf("actual single large runtime read required: %v %+v", err, read)
	}
	ctx := &types.AgentContext{Stage: types.StageExplore, RepoRoot: root, WorkDir: root, Mutable: bus.Mutable}
	eval := &explorerEvaluator{phase: 1, mutable: bus.Mutable, searchResult: &keywordSearchResult{Graph: &repomap.Graph{}}}
	schemas := []llm.ToolSchema{{Name: "read_file"}, {Name: "grep"}, {Name: "trace_query"}, {Name: "emit_evidence"}, {Name: "emit_investigation_complete"}}
	before := eval.FilterToolSchemas(ctx, schemas)
	results := []types.ToolResult{read}
	first := eval.postReadWithoutEmitSignal(LoopObservation{Phase: PhaseMidLoop, Iteration: 1, AllToolResults: results, LastToolResult: &read})
	if first.HintRequested || eval.midLoopNoEmitPushSent {
		t.Errorf("single runtime read must not newly arm a source-evidence nudge: %+v", first)
	}
	for _, line := range []int{401, 405} {
		params, _ := json.Marshal(map[string]any{"path": path, "pattern": "observation", "fixed_string": true, "line_start": line, "line_end": line + 1})
		navigation, err := (&tool.GrepTool{}).Execute(bus, params)
		if err != nil || !navigation.Success {
			t.Fatalf("actual bounded follow-up navigation required: %v %+v", err, navigation)
		}
		results = append(results, navigation)
	}
	escalated := eval.postReadWithoutEmitEscalationSignal(LoopObservation{Phase: PhaseMidLoop, Iteration: 3, AllToolResults: results, LastToolResult: &results[len(results)-1]})
	if escalated.HintRequested || eval.midLoopNoEmitEscalated {
		t.Errorf("bounded navigation after one runtime read must not create new source escalation: %+v", escalated)
	}
	if got := eval.FilterToolSchemas(ctx, schemas); !reflect.DeepEqual(before, got) {
		t.Fatalf("guidance-only runtime read created a source-only tool surface: before=%+v after=%+v first=%+v escalated=%+v", before, got, first, escalated)
	}
	if first.StopRequested || escalated.StopRequested || eval.investigationComplete {
		t.Fatal("read guidance must not complete or stop the investigation")
	}
}
