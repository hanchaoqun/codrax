package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestFixI_S1aR1ProductionRegression pins the exact failure shape
// that motivated Fix I — s1a r1 batch eval (2026-05-07): finalizer
// LLM emitted the gate.Run check enumeration as markdown numbered
// prose inside a BlockSection.Text field, with each item using
// inline backticks to quote a check name. Five names were partial
// hallucinations:
//
//	`checkAnswerContract`         (real: checkContractComplete)
//	`checkSubtopicConsistency`    (real: checkSubtopicCoherence)
//	`checkShapeSubjectConsistency`(real: checkShapeSubjectCoherence)
//	`checkCriterionResolution`    (real: checkCriterionResolvable)
//	`checkCoherence`              (fully fabricated)
//
// Without Fix I, none of the existing list-block validators saw
// these because the answer was rendered in section prose, not in
// items[].label. Fix I scans block text fields and fires on the
// fabricated names.
func TestFixI_S1aR1ProductionRegression(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "sec1",
				Kind: types.BlockSection,
				Text: "9 项检查按以下顺序执行，其中第 4-7 项在写模式下被跳过：\n\n" +
					"1. `checkCoverage`(第 148 行) — 始终执行。\n" +
					"2. `checkDAGClosure`(第 149 行) — 始终执行。\n" +
					"3. `checkBudgetSanity`(第 150 行) — 始终执行。\n" +
					"4. `checkAnswerContract`(第 156 行) — 仅 !isWrite 时执行。\n" +
					"5. `checkHypothesisCoverage`(第 157 行) — 仅 !isWrite 时执行。\n" +
					"6. `checkSubtopicConsistency`(第 168 行) — 仅 !isWrite 时执行。\n" +
					"7. `checkShapeSubjectConsistency`(第 169 行) — 仅 !isWrite 时执行。\n" +
					"8. `checkCriterionResolution`(第 171 行) — 始终执行。\n" +
					"9. `checkCoherence`(第 172 行) — 始终执行。\n",
			},
		},
	}
	// Oracle confirms the 4 real names; the 5 fabricated names absent.
	oracle := &stubOracleFixB{tiers: map[string]int{
		"checkCoverage":           1,
		"checkDAGClosure":         1,
		"checkBudgetSanity":       1,
		"checkHypothesisCoverage": 1,
	}}
	vs := validateInlineIdentifierHallucination(doc, oracle)
	if len(vs) != 1 {
		t.Fatalf("expected 1 violation listing 5 hallucinations; got %d:\n  %+v", len(vs), vs)
	}
	if vs[0].Kind != types.ViolInlineIdentifierHallucinated {
		t.Errorf("kind = %q, want ViolInlineIdentifierHallucinated", vs[0].Kind)
	}
	if !strings.Contains(vs[0].Detail, "answer-grade evidence surface") ||
		!strings.Contains(vs[0].Repair, "typed support lane") ||
		strings.Contains(vs[0].Repair, "no source file declares") {
		t.Fatalf("repair must steer the finalizer toward validated principal surfaces, got detail=%q repair=%q", vs[0].Detail, vs[0].Repair)
	}
	for _, fake := range []string{"checkAnswerContract", "checkSubtopicConsistency",
		"checkShapeSubjectConsistency", "checkCriterionResolution", "checkCoherence"} {
		if !strings.Contains(vs[0].Detail, fake) {
			t.Errorf("violation must list fabricated %q; got: %s", fake, vs[0].Detail)
		}
	}
	for _, real := range []string{"checkCoverage", "checkDAGClosure",
		"checkBudgetSanity", "checkHypothesisCoverage"} {
		if strings.Contains(vs[0].Detail, real) {
			t.Errorf("violation MUST NOT list real symbol %q; got: %s", real, vs[0].Detail)
		}
	}
}

