package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// denialStubOracle answers not-found for every symbol — the
// confirmed-fabrication shape.
type denialStubOracle struct{}

func (denialStubOracle) SymbolExists(string) (bool, int)     { return false, 0 }
func (denialStubOracle) SymbolExistsFlat(string) (bool, int) { return false, 0 }

// F8 ruling (2026-07-03, partial reversal of commit 2e3da86c): the
// three answer-surface hallucination sites (enumeration item label /
// diagram edge endpoint / inline identifier) stopped stamping the
// symbol-shaped TypedDenialAnswerSurfaceSymbolUnverified class.
//
// Rationale, recorded here so the stamp is not re-added by a future
// correctness audit: oracle ABSENCE is a noisy signal (repomap index
// coverage / tier / flat-form heuristics). 2e3da86c keyed an L1 hard
// gate (grep pattern refusal) and L2 prompt redaction on it, which
// violated the precise-signals red line and could deadlock the
// model's own label repair (the denied token gated the very searches
// needed to fix the label). The typed violation still fires
// (warning, soft, caveat-eligible), and the identifiers land on the
// ADVISORY lane feeding the enumeration-label verification
// supplement + telemetry — zero L1/L2 effect. The PRECISE denial
// lane (stampUngroundedEvidenceDenials: final Ungrounded status AND
// dual oracle miss — two precise signals ANDed) is deliberately
// unchanged; see TestStampUngroundedEvidenceDenials below.
func TestHallucinationSitesStampAdvisoryNotSymbolDenial(t *testing.T) {
	t.Run("enumeration item label", func(t *testing.T) {
		doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
			ID: "l1", Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{ID: "i1", Label: "completelyFabricatedIdentifierName"}},
		}}}
		denials := types.NewTypedDenialSet()
		v := validateEnumerationItemLabelHallucination(doc, denialStubOracle{}, denials)
		if len(v) == 0 {
			t.Fatalf("fabricated label must still produce the typed warning violation")
		}
		if denials.IsSymbolDenied("completelyFabricatedIdentifierName") {
			t.Fatalf("oracle miss must NOT stamp a symbol-shaped denial (L1 grep gate) — F8 ruling, reversal of 2e3da86c")
		}
		if denials.Len() != 0 {
			t.Fatalf("denial lane must stay empty, got %+v", denials.Snapshot())
		}
		adv := denials.AdvisoryAnswerSurfaceSymbolTokens()
		if len(adv) != 1 || adv[0] != "completelyFabricatedIdentifierName" {
			t.Fatalf("oracle miss must land on the advisory lane, got %+v", adv)
		}
		if denials.Sanitise("prose naming completelyFabricatedIdentifierName") !=
			"prose naming completelyFabricatedIdentifierName" {
			t.Fatalf("advisory records must not redact prompts (L2)")
		}
		// nil set: violation still fires, no panic.
		if v := validateEnumerationItemLabelHallucination(doc, denialStubOracle{}, nil); len(v) == 0 {
			t.Fatalf("nil denial set must not change the violation behavior")
		}
	})

	t.Run("diagram edge endpoint", func(t *testing.T) {
		doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
			ID: "d1", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramCallDAG,
				Body: "graph TD\n  fabricatedCallerFuncName --> fabricatedCalleeFuncName\n",
			},
		}}}
		denials := types.NewTypedDenialSet()
		v := validateDiagramEdgeEndpointHallucination(doc, denialStubOracle{}, denials)
		if len(v) == 0 {
			t.Fatalf("fabricated call-DAG endpoints must still produce the typed violation")
		}
		if denials.IsSymbolDenied("fabricatedCallerFuncName") || denials.Len() != 0 {
			t.Fatalf("diagram endpoint oracle miss must not stamp a symbol denial — its violation kind is permanently SOFT, a hard denial was an outright soft/hard inconsistency")
		}
		if len(denials.AdvisoryAnswerSurfaceSymbolTokens()) == 0 {
			t.Fatalf("diagram endpoint oracle miss must land on the advisory lane")
		}
	})

	t.Run("inline identifier", func(t *testing.T) {
		doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
			ID: "s1", Kind: types.BlockSection,
			Text: "The flow calls `fabricatedInlineHelperName` before returning.",
		}}}
		denials := types.NewTypedDenialSet()
		v := validateInlineIdentifierHallucination(doc, denialStubOracle{}, denials)
		if len(v) == 0 {
			t.Fatalf("fabricated inline identifier must still produce the typed violation")
		}
		if denials.IsSymbolDenied("fabricatedInlineHelperName") || denials.Len() != 0 {
			t.Fatalf("inline identifier oracle miss must not stamp a symbol denial")
		}
		if len(denials.AdvisoryAnswerSurfaceSymbolTokens()) == 0 {
			t.Fatalf("inline identifier oracle miss must land on the advisory lane")
		}
	})
}

