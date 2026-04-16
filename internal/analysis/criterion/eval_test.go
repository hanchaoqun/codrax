package criterion

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Every registered kind must have a live handler — this guards
// against accidentally registering a Kind without adding it to the
// dispatch switch.
func TestRegisteredKindsAllDispatchable(t *testing.T) {
	env := Env{
		Signals: types.ExecutionSignals{HasEnoughFacts: true},
		IR: &types.AnalysisIR{
			HypothesisSet: []types.Hypothesis{{ID: "h1", Status: types.HypConfirmed}},
			EvidencePlan: types.EvidencePlan{
				Budget: types.EvidenceBudget{MaxReactIters: 10},
			},
		},
		ReactItersUsed: 1,
	}
	for _, k := range RegisteredKinds() {
		c := types.Criterion{Kind: string(k)}
		// evidence_count / answer_set_bounded need a comparison expr.
		if k == KindEvidenceCount || k == KindAnswerSetBounded {
			c.Expr = ">=0"
		}
		if k == KindCitationCountGE {
			c.Expr = "0"
		}
		if k == KindSymbolPresent || k == KindNoCallSites ||
			k == KindContainsSymbol || k == KindInvariantBroken {
			c.Expr = "foo"
		}
		if k == KindRegexMatch {
			c.Expr = "."
		}
		if k == KindUserClauseUnresolved {
			c.Expr = "x"
		}
		r := Eval(c, env)
		if r.UnknownKind {
			t.Errorf("kind %q marked UnknownKind by dispatch", k)
		}
	}
}

func TestEval_UnknownKindFlagged(t *testing.T) {
	r := Eval(types.Criterion{Kind: "no_such_kind"}, Env{})
	if !r.UnknownKind {
		t.Fatal("expected UnknownKind=true for unregistered kind")
	}
}

func TestEval_SymbolPresent_MatchesEvidence(t *testing.T) {
	env := Env{
		Evidence: []types.EvidenceItem{{Subject: "FooBar"}},
	}
	r := Eval(types.Criterion{Kind: string(KindSymbolPresent), Expr: "FooBar"}, env)
	if !r.Satisfied {
		t.Errorf("expected satisfied: %s", r.Detail)
	}
}

func TestEval_EvidenceCount_Comparison(t *testing.T) {
	env := Env{Evidence: make([]types.EvidenceItem, 3)}
	cases := []struct {
		expr string
		want bool
	}{
		{">=2", true}, {">2", true}, {"==3", true}, {"<5", true},
		{">=5", false}, {"<2", false},
	}
	for _, c := range cases {
		r := Eval(types.Criterion{Kind: string(KindEvidenceCount), Expr: c.expr}, env)
		if r.Satisfied != c.want {
			t.Errorf("expr=%q: want %v got %v (%s)", c.expr, c.want, r.Satisfied, r.Detail)
		}
	}
}

func TestEval_HasEnoughFacts(t *testing.T) {
	off := Eval(types.Criterion{Kind: string(KindHasEnoughFacts)}, Env{})
	if off.Satisfied {
		t.Error("expected not satisfied when HasEnoughFacts=false")
	}
	on := Eval(types.Criterion{Kind: string(KindHasEnoughFacts)}, Env{Signals: types.ExecutionSignals{HasEnoughFacts: true}})
	if !on.Satisfied {
		t.Error("expected satisfied when HasEnoughFacts=true")
	}
}

func TestEvalAll_ReportsFailures(t *testing.T) {
	cs := []types.Criterion{
		{Kind: string(KindEvidenceCount), Expr: ">=1"},
		{Kind: string(KindEvidenceCount), Expr: ">=5"},
	}
	env := Env{Evidence: make([]types.EvidenceItem, 2)}
	ok, failed := EvalAll(cs, env)
	if ok {
		t.Error("expected not all ok")
	}
	if len(failed) != 1 {
		t.Errorf("expected 1 failure, got %d", len(failed))
	}
}
