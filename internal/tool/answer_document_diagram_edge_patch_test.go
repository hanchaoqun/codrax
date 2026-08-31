package tool

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/stageauthority"
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

func TestApplyModelAuthoredDiagramAtomicEditsWithParticipants_RemovesOnlyChosenNewOrphan(t *testing.T) {
	newLease := func(prev *types.AnswerDocumentV2) *types.AnswerDiagramRelationRepairLease {
		lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "semantic_relation_edge_unproven",
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence, BodyOccurrence: 1,
		}}, nil)
		if lease == nil || len(lease.Failures) != 1 || !lease.Failures[0].AllowsAction("remove") {
			t.Fatalf("test setup: executable failure lease missing: %+v", lease)
		}
		lease.OptionalOrphanCleanups = testDiagramOrphanCandidates("diag", "A")
		return lease
	}

	t.Run("sequence declaration removed after its only failed edge", func(t *testing.T) {
		prev := atomicPatchTestDocument()
		lease := newLease(prev)
		patch := &types.AnswerDocumentV2Patch{}
		err := applyModelAuthoredDiagramAtomicEditsWithParticipants(
			prev, patch,
			[]emitAnswerDiagramEdgeEdit{{FailureRef: lease.Failures[0].FailureRef, Action: "remove"}}, nil,
			[]emitAnswerDiagramParticipantEdit{{BlockID: "diag", ParticipantID: "A", Action: "remove_if_isolated"}},
			nil, lease,
		)
		if err != nil {
			t.Fatalf("atomic edge+participant cleanup rejected: %v", err)
		}
		if len(patch.ReplaceBlocks) != 1 {
			t.Fatalf("compiled replacements=%d", len(patch.ReplaceBlocks))
		}
		got := patch.ReplaceBlocks[0]
		if strings.Contains(got.Diagram.Body, "participant A") || strings.Contains(got.Diagram.Body, "A->>B") {
			t.Fatalf("selected orphan or failed edge survived:\n%s", got.Diagram.Body)
		}
		for _, want := range []string{"participant B", "participant C", "B->>C: keep label"} {
			if !strings.Contains(got.Diagram.Body, want) {
				t.Fatalf("unmentioned graph content lost %q:\n%s", want, got.Diagram.Body)
			}
		}
		if len(got.EdgeAnchors) != 1 || got.EdgeAnchors[0].FromNode != "B" || got.EdgeAnchors[0].ToNode != "C" {
			t.Fatalf("unmentioned anchor changed: %+v", got.EdgeAnchors)
		}
	})

	t.Run("conditional cleanup is a no-op for protected live candidates", func(t *testing.T) {
		for name, tc := range map[string]struct {
			participant string
			protected   []string
			addBoundary bool
		}{
			"requested": {participant: "A", protected: []string{"A"}},
			"boundary":  {participant: "A", addBoundary: true},
		} {
			t.Run(name, func(t *testing.T) {
				prev := atomicPatchTestDocument()
				if tc.addBoundary {
					prev.Blocks[1].ParticipantBoundaries = []types.DiagramParticipantBoundary{{
						Participant: "A", Status: types.DiagramParticipantBoundaryUnproven,
					}}
				}
				lease := newLease(prev)
				err := applyModelAuthoredDiagramAtomicEditsWithParticipants(
					prev, &types.AnswerDocumentV2Patch{},
					[]emitAnswerDiagramEdgeEdit{{FailureRef: lease.Failures[0].FailureRef, Action: "remove"}}, nil,
					[]emitAnswerDiagramParticipantEdit{{BlockID: "diag", ParticipantID: tc.participant, Action: "remove_if_isolated"}},
					tc.protected, lease,
				)
				if err != nil {
					t.Fatalf("protected conditional cleanup should be a safe no-op: %v", err)
				}
			})
		}
	})

	t.Run("unlisted still-connected declaration remains rejected", func(t *testing.T) {
		prev := atomicPatchTestDocument()
		lease := newLease(prev)
		err := applyModelAuthoredDiagramAtomicEditsWithParticipants(
			prev, &types.AnswerDocumentV2Patch{},
			[]emitAnswerDiagramEdgeEdit{{FailureRef: lease.Failures[0].FailureRef, Action: "remove"}}, nil,
			[]emitAnswerDiagramParticipantEdit{{BlockID: "diag", ParticipantID: "B", Action: "remove_if_isolated"}},
			nil, lease,
		)
		if err == nil || !strings.Contains(err.Error(), "unexpected/ineligible decisions") ||
			!strings.Contains(err.Error(), "diag/B") {
			t.Fatalf("unlisted participant cleanup must fail closed: %v", err)
		}
	})

	t.Run("sequence Note reference turns conditional removal into a no-op", func(t *testing.T) {
		prev := atomicPatchTestDocument()
		prev.Blocks[1].Diagram.Body += "    Note over A,C: keep authored context\n"
		lease := newLease(prev)
		// Simulate a stale producer candidate to pin the executor's independent
		// syntax-liveness recheck. Current production candidate generation no
		// longer publishes this row in the first place.
		lease.OptionalOrphanCleanups = testDiagramOrphanCandidates("diag", "A")
		patch := &types.AnswerDocumentV2Patch{}
		err := applyModelAuthoredDiagramAtomicEditsWithParticipants(
			prev, patch,
			[]emitAnswerDiagramEdgeEdit{{FailureRef: lease.Failures[0].FailureRef, Action: "remove"}}, nil,
			[]emitAnswerDiagramParticipantEdit{{BlockID: "diag", ParticipantID: "A", Action: "remove_if_isolated"}},
			nil, lease,
		)
		if err != nil || len(patch.ReplaceBlocks) != 1 ||
			!strings.Contains(patch.ReplaceBlocks[0].Diagram.Body, "participant A") ||
			!strings.Contains(patch.ReplaceBlocks[0].Diagram.Body, "Note over A,C") ||
			strings.Contains(patch.ReplaceBlocks[0].Diagram.Body, "A->>B") {
			t.Fatalf("Note-referenced conditional removal must preserve the declaration and Note: err=%v patch=%+v", err, patch)
		}
	})

	t.Run("flow standalone declaration uses the same contract", func(t *testing.T) {
		prev := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
			ID: "diag", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n  A[API]\n  B[Worker]\n  A -->|calls| B\n"},
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "A", ToNode: "B", FromIdentity: "API.Call", ToIdentity: "Worker.Run",
				RelationKind: types.DiagramRelCall, VisibleLabel: "calls",
			}},
		}}}
		lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "call_edge_unproven", FromNode: "A", ToNode: "B",
			FromIdentity: "API.Call", ToIdentity: "Worker.Run", RelationKind: types.DiagramRelCall, BodyOccurrence: 1,
		}}, nil)
		lease.OptionalOrphanCleanups = testDiagramOrphanCandidates("diag", "A")
		patch := &types.AnswerDocumentV2Patch{}
		err := applyModelAuthoredDiagramAtomicEditsWithParticipants(
			prev, patch, []emitAnswerDiagramEdgeEdit{{FailureRef: lease.Failures[0].FailureRef, Action: "remove"}}, nil,
			[]emitAnswerDiagramParticipantEdit{{BlockID: "diag", ParticipantID: "A", Action: "remove_if_isolated"}}, nil, lease,
		)
		if err != nil || len(patch.ReplaceBlocks) != 1 || strings.Contains(patch.ReplaceBlocks[0].Diagram.Body, "A[API]") {
			t.Fatalf("flow cleanup failed: err=%v patch=%+v", err, patch)
		}
	})
}

func TestAtomicDiagramParticipantProtectionUsesDecoratedPrimaryIdentity(t *testing.T) {
	protected := map[string]bool{"buscontext": true}
	for _, surface := range []string{
		`BusContext<br/>internal/types/context.go:7593`,
		`BusContext\\ninternal/types/context.go:7593`,
		"BusContext\ninternal/types/context.go:7593",
	} {
		if !atomicDiagramParticipantProtected(protected, "BC", surface) {
			t.Fatalf("decorated primary requested identity was not protected: %q", surface)
		}
	}
	if atomicDiagramParticipantProtected(protected, "MS", `MutableState<br/>context.go:113`) {
		t.Fatal("unrelated first-line identity must not become protected by metadata projection")
	}
}

func TestEmitAnswerDocumentPatch_WiresOptionalOrphanCleanupThroughProductionEnvelope(t *testing.T) {
	prev := atomicPatchTestDocument()
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "diag", Issue: "semantic_relation_edge_unproven",
		FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
		RelationKind: types.DiagramRelPrecedence, BodyOccurrence: 1,
	}}, nil)
	if lease == nil || len(lease.Failures) != 1 {
		t.Fatalf("test setup: live lease missing: %+v", lease)
	}
	lease.OptionalOrphanCleanups = testDiagramOrphanCandidates("diag", "A")
	mut := types.NewMutableState("production participant cleanup")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	params := json.RawMessage(fmt.Sprintf(`{
		"unchanged_block_ids":["summary"],
		"diagram_edge_edits":[{"failure_ref":%q,"action":"remove"}],
		"diagram_participant_edits":[{"block_id":"diag","participant_id":"A","action":"remove_if_isolated"}]
	}`, lease.Failures[0].FailureRef))
	res, err := (&EmitAnswerDocumentPatch{}).Execute(&types.BusContext{Mutable: mut}, params)
	if err != nil || !res.Success {
		t.Fatalf("production envelope rejected model-owned cleanup: err=%v res=%+v", err, res)
	}
	got := mut.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 2 || got.Blocks[1].Diagram == nil ||
		strings.Contains(got.Blocks[1].Diagram.Body, "participant A") ||
		strings.Contains(got.Blocks[1].Diagram.Body, "A->>B") ||
		!strings.Contains(got.Blocks[1].Diagram.Body, "B->>C: keep label") {
		t.Fatalf("production cleanup did not preserve the local graph: %+v", got)
	}
}

func TestApplyModelAuthoredDiagramAtomicEditsWithParticipants_RequiresExplicitNewOrphanDisposition(t *testing.T) {
	prev := atomicPatchTestDocument()
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "diag", Issue: "semantic_relation_edge_unproven",
		FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
		RelationKind: types.DiagramRelPrecedence, BodyOccurrence: 1,
	}}, nil)
	lease.OptionalOrphanCleanups = testDiagramOrphanCandidates("diag", "A")
	removeEdge := []emitAnswerDiagramEdgeEdit{{FailureRef: lease.Failures[0].FailureRef, Action: "remove"}}

	t.Run("omission fails after selected edits isolate the candidate", func(t *testing.T) {
		err := applyModelAuthoredDiagramAtomicEditsWithParticipants(
			prev, &types.AnswerDocumentV2Patch{}, removeEdge, nil, nil, nil, lease,
		)
		if err == nil || !strings.Contains(err.Error(), "became isolated") ||
			!strings.Contains(err.Error(), "retain_as_context") {
			t.Fatalf("missing explicit disposition must fail with both choices: %v", err)
		}
	})

	t.Run("retain uses only the model-authored visible label", func(t *testing.T) {
		patch := &types.AnswerDocumentV2Patch{}
		err := applyModelAuthoredDiagramAtomicEditsWithParticipants(
			prev, patch, removeEdge, nil,
			[]emitAnswerDiagramParticipantEdit{{
				BlockID: "diag", ParticipantID: "A", Action: "retain_as_context",
				VisibleLabel: "分析入口（背景，关系未证明）",
			}}, nil, lease,
		)
		if err != nil || len(patch.ReplaceBlocks) != 1 {
			t.Fatalf("explicit retain decision rejected: err=%v patch=%+v", err, patch)
		}
		body := patch.ReplaceBlocks[0].Diagram.Body
		if !strings.Contains(body, `participant A as "分析入口（背景，关系未证明）"`) ||
			strings.Contains(body, "A->>B") {
			t.Fatalf("retain must rewrite only the declaration label after edge removal:\n%s", body)
		}
	})

	t.Run("connected candidate needs no orphan disposition", func(t *testing.T) {
		structuralLease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: diagramSequenceRelationReplyConflict,
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence, BodyOccurrence: 1,
		}}, nil)
		if structuralLease == nil || !structuralLease.Failures[0].AllowsAction("replace") {
			t.Fatalf("test setup: structural failure must expose model-authored replacement: %+v", structuralLease)
		}
		patch := &types.AnswerDocumentV2Patch{}
		err := applyModelAuthoredDiagramAtomicEditsWithParticipants(
			prev, patch, []emitAnswerDiagramEdgeEdit{{
				FailureRef: structuralLease.Failures[0].FailureRef, Action: "replace",
				Edge: &types.DiagramEdgeAnchor{
					FromNode: "A", ToNode: "B", VisibleLabel: "模型修正关系",
				},
			}}, nil, nil, nil, structuralLease,
		)
		if err != nil || len(patch.ReplaceBlocks) != 1 ||
			!strings.Contains(patch.ReplaceBlocks[0].Diagram.Body, "A->>B: 模型修正关系") {
			t.Fatalf("a still-connected candidate must not be forced through orphan disposition: err=%v patch=%+v", err, patch)
		}
	})

	t.Run("retain encodes model line breaks without changing wording", func(t *testing.T) {
		patch := &types.AnswerDocumentV2Patch{}
		err := applyModelAuthoredDiagramAtomicEditsWithParticipants(
			prev, patch, removeEdge, nil,
			[]emitAnswerDiagramParticipantEdit{{
				BlockID: "diag", ParticipantID: "A", Action: "retain_as_context",
				VisibleLabel: "分析入口\n背景关系未证明",
			}}, nil, lease,
		)
		if err != nil || len(patch.ReplaceBlocks) != 1 ||
			!strings.Contains(patch.ReplaceBlocks[0].Diagram.Body, `分析入口<br/>背景关系未证明`) {
			t.Fatalf("multiline model label should receive syntax-only Mermaid encoding: err=%v patch=%+v", err, patch)
		}
	})
}

func TestApplyModelAuthoredDiagramAtomicEditsWithParticipants_TypedAdditionMakesConditionalCleanupNoop(t *testing.T) {
	prev := atomicPatchTestDocument()
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "diag", Issue: "semantic_relation_edge_unproven",
		FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
		RelationKind: types.DiagramRelPrecedence, BodyOccurrence: 1,
	}}, []types.AnswerDiagramRelationRepairCandidate{{
		BlockID: "diag", RelationKind: types.DiagramRelPrecedence,
		FromIdentity: "Analyzer", ToIdentity: "Extractor", Source: "pipeline.go:10",
		FromNodeIDs: []string{"A"}, ToNodeIDs: []string{"C"},
	}})
	if lease == nil || len(lease.Failures) != 1 || len(lease.AllowedAdditions) != 1 {
		t.Fatalf("test setup did not produce remove+add capabilities: %+v", lease)
	}
	lease.OptionalOrphanCleanups = testDiagramOrphanCandidates("diag", "A")
	patch := &types.AnswerDocumentV2Patch{}
	err := applyModelAuthoredDiagramAtomicEditsWithParticipants(
		prev, patch,
		[]emitAnswerDiagramEdgeEdit{
			{FailureRef: lease.Failures[0].FailureRef, Action: "remove"},
			{AdditionRef: lease.AllowedAdditions[0].AdditionRef, Action: "add", Edge: &types.DiagramEdgeAnchor{
				FromNode: "A", ToNode: "C", VisibleLabel: "分析完成后提取",
			}},
		}, nil,
		[]emitAnswerDiagramParticipantEdit{{BlockID: "diag", ParticipantID: "A", Action: "remove_if_isolated"}},
		nil, lease,
	)
	if err != nil || len(patch.ReplaceBlocks) != 1 {
		t.Fatalf("typed addition should make the conditional cleanup a one-pass no-op: err=%v patch=%+v", err, patch)
	}
	body := patch.ReplaceBlocks[0].Diagram.Body
	if !strings.Contains(body, "participant A") || strings.Contains(body, "A->>B") ||
		!strings.Contains(body, "A->>C: 分析完成后提取") {
		t.Fatalf("conditional cleanup changed the model-selected connected graph:\n%s", body)
	}
}