// Finalize-entry evidence stamping: only the final-Ungrounded +
// dual-oracle-miss shape stamps; grounded rows and short/prose
// subjects never do. This is the KEPT precise lane — F8 removed only
// the answer-surface oracle-miss stamps.
func TestStampUngroundedEvidenceDenials(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{
		{ID: "ev-1", AnchorSymbol: "fabricatedEvidenceAnchor", GroundingStatus: types.GroundingUngrounded},
		{ID: "ev-2", AnchorSymbol: "legitGroundedSymbolName", GroundingStatus: types.GroundingGrounded},
		{ID: "ev-3", Subject: "short", GroundingStatus: types.GroundingUngrounded},
	})
	denials := types.NewTypedDenialSet()
	stampUngroundedEvidenceDenials(denials, mut, denialStubOracle{})
	if !denials.IsSymbolDenied("fabricatedEvidenceAnchor") {
		t.Fatalf("final-ungrounded dual-miss subject must be stamped")
	}
	if denials.IsSymbolDenied("legitGroundedSymbolName") {
		t.Fatalf("grounded rows must never stamp")
	}
	if denials.IsSymbolDenied("short") {
		t.Fatalf("sub-floor tokens must never stamp")
	}

	// An oracle that vouches suppresses the stamp even when the row
	// stayed ungrounded (line/quote mismatch on a REAL symbol — the
	// repair-loop protection).
	vouching := vouchingStubOracle{}
	denials2 := types.NewTypedDenialSet()
	stampUngroundedEvidenceDenials(denials2, mut, vouching)
	if denials2.Len() != 0 {
		t.Fatalf("oracle-vouched subjects must never stamp, got %d", denials2.Len())
	}
}

type vouchingStubOracle struct{}

func (vouchingStubOracle) SymbolExists(string) (bool, int)     { return true, 1 }
func (vouchingStubOracle) SymbolExistsFlat(string) (bool, int) { return true, 1 }

// The perf-stall class is dual-shaped: its symbol half now reaches
// the symbol gates, per the long-standing IsSymbolDenied contract.
func TestPerfStallClassSymbolShaped(t *testing.T) {
	set := types.NewTypedDenialSet()
	set.Add(types.TypedDenial{
		Class:  types.TypedDenialExternalPerfStallUnresolved,
		Token:  "stalledRuntimeSymbol",
		Reason: "test",
	})
	if !set.IsSymbolDenied("stalledRuntimeSymbol") {
		t.Fatalf("perf-stall symbol token must be symbol-gate load-bearing")
	}
	set.Add(types.TypedDenial{
		Class:  types.TypedDenialExternalPerfStallUnresolved,
		Token:  "vendor/path/file.cc",
		Reason: "test",
	})
	if _, ok := set.FirstPathDenial("vendor/path/file.cc"); !ok {
		t.Fatalf("perf-stall path token must stay path-gate load-bearing")
	}
}

// F8-T5 registry ratchet: the three enum-label kinds stay
// SOFT-by-default under BOTH the registry and the active runtime
// policy, while Promotable=true keeps the operator strict lane
// (pipeline_contract_strict_kinds) as the typed escape for
// deployments that want enforcement. A flip of any of these to
// hard-by-default would replay the 12-commit softening history of
// this validator surface in reverse.
func TestEnumerationLabelKindsStaySoftByDefault(t *testing.T) {
	kinds := []types.ViolationKind{
		types.ViolEnumerationLabelUngrounded,
		types.ViolEnumerationLabelHallucinated,
		types.ViolEnumerationItemLabelExtractorDrift,
	}
	for _, kind := range kinds {
		spec, ok := types.ViolKindSpecFor(kind)
		if !ok {
			t.Fatalf("kind %q must be registered", kind)
		}
		if !spec.SoftByDefault {
			t.Fatalf("kind %q must stay SoftByDefault=true (oracle/pool mismatch is a noisy signal)", kind)
		}
		if !spec.Promotable {
			t.Fatalf("kind %q must stay Promotable=true (typed escape lane for operator strict promotion)", kind)
		}
		if !isSoftViolationKind(kind) {
			t.Fatalf("kind %q must be soft under the DEFAULT runtime policy", kind)
		}
	}
}

