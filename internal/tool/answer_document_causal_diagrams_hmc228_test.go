package tool

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc228CausalDiagramContext(t *testing.T, profile *types.RuntimeQuestionProfile) (*types.BusContext, []RuntimeDiagramRelation) {
	t.Helper()
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	start, end := 1.0, 1.065
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "en", Mutable: types.NewMutableState("Explain the document response"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "en", Intent: types.IntentTrace, PerfTrace: &types.PerfBundle{},
			RuntimeQuestionProfile: profile, RuntimeTargets: []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Source: "user_explicit"}},
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1 to 1.065 seconds"}}}}
	result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "wakeup_chain", "pid": 100, "time_start": start, "time_end": end, "min_duration_ms": 0.1})
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	rows := runtimeDiagramRelationsForContext(ctx)
	if len(rows) != 2 || types.BuildAnswerSemanticViewForBusContext(ctx).Family != types.QFRootCauseTrace {
		t.Fatalf("real causal context must contain two wakeups and the legacy trace family: %+v", rows)
	}
	return ctx, rows
}

func hmc228WakeDiagram(rows []RuntimeDiagramRelation) *types.AnswerDocumentV2 {
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, TraceCausalClaimCaliber: types.TraceCausalClaimNoConclusion, Text: "The measured wakeups link storage completion, document work, and the response thread."},
		{ID: "events", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{{Label: "Measured events", Text: "Wakeups are separate from scheduling and request residence.", CitationRef: -1}}},
		{ID: "sequence", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n"}},
	}}
	for _, row := range rows {
		doc.Blocks[2].Diagram.Body += fmt.Sprintf(" participant %s as %s\n participant %s as %s\n %s->>%s: observed wakeup\n", row.FromNode, row.FromLabel, row.ToNode, row.ToLabel, row.FromNode, row.ToNode)
		doc.Blocks[2].EdgeAnchors = append(doc.Blocks[2].EdgeAnchors, types.DiagramEdgeAnchor{FromNode: row.FromNode, ToNode: row.ToNode, FromIdentity: row.FromIdentity, ToIdentity: row.ToIdentity, RelationKind: row.Kind, VisibleLabel: "observed wakeup"})
	}
	return doc
}

func TestHMC228CausalDiagramPublicEmitRejectsUnprovedArrows(t *testing.T) {
	for _, name := range []string{"causal", "frame", "unspecified", "legacy"} {
		t.Run(name, func(t *testing.T) {
			profile := &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis}
			switch name {
			case "frame":
				profile.FrameCausalityRequested = true
			case "unspecified":
				profile.Scope = types.RuntimeQuestionScopeUnspecified
			case "legacy":
				profile = nil
			}
			ctx, rows := hmc228CausalDiagramContext(t, profile)
			doc := hmc228WakeDiagram(rows)
			doc.Blocks[2].Diagram.Body = "sequenceDiagram\n participant M as app-main\n participant W as document-worker\n participant B as backup\n participant S as storage-irq\n M->>W: dispatch\n W->>B: background work\n W->>S: storage request\n"
			doc.Blocks[2].EdgeAnchors = nil
			raw, _ := json.Marshal(doc)
			res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
			if err != nil || res.Success || res.Repair == nil {
				t.Fatalf("normal emit accepted unproved runtime arrows: err=%v result=%+v", err, res)
			}
			if ctx.Mutable.AnswerDocumentV2() != nil || ctx.Mutable.LastRejectedAnswerDocumentV2() == nil {
				t.Fatal("failed graph was accepted or its raw draft lost")
			}
			if res.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON] == "" {
				t.Fatalf("rejection must be the existing exact relation gate, not an unrelated requirement: %s", res.Summary)
			}
		})
	}
}

