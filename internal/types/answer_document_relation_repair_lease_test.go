package types

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestAnswerDiagramRelationRepairLease_LocalDeltaOnly(t *testing.T) {
	bad := DiagramEdgeAnchor{
		FromNode: "Analyze", ToNode: "Explore", FromIdentity: "runAnalyzePhase", ToIdentity: "runExplorePhase",
		RelationKind: DiagramRelCall, VisibleLabel: "old wording",
	}
	good := DiagramEdgeAnchor{
		FromNode: "Explore", ToNode: "Extract", FromIdentity: "runExplorePhase", ToIdentity: "runExtractPhase",
		RelationKind: DiagramRelCall, VisibleLabel: "old wording",
	}
	base := &AnswerDocumentV2{Blocks: []AnswerBlock{{ID: "flow", EdgeAnchors: []DiagramEdgeAnchor{bad, good}}}}
	lease := NewAnswerDiagramRelationRepairLease(base, []AnswerDiagramRelationRepairFailure{{
		BlockID: "flow", Issue: "call_edge_unproven", RelationKind: DiagramRelCall,
		FromNode: "Analyze", ToNode: "Explore", FromIdentity: "runAnalyzePhase", ToIdentity: "runExplorePhase",
	}}, []AnswerDiagramRelationRepairCandidate{{
		BlockID: "flow", RelationKind: DiagramRelPrecedence,
		FromIdentity: "analyzer", ToIdentity: "explorer", Source: "internal/types/enums.go:120-121",
	}})
	if lease == nil {
		t.Fatal("expected a lease from a valid typed failure")
	}

	t.Run("remove only failed relation and relabel retained relation", func(t *testing.T) {
		relabelled := good
		relabelled.VisibleLabel = "business wording stays model-owned"
		got := ValidateAnswerDiagramRelationRepairLease(lease, &AnswerDocumentV2{Blocks: []AnswerBlock{{
			ID: "flow", EdgeAnchors: []DiagramEdgeAnchor{relabelled},
		}}})
		if len(got) != 0 {
			t.Fatalf("local removal plus display relabel must pass: %+v", got)
		}
	})

	t.Run("correct failed relation on same endpoint pair", func(t *testing.T) {
		corrected := bad
		corrected.FromNode, corrected.ToNode = corrected.ToNode, corrected.FromNode
		corrected.FromIdentity, corrected.ToIdentity = corrected.ToIdentity, corrected.FromIdentity
		got := ValidateAnswerDiagramRelationRepairLease(lease, &AnswerDocumentV2{Blocks: []AnswerBlock{{
			ID: "flow", EdgeAnchors: []DiagramEdgeAnchor{corrected, good},
		}}})
		if len(got) != 0 {
			t.Fatalf("same-pair direction correction must remain model-owned: %+v", got)
		}
	})

	t.Run("removing unlisted relation is rejected", func(t *testing.T) {
		got := ValidateAnswerDiagramRelationRepairLease(lease, &AnswerDocumentV2{Blocks: []AnswerBlock{{ID: "flow"}}})
		if len(got) != 1 || got[0].Issue != "unlisted_relation_removed" || got[0].FromNode != "Explore" {
			t.Fatalf("expected precise unlisted removal, got %+v", got)
		}
	})

	t.Run("adding unrelated relation is rejected", func(t *testing.T) {
		unrelated := DiagramEdgeAnchor{FromNode: "Extract", ToNode: "Finalize", RelationKind: DiagramRelCall}
		got := ValidateAnswerDiagramRelationRepairLease(lease, &AnswerDocumentV2{Blocks: []AnswerBlock{{
			ID: "flow", EdgeAnchors: []DiagramEdgeAnchor{good, unrelated},
		}}})
		if len(got) != 1 || got[0].Issue != "unlisted_relation_added" || got[0].FromNode != "Extract" {
			t.Fatalf("expected precise unlisted addition, got %+v", got)
		}
	})

	t.Run("listed typed candidate may be selected in the same patch", func(t *testing.T) {
		candidate := DiagramEdgeAnchor{
			FromNode: "AnalyzeStage", ToNode: "ExploreStage",
			FromIdentity: "analyzer", ToIdentity: "explorer",
			RelationKind: DiagramRelPrecedence, VisibleLabel: "model-owned wording",
		}
		got := ValidateAnswerDiagramRelationRepairLease(lease, &AnswerDocumentV2{Blocks: []AnswerBlock{{
			ID: "flow", EdgeAnchors: []DiagramEdgeAnchor{good, candidate},
		}}})
		if len(got) != 0 {
			t.Fatalf("producer-listed typed candidate must be selectable without an empty intermediate graph: %+v", got)
		}
	})

	t.Run("listed typed candidate cannot be duplicated", func(t *testing.T) {
		candidate := DiagramEdgeAnchor{
			FromNode: "AnalyzeStage", ToNode: "ExploreStage",
			FromIdentity: "analyzer", ToIdentity: "explorer",
			RelationKind: DiagramRelPrecedence,
		}
		got := ValidateAnswerDiagramRelationRepairLease(lease, &AnswerDocumentV2{Blocks: []AnswerBlock{{
			ID: "flow", EdgeAnchors: []DiagramEdgeAnchor{good, candidate, candidate},
		}}})
		found := false
		for _, violation := range got {
			if violation.Issue == "listed_relation_expanded" {
				found = true
			}
		}
		if !found {
			t.Fatalf("listed candidate is a one-row permission, got %+v", got)
		}
	})

	t.Run("retaining failure cannot expand same pair", func(t *testing.T) {
		duplicate := bad
		duplicate.RelationKind = DiagramRelAssignment
		got := ValidateAnswerDiagramRelationRepairLease(lease, &AnswerDocumentV2{Blocks: []AnswerBlock{{
			ID: "flow", EdgeAnchors: []DiagramEdgeAnchor{bad, duplicate, good},
		}}})
		if len(got) != 1 || got[0].Issue != "failed_relation_expanded" {
			t.Fatalf("expected no-expansion violation, got %+v", got)
		}
	})
}