// TestFixI_AllRealSymbolsPass — no fire when every backtick token
// is a real graph symbol.
func TestFixI_AllRealSymbolsPass(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "sec1",
				Kind: types.BlockSection,
				Text: "Sources include `validateChecker` and `dataPipelineRunner`.",
			},
		},
	}
	oracle := &stubOracleFixB{tiers: map[string]int{
		"validateChecker":    1,
		"dataPipelineRunner": 1,
	}}
	if vs := validateInlineIdentifierHallucination(doc, oracle); len(vs) != 0 {
		t.Errorf("real graph-confirmed inline backticks MUST pass; got %+v", vs)
	}
}

// TestFixI_NilOracleSkips — disable when oracle nil.
func TestFixI_NilOracleSkips(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSection, Text: "`fakeFunctionName` is not a real symbol"},
		},
	}
	if vs := validateInlineIdentifierHallucination(doc, nil); len(vs) != 0 {
		t.Errorf("nil oracle MUST disable validator; got %+v", vs)
	}
}

// TestFixI_ShortIdentSkipsFloor — stdlib helpers like `Sprintf`
// (7 chars) skip the gate.
func TestFixI_ShortIdentSkipsFloor(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "s1",
				Kind: types.BlockSection,
				Text: "Use `Sprintf` and `Println` for formatting; `Get` returns the value.",
			},
		},
	}
	oracle := &stubOracleFixB{tiers: map[string]int{}} // empty
	if vs := validateInlineIdentifierHallucination(doc, oracle); len(vs) != 0 {
		t.Errorf("identifiers <10 chars MUST skip floor; got %+v", vs)
	}
}

// TestFixI_DotQualifiedSkipsViaLeadingIdent — `fmt.Sprintf` extracts
// leading "fmt" (3 chars, below floor) and skips. Standard pattern
// for stdlib references.
func TestFixI_DotQualifiedSkipsViaLeadingIdent(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "s1",
				Kind: types.BlockSection,
				Text: "Use `fmt.Sprintf` for printf-style formatting and `log.Println` for output.",
			},
		},
	}
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	if vs := validateInlineIdentifierHallucination(doc, oracle); len(vs) != 0 {
		t.Errorf("dot-qualified stdlib refs MUST skip via short leading ident; got %+v", vs)
	}
}

func TestFixI_PackageQualifiedInlineBacktickSupportedByEvidenceEndpoint(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "caveat1",
			Kind: types.BlockCaveat,
			Text: "The direct chain includes `normalizer.Normalize` and `counterfactual.Expand`.",
		}},
	}
	mut := types.NewMutableState("call chain")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: []types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/agent/analyzer.go",
			LineStart:       1625,
			AnchorKind:      types.AnchorCall,
			Subject:         "buildAnalysisIR",
			Object:          "normalizer.Normalize",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceConditional,
			Source:          "internal/agent/analyzer.go",
			LineStart:       1850,
			AnchorKind:      types.AnchorCall,
			Subject:         "buildAnalysisIR",
			Object:          "counterfactual.Expand",
			GroundingStatus: types.GroundingGrounded,
		},
	}})
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	if vs := validateInlineIdentifierHallucination(doc, oracle, mut); len(vs) != 0 {
		t.Fatalf("package-qualified inline identifiers backed by evidence endpoints should pass; got %+v", vs)
	}
}

func TestFixI_InlineIdentifierInCitedCurrentSourceFilePasses(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "mm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "mm", "page_alloc.c"),
		[]byte("#define ALLOC_KSWAPD 0x01\nstatic int gfp_to_alloc_flags(void) { return ALLOC_KSWAPD; }\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "section_fast",
			Kind: types.BlockSection,
			Text: "The fast path uses `ALLOC_KSWAPD` while preparing alloc flags.",
		}},
		Citations: []types.Citation{{File: "mm/page_alloc.c", Line: 1}},
	}
	mut := types.NewMutableState("page allocation")
	mut.SetSearchGraph(&repotypes.Graph{
		Root: root,
		FileIndex: map[string]*repotypes.FileInfo{
			"mm/page_alloc.c": &repotypes.FileInfo{RelPath: "mm/page_alloc.c"},
		},
	})
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	if vs := validateInlineIdentifierHallucination(doc, oracle, mut); len(vs) != 0 {
		t.Fatalf("inline identifier present verbatim in a cited current source file should pass; got %+v", vs)
	}
}