func TestHMC228CausalDiagramMixedAndPrivateAuthority(t *testing.T) {
	for _, mode := range []string{"mixed", "wrong_kind", "reverse", "private_sibling", "serialized_private"} {
		t.Run(mode, func(t *testing.T) {
			ctx, rows := hmc228CausalDiagramContext(t, &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis})
			doc := hmc228WakeDiagram(rows)
			switch mode {
			case "mixed":
				doc.Blocks[2].Diagram.Body += " participant B as backup\n " + rows[0].ToNode + "->>B: supposed dependency\n"
			case "wrong_kind":
				doc.Blocks[2].EdgeAnchors[0].RelationKind = types.DiagramRelCall
			case "reverse":
				a := &doc.Blocks[2].EdgeAnchors[0]
				doc.Blocks[2].Diagram.Body = strings.Replace(doc.Blocks[2].Diagram.Body, a.FromNode+"->>"+a.ToNode, a.ToNode+"->>"+a.FromNode, 1)
				a.FromNode, a.ToNode = a.ToNode, a.FromNode
				a.FromIdentity, a.ToIdentity = a.ToIdentity, a.FromIdentity
			case "private_sibling", "serialized_private":
				doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: "private_projection", Kind: types.BlockDiagram,
					SystemGeneratedKind: types.AnswerSystemGeneratedRuntimeTrace,
					Diagram:             &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart TD\n isolated_native_from --> isolated_native_to"}})
			}
			before, _ := json.Marshal(doc)
			if mode == "private_sibling" {
				if got := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, doc, types.BuildAnswerSemanticViewForBusContext(ctx), nil); len(got) != 0 {
					t.Fatalf("private projection must not be reclassified as a model claim: %+v", got)
				}
				after, _ := json.Marshal(doc)
				if string(before) != string(after) || doc.Blocks[3].SystemGeneratedKind != types.AnswerSystemGeneratedRuntimeTrace {
					t.Fatal("read-only authority projection mutated the original graph")
				}
				return
			}
			res, err := (&EmitAnswerDocument{}).Execute(ctx, before)
			if err != nil || res.Success || res.Repair == nil || res.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON] == "" {
				t.Fatalf("mode=%s model relation escaped exact authority: %v %s", mode, err, res.Summary)
			}
			if mode == "mixed" {
				failed := ctx.Mutable.LastRejectedAnswerDocumentV2()
				if !reflect.DeepEqual(failed.Blocks[2].EdgeAnchors, doc.Blocks[2].EdgeAnchors) || !strings.Contains(failed.Blocks[2].Diagram.Body, "supposed dependency") {
					t.Fatal("gate repaired/deleted arrows instead of preserving the failed draft")
				}
			}
		})
	}
}

func TestHMC228CausalDiagramKeepsExistingMetadataRepair(t *testing.T) {
	ctx, rows := hmc228CausalDiagramContext(t, &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis})
	doc := hmc228WakeDiagram(rows)
	doc.Blocks[2].EdgeAnchors = nil
	raw, _ := json.Marshal(doc)
	res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || res.Success || res.Repair == nil {
		t.Fatalf("missing exact metadata must request its existing repair: %v %s", err, res.Summary)
	}
	var delta types.AnswerDiagramRelationRepairDelta
	if err := json.Unmarshal([]byte(res.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta); err != nil {
		t.Fatal(err)
	}
	lease := types.NewAnswerDiagramRelationRepairLease(ctx.Mutable.LastRejectedAnswerDocumentV2(), delta.Failures, delta.AllowedAdditions)
	ctx.Mutable.SetAnswerDiagramRelationRepairLease(lease)
	var edits []emitAnswerDiagramEdgeEdit
	for _, failure := range lease.Failures {
		for _, candidate := range lease.AllowedAdditions {
			if candidate.RelationKind == types.DiagramRelWakeup && types.AnswerDiagramRelationRepairFailureCanAttachCandidate(failure, candidate) {
				edits = append(edits, emitAnswerDiagramEdgeEdit{Action: "attach", FailureRef: failure.FailureRef, AdditionRef: candidate.AdditionRef, Edge: &types.DiagramEdgeAnchor{FromNode: failure.FromNode, ToNode: failure.ToNode, VisibleLabel: "observed wakeup"}})
				break
			}
		}
	}
	if len(edits) != 2 {
		t.Fatalf("both real wakeups need executable metadata-only repairs, got %d; delta=%+v; lease=%+v", len(edits), delta, lease)
	}
	raw, _ = json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary", "events"}, "diagram_edge_edits": edits})
	res, err = (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
	if err != nil || !res.Success {
		t.Fatalf("actual metadata-only repair failed: %v %s", err, res.Summary)
	}
	accepted := ctx.Mutable.AnswerDocumentV2()
	if accepted.Blocks[2].Diagram.Body != doc.Blocks[2].Diagram.Body {
		t.Fatal("binding exact metadata changed the visible wakeup sequence")
	}
	copy := ctx.Mutable.AnswerDocumentV2()
	if _, isolated := IsolateRecoveredAnswerDiagrams(ctx, copy, nil, "en"); isolated != 0 || copy.Blocks[2].Diagram == nil {
		t.Fatal("an unrelated later failure must not quarantine the verified causal-mode relations")
	}
}

func TestHMC228CausalDiagramPublicEmitKeepsExactWakeups(t *testing.T) {
	for _, kind := range []types.DiagramKind{types.DiagramSequence, types.DiagramFlow, types.DiagramCallDAG, types.DiagramArchitecture} {
		t.Run(string(kind), func(t *testing.T) {
			hmc228CausalDiagramExactWakeups(t, kind)
		})
	}
}

func hmc228CausalDiagramExactWakeups(t *testing.T, kind types.DiagramKind) {
	t.Helper()
	ctx, rows := hmc228CausalDiagramContext(t, &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis})
	doc := hmc228WakeDiagram(rows)
	doc.Blocks[2].Diagram.Kind = kind
	if kind != types.DiagramSequence {
		doc.Blocks[2].Diagram.Body = "flowchart TD\n"
		for _, row := range rows {
			doc.Blocks[2].Diagram.Body += fmt.Sprintf(" %s[%s] -->|observed wakeup| %s[%s]\n", row.FromNode, row.FromLabel, row.ToNode, row.ToLabel)
		}
	}
	raw, _ := json.Marshal(doc)
	res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || !res.Success {
		t.Fatalf("actual wakeup relations must pass normal emit: %v %+v", err, res)
	}
	accepted := ctx.Mutable.AnswerDocumentV2()
	if got := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, accepted, types.BuildAnswerSemanticViewForBusContext(ctx), nil); len(got) != 0 {
		t.Fatalf("post-finalizer disagrees with actual emit: %+v", got)
	}
	if !strings.Contains(accepted.Blocks[2].Diagram.Body, "storage-irq-80") || !strings.Contains(accepted.Blocks[2].Diagram.Body, "document-worker-200") || !strings.Contains(accepted.Blocks[2].Diagram.Body, "app-main-100") {
		t.Fatal("real wakeup identities disappeared")
	}
}