func TestAnswerDiagramRelationRepairLeaseDefersOnlyProducerNamedOrdinaryBlocks(t *testing.T) {
	bad := DiagramEdgeAnchor{
		FromNode: "A", ToNode: "B", FromIdentity: "pkg.A", ToIdentity: "pkg.B",
		RelationKind: DiagramRelCall,
	}
	base := &AnswerDocumentV2{Blocks: []AnswerBlock{
		{ID: "diagram", Kind: BlockDiagram, EdgeAnchors: []DiagramEdgeAnchor{bad}},
		{ID: "chain", Kind: BlockOrderedList, SurfaceRole: SurfacePrincipal},
		{ID: "other", Kind: BlockTable},
	}}
	newLease := func() *AnswerDiagramRelationRepairLease {
		return NewAnswerDiagramRelationRepairLease(base, []AnswerDiagramRelationRepairFailure{{
			BlockID: "diagram", Issue: "call_edge_unproven", RelationKind: DiagramRelCall,
			FromNode: "A", ToNode: "B", FromIdentity: "pkg.A", ToIdentity: "pkg.B",
		}}, nil)
	}
	chainAnchor := DiagramEdgeAnchor{
		FromNode: "entry", ToNode: "worker", FromIdentity: "Entry.run", ToIdentity: "Worker.handle",
		RelationKind: DiagramRelCall, VisibleLabel: "调用 worker",
	}
	merged := &AnswerDocumentV2{Blocks: []AnswerBlock{
		{ID: "diagram", Kind: BlockDiagram},
		{ID: "chain", Kind: BlockOrderedList, SurfaceRole: SurfacePrincipal, EdgeAnchors: []DiagramEdgeAnchor{chainAnchor}},
		{ID: "other", Kind: BlockTable},
	}}

	withoutGrant := newLease()
	if got := ValidateAnswerDiagramRelationRepairLease(withoutGrant, merged); len(got) != 1 ||
		got[0].BlockID != "chain" || got[0].Issue != "unlisted_relation_added" {
		t.Fatalf("ordinary sibling addition must remain frozen without a producer grant: %+v", got)
	}

	lease := newLease()
	if !BindAnswerDiagramRelationRepairOrdinaryValidationBlocks(lease, base, []string{"chain"}) {
		t.Fatal("exact unique ordinary block must bind to the lease")
	}
	if got := ValidateAnswerDiagramRelationRepairLease(lease, merged); len(got) != 0 {
		t.Fatalf("producer-named sibling must defer to ordinary relation validation: %+v", got)
	}

	unlisted := *merged
	unlisted.Blocks = append([]AnswerBlock(nil), merged.Blocks...)
	unlisted.Blocks[2].EdgeAnchors = []DiagramEdgeAnchor{{
		FromNode: "x", ToNode: "y", FromIdentity: "X", ToIdentity: "Y", RelationKind: DiagramRelCall,
	}}
	if got := ValidateAnswerDiagramRelationRepairLease(lease, &unlisted); len(got) != 1 ||
		got[0].BlockID != "other" || got[0].Issue != "unlisted_relation_added" {
		t.Fatalf("grant for one sibling must not thaw another block: %+v", got)
	}

	wrongKind := *merged
	wrongKind.Blocks = append([]AnswerBlock(nil), merged.Blocks...)
	wrongKind.Blocks[1].Kind = BlockSummary
	if got := ValidateAnswerDiagramRelationRepairLease(lease, &wrongKind); len(got) != 1 ||
		got[0].BlockID != "chain" || got[0].Issue != "unlisted_relation_added" {
		t.Fatalf("ordinary grant must not authorize a kind-changed generic carrier: %+v", got)
	}

	for name, ids := range map[string][]string{
		"diagram": {"diagram"},
		"missing": {"missing"},
		"empty":   {""},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := newLease()
			if BindAnswerDiagramRelationRepairOrdinaryValidationBlocks(candidate, base, ids) {
				t.Fatalf("invalid ordinary block grant must fail closed: ids=%v lease=%+v", ids, candidate)
			}
		})
	}
}

