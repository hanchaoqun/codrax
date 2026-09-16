package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The public emit/patch boundary already permits a grounded same-caller
// support edge in the principal diagram. That permission must not enlarge the
// exact endpoint-boundary principal_path_edge list or manufacture evidence.
func b1708EndpointSequenceFixture(t *testing.T) (*types.BusContext, *types.AnswerDocumentV2) {
	t.Helper()
	repo := t.TempDir()
	const source = "package example\nfunc Entry() {\n Prepare()\n Shared()\n}\nfunc Wrapper() { Shared() }\nfunc Prepare() {}\nfunc Shared() {}\n"
	if err := os.WriteFile(filepath.Join(repo, "entry.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	call := func(id, from, to string, line int) types.EvidenceItem {
		return types.EvidenceItem{ID: id, Kind: types.EvidenceRelationship, Scope: types.ScopeLine,
			Subject: from, OwnerSymbol: from, Predicate: "calls", Object: to, AnchorKind: types.AnchorCall,
			AnchorSymbol: to, Source: "entry.go", LineStart: line, LineEnd: line,
			GroundingStatus: types.GroundingGrounded, Origin: types.ClaimOriginCurrentRepo}
	}
	evidence := []types.EvidenceItem{
		call("prepare", "Entry", "Prepare", 3),
		call("source-boundary", "Entry", "Shared", 4),
		call("sink-boundary", "Wrapper", "Shared", 6),
	}
	mu := types.NewMutableState("Show the grounded source call sites and exact requested endpoint boundary")
	mu.SetRepoRoot(repo)
	mu.AppendEvidence(evidence)
	mu.SetPrincipalSpanWaiver(&types.PrincipalSpanWaiver{Reason: types.PrincipalSpanWaiverNoDirectedPath,
		Rationale: "Both entry points call Shared without a directed path between the requested endpoints."})
	bus := &types.BusContext{RepoRoot: repo, WorkDir: t.TempDir(), Mutable: mu, EvidenceItems: evidence,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentTrace, PredicateAxis: types.AxisCall,
			DiagramHint:              &types.DiagramHint{Kind: types.DiagramSequence, Required: true},
			CallChainEndpointProfile: &types.CallChainEndpointProfile{Source: "Entry", Sink: "Wrapper"},
			AnalyzerHints:            types.AnalyzerHints{Kind: string(types.ReqCallChain), ExactTargets: []string{"Entry", "Wrapper"}},
		}}}
	anchor := func(from, to, fromID, toID string) types.DiagramEdgeAnchor {
		label := map[string]string{"E/P": "prepare the input", "E/S": "submit prepared input", "W/S": "forward the independent request"}[from+"/"+to]
		return types.DiagramEdgeAnchor{FromNode: from, ToNode: to, FromIdentity: fromID, ToIdentity: toID,
			RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge, VisibleLabel: label}
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "The two entry points independently call the shared operation; no path from Entry to Wrapper is proven."},
		{ID: "diagram", Kind: types.BlockDiagram, SurfaceRole: types.SurfacePrincipal,
			FacetIDs: []string{string(types.FacetDiagramSpine)},
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid",
				Body: "sequenceDiagram\n  participant E as Entry\n  participant P as Prepare\n  participant S as Shared\n  participant W as Wrapper\n  E->>P: prepare the input\n  E->>S: submit prepared input\n  W->>S: forward the independent request"},
			EdgeAnchors: []types.DiagramEdgeAnchor{anchor("E", "P", "Entry", "Prepare"), anchor("E", "S", "Entry", "Shared"), anchor("W", "S", "Wrapper", "Shared")}},
		{ID: "boundary", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
			FacetIDs: []string{string(types.FacetPrincipalPathEdge), string(types.FacetCurrentCodePath)},
			ClaimUses: []types.RenderedClaimUse{
				{ClaimForm: types.ClaimCallEdge, FacetID: string(types.FacetPrincipalPathEdge), EvidenceID: "source-boundary"},
				{ClaimForm: types.ClaimCallEdge, FacetID: string(types.FacetPrincipalPathEdge), EvidenceID: "sink-boundary"},
			},
			Items: []types.AnswerBlockItem{
				{ID: "source", Label: "Entry", Text: "Calls the shared operation.", EvidenceIDs: []string{"source-boundary"}, CitationRef: -1},
				{ID: "sink", Label: "Wrapper", Text: "Independently calls the same operation.", EvidenceIDs: []string{"sink-boundary"}, CitationRef: -1},
			},
			EdgeAnchors: []types.DiagramEdgeAnchor{anchor("E", "S", "Entry", "Shared"), anchor("W", "S", "Wrapper", "Shared")}},
	}}
	// Exercise the exact recipe-receipt gate used after these candidates have
	// been published, not just the general source-evidence gate.
	mu.SetFinalizerTypedRelationRecipeAnchors(doc.Blocks[1].EdgeAnchors)
	view := types.BuildAnswerSemanticViewForBusContext(bus)
	if view == nil || view.CallChainEndpointBoundary == nil || view.CallChainEndpointBoundary.EvidenceCapsule == nil ||
		view.CallChainEndpointBoundary.EvidenceCapsule.Status != types.CallChainEndpointEvidenceSharedCalleeBoundary ||
		len(types.CallChainEndpointBoundaryPrincipalEdges(view.CallChainEndpointBoundary.EvidenceCapsule)) != 2 {
		t.Fatalf("fixture must activate the exact two-edge endpoint boundary: %+v", view)
	}
	return bus, doc
}