func TestHMC228CausalDiagramPublicDependentMetadataRepair(t *testing.T) {
	ctx, rows := hmc228CausalDiagramContext(t, &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis})
	doc := hmc228WakeDiagram(rows)
	doc.Blocks[2].Diagram.Body += " participant X as unknown-one\n participant Y as unknown-two\n X->>Y: unsupported\n"
	for i := 0; i < 3; i++ {
		doc.Blocks[2].EdgeAnchors = append(doc.Blocks[2].EdgeAnchors, types.DiagramEdgeAnchor{FromNode: "X", ToNode: "Y", FromIdentity: "unknown-one", ToIdentity: "unknown-two", RelationKind: types.DiagramRelCall, VisibleLabel: fmt.Sprintf("unsupported %d", i)})
	}
	raw, _ := json.Marshal(doc)
	res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || res.Success || res.Repair == nil {
		t.Fatalf("unproved model edge must be rejected: %v %+v", err, res)
	}
	var delta types.AnswerDiagramRelationRepairDelta
	if err := json.Unmarshal([]byte(res.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta); err != nil {
		t.Fatal(err)
	}
	base := ctx.Mutable.LastRejectedAnswerDocumentV2()
	// Model JSON cannot manufacture this marker. A deterministic supplement
	// can coexist with the failed model graph in the retained repair base.
	base.Blocks = append(base.Blocks, types.AnswerBlock{ID: "native_projection", Kind: types.BlockDiagram,
		SystemGeneratedKind: types.AnswerSystemGeneratedRuntimeTrace,
		Diagram:             &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart TD\n NativeA --> NativeB"}})
	lease := types.NewAnswerDiagramRelationRepairLease(base, delta.Failures, delta.AllowedAdditions)
	var selected string
	for _, failure := range lease.Failures {
		if failure.CanRemoveVisibleBodyOccurrence("sequence", "X", "Y", 1, 1) {
			selected = failure.FailureRef
			break
		}
	}
	if selected == "" {
		t.Fatal("public failure lost its exact body removal")
	}
	ctx.Mutable.SetPendingAnswerDocumentPatchBase(base)
	ctx.Mutable.SetAnswerDiagramRelationRepairLease(lease)
	raw, _ = json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary", "events", "native_projection"}, "diagram_edge_edits": []map[string]string{{"failure_ref": selected, "action": "remove"}}})
	res, err = (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
	if err != nil || res.Success || res.Repair == nil || res.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome] != types.AnswerDocumentPatchOutcomeStagedForRetry {
		t.Fatalf("causal model metadata dependency must stage with a fresh repair: %v %+v", err, res)
	}
	staged := ctx.Mutable.PendingAnswerDocumentPatchBase()
	if staged == nil || strings.Contains(staged.Blocks[2].Diagram.Body, "X->>Y") || ctx.Mutable.AnswerDocumentV2() != nil {
		t.Fatal("dependency repair accepted invalid metadata or lost the selected body removal")
	}
	lease = ctx.Mutable.AnswerDiagramRelationRepairLease()
	if lease == nil || len(lease.Failures) == 0 {
		t.Fatal("causal metadata dependency lost its executable follow-up")
	}
	var removals []map[string]string
	for _, failure := range lease.Failures {
		if failure.Issue != diagramCallEdgeIssueAnchorWithoutBodyEdge || !failure.AllowsAction("remove") {
			t.Fatalf("unexpected dependency failure: %+v", failure)
		}
		removals = append(removals, map[string]string{"failure_ref": failure.FailureRef, "action": "remove"})
	}
	raw, _ = json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary", "events", "native_projection"}, "diagram_edge_edits": removals})
	res, err = (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
	if err != nil || !res.Success || len(ctx.Mutable.AnswerDocumentV2().Blocks[2].EdgeAnchors) != 2 {
		t.Fatalf("fresh dependency cleanup must preserve both true wakeups: %v %+v", err, res)
	}
	got := projectionClusterBlock(ctx.Mutable.AnswerDocumentV2().Blocks, "native_projection")
	want := projectionClusterBlock(base.Blocks, "native_projection")
	if got == nil || want == nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("metadata-only model repair changed the private projection: %+v", got)
	}
}