func TestAnswerDiagramRelationRepairLeaseAdditionRefsAreStableAndBaseBound(t *testing.T) {
	base := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID: "flow", Kind: BlockDiagram,
		EdgeAnchors: []DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "B", FromIdentity: "Caller", ToIdentity: "Callee",
			RelationKind: DiagramRelCall,
		}},
	}}}
	failures := []AnswerDiagramRelationRepairFailure{{
		BlockID: "flow", Issue: "call_edge_unproven", RelationKind: DiagramRelCall,
		FromNode: "A", ToNode: "B", FromIdentity: "Caller", ToIdentity: "Callee",
	}}
	candidates := []AnswerDiagramRelationRepairCandidate{{
		AdditionRef: "producer-must-not-survive",
		BlockID:     "flow", RelationKind: DiagramRelPrecedence,
		FromIdentity: "analyzer", ToIdentity: "explorer", Source: "stageauthority",
	}}
	first := NewAnswerDiagramRelationRepairLease(base, failures, candidates)
	second := NewAnswerDiagramRelationRepairLease(base, failures, candidates)
	if first == nil || second == nil || len(first.AllowedAdditions) != 1 || len(second.AllowedAdditions) != 1 {
		t.Fatalf("expected one live allowed addition: first=%+v second=%+v", first, second)
	}
	ref := first.AllowedAdditions[0].AdditionRef
	if !strings.HasPrefix(ref, "ra1-") || ref == "producer-must-not-survive" ||
		ref != second.AllowedAdditions[0].AdditionRef {
		t.Fatalf("same typed base must mint one stable lease-owned selector: first=%q second=%q", ref, second.AllowedAdditions[0].AdditionRef)
	}

	changed := *base
	changed.Blocks = append([]AnswerBlock(nil), base.Blocks...)
	changed.Blocks[0].EdgeAnchors = append([]DiagramEdgeAnchor(nil), base.Blocks[0].EdgeAnchors...)
	changed.Blocks[0].EdgeAnchors[0].ToIdentity = "DifferentCallee"
	third := NewAnswerDiagramRelationRepairLease(&changed, failures, candidates)
	if third == nil || third.AllowedAdditions[0].AdditionRef == ref {
		t.Fatalf("a changed typed base must invalidate the prior addition ref: old=%q new=%+v", ref, third)
	}
}

func TestAnswerDiagramRelationRepairLeaseSupportsAdditionOnlyGeneration(t *testing.T) {
	base := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID: "flow", Kind: BlockDiagram,
		Diagram: &AnswerDiagramBlock{Kind: DiagramFlow, Language: "mermaid", Body: "flowchart LR\n B[BusContext]"},
	}}}
	candidate := AnswerDiagramRelationRepairCandidate{
		BlockID: "flow", RelationKind: DiagramRelArgumentFlow,
		FromIdentity: "o.busCtx", ToIdentity: "ctxbuilder.BuildAgentContext", Source: "internal/orchestrator/extract_work.go:15",
	}
	lease := NewAnswerDiagramRelationRepairLease(base, nil, []AnswerDiagramRelationRepairCandidate{candidate})
	if lease == nil || len(lease.Failures) != 0 || len(lease.AllowedAdditions) != 1 ||
		!strings.HasPrefix(lease.AllowedAdditions[0].AdditionRef, "ra1-") {
		t.Fatalf("addition-only participant generation must mint one live capability: %+v", lease)
	}
	merged := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID: "flow", Kind: BlockDiagram,
		Diagram: &AnswerDiagramBlock{Kind: DiagramFlow, Language: "mermaid", Body: "flowchart LR\n B[BusContext]\n B --> C[Build context]"},
		EdgeAnchors: []DiagramEdgeAnchor{{
			FromNode: "B", ToNode: "C", FromIdentity: candidate.FromIdentity, ToIdentity: candidate.ToIdentity,
			RelationKind: candidate.RelationKind, VisibleLabel: "作为参数传递",
		}},
	}}}
	if got := ValidateAnswerDiagramRelationRepairLease(lease, merged); len(got) != 0 {
		t.Fatalf("the selected current-generation typed addition must pass: %+v", got)
	}

	unlisted := *merged
	unlisted.Blocks = append([]AnswerBlock(nil), merged.Blocks...)
	unlisted.Blocks[0].EdgeAnchors = append(unlisted.Blocks[0].EdgeAnchors, DiagramEdgeAnchor{
		FromNode: "X", ToNode: "Y", FromIdentity: "Unknown.X", ToIdentity: "Unknown.Y", RelationKind: DiagramRelCall,
	})
	if got := ValidateAnswerDiagramRelationRepairLease(lease, &unlisted); len(got) != 1 || got[0].Issue != "unlisted_relation_added" {
		t.Fatalf("addition-only lease must still reject every unlisted relation: %+v", got)
	}
	if got := NewAnswerDiagramRelationRepairLease(base, nil, nil); got != nil {
		t.Fatalf("an empty capability set must never create a hard lease: %+v", got)
	}
	missingCarrier := candidate
	missingCarrier.BlockID = "not-in-base"
	if got := NewAnswerDiagramRelationRepairLease(base, nil, []AnswerDiagramRelationRepairCandidate{missingCarrier}); got != nil {
		t.Fatalf("an addition capability must bind one exact existing carrier: %+v", got)
	}
}

