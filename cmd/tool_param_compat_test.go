package cmd

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestResolveToolParamCompatByAgent_DefaultInheritance(t *testing.T) {
	cfg := &types.ProvidersConfig{
		LLM: types.LLMProvidersConfig{
			Default: types.LLMProviderConfig{
				ToolParamCompat: &types.ToolParamCompatConfig{Mode: "repair"},
			},
			Agents: map[string]types.LLMProviderConfig{
				string(types.AgentFinalizer): {
					ToolParamCompat: &types.ToolParamCompatConfig{Mode: "off"},
				},
			},
		},
	}

	got, err := resolveToolParamCompatByAgent(cfg)
	if err != nil {
		t.Fatalf("resolveToolParamCompatByAgent: %v", err)
	}
	if got[types.AgentAnalyzer].NormalizedMode() != types.ToolParamCompatRepair {
		t.Fatalf("analyzer did not inherit repair mode: %+v", got[types.AgentAnalyzer])
	}
	if _, exists := got[types.AgentFinalizer]; exists {
		t.Fatalf("finalizer off override should be omitted from active policy map: %+v", got[types.AgentFinalizer])
	}
}

func TestResolveToolParamCompatByAgent_AbsentConfigDefaultsToSchemaSafeRepair(t *testing.T) {
	cfg := &types.ProvidersConfig{}

	got, err := resolveToolParamCompatByAgent(cfg)
	if err != nil {
		t.Fatalf("resolveToolParamCompatByAgent: %v", err)
	}
	for _, name := range types.AllAgentNames() {
		compat, ok := got[name]
		if !ok {
			t.Fatalf("missing default tool_param_compat policy for %s", name)
		}
		if compat.NormalizedMode() != types.ToolParamCompatRepair {
			t.Fatalf("%s default mode = %q, want repair", name, compat.NormalizedMode())
		}
		if compat.SplitStringArraysEnabled() {
			t.Fatalf("%s default must keep delimited string array split opt-in", name)
		}
	}
}

func TestResolveToolParamCompatByAgent_DefaultOffDisablesUnlessAgentOverrides(t *testing.T) {
	cfg := &types.ProvidersConfig{
		LLM: types.LLMProvidersConfig{
			Default: types.LLMProviderConfig{
				ToolParamCompat: &types.ToolParamCompatConfig{Mode: types.ToolParamCompatOff},
			},
			Agents: map[string]types.LLMProviderConfig{
				string(types.AgentExplorer): {
					ToolParamCompat: &types.ToolParamCompatConfig{Mode: types.ToolParamCompatRepair},
				},
			},
		},
	}

	got, err := resolveToolParamCompatByAgent(cfg)
	if err != nil {
		t.Fatalf("resolveToolParamCompatByAgent: %v", err)
	}
	if _, exists := got[types.AgentAnalyzer]; exists {
		t.Fatalf("default off should disable analyzer policy: %+v", got[types.AgentAnalyzer])
	}
	if got[types.AgentExplorer].NormalizedMode() != types.ToolParamCompatRepair {
		t.Fatalf("explorer override should enable repair, got %+v", got[types.AgentExplorer])
	}
}

func TestResolveToolParamCompatByAgent_InvalidModeFailsLoud(t *testing.T) {
	cfg := &types.ProvidersConfig{
		LLM: types.LLMProvidersConfig{
			Agents: map[string]types.LLMProviderConfig{
				string(types.AgentExplorer): {
					ToolParamCompat: &types.ToolParamCompatConfig{Mode: "relaxed"},
				},
			},
		},
	}

	_, err := resolveToolParamCompatByAgent(cfg)
	if err == nil {
		t.Fatal("expected invalid mode error")
	}
	if !strings.Contains(err.Error(), "llm.agents.explorer.tool_param_compat.mode") {
		t.Fatalf("error should point to the invalid provider key, got: %v", err)
	}
}
