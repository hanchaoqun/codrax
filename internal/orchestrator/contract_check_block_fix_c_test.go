package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestEnumerationItemLabelHallucination_S1aR1Reproduction pins the
// exact production scenario that motivated Fix C: finalizer
// fabricated `checkNamingConvention` and `checkSignalSufficiency`
// for items 8-9 of the gate.Run check enumeration (the real names
// at gate.go:171/172 are `checkCriterionResolvable` and
// `checkPendingFieldsWellformed`). The existing
// validateEnumerationItemLabelGrounding caught only one
// hallucination because evidence pool prose contained "signal"
// substring vouching for c9. The graph oracle catches BOTH because
// neither fabricated name has any Tier 1-2 graph definition.
func TestEnumerationItemLabelHallucination_S1aR1Reproduction(t *testing.T) {
	doc := docWithEnumItems("check_order",
		"checkCoverage",
		"checkDAGClosure",
		"checkBudgetSanity",
		"checkContractComplete",
		"checkHypothesisCoverage",
		"checkSubtopicCoherence",
		"checkShapeSubjectCoherence",
		"checkNamingConvention",  // hallucination — should fire
		"checkSignalSufficiency", // hallucination — should fire
	)
	// Oracle confirms only the 7 real names; the 2 hallucinations
	// return Tier=0 (not found in graph).
	oracle := &stubOracleFixB{tiers: map[string]int{
		"checkCoverage":              1,
		"checkDAGClosure":            1,
		"checkBudgetSanity":          1,
		"checkContractComplete":      1,
		"checkHypothesisCoverage":    1,
		"checkSubtopicCoherence":     1,
		"checkShapeSubjectCoherence": 1,
		// checkNamingConvention + checkSignalSufficiency: absent.
	}}

	vs := validateEnumerationItemLabelHallucination(doc, oracle)
	if len(vs) != 1 {
		t.Fatalf("expected 1 violation listing both hallucinations; got %d:\n  %+v", len(vs), vs)
	}
	if vs[0].Kind != types.ViolEnumerationLabelHallucinated {
		t.Errorf("kind = %q, want ViolEnumerationLabelHallucinated", vs[0].Kind)
	}
	if !strings.Contains(vs[0].Detail, "checkNamingConvention") {
		t.Errorf("detail must list checkNamingConvention; got: %s", vs[0].Detail)
	}
	if !strings.Contains(vs[0].Detail, "checkSignalSufficiency") {
		t.Errorf("detail must list checkSignalSufficiency; got: %s", vs[0].Detail)
	}
	if got, want := vs[0].ClusterKey, `block:check_order|root:block_items_label`; got != want {
		t.Errorf("ClusterKey = %q, want %q", got, want)
	}
}

// TestEnumerationItemLabelHallucination_AllRealSymbolsPass confirms
// the validator does NOT fire when every leading identifier is a
// graph-confirmed Tier 1-2 symbol.
func TestEnumerationItemLabelHallucination_AllRealSymbolsPass(t *testing.T) {
	doc := docWithEnumItems("list1",
		"checkCoverage", "checkDAGClosure", "checkBudgetSanity",
		"checkContractComplete", "checkHypothesisCoverage")
	oracle := &stubOracleFixB{tiers: map[string]int{
		"checkCoverage":           1,
		"checkDAGClosure":         1,
		"checkBudgetSanity":       1,
		"checkContractComplete":   1,
		"checkHypothesisCoverage": 1,
	}}
	if vs := validateEnumerationItemLabelHallucination(doc, oracle); len(vs) != 0 {
		t.Errorf("real graph-confirmed labels MUST pass; got %+v", vs)
	}
}

// TestEnumerationItemLabelHallucination_NilOracleSkips confirms
// that callers passing nil oracle disable the validator (unit-test
// path / no-graph runs).
func TestEnumerationItemLabelHallucination_NilOracleSkips(t *testing.T) {
	doc := docWithEnumItems("list1",
		"checkNamingConvention", "checkSignalSufficiency", "checkBudgetSanity")
	if vs := validateEnumerationItemLabelHallucination(doc, nil); len(vs) != 0 {
		t.Errorf("nil oracle MUST disable validator; got %+v", vs)
	}
}

// TestEnumerationItemLabelHallucination_ShortIdentSkipsFloor
// confirms the length floor: identifiers shorter than 10 chars
// bypass the validator (stdlib helpers like `Sprintf`, `Println`,
// `Get`, `New` are short and often Tier 0 in repomap).
func TestEnumerationItemLabelHallucination_ShortIdentSkipsFloor(t *testing.T) {
	doc := docWithEnumItems("list1",
		"Sprintf", "Println", "Connect")
	// All three are absent from the empty oracle, but all are
	// below the 10-char floor, so none should fire.
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	if vs := validateEnumerationItemLabelHallucination(doc, oracle); len(vs) != 0 {
		t.Errorf("identifiers <10 chars MUST skip the gate; got %+v", vs)
	}
}

