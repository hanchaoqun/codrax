package tool

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Source-backed typed fixtures exercise the public tools; this test does not
// claim to run the parser. The independently citable inheritance row retains
// its existing parser-producer requirement in the ordinary evidence gate.
func b1637bIdentityFixture(t *testing.T) (*types.BusContext, *types.AnswerDocumentV2) {
	t.Helper()
	repo, err := filepath.Abs("../../eval/fixtures/cpp-sink-hierarchy")
	if err != nil {
		t.Fatal(err)
	}
	evidence := []types.EvidenceItem{
		{ID: "b1637b-call", Kind: types.EvidenceRelationship, Scope: types.ScopeLine,
			Subject: "make_sink", Object: "SinkRegistry.create", Predicate: "calls",
			Source: "src/registry.cpp", LineStart: 32, LineEnd: 32, AnchorKind: types.AnchorCall,
			AnchorSymbol: "SinkRegistry.create", OwnerSymbol: "make_sink", Snippet: "return SinkRegistry::create(kind, path);",
			GroundingStatus: types.GroundingGrounded, Origin: types.ClaimOriginCurrentRepo},
		{ID: "b1637b-type", Kind: types.EvidenceRelationship, Scope: types.ScopeLine,
			Subject: "ConsoleSink", Object: "Sink", Predicate: "inheritance", Producer: types.EvidenceProducerRepoMapStructuralRelation,
			Source: "include/logx/console_sink.hpp", LineStart: 8, LineEnd: 8, AnchorKind: types.AnchorDefinition,
			AnchorSymbol: "ConsoleSink", Snippet: "class ConsoleSink : public Sink {",
			GroundingStatus: types.GroundingGrounded, Origin: types.ClaimOriginCurrentRepo},
	}
	mut := types.NewMutableState("Describe this call and the separate type relation")
	mut.SetRepoRoot(repo)
	mut.AppendEvidence(evidence)
	bus := &types.BusContext{RepoRoot: repo, WorkDir: t.TempDir(), Mutable: mut, EvidenceItems: evidence,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, PredicateAxis: types.AxisCall,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)}}}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Citations: []types.Citation{
		{File: evidence[0].Source, Line: 32, Quote: evidence[0].Snippet},
		{File: evidence[1].Source, Line: 8, Quote: evidence[1].Snippet},
	}, Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "The factory call and a separate backend inheritance declaration are shown; no bridge is claimed between them."},
		{ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal, Title: "Call and type fact",
			FacetIDs:  []string{string(types.FacetPrincipalPathEdge), string(types.FacetCurrentCodePath)},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge, FacetID: string(types.FacetPrincipalPathEdge)}, {ClaimForm: types.ClaimDefinitionFact}},
			Items: []types.AnswerBlockItem{
				{ID: "call", Label: "make_sink", Text: "Calls SinkRegistry.create.", EvidenceIDs: []string{evidence[0].ID}, CitationRef: 0},
				{ID: "type", Label: "ConsoleSink", Text: "Separately, ConsoleSink inherits Sink.", EvidenceIDs: []string{evidence[1].ID}, CitationRef: 1},
			}, EdgeAnchors: []types.DiagramEdgeAnchor{
				{FromNode: "make_sink", ToNode: "SinkRegistry.create", FromIdentity: "make_sink", ToIdentity: "SinkRegistry.create", RelationKind: types.DiagramRelCall, VisibleLabel: "calls"},
				{FromNode: "ConsoleSink", ToNode: "Sink", FromIdentity: "ConsoleSink", ToIdentity: "Sink", RelationKind: types.DiagramRelTypeRelation, VisibleLabel: "inherits"},
			}},
	}}
	return bus, doc
}

