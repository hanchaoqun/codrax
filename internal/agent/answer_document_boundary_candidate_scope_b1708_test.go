package agent

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1708RepairBoundarySeedIsNotPresentationCeiling(t *testing.T) {
	ctx := b1708BoundaryCandidateContext()
	hint := appendRetryDiagramSeedHint("Repair the model-authored diagram.", ctx, nil)
	for _, want := range []string{
		"minimal boundary reference, not the diagram's complete evidence scope",
		"same typed relation-authority and participant candidates supplied for initial authoring",
		"does not become a principal intermediate hop",
	} {
		if !strings.Contains(hint, want) {
			t.Fatalf("retry seed contradicts the presentation domain: missing %q", want)
		}
	}
	// A minimal seed keeps the exact boundary; the typed authoring capsule,
	// not this fallback floor, carries the wider independently grounded pool.
	seed := buildRetryCallChainEndpointBoundarySeed(ctx, types.DiagramSequence)
	if strings.Contains(seed.Fence, "prepareSchema") || strings.Contains(seed.Fence, "resolveSymbols") {
		t.Fatal("boundary seed must not promote independent support into the principal floor")
	}
	_, all, _, _ := answerDocCurrentSourceMechanismRelations(ctx)
	presentation := answerDocMechanismPresentationRelations(ctx, all)
	if len(presentation.principal) != 2 || len(presentation.diagram) != 4 {
		t.Fatalf("minimal boundary and broader presentation must stay separate: %+v", presentation)
	}
}

func TestB1708PrincipalMembershipKeepsSourceIdentity(t *testing.T) {
	ctx := b1708BoundaryCandidateContext()
	_, all, _, _ := answerDocCurrentSourceMechanismRelations(ctx)
	for _, edge := range append([]answerDocMechanismRelationEdge(nil), all...) {
		if edge.to != "quality.RunWith" {
			continue
		}
		other := edge
		other.sourceItem.Source = "another/repository.go"
		other.sourceItem.ID = "same-name-other-source"
		other.loc = "another/repository.go:30"
		all = append(all, other)
	}
	principal := answerDocMechanismEndpointBoundaryEdges(ctx, all)
	if len(principal) != 2 {
		t.Fatalf("same endpoint spellings must not grant principal source ownership: %+v", principal)
	}
	for _, edge := range principal {
		if edge.sourceItem.Source == "another/repository.go" {
			t.Fatal("foreign source spelling was promoted into exact endpoint boundary")
		}
	}
}

func TestB1708DefinitionOnlySinkDoesNotEraseIndependentDiagramCalls(t *testing.T) {
	ctx := b1708BoundaryCandidateContext()
	ctx.EvidenceItems = ctx.EvidenceItems[:3]
	ctx.EvidenceItems = append(ctx.EvidenceItems, types.EvidenceItem{
		ID: "sink-definition", Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition,
		Subject: "quality.Run", AnchorSymbol: "Run", Source: "pkg/quality.go", LineStart: 50,
		Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
	})
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: ctx.EvidenceItems})
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	if view.CallChainEndpointBoundary.EvidenceCapsule.Status != types.CallChainEndpointEvidenceEndpointUnresolved {
		t.Fatal("definition-only sink must not acquire graph incidence")
	}
	if len(types.CallChainEndpointBoundaryPrincipalEdges(view.CallChainEndpointBoundary.EvidenceCapsule)) != 0 {
		t.Fatal("independent calls must not become a principal endpoint path")
	}
	edges := b1708PublishedTemplateEdges(t, prompt)
	if len(edges) != 3 || !edges["call:compileRequest->prepareSchema"] {
		t.Fatalf("unproved requested endpoint erased independently grounded diagram operations: %v", edges)
	}
	for edge := range edges {
		if edge == "call:compileRequest->quality.Run" || edge == "call:quality.Run->quality.RunWith" {
			t.Fatalf("definition-only endpoint was given a fictional arrow: %s", edge)
		}
	}
}

