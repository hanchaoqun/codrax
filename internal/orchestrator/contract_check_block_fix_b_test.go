package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/contract"
	"github.com/hanchaoqun/codrax/internal/types"
)

// stubOracleFixB is the in-test types.SymbolOracle. tier=0 means
// "not found"; tier 1-2 = real symbol; tier 3+ = low-confidence
// match treated as not-found by the oracle gate. Mirrors the
// pattern used in internal/analysis/contract/checker_oracle_test.go
// but lives in package orchestrator so the V2 validator tests stay
// self-contained.
type stubOracleFixB struct {
	tiers map[string]int
}

func (s *stubOracleFixB) SymbolExists(name string) (bool, int) {
	if s == nil || s.tiers == nil {
		return false, 0
	}
	if t, ok := s.tiers[name]; ok && t > 0 {
		return true, t
	}
	return false, 0
}

// SymbolExistsFlat (Fix E) flattens both the query and each tier-
// map key, then matches case-insensitively across forms. Mirrors
// the production graphOracle's lazy flat index — built per-call
// (cheap for test scale).
func (s *stubOracleFixB) SymbolExistsFlat(name string) (bool, int) {
	if s == nil || s.tiers == nil {
		return false, 0
	}
	flatQ := contract.FlattenIdentifier(name)
	if flatQ == "" {
		return false, 0
	}
	for k, t := range s.tiers {
		if t <= 0 {
			continue
		}
		if contract.FlattenIdentifier(k) == flatQ {
			return true, t
		}
	}
	return false, 0
}

// TestEnumerationItemLabelExtractorMatch_FixB_S5aR1Reproduction pins
// the exact production scenario that motivated Fix B: extractor's
// emit_answer_symbol slate is incomplete + typo'd, but the finalizer
// answers with the correct verbatim names that the typed graph
// confirms. Without Fix B the validator falsely reports 3 drifted
// items and forces 3 wasted finalize re-dispatches.
func TestEnumerationItemLabelExtractorMatch_FixB_S5aR1Reproduction(t *testing.T) {
	// Extractor slate: 5 correct names, 2 typo'd ("Tiager" vs
	// "Triager"), and 1 missing (subExplorerEvaluator). This is the
	// observed s5a r1 extractor emit shape.
	mut := mutWithSymbols(
		"analyzerEvaluator",
		"answerDocumentEvaluator",
		"explorerEvaluator",
		"extractorEvaluator",
		"logTiagerEvaluator",  // typo: missing 'r'
		"perfTiagerEvaluator", // typo: missing 'r'
		"writeAnalyzerEvaluator",
	)
	// Finalizer answer: all 8 correct names (typed_graph confirms
	// 8 implements relations exist).
	doc := docWithEnumItems("enum_loopcontroller",
		"analyzerEvaluator",
		"answerDocumentEvaluator",
		"explorerEvaluator",
		"extractorEvaluator",
		"logTriagerEvaluator",   // correct spelling
		"perfTriagerEvaluator",  // correct spelling
		"subExplorerEvaluator",  // not in slate at all
		"writeAnalyzerEvaluator",
	)
	// Oracle confirms the 3 names that the slate failed to vouch for.
	oracle := &stubOracleFixB{tiers: map[string]int{
		"logTriagerEvaluator":  1,
		"perfTriagerEvaluator": 1,
		"subExplorerEvaluator": 1,
	}}

	if vs := validateEnumerationItemLabelExtractorMatch(doc, enumView(), mut, oracle); len(vs) != 0 {
		t.Errorf("Fix B should rescue the 3 drifted-but-graph-confirmed labels; "+
			"got %d violation(s):\n  %+v\n"+
			"This is the s5a r1 production regression — without Fix B the "+
			"finalizer would re-dispatch 3 times trying to match a slate that "+
			"is itself incomplete + typo'd.", len(vs), vs)
	}
}

