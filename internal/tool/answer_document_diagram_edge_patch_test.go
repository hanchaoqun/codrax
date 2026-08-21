package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

func atomicPatchTestDocument() *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: "keep"},
			{
				ID: "diag", Kind: types.BlockDiagram, Title: "model title",
				SurfaceRole: types.SurfacePrincipal,
				FacetIDs:    []string{"diagram_spine"},
				Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n    participant A\n    participant B\n    participant C\n    A->>B: old label\n    B->>C: keep label\n"},
				EdgeAnchors: []types.DiagramEdgeAnchor{
					{FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer", RelationKind: types.DiagramRelPrecedence, VisibleLabel: "old label"},
					{FromNode: "B", ToNode: "C", FromIdentity: "Explorer", ToIdentity: "Extractor", RelationKind: types.DiagramRelPrecedence, VisibleLabel: "keep label"},
				},
			},
		},
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_PreservesUnmentionedGraphContent(t *testing.T) {
	prev := atomicPatchTestDocument()
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{
		{
			BlockID: "diag", Action: "relabel",
			Match:        &types.DiagramEdgeAnchor{FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer", RelationKind: types.DiagramRelPrecedence},
			VisibleLabel: "确定分析范围后收集证据",
		},
	}, []emitAnswerDiagramBoundaryReplacement{{
		BlockID: "diag",
		ParticipantBoundaries: []types.DiagramParticipantBoundary{{
			Participant: "AnswerDocument", Status: types.DiagramParticipantBoundaryUnproven,
		}},
	}}, nil)
	if err != nil {
		t.Fatalf("atomic edit rejected: %v", err)
	}
	if len(patch.ReplaceBlocks) != 1 {
		t.Fatalf("replace blocks=%d, want one compiled replacement", len(patch.ReplaceBlocks))
	}
	got := patch.ReplaceBlocks[0]
	for _, want := range []string{
		"participant A", "participant B", "participant C",
		"A->>B: 确定分析范围后收集证据", "B->>C: keep label",
	} {
		if !strings.Contains(got.Diagram.Body, want) {
			t.Fatalf("compiled body lost %q:\n%s", want, got.Diagram.Body)
		}
	}
	if got.Title != "model title" || got.SurfaceRole != types.SurfacePrincipal || len(got.FacetIDs) != 1 {
		t.Fatalf("unmentioned block fields changed: %+v", got)
	}
	if len(got.EdgeAnchors) != 2 || got.EdgeAnchors[0].VisibleLabel != "确定分析范围后收集证据" || got.EdgeAnchors[1].VisibleLabel != "keep label" {
		t.Fatalf("anchor delta was not local: %+v", got.EdgeAnchors)
	}
	if len(got.ParticipantBoundaries) != 1 || got.ParticipantBoundaries[0].Participant != "AnswerDocument" {
		t.Fatalf("boundary replacement missing: %+v", got.ParticipantBoundaries)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_RemovesTypedFailedAnchorWithoutBody(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    participant A\n    participant B\n    participant C\n    B->>C: keep label\n"
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: diagramCallEdgeIssueAnchorWithoutBodyEdge,
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence,
		}}, nil)
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		BlockID: "diag", Action: "remove",
		Match: &types.DiagramEdgeAnchor{
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence,
		},
	}}, nil, lease)
	if err != nil {
		t.Fatalf("typed stale anchor must be removable without a body edge: %v", err)
	}
	got := patch.ReplaceBlocks[0]
	if strings.Contains(got.Diagram.Body, "A->>B") || !strings.Contains(got.Diagram.Body, "B->>C: keep label") {
		t.Fatalf("anchor-only remove changed Mermaid body:\n%s", got.Diagram.Body)
	}
	if len(got.EdgeAnchors) != 1 || got.EdgeAnchors[0].FromNode != "B" || got.EdgeAnchors[0].ToNode != "C" {
		t.Fatalf("anchor-only remove changed an unmentioned anchor: %+v", got.EdgeAnchors)
	}
}