// TestEnumerationItemLabelHallucination_ProseLabelSkips confirms
// labels that don't START with an identifier-shape token followed
// by a clean separator are skipped — they're not symbol references.
func TestEnumerationItemLabelHallucination_ProseLabelSkips(t *testing.T) {
	doc := docWithEnumItems("list1",
		"Step 1: read evidence",
		"Step 2: classify subject",
		"Step 3: emit answer")
	// Empty oracle — Step* would be absent. But labels are
	// prose-shaped (leading "Step" continues into digit prose), so
	// labelLeadingSymbolIdentifier returns "" and the gate skips.
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	if vs := validateEnumerationItemLabelHallucination(doc, oracle); len(vs) != 0 {
		t.Errorf("prose-shaped labels MUST skip the gate; got %+v", vs)
	}
}

// TestEnumerationItemLabelHallucination_LabelWithSeparator confirms
// that labels of shape `<identifier> — <prose>` correctly extract
// the leading identifier.
func TestEnumerationItemLabelHallucination_LabelWithSeparator(t *testing.T) {
	doc := docWithEnumItems("list1",
		"checkSignalSufficiency — 信号充分性",  // hallucination + em-dash separator
		"checkBudgetSanity (gate.go:150)", // real + paren separator
		"checkBudgetSanity: 预算合理性",        // real + colon separator
	)
	oracle := &stubOracleFixB{tiers: map[string]int{
		"checkBudgetSanity": 1,
		// checkSignalSufficiency absent
	}}
	vs := validateEnumerationItemLabelHallucination(doc, oracle)
	if len(vs) != 1 {
		t.Fatalf("expected 1 hallucination violation; got %d:\n  %+v", len(vs), vs)
	}
	if !strings.Contains(vs[0].Detail, "checkSignalSufficiency") {
		t.Errorf("detail must list checkSignalSufficiency; got: %s", vs[0].Detail)
	}
}

func TestEnumerationItemLabelHallucination_CitedCrossLanguageMemberSubjectPasses(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{
			{File: "internal/analysis/findings_validator/validator.go", Line: 70},
			{File: "src/render_engine/main.cpp", Line: 42},
			{File: "entry/src/main/ets/pages/Index.ets", Line: 18},
			{File: "src/cangjie_core/main.cj", Line: 9},
		},
		Blocks: []types.AnswerBlock{{
			ID:   "packages",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{
				{ID: "go_pkg", Label: "findings_validator", CitationRef: 0},
				// Deliberately citation-shifted: finalizers sometimes
				// offset citation_ref while the typed evidence endpoint is
				// still authoritative for the member label.
				{ID: "cpp_dir", Label: "render_engine", CitationRef: 0},
				{ID: "arkts_mod", Label: "arkts_ability", CitationRef: 2},
				{ID: "cj_pkg", Label: "cangjie_core", CitationRef: 3},
			},
		}},
	}
	mut := mutWithEvidence([]types.EvidenceItem{
		{Subject: "findings_validator", AnchorSymbol: "Validate", Source: "internal/analysis/findings_validator/validator.go", LineStart: 70},
		{Subject: "render_engine", AnchorSymbol: "Render", Source: "src/render_engine/main.cpp", LineStart: 42},
		{Subject: "arkts_ability", AnchorSymbol: "onPageShow", Source: "entry/src/main/ets/pages/Index.ets", LineStart: 18},
		{Subject: "cangjie_core", AnchorSymbol: "main", Source: "src/cangjie_core/main.cj", LineStart: 9},
	})
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	if vs := validateEnumerationItemLabelHallucination(doc, oracle, mut); len(vs) != 0 {
		t.Fatalf("cited package/module/directory subjects are legitimate cross-language member labels, got %+v", vs)
	}
}

func TestEnumerationItemLabelHallucination_AnswerSymbolCrossLanguageLabelsPass(t *testing.T) {
	doc := docWithEnumItems("arkts", "ParentComponent", "defaultHeader")
	mut := &types.MutableState{}
	mut.SetEmittedAnswerSymbols([]types.AnswerSymbol{
		{Name: "ParentComponent", File: "internal/thirdparty/tree-sitter-arkts/corpus/sources/03_state_management.ets", Line: 34},
		{Name: "defaultHeader", File: "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets", Line: 8},
	}, types.CompletenessComplete)
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	if vs := validateEnumerationItemLabelHallucination(doc, oracle, mut); len(vs) != 0 {
		t.Fatalf("file:line-backed AnswerSymbols must bypass symbol-oracle hallucination checks for non-Go languages; got %+v", vs)
	}
}

