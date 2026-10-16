package agent

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC249AnalyzerActualSourceDomainSurface(t *testing.T) {
	registry := tool.NewRegistry()
	tool.RegisterDefaults(registry)
	for _, source := range []string{"artifact", "repo", "mixed"} {
		t.Run(source, func(t *testing.T) {
			ctx := &types.AgentContext{Objective: "比较采集中的调用位置和检查进度", Stage: types.StageAnalyze, AgentName: types.AgentAnalyzer,
				Mutable: types.NewMutableState("source domain"), TurnRouteHint: types.TurnRouteHint{Route: "repo", Source: source, NeedsRepoAccess: true}}
			sk := skill.BuildAnalysisSkill()
			capture := &traceTeachingCaptureLLM{stop: errors.New("captured analyzer source domain")}
			_, executeErr := NewAnalyzerAgent(&Dependencies{Tools: registry, LLM: capture, MaxIterations: 1}).Execute(ctx, sk)
			if !errors.Is(executeErr, capture.stop) || capture.calls != 1 {
				t.Fatalf("actual analyzer request not captured: %v calls=%d", executeErr, capture.calls)
			}
			var raw json.RawMessage
			for _, s := range capture.tools {
				if s.Name == "emit_analysis" {
					raw = s.Parameters
				}
			}
			if len(raw) == 0 {
				t.Fatal("actual analyzer surface lacks emit_analysis")
			}
			var schema map[string]any
			if err := json.Unmarshal(raw, &schema); err != nil {
				t.Fatal(err)
			}
			props := schema["properties"].(map[string]any)
			dims := props["requested_answer_dimensions"].(map[string]any)["properties"].(map[string]any)["dimensions"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
			role := dims["role"].(map[string]any)["description"].(string)
			for _, want := range []string{"current checkout's implementation", "recorded callsites", "inspection status"} {
				if !strings.Contains(role, want) {
					t.Errorf("actual role schema missing %q", want)
				}
			}
			var prompt strings.Builder
			for _, m := range capture.messages {
				prompt.WriteString(m.Content)
			}
			for _, want := range []string{"recorded callsites", "inspection status", "available, not automatically required", "explicitly forbids current checkout/source evidence"} {
				if !strings.Contains(prompt.String(), want) {
					t.Errorf("actual analyzer prompt missing %q", want)
				}
			}
			if strings.Contains(prompt.String(), "External observations default to mixed external + current-source analysis") {
				t.Fatal("actual prompt still auto-promotes artifact to mixed source")
			}
			if strings.Count(role, skill.AnalysisObservedDimensionTeaching) != 1 {
				t.Fatal("observation semantics duplicated or lost")
			}
			if strings.Count(role, skill.AnalysisMultiSurfaceDimensionTeaching) != 1 || strings.Count(prompt.String(), skill.AnalysisMultiSurfaceDimensionTeaching) != 1 {
				t.Fatal("actual prompt/schema must each deliver the shared dimension meaning once")
			}
			for _, surface := range []string{role, prompt.String()} {
				for _, want := range []string{"information requested", "output carrier", "delivery action", "not a visual surface"} {
					if !strings.Contains(surface, want) {
						t.Errorf("actual analyzer dimension surface missing %q", want)
					}
				}
			}
		})
	}
}
