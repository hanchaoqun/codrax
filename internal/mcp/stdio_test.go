package mcp

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestFakeMCPServerHelper(t *testing.T) {
	if os.Getenv("CODRAX_FAKE_MCP_SERVER") != "1" {
		return
	}
	defer os.Exit(0)
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var req rpcRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}
		switch req.Method {
		case "initialize":
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"protocolVersion": defaultProtocolVersion,
					"capabilities":    map[string]any{},
					"serverInfo":      map[string]string{"name": "fake", "version": "test"},
				},
			})
		case "notifications/initialized":
			continue
		case "tools/list":
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"tools": []map[string]any{
						{
							"name":        "echo_tool",
							"description": "echoes a value",
							"inputSchema": map[string]any{
								"type":       "object",
								"properties": map[string]any{"value": map[string]string{"type": "string"}},
							},
						},
						{
							"name":        "slow_tool",
							"description": "responds after the caller times out",
							"inputSchema": map[string]any{"type": "object"},
						},
					},
				},
			})
		case "tools/call":
			var params struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &params)
			if params.Name == "slow_tool" {
				time.Sleep(80 * time.Millisecond)
			}
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"content": []map[string]string{{"type": "text", "text": "called " + params.Name}},
					"isError": false,
				},
			})
		case "resources/list":
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"resources": []map[string]string{{
						"uri":         "mcp://fake/note",
						"name":        "note",
						"description": "fake note",
						"mimeType":    "text/plain",
					}},
				},
			})
		case "resources/read":
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"contents": []map[string]string{{
						"uri":      "mcp://fake/note",
						"mimeType": "text/plain",
						"text":     "resource note body",
					}},
				},
			})
		case "prompts/list":
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"prompts": []map[string]string{{"name": "runbook", "description": "fake runbook"}},
				},
			})
		}
	}
}

func fakeServer(t *testing.T) *StdioServer {
	t.Helper()
	s, err := NewStdioServerFromConfig(types.MCPServerConfig{
		Name:      "fake",
		Transport: types.TransportStdio,
		Command:   os.Args[0],
		Args:      []string{"-test.run=TestFakeMCPServerHelper"},
		Env:       map[string]string{"CODRAX_FAKE_MCP_SERVER": "1"},
	})
	if err != nil {
		t.Fatalf("NewStdioServerFromConfig: %v", err)
	}
	return s
}

