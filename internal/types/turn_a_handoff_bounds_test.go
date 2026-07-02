package types

import (
	"fmt"
	"strings"
	"testing"
)

func boundTypesToolResult(name string, success bool, summaryLen int) ToolResult {
	return ToolResult{
		ToolName: name,
		Success:  success,
		Summary:  strings.Repeat("x", summaryLen),
	}
}

func boundTypesMCPResponse(server string, success bool, summaryLen int) MCPResponse {
	return MCPResponse{
		ServerName: server,
		Method:     "resources/read",
		Success:    success,
		Summary:    strings.Repeat("x", summaryLen),
	}
}

func TestBoundTurnAToolResultsSharedUtility(t *testing.T) {
	in := []ToolResult{
		boundTypesToolResult("grep", true, 2000),
		boundTypesToolResult("read_file", false, 2000),
		boundTypesToolResult("trace_query", false, 2000),
	}
	in[0].RawRef = "payload://first"
	in[2].RawRef = "payload://newest"

	// Value-ordered retention: the successful payload-bearing result outranks
	// the two failed results even though it is oldest; the byte cap then only
	// admits that one entry.
	out, truncation := BoundTurnAToolResultsWithTruncation(in, 2, 2500, PreserveSuccessfulToolResultWithPayload)
	if len(out) != 1 {
		t.Fatalf("expected only the preserved payload result to fit the byte cap, got %d: %+v", len(out), out)
	}
	if out[0].RawRef != "payload://first" || !out[0].Success {
		t.Fatalf("value-ordered retention should keep the successful payload result: %+v", out)
	}
	if !truncation.Active() || truncation.Dropped != 2 {
		t.Fatalf("expected truncation summary for 2 dropped results, got %+v", truncation)
	}
	if truncation.ByTool["read_file"] != 1 || truncation.ByTool["trace_query"] != 1 {
		t.Fatalf("truncation category counts mismatch: %+v", truncation.ByTool)
	}
	if label := truncation.Label(); !strings.Contains(label, "2 dropped") {
		t.Fatalf("truncation label should disclose the drop count, got %q", label)
	}
}

// TestBoundTurnAToolResultsValueOrderKeepsDeterministicRuntimeObservations is
// the Batch E1 pin: a retry window full of fresh grep noise must NOT evict
// earlier deterministic trace_query observations, the truncation must consume
// the grep noise first, and the drop must be reported.
func TestBoundTurnAToolResultsValueOrderKeepsDeterministicRuntimeObservations(t *testing.T) {
	traceResult := func(id string) ToolResult {
		return ToolResult{
			ToolName: "trace_query",
			Success:  true,
			Summary:  "typed trace observations " + id,
			Observations: []ObservationRecord{{
				ID:       id,
				Origin:   AnswerEvidenceOriginRuntimeArtifact,
				Producer: "trace_query",
				Summary:  "wakeup chain hop",
			}},
		}
	}
	countTrace := func(results []ToolResult) int {
		n := 0
		for _, r := range results {
			if r.ToolName == "trace_query" {
				n++
			}
		}
		return n
	}

	// Direction 1 (regression pin): trace observations EARLY, grep flood
	// LATER. Oldest-first dropping would evict every early trace result.
	early := []ToolResult{traceResult("trace-1"), traceResult("trace-2"), traceResult("trace-3")}
	for i := 0; i < 40; i++ {
		early = append(early, boundTypesToolResult("grep", true, 400))
	}
	out, truncation := BoundTurnAToolResultsWithTruncation(early, 10, 1<<20, PreserveSuccessfulToolResultWithPayload)
	if got := countTrace(out); got != 3 {
		t.Fatalf("early trace_query observations must survive a later grep flood, kept %d of 3: %+v", got, out)
	}
	if out[0].ToolName != "trace_query" || out[len(out)-1].ToolName != "grep" {
		t.Fatalf("chronological order must be preserved: %+v", out)
	}
	if !truncation.Active() || truncation.ByTool["grep"] != truncation.Dropped {
		t.Fatalf("grep noise must be dropped first, got %+v", truncation)
	}

	// Direction 2 (acceptance sequence): grep flood FIRST, trace_query later.
	late := make([]ToolResult, 0, 43)
	for i := 0; i < 40; i++ {
		late = append(late, boundTypesToolResult("grep", true, 400))
	}
	late = append(late, traceResult("trace-1"), traceResult("trace-2"), traceResult("trace-3"))
	out, truncation = BoundTurnAToolResultsWithTruncation(late, 10, 1<<20, PreserveSuccessfulToolResultWithPayload)
	if got := countTrace(out); got != 3 {
		t.Fatalf("late trace_query observations must survive, kept %d of 3: %+v", got, out)
	}
	if !truncation.Active() || truncation.ByTool["grep"] != truncation.Dropped || truncation.ByTool["trace_query"] != 0 {
		t.Fatalf("grep noise must be dropped first, got %+v", truncation)
	}
}

