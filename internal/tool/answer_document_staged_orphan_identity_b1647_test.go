package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The first public call really stages a graph with both an inherited
// identity-less edge and a newly authorized, identified edge on the same node
// pair. The next public schema permits only an orphan declaration decision.
// A system metadata normalization must not turn that decision into an
// unlisted relation change.
func TestB1647PublicStagedOrphanKeepsMixedIdentityBaseline(t *testing.T) {
	for _, shape := range []string{"mixed", "mixed_recipe_alias", "mixed_recipe_alias_with_evidence", "legacy_only", "identified_only"} {
		for _, action := range []string{"remove_if_isolated", "retain_as_context"} {
			t.Run(shape+"/"+action, func(t *testing.T) {
				bus, accepted, staged, _ := b1647StageOrphan(t, shape)
				lease := bus.Mutable.AnswerDiagramRelationRepairLease()
				leaseBefore, _ := json.Marshal(lease)
				schema := string((&EmitAnswerDocumentPatch{}).parametersForContext(types.BuildAnswerSemanticViewForBusContext(bus), bus.Mutable, bus))
				for _, want := range []string{`"participant_id":{"enum":["D"]`, `"remove_if_isolated"`, `"retain_as_context"`} {
					if !strings.Contains(schema, want) {
						t.Fatalf("current public orphan capability missing %s: %s", want, schema)
					}
				}
				if strings.Contains(schema, `"diagram_edge_edits"`) {
					t.Fatalf("orphan generation must not publish relation edits: %s", schema)
				}
				edit := map[string]any{"block_id": "diag", "participant_id": "D", "action": action}
				if action == "retain_as_context" {
					edit["visible_label"] = "OldCallee (context only)"
				}
				raw, _ := json.Marshal(map[string]any{"diagram_participant_edits": []any{edit}})
				result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
				if err != nil || !result.Success {
					t.Fatalf("current schema's exact orphan choice must publish: err=%v summary=%s staged=%+v", err, result.Summary, staged.Blocks[1].EdgeAnchors)
				}
				got := bus.Mutable.AnswerDocumentV2()
				if got == nil || !reflect.DeepEqual(got.Blocks[0], accepted.Blocks[0]) || len(got.Citations) < len(accepted.Citations) || !reflect.DeepEqual(got.Citations[:len(accepted.Citations)], accepted.Citations) {
					t.Fatalf("unselected model block or citation changed: %+v", got)
				}
				if bus.AnalysisIR == nil && len(got.Citations) != len(accepted.Citations) {
					t.Fatal("lease-only path must not grow the citation pool")
				}
				for _, block := range got.Blocks {
					for _, item := range block.Items {
						for _, ref := range types.AnswerBlockItemCitationRefs(item) {
							if ref >= len(accepted.Citations) && block.SystemGeneratedKind == "" {
								t.Fatal("new source-supplement citation was assigned to a model block")
							}
						}
					}
				}
				for _, citation := range got.Citations[len(accepted.Citations):] {
					matched := false
					for _, ev := range bus.EvidenceItems {
						matched = matched || (citation.File == ev.Source && citation.Line == ev.LineStart)
					}
					if !matched {
						t.Fatalf("system supplement borrowed an unrelated source: %+v", citation)
					}
				}
				if b1647MessageLines(got.Blocks[1].Diagram.Body) != b1647MessageLines(staged.Blocks[1].Diagram.Body) {
					t.Fatalf("orphan decision changed model messages: before=%q after=%q", staged.Blocks[1].Diagram.Body, got.Blocks[1].Diagram.Body)
				}
				if action == "remove_if_isolated" && strings.Contains(got.Blocks[1].Diagram.Body, "participant D") {
					t.Fatal("model-selected orphan removal was not committed")
				}
				if action == "retain_as_context" && !strings.Contains(got.Blocks[1].Diagram.Body, "OldCallee (context only)") {
					t.Fatal("model-selected context label was not retained")
				}
				if len(got.Blocks[1].EdgeAnchors) != len(staged.Blocks[1].EdgeAnchors) {
					t.Fatal("identity stabilization must not fold duplicate visible messages")
				}
				for i, anchor := range got.Blocks[1].EdgeAnchors {
					want := staged.Blocks[1].EdgeAnchors[i]
					if bus.AnalysisIR != nil {
						want.ClaimForm = types.ClaimCallEdge
					}
					if anchor != want {
						t.Fatalf("lease-only fixture must preserve the staged anchor metadata: got=%+v want=%+v", anchor, want)
					}
				}
				leaseAfter, _ := json.Marshal(lease)
				if string(leaseBefore) != string(leaseAfter) || bus.Mutable.PendingAnswerDocumentPatchBase() != nil || bus.Mutable.AnswerDiagramRelationRepairLease() != nil {
					t.Fatal("successful commit must consume retry state without mutating the captured lease")
				}
			})
		}
	}
}