// This extends the existing BuildInitialInstruction endpoint-boundary fixture
// with an explicitly required sequence diagram and two earlier sibling calls.
// All four relations are grounded; only two belong to the endpoint boundary.
// No source file, live model, or answer wording establishes extra authority.
func b1708BoundaryCandidateContext() *types.AgentContext {
	call := func(id, from, to, file string, line int) types.EvidenceItem {
		return types.EvidenceItem{
			ID: id, Producer: types.EvidenceProducerExplorerEmitEvidence,
			Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall,
			Subject: from, Predicate: "calls", Object: to, AnchorSymbol: to,
			Source: file, LineStart: line, Scope: types.ScopeLine,
			GroundingStatus: types.GroundingGrounded,
		}
	}
	evidence := []types.EvidenceItem{
		call("source-helper-a", "compileRequest", "prepareSchema", "pkg/analyzer.go", 10),
		call("source-helper-b", "compileRequest", "resolveSymbols", "pkg/analyzer.go", 20),
		call("source-boundary", "compileRequest", "quality.RunWith", "pkg/analyzer.go", 30),
		call("sink-boundary", "quality.Run", "quality.RunWith", "pkg/quality.go", 50),
	}
	mut := types.NewMutableState("B1708 typed sequence boundary fixture")
	mut.SetPrincipalSpanWaiver(&types.PrincipalSpanWaiver{
		Reason: types.PrincipalSpanWaiverNoDirectedPath, Rationale: "B1708 audit-only accepted boundary declaration",
	})
	mut.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: evidence})
	participants := []types.DiagramParticipantHint{
		{Identity: "compileRequest", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "quality.Run", Role: types.DiagramParticipantIncidentRequired},
	}
	return &types.AgentContext{
		Mutable: mut, EvidenceItems: evidence,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentTrace, PredicateAxis: types.AxisCall, Language: "zh",
				CallChainEndpointProfile: &types.CallChainEndpointProfile{Source: "compileRequest", Sink: "quality.Run"},
				AnalyzerHints:            types.AnalyzerHints{Kind: string(types.ReqCallChain), ExactTargets: []string{"compileRequest", "quality.Run"}},
				DiagramHint:              &types.DiagramHint{Kind: types.DiagramSequence, Required: true, Participants: participants},
			},
			AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{
				Required: true, RequiredKind: types.DiagramSequence, Participants: participants,
			}},
		},
	}
}

func b1708PublishedTemplateEdges(t *testing.T, prompt string) map[string]bool {
	t.Helper()
	edges := map[string]bool{}
	for _, line := range strings.Split(prompt, "\n") {
		const prefix = "- edge_anchors_json=`"
		if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, "`") {
			continue
		}
		var anchors []types.DiagramEdgeAnchor
		if err := json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimPrefix(line, prefix), "`")), &anchors); err != nil {
			t.Fatalf("published topology anchors are not native JSON: %v", err)
		}
		for _, anchor := range anchors {
			if anchor.VisibleLabel != "" {
				t.Fatalf("authoring input must leave the visible business action model-owned: %+v", anchor)
			}
			edges[fmt.Sprintf("%s:%s->%s", anchor.RelationKind, anchor.FromIdentity, anchor.ToIdentity)] = true
		}
	}
	if len(edges) == 0 {
		t.Fatal("fixture did not reach the public copy-ready diagram authoring surface")
	}
	return edges
}