func TestApplyModelAuthoredDiagramAtomicEditsWithParticipants_ReportsCompleteDispositionRoster(t *testing.T) {
	prev := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "diag", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			"    participant A",
			"    participant B",
			"    participant C",
			"    participant D",
			"    A->>B: first",
			"    C->>D: second",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "A", ToNode: "B", FromIdentity: "A.Run", ToIdentity: "B.Run", RelationKind: types.DiagramRelCall, VisibleLabel: "first"},
			{FromNode: "C", ToNode: "D", FromIdentity: "C.Run", ToIdentity: "D.Run", RelationKind: types.DiagramRelCall, VisibleLabel: "second"},
		},
	}}}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{
		{BlockID: "diag", Issue: "call_edge_unproven", FromNode: "A", ToNode: "B", FromIdentity: "A.Run", ToIdentity: "B.Run", RelationKind: types.DiagramRelCall, BodyOccurrence: 1},
		{BlockID: "diag", Issue: "call_edge_unproven", FromNode: "C", ToNode: "D", FromIdentity: "C.Run", ToIdentity: "D.Run", RelationKind: types.DiagramRelCall, BodyOccurrence: 1},
	}, nil)
	if lease == nil || len(lease.Failures) != 2 {
		t.Fatalf("test setup: relation failures missing: %+v", lease)
	}
	lease.OptionalOrphanCleanups = testDiagramOrphanCandidates("diag", "A", "C")

	t.Run("all newly isolated participants are returned together", func(t *testing.T) {
		err := applyModelAuthoredDiagramAtomicEditsWithParticipants(
			prev, &types.AnswerDocumentV2Patch{},
			[]emitAnswerDiagramEdgeEdit{
				{FailureRef: lease.Failures[0].FailureRef, Action: "remove"},
				{FailureRef: lease.Failures[1].FailureRef, Action: "remove"},
			}, nil, nil, nil, lease,
		)
		var roster *atomicDiagramParticipantDispositionRosterError
		if !errors.As(err, &roster) || len(roster.Missing) != 2 || len(roster.Unexpected) != 0 {
			t.Fatalf("expected one complete two-row missing roster, got err=%v roster=%+v", err, roster)
		}
		if roster.Missing[0].ParticipantID != "A" || roster.Missing[1].ParticipantID != "C" ||
			!strings.Contains(err.Error(), "diag/A") || !strings.Contains(err.Error(), "diag/C") {
			t.Fatalf("complete roster lost a participant: %v", err)
		}
		if rosterJSON, signature := atomicDiagramParticipantDispositionRosterMetadata(err); !strings.Contains(rosterJSON, `"participant_id":"A"`) ||
			!strings.Contains(rosterJSON, `"participant_id":"C"`) ||
			len(signature) != 67 || !strings.HasPrefix(signature, "v1:") {
			t.Fatalf("typed roster metadata is incomplete: json=%s signature=%q", rosterJSON, signature)
		}
	})

	t.Run("missing row remains while live connected conditional choice is a no-op", func(t *testing.T) {
		patch := &types.AnswerDocumentV2Patch{}
		err := applyModelAuthoredDiagramAtomicEditsWithParticipants(
			prev, patch,
			[]emitAnswerDiagramEdgeEdit{{FailureRef: lease.Failures[0].FailureRef, Action: "remove"}}, nil,
			[]emitAnswerDiagramParticipantEdit{{BlockID: "diag", ParticipantID: "C", Action: "remove_if_isolated"}},
			nil, lease,
		)
		var roster *atomicDiagramParticipantDispositionRosterError
		if !errors.As(err, &roster) || len(roster.Missing) != 1 || len(roster.Unexpected) != 0 {
			t.Fatalf("expected only the genuinely isolated missing row, got err=%v roster=%+v", err, roster)
		}
		if roster.Missing[0].ParticipantID != "A" {
			t.Fatalf("roster lost the genuinely isolated participant: %+v", roster)
		}
		if len(patch.ReplaceBlocks) != 0 {
			t.Fatal("failed roster validation must not author or commit a participant choice")
		}
	})
}

func TestApplyModelAuthoredDiagramAtomicEditsWithParticipants_UnlistedCleanupExplainsRemainingReply(t *testing.T) {
	prev := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "diag", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			"    participant Factory as resolve",
			"    participant Registry as REGISTRY",
			"    Factory->>Registry: lookup",
			"    Registry-->>Factory: class result",
		}, "\n")},
	}}}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "diag", Issue: "missing_call_anchor", FromNode: "Factory", ToNode: "Registry",
		RelationKind: types.DiagramRelCall, BodyOccurrence: 1,
	}}, nil)
	if lease == nil || len(lease.Failures) != 1 || !lease.Failures[0].AllowsAction("remove") {
		t.Fatalf("test setup: expected one remove-capable forward relation: %+v", lease)
	}
	err := applyModelAuthoredDiagramAtomicEditsWithParticipants(
		prev, &types.AnswerDocumentV2Patch{},
		[]emitAnswerDiagramEdgeEdit{{FailureRef: lease.Failures[0].FailureRef, Action: "remove"}}, nil,
		[]emitAnswerDiagramParticipantEdit{{BlockID: "diag", ParticipantID: "Registry", Action: "remove_if_isolated"}},
		nil, lease,
	)
	if err == nil || !strings.Contains(err.Error(), "Registry-->>Factory") ||
		!strings.Contains(err.Error(), "remains connected after the selected relation edits") ||
		!strings.Contains(err.Error(), "it is not isolated") {
		t.Fatalf("unlisted cleanup must report the exact surviving reply carrier: %v", err)
	}
}

func TestAtomicDiagramBaseIncidentEdgesRequireOneVisibleRemovalRefPerOccurrence(t *testing.T) {
	base := types.AnswerBlock{
		ID: "diag", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			" participant A",
			" participant B",
			" A->>B: first",
			" A->>B: second",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "B", FromIdentity: "A.Call", ToIdentity: "B.Run",
			RelationKind: types.DiagramRelCall, VisibleLabel: "first",
		}},
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{base}}
	lease := types.NewAnswerDiagramRelationRepairLease(doc, []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "diag", Issue: "call_edge_unproven", FromNode: "A", ToNode: "B",
		FromIdentity: "A.Call", ToIdentity: "B.Run", RelationKind: types.DiagramRelCall,
	}}, nil)
	if incident, ok := atomicDiagramBaseIncidentEdgesAreRemoveCapableFailures(base, "A", lease); ok || incident != 1 {
		t.Fatalf("one ambiguous ref must fail on the first uncovered repeated occurrence: incident=%d ok=%v lease=%+v", incident, ok, lease)
	}

	base.EdgeAnchors = nil
	doc.Blocks[0] = base
	lease = types.NewAnswerDiagramRelationRepairLease(doc, []types.AnswerDiagramRelationRepairFailure{
		{BlockID: "diag", Issue: "missing_relation_anchor", FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall, BodyOccurrence: 1},
		{BlockID: "diag", Issue: "missing_relation_anchor", FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall, BodyOccurrence: 2},
	}, nil)
	if incident, ok := atomicDiagramBaseIncidentEdgesAreRemoveCapableFailures(base, "A", lease); !ok || incident != 2 {
		t.Fatalf("two occurrence-bound visible refs should cover both edges: incident=%d ok=%v lease=%+v", incident, ok, lease)
	}
}

func TestEmitAnswerDocumentPatch_OrphanDispositionOmissionRepublishesLiveChoices(t *testing.T) {
	prev := atomicPatchTestDocument()
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "diag", Issue: "semantic_relation_edge_unproven",
		FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
		RelationKind: types.DiagramRelPrecedence, BodyOccurrence: 1,
	}}, nil)
	lease.OptionalOrphanCleanups = testDiagramOrphanCandidates("diag", "A")
	mut := types.NewMutableState("explicit orphan disposition")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	params := json.RawMessage(fmt.Sprintf(`{
		"unchanged_block_ids":["summary"],
		"diagram_edge_edits":[{"failure_ref":%q,"action":"remove"}]
	}`, lease.Failures[0].FailureRef))
	res, err := (&EmitAnswerDocumentPatch{}).Execute(&types.BusContext{Mutable: mut}, params)
	if err != nil || res.Success || res.Repair == nil {
		t.Fatalf("omitted disposition must remain on the live repair lane: err=%v res=%+v", err, res)
	}
	raw := res.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]
	for _, want := range []string{`"participant_id":"A"`, `"remove_if_isolated"`, `"retain_as_context"`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("republished live delta missing %q: %s", want, raw)
		}
	}
}

func TestEmitAnswerDocumentPatch_AtomicParticipantFailurePublishesWholeRollbackReplay(t *testing.T) {
	prev := atomicPatchTestDocument()
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "diag", Issue: "semantic_relation_edge_unproven",
		FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
		RelationKind: types.DiagramRelPrecedence, BodyOccurrence: 1,
	}}, nil)
	lease.OptionalOrphanCleanups = testDiagramOrphanCandidates("diag", "A", "C")
	mut := types.NewMutableState("atomic participant failure rollback replay")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	params := json.RawMessage(fmt.Sprintf(`{
		"unchanged_block_ids":["summary"],
		"diagram_edge_edits":[{"failure_ref":%q,"action":"remove"}],
		"diagram_participant_edits":[
			{"block_id":"diag","participant_id":"A","action":"remove_if_isolated"},
			{"block_id":"diag","participant_id":"B","action":"remove_if_isolated"}
		]
	}`, lease.Failures[0].FailureRef))
	res, err := (&EmitAnswerDocumentPatch{}).Execute(&types.BusContext{Mutable: mut}, params)
	if err != nil || res.Success || res.Repair == nil || res.Repair.Code != types.ToolRepairCodeAnswerDocRelationRepairScope {
		t.Fatalf("one invalid participant operation must reject the complete transaction on the live lane: err=%v res=%+v", err, res)
	}
	for _, want := range []string{
		"whole rejected patch transaction was rolled back",
		"none of its edge, boundary, participant, block, or citation operations were committed",
		"resubmit every operation you still choose together in one new atomic patch",
		"do not assume a valid sibling operation from the rejected call already applied",
	} {
		if !strings.Contains(res.Repair.Hint, want) {
			t.Fatalf("rollback replay hint missing %q: %+v", want, res.Repair)
		}
	}
	if raw := res.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]; !strings.Contains(raw, lease.Failures[0].FailureRef) || !strings.Contains(raw, `"participant_id":"A"`) {
		t.Fatalf("rollback response must republish the same live complete delta: %s", raw)
	}
	if raw := res.Repair.Metadata[types.ToolRepairMetaDiagramParticipantDispositionRosterJSON]; !strings.Contains(raw, `"participant_id":"B"`) || !strings.Contains(raw, `"unexpected"`) {
		t.Fatalf("rollback response must publish the exact participant mismatch roster: %s", raw)
	}
	if signature := res.Repair.Metadata[types.ToolRepairMetaDiagramRelationProgressSignature]; len(signature) != 67 || !strings.HasPrefix(signature, "v1:") {
		t.Fatalf("rollback response must publish a closed typed progress signature: %q", signature)
	}
	got := mut.AnswerDocumentV2()
	if got == nil || !strings.Contains(got.Blocks[1].Diagram.Body, "A->>B: old label") ||
		!strings.Contains(got.Blocks[1].Diagram.Body, "participant A") {
		t.Fatalf("rejected transaction must preserve the entire accepted base: %+v", got)
	}
}

func testDiagramOrphanCandidates(blockID string, participantIDs ...string) []types.AnswerDiagramOrphanCleanupCandidate {
	out := make([]types.AnswerDiagramOrphanCleanupCandidate, 0, len(participantIDs))
	for _, participantID := range participantIDs {
		out = append(out, types.AnswerDiagramOrphanCleanupCandidate{
			BlockID: blockID, ParticipantID: participantID,
			AllowedActions: []types.AnswerDiagramOrphanDispositionAction{
				types.AnswerDiagramOrphanDispositionRemove,
				types.AnswerDiagramOrphanDispositionRetain,
			},
		})
	}
	return out
}

func TestApplyModelAuthoredDiagramAtomicEdits_AdditionRefStampsOnlySelectedHiddenTuple(t *testing.T) {
	prev := atomicPatchTestDocument()
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "requested_stage_precedence_spine_incomplete",
			FromNode: "StageAnalyze", ToNode: "StageExplore", RelationKind: types.DiagramRelPrecedence,
		}}, []types.AnswerDiagramRelationRepairCandidate{{
			BlockID: "diag", RelationKind: types.DiagramRelPrecedence,
			FromIdentity: "dispatcher", ToIdentity: "finalizer",
			FromNodeIDs: []string{"businessAnalyze"}, ToNodeIDs: []string{"businessExplore"},
			Source: "internal/types/enums.go:120-121",
		}})
	if lease == nil || len(lease.AllowedAdditions) != 1 || lease.AllowedAdditions[0].AdditionRef == "" {
		t.Fatalf("expected one live referenced addition: %+v", lease)
	}
	additionRef := lease.AllowedAdditions[0].AdditionRef
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		Action: "add", AdditionRef: additionRef,
		FromNodeVisibleLabel: "分析阶段", ToNodeVisibleLabel: "探索阶段",
		Edge: &types.DiagramEdgeAnchor{
			FromNode: "businessAnalyze", ToNode: "businessExplore",
			VisibleLabel: "确定分析范围后收集证据",
		},
	}}, nil, lease)
	if err != nil {
		t.Fatalf("referenced allowed addition must compile: %v", err)
	}
	if len(patch.ReplaceBlocks) != 1 || len(patch.ReplaceBlocks[0].EdgeAnchors) != 3 {
		t.Fatalf("unexpected compiled patch: %+v", patch)
	}
	added := patch.ReplaceBlocks[0].EdgeAnchors[2]
	if added.FromNode != "businessAnalyze" || added.ToNode != "businessExplore" ||
		added.VisibleLabel != "确定分析范围后收集证据" ||
		added.FromIdentity != "dispatcher" || added.ToIdentity != "finalizer" ||
		added.RelationKind != types.DiagramRelPrecedence {
		t.Fatalf("ref must preserve visible authorship and stamp only its typed tuple: %+v", added)
	}

	merged, err := types.ApplyAnswerDocumentV2Patch(prev, patch)
	if err != nil {
		t.Fatalf("compiled patch must apply: %v", err)
	}
	// This is the production split from r806: a node-keyed recipe offers a
	// different Stage identity dialect. The already-complete ref-selected typed
	// pair must not be re-authored by that downstream metadata normalizer.
	fixed := normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(merged, []types.DiagramEdgeAnchor{{
		FromNode: "businessAnalyze", ToNode: "businessExplore",
		FromIdentity: "StageAnalyze", ToIdentity: "StageExplore",
		RelationKind: types.DiagramRelPrecedence,
	}})
	if fixed != 0 || merged.Blocks[1].EdgeAnchors[2].FromIdentity != "dispatcher" ||
		merged.Blocks[1].EdgeAnchors[2].ToIdentity != "finalizer" {
		t.Fatalf("recipe normalization must not overwrite the selected live candidate: fixed=%d anchor=%+v", fixed, merged.Blocks[1].EdgeAnchors[2])
	}
	if violations := types.ValidateAnswerDiagramRelationRepairLease(lease, merged); len(violations) != 0 {
		t.Fatalf("the exact ref-selected candidate must pass its own live lease: %+v", violations)
	}

	t.Run("stale ref fails closed", func(t *testing.T) {
		got := &types.AnswerDocumentV2Patch{}
		err := applyModelAuthoredDiagramAtomicEdits(prev, got, []emitAnswerDiagramEdgeEdit{{
			Action: "add", AdditionRef: "ra1-000000000000000000000000",
			Edge: &types.DiagramEdgeAnchor{FromNode: "X", ToNode: "Y", VisibleLabel: "model label"},
		}}, nil, lease)
		if err == nil || !strings.Contains(err.Error(), "unknown or stale") {
			t.Fatalf("stale addition ref must be rejected: %v", err)
		}
	})

	t.Run("unlisted visible endpoint mapping fails in the selected generation", func(t *testing.T) {
		got := &types.AnswerDocumentV2Patch{}
		err := applyModelAuthoredDiagramAtomicEdits(prev, got, []emitAnswerDiagramEdgeEdit{{
			Action: "add", AdditionRef: additionRef,
			FromNodeVisibleLabel: "分析阶段", ToNodeVisibleLabel: "探索阶段",
			Edge: &types.DiagramEdgeAnchor{
				FromNode: "Analyzer", ToNode: "Mutable", VisibleLabel: "model label",
			},
		}}, nil, lease)
		if err == nil || !strings.Contains(err.Error(), "is not a typed carrier") {
			t.Fatalf("one hidden tuple must not be stamped onto unrelated visible entities: %v", err)
		}
		if len(got.ReplaceBlocks) != 0 {
			t.Fatalf("endpoint-binding rejection must be transactional: %+v", got)
		}
	})

	t.Run("duplicate ref fails closed", func(t *testing.T) {
		got := &types.AnswerDocumentV2Patch{}
		edge := func(from, to string) emitAnswerDiagramEdgeEdit {
			return emitAnswerDiagramEdgeEdit{
				Action: "add", AdditionRef: additionRef,
				Edge: &types.DiagramEdgeAnchor{FromNode: from, ToNode: to, VisibleLabel: "model label"},
			}
		}
		err := applyModelAuthoredDiagramAtomicEdits(prev, got, []emitAnswerDiagramEdgeEdit{
			edge("X", "Y"), edge("P", "Q"),
		}, nil, lease)
		if err == nil || !strings.Contains(err.Error(), "reuses addition_ref") {
			t.Fatalf("one candidate ref must not authorize two visible edges: %v", err)
		}
	})

	t.Run("legacy technical mirrors are quarantined", func(t *testing.T) {
		got := &types.AnswerDocumentV2Patch{}
		err := applyModelAuthoredDiagramAtomicEdits(prev, got, []emitAnswerDiagramEdgeEdit{{
			Action: "add", AdditionRef: additionRef,
			FromNodeVisibleLabel: "分析阶段", ToNodeVisibleLabel: "探索阶段",
			Edge: &types.DiagramEdgeAnchor{
				FromNode: "businessAnalyze", ToNode: "businessExplore", FromIdentity: "other", ToIdentity: "explorer",
				RelationKind: types.DiagramRelPrecedence, VisibleLabel: "model label",
			},
		}}, nil, lease)
		if err != nil || len(got.ReplaceBlocks) != 1 || len(got.ReplaceBlocks[0].EdgeAnchors) != 3 {
			t.Fatalf("a live addition ref must quarantine hidden mirrors: err=%v patch=%+v", err, got)
		}
		added := got.ReplaceBlocks[0].EdgeAnchors[2]
		if added.FromNode != "businessAnalyze" || added.ToNode != "businessExplore" || added.VisibleLabel != "model label" ||
			added.FromIdentity != "dispatcher" || added.ToIdentity != "finalizer" ||
			added.RelationKind != types.DiagramRelPrecedence {
			t.Fatalf("ref must preserve visible authorship and restore only selected hidden fields: %+v", added)
		}
	})
}