func b1647StageOrphan(t *testing.T, shape string) (*types.BusContext, *types.AnswerDocumentV2, *types.AnswerDocumentV2, types.DiagramEdgeAnchor) {
	t.Helper()
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n participant A as Caller\n participant B as Callee\n participant C as OldCaller\n participant D as OldCallee\n A->>B: keep\n C->>D: remove\n"
	prev.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{
		{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall, VisibleLabel: "keep"},
		{FromNode: "C", ToNode: "D", FromIdentity: "OldCaller", ToIdentity: "OldCallee", RelationKind: types.DiagramRelCall, VisibleLabel: "remove"},
	}
	if shape == "identified_only" {
		prev.Blocks[1].EdgeAnchors[0].FromIdentity, prev.Blocks[1].EdgeAnchors[0].ToIdentity = "Caller", "Callee"
	}
	recipe := types.DiagramEdgeAnchor{FromNode: "A", ToNode: "B", FromIdentity: "Caller", ToIdentity: "Callee", RelationKind: types.DiagramRelCall}
	if strings.HasPrefix(shape, "mixed_recipe_alias") {
		// The source-derived capsule and model diagram own separate node-id
		// namespaces. Like the live qf case, both can use A/B for different
		// endpoint methods. This is not an equivalence permission.
		recipe.ToIdentity = "OtherCallee"
	}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "diag", Issue: "call_edge_unproven", FromNode: "C", ToNode: "D", FromIdentity: "OldCaller", ToIdentity: "OldCallee", RelationKind: types.DiagramRelCall, BodyOccurrence: 1,
	}}, []types.AnswerDiagramRelationRepairCandidate{{
		BlockID: "diag", FromIdentity: "Caller", ToIdentity: "Callee", RelationKind: types.DiagramRelCall, FromNodeIDs: []string{"A"}, ToNodeIDs: []string{"B"}, Source: "source.go:10",
	}})
	mixed := strings.HasPrefix(shape, "mixed")
	if lease == nil || len(lease.Failures) != 1 || (mixed && len(lease.AllowedAdditions) != 1) {
		t.Fatalf("invalid initial public lease: %+v", lease)
	}
	lease.OptionalOrphanCleanups = testDiagramOrphanCandidates("diag", "D")
	mut := types.NewMutableState("staged orphan mixed identity")
	bus := &types.BusContext{Mutable: mut}
	if strings.HasSuffix(shape, "_with_evidence") {
		bus.AnalysisIR = &types.AnalysisIR{}
		bus.EvidenceItems, bus.RepoRoot = b1647TwoCallOccurrences(t)
		bus.WorkDir = bus.RepoRoot
		ev := bus.EvidenceItems[0]
		prev.Citations = []types.Citation{{File: ev.Source, Line: ev.LineStart, LineEnd: ev.LineEnd, Scope: ev.Scope, Quote: ev.Snippet}}
		if types.BuildAnswerSemanticViewForBusContext(bus) == nil {
			t.Fatal("ordinary public pre-emit must actually be enabled")
		}
	}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	mut.SetFinalizerTypedRelationRecipeAvailable(true)
	mut.SetFinalizerTypedRelationRecipeAnchors([]types.DiagramEdgeAnchor{recipe})
	accepted := mut.AnswerDocumentV2()
	edits := fmt.Sprintf(`{"diagram_edge_edits":[{"action":"remove","failure_ref":%q}`, lease.Failures[0].FailureRef)
	if mixed {
		edits += fmt.Sprintf(`,{"action":"add","addition_ref":%q,"edge":{"from_node":"A","to_node":"B","visible_label":"keep"},"from_node_visible_label":"Caller","to_node_visible_label":"Callee"}`, lease.AllowedAdditions[0].AdditionRef)
	}
	edits += `]}`
	result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(edits))
	if err != nil || result.Success || result.Repair == nil || result.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome] != types.AnswerDocumentPatchOutcomeStagedForRetry {
		t.Fatalf("first public call must stage before orphan decision: err=%v result=%+v", err, result)
	}
	if !reflect.DeepEqual(accepted, mut.AnswerDocumentV2()) {
		t.Fatal("staging must not publish or rewrite the accepted document")
	}
	staged := mut.PendingAnswerDocumentPatchBase()
	live := mut.AnswerDiagramRelationRepairLease()
	if staged == nil || live == nil || !live.OrphanDispositionOnly || len(live.Failures) != 0 || len(live.AllowedAdditions) != 0 || len(live.OptionalOrphanCleanups) != 1 || live.OptionalOrphanCleanups[0].ParticipantID != "D" {
		t.Fatalf("expected exact new orphan-only generation: %+v", live)
	}
	wantCount := 1
	if mixed {
		wantCount = 2
	}
	if len(staged.Blocks[1].EdgeAnchors) != wantCount || !reflect.DeepEqual(staged.Blocks[1].EdgeAnchors, live.Blocks[0].BaseAnchors) {
		t.Fatalf("stage and lease must freeze the same exact anchor sequence: %+v / %+v", staged.Blocks[1].EdgeAnchors, live.Blocks)
	}
	if mixed && (staged.Blocks[1].EdgeAnchors[0].HasEndpointIdentityPair() || staged.Blocks[1].EdgeAnchors[1].FromIdentity != "Caller") {
		t.Fatal("missing the real mixed-identity staging precondition")
	}
	return bus, accepted, staged, recipe
}

