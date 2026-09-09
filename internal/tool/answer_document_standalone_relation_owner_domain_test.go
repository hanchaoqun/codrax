package tool

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The r1049 draft already declared both forms on its principal hop list.
// Use the real Java source locations, but select the condition itself as the
// guard endpoint: the separate invalid sequence self-loop is not this bug.
func b1637OwnerFixture(t *testing.T) (*types.BusContext, *types.AnswerDocumentV2) {
	t.Helper()
	repo, err := filepath.Abs("../../eval/fixtures/java-layered-service")
	if err != nil {
		t.Fatal(err)
	}
	const controller = "src/main/java/com/clinic/web/VisitController.java"
	const service = "src/main/java/com/clinic/service/VisitService.java"
	const condition = "repository.countOpenVisits(petId) >= max"
	evidence := []types.EvidenceItem{
		{ID: "b1637-call", Kind: types.EvidenceRelationship, Scope: types.ScopeLine,
			Subject: "VisitController.create", Object: "VisitService.schedule", Predicate: "calls",
			Source: controller, LineStart: 18, LineEnd: 18, AnchorKind: types.AnchorCall,
			AnchorSymbol: "schedule", OwnerSymbol: "VisitController.create",
			Snippet: "return service.schedule(petId, reason);", GroundingStatus: types.GroundingGrounded,
			Origin: types.ClaimOriginCurrentRepo},
		{ID: "b1637-guard", Kind: types.EvidenceConditional, Scope: types.ScopeLine,
			Subject: "VisitService.schedule", OwnerSymbol: "VisitService.schedule", Object: condition,
			Source: service, LineStart: 18, LineEnd: 18, AnchorKind: types.AnchorCondition,
			AnchorSymbol: condition, Condition: condition, Snippet: "if (" + condition + ") {",
			GroundingStatus: types.GroundingGrounded, Origin: types.ClaimOriginCurrentRepo},
	}
	mut := types.NewMutableState("trace the call chain and locate its capacity check")
	mut.SetRepoRoot(repo)
	mut.AppendEvidence(evidence)
	bus := &types.BusContext{
		RepoRoot: repo, WorkDir: t.TempDir(), Mutable: mut, EvidenceItems: evidence,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentTrace, PredicateAxis: types.AxisCall,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
		}},
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2",
		Citations: []types.Citation{
			{File: controller, Line: 18, Quote: evidence[0].Snippet},
			{File: service, Line: 18, Quote: evidence[1].Snippet},
		},
		Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: "The service checks capacity after the controller delegates to it."},
			{ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
				Title: "Model-selected path and condition", Text: "Keep this model-authored explanation unchanged.",
				FacetIDs: []string{string(types.FacetPrincipalPathEdge), string(types.FacetCurrentCodePath)},
				ClaimUses: []types.RenderedClaimUse{
					{ClaimForm: types.ClaimCallEdge, FacetID: string(types.FacetPrincipalPathEdge)},
					{ClaimForm: types.ClaimGuardCondition, FacetID: "branch_guard"},
				},
				Items: []types.AnswerBlockItem{
					{ID: "call", Label: "VisitController.create", Text: "Calls VisitService.schedule.", EvidenceIDs: []string{"b1637-call"}, CitationRef: 0},
					{ID: "guard", Label: "VisitService.schedule", Text: "Checks whether the visit count has reached the maximum.", EvidenceIDs: []string{"b1637-guard"}, CitationRef: 1},
				},
				EdgeAnchors: []types.DiagramEdgeAnchor{
					{FromNode: "controller", ToNode: "service", FromIdentity: "VisitController.create", ToIdentity: "VisitService.schedule", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge, VisibleLabel: "delegates"},
					{FromNode: "service", ToNode: "capacity", FromIdentity: "VisitService.schedule", ToIdentity: condition, RelationKind: types.DiagramRelGuard, ClaimForm: types.ClaimGuardCondition, VisibleLabel: "checks capacity"},
				},
			},
		},
	}
	return bus, doc
}

