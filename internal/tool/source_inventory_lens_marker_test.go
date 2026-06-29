package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPersistSourceInventoryLensExecutionMarkerPreservesCandidateRows(t *testing.T) {
	ctx := &types.BusContext{Mutable: types.NewMutableState("source inventory")}
	obs := types.SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Provenance:   []string{"repo_lens:tool_query", "repo_lens:roles"},
		Lens:         []string{"source_class_universe", "count"},
		SourceClasses: []types.SourceInventorySourceClassCount{{
			Role:     types.SourcePathRoleProduction,
			Count:    3,
			Complete: true,
		}},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRoleFunction,
			Complete: true,
			Count:    2,
			Members: []types.SourceInventoryObservationMember{
				{Name: "Run", File: "src/run.go", Line: 10},
				{Name: "Stop", File: "src/stop.go", Line: 20},
			},
		}},
	}

	persistSourceInventoryLensExecutionMarker(ctx, obs)
	stored := ctx.Mutable.SourceInventoryObservation()
	if !types.SourceInventoryLensExecuted(stored) {
		t.Fatalf("execution marker should satisfy lens-executed authority: %+v", stored)
	}
	if !stored.AdvisoryOnly {
		t.Fatalf("execution marker rows must remain advisory navigation facts: %+v", stored)
	}
	if len(stored.Sets) != 1 || len(stored.Sets[0].Members) != 2 {
		t.Fatalf("execution marker must preserve candidate row-set universe: %+v", stored.Sets)
	}
	if stored.Sets[0].Members[0].Name != "Run" || stored.Sets[0].Members[1].Name != "Stop" {
		t.Fatalf("candidate rows changed while persisting marker: %+v", stored.Sets[0].Members)
	}
}

func TestSourceInventoryLensStageProvenanceUsesPipelineStage(t *testing.T) {
	if got := SourceInventoryLensStageProvenance(&types.BusContext{PipelineStage: types.StageAnalyze}); got != types.SourceInventoryProvenanceStageAnalyze {
		t.Fatalf("analyze stage provenance = %q, want %q", got, types.SourceInventoryProvenanceStageAnalyze)
	}
	if got := SourceInventoryLensStageProvenance(&types.BusContext{PipelineStage: types.StageExplore}); got != types.SourceInventoryProvenanceStageExplore {
		t.Fatalf("explore stage provenance = %q, want %q", got, types.SourceInventoryProvenanceStageExplore)
	}
	if got := SourceInventoryLensStageProvenance(&types.BusContext{}); got != "" {
		t.Fatalf("empty stage provenance = %q, want empty", got)
	}
}