func b1647MessageLines(body string) string {
	var lines []string
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "->") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func TestB1647MixedIdentityStabilizationKeepsOccurrenceOwnership(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse=%t", reverse), func(t *testing.T) {
			bus, _, staged, recipe := b1647StageOrphan(t, "mixed")
			lease := bus.Mutable.AnswerDiagramRelationRepairLease()
			lease.Blocks[0].BaseAnchors[0].VisibleLabel = "inherited message"
			lease.Blocks[0].BaseAnchors[1].VisibleLabel = "new model message"
			// Multiple inherited occurrences still retain their multiplicity.
			lease.Blocks[0].BaseAnchors = append(lease.Blocks[0].BaseAnchors, lease.Blocks[0].BaseAnchors[0])
			staged.Blocks[1].EdgeAnchors = append([]types.DiagramEdgeAnchor(nil), lease.Blocks[0].BaseAnchors...)
			if reverse {
				lease.Blocks[0].BaseAnchors[0], lease.Blocks[0].BaseAnchors[1] = lease.Blocks[0].BaseAnchors[1], lease.Blocks[0].BaseAnchors[0]
				staged.Blocks[1].EdgeAnchors[0], staged.Blocks[1].EdgeAnchors[1] = staged.Blocks[1].EdgeAnchors[1], staged.Blocks[1].EdgeAnchors[0]
			}
			// B1647c: retain the original mixed/reordered occurrences, but
			// obtain comparison authority from this actual normalization only.
			beforeIdentityRepair := snapshotDiagramAnchorIdentities(staged)
			normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(staged, []types.DiagramEdgeAnchor{recipe})
			receipt := recordDiagramAnchorIdentityRepair(beforeIdentityRepair, staged)
			bodyBefore := staged.Blocks[1].Diagram.Body
			leaseBefore, _ := json.Marshal(lease)
			if n := stabilizeUnlistedRelationLeaseAnchorIdentities(staged, lease, receipt); n != 2 {
				t.Fatalf("want exactly two inherited identity slots, got %d", n)
			}
			for _, anchor := range staged.Blocks[1].EdgeAnchors {
				if (anchor.VisibleLabel == "new model message") != anchor.HasEndpointIdentityPair() {
					t.Fatalf("identity moved between differently labelled occurrences: %+v", anchor)
				}
			}
			if violations := types.ValidateAnswerDiagramRelationRepairLease(lease, staged); len(violations) != 0 {
				t.Fatalf("exact mixed baseline not restored: %+v", violations)
			}
			leaseAfter, _ := json.Marshal(lease)
			if bodyBefore != staged.Blocks[1].Diagram.Body || string(leaseBefore) != string(leaseAfter) {
				t.Fatal("comparison stabilization changed model text or its immutable lease")
			}
		})
	}
}