func TestB1708BuildInitialInstructionKeepsNoDirectedPathBoundary(t *testing.T) {
	ctx := b1708BoundaryCandidateContext()
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	if view == nil || view.CallChainEndpointBoundary == nil || view.CallChainEndpointBoundary.EvidenceCapsule == nil {
		t.Fatal("fixture did not produce a typed endpoint boundary")
	}
	boundary := view.CallChainEndpointBoundary
	if boundary.Disposition != types.CallChainEndpointNoDirectedPath || boundary.EvidenceCapsule.Status != types.CallChainEndpointEvidenceSharedCalleeBoundary {
		t.Fatalf("sibling calls must not manufacture source-to-requested-sink reachability: %+v", boundary)
	}
	if edges := types.CallChainEndpointBoundaryPrincipalEdges(boundary.EvidenceCapsule); len(edges) != 2 {
		t.Fatalf("principal endpoint boundary must retain exactly its two edges, not the sibling roster: %+v", edges)
	}
	b1708AssertPrincipalFacetExcludesSupport(t, view)
	for _, want := range []string{
		"**Diagram contract:** required (kind=sequence)",
		"Never turn the two inward-pointing paths into a source-to-sink chain",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("public instruction lost the honest endpoint/required-sequence contract %q", want)
		}
	}
	edges := b1708PublishedTemplateEdges(t, prompt)
	for _, want := range []string{"call:compileRequest->quality.RunWith", "call:quality.Run->quality.RunWith"} {
		if !edges[want] {
			t.Fatalf("published template lost the exact endpoint direction %q: %v", want, edges)
		}
	}
	for _, forbidden := range []string{"call:compileRequest->quality.Run", "call:quality.RunWith->quality.Run", "call:prepareSchema->resolveSymbols"} {
		if edges[forbidden] {
			t.Fatalf("presentation must not invent a path or sibling-to-sibling invocation: %s", forbidden)
		}
	}
}

func b1708AssertPrincipalFacetExcludesSupport(t *testing.T, view *types.AnswerSemanticView) {
	t.Helper()
	if view == nil || view.FacetCoverage == nil {
		t.Fatal("fixture did not compile principal facet coverage")
	}
	found := false
	for _, requirements := range [][]types.FacetRequirement{view.FacetCoverage.Required, view.FacetCoverage.Optional} {
		for _, requirement := range requirements {
			if requirement.Kind != types.FacetPrincipalPathEdge {
				continue
			}
			found = true
			for _, id := range requirement.SourceCandidate {
				if id != "source-boundary" && id != "sink-boundary" {
					t.Errorf("independent diagram support must not become a principal endpoint hop: %q", id)
				}
			}
		}
	}
	if !found {
		t.Fatal("fixture did not reach the principal_path_edge contract")
	}
}

func b1708AssertSmallPresentationPool(t *testing.T, edges map[string]bool) {
	t.Helper()
	// This fixture is deliberately smaller than the presentation budget. This
	// asserts retention for these four rows, not that every legal relation in an
	// arbitrarily large evidence pool must fit a bounded authoring template.
	want := map[string]bool{
		"call:compileRequest->prepareSchema":   true,
		"call:compileRequest->resolveSymbols":  true,
		"call:compileRequest->quality.RunWith": true,
		"call:quality.Run->quality.RunWith":    true,
	}
	for edge := range want {
		if !edges[edge] {
			t.Errorf("explicit sequence authoring input dropped grounded independent source call %q; got %v", edge, edges)
		}
	}
	for edge := range edges {
		if !want[edge] {
			t.Errorf("presentation invented, reversed, reconnected, or admitted an ungrounded relation %q", edge)
		}
	}
}

