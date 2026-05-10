// Package tool — pre_emit_enum_label_test.go (2026-05-10).
//
// P1 of the post-sweep optimization: preCheckEnumerationLabelGrounding
// catches LLM-emitted enumeration item labels whose leading
// identifier doesn't ground in the SymbolOracle, returning a
// fix hint that lets the LLM revise within the SAME dispatch
// instead of paying a full repair-loop round downstream.
//
// 70% of post-emit repair-loop violations in the 2026-05-10 sweep
// digest were enum-label hallucinations. Catching them at the
// chokepoint cuts repair cycles dramatically.
package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// stubOracle is a hand-rolled SymbolOracle for unit tests so we
// don't pull repomap (cycle).
type stubOracle struct {
	known map[string]int // name → minTier
}

func (s *stubOracle) SymbolExists(name string) (bool, int) {
	t, ok := s.known[name]
	return ok, t
}
func (s *stubOracle) SymbolExistsFlat(name string) (bool, int) {
	t, ok := s.known[name]
	return ok, t
}

// === preEmitLabelLeadingIdentifier ===

func TestPreEmitLabelLeadingIdentifier_Empty(t *testing.T) {
	if got := preEmitLabelLeadingIdentifier(""); got != "" {
		t.Errorf("empty: expected empty; got %q", got)
	}
}

func TestPreEmitLabelLeadingIdentifier_PureProse(t *testing.T) {
	// Starts with a digit — not identifier-shape.
	if got := preEmitLabelLeadingIdentifier("1. read evidence"); got != "" {
		t.Errorf("digit-leading: expected empty; got %q", got)
	}
}

func TestPreEmitLabelLeadingIdentifier_SimpleIdent(t *testing.T) {
	if got := preEmitLabelLeadingIdentifier("checkCoverage — verifies coverage"); got != "checkCoverage" {
		t.Errorf("simple ident: got %q, want checkCoverage", got)
	}
}

func TestPreEmitLabelLeadingIdentifier_DottedIdent(t *testing.T) {
	if got := preEmitLabelLeadingIdentifier("gate.Run — entry point"); got != "gate.Run" {
		t.Errorf("dotted ident: got %q", got)
	}
}

func TestPreEmitLabelLeadingIdentifier_StripsAfterPunct(t *testing.T) {
	if got := preEmitLabelLeadingIdentifier("buildAnalysisIR(): builds the IR"); got != "buildAnalysisIR" {
		t.Errorf("strips paren: got %q", got)
	}
}

// === preCheckEnumerationLabelGrounding ===

func TestPreCheckEnumLabel_NilOracle_NoOp(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "b1", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
				{Label: "fabricatedFunction — does nothing"},
			}},
		},
	}
	if hints := preCheckEnumerationLabelGrounding(doc, nil); hints != nil {
		t.Errorf("nil oracle: expected no-op; got %v", hints)
	}
}

func TestPreCheckEnumLabel_AllGrounded_NoHints(t *testing.T) {
	oracle := &stubOracle{known: map[string]int{
		"checkCoverage":   1,
		"checkDAGClosure": 1,
	}}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "b1", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
				{Label: "checkCoverage — coverage check"},
				{Label: "checkDAGClosure — closure check"},
			}},
		},
	}
	if hints := preCheckEnumerationLabelGrounding(doc, oracle); hints != nil {
		t.Errorf("all grounded: expected no hints; got %v", hints)
	}
}

func TestPreCheckEnumLabel_AllHallucinated_OneFixHintPerBlock(t *testing.T) {
	oracle := &stubOracle{known: map[string]int{}}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "hops", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
				{Label: "fabricatedFunction1 — fake"},
				{Label: "fabricatedFunction2 — also fake"},
			}},
		},
	}
	hints := preCheckEnumerationLabelGrounding(doc, oracle)
	if len(hints) != 1 {
		t.Fatalf("expected 1 fix hint per block; got %d", len(hints))
	}
	if !strings.Contains(hints[0].Field, "blocks[id=\"hops\"].items[].label") {
		t.Errorf("Field should include block id; got %q", hints[0].Field)
	}
	if !strings.Contains(hints[0].ExpectedShape, "fabricatedFunction1") {
		t.Errorf("ExpectedShape should list hallucinated names; got %q", hints[0].ExpectedShape)
	}
	if !strings.Contains(hints[0].ExpectedShape, "fabricatedFunction2") {
		t.Errorf("ExpectedShape should list both hallucinated names; got %q", hints[0].ExpectedShape)
	}
}