func TestB1647MixedIdentityStabilizationDoesNotHideRealChanges(t *testing.T) {
	for _, change := range []string{"add", "remove", "from_method", "to_method", "direction", "kind", "partial_baseline", "different_baseline_method", "ambiguous_recipe", "no_recipe", "visible_label", "claim_form"} {
		t.Run(change, func(t *testing.T) {
			bus, _, candidate, recipe := b1647StageOrphan(t, "mixed")
			lease := bus.Mutable.AnswerDiagramRelationRepairLease()
			recipes := []types.DiagramEdgeAnchor{recipe}
			beforeIdentityRepair := snapshotDiagramAnchorIdentities(candidate)
			normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(candidate, recipes)
			receipt := recordDiagramAnchorIdentityRepair(beforeIdentityRepair, candidate)
			anchors := &candidate.Blocks[1].EdgeAnchors
			switch change {
			case "add":
				*anchors = append(*anchors, (*anchors)[0])
			case "remove":
				*anchors = (*anchors)[:1]
			case "from_method":
				(*anchors)[0].FromIdentity = "OtherCaller"
			case "to_method":
				(*anchors)[0].ToIdentity = "OtherCallee"
			case "direction":
				(*anchors)[0].FromNode, (*anchors)[0].ToNode = "B", "A"
			case "kind":
				(*anchors)[0].RelationKind = types.DiagramRelPrecedence
			case "partial_baseline":
				lease.Blocks[0].BaseAnchors[0].FromIdentity = "Caller"
			case "different_baseline_method":
				lease.Blocks[0].BaseAnchors[1].ToIdentity = "OtherCallee"
			case "ambiguous_recipe":
				other := recipe
				other.ToIdentity = "OtherCallee"
				recipes = append(recipes, other)
			case "no_recipe":
				recipes = nil
			case "visible_label":
				(*anchors)[0].VisibleLabel = "different model message"
			case "claim_form":
				(*anchors)[0].ClaimForm = types.ClaimCallEdge
			}
			if change == "ambiguous_recipe" || change == "no_recipe" {
				// Those recipes cannot produce the already populated candidate;
				// verify that fact with the real normalizer and supply no forged
				// before/after delta to the receipt consumer.
				unrepaired := *candidate
				unrepaired.Blocks = append([]types.AnswerBlock(nil), candidate.Blocks...)
				unrepaired.Blocks[1].EdgeAnchors = append([]types.DiagramEdgeAnchor(nil), beforeIdentityRepair["diag"]...)
				if n := normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(&unrepaired, recipes); n != 0 {
					t.Fatalf("ambiguous/absent recipe unexpectedly produced identity: %d", n)
				}
				receipt = recordDiagramAnchorIdentityRepair(beforeIdentityRepair, &unrepaired)
			}
			before, _ := json.Marshal(candidate)
			if n := stabilizeUnlistedRelationLeaseAnchorIdentities(candidate, lease, receipt); n != 0 {
				t.Fatalf("ambiguous or real change must not be hidden: fixed=%d", n)
			}
			after, _ := json.Marshal(candidate)
			if string(before) != string(after) || len(types.ValidateAnswerDiagramRelationRepairLease(lease, candidate)) == 0 {
				t.Fatal("unchanged scope guard must continue rejecting this relation delta")
			}
		})
	}
}

func TestB1647PublicOrphanScopeStillRejectsUnlistedMutations(t *testing.T) {
	for _, change := range []string{"add_edge", "remove_edge", "method_identity", "citation_ref"} {
		t.Run(change, func(t *testing.T) {
			bus, accepted, staged, _ := b1647StageOrphan(t, "mixed")
			before := bus.Mutable.PendingAnswerDocumentPatchBase()
			leaseBefore, _ := json.Marshal(bus.Mutable.AnswerDiagramRelationRepairLease())
			block := staged.Blocks[1]
			switch change {
			case "add_edge":
				block.Diagram.Body += " B->>A: unlisted reverse call\n"
				block.EdgeAnchors = append(block.EdgeAnchors, types.DiagramEdgeAnchor{FromNode: "B", ToNode: "A", RelationKind: types.DiagramRelCall})
			case "remove_edge":
				block.EdgeAnchors = block.EdgeAnchors[:1]
			case "method_identity":
				block.EdgeAnchors[0].FromIdentity = "OtherCaller"
			case "citation_ref":
				block.Items = []types.AnswerBlockItem{{Label: "unlisted citation", CitationRef: 999}}
			}
			raw, _ := json.Marshal(map[string]any{
				"replace_blocks":            []types.AnswerBlock{block},
				"diagram_participant_edits": []any{map[string]any{"block_id": "diag", "participant_id": "D", "action": "remove_if_isolated"}},
			})
			result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
			if err != nil || result.Success || !strings.Contains(result.Summary, "whole-block") {
				t.Fatalf("unpublished whole-block escape must remain rejected: err=%v summary=%s", err, result.Summary)
			}
			leaseAfter, _ := json.Marshal(bus.Mutable.AnswerDiagramRelationRepairLease())
			if !reflect.DeepEqual(accepted, bus.Mutable.AnswerDocumentV2()) || !reflect.DeepEqual(before, bus.Mutable.PendingAnswerDocumentPatchBase()) || string(leaseBefore) != string(leaseAfter) {
				t.Fatal("rejected mutation changed accepted document, staged base, or capability generation")
			}
		})
	}
}