// TestEnumerationItemLabelExtractorMatch_FixB_OracleNoBackingForGenuineDrift
// confirms the oracle gate does NOT excuse genuine abstraction-drift:
// labels like "check 1 (gate.go:148)" that no real symbol backs still
// fire as drift even with the oracle wired.
func TestEnumerationItemLabelExtractorMatch_FixB_OracleNoBackingForGenuineDrift(t *testing.T) {
	mut := mutWithSymbols("checkCoverage", "checkDAGClosure", "checkBudgetSanity",
		"checkContractComplete", "checkHypothesisCoverage", "checkSubtopicCoherence",
		"checkShapeSubjectCoherence", "checkCriterionResolvable", "checkPendingFieldsWellformed")
	doc := docWithEnumItems("list1",
		"check 1 (gate.go:148)", "check 2 (gate.go:149)", "check 3 (gate.go:150)",
		"check 4 (gate.go:156)", "check 5 (gate.go:157)", "check 6 (gate.go:168)",
		"check 7 (gate.go:169)", "check 8 (gate.go:171)", "check 9 (gate.go:172)")
	// Oracle returns false for every "check N (...)" abstraction.
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	vs := validateEnumerationItemLabelExtractorMatch(doc, enumView(), mut, oracle)
	if len(vs) != 1 || vs[0].Kind != types.ViolEnumerationItemLabelExtractorDrift {
		t.Fatalf("genuine drift MUST still fire even with oracle wired; got %+v", vs)
	}
}

// TestEnumerationItemLabelExtractorMatch_FixB_OracleTier3IsRejected
// confirms that low-confidence (Tier 3+) symbol matches do NOT count
// as oracle vouching. The same precision contract as
// contract.shouldOracleGateInclude / checkMustIncludeOracle.
func TestEnumerationItemLabelExtractorMatch_FixB_OracleTier3IsRejected(t *testing.T) {
	mut := mutWithSymbols("realA", "realB", "realC")
	doc := docWithEnumItems("list1", "fakeX", "fakeY", "fakeZ")
	// All three labels exist as Tier 3 (low-confidence parse) symbols.
	oracle := &stubOracleFixB{tiers: map[string]int{
		"fakeX": 3, "fakeY": 3, "fakeZ": 3,
	}}
	vs := validateEnumerationItemLabelExtractorMatch(doc, enumView(), mut, oracle)
	if len(vs) != 1 {
		t.Fatalf("Tier 3 symbols MUST NOT vouch for finalizer labels (only Tier 1-2); got %+v", vs)
	}
}

// TestEnumerationItemLabelExtractorMatch_FixB_FlattenMatch_MultiLanguage
// locks the multi-language equivalence promise of Fix B's stage 3
// (flat-form match). Each row asserts that a finalizer label matches
// the extractor slate even when they're written in different casing
// conventions, mimicking the kinds of token-form crossings that
// happen across the analyzer→extractor→finalizer LLM handoff for
// every supported source language.
func TestEnumerationItemLabelExtractorMatch_FixB_FlattenMatch_MultiLanguage(t *testing.T) {
	cases := []struct {
		lang        string
		slateName   string
		labels      [3]string
		note        string
	}{
		{"go", "subExplorerEvaluator",
			[3]string{"sub_explorer_evaluator", "subExplorerEvaluator", "SubExplorerEvaluator"},
			"camel slate, snake / camel / Pascal labels"},
		{"python", "DataLoader",
			[3]string{"data_loader", "DataLoader", "dataloader"},
			"Pascal slate, snake / Pascal / flat labels"},
		{"rust", "TaskGraph",
			[3]string{"task_graph", "TaskGraph", "task-graph"},
			"Pascal slate, snake / Pascal / kebab labels"},
		{"rust", "MAX_RETRY",
			[3]string{"max_retry", "MAX_RETRY", "MaxRetry"},
			"SCREAMING_SNAKE slate, snake / SCREAMING / Pascal labels"},
		{"swift", "userSession",
			[3]string{"user_session", "userSession", "UserSession"},
			"camel slate, snake / camel / Pascal labels"},
		{"java", "RequestId",
			[3]string{"request_id", "requestId", "RequestId"},
			"Pascal slate, snake / camel / Pascal labels"},
		{"kotlin", "HomeViewModel",
			[3]string{"home_view_model", "homeViewModel", "HomeViewModel"},
			"Pascal slate, snake / camel / Pascal labels"},
		{"ruby", "UserAccount",
			[3]string{"user_account", "userAccount", "UserAccount"},
			"Pascal slate, snake / camel / Pascal labels"},
		{"ts", "requestHandler",
			[3]string{"request-handler", "requestHandler", "RequestHandler"},
			"camel slate, kebab / camel / Pascal labels"},
		{"arkts", "UserProfile",
			[3]string{"user_profile", "userProfile", "UserProfile"},
			"Pascal slate, snake / camel / Pascal labels"},
		{"cangjie", "TaskRunner",
			[3]string{"task_runner", "taskRunner", "TaskRunner"},
			"Pascal slate, snake / camel / Pascal labels"},
	}

	for _, tc := range cases {
		t.Run(tc.lang+"/"+tc.slateName, func(t *testing.T) {
			// Pad slate to 3 entries (validator's noise floor).
			mut := mutWithSymbols(tc.slateName, "padA_unused_padding", "padB_unused_padding")
			doc := docWithEnumItems("list1", tc.labels[0], tc.labels[1], tc.labels[2])
			vs := validateEnumerationItemLabelExtractorMatch(doc, enumView(), mut, nil)
			if len(vs) != 0 {
				t.Errorf("flat-form match MUST resolve %s casing variants for slate %q; got %+v\n  note: %s",
					tc.lang, tc.slateName, vs, tc.note)
			}
		})
	}
}