func TestB1708BuildInitialInstructionRetainsIndependentSameCallerCalls(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse_evidence=%t", reverse), func(t *testing.T) {
			ctx := b1708BoundaryCandidateContext()
			ungrounded := ctx.EvidenceItems[0]
			ungrounded.ID, ungrounded.Object, ungrounded.AnchorSymbol = "unproved-helper", "inventedHelper", "inventedHelper"
			ungrounded.GroundingStatus = types.GroundingUngrounded
			ctx.EvidenceItems = append(ctx.EvidenceItems, ungrounded)
			if reverse {
				for left, right := 0, len(ctx.EvidenceItems)-1; left < right; left, right = left+1, right-1 {
					ctx.EvidenceItems[left], ctx.EvidenceItems[right] = ctx.EvidenceItems[right], ctx.EvidenceItems[left]
				}
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: ctx.EvidenceItems})
			before, err := json.Marshal(ctx.EvidenceItems)
			if err != nil {
				t.Fatal(err)
			}
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			b1708AssertSmallPresentationPool(t, b1708PublishedTemplateEdges(t, prompt))
			b1708AssertPrincipalFacetExcludesSupport(t, types.BuildAnswerSemanticViewForAgentContext(ctx))
			for _, want := range []string{
				"grounded source-line order",
				"does not prove that every branch executes",
				"AUTHOR_BUSINESS_ACTION",
				"The model still authors every visible node, edge, label, diagram, and conclusion",
			} {
				if !strings.Contains(prompt, want) {
					t.Errorf("diagram support must retain presentation-only/model-ownership teaching %q", want)
				}
			}
			after, err := json.Marshal(ctx.EvidenceItems)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || ctx.Mutable.AnswerDocumentV2() != nil {
				t.Fatal("instruction construction must not rewrite evidence or author an answer")
			}
		})
	}
}

func TestB1708BuildInitialInstructionWithoutExplicitDiagramKeepsBoundaryOnlyRecipe(t *testing.T) {
	ctx := b1708BoundaryCandidateContext()
	ctx.AnalysisIR.RequestModel.DiagramHint = nil
	ctx.AnalysisIR.AnswerContract.Diagram = nil
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	if view == nil {
		t.Fatal("missing semantic view")
	}
	for _, requirement := range view.RequiredBlocks {
		if requirement.Kind == types.BlockDiagram && requirement.Required {
			t.Fatal("presentation support must not create a diagram obligation absent explicit intent")
		}
	}
	// Without an explicit diagram, a copy-ready diagram template is optional.
	// The public endpoint recipes themselves remain restricted to the boundary.
	edges := map[string]bool{}
	for _, line := range strings.Split(prompt, "\n") {
		if !strings.HasPrefix(line, "- edge_recipe[") {
			continue
		}
		_, raw, ok := strings.Cut(line, "edge_anchor_json=`")
		if !ok {
			t.Fatal("public relation recipe omitted its native anchor")
		}
		raw, _, _ = strings.Cut(raw, "`")
		var anchor types.DiagramEdgeAnchor
		if err := json.Unmarshal([]byte(raw), &anchor); err != nil {
			t.Fatal(err)
		}
		edges[fmt.Sprintf("%s:%s->%s", anchor.RelationKind, anchor.FromIdentity, anchor.ToIdentity)] = true
	}
	if len(edges) != 2 || !edges["call:compileRequest->quality.RunWith"] || !edges["call:quality.Run->quality.RunWith"] {
		t.Fatalf("without an explicit diagram the principal endpoint recipe must remain unchanged: %v", edges)
	}
	b1708AssertPrincipalFacetExcludesSupport(t, view)
}

