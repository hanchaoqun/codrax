package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1620SemanticReviewerModelNoteBoundary(t *testing.T) {
	ledger := types.CompileObservationLedger(types.ObservationLedgerInput{AggregateFacts: []types.AnswerAggregateFact{{
		Kind: types.AnswerAggregateMemberSet, Label: "functions", Value: "1", Role: types.AnswerAggregateRolePrincipalAnswer,
		Provenance: types.SourceInventoryPrincipalRowSetAggregateProvenance,
		Members:    []string{"Run"}, MemberNotes: []string{"may initialize workers"}, SupportRefs: []string{"Run @ src/a.cj:7"},
	}}})
	got := renderSemanticQualityUserMessage(SemanticQualityInput{Observations: semanticObservationSummaries(ledger, nil, nil)})
	for _, want := range []string{"claim_authority=`independently_proven`", "model_notes/advisory=", "may initialize workers", "src/a.cj:7", "not covered by record claim_authority"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, ` notes=["may initialize workers"`) {
		t.Fatalf("candidate flattened into inherited note: %s", got)
	}
}
