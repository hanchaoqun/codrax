package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// answer_document_patch_base_transaction_test.go — V2-1 / V2-2 pins
// (colleague_merge_audit §40.17 / §40.18 → §40.45).
//
// The answer-document patch transaction has one normalized base: the
// deterministic raw↔typed patch normalizers run once, positionally aligned to
// the model's JSON, before any system-appended atomic diagram block, and every
// stage / commit base is built by buildAnswerDocumentPatchBase. Retry-local
// generation state (pending base + relation lease) is written only through
// stageAnswerDocumentPatchGeneration and cleared only in the locked success
// epilogue (commitAcceptedAnswerDocumentLocked) or by explicit rollback.

// patchBaseTransactionFixture is atomicPatchTestDocument() plus two ordinary
// list blocks: `steps` (title/items/facets/role plus one typed relation
// anchor, the target of a sparse edge_anchors-only replacement) and `notes`
// (fully replaced with a model-submitted citation_ref). The lease carries one
// removable failed edge on `diag` and one optional orphan candidate (A).
func patchBaseTransactionFixture() (*types.AnswerDocumentV2, *types.AnswerDiagramRelationRepairLease) {
	prev := atomicPatchTestDocument()
	prev.Citations = []types.Citation{{File: "x.go", Line: 10}}
	prev.Blocks = append(prev.Blocks,
		types.AnswerBlock{
			ID: "steps", Kind: types.BlockOrderedList, Title: "model steps title",
			SurfaceRole: types.SurfacePrincipal, FacetIDs: []string{"call_chain"},
			Items: []types.AnswerBlockItem{{ID: "hop", Label: "entry", CitationRef: 0}},
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "entry", ToNode: "worker", FromIdentity: "pkg.entry", ToIdentity: "pkg.worker",
				RelationKind: types.DiagramRelCall, VisibleLabel: "call worker",
			}},
		},
		types.AnswerBlock{
			ID: "notes", Kind: types.BlockBulletList, Title: "notes",
			Items: []types.AnswerBlockItem{{ID: "n1", Label: "old note"}},
		},
	)
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "diag", Issue: "semantic_relation_edge_unproven",
		FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
		RelationKind: types.DiagramRelPrecedence, BodyOccurrence: 1,
	}}, nil)
	lease.OptionalOrphanCleanups = testDiagramOrphanCandidates("diag", "A")
	return prev, lease
}

func patchBaseTransactionMutable(t *testing.T) (*types.MutableState, *types.AnswerDiagramRelationRepairLease) {
	t.Helper()
	prev, lease := patchBaseTransactionFixture()
	if lease == nil {
		t.Fatal("fixture lease must be executable")
	}
	mut := types.NewMutableState("patch base transaction")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	return mut, lease
}