func TestAnswerDiagramRelationRepairLeaseNeverMintsAdditionForExistingCanonicalTuple(t *testing.T) {
	existing := DiagramEdgeAnchor{
		FromNode: "A", ToNode: "B", FromIdentity: "analyzer",
		ToIdentity: "explorer", RelationKind: DiagramRelPrecedence,
	}
	base := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID: "flow", Kind: BlockDiagram,
		Diagram:     &AnswerDiagramBlock{Kind: DiagramFlow, Language: "mermaid", Body: "flowchart LR\n A --> B"},
		EdgeAnchors: []DiagramEdgeAnchor{existing},
	}}}
	candidate := AnswerDiagramRelationRepairCandidate{
		BlockID: "flow", RelationKind: DiagramRelPrecedence,
		FromIdentity: "analyzer", ToIdentity: "explorer", Source: "stageauthority",
	}
	if got := NewAnswerDiagramRelationRepairLease(base, nil, []AnswerDiagramRelationRepairCandidate{candidate}); got != nil {
		t.Fatalf("an additions-only lease must disappear when its tuple already exists: %+v", got)
	}

	failure := AnswerDiagramRelationRepairFailure{
		BlockID: "flow", Issue: "call_edge_unproven", RelationKind: DiagramRelPrecedence,
		FromNode: "A", ToNode: "B", FromIdentity: "analyzer", ToIdentity: "explorer",
	}
	lease := NewAnswerDiagramRelationRepairLease(base, []AnswerDiagramRelationRepairFailure{failure}, []AnswerDiagramRelationRepairCandidate{candidate})
	if lease == nil || len(lease.Failures) != 1 || len(lease.AllowedAdditions) != 0 {
		t.Fatalf("an independent failure lease may survive, but its duplicate add capability must not: %+v", lease)
	}
}

func TestMergeAnswerDiagramRelationRepairDeltaJSONKeepsAdditionOnlyCapability(t *testing.T) {
	raw := `{"version":1,"failures":[],"preserve_unlisted_edges":true,"allowed_additions":[{"block_id":"flow","relation_kind":"argument_flow","from_identity":"o.busCtx","to_identity":"ctxbuilder.BuildAgentContext","source":"internal/orchestrator/extract_work.go:15"}]}`
	merged := MergeAnswerDiagramRelationRepairDeltaJSON([]string{raw})
	var delta AnswerDiagramRelationRepairDelta
	if err := json.Unmarshal([]byte(merged), &delta); err != nil || len(delta.Failures) != 0 || len(delta.AllowedAdditions) != 1 {
		t.Fatalf("addition-only delta must survive the typed merge: err=%v delta=%+v raw=%s", err, delta, merged)
	}
}

func TestAnswerDiagramRelationRepairLeasePreservesCompleteUnion(t *testing.T) {
	first := DiagramEdgeAnchor{
		FromNode: "Phase", ToNode: "Disp", FromIdentity: "Phase", ToIdentity: "Disp",
		RelationKind: DiagramRelCall,
	}
	second := DiagramEdgeAnchor{
		FromNode: "Disp", ToNode: "Bus", FromIdentity: "Disp", ToIdentity: "Bus",
		RelationKind: DiagramRelCall,
	}
	base := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID: "flow", Kind: BlockDiagram, EdgeAnchors: []DiagramEdgeAnchor{first, second},
	}}}
	failures := []AnswerDiagramRelationRepairFailure{
		{BlockID: "flow", Issue: "call_edge_unproven", RelationKind: DiagramRelCall, FromNode: "Phase", ToNode: "Disp", FromIdentity: "Phase", ToIdentity: "Disp"},
		{BlockID: "flow", Issue: "call_edge_unproven", RelationKind: DiagramRelCall, FromNode: "Disp", ToNode: "Bus", FromIdentity: "Disp", ToIdentity: "Bus"},
	}
	additions := make([]AnswerDiagramRelationRepairCandidate, 0, 10)
	for i := 0; i < 10; i++ {
		additions = append(additions, AnswerDiagramRelationRepairCandidate{
			BlockID: "flow", RelationKind: DiagramRelPrecedence,
			FromIdentity: fmt.Sprintf("Stage%d", i), ToIdentity: fmt.Sprintf("Stage%d", i+1),
			Source: fmt.Sprintf("internal/types/stage_binding.go:%d", i+1),
		})
	}
	lease := NewAnswerDiagramRelationRepairLease(base, failures, additions)
	if lease == nil || len(lease.Failures) != 2 || len(lease.AllowedAdditions) != 10 {
		t.Fatalf("lease must preserve the complete normalized union without first-eight truncation: %+v", lease)
	}
	got := ValidateAnswerDiagramRelationRepairLease(lease, &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID: "flow", Kind: BlockDiagram,
	}}})
	for _, violation := range got {
		if violation.Issue == "unlisted_relation_removed" {
			t.Fatalf("removing every same-cycle named failure must not be rejected as unlisted: %+v", got)
		}
	}
}