// F8-T2: the enumeration-label verification supplement replaces the
// generic enumeration-depth caveat bullet for the two enum-label
// kinds — one precise user-facing note instead of a vague caveat
// plus a note. Render condition is double-gated on precise signals:
// current-round typed violation kind AND verbatim advisory-token
// presence in the accepted document.
func TestEnumerationLabelSupplementReplacesGenericCaveat(t *testing.T) {
	buildCtx := func(lang string) (*types.BusContext, []types.Violation) {
		doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
			ID: "l1", Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{ID: "i1", Label: "completelyFabricatedIdentifierName — helper"}},
		}}}
		denials := types.NewTypedDenialSet()
		violations := validateEnumerationItemLabelHallucination(doc, denialStubOracle{}, denials)
		if len(violations) == 0 {
			t.Fatalf("fixture must produce the enum-label violation")
		}
		mut := types.NewMutableState("q")
		mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
		return &types.BusContext{Mutable: mut, TypedDenials: denials, Language: lang}, violations
	}

	genericEN, genericZH := "", ""
	for _, fam := range types.AllCaveatFamilies() {
		if fam.ID == types.CaveatFamilyEnumerationDepth {
			genericEN, genericZH = fam.EN, fam.ZH
		}
	}
	if genericEN == "" || genericZH == "" {
		t.Fatalf("enumeration-depth caveat family must exist with both languages")
	}

	t.Run("en", func(t *testing.T) {
		ctx, violations := buildCtx("en")
		out := AppendSoftContractCaveatsToAnswerForBus("answer body", violations, "en", ctx)
		if !strings.Contains(out, "System supplement: enumeration label verification") {
			t.Fatalf("supplement missing: %q", out)
		}
		if !strings.Contains(out, "`completelyFabricatedIdentifierName`") {
			t.Fatalf("supplement must list the unverified identifier: %q", out)
		}
		if strings.Contains(out, genericEN) {
			t.Fatalf("generic enumeration-depth bullet must be suppressed when the specific supplement renders: %q", out)
		}
	})

	t.Run("zh", func(t *testing.T) {
		ctx, violations := buildCtx("zh")
		out := AppendSoftContractCaveatsToAnswerForBus("回答正文", violations, "zh", ctx)
		if !strings.Contains(out, "系统补充：枚举标签核对") {
			t.Fatalf("ZH supplement missing: %q", out)
		}
		if !strings.Contains(out, "`completelyFabricatedIdentifierName`") {
			t.Fatalf("ZH supplement must list the unverified identifier: %q", out)
		}
		if strings.Contains(out, genericZH) {
			t.Fatalf("ZH generic bullet must be suppressed when the specific supplement renders: %q", out)
		}
	})

	t.Run("no advisory tokens keeps generic caveat", func(t *testing.T) {
		doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
			ID: "l1", Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{ID: "i1", Label: "poolMismatchOnlyLabel"}},
		}}}
		mut := types.NewMutableState("q")
		mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
		ctx := &types.BusContext{Mutable: mut, TypedDenials: types.NewTypedDenialSet(), Language: "en"}
		violations := []types.Violation{{Kind: types.ViolEnumerationLabelUngrounded, Detail: "pool mismatch"}}
		out := AppendSoftContractCaveatsToAnswerForBus("answer body", violations, "en", ctx)
		if strings.Contains(out, "System supplement: enumeration label verification") {
			t.Fatalf("supplement must not render without advisory tokens: %q", out)
		}
		if !strings.Contains(out, genericEN) {
			t.Fatalf("generic bullet must survive when no specific supplement renders: %q", out)
		}
	})

	t.Run("repaired label drops stale advisory token", func(t *testing.T) {
		// Stamp came from an earlier round; the accepted doc no longer
		// carries the identifier → no supplement (stale-proof), generic
		// caveat lane resumes for the residual violation.
		denials := types.NewTypedDenialSet()
		denials.AddAnswerSurfaceAdvisory("completelyFabricatedIdentifierName", "earlier round")
		doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
			ID: "l1", Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{ID: "i1", Label: "RealRepairedIdentifier"}},
		}}}
		mut := types.NewMutableState("q")
		mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
		ctx := &types.BusContext{Mutable: mut, TypedDenials: denials, Language: "en"}
		violations := []types.Violation{{Kind: types.ViolEnumerationLabelHallucinated, Detail: "residual"}}
		out := AppendSoftContractCaveatsToAnswerForBus("answer body", violations, "en", ctx)
		if strings.Contains(out, "System supplement: enumeration label verification") {
			t.Fatalf("stale advisory token must not resurface after deterministic repair: %q", out)
		}
	})
}
