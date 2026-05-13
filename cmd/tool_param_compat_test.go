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