func TestAnswerDiagramRelationRepairLeasePublishesExecutableFailureCapabilities(t *testing.T) {
	base := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID: "flow", Kind: BlockDiagram,
		Diagram: &AnswerDiagramBlock{Kind: DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n    A->>B: call\n    B->>C: result\n"},
		EdgeAnchors: []DiagramEdgeAnchor{
			{FromNode: "A", ToNode: "B", RelationKind: DiagramRelCall},
			{FromNode: "C", ToNode: "D", RelationKind: DiagramRelPrecedence},
		},
	}}}
	lease := NewAnswerDiagramRelationRepairLease(base, []AnswerDiagramRelationRepairFailure{
		{BlockID: "flow", Issue: "call_edge_unproven", RelationKind: DiagramRelCall, FromNode: "A", ToNode: "B"},
		{BlockID: "flow", Issue: DiagramRelationFailureMissingGroundedCallAnchor, RelationKind: DiagramRelCall, FromNode: "B", ToNode: "C", FromIdentity: "B.run", ToIdentity: "C.run"},
		{BlockID: "flow", Issue: "typed_anchor_without_visible_edge", RelationKind: DiagramRelPrecedence, FromNode: "C", ToNode: "D"},
	}, nil)
	if lease == nil || len(lease.Failures) != 3 {
		t.Fatalf("expected three executable failure rows: %+v", lease)
	}
	got := make(map[string]AnswerDiagramRelationRepairFailure, len(lease.Failures))
	for _, failure := range lease.Failures {
		got[failure.Issue] = failure
		if failure.FailureRef == "" {
			t.Fatalf("capability row missing opaque ref: %+v", failure)
		}
	}
	assertCapability := func(issue string, carrier AnswerDiagramRelationRepairTargetCarrier, actions ...AnswerDiagramRelationRepairAction) {
		t.Helper()
		failure := got[issue]
		if failure.TargetCarrier != carrier || fmt.Sprint(failure.AllowedActions) != fmt.Sprint(actions) {
			t.Fatalf("issue=%s capability=%s/%v want=%s/%v", issue, failure.TargetCarrier, failure.AllowedActions, carrier, actions)
		}
	}
	assertCapability("call_edge_unproven", AnswerDiagramRelationRepairCarrierPriorAnchor,
		AnswerDiagramRelationRepairActionRemove)
	assertCapability(DiagramRelationFailureMissingGroundedCallAnchor, AnswerDiagramRelationRepairCarrierVisibleBodyEdge,
		AnswerDiagramRelationRepairActionRemove, AnswerDiagramRelationRepairActionReplace)
	assertCapability("typed_anchor_without_visible_edge", AnswerDiagramRelationRepairCarrierStaleAnchor,
		AnswerDiagramRelationRepairActionRemove, AnswerDiagramRelationRepairActionReplace)
}

func TestAnswerDiagramRelationRepairLeasePairsExistingBodyWithExactTypedCandidate(t *testing.T) {
	base := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID: "flow", Kind: BlockDiagram,
		Diagram: &AnswerDiagramBlock{Kind: DiagramFlow, Language: "mermaid", Body: "flowchart LR\n A -->|model wording| B\n"},
	}}}
	failure := AnswerDiagramRelationRepairFailure{
		BlockID: "flow", Issue: "missing_relation_anchor", FromNode: "A", ToNode: "B",
		FromIdentity: "pkg.Source.run", ToIdentity: "pkg.Target.accept", BodyOccurrence: 1,
	}
	candidate := AnswerDiagramRelationRepairCandidate{
		BlockID: "flow", RelationKind: DiagramRelArgumentFlow,
		FromIdentity: "pkg.Source.run", ToIdentity: "pkg.Target.accept", Source: "internal/source.go:10",
	}
	lease := NewAnswerDiagramRelationRepairLease(base, []AnswerDiagramRelationRepairFailure{failure}, []AnswerDiagramRelationRepairCandidate{candidate})
	if lease == nil || len(lease.Failures) != 1 || len(lease.AllowedAdditions) != 1 {
		t.Fatalf("expected one paired live capability: %+v", lease)
	}
	got := lease.Failures[0]
	if got.TargetCarrier != AnswerDiagramRelationRepairCarrierVisibleBodyEdge ||
		!got.AllowsAction("remove") || !got.AllowsAction("attach") || got.AllowsAction("replace") {
		t.Fatalf("untyped visible body must publish remove+attach, not an impossible replace: %+v", got)
	}
	if !AnswerDiagramRelationRepairFailureCanAttachCandidate(got, lease.AllowedAdditions[0]) {
		t.Fatalf("the exact same-direction typed candidate must pair with the visible occurrence: failure=%+v candidate=%+v", got, lease.AllowedAdditions[0])
	}

	reversed := lease.AllowedAdditions[0]
	reversed.FromIdentity, reversed.ToIdentity = reversed.ToIdentity, reversed.FromIdentity
	if AnswerDiagramRelationRepairFailureCanAttachCandidate(got, reversed) {
		t.Fatal("reversed typed identities must not authorize attach")
	}
}

func TestAnswerDiagramRelationRepairLeaseRejectsAmbiguousFailureWithoutExecutableExit(t *testing.T) {
	base := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID: "flow", Kind: BlockDiagram,
		EdgeAnchors: []DiagramEdgeAnchor{
			{FromNode: "A", ToNode: "B", FromIdentity: "First", ToIdentity: "One", RelationKind: DiagramRelCall},
			{FromNode: "A", ToNode: "B", FromIdentity: "Second", ToIdentity: "Two", RelationKind: DiagramRelCall},
		},
	}}}
	lease := NewAnswerDiagramRelationRepairLease(base, []AnswerDiagramRelationRepairFailure{{
		BlockID: "flow", Issue: "call_edge_unproven", RelationKind: DiagramRelCall,
		FromNode: "A", ToNode: "B", FromIdentity: "Resolved", ToIdentity: "Unknown",
	}}, nil)
	if lease != nil {
		t.Fatalf("an ambiguous diagnostic with no local action must not mint a dead lease: %+v", lease)
	}
	optional := NewAnswerDiagramRelationRepairLeaseWithTargetRemoval(base, []AnswerDiagramRelationRepairFailure{{
		BlockID: "flow", Issue: "call_edge_unproven", RelationKind: DiagramRelCall,
		FromNode: "A", ToNode: "B", FromIdentity: "Resolved", ToIdentity: "Unknown",
	}}, nil, true)
	if optional == nil || !optional.AllowTargetDiagramRemoval ||
		!AnswerDiagramRelationRepairLeaseIsLocallyExecutable(optional) {
		t.Fatalf("an optional diagram's exact removal branch must be an executable model-owned exit: %+v", optional)
	}
}

