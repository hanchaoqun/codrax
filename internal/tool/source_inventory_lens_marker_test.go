package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPersistSourceInventoryLensExecutionMarkerDropsNavigationRows(t *testing.T) {
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
	if len(stored.Sets) != 0 {
		t.Fatalf("execution marker must not persist broad navigation rows as durable member authority: %+v", stored.Sets)
	}
}