func TestApplyModelAuthoredDiagramAtomicEdits_ValidatesTechnicalEndpointBeforeUniqueSequenceAliasRewrite(t *testing.T) {
	prev := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "diag", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramSequence, Language: "mermaid",
			Body: "sequenceDiagram\n  participant GateWith as gate.RunWith\n  participant Gate as gate.Run\n",
		},
	}}}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, nil, []types.AnswerDiagramRelationRepairCandidate{{
		BlockID: "diag", RelationKind: types.DiagramRelCall,
		FromIdentity: "Run", ToIdentity: "RunWith",
		FromNodeIDs: []string{"gate.Run"}, ToNodeIDs: []string{"GateWith"},
		Source: "internal/analysis/gate/gate.go:135",
	}})
	if lease == nil || len(lease.AllowedAdditions) != 1 {
		t.Fatalf("expected one live typed addition: %+v", lease)
	}
	patch := &types.AnswerDocumentV2Patch{}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		Action: "add", AdditionRef: lease.AllowedAdditions[0].AdditionRef,
		Edge: &types.DiagramEdgeAnchor{
			FromNode: "gate.Run", ToNode: "GateWith", VisibleLabel: "包装调用",
		},
	}}, nil, lease)
	if err != nil {
		t.Fatalf("an authorized technical endpoint must survive the trusted existing-alias rewrite: %v", err)
	}
	if len(patch.ReplaceBlocks) != 1 {
		t.Fatalf("unexpected compiled patch: %+v", patch)
	}
	got := patch.ReplaceBlocks[0]
	if !strings.Contains(got.Diagram.Body, "Gate->>GateWith: 包装调用") ||
		strings.Contains(got.Diagram.Body, "gate.Run->>GateWith") || len(got.EdgeAnchors) != 1 {
		t.Fatalf("the sequence edge must reuse the unique declared alias without creating an implicit duplicate: %+v", got)
	}
	anchor := got.EdgeAnchors[0]
	if anchor.FromNode != "Gate" || anchor.ToNode != "GateWith" ||
		anchor.FromIdentity != "Run" || anchor.ToIdentity != "RunWith" ||
		anchor.RelationKind != types.DiagramRelCall {
		t.Fatalf("alias normalization must preserve the selected hidden typed tuple: %+v", anchor)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_RequiresModelVisibleNameForNewEndpointAcrossFamilies(t *testing.T) {
	tests := []struct {
		name            string
		kind            types.DiagramKind
		body            string
		wantDeclaration string
		wantEdge        string
	}{
		{
			name: "flow", kind: types.DiagramFlow,
			body: `flowchart LR
  Existing["已有组件"]`,
			wantDeclaration: `InternalHandler_a1b2["业务处理器"]`,
			wantEdge:        `Existing -->|提交任务| InternalHandler_a1b2`,
		},
		{
			name: "sequence", kind: types.DiagramSequence,
			body: `sequenceDiagram
  participant Existing as "已有组件"`,
			wantDeclaration: `participant InternalHandler_a1b2 as "业务处理器"`,
			wantEdge:        `Existing->>InternalHandler_a1b2: 提交任务`,
		},
		{
			name: "class", kind: types.DiagramArchitecture,
			body: `classDiagram
  class Existing["已有组件"]`,
			wantDeclaration: `class InternalHandler_a1b2["业务处理器"]`,
			wantEdge:        `Existing --> InternalHandler_a1b2 : 提交任务`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prev := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
				ID: "diag", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{Kind: tc.kind, Language: "mermaid", Body: tc.body},
			}}}
			patch := &types.AnswerDocumentV2Patch{}
			err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
				BlockID: "diag", Action: "add", ToNodeVisibleLabel: "业务处理器",
				Edge: &types.DiagramEdgeAnchor{
					FromNode: "Existing", ToNode: "InternalHandler_a1b2",
					FromIdentity: "pkg.Existing", ToIdentity: "pkg.InternalHandler",
					RelationKind: types.DiagramRelCall, VisibleLabel: "提交任务",
				},
			}}, nil, nil)
			if err != nil {
				t.Fatalf("model-authored endpoint declaration rejected: %v", err)
			}
			if len(patch.ReplaceBlocks) != 1 || patch.ReplaceBlocks[0].Diagram == nil {
				t.Fatalf("compiled patch missing diagram: %+v", patch)
			}
			body := patch.ReplaceBlocks[0].Diagram.Body
			for _, want := range []string{tc.wantDeclaration, tc.wantEdge} {
				if !strings.Contains(body, want) {
					t.Fatalf("compiled %s diagram lost %q:\n%s", tc.name, want, body)
				}
			}
			if len(patch.ReplaceBlocks[0].EdgeAnchors) != 1 {
				t.Fatalf("relation anchor count changed: %+v", patch.ReplaceBlocks[0].EdgeAnchors)
			}
			anchor := patch.ReplaceBlocks[0].EdgeAnchors[0]
			if anchor.FromIdentity != "pkg.Existing" || anchor.ToIdentity != "pkg.InternalHandler" ||
				anchor.RelationKind != types.DiagramRelCall || anchor.VisibleLabel != "提交任务" {
				t.Fatalf("display declaration changed the selected relation tuple: %+v", anchor)
			}
		})
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_RejectsImplicitEndpointWithoutModelVisibleName(t *testing.T) {
	prev := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "diag", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: `flowchart LR
  Existing["已有组件"]`},
	}}}
	patch := &types.AnswerDocumentV2Patch{}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		BlockID: "diag", Action: "add",
		Edge: &types.DiagramEdgeAnchor{
			FromNode: "Existing", ToNode: "InternalHandler_a1b2",
			FromIdentity: "pkg.Existing", ToIdentity: "pkg.InternalHandler",
			RelationKind: types.DiagramRelCall, VisibleLabel: "提交任务",
		},
	}}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "to_node_visible_label is required") {
		t.Fatalf("implicit technical endpoint must require a model-authored display name: %v", err)
	}
	if len(patch.ReplaceBlocks) != 0 {
		t.Fatalf("rejected declaration must remain transactional: %+v", patch)
	}
}

func TestEnsureAtomicDiagramEndpointDeclarations_ExactExistingLabelReplayIsIdempotent(t *testing.T) {
	base := atomicPatchTestDocument()
	block := base.Blocks[1]
	original := block.Diagram.Body
	edit := emitAnswerDiagramEdgeEdit{
		FromNodeVisibleLabel: "A",
		ToNodeVisibleLabel:   "C",
		Edge: &types.DiagramEdgeAnchor{
			FromNode: "A", ToNode: "C", RelationKind: types.DiagramRelPrecedence,
		},
	}
	if err := ensureAtomicDiagramEndpointDeclarations(&block, edit); err != nil {
		t.Fatalf("an exact current-label replay after a staged retry must be idempotent: %v", err)
	}
	if block.Diagram.Body != original {
		t.Fatalf("an idempotent replay changed the model-authored diagram:\n%s", block.Diagram.Body)
	}

	edit.FromNodeVisibleLabel = "分析阶段"
	if err := ensureAtomicDiagramEndpointDeclarations(&block, edit); err == nil ||
		!strings.Contains(err.Error(), `exactly match the current explicit label "A"`) {
		t.Fatalf("a conflicting replay must remain fail-closed: %v", err)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_AnchoredComponentJoinRetargetsVisibleEndpointOnly(t *testing.T) {
	prev := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramArchitecture, Language: "mermaid", Body: strings.Join([]string{
			"flowchart LR",
			` Extractor["提炼阶段"]`,
			` TechnicalExtractor["提炼实现"] --> BuildAgentContext["构造共享上下文"]`,
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "TechnicalExtractor", ToNode: "BuildAgentContext",
			FromIdentity: "types.AgentExtractor", ToIdentity: "BuildAgentContext",
			RelationKind: types.DiagramRelArgumentFlow,
		}},
	}}}
	failure := bindDiagramRelationRepairAnchorBodyCarrier(prev, types.AnswerDiagramRelationRepairFailure{
		BlockID: "flow", Issue: diagramParticipantComponentJoinEndpointMappingIssue,
		FromNode: "TechnicalExtractor", ToNode: "BuildAgentContext",
		FromIdentity: "types.AgentExtractor", ToIdentity: "BuildAgentContext",
		RelationKind: types.DiagramRelArgumentFlow,
	})
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{failure}, nil)
	if lease == nil || len(lease.Failures) != 1 ||
		len(lease.Failures[0].AllowedActions) != 1 ||
		lease.Failures[0].AllowedActions[0] != types.AnswerDiagramRelationRepairActionReplace {
		t.Fatalf("anchored join fixture must expose one replace-only capability: %+v", lease)
	}
	patch := &types.AnswerDocumentV2Patch{}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		FailureRef: lease.Failures[0].FailureRef,
		Action:     string(types.AnswerDiagramRelationRepairActionReplace),
		Edge: &types.DiagramEdgeAnchor{
			FromNode: "Extractor", ToNode: "BuildAgentContext",
			VisibleLabel: "将提炼阶段接入共享上下文",
		},
	}}, nil, lease)
	if err != nil {
		t.Fatalf("replace-only anchored join capability must execute: %v", err)
	}
	if len(patch.ReplaceBlocks) != 1 || len(patch.ReplaceBlocks[0].EdgeAnchors) != 1 {
		t.Fatalf("replacement must stay local to one existing relation carrier: %+v", patch)
	}
	got := patch.ReplaceBlocks[0]
	anchor := got.EdgeAnchors[0]
	if anchor.FromNode != "Extractor" || anchor.ToNode != "BuildAgentContext" ||
		anchor.VisibleLabel != "将提炼阶段接入共享上下文" ||
		anchor.FromIdentity != "types.AgentExtractor" || anchor.ToIdentity != "BuildAgentContext" ||
		anchor.RelationKind != types.DiagramRelArgumentFlow {
		t.Fatalf("executor must preserve the hidden tuple and apply only model-authored presentation: %+v", anchor)
	}
	edges := mermaidcompat.ParseEdges(got.Diagram.Body)
	if len(edges) != 1 || edges[0].From != "Extractor" || edges[0].To != "BuildAgentContext" ||
		strings.Contains(got.Diagram.Body, "TechnicalExtractor --> BuildAgentContext") {
		t.Fatalf("visible carrier must be replaced once without duplicating the relation: edges=%+v\n%s", edges, got.Diagram.Body)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_AdditionRefRejectsUnrelatedParticipantMapping(t *testing.T) {
	prev := atomicPatchTestDocument()
	lease := types.NewAnswerDiagramRelationRepairLease(prev, nil, []types.AnswerDiagramRelationRepairCandidate{{
		BlockID: "diag", RelationKind: types.DiagramRelDataFlow,
		FromIdentity: "append", ToIdentity: "o.busCtx.ToolResults",
		ToNodeIDs: []string{"BusContext"}, Source: "typed-operation",
	}})
	if lease == nil || len(lease.AllowedAdditions) != 1 {
		t.Fatalf("expected one exact production-shaped addition: %+v", lease)
	}
	patch := &types.AnswerDocumentV2Patch{}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		Action: "add", AdditionRef: lease.AllowedAdditions[0].AdditionRef,
		Edge: &types.DiagramEdgeAnchor{
			FromNode: "Analyzer", ToNode: "Mutable", VisibleLabel: "model-authored data flow",
		},
	}}, nil, lease)
	if err == nil || !strings.Contains(err.Error(), "is not a typed carrier") {
		t.Fatalf("append -> ToolResults must not sign Analyzer -> Mutable: %v", err)
	}
	if len(patch.ReplaceBlocks) != 0 {
		t.Fatalf("rejected production-shaped mapping must leave no partial mutation: %+v", patch)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_AttachBindsSelectedCandidateToExistingBody(t *testing.T) {
	prev := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "summary"},
		{ID: "flow", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart LR\n  Source -->|旧的模型文案| Target\n  Keep -->|保留| Sibling\n",
		}},
	}}
	failure := types.AnswerDiagramRelationRepairFailure{
		BlockID: "flow", Issue: "missing_relation_anchor", FromNode: "Source", ToNode: "Target",
		FromIdentity: "pkg.Source.run", ToIdentity: "pkg.Target.accept", BodyOccurrence: 1,
	}
	candidate := types.AnswerDiagramRelationRepairCandidate{
		BlockID: "flow", RelationKind: types.DiagramRelArgumentFlow,
		FromIdentity: "pkg.Source.run", ToIdentity: "pkg.Target.accept", Source: "internal/source.go:10",
	}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{failure}, []types.AnswerDiagramRelationRepairCandidate{candidate})
	if lease == nil || !lease.Failures[0].AllowsAction("attach") {
		t.Fatalf("test setup missing paired attach capability: %+v", lease)
	}
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		FailureRef: lease.Failures[0].FailureRef, AdditionRef: lease.AllowedAdditions[0].AdditionRef,
		Action: "attach", Edge: &types.DiagramEdgeAnchor{
			FromNode: "Source", ToNode: "Target", VisibleLabel: "把输入交给目标处理",
		},
	}}, nil, lease)
	if err != nil {
		t.Fatalf("paired attach must compile: %v", err)
	}
	if len(patch.ReplaceBlocks) != 1 {
		t.Fatalf("unexpected compiled patch: %+v", patch)
	}
	got := patch.ReplaceBlocks[0]
	if strings.Count(got.Diagram.Body, "Source -->") != 1 ||
		!strings.Contains(got.Diagram.Body, "Source -->|把输入交给目标处理| Target") ||
		!strings.Contains(got.Diagram.Body, "Keep -->|保留| Sibling") || len(got.EdgeAnchors) != 1 {
		t.Fatalf("attach must replace one visible occurrence without duplicating or dropping siblings: %+v", got)
	}
	anchor := got.EdgeAnchors[0]
	if anchor.FromIdentity != candidate.FromIdentity || anchor.ToIdentity != candidate.ToIdentity ||
		anchor.RelationKind != candidate.RelationKind || anchor.VisibleLabel != "把输入交给目标处理" {
		t.Fatalf("attach did not preserve model wording and selected typed tuple: %+v", anchor)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_RemoveLiftsInlineBusinessNodeDeclarations(t *testing.T) {
	prev := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart TD\n" +
			"  subgraph 注册阶段\n" +
			"    decorator[@register(\"json\")] -->|注册| reg[REGISTRY 字典]\n" +
			"    reg -->|选择| jp[JsonPlugin 类]\n" +
			"  end\n"},
	}}}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "flow", Issue: "missing_call_anchor", FromNode: "decorator", ToNode: "reg",
		RelationKind: types.DiagramRelCall,
	}}, nil)
	patch := &types.AnswerDocumentV2Patch{}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		BlockID: "flow", Action: "remove",
		Match: &types.DiagramEdgeAnchor{FromNode: "decorator", ToNode: "reg", RelationKind: types.DiagramRelCall},
	}}, nil, lease)
	if err != nil {
		t.Fatalf("body-only removal must compile: %v", err)
	}
	if len(patch.ReplaceBlocks) != 1 {
		t.Fatalf("unexpected patch: %+v", patch)
	}
	body := patch.ReplaceBlocks[0].Diagram.Body
	for _, want := range []string{
		`    decorator[@register("json")]`,
		`    reg[REGISTRY 字典]`,
		`    reg -->|选择| jp[JsonPlugin 类]`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("remove lost business node carrier %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "decorator -->") || strings.Contains(body, "-->|注册|") {
		t.Fatalf("remove retained the deleted relation:\n%s", body)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_AttachLiftsInlineBusinessNodeDeclarations(t *testing.T) {
	prev := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n  Source[业务入口]:::entry -->|旧文案| Target[目标处理]\n"},
	}}}
	failure := types.AnswerDiagramRelationRepairFailure{
		BlockID: "flow", Issue: "missing_relation_anchor", FromNode: "Source", ToNode: "Target",
		FromIdentity: "pkg.Source.run", ToIdentity: "pkg.Target.accept", BodyOccurrence: 1,
	}
	candidate := types.AnswerDiagramRelationRepairCandidate{
		BlockID: "flow", RelationKind: types.DiagramRelArgumentFlow,
		FromIdentity: "pkg.Source.run", ToIdentity: "pkg.Target.accept", Source: "source.go:10",
	}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{failure}, []types.AnswerDiagramRelationRepairCandidate{candidate})
	patch := &types.AnswerDocumentV2Patch{}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		FailureRef: lease.Failures[0].FailureRef, AdditionRef: lease.AllowedAdditions[0].AdditionRef,
		Action: "attach", Edge: &types.DiagramEdgeAnchor{
			FromNode: "Source", ToNode: "Target", VisibleLabel: "交给目标处理",
		},
	}}, nil, lease)
	if err != nil {
		t.Fatalf("attach must compile: %v", err)
	}
	body := patch.ReplaceBlocks[0].Diagram.Body
	for _, want := range []string{
		"  Source[业务入口]:::entry\n",
		"  Target[目标处理]\n",
		"  Source -->|交给目标处理| Target\n",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("attach lost exact model-authored node carrier %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "旧文案") {
		t.Fatalf("attach retained the replaced relation wording:\n%s", body)
	}
}