func TestAnswerDiagramRelationRepairLeaseCoalescesSeveralIssuesOnOneCarrier(t *testing.T) {
	base := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID: "flow", Kind: BlockDiagram,
		EdgeAnchors: []DiagramEdgeAnchor{{
			FromNode: "IR", ToNode: "Bus", FromIdentity: "AnalysisIR", ToIdentity: "BusContext",
			RelationKind: DiagramRelAssignment,
		}},
	}}}
	lease := NewAnswerDiagramRelationRepairLease(base, []AnswerDiagramRelationRepairFailure{
		{BlockID: "flow", Issue: "assignment_edge_unproven", RelationKind: DiagramRelAssignment, FromNode: "IR", ToNode: "Bus", FromIdentity: "AnalysisIR", ToIdentity: "BusContext"},
		{BlockID: "flow", Issue: "sequence_relation_reply_operator_conflict", RelationKind: DiagramRelAssignment, FromNode: "IR", ToNode: "Bus", FromIdentity: "AnalysisIR", ToIdentity: "BusContext"},
	}, nil)
	if lease == nil || len(lease.Failures) != 1 {
		t.Fatalf("one carrier with two validator issues must publish one executable ref: %+v", lease)
	}
	failure := lease.Failures[0]
	if len(failure.RelatedIssues) != 2 || failure.TargetCarrier != AnswerDiagramRelationRepairCarrierPriorAnchor ||
		!failure.AllowsAction("remove") || failure.AllowsAction("replace") {
		t.Fatalf("coalesced carrier lost issue or action authority: %+v", failure)
	}
}

func TestAnswerDiagramRelationRepairLeaseEvidenceNegativeLabelCarrierNarrowsToRemoveOnly(t *testing.T) {
	base := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID: "flow", Kind: BlockDiagram,
		EdgeAnchors: []DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "B", FromIdentity: "Caller.Run", ToIdentity: "Callee.Handle",
			RelationKind: DiagramRelCall, VisibleLabel: "model label",
		}},
	}}}
	lease := NewAnswerDiagramRelationRepairLease(base, []AnswerDiagramRelationRepairFailure{
		{BlockID: "flow", Issue: "diagram_visible_label_mismatch", RelationKind: DiagramRelCall, FromNode: "A", ToNode: "B", FromIdentity: "Caller.Run", ToIdentity: "Callee.Handle"},
		{BlockID: "flow", Issue: "call_edge_unproven", RelationKind: DiagramRelCall, FromNode: "A", ToNode: "B", FromIdentity: "Caller.Run", ToIdentity: "Callee.Handle"},
	}, nil)
	if lease == nil || len(lease.Failures) != 1 {
		t.Fatalf("same carrier issues must compile to one capability: %+v", lease)
	}
	got := lease.Failures[0]
	if got.TargetCarrier != AnswerDiagramRelationRepairCarrierPriorAnchor || !got.AllowsAction("remove") ||
		got.AllowsAction("replace") || got.AllowsAction("relabel") {
		t.Fatalf("evidence-negative sibling must narrow the merged carrier to remove-only: %+v", got)
	}
}

func TestAnswerDiagramRelationRepairLeaseSkipsCaseEquivalentExistingAddition(t *testing.T) {
	base := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID: "flow", Kind: BlockDiagram,
		EdgeAnchors: []DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: DiagramRelPrecedence,
		}},
	}}}
	candidate := AnswerDiagramRelationRepairCandidate{
		BlockID: "flow", RelationKind: DiagramRelPrecedence,
		FromIdentity: "analyzer", ToIdentity: "explorer", Source: "typed-stage-authority",
	}
	if lease := NewAnswerDiagramRelationRepairLease(base, nil, []AnswerDiagramRelationRepairCandidate{candidate}); lease != nil {
		t.Fatalf("case-equivalent canonical relation must not be re-published as a new edge: %+v", lease)
	}
	reversed := candidate
	reversed.FromIdentity, reversed.ToIdentity = reversed.ToIdentity, reversed.FromIdentity
	if lease := NewAnswerDiagramRelationRepairLease(base, nil, []AnswerDiagramRelationRepairCandidate{reversed}); lease == nil || len(lease.AllowedAdditions) != 1 {
		t.Fatalf("direction remains semantic and must not be collapsed by case normalization: %+v", lease)
	}
}

func TestAnswerDiagramRelationRepairFailureRefsAreStableAndCarrierBound(t *testing.T) {
	base := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID: "flow", Kind: BlockDiagram,
		Diagram: &AnswerDiagramBlock{Kind: DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n    A->>B: visible\n"},
		EdgeAnchors: []DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: DiagramRelPrecedence, VisibleLabel: "visible",
		}},
	}}}
	failure := AnswerDiagramRelationRepairFailure{
		BlockID: "flow", Issue: "typed_anchor_without_visible_edge",
		FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
		RelationKind: DiagramRelPrecedence,
	}
	first := AssignAnswerDiagramRelationRepairFailureRefs(base, []AnswerDiagramRelationRepairFailure{failure})
	second := AssignAnswerDiagramRelationRepairFailureRefs(base, []AnswerDiagramRelationRepairFailure{failure})
	if len(first) != 1 || first[0].FailureRef == "" || first[0].FailureRef != second[0].FailureRef {
		t.Fatalf("same rejected carrier must produce one stable opaque ref: first=%+v second=%+v", first, second)
	}
	changed := *base
	changed.Blocks = append([]AnswerBlock(nil), base.Blocks...)
	changed.Blocks[0].EdgeAnchors = append([]DiagramEdgeAnchor(nil), base.Blocks[0].EdgeAnchors...)
	changed.Blocks[0].EdgeAnchors[0].FromIdentity = "ChangedAnalyzer"
	third := AssignAnswerDiagramRelationRepairFailureRefs(&changed, []AnswerDiagramRelationRepairFailure{failure})
	if third[0].FailureRef == first[0].FailureRef {
		t.Fatalf("changed carrier must invalidate an old ref: old=%s new=%s", first[0].FailureRef, third[0].FailureRef)
	}
	lease := NewAnswerDiagramRelationRepairLease(base, first, nil)
	if lease == nil || len(lease.Failures) != 1 || lease.Failures[0].FailureRef != first[0].FailureRef {
		t.Fatalf("live lease must retain the ref bound to its exact base: %+v", lease)
	}
}