func TestEmitAnswerDocumentPatch_RemovesTypedFailedAnchorWithoutBodyTransactionally(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    participant A\n    participant B\n    participant C\n    B->>C: keep label\n"
	mut := types.NewMutableState("atomic-anchor-without-body")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	mut.SetAnswerDiagramRelationRepairLease(types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: diagramCallEdgeIssueAnchorWithoutBodyEdge,
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence,
		}}, nil))
	res, err := (&EmitAnswerDocumentPatch{}).Execute(&types.BusContext{Mutable: mut}, json.RawMessage(`{
		"unchanged_block_ids":["summary"],
		"diagram_edge_edits":[{"block_id":"diag","action":"remove","match":{
			"from_node":"A","to_node":"B","from_identity":"Analyzer","to_identity":"Explorer","relation_kind":"precedence"
		}}]
	}`))
	if err != nil || !res.Success {
		t.Fatalf("typed stale-anchor transaction must pass: err=%v res=%+v", err, res)
	}
	got := mut.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 2 || len(got.Blocks[1].EdgeAnchors) != 1 ||
		strings.Contains(got.Blocks[1].Diagram.Body, "A->>B") ||
		!strings.Contains(got.Blocks[1].Diagram.Body, "B->>C: keep label") {
		t.Fatalf("stale-anchor transaction changed unrelated graph content: %+v", got)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_RestoresTypedFailedAnchorWithoutBody(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    participant A\n    participant B\n    participant C\n    B->>C: keep label\n"
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: diagramCallEdgeIssueAnchorWithoutBodyEdge,
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence,
		}}, nil)
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		BlockID: "diag", Action: "replace",
		Match: &types.DiagramEdgeAnchor{
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence,
		},
		Edge: &types.DiagramEdgeAnchor{
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence, VisibleLabel: "确定分析范围后收集证据",
		},
	}}, nil, lease)
	if err != nil {
		t.Fatalf("typed stale anchor must be restorable from a complete model edge: %v", err)
	}
	got := patch.ReplaceBlocks[0]
	if !strings.Contains(got.Diagram.Body, "A->>B: 确定分析范围后收集证据") ||
		!strings.Contains(got.Diagram.Body, "B->>C: keep label") {
		t.Fatalf("anchor-only replacement did not restore the selected visible edge:\n%s", got.Diagram.Body)
	}
	if len(got.EdgeAnchors) != 2 || got.EdgeAnchors[0].VisibleLabel != "确定分析范围后收集证据" ||
		got.EdgeAnchors[1].VisibleLabel != "keep label" {
		t.Fatalf("anchor-only replacement changed the wrong metadata: %+v", got.EdgeAnchors)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_UsesLiveFailureRefWithoutRetypingCoordinates(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    participant A\n    participant B\n    participant C\n    B->>C: keep label\n"
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: diagramCallEdgeIssueAnchorWithoutBodyEdge,
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence,
		}}, nil)
	if lease == nil || len(lease.Failures) != 1 || lease.Failures[0].FailureRef == "" {
		t.Fatalf("test setup missing live failure ref: %+v", lease)
	}
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		FailureRef: lease.Failures[0].FailureRef, Action: "remove",
	}}, nil, lease)
	if err != nil {
		t.Fatalf("live ref must resolve the exact stale anchor: %v", err)
	}
	got := patch.ReplaceBlocks[0]
	if len(got.EdgeAnchors) != 1 || got.EdgeAnchors[0].FromNode != "B" ||
		strings.Contains(got.Diagram.Body, "A->>B") || !strings.Contains(got.Diagram.Body, "B->>C: keep label") {
		t.Fatalf("ref-selected remove changed unmentioned graph content: %+v", got)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_FailureRefUsesUniqueLiveCarrierWhenValidatorIdentityIsMorePrecise(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    participant O\n    participant A\n    participant B\n    O->>A: dispatch stage\n    B->>A: keep\n"
	prev.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{
		{FromNode: "O", ToNode: "A", FromIdentity: "Orchestrator", ToIdentity: "AgentAnalyzer", RelationKind: types.DiagramRelCall, VisibleLabel: "dispatch stage"},
		{FromNode: "B", ToNode: "A", FromIdentity: "Builder", ToIdentity: "AgentAnalyzer", RelationKind: types.DiagramRelCall, VisibleLabel: "keep"},
	}
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: diagramCallEdgeIssueNoEvidence,
			FromNode: "O", ToNode: "A",
			// The evidence validator resolved the visible participant to a more
			// precise operation than the rejected anchor claimed.
			FromIdentity: "Orchestrator.dispatchStage", ToIdentity: "AgentAnalyzer",
		}}, nil)
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		FailureRef: lease.Failures[0].FailureRef, Action: "remove",
	}}, nil, lease)
	if err != nil {
		t.Fatalf("live failure ref must select the unique rejected carrier despite resolved identity drift: %v", err)
	}
	got := patch.ReplaceBlocks[0]
	if strings.Contains(got.Diagram.Body, "O->>A") || !strings.Contains(got.Diagram.Body, "B->>A: keep") || len(got.EdgeAnchors) != 1 {
		t.Fatalf("identity-drift repair changed the wrong graph content: %+v\n%s", got.EdgeAnchors, got.Diagram.Body)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_FailureRefRemovesRelationlessMissingCallAnchor(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    participant O\n    participant Ctx\n    O->>Ctx: build context\n"
	prev.Blocks[1].EdgeAnchors = nil
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: diagramCallEdgeIssueMissingAnchor,
			FromNode: "O", ToNode: "Ctx", FromIdentity: "Orchestrator", ToIdentity: "BusContext",
		}}, nil)
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		FailureRef: lease.Failures[0].FailureRef, Action: "remove",
	}}, nil, lease)
	if err != nil {
		t.Fatalf("missing-call failure ref must carry its structural call relation without model coordinate retyping: %v", err)
	}
	if got := patch.ReplaceBlocks[0].Diagram.Body; strings.Contains(got, "O->>Ctx") {
		t.Fatalf("body-only ref did not remove the named visible call: %s", got)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_FailureRefIdentityDriftStillFailsClosedOnAmbiguousCarrier(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    A->>B: first\n    A->>B: second\n"
	prev.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{
		{FromNode: "A", ToNode: "B", FromIdentity: "First.call", ToIdentity: "Target.one", RelationKind: types.DiagramRelCall},
		{FromNode: "A", ToNode: "B", FromIdentity: "Second.call", ToIdentity: "Target.two", RelationKind: types.DiagramRelCall},
	}
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: diagramCallEdgeIssueNoEvidence,
			FromNode: "A", ToNode: "B", FromIdentity: "Resolved.call", ToIdentity: "Resolved.target",
		}}, nil)
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		FailureRef: lease.Failures[0].FailureRef, Action: "remove",
	}}, nil, lease)
	if err == nil || !strings.Contains(err.Error(), "matches 2 candidate prior anchors") {
		t.Fatalf("identity drift must not guess between repeated same-pair operations: %v", err)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_FailureRefReplaceStillNeedsModelEdge(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    participant A\n    participant B\n    participant C\n    B->>C: keep label\n"
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: diagramCallEdgeIssueAnchorWithoutBodyEdge,
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence,
		}}, nil)
	ref := lease.Failures[0].FailureRef
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	if err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		FailureRef: ref, Action: "replace",
	}}, nil, lease); err == nil || !strings.Contains(err.Error(), "edge is required") {
		t.Fatalf("ref must not let the system author a replacement edge: %v", err)
	}
	patch = &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		FailureRef: ref, Action: "replace",
		Edge: &types.DiagramEdgeAnchor{
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence, VisibleLabel: "model wording",
		},
	}}, nil, lease)
	if err != nil || !strings.Contains(patch.ReplaceBlocks[0].Diagram.Body, "A->>B: model wording") {
		t.Fatalf("complete model-authored ref replacement must pass: err=%v patch=%+v", err, patch)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_FailureRefScopeFailsClosed(t *testing.T) {
	prev := atomicPatchTestDocument()
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "semantic_relation_edge_unproven",
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence,
		}}, nil)
	ref := lease.Failures[0].FailureRef
	tests := []struct {
		name  string
		edits []emitAnswerDiagramEdgeEdit
		want  string
	}{
		{name: "unknown or stale", edits: []emitAnswerDiagramEdgeEdit{{FailureRef: "rf1-not-live", Action: "remove"}}, want: "unknown or stale"},
		{name: "cross block", edits: []emitAnswerDiagramEdgeEdit{{FailureRef: ref, BlockID: "other", Action: "remove"}}, want: "belongs to block_id"},
		{name: "duplicate consumption", edits: []emitAnswerDiagramEdgeEdit{{FailureRef: ref, Action: "relabel", VisibleLabel: "one"}, {FailureRef: ref, Action: "remove"}}, want: "reuses failure_ref"},
		{name: "conflicting match", edits: []emitAnswerDiagramEdgeEdit{{FailureRef: ref, Action: "remove", Match: &types.DiagramEdgeAnchor{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelPrecedence}}}, want: "mutually exclusive"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
			err := applyModelAuthoredDiagramAtomicEdits(prev, patch, tc.edits, nil, lease)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("failure ref scope must fail closed with %q: %v", tc.want, err)
			}
			if len(patch.ReplaceBlocks) != 0 {
				t.Fatalf("rejected ref transaction must not publish a replacement: %+v", patch.ReplaceBlocks)
			}
		})
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_ExplicitMissListsBoundedFailureRefs(t *testing.T) {
	prev := atomicPatchTestDocument()
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "semantic_relation_edge_unproven",
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence,
		}}, nil)
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		BlockID: "diag", Action: "remove",
		Match: &types.DiagramEdgeAnchor{FromNode: "Analyzer", ToNode: "Explorer", RelationKind: types.DiagramRelPrecedence},
	}}, nil, lease)
	if err == nil || !strings.Contains(err.Error(), "live exact prior-anchor roster") ||
		!strings.Contains(err.Error(), lease.Failures[0].FailureRef) {
		t.Fatalf("explicit selector miss must expose the bounded live ref roster: %v", err)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_AnchorWithoutBodyPermissionIsExact(t *testing.T) {
	newPrev := func() *types.AnswerDocumentV2 {
		prev := atomicPatchTestDocument()
		prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    participant A\n    participant B\n    participant C\n    B->>C: keep label\n"
		return prev
	}
	edit := emitAnswerDiagramEdgeEdit{
		BlockID: "diag", Action: "remove",
		Match: &types.DiagramEdgeAnchor{
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence,
		},
	}
	for name, lease := range map[string]*types.AnswerDiagramRelationRepairLease{
		"missing lease": nil,
		"wrong issue": types.NewAnswerDiagramRelationRepairLease(newPrev(),
			[]types.AnswerDiagramRelationRepairFailure{{
				BlockID: "diag", Issue: "semantic_relation_edge_unproven",
				FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
				RelationKind: types.DiagramRelPrecedence,
			}}, nil),
		"wrong tuple": types.NewAnswerDiagramRelationRepairLease(newPrev(),
			[]types.AnswerDiagramRelationRepairFailure{{
				BlockID: "diag", Issue: diagramCallEdgeIssueAnchorWithoutBodyEdge,
				FromNode: "B", ToNode: "C", FromIdentity: "Explorer", ToIdentity: "Extractor",
				RelationKind: types.DiagramRelPrecedence,
			}}, nil),
	} {
		t.Run(name, func(t *testing.T) {
			patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
			err := applyModelAuthoredDiagramAtomicEdits(newPrev(), patch, []emitAnswerDiagramEdgeEdit{edit}, nil, lease)
			if err == nil || !strings.Contains(err.Error(), "Mermaid body has no matching edge") {
				t.Fatalf("non-exact stale-anchor permission must fail closed: %v", err)
			}
		})
	}
}

func TestEmitAnswerDocumentPatch_AtomicRelationEditsHonorTypedLease(t *testing.T) {
	prev := atomicPatchTestDocument()
	mut := types.NewMutableState("atomic")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	mut.SetAnswerDiagramRelationRepairLease(types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "semantic_relation_edge_unproven",
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence,
		}}, []types.AnswerDiagramRelationRepairCandidate{{
			BlockID: "diag", RelationKind: types.DiagramRelPrecedence,
			FromIdentity: "Extractor", ToIdentity: "Finalizer", Source: "stageauthority",
		}}))
	bus := &types.BusContext{Mutable: mut}
	raw := json.RawMessage(`{
		"unchanged_block_ids":["summary"],
		"diagram_edge_edits":[
			{"block_id":"diag","action":"remove","match":{"from_node":"A","to_node":"B","from_identity":"Analyzer","to_identity":"Explorer","relation_kind":"precedence"}},
			{"block_id":"diag","action":"add","edge":{"from_node":"C","to_node":"F","from_identity":"Extractor","to_identity":"Finalizer","relation_kind":"precedence","visible_label":"结构化事实就绪后组织答案"}}
		]
	}`)
	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
	if err != nil || !res.Success {
		t.Fatalf("listed atomic transaction must pass: err=%v res=%+v", err, res)
	}
	got := mut.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 2 || len(got.Blocks[1].EdgeAnchors) != 2 {
		t.Fatalf("unexpected persisted document: %+v", got)
	}
	if strings.Contains(got.Blocks[1].Diagram.Body, "A->>B: old label") ||
		!strings.Contains(got.Blocks[1].Diagram.Body, "B->>C: keep label") ||
		!strings.Contains(got.Blocks[1].Diagram.Body, "C->>F: 结构化事实就绪后组织答案") {
		t.Fatalf("atomic transaction changed the wrong graph surface:\n%s", got.Blocks[1].Diagram.Body)
	}
}