func b1708Execute(t *testing.T, bus *types.BusContext, payload any, patch bool) types.ToolResult {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	before := string(raw)
	beforeEvidence, err := json.Marshal(bus.Mutable.EmittedEvidence())
	if err != nil {
		t.Fatal(err)
	}
	var result types.ToolResult
	if patch {
		result, err = (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
	} else {
		result, err = (&EmitAnswerDocument{}).Execute(bus, raw)
	}
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != before {
		t.Fatal("public emitter mutated model input bytes")
	}
	afterEvidence, err := json.Marshal(bus.Mutable.EmittedEvidence())
	if err != nil || string(afterEvidence) != string(beforeEvidence) {
		t.Fatalf("public emitter mutated evidence: err=%v", err)
	}
	return result
}

func TestB1708PublicEndpointSequenceKeepsSupportingCallSeparateFromPrincipalList(t *testing.T) {
	for _, mode := range []string{"full", "patch"} {
		t.Run(mode, func(t *testing.T) {
			bus, doc := b1708EndpointSequenceFixture(t)
			wantDiagram := doc.Blocks[1]
			wantBoundary := doc.Blocks[2]
			var result types.ToolResult
			if mode == "patch" {
				base := *doc
				base.Blocks = append([]types.AnswerBlock(nil), doc.Blocks...)
				base.Blocks[1] = wantDiagram
				base.Blocks[1].Diagram = &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid",
					Body: "sequenceDiagram\n  participant E as Entry\n  participant S as Shared\n  participant W as Wrapper\n  E->>S: submit prepared input\n  W->>S: forward the independent request"}
				base.Blocks[1].EdgeAnchors = append([]types.DiagramEdgeAnchor(nil), wantDiagram.EdgeAnchors[1:]...)
				if prerequisite := b1708Execute(t, bus, &base, false); !prerequisite.Success {
					t.Fatalf("public endpoint-only base must pass: %+v", prerequisite)
				}
				result = b1708Execute(t, bus, types.AnswerDocumentV2Patch{
					ReplaceBlocks: []types.AnswerBlock{wantDiagram}, UnchangedBlockIDs: []string{"summary", "boundary"},
				}, true)
			} else {
				result = b1708Execute(t, bus, doc, false)
			}
			if !result.Success {
				t.Fatalf("grounded same-caller support in the principal sequence must remain legal: %+v", result)
			}
			got := bus.Mutable.AnswerDocumentV2()
			if got == nil {
				t.Fatal("successful public answer was not retained")
			}
			diagram := blockByID(t, got, "diagram")
			if diagram.Diagram.Body != wantDiagram.Diagram.Body || !reflect.DeepEqual(diagram.EdgeAnchors, wantDiagram.EdgeAnchors) {
				t.Fatalf("public %s rewrote model-owned sequence order, wording, or anchors: %+v", mode, diagram)
			}
			boundary := blockByID(t, got, "boundary")
			if len(boundary.Items) != 2 || boundary.Items[0].Text != wantBoundary.Items[0].Text ||
				boundary.Items[1].Text != wantBoundary.Items[1].Text ||
				!reflect.DeepEqual(boundary.Items[0].EvidenceIDs, []string{"source-boundary"}) ||
				!reflect.DeepEqual(boundary.Items[1].EvidenceIDs, []string{"sink-boundary"}) {
				t.Fatalf("support diagram permission changed principal list ownership: %+v", boundary)
			}
		})
	}
}