func b1637bIdentityJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func b1637bIdentityExecute(t *testing.T, entry string, bus *types.BusContext, doc *types.AnswerDocumentV2) types.ToolResult {
	t.Helper()
	before := string(b1637bIdentityJSON(t, doc))
	var result types.ToolResult
	var err error
	if entry == "emit" {
		result, err = (&EmitAnswerDocument{}).Execute(bus, b1637bIdentityJSON(t, doc))
	} else {
		result, err = (&EmitAnswerDocumentPatch{}).Execute(bus, b1637bIdentityJSON(t, types.AnswerDocumentV2Patch{
			ReplaceBlocks: []types.AnswerBlock{doc.Blocks[1]}, UnchangedBlockIDs: []string{"summary"},
		}))
	}
	if err != nil {
		t.Fatal(err)
	}
	if string(b1637bIdentityJSON(t, doc)) != before {
		t.Fatal("public tool mutated the caller's model document")
	}
	return result
}

func b1637bIdentityBaseline(t *testing.T, bus *types.BusContext, doc *types.AnswerDocumentV2) {
	t.Helper()
	if mismatches := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, bus.EvidenceItems); len(mismatches) != 0 {
		t.Fatalf("complete source-backed baseline has unrelated evidence failures: %+v", mismatches)
	}
	if result := b1637bIdentityExecute(t, "emit", bus, doc); !result.Success {
		t.Fatalf("complete source-backed baseline rejected: %+v", result)
	}
}

func TestB1637bActualEmitPatchRejectWrongPartialIdentity(t *testing.T) {
	for _, entry := range []string{"emit", "patch"} {
		for _, shape := range []string{"mixed_call", "mixed_type", "call_only"} {
			for _, side := range []string{"from", "to"} {
				t.Run(entry+"/"+shape+"/"+side, func(t *testing.T) {
					bus, doc := b1637bIdentityFixture(t)
					if shape == "call_only" {
						doc.Blocks[1].Items = doc.Blocks[1].Items[:1]
						doc.Blocks[1].ClaimUses = doc.Blocks[1].ClaimUses[:1]
						doc.Blocks[1].EdgeAnchors = doc.Blocks[1].EdgeAnchors[:1]
					}
					if entry == "patch" {
						b1637bIdentityBaseline(t, bus, doc)
					}
					acceptedBefore := string(b1637bIdentityJSON(t, bus.Mutable.AnswerDocumentV2()))
					anchorIndex := 0
					if shape == "mixed_type" {
						anchorIndex = 1
					}
					anchor := &doc.Blocks[1].EdgeAnchors[anchorIndex]
					anchor.FromIdentity, anchor.ToIdentity = "", ""
					if side == "from" {
						anchor.FromIdentity = "UnrelatedIdentity"
					} else {
						anchor.ToIdentity = "UnrelatedIdentity"
					}
					result := b1637bIdentityExecute(t, entry, bus, doc)
					if result.Success {
						t.Fatalf("PUBLIC IDENTITY BYPASS: %s accepted wrong partial %s identity; stored=%s", entry, shape, b1637bIdentityJSON(t, bus.Mutable.AnswerDocumentV2()))
					}
					if result.Repair == nil || !strings.Contains(result.Repair.Metadata[types.ToolRepairMetaDiagramRelationFailureIssues], diagramStandaloneRelationIdentityMissing) {
						t.Fatalf("wrong partial must fail the precise endpoint identity gate, not an unrelated gate: %+v", result)
					}
					if string(b1637bIdentityJSON(t, bus.Mutable.AnswerDocumentV2())) != acceptedBefore {
						t.Fatal("rejected partial identity changed the accepted document")
					}
				})
			}
		}
	}
}

func TestB1637bActualEmitPatchCompleteAndReceiptOmissions(t *testing.T) {
	for _, entry := range []string{"emit", "patch"} {
		for _, shape := range []string{"complete", "both_omitted", "from_omitted", "to_omitted"} {
			t.Run(entry+"/"+shape, func(t *testing.T) {
				bus, doc := b1637bIdentityFixture(t)
				if entry == "patch" {
					b1637bIdentityBaseline(t, bus, doc)
				}
				want := string(b1637bIdentityJSON(t, doc.Blocks))
				if shape != "complete" {
					bus.Mutable.SetFinalizerTypedRelationRecipeAvailable(true)
					bus.Mutable.SetFinalizerTypedRelationRecipeAnchors(doc.Blocks[1].EdgeAnchors)
					for i := range doc.Blocks[1].EdgeAnchors {
						if shape == "both_omitted" || shape == "from_omitted" {
							doc.Blocks[1].EdgeAnchors[i].FromIdentity = ""
						}
						if shape == "both_omitted" || shape == "to_omitted" {
							doc.Blocks[1].EdgeAnchors[i].ToIdentity = ""
						}
					}
				}
				result := b1637bIdentityExecute(t, entry, bus, doc)
				if !result.Success {
					t.Fatalf("complete identity or exact unique receipt omission must pass: %+v", result)
				}
				got := bus.Mutable.AnswerDocumentV2()
				if got == nil || string(b1637bIdentityJSON(t, got.Blocks)) != want {
					t.Fatalf("only omitted hidden identities may be restored; model fields changed: got=%s want=%s", b1637bIdentityJSON(t, got), want)
				}
			})
		}
	}
}

