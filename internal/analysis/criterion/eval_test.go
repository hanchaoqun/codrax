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

func TestEval_EvidenceCount_ObservationOnlyRuntimeUsesArtifactFacts(t *testing.T) {
	logBundle := &types.LogBundle{
		Errors: []types.LogError{{
			Type:    "panic",
			Message: "index out of bounds: index=5, size=3",
			Frames: []types.LogFrame{{
				Func: "Cart.itemAt",
				File: "src/cart/Cart.cj",
				Line: 78,
			}},
		}},
	}
	env := Env{
		IR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				Scenario:  types.ScenarioRootCause,
				LogTriage: logBundle,
				ExternalObservationPolicy: &types.ExternalObservationPolicy{
					CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
					SourceQuotes:      []string{"只分析日志"},
					Confidence:        0.9,
				},
			},
		},
		LogTriage: logBundle,
	}
	r := Eval(types.Criterion{Kind: string(KindEvidenceCount), Expr: ">=3"}, env)
	if !r.Satisfied {
		t.Fatalf("observation-only runtime artifact should waive repo evidence_count, got: %s", r.Detail)
	}
	if !strings.Contains(r.Detail, "external_observations=") ||
		!strings.Contains(r.Detail, "repo evidence_count floor >= 3 waived") {
		t.Fatalf("detail should explain artifact evidence accounting, got: %s", r.Detail)
	}
}

func TestEval_EvidenceCount_MCPObservationOnlyCompletionUsesExternalRows(t *testing.T) {
	env := Env{
		ObservationOnlyCompletion: true,
		MCPResponses: []types.MCPResponse{{
			ServerName:  "fixture",
			Method:      "tools/call",
			ResourceURI: "mcp://fixture/trace/sleep-wakeup",
			Success:     true,
			Observations: []types.MCPTypedObservation{
				{Summary: "sleep", ResourceURI: "mcp://fixture/trace/sleep-wakeup", LineStart: 7},
				{Summary: "wakeup", ResourceURI: "mcp://fixture/trace/sleep-wakeup", LineStart: 12},
			},
		}},
	}
	r := Eval(types.Criterion{Kind: string(KindEvidenceCount), Expr: ">=3"}, env)
	if !r.Satisfied {
		t.Fatalf("MCP-only typed observations should waive repo evidence_count, got: %s", r.Detail)
	}
	if !strings.Contains(r.Detail, "external_observations=2") {
		t.Fatalf("detail should count MCP typed rows, got: %s", r.Detail)
	}
}

func TestEval_EvidenceCount_MCPRowsDoNotWaiveMixedSourceQuestion(t *testing.T) {
	env := Env{
		MCPResponses: []types.MCPResponse{{
			ServerName: "fixture",
			Method:     "tools/call",
			Success:    true,
			Observations: []types.MCPTypedObservation{
				{Summary: "sleep", ResourceURI: "mcp://fixture/trace/sleep-wakeup", LineStart: 7},
			},
		}},
	}
	r := Eval(types.Criterion{Kind: string(KindEvidenceCount), Expr: ">=1"}, env)
	if r.Satisfied {
		t.Fatalf("MCP rows without observation-only completion must not waive current-source evidence_count, got: %s", r.Detail)
	}
}

func TestEval_EvidenceCount_CurrentVerificationDoesNotUseArtifactWaiver(t *testing.T) {
	logBundle := &types.LogBundle{
		Errors: []types.LogError{{Type: "panic"}},
	}
	env := Env{
		IR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentRootCause,
				DiagnosticProfile: types.DiagnosticIntentProfile{
					IsDiagnostic:        true,
					CurrentVersionCheck: true,
				},
				AnalyzerHints: types.AnalyzerHints{
					RequiredFileHints: []types.RequiredFileHint{{Path: "src/cart/Cart.cj"}},
				},
				LogTriage: logBundle,
			},
		},
		LogTriage: logBundle,
	}
	r := Eval(types.Criterion{Kind: string(KindEvidenceCount), Expr: ">=1"}, env)
	if r.Satisfied {
		t.Fatalf("current-code verification should still require repo evidence, got: %s", r.Detail)
	}
	if !strings.Contains(r.Detail, "|evidence|=0 >= 1") {
		t.Fatalf("detail should stay on repo evidence accounting, got: %s", r.Detail)
	}
}

func TestEval_CitationCountGE_ExternalSourceLogWaivesFloor(t *testing.T) {
	env := Env{
		LogTriage: &types.LogBundle{
			Errors: []types.LogError{{Type: "panic"}},
		},
		DraftCitations: 0,
	}
	r := Eval(types.Criterion{Kind: string(KindCitationCountGE), Expr: "2"}, env)
	if !r.Satisfied {
		t.Fatalf("external-source log should waive citation_count_ge, got: %s", r.Detail)
	}
	if strings.Contains(r.Detail, "citation_ref=-1") || !strings.Contains(r.Detail, "external_observation") {
		t.Fatalf("waiver detail should use typed external_observation wording, got: %s", r.Detail)
	}
}