func TestAtomicDiagramInlineNodeDeclarationLiftLeavesSequenceAndClassOutside(t *testing.T) {
	for _, body := range []string{
		"sequenceDiagram\n  participant A as 业务入口\n  A->>B: 调用\n",
		"classDiagram\n  Child --|> Base : 继承\n",
	} {
		lines := strings.Split(body, "\n")
		if got := atomicDiagramInlineNodeDeclarationsToLift(body, lines, 1); len(got) != 0 {
			t.Fatalf("independent Mermaid declaration family entered flow lift lane: body=%q got=%+v", body, got)
		}
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_AddWithBothLiveRefsAliasesAttach(t *testing.T) {
	prev := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n  Source -->|旧文案| Target\n"},
	}}}
	failure := types.AnswerDiagramRelationRepairFailure{
		BlockID: "flow", Issue: "missing_relation_anchor", FromNode: "Source", ToNode: "Target",
		FromIdentity: "pkg.Source.run", ToIdentity: "pkg.Target.accept", BodyOccurrence: 1,
	}
	candidate := types.AnswerDiagramRelationRepairCandidate{
		BlockID: "flow", RelationKind: types.DiagramRelArgumentFlow,
		FromIdentity: "pkg.Source.run", ToIdentity: "pkg.Target.accept", Source: "internal/source.go:10",
	}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{failure}, []types.AnswerDiagramRelationRepairCandidate{candidate})
	if lease == nil || len(lease.Failures) != 1 || len(lease.AllowedAdditions) != 1 || !lease.Failures[0].AllowsAction("attach") {
		t.Fatalf("test setup missing compatible live pair: %+v", lease)
	}
	patch := &types.AnswerDocumentV2Patch{}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		FailureRef:  lease.Failures[0].FailureRef,
		AdditionRef: lease.AllowedAdditions[0].AdditionRef,
		Action:      "add",
		Edge: &types.DiagramEdgeAnchor{
			FromNode: "Source", ToNode: "Target", VisibleLabel: "交给目标处理",
		},
	}}, nil, lease)
	if err != nil {
		t.Fatalf("add with both exact live refs must use the attach transport alias: %v", err)
	}
	if len(patch.ReplaceBlocks) != 1 ||
		!strings.Contains(patch.ReplaceBlocks[0].Diagram.Body, "Source -->|交给目标处理| Target") ||
		len(patch.ReplaceBlocks[0].EdgeAnchors) != 1 {
		t.Fatalf("transport alias did not compile the model-selected pair: %+v", patch)
	}
	anchor := patch.ReplaceBlocks[0].EdgeAnchors[0]
	if anchor.RelationKind != types.DiagramRelArgumentFlow ||
		anchor.FromIdentity != candidate.FromIdentity || anchor.ToIdentity != candidate.ToIdentity {
		t.Fatalf("transport alias changed the selected typed tuple: %+v", anchor)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_AddWithBothRefsStillRejectsIncompatibleCandidate(t *testing.T) {
	prev := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n  A --> B\n"},
	}}}
	failure := types.AnswerDiagramRelationRepairFailure{
		BlockID: "flow", Issue: "missing_relation_anchor", FromNode: "A", ToNode: "B",
		FromIdentity: "A.run", ToIdentity: "B.accept", BodyOccurrence: 1,
	}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{failure}, []types.AnswerDiagramRelationRepairCandidate{
		{BlockID: "flow", RelationKind: types.DiagramRelCall, FromIdentity: "A.run", ToIdentity: "B.accept", Source: "a.go:1"},
		{BlockID: "flow", RelationKind: types.DiagramRelCall, FromIdentity: "Other.run", ToIdentity: "Else.accept", Source: "b.go:2"},
	})
	var wrongRef string
	for _, addition := range lease.AllowedAdditions {
		if addition.FromIdentity == "Other.run" {
			wrongRef = addition.AdditionRef
		}
	}
	err := applyModelAuthoredDiagramAtomicEdits(prev, &types.AnswerDocumentV2Patch{}, []emitAnswerDiagramEdgeEdit{{
		FailureRef: lease.Failures[0].FailureRef, AdditionRef: wrongRef, Action: "add",
		Edge: &types.DiagramEdgeAnchor{FromNode: "A", ToNode: "B", VisibleLabel: "模型文案"},
	}}, nil, lease)
	if err == nil || !strings.Contains(err.Error(), "not compatible") {
		t.Fatalf("add alias must not authorize a different candidate: %v", err)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_AttachRejectsDifferentCandidate(t *testing.T) {
	prev := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n A --> B\n"},
	}}}
	failure := types.AnswerDiagramRelationRepairFailure{
		BlockID: "flow", Issue: "missing_relation_anchor", FromNode: "A", ToNode: "B",
		FromIdentity: "A.run", ToIdentity: "B.accept", BodyOccurrence: 1,
	}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{failure}, []types.AnswerDiagramRelationRepairCandidate{
		{BlockID: "flow", RelationKind: types.DiagramRelCall, FromIdentity: "A.run", ToIdentity: "B.accept", Source: "a.go:1"},
		{BlockID: "flow", RelationKind: types.DiagramRelCall, FromIdentity: "Other.run", ToIdentity: "Else.accept", Source: "b.go:2"},
	})
	if lease == nil || len(lease.AllowedAdditions) != 2 || !lease.Failures[0].AllowsAction("attach") {
		t.Fatalf("test setup missing mixed candidates: %+v", lease)
	}
	var wrongRef string
	for _, candidate := range lease.AllowedAdditions {
		if candidate.FromIdentity == "Other.run" {
			wrongRef = candidate.AdditionRef
		}
	}
	err := applyModelAuthoredDiagramAtomicEdits(prev, &types.AnswerDocumentV2Patch{}, []emitAnswerDiagramEdgeEdit{{
		FailureRef: lease.Failures[0].FailureRef, AdditionRef: wrongRef, Action: "attach",
		Edge: &types.DiagramEdgeAnchor{FromNode: "A", ToNode: "B", VisibleLabel: "model wording"},
	}}, nil, lease)
	if err == nil || !strings.Contains(err.Error(), "not compatible") {
		t.Fatalf("a different typed candidate must not bind to the selected body occurrence: %v", err)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_AdditionRefReusesUniqueDeclaredTypedParticipants(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n" +
		"    participant OC as Orchestrator\n" +
		"    participant CTX as BusContext\n" +
		"    participant ANA as StageAnalyze\n" +
		"    participant EXP as StageExplore\n" +
		"    participant EXT as StageExtract\n" +
		"    participant FIN as StageFinalize\n"
	prev.Blocks[1].EdgeAnchors = nil
	rows := []stageauthority.StageRow{
		{StageIdent: "StageAnalyze", StageValue: "analyze", AgentIdent: "AgentAnalyzer", AgentValue: "analyzer"},
		{StageIdent: "StageExplore", StageValue: "explore", AgentIdent: "AgentExplorer", AgentValue: "explorer"},
		{StageIdent: "StageExtract", StageValue: "extract", AgentIdent: "AgentExtractor", AgentValue: "extractor"},
		{StageIdent: "StageFinalize", StageValue: "finalize", AgentIdent: "AgentFinalizer", AgentValue: "finalizer"},
	}
	precedence := []stageauthority.PrecedenceRelation{
		{From: rows[0], To: rows[1]},
		{From: rows[1], To: rows[2]},
		{From: rows[2], To: rows[3]},
	}
	candidates := make([]types.AnswerDiagramRelationRepairCandidate, 0, len(precedence))
	for _, relation := range precedence {
		candidates = append(candidates, types.AnswerDiagramRelationRepairCandidate{
			BlockID: "diag", RelationKind: types.DiagramRelPrecedence,
			FromIdentity: relation.From.AgentValue, ToIdentity: relation.To.AgentValue,
			Source: "checkout-stage-authority",
		})
	}
	failures := []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "diag", Issue: "requested_stage_precedence_spine_incomplete",
		FromNode: "StageAnalyze", ToNode: "StageFinalize", RelationKind: types.DiagramRelPrecedence,
	}}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, failures, candidates)
	if lease == nil || len(lease.AllowedAdditions) != 3 {
		t.Fatalf("expected three referenced additions: %+v", lease)
	}
	modelNodes := [][2]string{{"analyze", "explorer"}, {"explorer", "extractor"}, {"extractor", "finalizer"}}
	labels := []string{"确定分析范围后收集证据", "证据就绪后提炼事实", "结构化事实就绪后组织答案"}
	edits := make([]emitAnswerDiagramEdgeEdit, 0, 3)
	for i := range lease.AllowedAdditions {
		edits = append(edits, emitAnswerDiagramEdgeEdit{
			Action: "add", AdditionRef: lease.AllowedAdditions[i].AdditionRef,
			Edge: &types.DiagramEdgeAnchor{
				FromNode: modelNodes[i][0], ToNode: modelNodes[i][1], VisibleLabel: labels[i],
			},
		})
	}
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	if err := applyModelAuthoredDiagramAtomicEdits(prev, patch, edits, nil, lease, precedence); err != nil {
		t.Fatalf("same typed stage aliases must canonicalize to the unique declared carriers: %v", err)
	}
	got := patch.ReplaceBlocks[0]
	for _, want := range []string{
		"ANA->>EXP: 确定分析范围后收集证据",
		"EXP->>EXT: 证据就绪后提炼事实",
		"EXT->>FIN: 结构化事实就绪后组织答案",
	} {
		if !strings.Contains(got.Diagram.Body, want) {
			t.Fatalf("canonical declared participant edge %q missing:\n%s", want, got.Diagram.Body)
		}
	}
	for _, forbidden := range []string{"analyze->>explorer", "explorer->>extractor", "extractor->>finalizer"} {
		if strings.Contains(got.Diagram.Body, forbidden) {
			t.Fatalf("implicit duplicate participant edge %q survived:\n%s", forbidden, got.Diagram.Body)
		}
	}
	wantNodes := [][2]string{{"ANA", "EXP"}, {"EXP", "EXT"}, {"EXT", "FIN"}}
	for i, anchor := range got.EdgeAnchors {
		if anchor.FromNode != wantNodes[i][0] || anchor.ToNode != wantNodes[i][1] ||
			anchor.FromIdentity != candidates[i].FromIdentity || anchor.ToIdentity != candidates[i].ToIdentity {
			t.Fatalf("edge %d lost visible carrier closure or hidden authority: %+v", i, anchor)
		}
	}

	t.Run("ambiguous duplicate declarations fail closed", func(t *testing.T) {
		ambiguous := *prev
		ambiguous.Blocks = append([]types.AnswerBlock(nil), prev.Blocks...)
		block := ambiguous.Blocks[1]
		diagram := *block.Diagram
		diagram.Body = strings.Replace(diagram.Body, "    participant EXP as StageExplore\n", "    participant EXP as StageExplore\n    participant EXP2 as AgentExplorer\n", 1)
		block.Diagram = &diagram
		ambiguous.Blocks[1] = block
		ambiguousLease := types.NewAnswerDiagramRelationRepairLease(&ambiguous, failures, candidates[:1])
		gotPatch := &types.AnswerDocumentV2Patch{}
		err := applyModelAuthoredDiagramAtomicEdits(&ambiguous, gotPatch, []emitAnswerDiagramEdgeEdit{{
			Action: "add", AdditionRef: ambiguousLease.AllowedAdditions[0].AdditionRef,
			Edge: &types.DiagramEdgeAnchor{FromNode: "analyze", ToNode: "explorer", VisibleLabel: labels[0]},
		}}, nil, ambiguousLease, precedence)
		if err == nil || !strings.Contains(err.Error(), "multiple declared sequence participants") {
			t.Fatalf("ambiguous same-identity declarations must not be guessed: %v", err)
		}
	})
}

func TestCanonicalizeAtomicSequenceAdditionNodeRefs_UsesExactCaseSensitiveParticipantID(t *testing.T) {
	block := types.AnswerBlock{
		ID: "diag", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramSequence, Language: "mermaid",
			Body: "sequenceDiagram\n    participant Analyze as StageAnalyze\n    participant Explore as StageExplore\n",
		},
	}
	rows := []stageauthority.StageRow{
		{StageIdent: "StageAnalyze", StageValue: "analyze", AgentIdent: "AgentAnalyzer", AgentValue: "analyzer"},
		{StageIdent: "StageExplore", StageValue: "explore", AgentIdent: "AgentExplorer", AgentValue: "explorer"},
	}
	edge := types.DiagramEdgeAnchor{
		FromNode: "analyze", ToNode: "Explore",
		FromIdentity: "analyzer", ToIdentity: "explorer",
		RelationKind: types.DiagramRelPrecedence,
	}
	if err := canonicalizeAtomicSequenceAdditionNodeRefs(
		&block, &edge, []stageauthority.PrecedenceRelation{{From: rows[0], To: rows[1]}},
	); err != nil {
		t.Fatal(err)
	}
	if edge.FromNode != "Analyze" || edge.ToNode != "Explore" {
		t.Fatalf("case-mismatched implicit endpoint was not rebound to exact participant IDs: %+v", edge)
	}
}