func TestPreCheckEnumLabel_BelowLengthFloor_Skipped(t *testing.T) {
	// short identifier (< 10 chars) → treated as stdlib helper, skipped.
	oracle := &stubOracle{known: map[string]int{}}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "b1", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
				{Label: "Sprintf — formats string"},
				{Label: "New — constructor"},
			}},
		},
	}
	if hints := preCheckEnumerationLabelGrounding(doc, oracle); hints != nil {
		t.Errorf("short identifiers below floor: expected no hints; got %v", hints)
	}
}

func TestPreCheckEnumLabel_ProseLabel_Skipped(t *testing.T) {
	// Prose label like "Step 1: read evidence" doesn't extract an
	// identifier-shape leading token.
	oracle := &stubOracle{known: map[string]int{}}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "b1", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
				{Label: "1. Step prose only"},
			}},
		},
	}
	if hints := preCheckEnumerationLabelGrounding(doc, oracle); hints != nil {
		t.Errorf("prose label: expected no hints; got %v", hints)
	}
}

func TestPreCheckEnumLabel_NonListBlocks_Skipped(t *testing.T) {
	// Diagram / section / scalar blocks have their own oracle paths;
	// the enum-label gate only fires on ordered_list / bullet_list /
	// table.
	oracle := &stubOracle{known: map[string]int{}}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "diag", Kind: types.BlockDiagram, Items: []types.AnswerBlockItem{
				{Label: "fabricatedNodeName"},
			}},
			{ID: "sect", Kind: types.BlockSection, Items: []types.AnswerBlockItem{
				{Label: "fabricatedSection"},
			}},
		},
	}
	if hints := preCheckEnumerationLabelGrounding(doc, oracle); hints != nil {
		t.Errorf("non-list block kinds: expected no hints; got %v", hints)
	}
}

func TestPreCheckEnumLabel_PartialHallucination_OnlyHallucinatedReported(t *testing.T) {
	// Mixed: one real + one fake → only the fake appears in the fix
	// hint's ExpectedShape.
	oracle := &stubOracle{known: map[string]int{
		"checkCoverage": 1,
	}}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "hops", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
				{Label: "checkCoverage — real"},
				{Label: "fabricatedFunction — fake"},
			}},
		},
	}
	hints := preCheckEnumerationLabelGrounding(doc, oracle)
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint; got %d", len(hints))
	}
	if strings.Contains(hints[0].ExpectedShape, "checkCoverage") {
		t.Errorf("real identifier should NOT appear in hint; got %q", hints[0].ExpectedShape)
	}
	if !strings.Contains(hints[0].ExpectedShape, "fabricatedFunction") {
		t.Errorf("hallucinated identifier missing; got %q", hints[0].ExpectedShape)
	}
}

func TestPreCheckEnumLabel_LowConfidenceTier_TreatedAsHallucinated(t *testing.T) {
	// Tier ≥ 3 = low-confidence parse, treated same as not-found.
	oracle := &stubOracle{known: map[string]int{
		"lowConfidenceMatch": 3,
	}}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "b1", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
				{Label: "lowConfidenceMatch — questionable"},
			}},
		},
	}
	hints := preCheckEnumerationLabelGrounding(doc, oracle)
	if len(hints) != 1 {
		t.Errorf("tier ≥ 3 should be flagged as hallucinated; got %d hints", len(hints))
	}
}

func TestPreCheckEnumLabel_DedupHallucinatedIdent(t *testing.T) {
	// Same hallucinated ident appears twice in items — fix hint
	// should list it only once.
	oracle := &stubOracle{known: map[string]int{}}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "b1", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
				{Label: "fabricatedFunction — first"},
				{Label: "fabricatedFunction — duplicate"},
			}},
		},
	}
	hints := preCheckEnumerationLabelGrounding(doc, oracle)
	if len(hints) != 1 {
		t.Fatalf("got %d hints", len(hints))
	}
	occurrences := strings.Count(hints[0].ExpectedShape, "fabricatedFunction")
	if occurrences != 1 {
		t.Errorf("dedup expected 1 occurrence; got %d in %q", occurrences, hints[0].ExpectedShape)
	}
}

// === Integration: runPreEmitChecks fires the new check ===

func TestRunPreEmitChecks_EnumLabelHallucination_FiresAlongsideOtherChecks(t *testing.T) {
	oracle := &stubOracle{known: map[string]int{}}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "hops", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
				{Label: "fabricatedFunctionName — fake"},
			}},
		},
	}
	view := &types.AnswerSemanticView{
		// No required blocks → other checks pass; only enum-label
		// gate should fire.
	}
	hints := runPreEmitChecks(doc, view, oracle)
	if len(hints) == 0 {
		t.Fatal("expected at least one hint from enum-label gate")
	}
	found := false
	for _, h := range hints {
		if strings.Contains(h.Field, "items[].label") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("enum-label hint missing from runPreEmitChecks output; got %v", hints)
	}
}