// patchBaseTransactionRelationPhase is the model's phase-one JSON: one exact
// failed-edge removal, one sparse relation-metadata-only replacement of
// `steps` (identical anchors; content omitted), one full replacement of `notes`
// with a model-submitted citation_ref.
func patchBaseTransactionRelationPhase(lease *types.AnswerDiagramRelationRepairLease, extra string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{
		"unchanged_block_ids":["summary"],
		"diagram_edge_edits":[{"failure_ref":%q,"action":"remove"}],
		"replace_blocks":[
			{"id":"steps","edge_anchors":[{"from_node":"entry","to_node":"worker","from_identity":"pkg.entry","to_identity":"pkg.worker","relation_kind":"call","visible_label":"call worker"}]},
			{"id":"notes","kind":"bullet_list","title":"notes","items":[{"id":"n1","label":"new note","citation_ref":0}]}
		]%s
	}`, lease.Failures[0].FailureRef, extra))
}

const patchBaseTransactionDispositionJSON = `"diagram_participant_edits":[{"block_id":"diag","participant_id":"A","action":"retain_as_context","visible_label":"分析入口（背景）"}]`

func blockByID(t *testing.T, doc *types.AnswerDocumentV2, id string) types.AnswerBlock {
	t.Helper()
	if doc == nil {
		t.Fatalf("document is nil while looking for block %q", id)
	}
	for _, block := range doc.Blocks {
		if block.ID == id {
			return block
		}
	}
	t.Fatalf("block %q not found in %+v", id, doc)
	return types.AnswerBlock{}
}

// TestEmitAnswerDocumentPatch_OrphanStagingBaseIsPatchNormalized (§40.17
// acceptance pin): the base staged by the orphan-roster branch is the
// patch-normalized document — the sparse relation-metadata-only replacement
// keeps the model's prior title/items/facets/role, and a model-submitted
// citation_ref carries its submission provenance into phase two.
func TestEmitAnswerDocumentPatch_OrphanStagingBaseIsPatchNormalized(t *testing.T) {
	mut, lease := patchBaseTransactionMutable(t)
	res, err := (&EmitAnswerDocumentPatch{}).Execute(&types.BusContext{Mutable: mut}, patchBaseTransactionRelationPhase(lease, ""))
	if err != nil || res.Success || res.Repair == nil {
		t.Fatalf("relation-only phase must stage and request dispositions: err=%v res=%+v", err, res)
	}
	if got := res.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome]; got != types.AnswerDocumentPatchOutcomeStagedForRetry {
		t.Fatalf("outcome=%q, want staged_for_retry", got)
	}
	staged := mut.PendingAnswerDocumentPatchBase()
	if staged == nil {
		t.Fatal("phase one must stage the merged base")
	}
	steps := blockByID(t, staged, "steps")
	if steps.Title != "model steps title" || len(steps.Items) != 1 || steps.Items[0].Label != "entry" ||
		steps.SurfaceRole != types.SurfacePrincipal || !reflect.DeepEqual(steps.FacetIDs, []string{"call_chain"}) {
		t.Fatalf("staged base lost the sparse replacement's preserved model content: %+v", steps)
	}
	if len(steps.EdgeAnchors) != 1 || steps.EdgeAnchors[0].VisibleLabel != "call worker" {
		t.Fatalf("staged base lost the sparse replacement's typed anchors: %+v", steps.EdgeAnchors)
	}
	notes := blockByID(t, staged, "notes")
	if len(notes.Items) != 1 || notes.Items[0].Label != "new note" ||
		!notes.Items[0].CitationRefsModelSubmitted || !reflect.DeepEqual(notes.Items[0].CitationRefsModelSubmittedValues, []int{0}) {
		t.Fatalf("staged base lost model citation submission provenance: %+v", notes.Items)
	}
	diag := blockByID(t, staged, "diag")
	if strings.Contains(diag.Diagram.Body, "A->>B: old label") || len(diag.EdgeAnchors) != 1 {
		t.Fatalf("staged base must carry the executed relation edit: %+v", diag)
	}
	if orphanLease := mut.AnswerDiagramRelationRepairLease(); orphanLease == nil || !orphanLease.OrphanDispositionOnly {
		t.Fatalf("phase one must publish the orphan-only generation lease: %+v", orphanLease)
	}
}

// TestEmitAnswerDocumentPatch_TwoPhaseCommitEqualsSingleCallCommit (§40.17 ③
// tripwire): the same model operations submitted as one atomic call (edge
// removal + disposition) or as two generations (relation phase, then
// disposition-only phase) produce byte-identical accepted documents and
// deep-equal blocks including internal provenance flags.
func TestEmitAnswerDocumentPatch_TwoPhaseCommitEqualsSingleCallCommit(t *testing.T) {
	single, singleLease := patchBaseTransactionMutable(t)
	res, err := (&EmitAnswerDocumentPatch{}).Execute(&types.BusContext{Mutable: single},
		patchBaseTransactionRelationPhase(singleLease, ",\n"+patchBaseTransactionDispositionJSON))
	if err != nil || !res.Success {
		t.Fatalf("single-call commit failed: err=%v res=%+v", err, res)
	}

	staged, stagedLease := patchBaseTransactionMutable(t)
	res, err = (&EmitAnswerDocumentPatch{}).Execute(&types.BusContext{Mutable: staged}, patchBaseTransactionRelationPhase(stagedLease, ""))
	if err != nil || res.Success {
		t.Fatalf("phase one must stage: err=%v res=%+v", err, res)
	}
	res, err = (&EmitAnswerDocumentPatch{}).Execute(&types.BusContext{Mutable: staged}, json.RawMessage("{"+patchBaseTransactionDispositionJSON+"}"))
	if err != nil || !res.Success {
		t.Fatalf("phase two commit failed: err=%v res=%+v", err, res)
	}

	singleDoc, stagedDoc := single.AnswerDocumentV2(), staged.AnswerDocumentV2()
	singleJSON, _ := json.Marshal(singleDoc)
	stagedJSON, _ := json.Marshal(stagedDoc)
	if string(singleJSON) != string(stagedJSON) {
		t.Fatalf("two-phase commit diverged from single-call commit:\nsingle=%s\nstaged=%s", singleJSON, stagedJSON)
	}
	for _, id := range []string{"steps", "notes", "diag"} {
		if a, b := blockByID(t, singleDoc, id), blockByID(t, stagedDoc, id); !reflect.DeepEqual(a, b) {
			t.Fatalf("block %q internal state diverged between paths:\nsingle=%+v\nstaged=%+v", id, a, b)
		}
	}
	for _, mut := range []*types.MutableState{single, staged} {
		if mut.PendingAnswerDocumentPatchBase() != nil || mut.AnswerDiagramRelationRepairLease() != nil {
			t.Fatal("success epilogue must clear retry-local generation state")
		}
	}
}

// TestEmitAnswerDocumentPatch_PersistLaneRejectionRetainsRelationLease
// (§40.18 acceptance pin): a persist-lane rejection (model relation_claims
// naming no typed authority) rolls the whole transaction back — accepted
// document unchanged, nothing staged — and the relation lease survives so the
// model can resubmit the same exact local repair. The following successful
// generation consumes the lease in the locked success epilogue.
func TestEmitAnswerDocumentPatch_PersistLaneRejectionRetainsRelationLease(t *testing.T) {
	mut := types.NewMutableState("persist lane rejection")
	base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "keep"},
		{
			ID: "flow", Kind: types.BlockDiagram,
			Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n A->>B: call"},
			EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall}},
		},
	}}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
	lease := types.NewAnswerDiagramRelationRepairLease(base, []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "flow", Issue: "call_edge_unproven", FromNode: "A", ToNode: "B",
	}}, nil)
	if lease == nil {
		t.Fatal("fixture lease must be executable")
	}
	mut.SetAnswerDiagramRelationRepairLease(lease)
	ref := lease.Failures[0].FailureRef

	res, err := (&EmitAnswerDocumentPatch{}).Execute(&types.BusContext{Mutable: mut}, json.RawMessage(fmt.Sprintf(`{
		"diagram_edge_edits":[{"failure_ref":%q,"action":"remove"}],
		"replace_blocks":[{"id":"summary","kind":"summary","text":"keep",
			"relation_claims":[{"authority_id":"no-such-authority","member_refs":["m"],"physical_relation":"overlap","addition":"forbidden"}]}]
	}`, ref)))
	if err != nil || res.Success {
		t.Fatalf("persist-lane relation_claims rejection expected: err=%v res=%+v", err, res)
	}
	if !strings.Contains(res.Summary, "do not match typed trace authority") {
		t.Fatalf("rejection must come from the persist lane: %s", res.Summary)
	}
	if res.Repair == nil || res.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome] != types.AnswerDocumentPatchOutcomeNotStaged {
		t.Fatalf("persist-lane rejection must report not_staged: %+v", res.Repair)
	}
	if got := mut.AnswerDiagramRelationRepairLease(); got == nil || len(got.Failures) != 1 || got.Failures[0].FailureRef != ref {
		t.Fatalf("persist-lane rejection must retain the relation lease for the resubmission: %+v", got)
	}
	if mut.PendingAnswerDocumentPatchBase() != nil {
		t.Fatal("persist-lane rejection must not stage a base")
	}
	if got := mut.AnswerDocumentV2(); !reflect.DeepEqual(got, base) {
		t.Fatalf("persist-lane rejection must leave the accepted document unchanged: %+v", got)
	}

	res, err = (&EmitAnswerDocumentPatch{}).Execute(&types.BusContext{Mutable: mut}, json.RawMessage(fmt.Sprintf(`{
		"unchanged_block_ids":["summary"],
		"diagram_edge_edits":[{"failure_ref":%q,"action":"remove"}]
	}`, ref)))
	if err != nil || !res.Success {
		t.Fatalf("resubmitted exact repair must commit: err=%v res=%+v", err, res)
	}
	if mut.AnswerDiagramRelationRepairLease() != nil || mut.PendingAnswerDocumentPatchBase() != nil {
		t.Fatal("success epilogue must consume the lease and staged base")
	}
	if got := mut.AnswerDocumentV2(); got == nil || len(blockByID(t, got, "flow").EdgeAnchors) != 0 {
		t.Fatalf("committed generation must carry the removed anchor: %+v", got)
	}
}