func TestFixI_ChangeImpactCurrentRequestProposalSurfacePasses(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "The proposed new name `ShapeScalar` is a request surface, not an existing repo symbol.",
		}},
	}
	mut := types.NewMutableState("rename")
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "如果把 internal/types/analysis_ir.go 里的 ShapeValue 常量改名为 ShapeScalar，还需要改哪些文件？",
		ChangeImpactProfile: &types.ChangeImpactProfile{
			IsChangeImpact:  true,
			Target:          "ShapeValue",
			RequestedOutput: types.ImpactOutputFiles,
		},
	})
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	if vs := validateInlineIdentifierHallucination(doc, oracle, mut); len(vs) != 0 {
		t.Fatalf("user-proposed change-impact surface should not be treated as a fabricated existing symbol; got %+v", vs)
	}
}

func TestFixI_AggregateMemberSetInlineIdentifierPasses(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "The guard stack includes `PreEmitCheck` as a principal mechanism.",
		}},
	}
	mut := types.NewMutableState("principal mechanism lookup")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "codrax hallucination guard mechanisms",
		Value:   "1",
		Members: []string{"PreEmitCheck"},
	}})
	mut.SetInvestigationComplete("accepted aggregate member set")
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	if vs := validateInlineIdentifierHallucination(doc, oracle, mut); len(vs) != 0 {
		t.Fatalf("inline identifiers copied from principal member_set should not be forced through graph oracle; got %+v", vs)
	}
}

func TestFixI_NonChangeImpactCurrentRequestIdentifierStillRequiresEvidence(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "The answer cites `ShapeScalar` as an existing code identifier.",
		}},
	}
	mut := types.NewMutableState("lookup")
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "ShapeScalar 是什么？",
	})
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	vs := validateInlineIdentifierHallucination(doc, oracle, mut)
	if len(vs) != 1 {
		t.Fatalf("non-change-impact request text alone must not vouch for an existing code identifier; got %+v", vs)
	}
}

// TestFixI_TitleAndItemTextScanned — Title field + Items[].Text
// (in addition to Text field) get scanned.
func TestFixI_TitleAndItemTextScanned(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:    "ol1",
				Kind:  types.BlockOrderedList,
				Title: "Implementations of `validateFakeChecker` interface",
				Items: []types.AnswerBlockItem{
					{ID: "i1", Label: "realFunction", Text: "Calls `anotherFakeName` to compute."},
				},
			},
		},
	}
	oracle := &stubOracleFixB{tiers: map[string]int{
		"realFunction": 1,
	}}
	vs := validateInlineIdentifierHallucination(doc, oracle)
	if len(vs) != 1 {
		t.Fatalf("expected 1 violation listing both title + item-text fakes; got %d:\n  %+v", len(vs), vs)
	}
	if !strings.Contains(vs[0].Detail, "validateFakeChecker") {
		t.Errorf("title fake must fire; got: %s", vs[0].Detail)
	}
	if !strings.Contains(vs[0].Detail, "anotherFakeName") {
		t.Errorf("item-text fake must fire; got: %s", vs[0].Detail)
	}
}

// TestFixI_DiagramBlockSkips — Diagram blocks are out of scope
// (handled by Fix D).
func TestFixI_DiagramBlockSkips(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:    "d1",
				Kind:  types.BlockDiagram,
				Title: "`fabricatedDiagramTitle` description",
				Diagram: &types.AnswerDiagramBlock{
					Kind: types.DiagramFlow,
					Body: "graph TD\n  A --> B\n",
				},
			},
		},
	}
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	if vs := validateInlineIdentifierHallucination(doc, oracle); len(vs) != 0 {
		t.Errorf("Diagram blocks MUST skip Fix I (Fix D handles); got %+v", vs)
	}
}

