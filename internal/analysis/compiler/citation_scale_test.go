package compiler

import (
	"strconv"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestScaleCitationCount covers the complexity × subTopics matrix.
// Every assertion is independent so if one axis drifts only the
// relevant row fails.
func TestScaleCitationCount(t *testing.T) {
	cases := []struct {
		name       string
		base       int
		complexity types.Complexity
		subTopics  int
		want       int
	}{
		// Complexity axis on the high-base (architecture_explain / root_cause) template.
		{"simple drops by 1 from high base", 3, types.ComplexitySimple, 1, 2},
		{"moderate drops by 1 from high base (the repomap case)", 3, types.ComplexityModerate, 1, 2},
		{"complex keeps high base", 3, types.ComplexityComplex, 1, 3},

		// Complexity axis on medium-base (config_trace) template.
		{"simple drops from medium base", 2, types.ComplexitySimple, 1, 1},
		{"moderate keeps medium base (no drop below 2)", 2, types.ComplexityModerate, 1, 2},
		{"complex keeps medium base", 2, types.ComplexityComplex, 1, 2},

		// Low-base (generic) template — moderate must not drop to 0.
		{"simple on low base hits floor 1", 1, types.ComplexitySimple, 1, 1},
		{"moderate keeps low base", 1, types.ComplexityModerate, 1, 1},

		// Multi-topic boost.
		{"3 subtopics + simple raises to 3", 3, types.ComplexitySimple, 3, 3},
		{"5 subtopics clamps to 3", 2, types.ComplexityModerate, 5, 3},
		{"multi-topic never lowers below complexity-adjusted", 3, types.ComplexityComplex, 2, 3},

		// Empty complexity (legacy RequestModel) leaves base untouched
		// except for multi-topic boost.
		{"empty complexity single topic passes through", 3, "", 1, 3},
		{"empty complexity + subtopics boosts", 1, "", 3, 3},

		// Hard floor / ceiling.
		{"floor 1 prevents zero gate", 1, types.ComplexitySimple, 1, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := scaleCitationCount(c.base, c.complexity, c.subTopics)
			if got != c.want {
				t.Errorf("scaleCitationCount(base=%d, %s, subTopics=%d) = %d, want %d",
					c.base, c.complexity, c.subTopics, got, c.want)
			}
		})
	}
}

// TestApplyAdaptiveCitationThresholds_RewriteThreePlaces proves the
// pass rewrites all three threshold slots (TaskNode SuccessCriteria,
// AnswerContract.AcceptanceTests, CitationReq.MinCitations) so they
// stay in sync. Previously if the compiler set base=3 in the template
// but only one of the three threshold fields was scaled, runtime
// would compare the answer against inconsistent bars.
func TestApplyAdaptiveCitationThresholds_RewriteThreePlaces(t *testing.T) {
	out := &Output{
		TaskGraph: types.TaskGraph{
			Nodes: []types.TaskNode{
				{ID: "fin", Type: types.NodeFinalize, SuccessCriteria: []types.Criterion{
					{Kind: types.CritCitationCountGE, Expr: "3"},
				}},
			},
		},
		AnswerContract: types.AnswerContract{
			CitationReq: types.CitationReq{MinCitations: 3},
			AcceptanceTests: []types.Criterion{
				{Kind: types.CritCitationCountGE, Expr: "3"},
			},
		},
	}
	rm := types.RequestModel{Complexity: types.ComplexityModerate}
	applyAdaptiveCitationThresholds(out, rm)

	if got := out.TaskGraph.Nodes[0].SuccessCriteria[0].Expr; got != "2" {
		t.Errorf("TaskNode SuccessCriteria not scaled: got %q, want \"2\"", got)
	}
	if got := out.AnswerContract.AcceptanceTests[0].Expr; got != "2" {
		t.Errorf("AnswerContract.AcceptanceTests not scaled: got %q, want \"2\"", got)
	}
	if got := out.AnswerContract.CitationReq.MinCitations; got != 2 {
		t.Errorf("CitationReq.MinCitations not scaled: got %d, want 2", got)
	}
}

// TestApplyAdaptiveCitationThresholds_NoOp_OnLegacyRequestModel pins
// backward compatibility: a RequestModel with empty Complexity and
// zero SubTopics (the shape every existing compile_test.go literal
// produces) must not trigger any rewrite.
func TestApplyAdaptiveCitationThresholds_NoOp_OnLegacyRequestModel(t *testing.T) {
	out := &Output{
		TaskGraph: types.TaskGraph{Nodes: []types.TaskNode{
			{ID: "fin", Type: types.NodeFinalize, SuccessCriteria: []types.Criterion{
				{Kind: types.CritCitationCountGE, Expr: "3"},
			}},
		}},
		AnswerContract: types.AnswerContract{CitationReq: types.CitationReq{MinCitations: 3}},
	}
	applyAdaptiveCitationThresholds(out, types.RequestModel{})
	if out.TaskGraph.Nodes[0].SuccessCriteria[0].Expr != "3" {
		t.Errorf("legacy RM must be no-op on SuccessCriteria")
	}
	if out.AnswerContract.CitationReq.MinCitations != 3 {
		t.Errorf("legacy RM must be no-op on CitationReq.MinCitations")
	}
}

// TestApplyAdaptiveCitationThresholds_NonIntExprLeftAlone guards
// against future additions of non-numeric citation_count_ge exprs
// (e.g. ">= somevar"). The scaler skips those rather than panicking
// on strconv.Atoi failure.
func TestApplyAdaptiveCitationThresholds_NonIntExprLeftAlone(t *testing.T) {
	out := &Output{
		TaskGraph: types.TaskGraph{Nodes: []types.TaskNode{
			{ID: "fin", Type: types.NodeFinalize, SuccessCriteria: []types.Criterion{
				{Kind: types.CritCitationCountGE, Expr: "bogus"},
			}},
		}},
	}
	applyAdaptiveCitationThresholds(out, types.RequestModel{Complexity: types.ComplexityComplex})
	if out.TaskGraph.Nodes[0].SuccessCriteria[0].Expr != "bogus" {
		t.Errorf("non-int expr should be left alone")
	}
}

var _ = strconv.Itoa // keep strconv import in case the test file grows