func TestB1708ObserveRepairSharesInitialDiagramPresentationPool(t *testing.T) {
	for _, toolName := range []string{"emit_answer_document", "emit_answer_document_patch"} {
		t.Run(toolName, func(t *testing.T) {
			ctx := b1708BoundaryCandidateContext()
			ctx.Mutable.SetPendingAnswerDocumentPatchBase(&types.AnswerDocumentV2{
				DocumentModel: "v2",
				Blocks: []types.AnswerBlock{
					{ID: "model-summary", Kind: types.BlockSummary, Text: "model-owned summary"},
					{ID: "model-sequence", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{
						Kind: types.DiagramSequence, Language: "mermaid",
						Body: "sequenceDiagram\n participant source as compileRequest\n participant sink as quality.Run\n source->>sink: model-owned unsupported bridge",
					}},
				},
			})
			before, err := json.Marshal(ctx.Mutable.PendingAnswerDocumentPatchBase())
			if err != nil {
				t.Fatal(err)
			}
			evaluator := &answerDocumentEvaluator{diagramRequired: true, mu: ctx.Mutable}
			initial := evaluator.BuildInitialInstruction(ctx, nil)
			initialEdges := b1708PublishedTemplateEdges(t, initial)
			// Observe is the public repair consumer. The producer-shaped typed
			// violation intentionally has no executable relation lease, selecting
			// the shared authoring-input repair lane rather than a local mutation.
			signal := evaluator.Observe(ctx, LoopObservation{
				Phase: PhaseMidLoop,
				LastToolResult: &types.ToolResult{
					ToolName: toolName, Success: false,
					Repair: &types.ToolRepair{
						Code: "answer_doc_pre_emit_contract",
						Metadata: map[string]string{
							"violation_kinds":                       string(types.ViolDiagramCallEdgeUnproven),
							types.ToolRepairMetaOffendingBlockKinds: string(types.BlockDiagram),
						},
					},
				},
			})
			if !signal.HintRequested || signal.StopRequested {
				t.Fatalf("public Observe did not reach a required-diagram repair: %+v", signal)
			}
			repairEdges := b1708PublishedTemplateEdges(t, signal.Hint)
			b1708AssertSmallPresentationPool(t, initialEdges)
			b1708AssertSmallPresentationPool(t, repairEdges)
			if len(initialEdges) != len(repairEdges) {
				t.Errorf("initial and repair authoring scopes diverged: initial=%v repair=%v", initialEdges, repairEdges)
			}
			for edge := range initialEdges {
				if !repairEdges[edge] {
					t.Errorf("repair dropped initial typed presentation choice %q", edge)
				}
			}
			for _, want := range []string{"emit_answer_document_patch", "unchanged_block_ids", "AUTHOR_BUSINESS_ACTION", "does not write a visible label or rewrite the model's prose, ordering, or conclusions"} {
				if !strings.Contains(signal.Hint, want) {
					t.Errorf("repair must retain required-diagram/model-ownership contract %q", want)
				}
			}
			after, err := json.Marshal(ctx.Mutable.PendingAnswerDocumentPatchBase())
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || ctx.Mutable.AnswerDocumentV2() != nil {
				t.Fatal("repair guidance must not author or mutate the model's answer")
			}
		})
	}
}

func TestB1708BuildInitialInstructionParticipantChoicesIntersectPublishedDiagramScope(t *testing.T) {
	ctx := b1708BoundaryCandidateContext()
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	edges := b1708PublishedTemplateEdges(t, prompt)
	var choices string
	for _, line := range strings.Split(prompt, "\n") {
		if strings.HasPrefix(line, "- First-pass typed participant endpoint choices") {
			choices = line
			break
		}
	}
	if choices == "" {
		t.Fatal("fixture did not reach the first-pass participant-choice public contract")
	}
	for _, participant := range ctx.AnalysisIR.RequestModel.DiagramHint.Participants {
		pattern := `typed_candidate\[` + regexp.QuoteMeta(participant.Identity) + `\]\[[0-9]+\]=\{relation_kind:"([^"]+)",visible_arrow_label:"[^"]*",from_identity:"([^"]+)",to_identity:"([^"]+)"`
		matches := regexp.MustCompile(pattern).FindAllStringSubmatch(choices, -1)
		if len(matches) == 0 {
			t.Fatalf("incident-required participant %q received no first-pass choices", participant.Identity)
		}
		var offered []string
		compatible := false
		for _, match := range matches {
			key := fmt.Sprintf("%s:%s->%s", match[1], match[2], match[3])
			offered = append(offered, key)
			compatible = compatible || edges[key]
		}
		if !compatible {
			var published []string
			for key := range edges {
				published = append(published, key)
			}
			sort.Strings(published)
			t.Errorf("B1708 incompatible authoring domains for %q: first-pass guidance requires choosing one of %v, but the published diagram topology permits %v; endpoint-only instruction present=%t. Keep the no-directed-path boundary; share the diagram/participant presentation scope instead of forcing an outside-scope repair", participant.Identity, offered, published, strings.Contains(prompt, "draw only the exact endpoint-boundary subgraph"))
		}
	}
}
