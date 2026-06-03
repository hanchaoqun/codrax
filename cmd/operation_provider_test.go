package cmd

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mcp"
	"github.com/hanchaoqun/codrax/internal/operation"
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
		{Name: "slides", OperationProvider: &yes, OperationKinds: []string{"presentation_generation", "document_generation"}, OperationSurfaces: []string{"slides"}, OperationSideEffects: []string{"local_file_write"}, OperationTool: "run_operation", OperationDescription: "Create decks", OperationInputSchema: `{"type":"object"}`, OperationExamples: []string{"make a deck"}, OperationLazyStart: &yes, OperationRequiresConfirmation: &yes},
		{Name: "docs", OperationProvider: &yes, OperationKinds: []string{"document_generation"}}, // not registered
		{Name: "plain", OperationProvider: &no, OperationKinds: []string{"browser_operation"}},
	})

	if len(providers) != 2 {
		t.Fatalf("providers len=%d, want 2: %+v", len(providers), providers)
	}
	if providers[0].Name != "mcp:slides" || providers[0].Kind != "presentation_generation" || !providers[0].RequiresGate || providers[0].ToolName != "run_operation" {
		t.Fatalf("first provider mismatch: %+v", providers[0])
	}
	if providers[0].Description != "Create decks" || providers[0].InputSchema == "" || len(providers[0].Examples) != 1 || !providers[0].LazyStart || providers[0].Source != "mcp" || !providers[0].Loaded {
		t.Fatalf("first provider descriptor not preserved: %+v", providers[0])
	}
	if providers[1].Kind != "document_generation" {
		t.Fatalf("second provider kind=%q", providers[1].Kind)
	}
}

func TestOperationProvidersFromMCPConfigsIncludesLazyUnregisteredProvider(t *testing.T) {
	reg := mcp.NewRegistry()
	yes := true
	providers := operationProvidersFromMCPConfigs(reg, []types.MCPServerConfig{
		{Name: "lazy_slides", OperationProvider: &yes, OperationLazyStart: &yes, OperationKinds: []string{"presentation_generation"}, OperationTool: "run_operation"},
		{Name: "eager_missing", OperationProvider: &yes, OperationKinds: []string{"document_generation"}, OperationTool: "run_operation"},
	})
	if len(providers) != 1 {
		t.Fatalf("providers len=%d, want 1: %+v", len(providers), providers)
	}
	if providers[0].Name != "mcp:lazy_slides" || !providers[0].LazyStart || providers[0].Loaded {
		t.Fatalf("lazy provider mismatch: %+v", providers[0])
	}
}

func TestOperationProvidersFromSkillConfigsUsesLocalDescriptor(t *testing.T) {
	no := false
	providers := operationProvidersFromSkillConfigs([]types.OperationSkillConfig{
		{
			Name:                          "local_ppt",
			Description:                   "Preferred deck generator",
			WhenToUse:                     []string{"Use for local PPTX decks"},
			WhenNotToUse:                  []string{"Do not use for source-code explanation"},
			OperationKinds:                []string{"presentation_generation", "artifact_generation"},
			OperationSurfaces:             []string{"slides"},
			OperationSideEffects:          []string{"local_file_write"},
			OperationRequiresConfirmation: &no,
			OperationDescription:          "Generate local decks",
			OperationInputSchema:          `{"type":"object"}`,
			OperationExamples:             []string{"make a deck"},
			OperationLazyStart:            &no,
			Command:                       "./tools/ppt-skill",
		},
	})
	if len(providers) != 2 {
		t.Fatalf("providers len=%d, want 2: %+v", len(providers), providers)
	}
	if providers[0].Name != "skill:local_ppt" || providers[0].Kind != "presentation_generation" || providers[0].ToolName != "run" {
		t.Fatalf("first provider mismatch: %+v", providers[0])
	}
	if providers[0].Source != "skill" || providers[0].LazyStart || !providers[0].Loaded || providers[0].RequiresGate {
		t.Fatalf("local skill descriptor flags mismatch: %+v", providers[0])
	}
	if providers[0].Description != "Preferred deck generator" || providers[0].InputSchema == "" || len(providers[0].Examples) != 1 || len(providers[0].WhenToUse) != 1 || len(providers[0].WhenNotToUse) != 1 {
		t.Fatalf("local skill descriptor metadata missing: %+v", providers[0])
	}
}