func TestEmitAnswerDocumentPatch_AtomicAllowedAdditionRestoresTypedIdentityBeforeLease(t *testing.T) {
	newBus := func(allowed types.AnswerDiagramRelationRepairCandidate, recipes []types.DiagramEdgeAnchor) *types.BusContext {
		prev := atomicPatchTestDocument()
		mut := types.NewMutableState("atomic typed identity ordering")
		mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
		mut.SetAnswerDiagramRelationRepairLease(types.NewAnswerDiagramRelationRepairLease(prev,
			[]types.AnswerDiagramRelationRepairFailure{{
				BlockID: "diag", Issue: "semantic_relation_edge_unproven",
				FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelPrecedence,
			}}, []types.AnswerDiagramRelationRepairCandidate{allowed}))
		mut.SetFinalizerTypedRelationRecipeAvailable(true)
		mut.SetFinalizerTypedRelationRecipeAnchors(recipes)
		return &types.BusContext{Mutable: mut}
	}
	allowed := types.AnswerDiagramRelationRepairCandidate{
		BlockID: "diag", RelationKind: types.DiagramRelPrecedence,
		FromIdentity: "Extractor", ToIdentity: "Finalizer", Source: "stageauthority",
	}
	recipe := types.DiagramEdgeAnchor{
		FromNode: "C", ToNode: "F", FromIdentity: "Extractor", ToIdentity: "Finalizer",
		RelationKind: types.DiagramRelPrecedence,
	}
	raw := json.RawMessage(`{
		"unchanged_block_ids":["summary"],
		"diagram_edge_edits":[{"block_id":"diag","action":"add","edge":{
			"from_node":"C","to_node":"F","relation_kind":"precedence","visible_label":"结构化事实就绪后组织答案"
		}}]
	}`)

	t.Run("unique typed receipt completes invisible identity metadata", func(t *testing.T) {
		bus := newBus(allowed, []types.DiagramEdgeAnchor{recipe})
		res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
		if err != nil || !res.Success {
			t.Fatalf("model-selected allowed edge must pass after exact metadata completion: err=%v res=%+v", err, res)
		}
		doc := bus.Mutable.AnswerDocumentV2()
		if doc == nil || len(doc.Blocks) != 2 || len(doc.Blocks[1].EdgeAnchors) != 3 {
			t.Fatalf("unexpected persisted document: %+v", doc)
		}
		got := doc.Blocks[1].EdgeAnchors[2]
		if got.FromNode != "C" || got.ToNode != "F" || got.VisibleLabel != "结构化事实就绪后组织答案" ||
			got.FromIdentity != "Extractor" || got.ToIdentity != "Finalizer" {
			t.Fatalf("identity completion changed visible model authorship or lost typed metadata: %+v", got)
		}
	})

	t.Run("missing receipt remains fail closed", func(t *testing.T) {
		bus := newBus(allowed, nil)
		res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
		if err != nil {
			t.Fatalf("unexpected execution error: %v", err)
		}
		if res.Success || !strings.Contains(res.Summary, "unlisted_relation_added") {
			t.Fatalf("missing typed receipt must not authorize an identity-less addition: %+v", res)
		}
	})

	t.Run("ambiguous receipt remains fail closed", func(t *testing.T) {
		other := recipe
		other.FromIdentity = "OtherExtractor"
		bus := newBus(allowed, []types.DiagramEdgeAnchor{recipe, other})
		res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
		if err != nil {
			t.Fatalf("unexpected execution error: %v", err)
		}
		if res.Success || !strings.Contains(res.Summary, "unlisted_relation_added") {
			t.Fatalf("ambiguous typed receipt must not choose an identity pair: %+v", res)
		}
	})

	t.Run("receipt outside allowed additions stays rejected", func(t *testing.T) {
		otherAllowed := allowed
		otherAllowed.FromIdentity = "OtherExtractor"
		bus := newBus(otherAllowed, []types.DiagramEdgeAnchor{recipe})
		res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
		if err != nil {
			t.Fatalf("unexpected execution error: %v", err)
		}
		if res.Success || !strings.Contains(res.Summary, "unlisted_relation_added") {
			t.Fatalf("a typed recipe that is not lease-listed must remain rejected: %+v", res)
		}
	})
}