// TestFixI_ScalarBlockSupported — scalar block text scanned.
func TestFixI_ScalarBlockSupported(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "sc1",
				Kind: types.BlockScalar,
				Text: "The function `fabricatedSymbolName` returns the count.",
			},
		},
	}
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	vs := validateInlineIdentifierHallucination(doc, oracle)
	if len(vs) != 1 {
		t.Fatalf("scalar block hallucination MUST fire; got %d:\n  %+v", len(vs), vs)
	}
}

// TestFixI_OracleTier3Rejected — Tier 3+ doesn't vouch (precision
// contract — same as Fix C/D).
func TestFixI_OracleTier3Rejected(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSection, Text: "See `lowConfidenceSymbol`."},
		},
	}
	oracle := &stubOracleFixB{tiers: map[string]int{
		"lowConfidenceSymbol": 3,
	}}
	vs := validateInlineIdentifierHallucination(doc, oracle)
	if len(vs) != 1 {
		t.Fatalf("Tier 3 MUST NOT vouch; got %d:\n  %+v", len(vs), vs)
	}
}

// TestFixI_FlatFormCrossing — Fix E flat-form match works on
// inline backticks too: `pipeline_max_steps` (yaml form) resolves
// graph's `PipelineMaxSteps` (Pascal form), no false fire.
func TestFixI_FlatFormCrossing(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "s1",
				Kind: types.BlockSection,
				Text: "Set `pipeline_max_steps` and `WRITE_ENABLED` in codrax.yaml.",
			},
		},
	}
	oracle := &stubOracleFixB{tiers: map[string]int{
		"PipelineMaxSteps": 1,
		"WriteEnabled":     1,
	}}
	if vs := validateInlineIdentifierHallucination(doc, oracle); len(vs) != 0 {
		t.Errorf("yaml/SCREAMING forms MUST resolve Pascal Go fields via Fix E; got %+v", vs)
	}
}

// TestFixI_MultiBlockPerViolationClustering — each affected block
// produces its own per-block violation (mirrors Fix C clustering).
func TestFixI_MultiBlockPerViolationClustering(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "b1", Kind: types.BlockSection, Text: "`fakeOne` is invoked"},
			{ID: "b2", Kind: types.BlockSection, Text: "`fakeTwoLong` is invoked"},
		},
	}
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	vs := validateInlineIdentifierHallucination(doc, oracle)
	// fakeOne is 7 chars (below floor) → skip; only fakeTwoLong fires
	if len(vs) != 1 {
		t.Fatalf("only b2 should fire (fakeTwoLong ≥10 chars); got %d violations:\n  %+v", len(vs), vs)
	}
	if !strings.Contains(vs[0].Detail, "fakeTwoLong") || strings.Contains(vs[0].Detail, "fakeOne") {
		t.Errorf("only fakeTwoLong should be listed; got: %s", vs[0].Detail)
	}
}

// TestFixI_DedupeAcrossSurfaces — same fabricated identifier
// appearing in title + text + items[].text reports once per block
// (deduped via the `seen` set).
func TestFixI_DedupeAcrossSurfaces(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:    "b1",
				Kind:  types.BlockOrderedList,
				Title: "Methods on `fabricatedClassName`",
				Text:  "All `fabricatedClassName` methods follow this pattern.",
				Items: []types.AnswerBlockItem{
					{ID: "i1", Text: "Field `fabricatedClassName.foo` returns the value."},
				},
			},
		},
	}
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	vs := validateInlineIdentifierHallucination(doc, oracle)
	if len(vs) != 1 {
		t.Fatalf("expected 1 violation per block; got %d:\n  %+v", len(vs), vs)
	}
	if strings.Count(vs[0].Detail, `ident="fabricatedClassName"`) != 1 {
		t.Errorf("hallucinated ident should be deduped per block; got Detail=%s", vs[0].Detail)
	}
}