func TestEmitAnswerDocumentPatch_ProductionWiresDeclaredStageParticipantReuse(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n" +
		"    participant ANA as StageAnalyze\n" +
		"    participant EXP as StageExplore\n" +
		"    participant EXT as StageExtract\n" +
		"    participant FIN as StageFinalize\n"
	prev.Blocks[1].EdgeAnchors = nil
	mut := types.NewMutableState("atomic declared stage participant production wiring")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	ctx := &types.BusContext{
		RepoRoot: repoRoot,
		Mode:     types.ModeRead,
		Mutable:  mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
			DiagramHint: &types.DiagramHint{Kind: types.DiagramSequence, Required: true, Participants: []types.DiagramParticipantHint{
				{Identity: "analyzer", Role: types.DiagramParticipantIncidentRequired},
				{Identity: "explorer", Role: types.DiagramParticipantIncidentRequired},
				{Identity: "extractor", Role: types.DiagramParticipantIncidentRequired},
				{Identity: "finalizer", Role: types.DiagramParticipantIncidentRequired},
			}},
		}},
	}
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	precedence := diagramVerifiedReadModeStagePrecedence(ctx, view)
	if len(precedence) != 3 {
		t.Fatalf("production context must expose the three adjacent stage rows: %+v", precedence)
	}
	candidates := make([]types.AnswerDiagramRelationRepairCandidate, 0, len(precedence))
	for _, relation := range precedence {
		candidates = append(candidates, types.AnswerDiagramRelationRepairCandidate{
			BlockID: "diag", RelationKind: types.DiagramRelPrecedence,
			FromIdentity: relation.From.AgentValue, ToIdentity: relation.To.AgentValue,
			Source: "checkout-stage-authority",
		})
	}
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "requested_stage_precedence_spine_incomplete",
			FromNode: "StageAnalyze", ToNode: "StageFinalize", RelationKind: types.DiagramRelPrecedence,
		}}, candidates)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	if lease == nil || len(lease.AllowedAdditions) != 3 {
		t.Fatalf("expected three live referenced additions: %+v", lease)
	}
	modelNodes := [][2]string{{"analyze", "explorer"}, {"explorer", "extractor"}, {"extractor", "finalizer"}}
	labels := []string{"确定分析范围后收集证据", "证据就绪后提炼事实", "结构化事实就绪后组织答案"}
	edits := make([]string, 0, 3)
	for i, candidate := range lease.AllowedAdditions {
		edits = append(edits, fmt.Sprintf(`{"action":"add","addition_ref":%q,"edge":{"from_node":%q,"to_node":%q,"visible_label":%q}}`,
			candidate.AdditionRef, modelNodes[i][0], modelNodes[i][1], labels[i]))
	}
	params := json.RawMessage(`{"unchanged_block_ids":["summary"],"diagram_edge_edits":[` + strings.Join(edits, ",") + `]}`)
	res, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("production patch wiring must reuse the declared stage carriers: err=%v res=%+v", err, res)
	}
	doc := mut.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 {
		t.Fatalf("unexpected persisted document: %+v", doc)
	}
	body := doc.Blocks[1].Diagram.Body
	for _, want := range []string{
		"ANA->>EXP: 确定分析范围后收集证据",
		"EXP->>EXT: 证据就绪后提炼事实",
		"EXT->>FIN: 结构化事实就绪后组织答案",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("production wiring did not use declared participant edge %q:\n%s", want, body)
		}
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_RemovesTypedFailedAnchorWithoutBody(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    participant A\n    participant B\n    participant C\n    B->>C: keep label\n"
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: diagramCallEdgeIssueAnchorWithoutBodyEdge,
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence, BodyOccurrence: 1,
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
		"unchanged_block_ids":["diag"],
		"replace_blocks":[{"id":"summary","kind":"summary","text":"updated sibling"}],
		"diagram_edge_edits":[{"block_id":"diag","action":"remove","match":{
			"from_node":"A","to_node":"B","from_identity":"Analyzer","to_identity":"Explorer","relation_kind":"precedence"
		}}]
	}`))
	if err != nil || !res.Success {
		t.Fatalf("typed stale-anchor transaction must pass: err=%v res=%+v", err, res)
	}
	got := mut.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 2 || len(got.Blocks[1].EdgeAnchors) != 1 ||
		got.Blocks[0].Text != "updated sibling" ||
		strings.Contains(got.Blocks[1].Diagram.Body, "A->>B") ||
		!strings.Contains(got.Blocks[1].Diagram.Body, "B->>C: keep label") {
		t.Fatalf("stale-anchor transaction changed unrelated graph content: %+v", got)
	}
}

func TestLocalDiagramLeaseWholeBlockMutationViolation_TargetOnly(t *testing.T) {
	prev := atomicPatchTestDocument()
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "call_edge_unproven",
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence,
		}}, nil)
	if lease == nil {
		t.Fatal("test setup: expected live local lease")
	}
	for name, params := range map[string]*emitAnswerDocumentPatchParams{
		"replace target": {ReplaceBlocks: []emitAnswerBlockV2{{ID: "diag"}}},
		"add target":     {AddBlocks: []emitAnswerBlockV2{{ID: "diag"}}},
		"remove target":  {RemoveBlockIDs: []string{"diag"}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := localDiagramLeaseWholeBlockMutationViolation(params, lease, prev, nil); got == nil || got.BlockID != "diag" {
				t.Fatalf("whole target mutation must be rejected from typed lease: %+v", got)
			}
		})
	}
	if got := localDiagramLeaseWholeBlockMutationViolation(&emitAnswerDocumentPatchParams{
		ReplaceBlocks: []emitAnswerBlockV2{{ID: "summary"}},
	}, lease, prev, nil); got != nil {
		t.Fatalf("unrelated sibling replacement must remain available: %+v", got)
	}
	if got := localDiagramLeaseWholeBlockMutationViolation(&emitAnswerDocumentPatchParams{
		RemoveBlockIDs: []string{"summary"},
	}, lease, prev, nil); got != nil {
		t.Fatalf("unrelated sibling removal must remain available for a simultaneous structural repair: %+v", got)
	}
	if got := localDiagramLeaseWholeBlockMutationViolation(&emitAnswerDocumentPatchParams{
		AddBlocks: []emitAnswerBlockV2{{ID: "new"}},
	}, lease, prev, nil); got == nil {
		t.Fatal("executor must reject an addition omitted by the live local schema")
	}
	requiredSummaryView := &types.AnswerSemanticView{RequiredBlocks: []types.BlockRequirement{{
		Kind: types.BlockSummary, MinCount: 2, MaxCount: 2, Required: true,
	}}}
	if got := localDiagramLeaseWholeBlockMutationViolation(&emitAnswerDocumentPatchParams{
		AddBlocks: []emitAnswerBlockV2{{ID: "required-summary", Kind: "summary", Text: "model-authored"}},
	}, lease, prev, requiredSummaryView); got != nil {
		t.Fatalf("executor must admit an addition that strictly closes a typed required-block deficit: %+v", got)
	}
	if got := localDiagramLeaseWholeBlockMutationViolation(&emitAnswerDocumentPatchParams{
		AddBlocks: []emitAnswerBlockV2{{ID: "optional-caveat", Kind: "caveat", Text: "extra"}},
	}, lease, prev, requiredSummaryView); got == nil || got.Issue != "whole_add_not_authorized" {
		t.Fatalf("typed required-block addition must not widen into optional roster growth: %+v", got)
	}
	lease.AllowTargetDiagramRemoval = true
	if got := localDiagramLeaseWholeBlockMutationViolation(&emitAnswerDocumentPatchParams{
		RemoveBlockIDs: []string{"diag"},
	}, lease, prev, nil); got != nil {
		t.Fatalf("typed optional presentation contract must admit exact model-selected target removal: %+v", got)
	}
	if got := localDiagramLeaseWholeBlockMutationViolation(&emitAnswerDocumentPatchParams{
		ReplaceBlocks: []emitAnswerBlockV2{{ID: "diag"}},
	}, lease, prev, nil); got == nil || got.Issue != "whole_replace_not_authorized" {
		t.Fatalf("optional removal must not broaden into whole target replacement: %+v", got)
	}
	removed := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, Text: "keep",
	}}}
	if violations := types.ValidateAnswerDiagramRelationRepairLease(lease, removed); len(violations) != 0 {
		t.Fatalf("optional exact removal must consume the target body and anchors together: %+v", violations)
	}
	mut := types.NewMutableState("optional target removal execution")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	result, err := (&EmitAnswerDocumentPatch{}).Execute(&types.BusContext{Mutable: mut}, json.RawMessage(fmt.Sprintf(`{
		"unchanged_block_ids":["summary"],
		"remove_block_ids":["diag"],
		"diagram_edge_edits":[{"failure_ref":%q,"action":"remove"}]
	}`, lease.Failures[0].FailureRef)))
	if err != nil || !result.Success {
		t.Fatalf("production patch path must absorb a redundant local edit shadowed by explicit optional target removal: err=%v result=%+v", err, result)
	}
	if got := mut.AnswerDocumentV2(); got == nil || len(got.Blocks) != 1 || got.Blocks[0].ID != "summary" {
		t.Fatalf("optional target removal changed the wrong roster: %+v", got)
	}
	if got := mut.AnswerDiagramRelationRepairLease(); got != nil {
		t.Fatalf("successful optional target removal must consume the lease: %+v", got)
	}
}

func TestEmitAnswerDocumentPatch_LocalDiagramLeaseAllowsUnrelatedRemovalInSameTransaction(t *testing.T) {
	prev := atomicPatchTestDocument()
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "call_edge_unproven",
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence,
		}}, nil)
	if lease == nil || len(lease.Failures) != 1 || strings.TrimSpace(lease.Failures[0].FailureRef) == "" {
		t.Fatalf("test setup: expected one executable diagram failure: %+v", lease)
	}
	mut := types.NewMutableState("diagram relation plus unrelated structural repair")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	params := json.RawMessage(fmt.Sprintf(`{
		"remove_block_ids":["summary"],
		"diagram_edge_edits":[{"failure_ref":%q,"action":"remove"}]
	}`, lease.Failures[0].FailureRef))
	result, err := (&EmitAnswerDocumentPatch{}).Execute(&types.BusContext{Mutable: mut}, params)
	if err != nil || !result.Success {
		t.Fatalf("one transaction must admit an unrelated exact removal plus the selected atomic diagram repair: err=%v result=%+v", err, result)
	}
	got := mut.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 1 || got.Blocks[0].ID != "diag" || strings.Contains(got.Blocks[0].Diagram.Body, "A->>B") {
		t.Fatalf("combined repair changed the wrong carriers: %+v", got)
	}
	if mut.AnswerDiagramRelationRepairLease() != nil {
		t.Fatal("successful combined repair must consume the relation lease")
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
		t.Fatalf("live ref must resolve the exact stale anchor even when the producer retained a diagnostic body occurrence: %v", err)
	}
	got := patch.ReplaceBlocks[0]
	if len(got.EdgeAnchors) != 1 || got.EdgeAnchors[0].FromNode != "B" ||
		strings.Contains(got.Diagram.Body, "A->>B") || !strings.Contains(got.Diagram.Body, "B->>C: keep label") {
		t.Fatalf("ref-selected remove changed unmentioned graph content: %+v", got)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_StaleRefDoesNotRequireValidatorIdentityToEqualBaseAnchor(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    participant A\n    participant B\n    participant C\n    B->>C: keep label\n"
	// Production witness r803: the validator resolves broader participant
	// identities while the rejected anchor keeps its original technical
	// identities. The unique A->B precedence carrier is still exact.
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: diagramCallEdgeIssueAnchorWithoutBodyEdge,
			FromNode: "A", ToNode: "B", FromIdentity: "analyze", ToIdentity: "explorer",
			RelationKind: types.DiagramRelPrecedence,
		}}, nil)
	if lease == nil || len(lease.Failures) != 1 ||
		lease.Failures[0].TargetCarrier != types.AnswerDiagramRelationRepairCarrierStaleAnchor ||
		lease.Failures[0].FailureRef == "" {
		t.Fatalf("test setup missing exact stale-anchor capability: %+v", lease)
	}
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		FailureRef: lease.Failures[0].FailureRef, Action: "remove",
	}}, nil, lease)
	if err != nil {
		t.Fatalf("ref-selected stale metadata remove must not require a nonexistent body edge: %v", err)
	}
	got := patch.ReplaceBlocks[0]
	if len(got.EdgeAnchors) != 1 || got.EdgeAnchors[0].FromNode != "B" ||
		strings.Contains(got.Diagram.Body, "A->>B") || !strings.Contains(got.Diagram.Body, "B->>C: keep label") {
		t.Fatalf("stale metadata remove changed unmentioned graph content: %+v", got)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_StaleRefRestoresModelAuthoredEdgeAcrossIdentityProjection(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    participant A\n    participant B\n    participant C\n    B->>C: keep label\n"
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: diagramCallEdgeIssueAnchorWithoutBodyEdge,
			FromNode: "A", ToNode: "B", FromIdentity: "analyze", ToIdentity: "explorer",
			RelationKind: types.DiagramRelPrecedence,
		}}, nil)
	if lease == nil || len(lease.Failures) != 1 || lease.Failures[0].FailureRef == "" {
		t.Fatalf("test setup missing exact stale-anchor capability: %+v", lease)
	}
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		FailureRef: lease.Failures[0].FailureRef, Action: "replace",
		Edge: &types.DiagramEdgeAnchor{
			FromNode: "A", ToNode: "B", VisibleLabel: "确定范围后收集证据",
		},
	}}, nil, lease)
	if err != nil {
		t.Fatalf("ref-selected stale anchor replacement must stay executable: %v", err)
	}
	got := patch.ReplaceBlocks[0]
	if !strings.Contains(got.Diagram.Body, "A->>B: 确定范围后收集证据") ||
		!strings.Contains(got.Diagram.Body, "B->>C: keep label") ||
		len(got.EdgeAnchors) != 2 || got.EdgeAnchors[0].VisibleLabel != "确定范围后收集证据" ||
		got.EdgeAnchors[0].RelationKind != types.DiagramRelPrecedence ||
		got.EdgeAnchors[0].FromIdentity != "analyze" || got.EdgeAnchors[0].ToIdentity != "explorer" {
		t.Fatalf("stale replacement did not preserve model-authored visible edge and siblings: %+v", got)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_FailureRefAcceptsOnlyMatchingRedundantCoordinates(t *testing.T) {
	prev := atomicPatchTestDocument()
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: diagramCallEdgeIssueNoEvidence,
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence, BodyOccurrence: 1,
		}}, nil)
	if lease == nil || len(lease.Failures) != 1 || lease.Failures[0].FailureRef == "" {
		t.Fatalf("test setup missing live failure ref: %+v", lease)
	}
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		FailureRef: lease.Failures[0].FailureRef,
		BlockID:    "diag", BodyOccurrence: 1, Action: "remove",
		Match: &types.DiagramEdgeAnchor{
			FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelPrecedence,
		},
	}}, nil, lease)
	if err != nil {
		t.Fatalf("a live ref must absorb matching legacy coordinates without another model retry: %v", err)
	}
	if len(patch.ReplaceBlocks) != 1 || strings.Contains(patch.ReplaceBlocks[0].Diagram.Body, "A->>B") ||
		len(patch.ReplaceBlocks[0].EdgeAnchors) != 1 {
		t.Fatalf("matching redundant coordinates changed the ref-selected outcome: %+v", patch.ReplaceBlocks)
	}
}

func TestEmitAnswerDocumentPatch_LiveFailureRefsUseSameGenerationStagingBody(t *testing.T) {
	mut := types.NewMutableState("same-generation relation staging")
	accepted := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "accepted summary"},
		{ID: "flow", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram",
		}},
	}}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, accepted)
	staged := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "staged summary"},
		{ID: "flow", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramSequence, Language: "mermaid",
			Body: "sequenceDiagram\n    A->>B: first\n    B->>C: second",
		}},
	}}
	failures := []types.AnswerDiagramRelationRepairFailure{
		{BlockID: "flow", Issue: diagramCallEdgeIssueMissingAnchor, RelationKind: types.DiagramRelCall, FromNode: "A", ToNode: "B", FromIdentity: "A.Run", ToIdentity: "B.Handle"},
		{BlockID: "flow", Issue: diagramCallEdgeIssueMissingAnchor, RelationKind: types.DiagramRelCall, FromNode: "B", ToNode: "C", FromIdentity: "B.Handle", ToIdentity: "C.Store"},
	}
	lease := types.NewAnswerDiagramRelationRepairLease(staged, failures, nil)
	if lease == nil || len(lease.Failures) != 2 {
		t.Fatalf("expected two live failures: %+v", lease)
	}
	mut.SetPendingAnswerDocumentPatchBase(staged)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	params := fmt.Sprintf(`{
		"unchanged_block_ids":["summary"],
		"diagram_edge_edits":[
			{"failure_ref":%q,"action":"remove"},
			{"failure_ref":%q,"action":"remove"}
		]
	}`, lease.Failures[0].FailureRef, lease.Failures[1].FailureRef)

	res, err := (&EmitAnswerDocumentPatch{}).Execute(&types.BusContext{Mutable: mut}, json.RawMessage(params))
	if err != nil || !res.Success {
		t.Fatalf("all refs must execute against their staged generation: res=%+v err=%v", res, err)
	}
	got := mut.AnswerDocumentV2()
	if got == nil || got.Blocks[0].Text != "staged summary" || len(mermaidcompat.ParseEdges(got.Blocks[1].Diagram.Body)) != 0 {
		t.Fatalf("accepted document must be staged generation with only selected edges removed: %+v", got)
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

func TestApplyModelAuthoredDiagramAtomicEdits_FailureRefRemovesGroundedBodyOnlyEdge(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    participant O\n    participant Ctx\n    O->>Ctx: build context\n"
	prev.Blocks[1].EdgeAnchors = nil
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: diagramCallEdgeIssueMissingGroundedAnchor,
			FromNode: "O", ToNode: "Ctx", FromIdentity: "Orchestrator.Run", ToIdentity: "BuildContext",
			RelationKind: types.DiagramRelCall,
		}}, nil)
	if lease == nil || lease.Failures[0].TargetCarrier != types.AnswerDiagramRelationRepairCarrierVisibleBodyEdge ||
		!lease.Failures[0].AllowsAction("remove") {
		t.Fatalf("grounded missing-anchor row must publish its executable body carrier: %+v", lease)
	}
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		FailureRef: lease.Failures[0].FailureRef, Action: "remove",
	}}, nil, lease)
	if err != nil {
		t.Fatalf("grounded missing-anchor failure ref must remove its exact visible body edge: %v", err)
	}
	if got := patch.ReplaceBlocks[0].Diagram.Body; strings.Contains(got, "O->>Ctx") {
		t.Fatalf("body-only grounded ref did not remove the named visible edge: %s", got)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_FailureRefsSelectAndRemoveRepeatedBodyEdges(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n" +
		"    participant DC\n" +
		"    participant BC\n" +
		"    DC->>BC: first\n" +
		"    DC->>BC: second\n" +
		"    DC->>BC: third\n" +
		"    DC->>BC: fourth\n"
	prev.Blocks[1].EdgeAnchors = nil
	failures := make([]types.AnswerDiagramRelationRepairFailure, 0, 4)
	for occurrence := 1; occurrence <= 4; occurrence++ {
		failures = append(failures, types.AnswerDiagramRelationRepairFailure{
			BlockID: "diag", Issue: diagramCallEdgeIssueMissingAnchor,
			FromNode: "DC", ToNode: "BC", RelationKind: types.DiagramRelCall,
			BodyOccurrence: occurrence,
		})
	}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, failures, nil)
	if lease == nil || len(lease.Failures) != 4 {
		t.Fatalf("each repeated visible relation needs one executable carrier: %+v", lease)
	}
	edits := make([]emitAnswerDiagramEdgeEdit, 0, len(lease.Failures))
	seen := make(map[int]bool)
	for _, failure := range lease.Failures {
		if failure.FailureRef == "" || failure.TargetCarrier != types.AnswerDiagramRelationRepairCarrierVisibleBodyEdge ||
			!failure.AllowsAction("remove") || failure.BodyOccurrence < 1 || failure.BodyOccurrence > 4 ||
			seen[failure.BodyOccurrence] {
			t.Fatalf("repeated relation failure lacks an exact independent capability: %+v", failure)
		}
		seen[failure.BodyOccurrence] = true
		edits = append(edits, emitAnswerDiagramEdgeEdit{FailureRef: failure.FailureRef, Action: "remove"})
	}

	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	if err := applyModelAuthoredDiagramAtomicEdits(prev, patch, edits, nil, lease); err != nil {
		t.Fatalf("failure refs must select their base occurrences without model-authored coordinates: %v", err)
	}
	if got := mermaidcompat.ParseEdges(patch.ReplaceBlocks[0].Diagram.Body); len(got) != 0 {
		t.Fatalf("all four selected repeated relations should be removed, got %+v\n%s", got, patch.ReplaceBlocks[0].Diagram.Body)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_RepeatedPriorAnchorsReceiveExecutableBodyRefs(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n" +
		"    participant A\n" +
		"    participant B\n" +
		"    A->>B: first unproven flow\n" +
		"    A->>B: second unproven flow\n"
	prev.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{
		{FromNode: "A", ToNode: "B", FromIdentity: "applyStageOutput", ToIdentity: "BusContext",
			RelationKind: types.DiagramRelDataFlow, VisibleLabel: "first unproven flow"},
		{FromNode: "A", ToNode: "B", FromIdentity: "applyStageOutput", ToIdentity: "BusContext",
			RelationKind: types.DiagramRelDataFlow, VisibleLabel: "second unproven flow"},
	}
	failures := []types.AnswerDiagramRelationRepairFailure{
		{BlockID: "diag", Issue: diagramDataFlowEdgeIssueNoEvidence,
			FromNode: "A", ToNode: "B", FromIdentity: "applyStageOutput", ToIdentity: "BusContext",
			RelationKind: types.DiagramRelDataFlow, BodyOccurrence: 1},
		{BlockID: "diag", Issue: diagramDataFlowEdgeIssueNoEvidence,
			FromNode: "A", ToNode: "B", FromIdentity: "applyStageOutput", ToIdentity: "BusContext",
			RelationKind: types.DiagramRelDataFlow, BodyOccurrence: 2},
	}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, failures, nil)
	if lease == nil || len(lease.Failures) != 2 {
		t.Fatalf("repeated prior-anchor failures must produce one live capability per body occurrence: %+v", lease)
	}
	seenRefs := make(map[string]bool)
	edits := make([]emitAnswerDiagramEdgeEdit, 0, len(lease.Failures))
	for _, failure := range lease.Failures {
		if failure.TargetCarrier != types.AnswerDiagramRelationRepairCarrierVisibleBodyEdge ||
			!failure.AllowsAction("remove") || len(failure.AllowedActions) != 1 ||
			failure.BodyOccurrence < 1 || failure.BodyOccurrence > 2 ||
			failure.FailureRef == "" || seenRefs[failure.FailureRef] {
			t.Fatalf("repeated prior anchor published a non-executable or ambiguous ref: %+v", failure)
		}
		seenRefs[failure.FailureRef] = true
		edits = append(edits, emitAnswerDiagramEdgeEdit{FailureRef: failure.FailureRef, Action: "remove"})
	}

	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	if err := applyModelAuthoredDiagramAtomicEdits(prev, patch, edits, nil, lease); err != nil {
		t.Fatalf("occurrence-bound repeated prior-anchor refs must execute atomically: %v", err)
	}
	if len(patch.ReplaceBlocks) != 1 {
		t.Fatalf("compiled replacements=%d, want one diagram", len(patch.ReplaceBlocks))
	}
	got := patch.ReplaceBlocks[0]
	if len(got.EdgeAnchors) != 0 || len(mermaidcompat.ParseEdges(got.Diagram.Body)) != 0 {
		t.Fatalf("both selected body and anchor occurrences must be removed exactly: anchors=%+v\n%s", got.EdgeAnchors, got.Diagram.Body)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_FailureRefRejectsUnlistedAction(t *testing.T) {
	prev := atomicPatchTestDocument()
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: diagramCallEdgeIssueNoEvidence,
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence,
		}}, nil)
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		FailureRef: lease.Failures[0].FailureRef, Action: "relabel", VisibleLabel: "wording only",
	}}, nil, lease)
	if err == nil || !strings.Contains(err.Error(), "does not allow action=relabel") ||
		!strings.Contains(err.Error(), "allowed_actions=[remove]") {
		t.Fatalf("relation-evidence failure must expose its exact executable actions: %v", err)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_LabelPairRefRelabelsOnlyDisplaySurfaces(t *testing.T) {
	prev := atomicPatchTestDocument()
	baseAnchor := prev.Blocks[1].EdgeAnchors[0]
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: diagramTypedRecipeMissingVisibleLabel,
			FromNode: baseAnchor.FromNode, ToNode: baseAnchor.ToNode,
			FromIdentity: baseAnchor.FromIdentity, ToIdentity: baseAnchor.ToIdentity,
			RelationKind: baseAnchor.RelationKind, BodyOccurrence: 1,
		}}, nil)
	if lease == nil || len(lease.Failures) != 1 ||
		lease.Failures[0].TargetCarrier != types.AnswerDiagramRelationRepairCarrierLabelPair ||
		!lease.Failures[0].AllowsAction("relabel") {
		t.Fatalf("test setup did not produce a label-pair ref: %+v", lease)
	}
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	if err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		FailureRef: lease.Failures[0].FailureRef, Action: "relabel", VisibleLabel: "模型选择的业务标签",
	}}, nil, lease); err != nil {
		t.Fatalf("label-pair ref must execute without legacy hidden coordinates: %v", err)
	}
	if len(patch.ReplaceBlocks) != 1 {
		t.Fatalf("label-pair edit must replace only its carrier: %+v", patch.ReplaceBlocks)
	}
	got := patch.ReplaceBlocks[0]
	if !strings.Contains(got.Diagram.Body, "模型选择的业务标签") || len(got.EdgeAnchors) != len(prev.Blocks[1].EdgeAnchors) ||
		got.EdgeAnchors[0].VisibleLabel != "模型选择的业务标签" ||
		got.EdgeAnchors[0].FromIdentity != baseAnchor.FromIdentity ||
		got.EdgeAnchors[0].ToIdentity != baseAnchor.ToIdentity ||
		got.EdgeAnchors[0].RelationKind != baseAnchor.RelationKind ||
		got.EdgeAnchors[1] != prev.Blocks[1].EdgeAnchors[1] {
		t.Fatalf("relabel must change only the selected body/anchor display wording: %+v", got)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_LabelPairRefCannotPruneGroundedCarrier(t *testing.T) {
	prev := atomicPatchTestDocument()
	baseAnchor := prev.Blocks[1].EdgeAnchors[0]
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: diagramTypedRecipeMissingVisibleLabel,
			FromNode: baseAnchor.FromNode, ToNode: baseAnchor.ToNode,
			FromIdentity: baseAnchor.FromIdentity, ToIdentity: baseAnchor.ToIdentity,
			RelationKind: baseAnchor.RelationKind, BodyOccurrence: 1,
		}}, nil)
	if lease == nil || lease.Failures[0].AllowsAction("remove") || !lease.Failures[0].AllowsAction("relabel") {
		t.Fatalf("presentation-only label pair must remain relabel-only: %+v", lease)
	}
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		FailureRef: lease.Failures[0].FailureRef, Action: "remove",
	}}, nil, lease)
	if err == nil || !strings.Contains(err.Error(), "does not allow action=remove") {
		t.Fatalf("presentation-only repair must not authorize relation deletion: %v", err)
	}
	if len(patch.ReplaceBlocks) != 0 {
		t.Fatalf("rejected presentation deletion mutated the patch: %+v", patch.ReplaceBlocks)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_OneRefRepairsSeveralIssuesOnSameCarrier(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    participant IR\n    participant Bus\n    participant C\n    IR-->>Bus: stores result\n    Bus->>C: keep\n"
	prev.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{
		{FromNode: "IR", ToNode: "Bus", FromIdentity: "AnalysisIR", ToIdentity: "BusContext", RelationKind: types.DiagramRelAssignment},
		{FromNode: "Bus", ToNode: "C", RelationKind: types.DiagramRelCall},
	}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{
		{BlockID: "diag", Issue: diagramAssignmentEdgeIssueNoEvidence, RelationKind: types.DiagramRelAssignment, FromNode: "IR", ToNode: "Bus", FromIdentity: "AnalysisIR", ToIdentity: "BusContext"},
		{BlockID: "diag", Issue: diagramSequenceRelationReplyConflict, RelationKind: types.DiagramRelAssignment, FromNode: "IR", ToNode: "Bus", FromIdentity: "AnalysisIR", ToIdentity: "BusContext"},
	}, nil)
	if lease == nil || len(lease.Failures) != 1 || len(lease.Failures[0].RelatedIssues) != 2 {
		t.Fatalf("same carrier must expose one live ref with both issues: %+v", lease)
	}
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		FailureRef: lease.Failures[0].FailureRef, Action: "remove",
	}}, nil, lease)
	if err != nil {
		t.Fatalf("one model-selected operation must repair the shared carrier: %v", err)
	}
	got := patch.ReplaceBlocks[0]
	if strings.Contains(got.Diagram.Body, "IR-->>Bus") || !strings.Contains(got.Diagram.Body, "Bus->>C: keep") || len(got.EdgeAnchors) != 1 {
		t.Fatalf("coalesced repair changed an unrelated relation: %+v\n%s", got.EdgeAnchors, got.Diagram.Body)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_MixedCarrierBatchRemainsExecutable(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n" +
		"    Run->>Phase: keep\n" +
		"    Analyzer-->>IR: unsupported return\n" +
		"    IR-->>Bus: unsupported assignment\n" +
		"    Bus->>Task: unanchored call\n" +
		"    Loop->>Explorer: unsupported ordering\n" +
		"    Explorer-->>Bus: unanchored result\n"
	prev.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{
		{FromNode: "Run", ToNode: "Phase", RelationKind: types.DiagramRelCall},
		{FromNode: "Analyzer", ToNode: "IR", RelationKind: types.DiagramRelReturn},
		{FromNode: "IR", ToNode: "Bus", RelationKind: types.DiagramRelAssignment},
		{FromNode: "Run", ToNode: "Task", RelationKind: types.DiagramRelCall},
		{FromNode: "Loop", ToNode: "Explorer", RelationKind: types.DiagramRelPrecedence},
	}
	failures := []types.AnswerDiagramRelationRepairFailure{
		{BlockID: "diag", Issue: diagramReturnEdgeIssueNoEvidence, RelationKind: types.DiagramRelReturn, FromNode: "Analyzer", ToNode: "IR"},
		{BlockID: "diag", Issue: diagramAssignmentEdgeIssueNoEvidence, RelationKind: types.DiagramRelAssignment, FromNode: "IR", ToNode: "Bus"},
		{BlockID: "diag", Issue: diagramSequenceRelationReplyConflict, RelationKind: types.DiagramRelAssignment, FromNode: "IR", ToNode: "Bus"},
		{BlockID: "diag", Issue: diagramCallEdgeIssueMissingGroundedAnchor, RelationKind: types.DiagramRelCall, FromNode: "Bus", ToNode: "Task"},
		{BlockID: "diag", Issue: diagramCallEdgeIssueAnchorWithoutBodyEdge, RelationKind: types.DiagramRelCall, FromNode: "Run", ToNode: "Task"},
		{BlockID: "diag", Issue: diagramSemanticRelationIssueNoEvidence, RelationKind: types.DiagramRelPrecedence, FromNode: "Loop", ToNode: "Explorer"},
		{BlockID: "diag", Issue: diagramCallEdgeIssueMissingAnchor, RelationKind: types.DiagramRelCall, FromNode: "Explorer", ToNode: "Bus"},
	}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, failures, nil)
	if lease == nil || len(lease.Failures) != 6 {
		t.Fatalf("seven issues on six carriers must compile to six live operations: %+v", lease)
	}
	edits := make([]emitAnswerDiagramEdgeEdit, 0, len(lease.Failures))
	for _, failure := range lease.Failures {
		if !failure.AllowsAction("remove") {
			t.Fatalf("mixed carrier unexpectedly lacks remove capability: %+v", failure)
		}
		edits = append(edits, emitAnswerDiagramEdgeEdit{FailureRef: failure.FailureRef, Action: "remove"})
	}
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	if err := applyModelAuthoredDiagramAtomicEdits(prev, patch, edits, nil, lease); err != nil {
		t.Fatalf("one operation per exact carrier must not become stale within the transaction: %v", err)
	}
	got := patch.ReplaceBlocks[0]
	if !strings.Contains(got.Diagram.Body, "Run->>Phase: keep") || len(got.EdgeAnchors) != 1 || got.EdgeAnchors[0].FromNode != "Run" || got.EdgeAnchors[0].ToNode != "Phase" {
		t.Fatalf("mixed repair failed to preserve the unlisted healthy edge: %+v\n%s", got.EdgeAnchors, got.Diagram.Body)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_AmbiguousCarrierDoesNotMintLocalLease(t *testing.T) {
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
	if lease != nil {
		t.Fatalf("identity drift must fail before a dead local capability reaches the executor: %+v", lease)
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
		{name: "duplicate consumption", edits: []emitAnswerDiagramEdgeEdit{{FailureRef: ref, Action: "remove"}, {FailureRef: ref, Action: "remove"}}, want: "reuses failure_ref"},
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

func TestApplyModelAuthoredDiagramAtomicEdits_FailureRefQuarantinesSelectorMirrors(t *testing.T) {
	prev := atomicPatchTestDocument()
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "semantic_relation_edge_unproven",
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence,
		}}, nil)
	ref := lease.Failures[0].FailureRef
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		FailureRef: ref, BlockID: "diag", Action: "remove", Occurrence: 17, BodyOccurrence: 23,
		Match: &types.DiagramEdgeAnchor{
			FromNode: "legacy-X", ToNode: "legacy-Y", FromIdentity: "legacy-source",
			ToIdentity: "legacy-target", RelationKind: types.DiagramRelCall,
		},
	}}, nil, lease)
	if err != nil || len(patch.ReplaceBlocks) != 1 {
		t.Fatalf("live ref must compile after quarantining selector mirrors: err=%v patch=%+v", err, patch)
	}
	if strings.Contains(patch.ReplaceBlocks[0].Diagram.Body, "A->>B") ||
		len(patch.ReplaceBlocks[0].EdgeAnchors) != 1 {
		t.Fatalf("only the exact ref-owned carrier should be removed: %+v", patch.ReplaceBlocks[0])
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
			{"block_id":"diag","action":"add","to_node_visible_label":"答案组织阶段","edge":{"from_node":"C","to_node":"F","from_identity":"Extractor","to_identity":"Finalizer","relation_kind":"precedence","visible_label":"结构化事实就绪后组织答案"}}
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
		FromIdentity: "Extractor", ToIdentity: "Finalizer",
		FromNodeIDs: []string{"BusinessExtract"}, ToNodeIDs: []string{"BusinessFinalize"},
		Source: "stageauthority",
	}
	recipe := types.DiagramEdgeAnchor{
		FromNode: "C", ToNode: "F", FromIdentity: "Extractor", ToIdentity: "Finalizer",
		RelationKind: types.DiagramRelPrecedence,
	}
	raw := json.RawMessage(`{
		"unchanged_block_ids":["summary"],
		"diagram_edge_edits":[{"block_id":"diag","action":"add","to_node_visible_label":"答案组织阶段","edge":{
			"from_node":"C","to_node":"F","relation_kind":"precedence","visible_label":"结构化事实就绪后组织答案"
		}}]
	}`)

	t.Run("live addition ref removes recipe dialect dependency", func(t *testing.T) {
		bus := newBus(allowed, nil)
		lease := bus.Mutable.AnswerDiagramRelationRepairLease()
		if lease == nil || len(lease.AllowedAdditions) != 1 || lease.AllowedAdditions[0].AdditionRef == "" {
			t.Fatalf("expected a referenced live addition: %+v", lease)
		}
		params := fmt.Sprintf(`{
			"unchanged_block_ids":["summary"],
			"diagram_edge_edits":[{"action":"add","addition_ref":%q,
				"from_node_visible_label":"事实提炼阶段","to_node_visible_label":"答案组织阶段","edge":{
				"from_node":"BusinessExtract","to_node":"BusinessFinalize","visible_label":"结构化事实就绪后组织答案"
			}}]
		}`, lease.AllowedAdditions[0].AdditionRef)
		res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(params))
		if err != nil || !res.Success {
			t.Fatalf("selected live candidate must not depend on guessing a recipe node dialect: err=%v res=%+v", err, res)
		}
		doc := bus.Mutable.AnswerDocumentV2()
		if doc == nil || len(doc.Blocks) != 2 || len(doc.Blocks[1].EdgeAnchors) != 3 {
			t.Fatalf("unexpected persisted document: %+v", doc)
		}
		got := doc.Blocks[1].EdgeAnchors[2]
		if got.FromNode != "BusinessExtract" || got.ToNode != "BusinessFinalize" ||
			got.FromIdentity != "Extractor" || got.ToIdentity != "Finalizer" ||
			got.VisibleLabel != "结构化事实就绪后组织答案" {
			t.Fatalf("addition ref changed visible authorship or lost its hidden tuple: %+v", got)
		}
	})

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
		"diagram_edge_edits":[{"block_id":"diag","action":"add","to_node_visible_label":"答案组织阶段","edge":{"from_node":"C","to_node":"F","from_identity":"Extractor","to_identity":"Finalizer","relation_kind":"precedence","visible_label":"组织答案"}}]
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

func TestPriorAnchorWithoutUniqueRepeatedBodyOccurrencePublishesAnchorOnlyCapability(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n" +
		"    participant TASK as runTaskPhase\n" +
		"    participant DISP as dispatchStage\n" +
		"    TASK->>DISP: execute StageExplore\n" +
		"    TASK->>DISP: execute StageExtract\n" +
		"    TASK->>DISP: execute StageFinalize\n"
	prev.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "TASK", ToNode: "DISP", FromIdentity: "runTaskPhase", ToIdentity: "dispatchStage",
		// Even an exact message-label match cannot mint a hard occurrence:
		// visible model wording is outside the structural locator contract.
		RelationKind: types.DiagramRelPrecedence, VisibleLabel: "execute StageExplore",
	}}
	failure := bindDiagramRelationRepairAnchorBodyCarrier(prev, types.AnswerDiagramRelationRepairFailure{
		BlockID: "diag", Issue: diagramSemanticRelationIssueNoEvidence,
		FromNode: "TASK", ToNode: "DISP", FromIdentity: "runTaskPhase", ToIdentity: "dispatchStage",
		RelationKind: types.DiagramRelPrecedence,
	})
	if failure.BodyOccurrence != 0 ||
		failure.TargetCarrier != types.AnswerDiagramRelationRepairCarrierPriorAnchorMetadata {
		t.Fatalf("aggregate prior anchor must not guess one of three visible occurrences: %+v", failure)
	}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{failure}, nil)
	if lease == nil || len(lease.Failures) != 1 ||
		lease.Failures[0].TargetCarrier != types.AnswerDiagramRelationRepairCarrierPriorAnchorMetadata ||
		!lease.Failures[0].AllowsAction("remove") || lease.Failures[0].AllowsAction("replace") {
		t.Fatalf("metadata-only prior anchor must publish one executable remove-only ref: %+v", lease)
	}

	originalBody := prev.Blocks[1].Diagram.Body
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	if err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		FailureRef: lease.Failures[0].FailureRef, Action: "remove",
	}}, nil, lease); err != nil {
		t.Fatalf("remove-only prior anchor ref must not ask for body_occurrence: %v", err)
	}
	got := patch.ReplaceBlocks[0]
	if got.Diagram.Body != originalBody || len(got.EdgeAnchors) != 0 {
		t.Fatalf("metadata-only remove must preserve every visible edge and remove only the selected anchor: anchors=%+v\n%s", got.EdgeAnchors, got.Diagram.Body)
	}
}

func TestPriorAnchorMetadataAndRepeatedVisibleFailuresApplyInOneAtomicPatch(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n" +
		"    participant TASK as runTaskPhase\n" +
		"    participant DISP as dispatchStage\n" +
		"    TASK->>DISP: execute StageExplore\n" +
		"    TASK->>DISP: execute StageExtract\n" +
		"    TASK->>DISP: execute StageFinalize\n"
	prev.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "TASK", ToNode: "DISP", FromIdentity: "runTaskPhase", ToIdentity: "dispatchStage",
		RelationKind: types.DiagramRelPrecedence, VisibleLabel: "遍历其余阶段并分发",
	}}
	anchorFailure := bindDiagramRelationRepairAnchorBodyCarrier(prev, types.AnswerDiagramRelationRepairFailure{
		BlockID: "diag", Issue: diagramSemanticRelationIssueNoEvidence,
		FromNode: "TASK", ToNode: "DISP", FromIdentity: "runTaskPhase", ToIdentity: "dispatchStage",
		RelationKind: types.DiagramRelPrecedence,
	})
	failures := []types.AnswerDiagramRelationRepairFailure{anchorFailure}
	for occurrence := 1; occurrence <= 3; occurrence++ {
		failures = append(failures, types.AnswerDiagramRelationRepairFailure{
			BlockID: "diag", Issue: diagramCallEdgeIssueMissingAnchor,
			FromNode: "TASK", ToNode: "DISP", RelationKind: types.DiagramRelCall,
			BodyOccurrence: occurrence,
		})
	}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, failures, nil)
	if lease == nil || len(lease.Failures) != 4 {
		t.Fatalf("anchor metadata plus three visible failures must stay independently executable: %+v", lease)
	}
	edits := make([]emitAnswerDiagramEdgeEdit, 0, len(lease.Failures))
	for _, failure := range lease.Failures {
		if !failure.AllowsAction("remove") {
			t.Fatalf("fixture failure must expose model-selectable remove: %+v", failure)
		}
		edits = append(edits, emitAnswerDiagramEdgeEdit{FailureRef: failure.FailureRef, Action: "remove"})
	}
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	if err := applyModelAuthoredDiagramAtomicEdits(prev, patch, edits, nil, lease); err != nil {
		t.Fatalf("one atomic patch must execute exact anchor and body carriers without coordinate retyping: %v", err)
	}
	got := patch.ReplaceBlocks[0]
	if len(got.EdgeAnchors) != 0 || len(mermaidcompat.ParseEdges(got.Diagram.Body)) != 0 {
		t.Fatalf("all model-selected failed carriers should be removed exactly: anchors=%+v\n%s", got.EdgeAnchors, got.Diagram.Body)
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

func TestApplyModelAuthoredDiagramAtomicEdits_AbsorbsRedundantUnchangedCarrier(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    A->>B: first\n"
	prev.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall}}
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary", "diag"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		BlockID: "diag", Action: "remove",
		Match: &types.DiagramEdgeAnchor{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall},
	}}, nil, nil)
	if err != nil {
		t.Fatalf("same-block unchanged assertion must be absorbed by the atomic compiler: %v", err)
	}
	if len(patch.UnchangedBlockIDs) != 1 || patch.UnchangedBlockIDs[0] != "summary" {
		t.Fatalf("only the atomically edited block should leave unchanged_block_ids: %+v", patch.UnchangedBlockIDs)
	}
	if len(patch.ReplaceBlocks) != 1 || strings.Contains(patch.ReplaceBlocks[0].Diagram.Body, "A->>B") || len(patch.ReplaceBlocks[0].EdgeAnchors) != 0 {
		t.Fatalf("atomic edit was not compiled over the immutable base: %+v", patch.ReplaceBlocks)
	}
}

func TestEmitAnswerDocumentPatch_RemovesEverySelectedAnchorSharingOneVisibleBodyOccurrence(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    participant Phase\n    participant Fin\n    Phase->>Fin: hand off result\n"
	prev.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{
		{FromNode: "Phase", ToNode: "Fin", FromIdentity: "Phase.run", ToIdentity: "Fin.accept", RelationKind: types.DiagramRelCall},
		{FromNode: "Phase", ToNode: "Fin", FromIdentity: "analyzer", ToIdentity: "finalizer", RelationKind: types.DiagramRelPrecedence},
	}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{
		{BlockID: "diag", Issue: "call_edge_unproven", FromNode: "Phase", ToNode: "Fin", FromIdentity: "Phase.run", ToIdentity: "Fin.accept", RelationKind: types.DiagramRelCall, BodyOccurrence: 1},
		{BlockID: "diag", Issue: "semantic_relation_edge_unproven", FromNode: "Phase", ToNode: "Fin", FromIdentity: "analyzer", ToIdentity: "finalizer", RelationKind: types.DiagramRelPrecedence, BodyOccurrence: 1},
	}, nil)
	if lease == nil || len(lease.Failures) != 2 {
		t.Fatalf("two typed claims sharing one visible line must retain two model-selectable refs: %+v", lease)
	}
	mut := types.NewMutableState("shared visible relation carrier")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	params := json.RawMessage(fmt.Sprintf(`{
		"unchanged_block_ids":["summary"],
		"diagram_edge_edits":[
			{"failure_ref":%q,"action":"remove"},
			{"failure_ref":%q,"action":"remove"}
		]
	}`, lease.Failures[0].FailureRef, lease.Failures[1].FailureRef))
	res, err := (&EmitAnswerDocumentPatch{}).Execute(&types.BusContext{Mutable: mut}, params)
	if err != nil || !res.Success {
		t.Fatalf("all model-selected refs on one body occurrence must close in one transaction: err=%v res=%+v", err, res)
	}
	got := mut.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 2 || len(got.Blocks[1].EdgeAnchors) != 0 ||
		strings.Contains(got.Blocks[1].Diagram.Body, "Phase->>Fin") {
		t.Fatalf("shared statement must be removed once with both selected anchors: %+v", got)
	}
}

func TestEmitAnswerDocumentPatch_RemovesSharedLabelAnchorAndUnanchoredVisibleRelation(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    participant A\n    participant B\n    A->>B: model wording\n"
	prev.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "A", ToNode: "B", FromIdentity: "State.Value", ToIdentity: "Bus.Value",
		RelationKind: types.DiagramRelAssignment, VisibleLabel: "model wording",
	}}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{
		{BlockID: "diag", Issue: "diagram_visible_label_mismatch", FromNode: "A", ToNode: "B",
			FromIdentity: "State.Value", ToIdentity: "Bus.Value", RelationKind: types.DiagramRelAssignment, BodyOccurrence: 1},
		{BlockID: "diag", Issue: "missing_call_anchor", FromNode: "A", ToNode: "B",
			FromIdentity: "Caller.Run", ToIdentity: "Callee.Handle", RelationKind: types.DiagramRelCall, BodyOccurrence: 1},
	}, nil)
	if lease == nil || len(lease.Failures) != 2 {
		t.Fatalf("shared statement must publish both exact semantic targets: %+v", lease)
	}
	mut := types.NewMutableState("shared label and visible relation carrier")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	params := json.RawMessage(fmt.Sprintf(`{
		"unchanged_block_ids":["summary"],
		"diagram_edge_edits":[
			{"failure_ref":%q,"action":"remove"},
			{"failure_ref":%q,"action":"remove"}
		]
	}`, lease.Failures[0].FailureRef, lease.Failures[1].FailureRef))
	res, err := (&EmitAnswerDocumentPatch{}).Execute(&types.BusContext{Mutable: mut}, params)
	if err != nil || !res.Success {
		t.Fatalf("jointly advertised shared-body removal must execute atomically: err=%v res=%+v", err, res)
	}
	got := mut.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 2 || len(got.Blocks[1].EdgeAnchors) != 0 ||
		strings.Contains(got.Blocks[1].Diagram.Body, "A->>B") {
		t.Fatalf("shared statement and its exact prior anchor must be removed once: %+v", got)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_SharedBodyRejectsMixedActions(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    A->>B: shared\n"
	prev.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{
		{FromNode: "A", ToNode: "B", FromIdentity: "A.call", ToIdentity: "B.run", RelationKind: types.DiagramRelCall},
		{FromNode: "A", ToNode: "B", FromIdentity: "analyzer", ToIdentity: "explorer", RelationKind: types.DiagramRelPrecedence},
	}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{
		{BlockID: "diag", Issue: "call_edge_unproven", FromNode: "A", ToNode: "B", FromIdentity: "A.call", ToIdentity: "B.run", RelationKind: types.DiagramRelCall, BodyOccurrence: 1},
		{BlockID: "diag", Issue: diagramSequenceRelationReplyConflict, FromNode: "A", ToNode: "B", FromIdentity: "analyzer", ToIdentity: "explorer", RelationKind: types.DiagramRelPrecedence, BodyOccurrence: 1},
	}, nil)
	edits := []emitAnswerDiagramEdgeEdit{
		{FailureRef: lease.Failures[0].FailureRef, Action: "remove"},
		{FailureRef: lease.Failures[1].FailureRef, Action: "replace", Edge: &types.DiagramEdgeAnchor{
			FromNode: "A", ToNode: "B", FromIdentity: "analyzer", ToIdentity: "explorer",
			RelationKind: types.DiagramRelPrecedence, VisibleLabel: "model replacement",
		}},
	}
	err := applyModelAuthoredDiagramAtomicEdits(prev, &types.AnswerDocumentV2Patch{}, edits, nil, lease)
	if err == nil || !strings.Contains(err.Error(), "permit only model-selected action=remove") {
		t.Fatalf("overlapping remove/replace must remain fail-closed instead of choosing for the model: %v", err)
	}
}

func TestEmitAnswerDocumentPatch_NonDiagramFailureRefRemovesOnlyExactAnchorMetadata(t *testing.T) {
	anchor := types.DiagramEdgeAnchor{
		FromNode: "Phase", ToNode: "Fin", FromIdentity: "analyzer", ToIdentity: "finalizer",
		RelationKind: types.DiagramRelPrecedence,
	}
	prev := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "keep summary"},
		{ID: "ol1", Kind: types.BlockOrderedList, Title: "keep title", Text: "keep text", SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{{ID: "step", Label: "keep label", Text: "keep item"}}, EdgeAnchors: []types.DiagramEdgeAnchor{anchor}},
	}}
	failure := bindDiagramRelationRepairAnchorBodyCarrier(prev, types.AnswerDiagramRelationRepairFailure{
		BlockID: "ol1", Issue: "semantic_relation_edge_unproven",
		FromNode: anchor.FromNode, ToNode: anchor.ToNode, FromIdentity: anchor.FromIdentity, ToIdentity: anchor.ToIdentity,
		RelationKind: anchor.RelationKind,
	})
	if failure.TargetCarrier != types.AnswerDiagramRelationRepairCarrierPriorAnchorMetadata {
		t.Fatalf("non-diagram relation metadata must publish remove-only metadata capability: %+v", failure)
	}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{failure}, nil)
	if lease == nil || len(lease.Failures) != 1 || !lease.Failures[0].AllowsAction("remove") || lease.Failures[0].AllowsAction("replace") {
		t.Fatalf("non-diagram metadata lease is not exactly executable: %+v", lease)
	}
	mut := types.NewMutableState("non diagram relation metadata")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	params := json.RawMessage(fmt.Sprintf(`{
		"unchanged_block_ids":["summary"],
		"diagram_edge_edits":[{"failure_ref":%q,"action":"remove"}]
	}`, lease.Failures[0].FailureRef))
	res, err := (&EmitAnswerDocumentPatch{}).Execute(&types.BusContext{Mutable: mut}, params)
	if err != nil || !res.Success {
		t.Fatalf("exact non-diagram metadata removal must pass production executor: err=%v res=%+v", err, res)
	}
	got := mut.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 2 {
		t.Fatalf("patched document missing: %+v", got)
	}
	block := got.Blocks[1]
	if block.Kind != types.BlockOrderedList || block.Title != "keep title" || block.Text != "keep text" ||
		len(block.Items) != 1 || block.Items[0].Label != "keep label" || block.Items[0].Text != "keep item" ||
		len(block.EdgeAnchors) != 0 {
		t.Fatalf("metadata-only edit changed visible ordered-list content: %+v", block)
	}
}

func TestEmitAnswerDocumentPatch_NonDiagramFailureAndCandidateRefsRebindOnlyExactAnchorMetadata(t *testing.T) {
	anchor := types.DiagramEdgeAnchor{
		FromNode: "caller", ToNode: "callee", RelationKind: types.DiagramRelCall,
		VisibleLabel: "model-authored business wording",
	}
	prev := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "keep summary"},
		{ID: "chain", Kind: types.BlockOrderedList, Title: "keep title", Text: "keep text", SurfaceRole: types.SurfacePrincipal,
			ClaimUses:   []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
			Items:       []types.AnswerBlockItem{{ID: "step", Label: "keep label", Text: "keep item"}},
			EdgeAnchors: []types.DiagramEdgeAnchor{anchor}},
	}}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "chain", Issue: diagramStandaloneRelationIdentityMissing,
		FromNode: anchor.FromNode, ToNode: anchor.ToNode, RelationKind: anchor.RelationKind,
		TargetCarrier: types.AnswerDiagramRelationRepairCarrierPriorAnchorMetadata,
	}}, []types.AnswerDiagramRelationRepairCandidate{{
		BlockID: "chain", RelationKind: types.DiagramRelCall,
		FromIdentity: "pkg.Caller.run", ToIdentity: "pkg.Callee.accept", Source: "src/call.go:10",
	}})
	if lease == nil || len(lease.Failures) != 1 || len(lease.AllowedAdditions) != 1 ||
		!lease.Failures[0].AllowsAction("attach") {
		t.Fatalf("test setup missing standalone metadata attach capability: %+v", lease)
	}
	mut := types.NewMutableState("non diagram metadata attach")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	ctx := &types.BusContext{
		Mutable: mut,
		EvidenceItems: []types.EvidenceItem{{
			ID: "ev-call", Kind: types.EvidenceRelationship, Subject: "pkg.Caller.run", Object: "pkg.Callee.accept",
			Predicate: "calls", Source: "src/call.go", LineStart: 10, LineEnd: 10,
			AnchorKind: types.AnchorCall, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
		}},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentTrace, PredicateAxis: types.AxisCall,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
		}},
	}
	params := json.RawMessage(fmt.Sprintf(`{
		"unchanged_block_ids":["summary"],
		"diagram_edge_edits":[{"failure_ref":%q,"addition_ref":%q,"action":"attach"}]
	}`, lease.Failures[0].FailureRef, lease.AllowedAdditions[0].AdditionRef))
	res, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("exact standalone metadata attach must pass production executor: err=%v res=%+v", err, res)
	}
	got := mut.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 2 {
		t.Fatalf("patched document missing: %+v", got)
	}
	block := got.Blocks[1]
	if block.Kind != types.BlockOrderedList || block.Title != "keep title" || block.Text != "keep text" ||
		len(block.Items) != 1 || block.Items[0].Label != "keep label" || block.Items[0].Text != "keep item" ||
		len(block.EdgeAnchors) != 1 {
		t.Fatalf("metadata attach changed reader-visible ordered-list content: %+v", block)
	}
	gotAnchor := block.EdgeAnchors[0]
	if gotAnchor.FromNode != anchor.FromNode || gotAnchor.ToNode != anchor.ToNode ||
		gotAnchor.VisibleLabel != anchor.VisibleLabel || gotAnchor.RelationKind != types.DiagramRelCall ||
		gotAnchor.FromIdentity != "pkg.Caller.run" || gotAnchor.ToIdentity != "pkg.Callee.accept" {
		t.Fatalf("metadata attach did not preserve local carrier and install only selected typed identities: %+v", gotAnchor)
	}
}

func TestEmitAnswerDocumentPatch_NonDiagramAdditionRefAddsOnlySelectedHiddenAnchor(t *testing.T) {
	prev := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "keep summary"},
		{ID: "chain", Kind: types.BlockOrderedList, Title: "keep title", Text: "keep text", SurfaceRole: types.SurfacePrincipal,
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge, EvidenceID: "ev-call"}},
			Items:     []types.AnswerBlockItem{{ID: "step", Label: "caller to callee", Text: "keep item", EvidenceIDs: []string{"ev-call"}}}},
	}}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, nil, []types.AnswerDiagramRelationRepairCandidate{{
		BlockID: "chain", RelationKind: types.DiagramRelCall,
		FromIdentity: "pkg.Caller.run", ToIdentity: "pkg.Callee.accept", EvidenceID: "ev-call", Source: "src/call.go:10",
	}})
	if lease == nil || len(lease.AllowedAdditions) != 1 ||
		!answerDocumentStandaloneRelationAdditionCandidateSelected(prev, lease.AllowedAdditions[0]) {
		t.Fatalf("test setup missing exact model-selected standalone addition: %+v", lease)
	}
	mut := types.NewMutableState("standalone relation anchor addition")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	ctx := &types.BusContext{
		Mutable: mut,
		EvidenceItems: []types.EvidenceItem{{
			ID: "ev-call", Kind: types.EvidenceRelationship, Subject: "pkg.Caller.run", Object: "pkg.Callee.accept",
			Predicate: "calls", Source: "src/call.go", LineStart: 10, LineEnd: 10,
			AnchorKind: types.AnchorCall, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
		}},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, PredicateAxis: types.AxisCall,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)}}},
	}
	params := json.RawMessage(fmt.Sprintf(`{
		"unchanged_block_ids":["summary"],
		"diagram_edge_edits":[{"addition_ref":%q,"action":"add","edge":{"from_node":"caller","to_node":"callee","visible_label":"调用"}}]
	}`, lease.AllowedAdditions[0].AdditionRef))
	res, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("exact standalone hidden-anchor addition must pass: err=%v res=%+v", err, res)
	}
	got := mut.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 2 {
		t.Fatalf("patched document missing: %+v", got)
	}
	block := got.Blocks[1]
	if block.Title != "keep title" || block.Text != "keep text" || len(block.Items) != 1 ||
		block.Items[0].Label != "caller to callee" || block.Items[0].Text != "keep item" || len(block.EdgeAnchors) != 1 {
		t.Fatalf("hidden anchor addition changed model-visible relation carrier: %+v", block)
	}
	anchor := block.EdgeAnchors[0]
	if anchor.FromIdentity != "pkg.Caller.run" || anchor.ToIdentity != "pkg.Callee.accept" ||
		anchor.RelationKind != types.DiagramRelCall || anchor.VisibleLabel != "调用" {
		t.Fatalf("typed hidden anchor was not bound to the model-selected evidence: %+v", anchor)
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

func TestApplyModelAuthoredDiagramAtomicEdits_BoundaryRefsPreserveUnmentionedRowsAndGraph(t *testing.T) {
	prev := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart TD\n A-->|work|B"},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence, VisibleLabel: "work",
		}},
		ParticipantBoundaries: []types.DiagramParticipantBoundary{
			{Participant: "Analyzer", Status: types.DiagramParticipantBoundaryUnproven},
			{Participant: "Keep", Status: types.DiagramParticipantBoundaryUnproven},
		},
	}}}
	lease := types.WithAnswerDiagramParticipantBoundaryRepairFailures(prev, nil,
		[]types.AnswerDiagramParticipantBoundaryRepairFailure{
			{BlockID: "flow", Participant: "Analyzer", Issue: "stale_boundary_for_connected_participant"},
			{BlockID: "flow", Participant: "Mutable", Issue: "missing_unproven_boundary"},
		})
	if lease == nil || len(lease.ParticipantBoundaryFailures) != 2 {
		t.Fatalf("test setup: expected two boundary refs: %+v", lease)
	}
	byParticipant := make(map[string]types.AnswerDiagramParticipantBoundaryRepairFailure)
	for _, failure := range lease.ParticipantBoundaryFailures {
		byParticipant[failure.Participant] = failure
	}
	patch := &types.AnswerDocumentV2Patch{}
	err := applyModelAuthoredDiagramAtomicEditsWithParticipantsAndBoundaries(
		prev, patch, nil, nil,
		[]emitAnswerDiagramBoundaryEdit{
			{BoundaryRef: byParticipant["Analyzer"].BoundaryRef, Action: "remove_boundary"},
			{BoundaryRef: byParticipant["Mutable"].BoundaryRef, Action: "add_unproven"},
		}, nil, nil, lease,
	)
	if err != nil {
		t.Fatalf("exact boundary refs should execute: %v", err)
	}
	if len(patch.ReplaceBlocks) != 1 {
		t.Fatalf("expected one compiled local block replacement: %+v", patch.ReplaceBlocks)
	}
	got := patch.ReplaceBlocks[0]
	if got.Diagram == nil || got.Diagram.Body != prev.Blocks[0].Diagram.Body ||
		len(got.EdgeAnchors) != 1 || got.EdgeAnchors[0] != prev.Blocks[0].EdgeAnchors[0] {
		t.Fatalf("boundary refs must preserve visible graph and typed anchors: %+v", got)
	}
	if len(got.ParticipantBoundaries) != 2 || got.ParticipantBoundaries[0].Participant != "Keep" ||
		got.ParticipantBoundaries[1].Participant != "Mutable" {
		t.Fatalf("only selected boundary rows may change: %+v", got.ParticipantBoundaries)
	}

	stale := *prev
	stale.Blocks = append([]types.AnswerBlock(nil), prev.Blocks...)
	stale.Blocks[0].ParticipantBoundaries = append(
		append([]types.DiagramParticipantBoundary(nil), prev.Blocks[0].ParticipantBoundaries...),
		types.DiagramParticipantBoundary{Participant: "Other", Status: types.DiagramParticipantBoundaryUnproven},
	)
	err = applyModelAuthoredDiagramAtomicEditsWithParticipantsAndBoundaries(
		&stale, &types.AnswerDocumentV2Patch{}, nil, nil,
		[]emitAnswerDiagramBoundaryEdit{{BoundaryRef: byParticipant["Analyzer"].BoundaryRef, Action: "remove_boundary"}},
		nil, nil, lease,
	)
	if err == nil || !strings.Contains(err.Error(), "current boundary generation") {
		t.Fatalf("a ref rebound to another boundary generation must fail closed: %v", err)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_VisibilityRefAddsDeclarationWithoutRelation(t *testing.T) {
	prev := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart TD\n A-->|work|B"},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence, VisibleLabel: "work",
		}},
		ParticipantBoundaries: []types.DiagramParticipantBoundary{{
			Participant: "BusContext", Status: types.DiagramParticipantBoundaryUnproven,
		}},
	}}}
	lease := types.WithAnswerDiagramParticipantVisibilityRepairFailures(prev, nil,
		[]types.AnswerDiagramParticipantVisibilityRepairFailure{{
			BlockID: "flow", Participant: "BusContext", Issue: "boundary_participant_not_visible",
		}})
	if lease == nil || len(lease.ParticipantVisibilityFailures) != 1 {
		t.Fatalf("test setup: expected one visibility ref: %+v", lease)
	}
	edit := emitAnswerDiagramParticipantEdit{
		ParticipantRef: lease.ParticipantVisibilityFailures[0].ParticipantRef,
		Action:         "ensure_visible",
		NodeID:         "BusContextNode",
		VisibleLabel:   "BusContext",
	}
	patch := &types.AnswerDocumentV2Patch{}
	if err := applyModelAuthoredDiagramAtomicEditsWithParticipantsAndBoundaries(
		prev, patch, nil, nil, nil, []emitAnswerDiagramParticipantEdit{edit}, nil, lease,
	); err != nil {
		t.Fatalf("exact visibility ref should execute: %v", err)
	}
	if len(patch.ReplaceBlocks) != 1 {
		t.Fatalf("expected one local replacement: %+v", patch.ReplaceBlocks)
	}
	got := patch.ReplaceBlocks[0]
	if got.Diagram == nil || !strings.Contains(got.Diagram.Body, `BusContextNode["BusContext"]`) ||
		len(mermaidcompat.ParseEdges(got.Diagram.Body)) != 1 || len(got.EdgeAnchors) != 1 ||
		got.EdgeAnchors[0] != prev.Blocks[0].EdgeAnchors[0] || len(got.ParticipantBoundaries) != 1 {
		t.Fatalf("visibility edit changed a relation/anchor/boundary or missed declaration: %+v", got)
	}

	stale := *prev
	stale.Blocks = append([]types.AnswerBlock(nil), prev.Blocks...)
	diagram := *prev.Blocks[0].Diagram
	diagram.Body += "\n C"
	stale.Blocks[0].Diagram = &diagram
	err := applyModelAuthoredDiagramAtomicEditsWithParticipantsAndBoundaries(
		&stale, &types.AnswerDocumentV2Patch{}, nil, nil, nil,
		[]emitAnswerDiagramParticipantEdit{edit}, nil, lease,
	)
	if err == nil || !strings.Contains(err.Error(), "current diagram generation") {
		t.Fatalf("stale visibility ref must fail closed: %v", err)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_VisibilityRefAcceptsExactMultiwordDisplayIdentity(t *testing.T) {
	base := func() *types.AnswerDocumentV2 {
		return &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
			ID: "sequence", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramSequence, Language: "mermaid",
				Body: "sequenceDiagram\n participant Analyze\n Analyze->>Analyze: inspect",
			},
			ParticipantBoundaries: []types.DiagramParticipantBoundary{{
				Participant: "read mode", Status: types.DiagramParticipantBoundaryUnproven,
			}},
		}}}
	}
	makeLease := func(prev *types.AnswerDocumentV2) *types.AnswerDiagramRelationRepairLease {
		return types.WithAnswerDiagramParticipantVisibilityRepairFailures(prev, nil,
			[]types.AnswerDiagramParticipantVisibilityRepairFailure{{
				BlockID: "sequence", Participant: "read mode", Issue: "boundary_participant_not_visible",
			}})
	}

	prev := base()
	lease := makeLease(prev)
	if lease == nil || len(lease.ParticipantVisibilityFailures) != 1 {
		t.Fatalf("test setup: expected one visibility ref: %+v", lease)
	}
	patch := &types.AnswerDocumentV2Patch{}
	err := applyModelAuthoredDiagramAtomicEditsWithParticipantsAndBoundaries(
		prev, patch, nil, nil, nil,
		[]emitAnswerDiagramParticipantEdit{{
			ParticipantRef: lease.ParticipantVisibilityFailures[0].ParticipantRef,
			Action:         "ensure_visible", NodeID: "readmode", VisibleLabel: "read mode",
		}}, nil, lease,
	)
	if err != nil {
		t.Fatalf("a safe node id with the exact multiword display identity should execute: %v", err)
	}
	if len(patch.ReplaceBlocks) != 1 || patch.ReplaceBlocks[0].Diagram == nil ||
		!strings.Contains(patch.ReplaceBlocks[0].Diagram.Body, `participant readmode as "read mode"`) ||
		len(mermaidcompat.ParseEdges(patch.ReplaceBlocks[0].Diagram.Body)) != 1 {
		t.Fatalf("visibility edit missed the declaration or changed relations: %+v", patch.ReplaceBlocks)
	}

	prev = base()
	lease = makeLease(prev)
	err = applyModelAuthoredDiagramAtomicEditsWithParticipantsAndBoundaries(
		prev, &types.AnswerDocumentV2Patch{}, nil, nil, nil,
		[]emitAnswerDiagramParticipantEdit{{
			ParticipantRef: lease.ParticipantVisibilityFailures[0].ParticipantRef,
			Action:         "ensure_visible", NodeID: "readmode", VisibleLabel: "read",
		}}, nil, lease,
	)
	if err == nil || !strings.Contains(err.Error(), "exact required participant identity") {
		t.Fatalf("a substring display label must remain rejected: %v", err)
	}
}

func TestApplyModelAuthoredDiagramAtomicEdits_AttachAndBoundaryRefShareOneTransaction(t *testing.T) {
	prev := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart TD\n A-->|next|B"},
		ParticipantBoundaries: []types.DiagramParticipantBoundary{{
			Participant: "Analyzer", Status: types.DiagramParticipantBoundaryUnproven,
		}},
	}}}
	relationLease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "flow", Issue: "missing_relation_anchor", FromNode: "A", ToNode: "B",
			FromIdentity: "Analyzer", ToIdentity: "Explorer", RelationKind: types.DiagramRelPrecedence,
			BodyOccurrence: 1,
		}}, []types.AnswerDiagramRelationRepairCandidate{{
			BlockID: "flow", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence, Source: "internal/orchestrator/topology.go:1",
		}})
	lease := types.WithAnswerDiagramParticipantBoundaryRepairFailures(prev, relationLease,
		[]types.AnswerDiagramParticipantBoundaryRepairFailure{{
			BlockID: "flow", Participant: "Analyzer", Issue: "stale_boundary_for_connected_participant",
		}})
	if lease == nil || len(lease.Failures) != 1 || len(lease.AllowedAdditions) != 1 ||
		len(lease.ParticipantBoundaryFailures) != 1 ||
		!lease.Failures[0].AllowsAction("attach") {
		t.Fatalf("test setup: expected attach plus boundary capabilities: %+v", lease)
	}
	patch := &types.AnswerDocumentV2Patch{}
	err := applyModelAuthoredDiagramAtomicEditsWithParticipantsAndBoundaries(
		prev, patch,
		[]emitAnswerDiagramEdgeEdit{{
			FailureRef: lease.Failures[0].FailureRef, AdditionRef: lease.AllowedAdditions[0].AdditionRef,
			Action: "attach", Edge: &types.DiagramEdgeAnchor{FromNode: "A", ToNode: "B", VisibleLabel: "next"},
		}}, nil,
		[]emitAnswerDiagramBoundaryEdit{{
			BoundaryRef: lease.ParticipantBoundaryFailures[0].BoundaryRef, Action: "remove_boundary",
		}}, nil, nil, lease,
	)
	if err != nil {
		t.Fatalf("joint attach+boundary transaction should execute: %v", err)
	}
	visibleEdges := mermaidcompat.ParseEdges(patch.ReplaceBlocks[0].Diagram.Body)
	if len(patch.ReplaceBlocks) != 1 || len(patch.ReplaceBlocks[0].EdgeAnchors) != 1 ||
		len(patch.ReplaceBlocks[0].ParticipantBoundaries) != 0 || len(visibleEdges) != 1 ||
		visibleEdges[0].From != "A" || visibleEdges[0].To != "B" {
		t.Fatalf("joint transaction must attach in place and remove only selected boundary: %+v", patch.ReplaceBlocks)
	}
}
