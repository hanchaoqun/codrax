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

	out := BoundTurnAToolResults(in, 2, 2500, PreserveSuccessfulToolResultWithPayload)
	if len(out) != 2 {
		t.Fatalf("expected newest plus preserved payload result, got %d: %+v", len(out), out)
	}
	if out[0].RawRef != "payload://first" || out[1].RawRef != "payload://newest" {
		t.Fatalf("preservation/order mismatch: %+v", out)
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
	if len(got.ToolResults) != turnAArtifactsMutableToolResultsCountCap+1 {
		t.Fatalf("tool results not bounded with preserved base: got %d", len(got.ToolResults))
	}
	if got.ToolResults[0].Summary != "base" {
		t.Fatalf("base successful tool result should be preserved at head: %+v", got.ToolResults[0])
	}
	wantNewestTool := fmt.Sprintf("tool-%03d", turnAArtifactsMutableToolResultsCountCap+49)
	if got.ToolResults[len(got.ToolResults)-1].RawRef != wantNewestTool {
		t.Fatalf("newest tool result should survive, got %+v", got.ToolResults[len(got.ToolResults)-1])
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