func TestB1637bEveryForwardMappedFormOwnsItsExplicitRelation(t *testing.T) {
	for _, relation := range types.AllDiagramRelationKinds() {
		form := types.ClaimFormForRelation(relation)
		if !form.IsValid() {
			continue
		}
		t.Run(string(relation), func(t *testing.T) {
			_, doc := b1637bIdentityFixture(t)
			block := &doc.Blocks[1]
			block.ClaimUses[1].ClaimForm = form
			block.EdgeAnchors[1].RelationKind = relation
			before := string(b1637bIdentityJSON(t, doc))
			if !answerBlockCarriesStandaloneTypedRelations(*block) {
				t.Fatalf("forward-mapped explicit %s owner cannot be removed by a lossy inverse", form)
			}
			if string(b1637bIdentityJSON(t, doc)) != before {
				t.Fatal("classification changed model fields")
			}
		})
	}
	// A generic definition still cannot infer a type relation. Unmapped forms
	// must not retain unrelated orphan anchors merely because they are valid.
	if types.RelationForClaimForm(types.ClaimDefinitionFact) != types.DiagramRelUnknown {
		t.Fatal("ordinary definitions must not be upgraded to directed type proof")
	}
	_, definition := b1637bIdentityFixture(t)
	definition.Blocks[1].ClaimUses = definition.Blocks[1].ClaimUses[1:]
	definition.Blocks[1].EdgeAnchors = definition.Blocks[1].EdgeAnchors[:1]
	if answerBlockCarriesStandaloneTypedRelations(definition.Blocks[1]) ||
		normalizeOrphanDiagramEdgeAnchors(definition, &types.AnswerSemanticView{Family: types.QFCallChain}) != 1 {
		t.Fatal("plain definition plus stale call metadata must retain the old orphan-removal boundary")
	}
	for _, form := range []types.ClaimForm{types.ClaimAbsenceFact, types.ClaimLiteralValueFact, types.ClaimTextReferenceFact, types.ClaimUnknown, "invalid"} {
		block := types.AnswerBlock{ClaimUses: []types.RenderedClaimUse{{ClaimForm: form}}}
		if len(answerBlockStandaloneRelationClaimForms(block)) != 0 {
			t.Fatalf("non-relation form %s acquired standalone relation authority", form)
		}
	}
}