func TestStdioServerInitializeListAndCallTool(t *testing.T) {
	s := fakeServer(t)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Initialize(nil); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	tools := s.ListTools()
	if len(tools) != 2 || tools[0].Name != "echo_tool" {
		t.Fatalf("unexpected tools: %+v", tools)
	}
	resp, err := s.CallTool("echo_tool", json.RawMessage(`{"value":"ok"}`))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !resp.Success || !strings.Contains(resp.Summary, "called echo_tool") {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestStdioServerLateResponseDoesNotPoisonNextCall(t *testing.T) {
	s := fakeServer(t)
	s.callTimeout = 10 * time.Millisecond
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Initialize(nil); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if _, err := s.CallTool("slow_tool", json.RawMessage(`{}`)); err == nil {
		t.Fatal("slow_tool should time out")
	}
	s.callTimeout = time.Second
	resp, err := s.CallTool("echo_tool", json.RawMessage(`{"value":"ok"}`))
	if err != nil {
		t.Fatalf("second CallTool should not receive the late slow_tool response: %v", err)
	}
	if !strings.Contains(resp.Summary, "called echo_tool") {
		t.Fatalf("late response poisoned second call: %+v", resp)
	}
}

func TestStdioServerResourcesAndPrompts(t *testing.T) {
	s := fakeServer(t)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Initialize(nil); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	resources := s.ListResources()
	if len(resources) != 1 || resources[0].URI != "mcp://fake/note" {
		t.Fatalf("unexpected resources: %+v", resources)
	}
	prompts := s.ListPrompts()
	if len(prompts) != 1 || prompts[0].Name != "runbook" {
		t.Fatalf("unexpected prompts: %+v", prompts)
	}
	resp, err := s.ReadResource("mcp://fake/note")
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if !resp.Success || resp.ResourceURI != "mcp://fake/note" || !strings.Contains(resp.Summary, "resource note body") {
		t.Fatalf("unexpected resource response: %+v", resp)
	}
}

func TestDecodeToolCallResultTypedObservationEnvelope(t *testing.T) {
	raw := json.RawMessage(`{
		"content":[{
			"type":"text",
			"mimeType":"application/vnd.codrax.observation+json",
			"text":"{\"summary\":\"trace facts\",\"resource_uri\":\"mcp://trace/run\",\"observations\":[{\"summary\":\"sleep entry\",\"line_start\":\"1102717\"},{\"summary\":\"wakeup\",\"line_start\":1139180,\"line_end\":1139180,\"selector\":\"pid=36379\"}]}"
		}],
		"isError":false
	}`)
	got := decodeToolCallResult(raw)
	if got.IsError || got.MIMEType != "application/vnd.codrax.observation+json" {
		t.Fatalf("unexpected typed result flags: %+v", got)
	}
	if !strings.Contains(got.Summary, "trace facts") {
		t.Fatalf("typed envelope should use compact summary, got %q", got.Summary)
	}
	if len(got.Observations) != 2 {
		t.Fatalf("typed envelope should produce two observations, got %+v", got.Observations)
	}
	if got.Observations[0].ResourceURI != "mcp://trace/run" ||
		got.Observations[0].LineStart != 1102717 ||
		got.Observations[1].LineStart != 1139180 ||
		got.Observations[1].Selector != "pid=36379" {
		t.Fatalf("typed coordinates not preserved: %+v", got.Observations)
	}
}

func TestDecodeToolCallResultOrdinaryJSONDoesNotActivateTypedCoordinates(t *testing.T) {
	raw := json.RawMessage(`{
		"content":[{
			"type":"text",
			"mimeType":"application/json",
			"text":"{\"summary\":\"not a codrax envelope\",\"line_start\":42}"
		}],
		"isError":false
	}`)
	got := decodeToolCallResult(raw)
	if len(got.Observations) != 0 {
		t.Fatalf("ordinary JSON must not become typed line evidence: %+v", got.Observations)
	}
	if !strings.Contains(got.Summary, "line_start") {
		t.Fatalf("ordinary JSON should remain normal summary text, got %q", got.Summary)
	}
}

func TestDecodeResourceReadResultTypedEnvelopeInheritsURIAndSanitizesRange(t *testing.T) {
	raw := json.RawMessage(`{
		"contents":[{
			"uri":"mcp://docs/spec",
			"mimeType":"application/vnd.codrax.observation+json",
			"text":"{\"summary\":\"spec row\",\"line_start\":50,\"line_end\":40}"
		}]
	}`)
	got := decodeResourceReadResult(raw, "mcp://docs/fallback")
	if len(got.Observations) != 1 {
		t.Fatalf("resource typed envelope should produce one observation: %+v", got)
	}
	obs := got.Observations[0]
	if obs.ResourceURI != "mcp://docs/spec" || obs.LineStart != 50 || obs.LineEnd != 0 {
		t.Fatalf("resource URI inheritance/range sanitization failed: %+v", obs)
	}
}

func TestRegistryExposesNamespacedTools(t *testing.T) {
	reg := NewRegistry()
	s := fakeServer(t)
	s.tools = []ToolSchema{{Name: "fetch.logs", Description: "fetch logs", Parameters: defaultToolParameters}}
	if err := reg.Register(s); err != nil {
		t.Fatalf("Register: %v", err)
	}
	tools := reg.ListAllTools()
	if len(tools) != 1 || tools[0].Name != "fake__fetch_logs" {
		t.Fatalf("unexpected namespaced tools: %+v", tools)
	}
	server, raw, ok := reg.ResolveNamespaced("fake__fetch_logs")
	if !ok || server.Name() != "fake" || raw != "fetch.logs" {
		t.Fatalf("ResolveNamespaced failed: server=%v raw=%q ok=%t", server, raw, ok)
	}
}
