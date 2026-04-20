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
		if k == KindRelationAbsent {
			c.Expr = "foo,bar"
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

// Relation-absent fires when no single evidence row ties the two
// subjects together. Used by multi-subject explain/trace hypotheses
// (e.g. "blob 和 log 的关系") as the falsification criterion.
func TestEval_RelationAbsent_BothInOneEvidence_NotAbsent(t *testing.T) {
	env := Env{
		Evidence: []types.EvidenceItem{
			{ID: "ev1", Subject: "Blob", Summary: "Blob writes into log pipeline"},
		},
	}
	r := Eval(types.Criterion{Kind: string(KindRelationAbsent), Expr: "Blob,log"}, env)
	if r.Satisfied {
		t.Errorf("expected NOT absent when one evidence mentions both; got detail=%q", r.Detail)
	}
}

func TestEval_RelationAbsent_OnlyOneSideSeen_Absent(t *testing.T) {
	env := Env{
		Evidence: []types.EvidenceItem{
			{ID: "ev1", Subject: "Blob", Summary: "Blob writes into a buffer"},
			{ID: "ev2", Subject: "Log", Summary: "Log formats a line"},
		},
	}
	r := Eval(types.Criterion{Kind: string(KindRelationAbsent), Expr: "Blob,Log"}, env)
	if !r.Satisfied {
		t.Errorf("expected absent when evidence mentions each side separately; got detail=%q", r.Detail)
	}
}

func TestEval_RelationAbsent_EmptyEvidence_Absent(t *testing.T) {
	r := Eval(types.Criterion{Kind: string(KindRelationAbsent), Expr: "A,B"}, Env{})
	if !r.Satisfied {
		t.Errorf("empty evidence must count as absent; got detail=%q", r.Detail)
	}
}

func TestEval_RelationAbsent_MalformedExpr(t *testing.T) {
	for _, expr := range []string{"", "A", ",B", "A,"} {
		r := Eval(types.Criterion{Kind: string(KindRelationAbsent), Expr: expr}, Env{})
		if r.Satisfied || r.UnknownKind {
			t.Errorf("expr %q: malformed input must fail loudly (Satisfied=false, UnknownKind=false); got %+v", expr, r)
		}
	}
}

func TestEval_RelationAbsent_CaseInsensitive(t *testing.T) {
	env := Env{
		Evidence: []types.EvidenceItem{
			{ID: "ev1", Object: "BLOB", Summary: "reads from Log store"},
		},
	}
	r := Eval(types.Criterion{Kind: string(KindRelationAbsent), Expr: "blob,log"}, env)
	if r.Satisfied {
		t.Errorf("case-insensitive match should detect BOTH; got detail=%q", r.Detail)
	}
}
