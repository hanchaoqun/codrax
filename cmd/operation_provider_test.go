package cmd

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mcp"
	"github.com/hanchaoqun/codrax/internal/types"
)

type operationProviderFakeMCP struct{ name string }

func (s operationProviderFakeMCP) Name() string                        { return s.name }
func (s operationProviderFakeMCP) Transport() types.TransportType      { return types.TransportStdio }
func (s operationProviderFakeMCP) ListTools() []mcp.ToolSchema         { return nil }
func (s operationProviderFakeMCP) ListResources() []mcp.ResourceSchema { return nil }
func (s operationProviderFakeMCP) ReadResource(string) (types.MCPResponse, error) {
	return types.MCPResponse{}, nil
}
func (s operationProviderFakeMCP) ListPrompts() []mcp.PromptSchema { return nil }
func (s operationProviderFakeMCP) CallTool(string, json.RawMessage) (types.MCPResponse, error) {
	return types.MCPResponse{}, nil
}
func (s operationProviderFakeMCP) Close() error { return nil }

func TestOperationProvidersFromMCPConfigsRequiresOptInAndRegisteredServer(t *testing.T) {
	reg := mcp.NewRegistry()
	if err := reg.Register(operationProviderFakeMCP{name: "slides"}); err != nil {
		t.Fatalf("register fake MCP: %v", err)
	}
	yes := true
	no := false
	providers := operationProvidersFromMCPConfigs(reg, []types.MCPServerConfig{
		{Name: "slides", OperationProvider: &yes, OperationKinds: []string{"presentation_generation", "document_generation"}, OperationSurfaces: []string{"slides"}, OperationSideEffects: []string{"local_file_write"}, OperationTool: "run_operation", OperationRequiresConfirmation: &yes},
		{Name: "docs", OperationProvider: &yes, OperationKinds: []string{"document_generation"}}, // not registered
		{Name: "plain", OperationProvider: &no, OperationKinds: []string{"browser_operation"}},
	})

	if len(providers) != 2 {
		t.Fatalf("providers len=%d, want 2: %+v", len(providers), providers)
	}
	if providers[0].Name != "mcp:slides" || providers[0].Kind != "presentation_generation" || !providers[0].RequiresGate || providers[0].ToolName != "run_operation" {
		t.Fatalf("first provider mismatch: %+v", providers[0])
	}
	if providers[1].Kind != "document_generation" {
		t.Fatalf("second provider kind=%q", providers[1].Kind)
	}
}