func TestB1708PublicEndpointSequenceCannotBroadenPrincipalOrEvidenceAuthority(t *testing.T) {
	for _, mode := range []string{"principal_sibling", "unsupported_diagram", "reversed_diagram"} {
		for _, operation := range []string{"full", "patch"} {
			t.Run(mode+"/"+operation, func(t *testing.T) {
				bus, doc := b1708EndpointSequenceFixture(t)
				var accepted *types.AnswerDocumentV2
				if operation == "patch" {
					if prerequisite := b1708Execute(t, bus, doc, false); !prerequisite.Success {
						t.Fatalf("public valid answer prerequisite: %+v", prerequisite)
					}
					accepted = bus.Mutable.AnswerDocumentV2()
				}
				switch mode {
				case "principal_sibling":
					doc.Blocks[2].Items = append(doc.Blocks[2].Items, types.AnswerBlockItem{
						ID: "support-borrowed-as-hop", Label: "Prepare", Text: "Prepare the input.", EvidenceIDs: []string{"prepare"}, CitationRef: -1})
					doc.Blocks[2].EdgeAnchors = append(doc.Blocks[2].EdgeAnchors, doc.Blocks[1].EdgeAnchors[0])
				case "unsupported_diagram":
					doc.Blocks[1].Diagram.Body = strings.ReplaceAll(doc.Blocks[1].Diagram.Body, "Prepare", "Invented")
					doc.Blocks[1].EdgeAnchors[0].ToIdentity = "Invented"
				case "reversed_diagram":
					doc.Blocks[1].Diagram.Body = strings.Replace(doc.Blocks[1].Diagram.Body, "E->>P:", "P->>E:", 1)
					anchor := &doc.Blocks[1].EdgeAnchors[0]
					anchor.FromNode, anchor.ToNode = anchor.ToNode, anchor.FromNode
					anchor.FromIdentity, anchor.ToIdentity = anchor.ToIdentity, anchor.FromIdentity
				}
				var result types.ToolResult
				if operation == "patch" {
					result = b1708Execute(t, bus, types.AnswerDocumentV2Patch{
						ReplaceBlocks: doc.Blocks[1:], UnchangedBlockIDs: []string{"summary"},
					}, true)
				} else {
					result = b1708Execute(t, bus, doc, false)
				}
				if result.Success || result.Repair == nil {
					t.Fatalf("supporting sequence permission must not authorize %s: %+v", mode, result)
				}
				if mode == "principal_sibling" {
					if !strings.Contains(result.Summary, "support-borrowed-as-hop") || !strings.Contains(result.Summary, "Off-facet items") {
						t.Fatalf("rejection must retain the exact endpoint facet ownership reason: %+v", result)
					}
				} else if !strings.Contains(result.Repair.Metadata[types.ToolRepairMetaDiagramRelationFailureIssues], diagramCallEdgeIssueNoEvidence) {
					t.Fatalf("diagram rejection must retain the source relation evidence gate: %+v", result)
				}
				if !reflect.DeepEqual(bus.Mutable.AnswerDocumentV2(), accepted) {
					t.Fatal("rejected diagram or principal carrier changed the accepted answer")
				}
			})
		}
	}
}