func TestAnswerDiagramRelationRepairLeaseIdentityOnlyFailureCanGainNodes(t *testing.T) {
	identityOnly := DiagramEdgeAnchor{
		FromIdentity: "analyzer", ToIdentity: "explorer", RelationKind: DiagramRelPrecedence,
	}
	sibling := DiagramEdgeAnchor{
		FromNode: "Explore", ToNode: "Extract", FromIdentity: "explorer", ToIdentity: "extractor",
		RelationKind: DiagramRelPrecedence,
	}
	base := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID: "flow", Kind: BlockDiagram, EdgeAnchors: []DiagramEdgeAnchor{identityOnly, sibling},
	}}}
	failure := AnswerDiagramRelationRepairFailure{
		BlockID: "flow", Issue: "typed_anchor_without_visible_edge",
		RelationKind: DiagramRelPrecedence, FromIdentity: "analyzer", ToIdentity: "explorer",
	}
	if !AnswerDiagramRelationRepairFailureHasCompleteLocator(failure) {
		t.Fatal("complete technical identity pair must be a precise repair locator")
	}
	lease := NewAnswerDiagramRelationRepairLease(base, []AnswerDiagramRelationRepairFailure{failure}, nil)
	if lease == nil {
		t.Fatal("identity-only failure must install a typed lease")
	}
	withNodes := identityOnly
	withNodes.FromNode, withNodes.ToNode = "Analyze", "Explore"
	got := ValidateAnswerDiagramRelationRepairLease(lease, &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID: "flow", Kind: BlockDiagram, EdgeAnchors: []DiagramEdgeAnchor{withNodes, sibling},
	}}})
	if len(got) != 0 {
		t.Fatalf("the model may add node ids to the same listed technical relation: %+v", got)
	}

	halfNode := failure
	halfNode.FromNode = "Analyze"
	if AnswerDiagramRelationRepairFailureHasCompleteLocator(halfNode) ||
		NewAnswerDiagramRelationRepairLease(base, []AnswerDiagramRelationRepairFailure{halfNode}, nil) != nil {
		t.Fatal("a half-present node pair must remain malformed even with complete identities")
	}
	halfIdentity := AnswerDiagramRelationRepairFailure{
		BlockID: "flow", Issue: "call_edge_unproven", FromNode: "A", ToNode: "B", FromIdentity: "only-one-side",
	}
	if AnswerDiagramRelationRepairFailureHasCompleteLocator(halfIdentity) {
		t.Fatal("a half-present identity pair must remain malformed even with complete nodes")
	}
}

func TestAnswerDiagramRelationRepairFailureCanRemoveVisibleBodyOccurrence(t *testing.T) {
	base := AnswerDiagramRelationRepairFailure{
		FailureRef: "rf-live", BlockID: "diag", FromNode: "A", ToNode: "B",
		TargetCarrier:  AnswerDiagramRelationRepairCarrierVisibleBodyEdge,
		AllowedActions: []AnswerDiagramRelationRepairAction{AnswerDiagramRelationRepairActionRemove},
		BodyOccurrence: 1,
	}
	if !base.CanRemoveVisibleBodyOccurrence("diag", "A", "B", 1, 2) {
		t.Fatal("an exact visible-body occurrence must be removable")
	}
	if base.CanRemoveVisibleBodyOccurrence("diag", "A", "B", 2, 2) {
		t.Fatal("one exact ref must not cover a sibling occurrence")
	}

	zeroOccurrence := base
	zeroOccurrence.BodyOccurrence = 0
	if !zeroOccurrence.CanRemoveVisibleBodyOccurrence("diag", "A", "B", 1, 1) {
		t.Fatal("one occurrence-agnostic ref is safe for a unique pair")
	}
	if zeroOccurrence.CanRemoveVisibleBodyOccurrence("diag", "A", "B", 1, 2) {
		t.Fatal("one occurrence-agnostic ref must not cover a repeated pair")
	}

	for _, carrier := range []AnswerDiagramRelationRepairTargetCarrier{
		AnswerDiagramRelationRepairCarrierStaleAnchor,
		AnswerDiagramRelationRepairCarrierPriorAnchorMetadata,
		AnswerDiagramRelationRepairCarrierLabelPair,
	} {
		metadataOnly := base
		metadataOnly.TargetCarrier = carrier
		if metadataOnly.CanRemoveVisibleBodyOccurrence("diag", "A", "B", 1, 1) {
			t.Fatalf("carrier %q cannot authorize visible-body orphan cleanup", carrier)
		}
	}

	priorAnchor := base
	priorAnchor.TargetCarrier = AnswerDiagramRelationRepairCarrierPriorAnchor
	if !priorAnchor.CanRemoveVisibleBodyOccurrence("diag", "A", "B", 1, 1) {
		t.Fatal("a unique prior-anchor remove deletes its visible occurrence too")
	}
}

