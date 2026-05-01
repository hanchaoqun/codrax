package criterion

import (
	"strings"
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

// TestEval_ExternalArtifactDecoded_VacuousWithoutBundle pins the
// "no bundle attached → vacuously satisfied" contract so existing
// read-mode runs without --log / --htrace stay byte-identical.
func TestEval_ExternalArtifactDecoded_VacuousWithoutBundle(t *testing.T) {
	env := Env{DraftAnswer: "any text without bundle reference"}
	r := Eval(types.Criterion{Kind: string(KindExternalArtifactDecoded)}, env)
	if !r.Satisfied {
		t.Fatalf("nil-bundle env must be vacuously satisfied; got %q", r.Detail)
	}
}

// TestEval_ExternalArtifactDecoded_LogBundleHit verifies the
// positive path: a LogBundle with extracted Errors[].Type +
// Frames[].Func tokens, and a draft answer that mentions enough of
// them, satisfies the criterion.
func TestEval_ExternalArtifactDecoded_LogBundleHit(t *testing.T) {
	bundle := &types.LogBundle{
		Meta: types.LogMeta{Signals: []types.LogSignal{types.SignalPanic, types.SignalCrash}},
		Errors: []types.LogError{{
			Type: "SIGSEGV",
			Frames: []types.LogFrame{
				{Func: "buildAnalysisIR", Pkg: "agent"},
				{Func: "ParseOutput", Pkg: "agent"},
			},
		}},
	}
	draft := "Stack shows panic + crash + SIGSEGV in buildAnalysisIR called from ParseOutput in package agent."
	env := Env{DraftAnswer: draft, LogTriage: bundle}
	r := Eval(types.Criterion{Kind: string(KindExternalArtifactDecoded)}, env)
	if !r.Satisfied {
		t.Fatalf("majority of bundle tokens referenced should satisfy; got %q", r.Detail)
	}
}

// TestEval_ExternalArtifactDecoded_LogBundleMiss verifies the
// failure path: a draft that ignores almost every bundle token
// fails the criterion AND the rationale lists the missing tokens
// so the finalizer's retry hint can name them verbatim.
func TestEval_ExternalArtifactDecoded_LogBundleMiss(t *testing.T) {
	bundle := &types.LogBundle{
		Meta: types.LogMeta{Signals: []types.LogSignal{types.SignalPanic}},
		Errors: []types.LogError{{
			Type: "SIGSEGV",
			Frames: []types.LogFrame{
				{Func: "buildAnalysisIR", Pkg: "agent"},
				{Func: "ParseOutput", Pkg: "agent"},
				{Func: "OtherSym", Pkg: "different"},
			},
		}},
	}
	draft := "the file analyzer.go has a problem"
	env := Env{DraftAnswer: draft, LogTriage: bundle}
	r := Eval(types.Criterion{Kind: string(KindExternalArtifactDecoded)}, env)
	if r.Satisfied {
		t.Fatalf("draft missing every bundle token must fail; got %q", r.Detail)
	}
	if !strings.Contains(r.Detail, "SIGSEGV") {
		t.Errorf("rationale must enumerate at least one missing token (SIGSEGV); got %q", r.Detail)
	}
}

// TestEval_ExternalArtifactDecoded_PerfBundleHit covers the
// PerfBundle branch — same logic, different bundle shape (jank
// trigger spans + stall symbols + startup mode).
func TestEval_ExternalArtifactDecoded_PerfBundleHit(t *testing.T) {
	bundle := &types.PerfBundle{
		Meta:  types.PerfMeta{Signals: []string{"jank", "main-thread-stall"}},
		Janks: []types.PerfJank{{TriggerSpan: "RecyclerView.Bind", Reason: "io"}},
		Stalls: []types.PerfStall{
			{Symbol: "DiskCache.read", Kind: "io"},
		},
		Startup: &types.PerfStartup{Mode: "cold"},
	}
	draft := "Cold-start jank: main-thread-stall in RecyclerView.Bind triggered by io waiting for DiskCache.read."
	env := Env{DraftAnswer: draft, PerfTrace: bundle}
	r := Eval(types.Criterion{Kind: string(KindExternalArtifactDecoded)}, env)
	if !r.Satisfied {
		t.Fatalf("majority of perf-bundle tokens referenced should satisfy; got %q", r.Detail)
	}
}

// TestEval_ExternalArtifactDecoded_ExprOverridesFloor pins the
// per-criterion threshold override path: callers can pass
// Expr="0.5" to enforce a custom floor for a specific test
// without mutating package state.
func TestEval_ExternalArtifactDecoded_ExprOverridesFloor(t *testing.T) {
	bundle := &types.LogBundle{
		Errors: []types.LogError{{Type: "SIGSEGV"}},
	}
	rPass := Eval(types.Criterion{Kind: string(KindExternalArtifactDecoded), Expr: "0.5"},
		Env{DraftAnswer: "SIGSEGV crash happened", LogTriage: bundle})
	if !rPass.Satisfied {
		t.Errorf("0.5 floor with 1/1 token referenced must satisfy; got %q", rPass.Detail)
	}
	rFail := Eval(types.Criterion{Kind: string(KindExternalArtifactDecoded), Expr: "0.5"},
		Env{DraftAnswer: "no signal mentioned at all", LogTriage: bundle})
	if rFail.Satisfied {
		t.Errorf("draft missing the only token must fail at 0.5 floor; got %q", rFail.Detail)
	}
}