// TestEnumerationItemLabelExtractorMatch_FixB_FlattenMatch_PreservesProseBoundary
// confirms that flat-form matching does NOT collapse prose
// whitespace — "the sub explorer" (English) is NOT a match for
// slate "subExplorerEvaluator" because the two whitespace-separated
// tokens never form a single identifier-shaped run.
func TestEnumerationItemLabelExtractorMatch_FixB_FlattenMatch_PreservesProseBoundary(t *testing.T) {
	mut := mutWithSymbols("subExplorerEvaluator", "extractorEvaluator", "writeAnalyzerEvaluator")
	doc := docWithEnumItems("list1",
		"the sub explorer", // prose (whitespace breaks the run) — must NOT match
		"the extractor",     // prose
		"a write analyzer",  // prose
	)
	vs := validateEnumerationItemLabelExtractorMatch(doc, enumView(), mut, nil)
	if len(vs) == 0 {
		t.Errorf("prose-with-whitespace labels MUST NOT satisfy via flat-match; got pass")
	}
}

// TestEnumerationItemLabelExtractorMatch_FixB_FlattenMatch_DropsTooShort
// confirms the ≥4 char floor on the flat needle: very short flatten
// outputs ("ab") are too easy to substring-collide and bypass the
// fallback.
func TestEnumerationItemLabelExtractorMatch_FixB_FlattenMatch_DropsTooShort(t *testing.T) {
	// Slate names that flatten to <4 chars should NOT enter the
	// flat lookup, so they cannot rescue arbitrary 3-char labels.
	mut := mutWithSymbols("ab", "cd", "ef") // too short
	doc := docWithEnumItems("list1",
		"unrelated_alpha", "unrelated_beta", "unrelated_gamma")
	vs := validateEnumerationItemLabelExtractorMatch(doc, enumView(), mut, nil)
	if len(vs) == 0 {
		t.Errorf("≤3-char slate names MUST NOT vouch via flat fallback; expected drift fire")
	}
}

// TestEnumerationItemLabelExtractorMatch_FixB_BackwardsCompatNilOracle
// confirms that callers passing nil for the oracle parameter see the
// pre-Fix-B behaviour byte-identically (only stages 1-3 active). This
// matches the legacy semantics that runV2BlockOraclesWithMut keeps.
func TestEnumerationItemLabelExtractorMatch_FixB_BackwardsCompatNilOracle(t *testing.T) {
	mut := mutWithSymbols("checkCoverage", "checkDAGClosure", "checkBudgetSanity",
		"checkContractComplete", "checkHypothesisCoverage", "checkSubtopicCoherence",
		"checkShapeSubjectCoherence", "checkCriterionResolvable", "checkPendingFieldsWellformed")
	// Verbatim labels — passes regardless of oracle.
	doc := docWithEnumItems("list1",
		"checkCoverage", "checkDAGClosure", "checkBudgetSanity",
		"checkContractComplete", "checkHypothesisCoverage", "checkSubtopicCoherence",
		"checkShapeSubjectCoherence", "checkCriterionResolvable", "checkPendingFieldsWellformed")
	if vs := validateEnumerationItemLabelExtractorMatch(doc, enumView(), mut, nil); len(vs) != 0 {
		t.Errorf("nil-oracle path must keep stages 1-3 working; got %+v", vs)
	}
	// Abstract drift — fires regardless of oracle.
	doc2 := docWithEnumItems("list1",
		"check 1 (gate.go:148)", "check 2 (gate.go:149)", "check 3 (gate.go:150)",
		"check 4 (gate.go:156)", "check 5 (gate.go:157)", "check 6 (gate.go:168)",
		"check 7 (gate.go:169)", "check 8 (gate.go:171)", "check 9 (gate.go:172)")
	vs := validateEnumerationItemLabelExtractorMatch(doc2, enumView(), mut, nil)
	if len(vs) != 1 || !strings.Contains(vs[0].Detail, "checkCoverage") {
		t.Errorf("nil-oracle abstract drift must still fire; got %+v", vs)
	}
}
