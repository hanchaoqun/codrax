package skill_test

import (
	"strings"
	"testing"

	promptcontext "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1591PlacementTeachingPreservesOptionalLocalContext(t *testing.T) {
	for _, tc := range []struct {
		name  string
		agent types.AgentName
		stage types.PipelineStage
	}{{"write-analysis-skill", types.AgentWriteAnalyzer, types.StageWriteAnalyze}, {"change-plan-skill", types.AgentPlanner, types.StagePlan}} {
		t.Run(tc.name, func(t *testing.T) {
			r := skill.NewRegistry()
			skill.RegisterDefaults(r)
			cfg, err := r.Get(tc.name)
			if err != nil {
				t.Fatal(err)
			}
			ac := &types.AgentContext{AgentName: tc.agent, Stage: tc.stage, Objective: "Adjust the selected label."}
			var prompt strings.Builder
			for _, message := range promptcontext.ToMessages(promptcontext.BuildPromptContext(ac, cfg)) {
				prompt.WriteString(message.Content)
			}
			if !strings.Contains(prompt.String(), types.WritePlacementRefsTeaching) {
				t.Fatalf("actual prompt lost shared placement-ref authority teaching: %s", prompt.String())
			}
			for _, forbidden := range []string{
				"A later verification probe must bind placement_refs[]",
				"If a referenced contract carries placement context, the probe should inspect the rendered line/surface and bind placement_refs[]",
			} {
				if strings.Contains(prompt.String(), forbidden) {
					t.Fatalf("prompt still promotes any local context into a required placement proof: %q", forbidden)
				}
			}
		})
	}
}
