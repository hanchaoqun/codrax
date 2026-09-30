package context

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRequestBoundaryContextAcrossReadStages(t *testing.T) {
	for _, stage := range []types.PipelineStage{types.StageAnalyze, types.StageExplore, types.StageExtract, types.StageFinalize, types.StagePlan, types.StageApply, types.StageVerify} {
		bus := &types.BusContext{Mutable: types.NewMutableState("request"), TurnRouteHint: types.TurnRouteHint{RequiredOutcomes: types.TurnOutcomeMeasurement | types.TurnOutcomeSourceExplanation}, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeThreadLookups: []types.RuntimeThreadLookup{{PID: 10, SourceQuote: "线程10所属进程"}}}}}
		ac := BuildAgentContext(bus, types.AgentExplorer, stage)
		pc := BuildPromptContext(ac, &skill.Config{Name: "boundary"})
		found := 0
		for _, section := range pc.SystemSections {
			if section.Title != "Requested results and lookup inputs" {
				continue
			}
			found++
			if !strings.Contains(section.Content, "source_explanation") || !strings.Contains(section.Content, "not diagnostic/root-cause subjects") {
				t.Fatal("boundary not visible")
			}
		}
		if stage.IsWrite() && found != 0 || !stage.IsWrite() && found != 1 {
			t.Fatalf("stage %v sections=%d", stage, found)
		}
	}
	pc := &types.PromptContext{}
	appendRequestBoundaryContext(pc, &types.AgentContext{})
	appendRequestBoundaryContext(pc, nil)
	if len(pc.SystemSections) != 0 {
		t.Fatal("unrelated tasks gained context")
	}
}