func TestEval_CitationCountGE_CapsToPrincipalMemberSetCardinality(t *testing.T) {
	env := Env{
		IR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
					IsRelationalLookup:    true,
				},
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqEnumeration)},
			},
		},
		AggregateFacts: []types.AnswerAggregateFact{{
			Kind:    types.AnswerAggregateMemberSet,
			Label:   "default subagent names",
			Value:   "1",
			Role:    types.AnswerAggregateRolePrincipalAnswer,
			Members: []string{"explorer"},
		}},
		DraftCitations: 1,
	}
	r := Eval(types.Criterion{Kind: string(KindCitationCountGE), Expr: "3"}, env)
	if !r.Satisfied {
		t.Fatalf("singleton principal member set should cap citation_count_ge, got: %s", r.Detail)
	}
	if !strings.Contains(r.Detail, "principal-member floor 1") {
		t.Fatalf("detail should explain effective floor, got: %s", r.Detail)
	}
}

func TestEval_ContractSatisfied_ExternalSourceTraceWaivesFloor(t *testing.T) {
	env := Env{
		IR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: true, MinCitations: 2},
			},
		},
		PerfTrace: &types.PerfBundle{
			Stalls: []types.PerfStall{{Symbol: "renderFrame", Kind: "main_thread"}},
		},
		DraftAnswer:    "renderFrame stalls on the main thread",
		DraftCitations: 0,
	}
	r := Eval(types.Criterion{Kind: string(KindContractSatisfied)}, env)
	if !r.Satisfied {
		t.Fatalf("external-source trace should waive contract citation floor, got: %s", r.Detail)
	}
	if strings.Contains(r.Detail, "citation_ref=-1") || !strings.Contains(r.Detail, "external_observation") {
		t.Fatalf("waiver detail should use typed external_observation wording, got: %s", r.Detail)
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
	// Phase 6 stage 13 (2026-05-03) fixture update: typed-pair
	// match requires both symbols to appear in typed slots
	// (Subject / Object / AnchorSymbol), NOT in free-form
	// Summary prose. The retired blob substring path falsely
	// matched "log" inside the Summary string; the new path
	// requires the relation to be structurally encoded.
	env := Env{
		Evidence: []types.EvidenceItem{
			{ID: "ev1", Subject: "Blob", Object: "log", Summary: "Blob writes into log pipeline"},
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
	// Phase 6 stage 13 (2026-05-03) fixture update: same reason
	// as TestEval_RelationAbsent_BothInOneEvidence_NotAbsent —
	// both symbols must appear in typed slots. Case-insensitive
	// equality is preserved (BLOB/blob, Log/log).
	env := Env{
		Evidence: []types.EvidenceItem{
			{ID: "ev1", Object: "BLOB", Subject: "Log", Summary: "reads from Log store"},
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

// TestEval_ExternalArtifactDecoded_LogBundleCompatibility verifies
// the post-G63 compatibility behavior: this criterion kind stays
// registered for old success-criteria payloads, but it must not
// inspect the model-authored DraftAnswer text. Typed artifact coverage
// is enforced by the orchestrator's AnswerDocumentV2 carrier check.
func TestEval_ExternalArtifactDecoded_LogBundleCompatibility(t *testing.T) {
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
	env := Env{DraftAnswer: "intentionally unrelated prose", LogTriage: bundle}
	r := Eval(types.Criterion{Kind: string(KindExternalArtifactDecoded)}, env)
	if !r.Satisfied {
		t.Fatalf("compatibility criterion must not fail on DraftAnswer text; got %q", r.Detail)
	}
}

// TestEval_ExternalArtifactDecoded_PerfBundleCompatibility covers the
// perf branch of the same compatibility no-op.
func TestEval_ExternalArtifactDecoded_PerfBundleCompatibility(t *testing.T) {
	bundle := &types.PerfBundle{
		Meta:  types.PerfMeta{Signals: []string{"jank", "main-thread-stall"}},
		Janks: []types.PerfJank{{TriggerSpan: "RecyclerView.Bind", Reason: "io"}},
		Stalls: []types.PerfStall{
			{Symbol: "DiskCache.read", Kind: "io"},
		},
		Startup: &types.PerfStartup{Mode: "cold"},
	}
	env := Env{DraftAnswer: "intentionally unrelated prose", PerfTrace: bundle}
	r := Eval(types.Criterion{Kind: string(KindExternalArtifactDecoded)}, env)
	if !r.Satisfied {
		t.Fatalf("compatibility criterion must not fail on DraftAnswer text; got %q", r.Detail)
	}
}

// TestEval_ExternalArtifactDecoded_ExprIgnoredCompatibly ensures the
// legacy threshold expression cannot revive token-density control.
func TestEval_ExternalArtifactDecoded_ExprIgnoredCompatibly(t *testing.T) {
	bundle := &types.LogBundle{
		Errors: []types.LogError{{Type: "SIGSEGV"}},
	}
	r := Eval(types.Criterion{Kind: string(KindExternalArtifactDecoded), Expr: "1.0"},
		Env{DraftAnswer: "no signal mentioned at all", LogTriage: bundle})
	if !r.Satisfied {
		t.Errorf("legacy Expr floor must be ignored by compatibility evaluator; got %q", r.Detail)
	}
}