func TestEmitAnswerDocumentPatch_AtomicUnlistedAdditionStillRejectedByLease(t *testing.T) {
	prev := atomicPatchTestDocument()
	mut := types.NewMutableState("atomic-unlisted")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	mut.SetAnswerDiagramRelationRepairLease(types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "semantic_relation_edge_unproven",
			FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelPrecedence,
		}}, nil))
	bus := &types.BusContext{Mutable: mut}
	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{
		"diagram_edge_edits":[{"block_id":"diag","action":"add","edge":{"from_node":"C","to_node":"F","from_identity":"Extractor","to_identity":"Finalizer","relation_kind":"precedence","visible_label":"组织答案"}}]
	}`))
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}
	if res.Success || res.Repair == nil || res.Repair.Code != types.ToolRepairCodeAnswerDocRelationRepairScope || !strings.Contains(res.Summary, "unlisted_relation_added") {
		t.Fatalf("unlisted atomic relation must remain fail-closed: %+v", res)
	}
	if got := mut.AnswerDocumentV2(); got == nil || strings.Contains(got.Blocks[1].Diagram.Body, "C->>F") {
		t.Fatalf("rejected atomic transaction polluted the accepted base: %+v", got)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_RemovesTypedFailedBodyOnlyEdge(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    participant A\n    participant B\n    A->>A: unsupported self call\n    A->>B: old label\n"
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "missing_call_anchor", FromNode: "A", ToNode: "A",
			RelationKind: types.DiagramRelCall,
		}}, nil)
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		BlockID: "diag", Action: "remove",
		Match: &types.DiagramEdgeAnchor{FromNode: "A", ToNode: "A", RelationKind: types.DiagramRelCall},
	}}, nil, lease)
	if err != nil {
		t.Fatalf("typed failed body-only edge must be removable: %v", err)
	}
	got := patch.ReplaceBlocks[0]
	if strings.Contains(got.Diagram.Body, "A->>A") || !strings.Contains(got.Diagram.Body, "A->>B: old label") {
		t.Fatalf("body-only remove changed the wrong Mermaid content:\n%s", got.Diagram.Body)
	}
	if len(got.EdgeAnchors) != 2 {
		t.Fatalf("body-only remove must not change unrelated anchors: %+v", got.EdgeAnchors)
	}
}

func TestEmitAnswerDocumentPatch_RemovesTypedFailedBodyOnlyEdgeTransactionally(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    participant A\n    participant B\n    A->>A: unsupported self call\n    A->>B: old label\n"
	mut := types.NewMutableState("atomic-body-only")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	mut.SetAnswerDiagramRelationRepairLease(types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "missing_call_anchor", FromNode: "A", ToNode: "A",
			RelationKind: types.DiagramRelCall,
		}}, nil))
	bus := &types.BusContext{Mutable: mut}
	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{
		"unchanged_block_ids":["summary"],
		"diagram_edge_edits":[{"block_id":"diag","action":"remove","match":{"from_node":"A","to_node":"A","relation_kind":"call"}}]
	}`))
	if err != nil || !res.Success {
		t.Fatalf("body-only producer-scoped transaction must pass: err=%v res=%+v", err, res)
	}
	got := mut.AnswerDocumentV2()
	if got == nil || strings.Contains(got.Blocks[1].Diagram.Body, "A->>A") ||
		!strings.Contains(got.Blocks[1].Diagram.Body, "A->>B: old label") {
		t.Fatalf("persisted transaction changed the wrong graph content: %+v", got)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_RejectsUnlistedBodyOnlyEdge(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    A->>A: unsupported self call\n    A->>B: old label\n"
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		BlockID: "diag", Action: "remove",
		Match: &types.DiagramEdgeAnchor{FromNode: "A", ToNode: "A", RelationKind: types.DiagramRelCall},
	}}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "exact prior anchor") {
		t.Fatalf("unlisted body-only edge must remain outside atomic scope: %v", err)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_ReplacesTypedFailedBodyOnlyEdge(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    participant A\n    participant B\n    participant C\n    A->>A: unsupported self call\n    A->>B: old label\n"
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "missing_call_anchor", FromNode: "A", ToNode: "A",
			RelationKind: types.DiagramRelCall,
		}}, []types.AnswerDiagramRelationRepairCandidate{{
			BlockID: "diag", RelationKind: types.DiagramRelPrecedence,
			FromIdentity: "Analyzer", ToIdentity: "Explorer", Source: "stageauthority",
		}})
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		BlockID: "diag", Action: "replace",
		Match: &types.DiagramEdgeAnchor{FromNode: "A", ToNode: "A", RelationKind: types.DiagramRelCall},
		Edge: &types.DiagramEdgeAnchor{
			FromNode: "A", ToNode: "C", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence, VisibleLabel: "确定范围后继续",
		},
	}}, nil, lease)
	if err != nil {
		t.Fatalf("typed failed body-only edge replacement rejected: %v", err)
	}
	got := patch.ReplaceBlocks[0]
	if strings.Contains(got.Diagram.Body, "A->>A") || !strings.Contains(got.Diagram.Body, "A->>C: 确定范围后继续") {
		t.Fatalf("body-only replacement did not stay local:\n%s", got.Diagram.Body)
	}
	if len(got.EdgeAnchors) != 3 || got.EdgeAnchors[2].ToNode != "C" {
		t.Fatalf("replacement anchor was not model-authored into the carrier: %+v", got.EdgeAnchors)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_NormalGraphMayExceedSixteenOperations(t *testing.T) {
	prev := atomicPatchTestDocument()
	var body strings.Builder
	body.WriteString("sequenceDiagram\n")
	failures := make([]types.AnswerDiagramRelationRepairFailure, 0, 17)
	edits := make([]emitAnswerDiagramEdgeEdit, 0, 17)
	for i := 0; i < 17; i++ {
		from := string(rune('A' + i))
		to := from + "_bad"
		body.WriteString("    " + from + "->>" + to + ": unsupported\n")
		failures = append(failures, types.AnswerDiagramRelationRepairFailure{
			BlockID: "diag", Issue: "missing_call_anchor", FromNode: from, ToNode: to,
			RelationKind: types.DiagramRelCall,
		})
		edits = append(edits, emitAnswerDiagramEdgeEdit{
			BlockID: "diag", Action: "remove",
			Match: &types.DiagramEdgeAnchor{FromNode: from, ToNode: to, RelationKind: types.DiagramRelCall},
		})
	}
	prev.Blocks[1].Diagram.Body = body.String()
	prev.Blocks[1].EdgeAnchors = nil
	lease := types.NewAnswerDiagramRelationRepairLease(prev, failures, nil)
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	if err := applyModelAuthoredDiagramAtomicEdits(prev, patch, edits, nil, lease); err != nil {
		t.Fatalf("17 exact model-authored repairs must fit one validated transaction: %v", err)
	}
	if edges := mermaidcompat.ParseEdges(patch.ReplaceBlocks[0].Diagram.Body); len(edges) != 0 {
		t.Fatalf("all 17 selected failed edges should be removed, got %+v", edges)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_RequiresBodyOccurrenceForAmbiguousPair(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    A->>B: setup context\n    A-->>B: model output\n"
	prev.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelDataFlow,
	}}
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "data_flow_edge_unproven", FromNode: "A", ToNode: "B",
			RelationKind: types.DiagramRelDataFlow,
		}}, nil)
	baseEdit := emitAnswerDiagramEdgeEdit{
		BlockID: "diag", Action: "remove",
		Match: &types.DiagramEdgeAnchor{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelDataFlow},
	}
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	if err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{baseEdit}, nil, lease); err == nil ||
		!strings.Contains(err.Error(), "set body_occurrence explicitly") {
		t.Fatalf("ambiguous body pair must fail closed with an executable selector hint: %v", err)
	}

	baseEdit.BodyOccurrence = 2
	patch = &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	if err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{baseEdit}, nil, lease); err != nil {
		t.Fatalf("explicit body occurrence must select the model-owned edge: %v", err)
	}
	got := patch.ReplaceBlocks[0]
	if !strings.Contains(got.Diagram.Body, "A->>B: setup context") || strings.Contains(got.Diagram.Body, "model output") || len(got.EdgeAnchors) != 0 {
		t.Fatalf("explicit body occurrence removed the wrong edge/anchor: %+v\n%s", got.EdgeAnchors, got.Diagram.Body)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_BodyOnlyFailureMaySharePairWithDifferentAnchor(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    A-->>B: grounded result\n    A->>B: unanchored invocation\n"
	prev.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "A", ToNode: "B", FromIdentity: "ResultProducer", ToIdentity: "ResultConsumer",
		RelationKind: types.DiagramRelReturn, VisibleLabel: "grounded result",
	}}
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "missing_call_anchor", FromNode: "A", ToNode: "B",
			RelationKind: types.DiagramRelCall,
		}}, nil)
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		BlockID: "diag", Action: "remove", BodyOccurrence: 2,
		Match: &types.DiagramEdgeAnchor{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall},
	}}, nil, lease)
	if err != nil {
		t.Fatalf("typed failed body-only relation must not be blocked by a different anchor on the same pair: %v", err)
	}
	got := patch.ReplaceBlocks[0]
	if !strings.Contains(got.Diagram.Body, "A-->>B: grounded result") || strings.Contains(got.Diagram.Body, "unanchored invocation") {
		t.Fatalf("body-only edit selected the wrong same-pair statement:\n%s", got.Diagram.Body)
	}
	if len(got.EdgeAnchors) != 1 || got.EdgeAnchors[0].RelationKind != types.DiagramRelReturn || got.EdgeAnchors[0].VisibleLabel != "grounded result" {
		t.Fatalf("different same-pair anchor must remain byte-semantically intact: %+v", got.EdgeAnchors)
	}
}

