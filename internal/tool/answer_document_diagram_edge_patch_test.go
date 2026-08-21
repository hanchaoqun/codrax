package tool

import (
	"encoding/json"
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

	t.Run("requested boundary and still-connected declarations fail closed", func(t *testing.T) {
		for name, tc := range map[string]struct {
			participant string
			protected   []string
			addBoundary bool
		}{
			"requested":       {participant: "A", protected: []string{"A"}},
			"boundary":        {participant: "A", addBoundary: true},
			"still connected": {participant: "B"},
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
				if err == nil {
					t.Fatal("unsafe participant cleanup unexpectedly passed")
				}
			})
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

func TestApplyModelAuthoredDiagramAtomicEdits_AdditionRefStampsOnlySelectedHiddenTuple(t *testing.T) {
	prev := atomicPatchTestDocument()
	lease := types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "requested_stage_precedence_spine_incomplete",
			FromNode: "StageAnalyze", ToNode: "StageExplore", RelationKind: types.DiagramRelPrecedence,
		}}, []types.AnswerDiagramRelationRepairCandidate{{
			BlockID: "diag", RelationKind: types.DiagramRelPrecedence,
			FromIdentity: "analyzer", ToIdentity: "explorer", Source: "internal/types/enums.go:120-121",
		}})
	if lease == nil || len(lease.AllowedAdditions) != 1 || lease.AllowedAdditions[0].AdditionRef == "" {
		t.Fatalf("expected one live referenced addition: %+v", lease)
	}
	additionRef := lease.AllowedAdditions[0].AdditionRef
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
		Action: "add", AdditionRef: additionRef,
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
		added.FromIdentity != "analyzer" || added.ToIdentity != "explorer" ||
		added.RelationKind != types.DiagramRelPrecedence {
		t.Fatalf("ref must preserve visible authorship and stamp only its typed tuple: %+v", added)
	}

	merged, err := types.ApplyAnswerDocumentV2Patch(prev, patch)
	if err != nil {
		t.Fatalf("compiled patch must apply: %v", err)
	}
	// This is the production split from r806: a node-keyed recipe offers a
	// different Stage identity dialect. The already-complete ref-selected Agent
	// pair must not be re-authored by that downstream metadata normalizer.
	fixed := normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(merged, []types.DiagramEdgeAnchor{{
		FromNode: "businessAnalyze", ToNode: "businessExplore",
		FromIdentity: "StageAnalyze", ToIdentity: "StageExplore",
		RelationKind: types.DiagramRelPrecedence,
	}})
	if fixed != 0 || merged.Blocks[1].EdgeAnchors[2].FromIdentity != "analyzer" ||
		merged.Blocks[1].EdgeAnchors[2].ToIdentity != "explorer" {
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
			Edge: &types.DiagramEdgeAnchor{
				FromNode: "X", ToNode: "Y", FromIdentity: "other", ToIdentity: "explorer",
				RelationKind: types.DiagramRelPrecedence, VisibleLabel: "model label",
			},
		}}, nil, lease)
		if err != nil || len(got.ReplaceBlocks) != 1 || len(got.ReplaceBlocks[0].EdgeAnchors) != 3 {
			t.Fatalf("a live addition ref must quarantine hidden mirrors: err=%v patch=%+v", err, got)
		}
		added := got.ReplaceBlocks[0].EdgeAnchors[2]
		if added.FromNode != "X" || added.ToNode != "Y" || added.VisibleLabel != "model label" ||
			added.FromIdentity != "analyzer" || added.ToIdentity != "explorer" ||
			added.RelationKind != types.DiagramRelPrecedence {
			t.Fatalf("ref must preserve visible authorship and restore only selected hidden fields: %+v", added)
		}
	})
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
			if got := localDiagramLeaseWholeBlockMutationViolation(params, lease); got == nil || got.BlockID != "diag" {
				t.Fatalf("whole target mutation must be rejected from typed lease: %+v", got)
			}
		})
	}
	if got := localDiagramLeaseWholeBlockMutationViolation(&emitAnswerDocumentPatchParams{
		ReplaceBlocks: []emitAnswerBlockV2{{ID: "summary"}},
	}, lease); got != nil {
		t.Fatalf("unrelated sibling replacement must remain available: %+v", got)
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
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence, VisibleLabel: "确定范围后收集证据",
		},
	}}, nil, lease)
	if err != nil {
		t.Fatalf("ref-selected stale anchor replacement must stay executable: %v", err)
	}
	got := patch.ReplaceBlocks[0]
	if !strings.Contains(got.Diagram.Body, "A->>B: 确定范围后收集证据") ||
		!strings.Contains(got.Diagram.Body, "B->>C: keep label") ||
		len(got.EdgeAnchors) != 2 || got.EdgeAnchors[0].VisibleLabel != "确定范围后收集证据" {
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
		!strings.Contains(err.Error(), "allowed_actions=[remove replace]") {
		t.Fatalf("relation-evidence failure must expose its exact executable actions: %v", err)
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
	if err == nil || !strings.Contains(err.Error(), "carrier=unknown") || !strings.Contains(err.Error(), "allowed_actions=[]") {
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

	t.Run("live addition ref removes recipe dialect dependency", func(t *testing.T) {
		bus := newBus(allowed, nil)
		lease := bus.Mutable.AnswerDiagramRelationRepairLease()
		if lease == nil || len(lease.AllowedAdditions) != 1 || lease.AllowedAdditions[0].AdditionRef == "" {
			t.Fatalf("expected a referenced live addition: %+v", lease)
		}
		params := fmt.Sprintf(`{
			"unchanged_block_ids":["summary"],
			"diagram_edge_edits":[{"action":"add","addition_ref":%q,"edge":{
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

func TestApplyModelAuthoredDiagramAtomicEdits_SharedBodyRejectsMixedActions(t *testing.T) {
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Body = "sequenceDiagram\n    A->>B: shared\n"
	prev.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{
		{FromNode: "A", ToNode: "B", FromIdentity: "A.call", ToIdentity: "B.run", RelationKind: types.DiagramRelCall},
		{FromNode: "A", ToNode: "B", FromIdentity: "analyzer", ToIdentity: "explorer", RelationKind: types.DiagramRelPrecedence},
	}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{
		{BlockID: "diag", Issue: "call_edge_unproven", FromNode: "A", ToNode: "B", FromIdentity: "A.call", ToIdentity: "B.run", RelationKind: types.DiagramRelCall, BodyOccurrence: 1},
		{BlockID: "diag", Issue: "semantic_relation_edge_unproven", FromNode: "A", ToNode: "B", FromIdentity: "analyzer", ToIdentity: "explorer", RelationKind: types.DiagramRelPrecedence, BodyOccurrence: 1},
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