func TestMutableState_AnswerDiagramRelationRepairLeaseLifecycle(t *testing.T) {
	m := NewMutableState("test")
	lease := &AnswerDiagramRelationRepairLease{Version: 1, Failures: []AnswerDiagramRelationRepairFailure{{
		BlockID: "flow", Issue: "call_edge_unproven", FromNode: "A", ToNode: "B",
	}}, OptionalOrphanCleanups: []AnswerDiagramOrphanCleanupCandidate{{
		BlockID: "flow", ParticipantID: "A",
		AllowedActions: []AnswerDiagramOrphanDispositionAction{
			AnswerDiagramOrphanDispositionRemove, AnswerDiagramOrphanDispositionRetain,
		},
	}}}
	m.SetAnswerDiagramRelationRepairLease(lease)
	got := m.AnswerDiagramRelationRepairLease()
	if got == nil || len(got.Failures) != 1 {
		t.Fatalf("lease was not retained: %+v", got)
	}
	got.Failures[0].FromNode = "mutated"
	got.OptionalOrphanCleanups[0].AllowedActions[0] = "mutated"
	if fresh := m.AnswerDiagramRelationRepairLease(); fresh.Failures[0].FromNode != "A" ||
		fresh.OptionalOrphanCleanups[0].AllowedActions[0] != AnswerDiagramOrphanDispositionRemove {
		t.Fatal("lease getter must return a defensive copy")
	}
	m.SetAnswerDocumentV2WithMutation(MutationReplaceAll, &AnswerDocumentV2{DocumentModel: "v2"})
	if got := m.AnswerDiagramRelationRepairLease(); got != nil {
		t.Fatalf("accepted emit must clear retry-local lease: %+v", got)
	}
}

func TestMutableState_PendingAnswerDocumentPatchBaseLifecycle(t *testing.T) {
	m := NewMutableState("test")
	staged := &AnswerDocumentV2{DocumentModel: "v2", Blocks: []AnswerBlock{{
		ID: "summary", Kind: BlockSummary, Text: "staged",
	}}}
	m.SetPendingAnswerDocumentPatchBase(staged)
	staged.Blocks[0].Text = "caller mutation"
	got := m.PendingAnswerDocumentPatchBase()
	if got == nil || got.Blocks[0].Text != "staged" {
		t.Fatalf("setter must retain a defensive staged copy: %+v", got)
	}
	got.Blocks[0].Text = "reader mutation"
	if fresh := m.PendingAnswerDocumentPatchBase(); fresh == nil || fresh.Blocks[0].Text != "staged" {
		t.Fatalf("getter must return a defensive staged copy: %+v", fresh)
	}

	m.SetAnswerDocumentV2WithMutation(MutationPartial, &AnswerDocumentV2{DocumentModel: "v2"})
	if got := m.PendingAnswerDocumentPatchBase(); got != nil {
		t.Fatalf("accepted patch must clear retry-local staging: %+v", got)
	}
	m.SetPendingAnswerDocumentPatchBase(staged)
	m.ResetAnswerDocumentV2()
	if got := m.PendingAnswerDocumentPatchBase(); got != nil {
		t.Fatalf("task reset must clear retry-local staging: %+v", got)
	}
}

func TestAnswerDiagramRelationRepairLease_FreezesRequiredDiagramCarrier(t *testing.T) {
	base := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID: "flow", Kind: BlockDiagram,
		Diagram:     &AnswerDiagramBlock{Kind: DiagramSequence, Body: "sequenceDiagram"},
		EdgeAnchors: []DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", RelationKind: DiagramRelCall}},
	}}}
	lease := NewAnswerDiagramRelationRepairLease(base, []AnswerDiagramRelationRepairFailure{{
		BlockID: "flow", Issue: "call_edge_unproven", FromNode: "A", ToNode: "B",
	}}, nil)

	tests := []struct {
		name  string
		doc   *AnswerDocumentV2
		issue string
	}{
		{
			name:  "kind changed",
			doc:   &AnswerDocumentV2{Blocks: []AnswerBlock{{ID: "flow", Kind: BlockTable}}},
			issue: "relation_diagram_carrier_kind_changed",
		},
		{
			name:  "carrier removed",
			doc:   &AnswerDocumentV2{Blocks: []AnswerBlock{{ID: "summary", Kind: BlockSummary}}},
			issue: "relation_diagram_carrier_removed",
		},
		{
			name: "second diagram added",
			doc: &AnswerDocumentV2{Blocks: []AnswerBlock{
				{ID: "flow", Kind: BlockDiagram, Diagram: &AnswerDiagramBlock{Kind: DiagramSequence, Body: "sequenceDiagram"}},
				{ID: "flow-2", Kind: BlockDiagram, Diagram: &AnswerDiagramBlock{Kind: DiagramSequence, Body: "sequenceDiagram"}},
			}},
			issue: "relation_diagram_carrier_added",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidateAnswerDiagramRelationRepairLease(lease, tc.doc)
			found := false
			for _, violation := range got {
				if violation.Issue == tc.issue {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected %s, got %+v", tc.issue, got)
			}
		})
	}
}