func TestB1637bActualEmitPatchPreserveTypeProofAndDirectionGates(t *testing.T) {
	for _, entry := range []string{"emit", "patch"} {
		for _, shape := range []string{"wrong_producer", "reverse_type", "missing_type_owner"} {
			t.Run(entry+"/"+shape, func(t *testing.T) {
				bus, doc := b1637bIdentityFixture(t)
				if entry == "patch" {
					b1637bIdentityBaseline(t, bus, doc)
				}
				issue := diagramTypeRelationEdgeIssueNoEvidence
				switch shape {
				case "wrong_producer":
					// Keep the exact citable row/ID/source and change only its
					// parser-owned type-proof prerequisite, not citation validity.
					bus.EvidenceItems[1].Producer = ""
					accepted := bus.Mutable.AnswerDocumentV2()
					bus.Mutable = types.NewMutableState("same citable definition without parser type proof")
					bus.Mutable.SetRepoRoot(bus.RepoRoot)
					bus.Mutable.AppendEvidence(bus.EvidenceItems)
					if accepted != nil {
						bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, accepted)
					}
				case "reverse_type":
					doc.Blocks[1].EdgeAnchors[1].FromIdentity = "Sink"
					doc.Blocks[1].EdgeAnchors[1].ToIdentity = "ConsoleSink"
				case "missing_type_owner":
					doc.Blocks[1].ClaimUses = doc.Blocks[1].ClaimUses[:1]
					issue = diagramStandaloneRelationAnchorHasNoClaim
				}
				before := string(b1637bIdentityJSON(t, bus.Mutable.AnswerDocumentV2()))
				result := b1637bIdentityExecute(t, entry, bus, doc)
				if result.Success || (!strings.Contains(result.Summary, issue) &&
					(result.Repair == nil || !strings.Contains(result.Repair.Metadata[types.ToolRepairMetaDiagramRelationFailureIssues], issue))) {
					t.Fatalf("original precise %s gate did not reject %s: %+v", issue, shape, result)
				}
				if string(b1637bIdentityJSON(t, bus.Mutable.AnswerDocumentV2())) != before {
					t.Fatal("rejected relation changed the accepted model document")
				}
			})
		}
	}
}

func TestB1637bMixedCarrierPreservesOptionalDiagramAliases(t *testing.T) {
	_, doc := b1637bIdentityFixture(t)
	want := string(b1637bIdentityJSON(t, doc.Blocks))
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: "optional", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart TD\n  A[\"make_sink\"] --> B[\"SinkRegistry.create\"]\n  C[\"ConsoleSink\"] --> D[\"Sink\"]\n"}})
	before := string(b1637bIdentityJSON(t, doc))
	if fixed := normalizeDiagramEdgeAnchorMetadata(doc); fixed != 0 {
		t.Fatalf("model-owned standalone labels must not be rewritten as diagram aliases: fixed=%d", fixed)
	}
	effective := diagramEvidenceEffectiveAnchorsForBlock(doc, 2, diagramEvidenceBodyEdgeBlockCounts(doc))
	if len(effective) != 2 || effective[0].FromNode != "A" || effective[0].ToNode != "B" ||
		effective[1].FromNode != "C" || effective[1].ToNode != "D" {
		t.Fatalf("sibling diagram must receive only an ephemeral alias copy: %+v", effective)
	}
	if string(b1637bIdentityJSON(t, doc)) != before {
		t.Fatal("validation aliases changed model text, body or anchors")
	}
	doc.Blocks = doc.Blocks[:2]
	if removed := normalizeOrphanDiagramEdgeAnchors(doc, &types.AnswerSemanticView{Family: types.QFCallChain}); removed != 0 ||
		string(b1637bIdentityJSON(t, doc.Blocks)) != want {
		t.Fatal("removing an optional visual changed the mixed standalone model carrier")
	}
}

func TestB1637bTraceAndNonCarrierIsolation(t *testing.T) {
	for _, mode := range []string{"trace", "supporting", "summary", "no_anchor"} {
		t.Run(mode, func(t *testing.T) {
			bus, doc := b1637bIdentityFixture(t)
			view := &types.AnswerSemanticView{Family: types.QFCallChain}
			switch mode {
			case "trace":
				view.Family = types.QFRootCauseTrace
			case "supporting":
				doc.Blocks[1].SurfaceRole = ""
			case "summary":
				doc.Blocks[1].Kind = types.BlockSummary
			case "no_anchor":
				doc.Blocks[1].EdgeAnchors = nil
			}
			wantBlocks := append([]types.AnswerBlock(nil), doc.Blocks...)
			wantBlocks[1].EdgeAnchors = nil
			normalizeOrphanDiagramEdgeAnchors(doc, view)
			if !reflect.DeepEqual(doc.Blocks, wantBlocks) {
				t.Fatal("existing non-carrier orphan removal or model content changed")
			}
			if got := DiagramCallEdgeEvidenceMismatches(doc, view, bus.EvidenceItems); len(got) != 0 {
				t.Fatalf("source identity gate leaked into %s: %+v", mode, got)
			}
		})
	}
}