// TestBoundTurnAToolResultsDeterministicRuntimeFloor pins the floor: even a
// byte budget too small for the greedy pass keeps the newest deterministic
// runtime observation result.
func TestBoundTurnAToolResultsDeterministicRuntimeFloor(t *testing.T) {
	trace := ToolResult{
		ToolName: "trace_query",
		Success:  true,
		Summary:  strings.Repeat("t", 4000),
		Observations: []ObservationRecord{{
			ID:       "trace-floor",
			Origin:   AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query:run2",
			Summary:  "root cause rank",
		}},
	}
	in := []ToolResult{trace, boundTypesToolResult("grep", true, 100)}
	out, _ := BoundTurnAToolResultsWithTruncation(in, 1, 300, nil)
	found := false
	for _, r := range out {
		if len(r.Observations) > 0 && r.Observations[0].ID == "trace-floor" {
			found = true
		}
	}
	if !found {
		t.Fatalf("deterministic runtime observation result must be floor-preserved, got %+v", out)
	}
}

func TestBoundTurnAMCPResponsesSharedUtility(t *testing.T) {
	in := []MCPResponse{
		boundTypesMCPResponse("docs", true, 2000),
		boundTypesMCPResponse("docs", false, 2000),
		boundTypesMCPResponse("docs", false, 2000),
	}
	in[0].RawRef = "mcp://docs/first"
	in[2].RawRef = "mcp://docs/newest"

	out := BoundTurnAMCPResponses(in, 2, 2500, PreserveSuccessfulMCPResponseWithPayload)
	if len(out) != 2 {
		t.Fatalf("expected newest plus preserved MCP response, got %d: %+v", len(out), out)
	}
	if out[0].RawRef != "mcp://docs/first" || out[1].RawRef != "mcp://docs/newest" {
		t.Fatalf("preservation/order mismatch: %+v", out)
	}
}

func TestTurnAArtifactsExploreForkMergeBoundsToolAndMCPResults(t *testing.T) {
	parent := NewMutableState("")
	parent.SetTurnAArtifacts(TurnAArtifacts{
		ToolResults:  []ToolResult{{ToolName: "grep", Summary: "base", Success: true}},
		MCPResponses: []MCPResponse{{ServerName: "docs", Method: "resources/read", Summary: "base", Success: true}},
	})
	fork := parent.ForkForExploreDispatch()
	ta := fork.TurnAArtifacts()
	for i := 0; i < turnAArtifactsMutableToolResultsCountCap+50; i++ {
		r := boundTypesToolResult("read_file", false, 10)
		r.RawRef = fmt.Sprintf("tool-%03d", i)
		ta.ToolResults = append(ta.ToolResults, r)
	}
	for i := 0; i < turnAArtifactsMutableMCPResponsesCountCap+50; i++ {
		r := boundTypesMCPResponse("docs", false, 10)
		r.RawRef = fmt.Sprintf("mcp-%03d", i)
		ta.MCPResponses = append(ta.MCPResponses, r)
	}
	fork.SetTurnAArtifacts(*ta)

	parent.MergeExploreFork(fork)
	got := parent.TurnAArtifacts()
	if got == nil {
		t.Fatal("expected merged TurnAArtifacts")
	}
	if len(got.ToolResults) != turnAArtifactsMutableToolResultsCountCap {
		t.Fatalf("tool results not bounded to the merged count cap: got %d", len(got.ToolResults))
	}
	if got.ToolResults[0].Summary != "base" {
		t.Fatalf("base successful tool result should be preserved at head: %+v", got.ToolResults[0])
	}
	wantNewestTool := fmt.Sprintf("tool-%03d", turnAArtifactsMutableToolResultsCountCap+49)
	if got.ToolResults[len(got.ToolResults)-1].RawRef != wantNewestTool {
		t.Fatalf("newest tool result should survive, got %+v", got.ToolResults[len(got.ToolResults)-1])
	}
	if !got.ToolResultTruncation.Active() || got.ToolResultTruncation.ByTool["read_file"] == 0 {
		t.Fatalf("merged truncation summary should report dropped read_file noise, got %+v", got.ToolResultTruncation)
	}
	if len(got.MCPResponses) != turnAArtifactsMutableMCPResponsesCountCap+1 {
		t.Fatalf("MCP responses not bounded with preserved base: got %d", len(got.MCPResponses))
	}
	if got.MCPResponses[0].Summary != "base" {
		t.Fatalf("base successful MCP response should be preserved at head: %+v", got.MCPResponses[0])
	}
	wantNewestMCP := fmt.Sprintf("mcp-%03d", turnAArtifactsMutableMCPResponsesCountCap+49)
	if got.MCPResponses[len(got.MCPResponses)-1].RawRef != wantNewestMCP {
		t.Fatalf("newest MCP response should survive, got %+v", got.MCPResponses[len(got.MCPResponses)-1])
	}
}