// TestEnumerationItemLabelHallucination_OracleTier3Rejected
// confirms that low-confidence (Tier 3+) symbol matches do NOT
// vouch — same precision contract as contract.must_include.
func TestEnumerationItemLabelHallucination_OracleTier3Rejected(t *testing.T) {
	doc := docWithEnumItems("list1",
		"checkSignalSufficiency", // length=22; Tier 3 in oracle
		"realFunctionName",       // length=16; Tier 1
		"anotherRealOne")         // length=14; Tier 1
	oracle := &stubOracleFixB{tiers: map[string]int{
		"checkSignalSufficiency": 3, // low-confidence parse
		"realFunctionName":       1,
		"anotherRealOne":         1,
	}}
	vs := validateEnumerationItemLabelHallucination(doc, oracle)
	if len(vs) != 1 {
		t.Fatalf("Tier 3 MUST NOT vouch; got %d:\n  %+v", len(vs), vs)
	}
	if !strings.Contains(vs[0].Detail, "checkSignalSufficiency") {
		t.Errorf("detail must list the Tier-3 hallucination; got: %s", vs[0].Detail)
	}
}

// TestEnumerationItemLabelHallucination_NonListBlocksSkip
// confirms only ordered_list / bullet_list / table block kinds are
// scanned. Scalar / decision / summary / diagram blocks have their
// own oracle paths.
func TestEnumerationItemLabelHallucination_NonListBlocksSkip(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID: "summary1", Kind: types.BlockSummary,
				Items: []types.AnswerBlockItem{
					{ID: "i1", Label: "checkSignalSufficiency"},
				},
			},
			{
				ID: "scalar1", Kind: types.BlockScalar,
				Items: []types.AnswerBlockItem{
					{ID: "i1", Label: "checkNamingConvention"},
				},
			},
		},
	}
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	if vs := validateEnumerationItemLabelHallucination(doc, oracle); len(vs) != 0 {
		t.Errorf("non-list blocks MUST skip the gate; got %+v", vs)
	}
}

// TestEnumerationItemLabelHallucination_TableBlockScanned confirms
// table blocks ARE included (they too carry "list of named
// symbols" semantics).
func TestEnumerationItemLabelHallucination_TableBlockScanned(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID: "table1", Kind: types.BlockTable,
				Items: []types.AnswerBlockItem{
					{ID: "i1", Label: "checkSignalSufficiency"},
					{ID: "i2", Label: "checkNamingConvention"},
				},
			},
		},
	}
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	vs := validateEnumerationItemLabelHallucination(doc, oracle)
	if len(vs) != 1 {
		t.Fatalf("table block MUST be scanned; got %d violations: %+v", len(vs), vs)
	}
	if !strings.Contains(vs[0].Detail, "checkSignalSufficiency") || !strings.Contains(vs[0].Detail, "checkNamingConvention") {
		t.Errorf("both hallucinations should appear; got: %s", vs[0].Detail)
	}
}

// TestEnumerationItemLabelHallucination_MultiBlockSplitsViolations
// confirms each affected block produces its own violation
// (matches the per-block clustering pattern of sister validators).
func TestEnumerationItemLabelHallucination_MultiBlockSplitsViolations(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID: "list1", Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{
					{ID: "i1", Label: "checkNamingConvention"},
				},
			},
			{
				ID: "list2", Kind: types.BlockBulletList,
				Items: []types.AnswerBlockItem{
					{ID: "j1", Label: "checkSignalSufficiency"},
				},
			},
		},
	}
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	vs := validateEnumerationItemLabelHallucination(doc, oracle)
	if len(vs) != 2 {
		t.Fatalf("expected one violation per block; got %d (%+v)", len(vs), vs)
	}
	if vs[0].ClusterKey == vs[1].ClusterKey {
		t.Errorf("each block should have a distinct ClusterKey; got %q == %q", vs[0].ClusterKey, vs[1].ClusterKey)
	}
}

// TestLabelLeadingSymbolIdentifier_TableInvariants pins the
// helper's behaviour across the shape variants the validator
// depends on.
func TestLabelLeadingSymbolIdentifier_TableInvariants(t *testing.T) {
	cases := []struct {
		in   string
		want string
		note string
	}{
		{"checkCoverage", "checkCoverage", "bare identifier"},
		{"  checkCoverage  ", "checkCoverage", "trims surrounding whitespace"},
		{"checkCoverage — 资源检查", "checkCoverage", "em-dash separator"},
		{"checkCoverage (note)", "checkCoverage", "paren separator"},
		{"checkCoverage: desc", "checkCoverage", "colon separator"},
		{"checkCoverage. text", "checkCoverage", "dot separator"},
		{"Step 1: read evidence", "", "ident continues into digit prose"},
		{"the checkCoverage helper", "", "prose: 'the' continues into another identifier — skip"},
		{"", "", "empty"},
		{"123abc", "123abc", "leading digits OK (matches ident shape)"},
		{"_private_var", "_private_var", "underscore-leading identifier"},
		{"snake_case_name", "snake_case_name", "snake_case identifier"},
		{"PascalCase", "PascalCase", "Pascal identifier"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := labelLeadingSymbolIdentifier(tc.in)
			if got != tc.want {
				t.Errorf("labelLeadingSymbolIdentifier(%q) = %q, want %q\n  note: %s", tc.in, got, tc.want, tc.note)
			}
		})
	}
}