func TestEmitAnswerDocumentPatch_BodyOnlyFailureMaySharePairWithDifferentAnchorTransactionally(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    A-->>B: grounded result\n    A->>B: unanchored invocation\n"
	prev.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "A", ToNode: "B", FromIdentity: "ResultProducer", ToIdentity: "ResultConsumer",
		RelationKind: types.DiagramRelReturn, VisibleLabel: "grounded result",
	}}
	mut := types.NewMutableState("atomic-body-only-shared-pair")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	mut.SetAnswerDiagramRelationRepairLease(types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "missing_call_anchor", FromNode: "A", ToNode: "B",
			RelationKind: types.DiagramRelCall,
		}}, nil))
	res, err := (&EmitAnswerDocumentPatch{}).Execute(&types.BusContext{Mutable: mut}, json.RawMessage(`{
		"unchanged_block_ids":["summary"],
		"diagram_edge_edits":[{
			"block_id":"diag","action":"remove","body_occurrence":2,
			"match":{"from_node":"A","to_node":"B","relation_kind":"call"}
		}]
	}`))
	if err != nil || !res.Success {
		t.Fatalf("shared-pair body-only transaction must pass the full lease and validation path: err=%v res=%+v", err, res)
	}
	got := mut.AnswerDocumentV2()
	if got == nil || strings.Contains(got.Blocks[1].Diagram.Body, "unanchored invocation") ||
		!strings.Contains(got.Blocks[1].Diagram.Body, "A-->>B: grounded result") || len(got.Blocks[1].EdgeAnchors) != 1 {
		t.Fatalf("transaction did not preserve the grounded sibling relation: %+v", got)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_RejectsCompoundAndConflictingCarrier(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    A->>B: first\n"
	prev.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall}}
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"diag"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		BlockID: "diag", Action: "remove",
		Match: &types.DiagramEdgeAnchor{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall},
	}}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "conflicts with unchanged_block_ids") {
		t.Fatalf("same carrier must not be both unchanged and atomically edited: %v", err)
	}
}

func TestRelabelAtomicMermaidEdgeLine_AllSupportedFamilies(t *testing.T) {
	for name, tc := range map[string][3]string{
		"sequence": {"sequenceDiagram\n  A->>B: old", "  A->>B: old", "A->>B: new"},
		"flow":     {"flowchart LR\n  A -->|old| B", "  A -->|old| B", "A -->|new| B"},
		"class":    {"classDiagram\n  A --> B : old", "  A --> B : old", "A --> B : new"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := relabelAtomicMermaidEdgeLine(tc[0], tc[1], "new")
			if err != nil || !strings.Contains(got, tc[2]) {
				t.Fatalf("got=%q err=%v want substring %q", got, err, tc[2])
			}
		})
	}
}