func TestOperationProvidersFromSkillConfigsAddsEntryWorkflowProvider(t *testing.T) {
	no := false
	yes := true
	providers := operationProvidersFromSkillConfigs([]types.OperationSkillConfig{
		{
			Name:                          "manual_reader",
			Description:                   "Read manuals and hand off to artifact builders",
			WhenToUse:                     []string{"Use when a task needs learning an unfamiliar tool manual"},
			WhenNotToUse:                  []string{"Do not use for pure source analysis"},
			OperationKinds:                []string{"artifact_generation"},
			OperationSurfaces:             []string{"local_file"},
			OperationSideEffects:          []string{"local_file_write"},
			OperationRequiresConfirmation: &no,
			Workflows: []types.OperationSkillWorkflowConfig{{
				Name:           "manual_to_deck",
				Summary:        "Read a manual and call a deck builder",
				Entry:          &yes,
				OperationKind:  "external_skill_workflow",
				TargetSurface:  "slides",
				NextProviders:  []string{"skill:ppt_builder"},
				ReturnProvider: "skill:manual_reader",
				Steps: []types.OperationSkillWorkflowStepConfig{{
					ID:            "build_deck",
					Provider:      "skill:ppt_builder",
					OperationKind: "presentation_generation",
					TargetSurface: "slides",
					Description:   "Generate slides from the extracted summary",
				}},
			}},
			OutputContract: types.OperationSkillOutputContract{
				ArtifactRefs:  &yes,
				NextActions:   &yes,
				WorkflowState: &yes,
			},
			Command: "./tools/manual-reader",
		},
	})
	var workflowProvider operation.ProviderInfo
	for _, provider := range providers {
		if provider.Kind == "external_skill_workflow" {
			workflowProvider = provider
			break
		}
	}
	if workflowProvider.Name != "skill:manual_reader" {
		t.Fatalf("workflow provider not found: %+v", providers)
	}
	if len(workflowProvider.Workflows) != 1 || workflowProvider.Workflows[0].Name != "manual_to_deck" {
		t.Fatalf("workflow metadata missing: %+v", workflowProvider)
	}
	if !workflowProvider.OutputContract.NextActions || !workflowProvider.OutputContract.WorkflowState {
		t.Fatalf("output contract missing: %+v", workflowProvider.OutputContract)
	}
	if len(workflowProvider.Surfaces) != 2 || workflowProvider.Surfaces[0] != "local_file" || workflowProvider.Surfaces[1] != "slides" {
		t.Fatalf("workflow surfaces=%v", workflowProvider.Surfaces)
	}
}

func TestOperationProvidersFromSkillConfigsSkipsIncompleteConfigs(t *testing.T) {
	providers := operationProvidersFromSkillConfigs([]types.OperationSkillConfig{
		{Name: "missing_command", OperationKinds: []string{"presentation_generation"}},
		{Command: "./tool", OperationKinds: []string{"document_generation"}},
	})
	if len(providers) != 0 {
		t.Fatalf("providers len=%d, want 0: %+v", len(providers), providers)
	}
}

func TestEagerMCPServerConfigsSkipsOnlyLazyOperationProviders(t *testing.T) {
	yes := true
	no := false
	cfgs := []types.MCPServerConfig{
		{Name: "lazy_op", OperationProvider: &yes, OperationLazyStart: &yes},
		{Name: "not_op_lazy_flag", OperationProvider: &no, OperationLazyStart: &yes},
		{Name: "eager_op", OperationProvider: &yes},
	}
	got := eagerMCPServerConfigs(cfgs)
	if len(got) != 2 {
		t.Fatalf("eager configs len=%d, want 2: %+v", len(got), got)
	}
	if got[0].Name != "not_op_lazy_flag" || got[1].Name != "eager_op" {
		t.Fatalf("unexpected eager configs: %+v", got)
	}
}