func TestB1647RecipeAliasCollisionDoesNotPolluteOrdinaryPreEmit(t *testing.T) {
	bus, _, doc, _ := b1647StageOrphan(t, "mixed_recipe_alias")
	view := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}
	for i := range doc.Blocks[1].EdgeAnchors {
		doc.Blocks[1].EdgeAnchors[i].ClaimForm = types.ClaimCallEdge
	}
	before := append([]types.DiagramEdgeAnchor(nil), doc.Blocks[1].EdgeAnchors...)
	bodyBefore := doc.Blocks[1].Diagram.Body
	normalizeAnswerDocumentForPreEmit("emit_answer_document_patch", doc, view, bus, newPreEmitCheckContext(bus))
	if !reflect.DeepEqual(before, doc.Blocks[1].EdgeAnchors) || doc.Blocks[1].Diagram.Body != bodyBefore {
		t.Fatalf("capsule A/B must not overwrite the model diagram's different typed endpoint: before=%+v after=%+v", before, doc.Blocks[1].EdgeAnchors)
	}
	evidence, _ := b1647TwoCallOccurrences(t)
	if issues := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(issues) != 0 {
		t.Fatalf("the unchanged current diagram must still pass its own exact evidence: %+v", issues)
	}
	if issues := DiagramCallEdgeEvidenceMismatches(doc, view, evidence[:1]); len(issues) != 1 || issues[0].Issue != "call_edge_occurrence_unproven" {
		t.Fatalf("one actual call must not authorize two model messages: %+v", issues)
	}
	// A typed model pair only prevents a conflicting weak repair. It is not
	// evidence, and the unchanged evidence gate still rejects a wrong method.
	doc.Blocks[1].EdgeAnchors[1].ToIdentity = "UnprovenMethod"
	normalizeAnswerDocumentForPreEmit("emit_answer_document_patch", doc, view, bus, newPreEmitCheckContext(bus))
	if doc.Blocks[1].EdgeAnchors[0].HasEndpointIdentityPair() || doc.Blocks[1].EdgeAnchors[1].ToIdentity != "UnprovenMethod" {
		t.Fatal("weak repair changed either the unknown slot or the model's explicit identity")
	}
	if issues := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(issues) == 0 {
		t.Fatal("an explicit but unproved method must not gain evidence authority")
	}
}

func b1647TwoCallOccurrences(t *testing.T) ([]types.EvidenceItem, string) {
	t.Helper()
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "calls.go"), []byte("package p\nfunc Callee() {}\nfunc Caller() {\n Callee()\n Callee()\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{RepoRoot: repo, WorkDir: repo, Mutable: types.NewMutableState("read both actual calls")}
	read, err := (&ReadFile{}).Execute(bus, json.RawMessage(`{"path":"calls.go","offset":0,"limit":20}`))
	if err != nil || !read.Success {
		t.Fatalf("actual source read failed: err=%v result=%+v", err, read)
	}
	bus.ToolResults = []types.ToolResult{read}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{read}})
	emitted, err := (&EmitEvidence{}).Execute(bus, json.RawMessage(`{"items":[
		{"scope":"line","evidence_kind":"mechanism","subject":"Caller","predicate":"calls","object":"Callee","source":"calls.go","line_start":4,"anchor_kind":"call","anchor_symbol":"Callee"},
		{"scope":"line","evidence_kind":"mechanism","subject":"Caller","predicate":"calls","object":"Callee","source":"calls.go","line_start":5,"anchor_kind":"call","anchor_symbol":"Callee"}
	]}`))
	if err != nil || !emitted.Success {
		t.Fatalf("actual call extraction failed: err=%v summary=%s", err, emitted.Summary)
	}
	var calls []types.EvidenceItem
	lines := map[int]bool{}
	for _, ev := range bus.Mutable.EmittedEvidence() {
		if ev.Producer == EmitEvidenceProducer && types.ClaimFormOf(ev) == types.ClaimCallEdge {
			if ev.Source != "calls.go" || (ev.LineStart != 4 && ev.LineStart != 5) || ev.LineEnd != ev.LineStart || ev.GroundingStatus != types.GroundingGrounded || !strings.Contains(ev.Snippet, "Callee()") {
				t.Fatalf("unexpected source-grounded call receipt: %+v", ev)
			}
			calls = append(calls, ev)
			lines[ev.LineStart] = true
		}
	}
	if len(calls) != 2 || !lines[4] || !lines[5] || calls[0].ID == calls[1].ID {
		t.Fatalf("need two independent actual call-site receipts: %+v", bus.Mutable.EmittedEvidence())
	}
	return calls, repo
}