func TestB1637ActualEmitAcceptsExplicitGuardOwnerOnPrincipalPath(t *testing.T) {
	bus, doc := b1637OwnerFixture(t)
	if mismatches := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, bus.EvidenceItems); len(mismatches) != 0 {
		t.Fatalf("fixture must independently satisfy the existing exact relation evidence gate: %+v", mismatches)
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(doc)
	res, err := (&EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || !res.Success {
		t.Fatalf("actual emit must not deny an explicitly present guard owner: err=%v summary=%s repair=%+v", err, res.Summary, res.Repair)
	}
	after, _ := json.Marshal(doc)
	if string(after) != string(before) {
		t.Fatal("Execute mutated the caller's model document")
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 2 {
		t.Fatalf("accepted model blocks missing: %+v", got)
	}
	for i, want := range doc.Blocks {
		actual := got.Blocks[i]
		// Existing internal citation-provenance stamps are not model fields.
		// Compare the complete published block JSON, including all model text,
		// evidence IDs, anchors, claim forms and source citation indexes.
		wantJSON, _ := json.Marshal(want)
		actualJSON, _ := json.Marshal(actual)
		if string(wantJSON) != string(actualJSON) {
			t.Fatalf("owner correction changed model content/relations: want=%+v got=%+v", want, actual)
		}
	}

	// The patch executor uses the same pre-emit gate; it must not require the
	// model to remove its already owned guard just to edit unrelated prose.
	doc.Blocks[1].Text = "The model revises only this explanation."
	patch, err := json.Marshal(types.AnswerDocumentV2Patch{
		ReplaceBlocks: []types.AnswerBlock{doc.Blocks[1]}, UnchangedBlockIDs: []string{"summary"},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err = (&EmitAnswerDocumentPatch{}).Execute(bus, patch)
	if err != nil || !res.Success {
		t.Fatalf("actual patch lost the same-block guard owner: err=%v result=%+v", err, res)
	}
	got = bus.Mutable.AnswerDocumentV2()
	wantBlock, _ := json.Marshal(doc.Blocks[1])
	actualBlock, _ := json.Marshal(got.Blocks[1])
	if string(wantBlock) != string(actualBlock) {
		t.Fatalf("patch changed unrelated model fields: want=%s got=%s", wantBlock, actualBlock)
	}
}

func TestB1637OwnerDomainIncludesEveryMappedExplicitRelation(t *testing.T) {
	for _, relation := range types.AllDiagramRelationKinds() {
		form := types.ClaimFormForRelation(relation)
		if form == types.ClaimUnknown {
			continue // No ownership form exists; the independent evidence gate still owns this relation.
		}
		t.Run(string(relation), func(t *testing.T) {
			_, doc := b1637OwnerFixture(t)
			block := &doc.Blocks[1]
			block.ClaimUses[1].ClaimForm = form
			block.EdgeAnchors[1].RelationKind = relation
			block.EdgeAnchors[1].ClaimForm = form
			before, _ := json.Marshal(doc)
			view := &types.AnswerSemanticView{Family: types.QFCallChain}
			if hints := preCheckStandaloneCallChainRelationAnchorPresence(doc, view); len(hints) != 0 {
				t.Fatalf("explicit %s owner was lost by the narrower principal trigger set: %+v", form, hints)
			}
			if hints := preCheckStandaloneCallChainRelationAnchorPresence(doc, view); len(hints) != 0 {
				t.Fatalf("second read changed the ownership result: %+v", hints)
			}
			after, _ := json.Marshal(doc)
			if string(before) != string(after) {
				t.Fatal("read-only ownership check changed model bytes")
			}
		})
	}
}

// Keep this helper local to this regression: it inspects typed diagnostics,
// never decides product authority from the error's natural-language text.
func b1637HasIssue(hints []emitFixHint, issue string) bool {
	for _, hint := range hints {
		for _, got := range hint.DiagramRelationFailureIssues {
			if got == issue {
				return true
			}
		}
	}
	return false
}

func TestB1637EveryMappedOwnerRemainsSameBlockAndExplicit(t *testing.T) {
	for _, relation := range types.AllDiagramRelationKinds() {
		form := types.ClaimFormForRelation(relation)
		if form == types.ClaimUnknown {
			continue
		}
		for _, mode := range []string{"missing", "wrong", "other_block"} {
			t.Run(string(relation)+"/"+mode, func(t *testing.T) {
				_, doc := b1637OwnerFixture(t)
				block := &doc.Blocks[1]
				// Keep a distinct, genuinely owned principal trigger so this tests
				// companion ownership even when the companion itself is a call.
				trigger := types.DiagramRelCall
				if form == types.ClaimCallEdge {
					trigger = types.DiagramRelCallback
				}
				block.EdgeAnchors[0].RelationKind = trigger
				block.EdgeAnchors[0].ClaimForm = types.ClaimFormForRelation(trigger)
				block.EdgeAnchors[1].RelationKind = relation
				block.EdgeAnchors[1].ClaimForm = form
				block.ClaimUses = []types.RenderedClaimUse{{ClaimForm: types.ClaimFormForRelation(trigger)}}
				switch mode {
				case "wrong":
					wrong := types.ClaimGuardCondition
					if form == wrong {
						wrong = types.ClaimReturnFact
					}
					block.ClaimUses = append(block.ClaimUses, types.RenderedClaimUse{ClaimForm: wrong})
				case "other_block":
					doc.Blocks[0].ClaimUses = []types.RenderedClaimUse{{ClaimForm: form}}
				}
				before, _ := json.Marshal(doc)
				hints := preCheckStandaloneCallChainRelationAnchorPresence(doc, &types.AnswerSemanticView{Family: types.QFCallChain})
				if len(hints) != 1 || !b1637HasIssue(hints, diagramStandaloneRelationAnchorHasNoClaim) ||
					!strings.Contains(hints[0].ExpectedShape, "["+string(form)+"]") {
					t.Fatalf("%s owner must not be borrowed/inferred: %+v", mode, hints)
				}
				hard, advisory := splitPreEmitHintsByGate(tagPreEmitHints(types.ViolDiagramCallEdgeUnproven, hints))
				if len(hard) != 1 || len(advisory) != 0 {
					t.Fatalf("missing explicit owner was softened: hard=%+v advisory=%+v", hard, advisory)
				}
				after, _ := json.Marshal(doc)
				if string(before) != string(after) {
					t.Fatal("rejecting ownership check modified the model document")
				}
			})
		}
	}
}

func TestB1637PrincipalTriggerAndAnchorObligationsUnchanged(t *testing.T) {
	if got, want := types.CallChainPrincipalClaimForms(), []types.ClaimForm{
		types.ClaimDefinitionFact, types.ClaimCallEdge, types.ClaimCallbackHandoff, types.ClaimRegistrationEdge,
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("companion ownership must not enlarge principal eligibility: %v", got)
	}
	for _, form := range []types.ClaimForm{types.ClaimGuardCondition, types.ClaimReturnFact, types.ClaimDefinitionFact} {
		t.Run(string(form), func(t *testing.T) {
			_, doc := b1637OwnerFixture(t)
			block := &doc.Blocks[1]
			block.ClaimUses = []types.RenderedClaimUse{{ClaimForm: form, FacetID: string(types.FacetPrincipalPathEdge)}}
			block.EdgeAnchors = block.EdgeAnchors[1:]
			block.EdgeAnchors[0].RelationKind = types.RelationForClaimForm(form)
			block.EdgeAnchors[0].ClaimForm = form
			hints := preCheckStandaloneCallChainRelationAnchorPresence(doc, &types.AnswerSemanticView{Family: types.QFCallChain})
			if len(hints) != 1 || !b1637HasIssue(hints, diagramStandalonePrincipalPathMissingOwner) {
				t.Fatalf("%s alone cannot satisfy principal_path_edge: %+v", form, hints)
			}
			block.FacetIDs = nil
			block.ClaimUses[0].FacetID = ""
			if got := preCheckStandaloneCallChainRelationAnchorPresence(doc, &types.AnswerSemanticView{Family: types.QFCallChain}); len(got) != 0 {
				t.Fatalf("non-path companion-only block gained a new obligation: %+v", got)
			}
		})
	}
	for _, form := range []types.ClaimForm{types.ClaimCallEdge, types.ClaimCallbackHandoff, types.ClaimRegistrationEdge} {
		t.Run("missing_anchor/"+string(form), func(t *testing.T) {
			_, doc := b1637OwnerFixture(t)
			doc.Blocks[1].ClaimUses = []types.RenderedClaimUse{{ClaimForm: form}, {ClaimForm: types.ClaimGuardCondition}}
			doc.Blocks[1].EdgeAnchors = nil
			hints := preCheckStandaloneCallChainRelationAnchorPresence(doc, &types.AnswerSemanticView{Family: types.QFCallChain})
			if len(hints) != 1 || !b1637HasIssue(hints, diagramStandaloneRelationClaimHasNoAnchor) {
				t.Fatalf("explicit forms cannot replace actual endpoint anchors: %+v", hints)
			}
		})
	}
}

func TestB1637ActualEmitStillRejectsMissingOwnershipOrProof(t *testing.T) {
	for _, mode := range []string{"missing_owner", "wrong_owner", "other_block_owner", "reverse_guard", "missing_guard_evidence", "missing_anchors", "guard_only_path"} {
		t.Run(mode, func(t *testing.T) {
			bus, doc := b1637OwnerFixture(t)
			block := &doc.Blocks[1]
			issue := diagramStandaloneRelationAnchorHasNoClaim
			switch mode {
			case "missing_owner":
				block.ClaimUses = block.ClaimUses[:1]
			case "wrong_owner":
				block.ClaimUses[1].ClaimForm = types.ClaimReturnFact
			case "other_block_owner":
				doc.Blocks[0].ClaimUses = []types.RenderedClaimUse{block.ClaimUses[1]}
				block.ClaimUses = block.ClaimUses[:1]
			case "reverse_guard":
				anchor := &block.EdgeAnchors[1]
				anchor.FromIdentity, anchor.ToIdentity = anchor.ToIdentity, anchor.FromIdentity
				issue = diagramSemanticRelationIssueNoEvidence
			case "missing_guard_evidence":
				bus.EvidenceItems = bus.EvidenceItems[:1]
				bus.Mutable = types.NewMutableState("same model claims without guard proof")
				bus.Mutable.SetRepoRoot(bus.RepoRoot)
				bus.Mutable.AppendEvidence(bus.EvidenceItems)
				issue = diagramSemanticRelationIssueNoEvidence
			case "missing_anchors":
				block.EdgeAnchors = nil
				issue = diagramStandaloneRelationClaimHasNoAnchor
			case "guard_only_path":
				block.ClaimUses = block.ClaimUses[1:]
				block.EdgeAnchors = block.EdgeAnchors[1:]
				issue = diagramStandalonePrincipalPathMissingOwner
			}
			raw, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			res, err := (&EmitAnswerDocument{}).Execute(bus, raw)
			if err != nil || res.Success || res.Repair == nil ||
				!strings.Contains(res.Repair.Metadata[types.ToolRepairMetaDiagramRelationFailureIssues], issue) {
				t.Fatalf("actual %s must retain its precise rejection %s: err=%v result=%+v", mode, issue, err, res)
			}
			if bus.Mutable.AnswerDocumentV2() != nil {
				t.Fatal("rejected model claims reached the accepted document")
			}
		})
	}
}

func TestB1637OwnerDomainDoesNotReachOtherSurfaces(t *testing.T) {
	for _, mode := range []string{"diagram", "summary", "supporting", "trace", "generic", "nil_view", "nil_document"} {
		t.Run(mode, func(t *testing.T) {
			_, doc := b1637OwnerFixture(t)
			view := &types.AnswerSemanticView{Family: types.QFCallChain}
			doc.Blocks[1].ClaimUses = doc.Blocks[1].ClaimUses[:1] // Deliberately no guard owner.
			switch mode {
			case "diagram":
				doc.Blocks[1].Kind = types.BlockDiagram
				doc.Blocks[1].Diagram = &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n  A->>B: model message\n"}
			case "summary":
				doc.Blocks[1].Kind = types.BlockSummary
			case "supporting":
				doc.Blocks[1].SurfaceRole = ""
			case "trace":
				view.Family = types.QFRootCauseTrace
			case "generic":
				view.Family = types.QFGeneric
			case "nil_view":
				view = nil
			case "nil_document":
				doc = nil
			}
			before, _ := json.Marshal(doc)
			if got := preCheckStandaloneCallChainRelationAnchorPresence(doc, view); len(got) != 0 {
				t.Fatalf("local owner repair expanded into %s: %+v", mode, got)
			}
			after, _ := json.Marshal(doc)
			if string(before) != string(after) {
				t.Fatal("non-applicable model document was modified")
			}
		})
	}
}
