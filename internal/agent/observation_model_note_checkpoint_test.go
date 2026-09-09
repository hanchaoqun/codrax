package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1620ToolHistoryCheckpointModelNoteBoundary(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind: types.AnswerAggregateMemberSet, Label: "functions", Value: "1", Role: types.AnswerAggregateRolePrincipalAnswer,
		Provenance: types.SourceInventoryPrincipalRowSetAggregateProvenance,
		Members:    []string{"Run"}, MemberNotes: []string{"may initialize workers"}, SupportRefs: []string{"Run @ src/a.cj:7"},
	}})
	mut.RetainInvestigationAggregateFacts()
	got := renderToolHistoryObservationCheckpoint(&types.AgentContext{Stage: types.StageExplore, Mutable: mut}, 8)
	for _, want := range []string{"claim_authority=`independently_proven`", "model_notes/advisory=", "may initialize workers", "src/a.cj:7", "not covered by record claim_authority"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, " note=may initialize workers") {
		t.Fatalf("candidate flattened into inherited note: %s", got)
	}
}
