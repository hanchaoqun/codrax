package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderExplorerWriteExplorationRequest_ReadOnlyHandoffBoundary(t *testing.T) {
	mu := types.NewMutableState("fix duration boundary")
	mu.SetWriteExplorationRequest(&types.WriteExplorationRequest{
		BatchID:              "batch-1",
		Goal:                 "inspect Duration milliseconds bounds",
		ExplorationQuestions: []string{"where is the constructor boundary checked?"},
		CandidatePaths:       []string{"src/duration.rs", "tests/duration_min.rs"},
		EvidenceRequirements: []string{"source line for constructor and boundary constants"},
		MaxRounds:            2,
	})

	got := renderExplorerWriteExplorationRequest(&types.AgentContext{Mutable: mu})
	gotLower := strings.ToLower(got)
	for _, want := range []string{
		"read-only source exploration",
		"do not edit files",
		"do not call shell commands",
		"do not try to implement the fix in this stage",
		"record it as evidence or in the completion reason",
		"the controller will hand it to the planner",
		"src/duration.rs",
		"max_rounds: 2",
	} {
		if !strings.Contains(gotLower, strings.ToLower(want)) {
			t.Fatalf("write exploration prompt missing %q; got:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"if the user says",
		"parse the user's wording",
		"inspect model rationale",
	} {
		if strings.Contains(gotLower, forbidden) {
			t.Fatalf("write exploration prompt contains hard-routing prose smell %q; got:\n%s", forbidden, got)
		}
	}
}
