package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/stageauthority"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func diagramEvidenceTestCall(subject, object string) types.EvidenceItem {
	return types.EvidenceItem{
		ID:              "ev-" + subject + "-" + object,
		Kind:            types.EvidenceRelationship,
		Subject:         subject,
		Predicate:       "calls",
		Object:          object,
		Source:          "internal/example.go",
		LineStart:       10,
		AnchorKind:      types.AnchorCall,
		Scope:           types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
		Producer:        types.EvidenceProducerExplorerEmitEvidence,
	}
}

func TestDiagramStagePrecedenceAuthorityAcceptsOnlyAdjacentReadLane(t *testing.T) {
	rows := []stageauthority.StageRow{
		{StageIdent: "StageAnalyze", StageValue: "analyze", AgentIdent: "AgentAnalyzer", AgentValue: "analyzer"},
		{StageIdent: "StageExplore", StageValue: "explore", AgentIdent: "AgentExplorer", AgentValue: "explorer"},
		{StageIdent: "StageExtract", StageValue: "extract", AgentIdent: "AgentExtractor", AgentValue: "extractor"},
		{StageIdent: "StageFinalize", StageValue: "finalize", AgentIdent: "AgentFinalizer", AgentValue: "finalizer"},
	}
	authority := []stageauthority.PrecedenceRelation{
		{From: rows[0], To: rows[1]},
		{From: rows[1], To: rows[2]},
		{From: rows[2], To: rows[3]},
	}
	doc := func(fromID, fromLabel, toID, toLabel string) *types.AnswerDocumentV2 {
		return &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
			ID: "stages", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid",
				Body: "flowchart TD\n  " + fromID + "[" + fromLabel + "] --> " + toID + "[" + toLabel + "]\n"},
			EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: fromID, ToNode: toID, RelationKind: types.DiagramRelPrecedence}},
		}}}
	}
	view := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}
	for _, tc := range []struct {
		name       string
		fromID     string
		fromLabel  string
		toID       string
		toLabel    string
		wantReject bool
	}{
		{name: "stage identifiers", fromID: "A", fromLabel: "StageAnalyze", toID: "E", toLabel: "StageExplore"},
		{name: "agent values", fromID: "A", fromLabel: "Analyzer", toID: "E", toLabel: "Explorer"},
		{name: "reverse", fromID: "E", fromLabel: "StageExplore", toID: "A", toLabel: "StageAnalyze", wantReject: true},
		{name: "skipped stage", fromID: "A", fromLabel: "StageAnalyze", toID: "X", toLabel: "StageExtract", wantReject: true},
		{name: "extra participant", fromID: "B", fromLabel: "BusContext", toID: "A", toLabel: "StageAnalyze", wantReject: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := DiagramCallEdgeEvidenceMismatches(doc(tc.fromID, tc.fromLabel, tc.toID, tc.toLabel), view, nil, authority)
			if tc.wantReject && len(got) == 0 {
				t.Fatal("unproved/non-adjacent relation must be rejected")
			}
			if !tc.wantReject && len(got) != 0 {
				t.Fatalf("exact adjacent relation should pass: %+v", got)
			}
		})
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc("A", "StageAnalyze", "E", "StageExplore"),
		&types.AnswerSemanticView{Family: types.QFRootCauseTrace, RelationAxis: types.AxisFlow}, nil, authority); len(got) != 0 {
		t.Fatalf("runtime Trace must remain outside source stage authority: %+v", got)
	}
}

func TestDiagramStagePrecedenceAuthorityTreatsSameStageLabelAliasesAsOneEndpoint(t *testing.T) {
	rows := []stageauthority.StageRow{
		{StageIdent: "StageAnalyze", StageValue: "analyze", AgentIdent: "AgentAnalyzer", AgentValue: "analyzer"},
		{StageIdent: "StageExplore", StageValue: "explore", AgentIdent: "AgentExplorer", AgentValue: "explorer"},
		{StageIdent: "StageExtract", StageValue: "extract", AgentIdent: "AgentExtractor", AgentValue: "extractor"},
		{StageIdent: "StageFinalize", StageValue: "finalize", AgentIdent: "AgentFinalizer", AgentValue: "finalizer"},
	}
	authority := []stageauthority.PrecedenceRelation{
		{From: rows[0], To: rows[1]},
		{From: rows[1], To: rows[2]},
		{From: rows[2], To: rows[3]},
	}
	doc := func(extractorLabel string) *types.AnswerDocumentV2 {
		return &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
			ID: "stages", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid",
				Body: "flowchart TD\n  E[\"explorer\\nStageExplore\"] --> X[\"" + extractorLabel + "\"]\n  X --> F[\"finalizer\\nStageFinalize\"]\n"},
			EdgeAnchors: []types.DiagramEdgeAnchor{
				{FromNode: "E", ToNode: "X", RelationKind: types.DiagramRelPrecedence},
				{FromNode: "X", ToNode: "F", RelationKind: types.DiagramRelPrecedence},
			},
		}}}
	}
	view := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}
	if got := DiagramCallEdgeEvidenceMismatches(doc("extractor\\nStageExtract"), view, nil, authority); len(got) != 0 {
		t.Fatalf("two exact aliases of one stage must retain both precedence edges: %+v", got)
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc("extractor\\nStageFinalize"), view, nil, authority); len(got) == 0 {
		t.Fatal("a label whose exact lines resolve to different stages must remain ambiguous")
	}
}

func TestDiagramStagePrecedenceAuthorityBindsVisibleAndAnchorAliasesWithinOneVerifiedRow(t *testing.T) {
	rows := []stageauthority.StageRow{
		{StageIdent: "StageAnalyze", StageValue: "analyze", AgentIdent: "AgentAnalyzer", AgentValue: "analyzer"},
		{StageIdent: "StageExplore", StageValue: "explore", AgentIdent: "AgentExplorer", AgentValue: "explorer"},
	}
	authority := []stageauthority.PrecedenceRelation{{From: rows[0], To: rows[1]}}
	evidence := []types.EvidenceItem{
		{
			Kind: types.EvidenceMechanism, Subject: "AgentAnalyzer", AnchorSymbol: "AgentAnalyzer",
			Source: "internal/types/enums.go", LineStart: 130, Scope: types.ScopeLine,
			AnchorKind: types.AnchorDefinition, GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind: types.EvidenceMechanism, Subject: "AgentExplorer", AnchorSymbol: "AgentExplorer",
			Source: "internal/types/enums.go", LineStart: 131, Scope: types.ScopeLine,
			AnchorKind: types.AnchorDefinition, GroundingStatus: types.GroundingGrounded,
		},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "stages", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			"  participant analyzer as AgentAnalyzer",
			"  participant explorer as AgentExplorer",
			"  analyzer->>explorer: next stage",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "analyzer", ToNode: "explorer",
			FromIdentity: "analyzer", ToIdentity: "explorer",
			RelationKind: types.DiagramRelPrecedence,
		}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence, authority); len(got) != 0 {
		t.Fatalf("visible declaration aliases and value anchors from one verified stage row must agree: %+v", got)
	}

	// The exception is scoped to an exact verified precedence pair. Reversing
	// the typed identities is still a visible-node contradiction.
	doc.Blocks[0].EdgeAnchors[0].FromIdentity = "explorer"
	doc.Blocks[0].EdgeAnchors[0].ToIdentity = "analyzer"
	got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence, authority)
	var conflict *DiagramCallEdgeEvidenceMismatch
	for i := range got {
		if got[i].Issue == diagramEdgeAnchorNodeIdentityConflict {
			conflict = &got[i]
			break
		}
	}
	if conflict == nil || conflict.FromNodeSymbol != "AgentAnalyzer" || conflict.ToNodeSymbol != "AgentExplorer" {
		t.Fatalf("cross-stage identity reversal must remain a typed, actionable conflict: %+v", got)
	}

	// A call edge cannot borrow the stage alias family. Relation kinds keep
	// their independent exact authority.
	doc.Blocks[0].EdgeAnchors[0].FromIdentity = "analyzer"
	doc.Blocks[0].EdgeAnchors[0].ToIdentity = "explorer"
	doc.Blocks[0].EdgeAnchors[0].RelationKind = types.DiagramRelCall
	got = DiagramCallEdgeEvidenceMismatches(doc, view, evidence, authority)
	conflict = nil
	for i := range got {
		if got[i].Issue == diagramEdgeAnchorNodeIdentityConflict {
			conflict = &got[i]
			break
		}
	}
	if conflict == nil {
		t.Fatalf("call relations must not inherit the stage-only alias bridge: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatchesRepeatedCallOccurrenceNeedsDistinctCallSites(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "pipeline", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramSequence, Language: "mermaid",
			Body: "sequenceDiagram\n  participant n1 as Orchestrator.runAnalyzePhase\n  participant n2 as Orchestrator.dispatchStage\n  n1->>n2: call\n  n1->>n2: call\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "n1", ToNode: "n2", RelationKind: types.DiagramRelCall},
			{FromNode: "n1", ToNode: "n2", RelationKind: types.DiagramRelCall},
		},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFGeneric, RelationAxis: types.AxisFlow}
	call := types.EvidenceItem{
		ID: "call-1", Kind: types.EvidenceRelationship, Scope: types.ScopeLine,
		AnchorKind: types.AnchorCall, AnchorSymbol: "dispatchStage",
		Subject: "Orchestrator.runAnalyzePhase", Object: "Orchestrator.dispatchStage",
		Source: "internal/orchestrator/orchestrator.go", LineStart: 2485,
		GroundingStatus: types.GroundingGrounded,
	}

	got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{call})
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueOccurrenceUnproven {
		t.Fatalf("one call site must own only one visible occurrence, got %+v", got)
	}

	second := call
	second.ID = "call-2"
	second.LineStart = 2498
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{call, second}); len(got) != 0 {
		t.Fatalf("two distinct call sites may own two visible occurrences, got %+v", got)
	}

	duplicate := call
	duplicate.ID = "duplicate-copy"
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{call, duplicate}); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueOccurrenceUnproven {
		t.Fatalf("a duplicated evidence row must not increase occurrence authority, got %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatchesActorSelfMessagesKeepDistinctTypedOperations(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "pipeline", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramSequence, Language: "mermaid",
			Body: "sequenceDiagram\n  participant orch as Orchestrator\n  orch->>orch: 分析请求\n  orch->>orch: 执行调查图\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "orch", ToNode: "orch", FromIdentity: "Orchestrator.runAnalyzePhase", ToIdentity: "Orchestrator.dispatchStage", RelationKind: types.DiagramRelCall},
			{FromNode: "orch", ToNode: "orch", FromIdentity: "Orchestrator.runTaskGraph", ToIdentity: "Orchestrator.runReadSchedulerLoop", RelationKind: types.DiagramRelCall},
		},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFGeneric, RelationAxis: types.AxisFlow}
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("Orchestrator.runAnalyzePhase", "Orchestrator.dispatchStage"),
		diagramEvidenceTestCall("Orchestrator.runTaskGraph", "Orchestrator.runReadSchedulerLoop"),
	}
	evidence[1].ID = "ev-run-task-graph"
	evidence[1].LineStart = 4218
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 0 {
		t.Fatalf("actor self-messages must retain their ordered typed operation identities: %+v", got)
	}

	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence[:1]); len(got) != 1 ||
		got[0].Issue != diagramCallEdgeIssueNoEvidence ||
		got[0].FromSymbol != "Orchestrator.runTaskGraph" ||
		got[0].ToSymbol != "Orchestrator.runReadSchedulerLoop" {
		t.Fatalf("an unproved second self-message must still fail closed on its own typed pair: %+v", got)
	}

	doc.Blocks[0].Diagram.Body += "  orch->>orch: 未授权动作\n"
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("a visible self-message without an occurrence-aligned typed pair must fail closed: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatchesActorSelfMessagesRejectHiddenTypedOperation(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "pipeline", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramSequence, Language: "mermaid",
			Body: "sequenceDiagram\n  participant orch as Orchestrator\n  orch->>orch: 分析请求\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "orch", ToNode: "orch", FromIdentity: "Orchestrator.runAnalyzePhase", ToIdentity: "Orchestrator.dispatchStage", RelationKind: types.DiagramRelCall},
			{FromNode: "orch", ToNode: "orch", FromIdentity: "Orchestrator.runTaskGraph", ToIdentity: "Orchestrator.runReadSchedulerLoop", RelationKind: types.DiagramRelCall},
		},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFGeneric, RelationAxis: types.AxisFlow}
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("Orchestrator.runAnalyzePhase", "Orchestrator.dispatchStage"),
		diagramEvidenceTestCall("Orchestrator.runTaskGraph", "Orchestrator.runReadSchedulerLoop"),
	}
	evidence[1].ID = "ev-run-task-graph"
	evidence[1].LineStart = 4218
	got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence)
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueAnchorWithoutBodyEdge ||
		got[0].FromSymbol != "Orchestrator.runTaskGraph" ||
		got[0].ToSymbol != "Orchestrator.runReadSchedulerLoop" {
		t.Fatalf("a second exact operation anchor needs a second visible self-message: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_CallbackHandoffNeedsExactTypedDirection(t *testing.T) {
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCallback,
		Subject: "loop.run_in_executor", Object: "plugin.handle", AnchorSymbol: "plugin.handle",
		Source: "pipeline/runner.py", LineStart: 17, GroundingStatus: types.GroundingGrounded,
	}}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "d", Kind: types.BlockDiagram,
		Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramCallDAG, Language: "mermaid", Body: "flowchart TD\n  Exec[\"loop.run_in_executor\"] -->|handoff| Plugin[\"plugin.handle\"]"},
		EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "Exec", ToNode: "Plugin", RelationKind: types.DiagramRelCallback}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFCallChain}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 0 {
		t.Fatalf("typed callback handoff rejected: %+v", got)
	}
	doc.Blocks[0].EdgeAnchors[0].FromNode, doc.Blocks[0].EdgeAnchors[0].ToNode = "Plugin", "Exec"
	doc.Blocks[0].Diagram.Body = "flowchart TD\n  Plugin[\"plugin.handle\"] -->|handoff| Exec[\"loop.run_in_executor\"]"
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 1 || got[0].Issue != diagramCallbackEdgeIssueNoEvidence {
		t.Fatalf("reverse callback handoff must fail closed: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_ArgumentFlowNeedsExactTypedDirection(t *testing.T) {
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceRelationship, AnchorKind: types.AnchorArgument,
		Subject: "o.busCtx", Object: "ctxbuilder.BuildAgentContext", AnchorSymbol: "o.busCtx",
		Source: "internal/orchestrator/orchestrator.go", LineStart: 8026,
		GroundingStatus: types.GroundingGrounded,
	}}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "d", Kind: types.BlockDiagram,
		Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n  Bus[o.busCtx] --> Builder[ctxbuilder.BuildAgentContext]"},
		EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "Bus", ToNode: "Builder", RelationKind: types.DiagramRelArgumentFlow}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFGeneric, RelationAxis: types.AxisFlow}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 0 {
		t.Fatalf("typed argument handoff rejected: %+v", got)
	}
	doc.Blocks[0].Diagram.Body = "flowchart LR\n  Builder[ctxbuilder.BuildAgentContext] --> Bus[o.busCtx]"
	doc.Blocks[0].EdgeAnchors[0].FromNode, doc.Blocks[0].EdgeAnchors[0].ToNode = "Builder", "Bus"
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 1 || got[0].Issue != diagramArgumentFlowEdgeIssueNoEvidence {
		t.Fatalf("reverse argument handoff must fail closed: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_ExactTypedTupleCannotAuthorizeSeveralVisibleEndpointPairs(t *testing.T) {
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceRelationship, AnchorKind: types.AnchorArgument,
		Subject: "o.busCtx", Object: "ctxbuilder.BuildAgentContext", AnchorSymbol: "o.busCtx",
		Source: "internal/orchestrator/extract_work.go", LineStart: 15,
		GroundingStatus: types.GroundingGrounded,
	}}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "pipeline", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: strings.Join([]string{
			"flowchart LR",
			"  BC[BusContext] --> A[Analyzer]",
			"  BC --> E[Explorer]",
			"  BC --> X[Extractor]",
			"  BC --> F[Finalizer]",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "BC", ToNode: "A", FromIdentity: "o.busCtx", ToIdentity: "ctxbuilder.BuildAgentContext", RelationKind: types.DiagramRelArgumentFlow},
			{FromNode: "BC", ToNode: "E", FromIdentity: "o.busCtx", ToIdentity: "ctxbuilder.BuildAgentContext", RelationKind: types.DiagramRelArgumentFlow},
			{FromNode: "BC", ToNode: "X", FromIdentity: "o.busCtx", ToIdentity: "ctxbuilder.BuildAgentContext", RelationKind: types.DiagramRelArgumentFlow},
			{FromNode: "BC", ToNode: "F", FromIdentity: "o.busCtx", ToIdentity: "ctxbuilder.BuildAgentContext", RelationKind: types.DiagramRelArgumentFlow},
		},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFGeneric, RelationAxis: types.AxisFlow}
	got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence)
	if len(got) != 4 {
		t.Fatalf("every conflicting visible mapping must be surfaced without choosing one for the model: %+v", got)
	}
	for _, mismatch := range got {
		if mismatch.Issue != diagramTypedRelationTupleEndpointReused ||
			mismatch.FromSymbol != "o.busCtx" || mismatch.ToSymbol != "ctxbuilder.BuildAgentContext" {
			t.Fatalf("unexpected tuple-reuse diagnosis: %+v", got)
		}
	}

	// A single model-authored mapping of that fact is valid.
	doc.Blocks[0].Diagram.Body = "flowchart LR\n  BC[BusContext] --> X[Extractor]"
	doc.Blocks[0].EdgeAnchors = doc.Blocks[0].EdgeAnchors[2:3]
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 0 {
		t.Fatalf("one exact tuple-to-visible-pair mapping must remain legal: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_DifferentTypedTuplesMayUseDifferentVisibleEndpointPairs(t *testing.T) {
	evidence := []types.EvidenceItem{
		{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorArgument, Subject: "ctx", Object: "BuildAnalyzer", Source: "pipeline.go", LineStart: 10, GroundingStatus: types.GroundingGrounded},
		{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorArgument, Subject: "ctx", Object: "BuildExplorer", Source: "pipeline.go", LineStart: 11, GroundingStatus: types.GroundingGrounded},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "pipeline", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n  BC --> A\n  BC --> E"},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "BC", ToNode: "A", FromIdentity: "ctx", ToIdentity: "BuildAnalyzer", RelationKind: types.DiagramRelArgumentFlow},
			{FromNode: "BC", ToNode: "E", FromIdentity: "ctx", ToIdentity: "BuildExplorer", RelationKind: types.DiagramRelArgumentFlow},
		},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFGeneric, RelationAxis: types.AxisFlow}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 0 {
		t.Fatalf("distinct typed relation tuples must retain their independent visible edges: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SequenceCallbackUsesCallbackAuthority(t *testing.T) {
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCallback,
		Subject: "loop.run_in_executor", Object: "handle", AnchorSymbol: "handle",
		Source: "pipeline/runner.py", LineStart: 17, Scope: types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
	}}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "callback-sequence", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			"  participant n6 as loop.run_in_executor",
			"  participant n7 as handle",
			"  n6->>n7: callback",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "n6", ToNode: "n7", RelationKind: types.DiagramRelCallback}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFCallChain}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 0 {
		t.Fatalf("copy-ready sequence callback must consume callback authority instead of call authority: %+v", got)
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, nil); len(got) != 1 || got[0].Issue != diagramCallbackEdgeIssueNoEvidence {
		t.Fatalf("an unproved sequence callback must fail on callback evidence, not missing call authority: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SequenceNonCallOwnerNeverAlsoRequiresCall(t *testing.T) {
	for _, relation := range types.AllDiagramRelationKinds() {
		if relation == types.DiagramRelCall {
			continue
		}
		t.Run(string(relation), func(t *testing.T) {
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
				ID: "typed-sequence", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
					"sequenceDiagram",
					"  participant A as Source.Member",
					"  participant B as Target.Member",
					"  A->>B: typed relation",
				}, "\n")},
				EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", RelationKind: relation}},
			}}}
			view := &types.AnswerSemanticView{Family: types.QFGeneric, RelationAxis: types.AxisFlow}
			got := DiagramCallEdgeEvidenceMismatches(doc, view, nil)
			if len(got) == 0 {
				t.Fatalf("unsupported typed relation %q must still fail its own evidence gate", relation)
			}
			for _, mismatch := range got {
				if mismatch.Issue == diagramCallEdgeIssueMissingAnchor ||
					mismatch.Issue == diagramCallEdgeIssueNoEvidence ||
					mismatch.Issue == diagramCallEdgeIssueOccurrenceUnproven {
					t.Fatalf("typed non-call relation %q was also forced through call authority: %+v", relation, got)
				}
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SequenceAssignmentUsesAssignmentAuthority(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "assignment-sequence", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			"  participant EA as EmitAnalysis.Execute",
			"  participant MR as Mutable.RequestModel",
			"  EA-->MR: SetRequestModel",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "EA", ToNode: "MR", RelationKind: types.DiagramRelAssignment}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFGeneric, RelationAxis: types.AxisFlow}
	assignment := types.EvidenceItem{
		ID: "assignment", Kind: types.EvidenceDirect, Scope: types.ScopeLine,
		AnchorKind: types.AnchorAssignment, Subject: "EmitAnalysis.Execute", Object: "Mutable.RequestModel",
		Source: "internal/tool/emit_analysis.go", LineStart: 2083, GroundingStatus: types.GroundingGrounded,
		Snippet: "EmitAnalysis.Execute = Mutable.RequestModel",
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{assignment}); len(got) != 0 {
		t.Fatalf("same-direction typed assignment must own a sequence edge without call authority: %+v", got)
	}

	doc.Blocks[0].EdgeAnchors[0].RelationKind = types.DiagramRelCall
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{assignment}); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("assignment evidence must not authorize an explicitly relabelled call: %+v", got)
	}
}

// M6-U3-1/U3-2: every non-call strict relation family shares the same
// side-aware identity projection as calls. One uniquely qualified typed
// identity may be displayed by its short tail, while owner ambiguity fails
// closed and short evidence cannot mint a model-authored qualified owner.
func TestDiagramRelationEvidence_AllStrictFamiliesUseExactOrUniqueShortProjection(t *testing.T) {
	tests := []struct {
		name      string
		evidence  types.EvidenceItem
		fromShort string
		toShort   string
		fromFull  string
		toFull    string
		match     func([]types.EvidenceItem, string, string) bool
		shorten   func(*types.EvidenceItem)
	}{
		{
			name: "callback",
			evidence: types.EvidenceItem{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCallback,
				Subject: "runtime.Executor.run", Object: "plugin.Handler.handle", AnchorSymbol: "plugin.Handler.handle",
				Source: "pipeline/runner.py", LineStart: 17, GroundingStatus: types.GroundingGrounded},
			fromShort: "run", toShort: "handle", fromFull: "runtime.Executor.run", toFull: "plugin.Handler.handle",
			match:   diagramCallbackEdgeHasTypedEvidence,
			shorten: func(ev *types.EvidenceItem) { ev.Subject, ev.Object, ev.AnchorSymbol = "run", "handle", "handle" },
		},
		{
			name: "type_relation",
			evidence: types.EvidenceItem{Kind: types.EvidenceRelationship, Producer: "repomap_structural_relation",
				Subject: "storage.FileSink", Predicate: "inheritance", Object: "logging.Sink",
				Source: "include/file_sink.hpp", LineStart: 10, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition},
			fromShort: "FileSink", toShort: "Sink", fromFull: "storage.FileSink", toFull: "logging.Sink",
			match:   diagramTypeRelationEdgeHasTypedEvidence,
			shorten: func(ev *types.EvidenceItem) { ev.Subject, ev.Object = "FileSink", "Sink" },
		},
		{
			name: "registration",
			evidence: types.EvidenceItem{Kind: types.EvidenceRegistration, Subject: "runtime.Module.register",
				Object: "worker.Handler.handle", Source: "src/lib.rs", LineStart: 47,
				AnchorKind: types.AnchorDefinition, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
			fromShort: "register", toShort: "handle", fromFull: "runtime.Module.register", toFull: "worker.Handler.handle",
			match:   diagramRegistrationEdgeHasTypedEvidence,
			shorten: func(ev *types.EvidenceItem) { ev.Subject, ev.Object = "register", "handle" },
		},
		{
			name: "value_flow",
			evidence: types.EvidenceItem{Kind: types.EvidenceDirect, Subject: "factory.Provider.create",
				Object: "processor.FastProcessor", Source: "src/factory.cj", LineStart: 17,
				AnchorKind: types.AnchorReturn, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
			fromShort: "create", toShort: "FastProcessor", fromFull: "factory.Provider.create", toFull: "processor.FastProcessor",
			match: func(rows []types.EvidenceItem, from, to string) bool {
				return diagramValueFlowEdgeHasTypedEvidence(rows, from, to, types.DiagramRelReturn)
			},
			shorten: func(ev *types.EvidenceItem) { ev.Subject, ev.Object = "create", "FastProcessor" },
		},
		{
			name: "logical_guard",
			evidence: types.EvidenceItem{Kind: types.EvidenceConditional, AnchorKind: types.AnchorCondition,
				Subject: "service.Service.run", OwnerSymbol: "service.Service.run", AnchorSymbol: "config.enabled",
				Source: "service.go", LineStart: 10, GroundingStatus: types.GroundingGrounded},
			fromShort: "run", toShort: "enabled", fromFull: "service.Service.run", toFull: "config.enabled",
			match: func(rows []types.EvidenceItem, from, to string) bool {
				return diagramLogicalRelationEdgeHasTypedEvidence(rows, from, to, types.DiagramRelGuard)
			},
			shorten: func(ev *types.EvidenceItem) {
				ev.Subject, ev.OwnerSymbol, ev.AnchorSymbol = "run", "run", "enabled"
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.match([]types.EvidenceItem{tc.evidence}, tc.fromShort, tc.toShort) {
				t.Fatalf("one uniquely qualified typed relation must authorize its short presentation")
			}

			ambiguous := tc.evidence
			ambiguous.ID = "ambiguous-owner"
			ambiguous.Source = "other/source"
			ambiguous.LineStart++
			ambiguous.Subject = "other.Owner." + tc.fromShort
			ambiguous.OwnerSymbol = ambiguous.Subject
			if tc.match([]types.EvidenceItem{tc.evidence, ambiguous}, tc.fromShort, tc.toShort) {
				t.Fatalf("same-tail source identities under different owners must fail closed")
			}

			shortEvidence := tc.evidence
			tc.shorten(&shortEvidence)
			if tc.match([]types.EvidenceItem{shortEvidence}, tc.fromFull, tc.toFull) {
				t.Fatalf("short typed evidence must not mint qualified diagram owners")
			}
		})
	}
}

func diagramEvidenceTestDefinition(owner, operation, source string, line int) types.EvidenceItem {
	return types.EvidenceItem{
		ID:              "ev-def-" + owner + "-" + operation,
		Kind:            types.EvidenceDirect,
		Subject:         owner,
		Source:          source,
		LineStart:       line,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    operation,
		Scope:           types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
	}
}

func diagramEvidenceTestDoc(from, to string) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "sequence",
		Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind:     types.DiagramSequence,
			Language: "mermaid",
			Body: "sequenceDiagram\n" +
				"  participant A as Alpha.Run\n" +
				"  participant B as Beta.Run\n" +
				"  " + from + "->>" + to + ": invoke\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode:     from,
			ToNode:       to,
			RelationKind: types.DiagramRelCall,
			ClaimForm:    types.ClaimCallEdge,
		}},
	}}}
}

func TestDiagramCallEdgeEvidenceMismatches_DirectionUsesTypedEvidence(t *testing.T) {
	view := &types.AnswerSemanticView{Family: types.QFCallChain}
	evidence := []types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")}
	if got := DiagramCallEdgeEvidenceMismatches(diagramEvidenceTestDoc("A", "B"), view, evidence); len(got) != 0 {
		t.Fatalf("typed call direction should pass: %+v", got)
	}
	got := DiagramCallEdgeEvidenceMismatches(diagramEvidenceTestDoc("B", "A"), view, evidence)
	if len(got) != 1 || got[0].FromSymbol != "Beta.Run" || got[0].ToSymbol != "Alpha.Run" {
		t.Fatalf("reverse edge should be rejected from structured direction, got %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_QualifiedPresentationSeparatorsDoNotChangeIdentity(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "cpp-call", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramCallDAG, Language: "mermaid",
			Body: "flowchart TD\n  A[Logger::log] --> B[sink_->write]\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall,
			ClaimForm: types.ClaimCallEdge,
		}},
	}}}
	evidence := diagramEvidenceTestCall("Logger.log", "sink_.write")
	evidence.AnchorSymbol = "sink_.write"
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFGeneric}, []types.EvidenceItem{evidence}); len(got) != 0 {
		t.Fatalf("language-native presentation separators must preserve exact typed identity: %+v", got)
	}

	wrongOwner := evidence
	wrongOwner.Subject = "OtherLogger.log"
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFGeneric}, []types.EvidenceItem{wrongOwner}); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("presentation normalization must not collapse a different qualified owner: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SequenceReplyIsNotACallEdge(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body += "  B-->>A: result\n"
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
	if len(got) != 0 {
		t.Fatalf("a standard sequence reply must not require a reverse call edge: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SequenceCallCannotUseReplyOperator(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = strings.Replace(doc.Blocks[0].Diagram.Body, "A->>B", "A-->>B", 1)
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueReplyOperatorConflict ||
		got[0].FromNode != "A" || got[0].ToNode != "B" {
		t.Fatalf("a typed forward call rendered as a reply must fail only the operator contract: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SequenceCallReplyOperatorAppliesToGenericFamily(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = strings.Replace(doc.Blocks[0].Diagram.Body, "A->>B", "A-->>B", 1)
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFGeneric},
		[]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueReplyOperatorConflict {
		t.Fatalf("generic classification must not bypass sequence call/operator consistency: %+v", got)
	}
}

func TestDiagramSequenceReplyOperatorRelationConflict_ClosedMatrix(t *testing.T) {
	for _, relation := range types.AllDiagramRelationKinds() {
		t.Run(string(relation), func(t *testing.T) {
			got, conflict := diagramSequenceReplyOperatorRelationConflict(map[types.DiagramRelationKind]bool{relation: true})
			if relation == types.DiagramRelReturn {
				if conflict || got != types.DiagramRelUnknown {
					t.Fatalf("typed return must remain compatible with -->>: relation=%q conflict=%v", got, conflict)
				}
				return
			}
			if !conflict || got != relation {
				t.Fatalf("forward relation %q must conflict with response syntax: relation=%q conflict=%v", relation, got, conflict)
			}
		})
	}
	if got, conflict := diagramSequenceReplyOperatorRelationConflict(nil); conflict || got != types.DiagramRelUnknown {
		t.Fatalf("an unowned structural reply must remain available to the pairing check: relation=%q conflict=%v", got, conflict)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SequencePrecedenceCannotUseReplyOperator(t *testing.T) {
	relation := diagramTestReadModePrecedence()[0]
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "stage-sequence", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			"  participant A as StageAnalyze",
			"  participant E as StageExplore",
			"  A-->>E: next stage",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "E", RelationKind: types.DiagramRelPrecedence}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFGeneric, RelationAxis: types.AxisFlow}
	got := DiagramCallEdgeEvidenceMismatches(doc, view, nil, []stageauthority.PrecedenceRelation{relation})
	if len(got) != 1 || got[0].Issue != diagramSequenceRelationReplyConflict || got[0].Relation != types.DiagramRelPrecedence {
		t.Fatalf("checkout-proved stage order rendered as a reply must fail only operator semantics: %+v", got)
	}
	doc.Blocks[0].Diagram.Body = strings.Replace(doc.Blocks[0].Diagram.Body, "A-->>E", "A->>E", 1)
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, nil, []stageauthority.PrecedenceRelation{relation}); len(got) != 0 {
		t.Fatalf("the same typed precedence relation with forward syntax must pass: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_RuntimeTraceKeepsIndependentDiagramAuthority(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = strings.Replace(doc.Blocks[0].Diagram.Body, "A->>B", "A-->>B", 1)
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFRootCauseTrace}, nil)
	if len(got) != 0 {
		t.Fatalf("runtime trace causal diagrams must stay outside source sequence contracts: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SequenceMessagePayloadCannotPolluteSiblingEndpointIdentity(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "value-flow", Kind: types.BlockOrderedList,
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "resolve", ToNode: "JsonPlugin instance", RelationKind: types.DiagramRelReturn,
			}},
		},
		{
			ID: "sequence", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramSequence, Language: "mermaid",
				Body: strings.Join([]string{
					"sequenceDiagram",
					"  participant RP as run_pipeline",
					"  participant RSV as resolve",
					`  RP->>RSV: resolve("json")`,
				}, "\n"),
			},
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "RP", ToNode: "RSV", RelationKind: types.DiagramRelCall,
			}},
		},
	}}
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("run_pipeline", "resolve"),
		{
			Kind: types.EvidenceDirect, Subject: "resolve", Object: "JsonPlugin instance",
			Source: "plugins/registry.py", LineStart: 31, AnchorKind: types.AnchorReturn,
			Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
		},
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence); len(got) != 0 {
		t.Fatalf("sequence message arguments must not rewrite a sibling typed endpoint: %+v", got)
	}
	labels := diagramEvidenceNodeLabels(doc.Blocks[1].Diagram.Body, types.DiagramSequence)
	if got, exists := labels["resolve"]; exists {
		t.Fatalf("message payload minted a node declaration resolve=%q", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SequenceParticipantPresentationArrowCannotMintTypedEdge(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "sequence", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramSequence, Language: "mermaid",
			Body: strings.Join([]string{
				"sequenceDiagram",
				"  participant SW as sink_->write",
				"  participant CS as ConsoleSink::write",
				"  SW->>CS: write(message)",
			}, "\n"),
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "SW", ToNode: "CS", RelationKind: types.DiagramRelCall,
		}},
	}}}
	evidence := []types.EvidenceItem{diagramEvidenceTestCall("sink_.write", "ConsoleSink.write")}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence); len(got) != 0 {
		t.Fatalf("participant display label arrow bytes must not become typed body edges: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SequenceReplyCannotHideReverseCallAnchor(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body += "  B-->>A: result\n"
	doc.Blocks[0].EdgeAnchors = append(doc.Blocks[0].EdgeAnchors, types.DiagramEdgeAnchor{
		FromNode: "B", ToNode: "A", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
	})
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
	if len(got) != 2 || got[0].Issue != diagramCallEdgeIssueReplyOperatorConflict ||
		got[1].Issue != diagramCallEdgeIssueNoEvidence ||
		got[0].FromSymbol != "Beta.Run" || got[0].ToSymbol != "Alpha.Run" {
		t.Fatalf("an explicit reverse call anchor must use invocation syntax and prove its own direction: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_StructuralReplyStaysReplyWhenReverseCallExists(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body += "  B-->>A: result\n"
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("Alpha.Run", "Beta.Run"),
		diagramEvidenceTestCall("Beta.Run", "Alpha.Run"),
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence); len(got) != 0 {
		t.Fatalf("unrelated reverse typed evidence must not recapture a structurally paired response: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_UnpairedDashedEdgeIsNotSelfAuthorizingReply(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n  participant A as Alpha.Run\n  participant B as Beta.Run\n  B-->>A: invoke\n"
	doc.Blocks[0].EdgeAnchors = nil
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, nil)
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueMissingAnchor {
		t.Fatalf("an unpaired dashed edge must remain in the fail-closed call contract: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SequenceReplyCannotBorrowFutureInvocation(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = strings.Join([]string{
		"sequenceDiagram",
		"  participant A as Alpha.Run",
		"  participant B as Beta.Run",
		"  B-->>A: early result",
		"  A->>B: invoke",
	}, "\n")
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, []types.EvidenceItem{
		diagramEvidenceTestCall("Alpha.Run", "Beta.Run"),
	})
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueMissingAnchor ||
		got[0].FromNode != "B" || got[0].ToNode != "A" {
		t.Fatalf("a reply before its invocation must remain unpaired and fail closed: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SequenceInvocationCannotAuthorizeSeveralReplies(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body += "  B-->>A: first result\n  B-->>A: second result\n"
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, []types.EvidenceItem{
		diagramEvidenceTestCall("Alpha.Run", "Beta.Run"),
	})
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueMissingAnchor ||
		got[0].FromNode != "B" || got[0].ToNode != "A" || got[0].BodyOccurrence != 2 {
		t.Fatalf("one invocation must authorize only its first structural reply: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SequenceNestedRepliesConsumeNearestPriorInvocations(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "sequence", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			"  participant A as Alpha.Run",
			"  participant B as Beta.Run",
			"  participant C as Gamma.Run",
			"  A->>B: invokeB",
			"  B->>C: invokeC",
			"  C-->>B: resultC",
			"  B-->>A: resultB",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall},
			{FromNode: "B", ToNode: "C", RelationKind: types.DiagramRelCall},
		},
	}}}
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("Alpha.Run", "Beta.Run"),
		diagramEvidenceTestCall("Beta.Run", "Gamma.Run"),
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence); len(got) != 0 {
		t.Fatalf("nested call replies must pair with their preceding reverse endpoints: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_ClassParticipantsResolveUniqueTypedMethods(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "sequence", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			"  participant C as VisitController",
			"  participant S as VisitService",
			"  participant R as VisitRepository",
			"  C->>S: schedule(petId, reason)",
			"  S->>R: countOpenVisits(petId)",
			"  S->>R: insert(petId, reason)",
			"  R-->>S: insertedId",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "C", ToNode: "S", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
			{FromNode: "S", ToNode: "R", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
		},
	}}}
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("VisitController.create", "VisitService.schedule"),
		diagramEvidenceTestCall("VisitService.schedule", "VisitRepository.countOpenVisits"),
		diagramEvidenceTestCall("VisitService.schedule", "VisitRepository.insert"),
	}
	evidence[0].AnchorSymbol = "schedule"
	evidence[1].AnchorSymbol = "countOpenVisits"
	evidence[2].AnchorSymbol = "insert"
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence); len(got) != 0 {
		t.Fatalf("class participants plus an exact message operation should resolve unique typed method edges: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_ClassParticipantsResolveGroundingQualifiedAnchor(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "sequence", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			"  participant C as VisitController",
			"  participant S as VisitService",
			"  C->>S: schedule(petId, reason)",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "C", ToNode: "S", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
		}},
	}}}
	call := diagramEvidenceTestCall("VisitController.create", "VisitService.schedule")
	call.OwnerSymbol = "VisitController.create"
	// This is the production shape after normalizeCallEvidenceDirection: the
	// call anchor carries the resolved callee, not only its short operation.
	call.AnchorSymbol = "VisitService.schedule"
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, []types.EvidenceItem{call}); len(got) != 0 {
		t.Fatalf("class participants must consume the exact operation of a grounding-qualified anchor: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_ClassParticipantsFailClosedWhenMethodIsAmbiguous(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as VisitService\n" +
		"  participant B as VisitRepository\n" +
		"  A->>B: invoke\n"
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("VisitService.schedule", "VisitRepository.countOpenVisits"),
		diagramEvidenceTestCall("VisitService.schedule", "VisitRepository.insert"),
	}
	evidence[0].AnchorSymbol = "countOpenVisits"
	evidence[1].AnchorSymbol = "insert"
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence)
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("class-only endpoints with no exact message operation must remain unproven: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_NodeDisplayMetadataDoesNotChangeIdentity(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "flowchart TD\n" +
		`  A["Alpha.Run\ninternal/example.go:10"]` + "\n" +
		`  B["Beta.Run<br/>internal/example.go:20"]` + "\n" +
		"  A --> B\n"
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
	if len(got) != 0 {
		t.Fatalf("file/line display suffix must not become part of typed endpoint identity: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_CompactDisplayQualifierDoesNotChangeIdentityAcrossLanguages(t *testing.T) {
	tests := []struct {
		language string
		caller   string
		callee   string
	}{
		{"go", "service.Handle", "repo.Insert"},
		{"python", "service.handle", "repo.insert"},
		{"javascript", "Service.handle", "Repository.insert"},
		{"typescript", "Service.handle", "Repository.insert"},
		{"java", "VisitService.schedule", "VisitRepository.insert"},
		{"kotlin", "VisitService.schedule", "VisitRepository.insert"},
		{"rust", "py.tokenize_bytes", "tokenize_bytes"},
		{"c", "service_handle", "repo_insert"},
		{"cpp", "Service::handle", "Repository::insert"},
		{"ruby", "Service::handle", "Repository::insert"},
		{"swift", "Service.handle", "Repository.insert"},
		{"lua", "service.handle", "repo.insert"},
		{"arkts", "VisitService.schedule", "VisitRepository.insert"},
		{"cangjie", "clinic::VisitService::schedule", "clinic::VisitRepository::insert"},
	}
	for _, tc := range tests {
		t.Run(tc.language, func(t *testing.T) {
			doc := diagramEvidenceTestDoc("A", "B")
			doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
				"  participant A as " + tc.caller + " (caller)\n" +
				"  participant B as " + tc.callee + " (" + tc.language + ")\n" +
				"  A->>B: invoke\n"
			if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
				[]types.EvidenceItem{diagramEvidenceTestCall(tc.caller, tc.callee)}); len(got) != 0 {
				t.Fatalf("%s display qualifier must not change typed endpoint identity: %+v", tc.language, got)
			}
		})
	}
}

func TestDiagramEvidenceIdentityBeforeDisplayQualifierFailsClosedOnCallAndIdentityShapes(t *testing.T) {
	for _, label := range []string{
		"resolve(json)",
		"resolve (Other.Run)",
		"resolve (Rust core)",
		"business service (Rust)",
		"resolve (Rust",
	} {
		if identity, ok := diagramEvidenceIdentityBeforeDisplayQualifier(label); ok {
			t.Fatalf("non-qualifier shape %q unexpectedly projected to %q", label, identity)
		}
	}
	for label, want := range map[string]string{
		"resolve (Rust)":                "resolve",
		"Service::handle (C++)":         "Service::handle",
		"VisitService.schedule (ArkTS)": "VisitService.schedule",
	} {
		if identity, ok := diagramEvidenceIdentityBeforeDisplayQualifier(label); !ok || identity != want {
			t.Fatalf("display qualifier %q = (%q,%t), want %q", label, identity, ok, want)
		}
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SequenceParticipantSourceFileQualifiersStayDisplayOnly(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "sequence", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			"  participant main",
			"  participant run as run (main.rs)",
			"  participant walker as walker::collect_files",
			"  participant walk as walk (walker.rs)",
			"  participant index_file as index_file (main.rs)",
			"  participant matcher as Matcher (matcher.rs)",
			"  main->>run: run(&pattern, fixed)",
			"  run->>walker: collect_files",
			"  walker->>walk: walk(root, &mut out)",
			"  run->>index_file: index_file(f, m.as_ref())",
			"  index_file->>matcher: is_match (逐行)",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "main", ToNode: "run", RelationKind: types.DiagramRelCall},
			{FromNode: "run", ToNode: "walker", RelationKind: types.DiagramRelCall},
			{FromNode: "walker", ToNode: "walk", RelationKind: types.DiagramRelCall},
			{FromNode: "run", ToNode: "index_file", RelationKind: types.DiagramRelCall},
			{FromNode: "index_file", ToNode: "matcher", RelationKind: types.DiagramRelCall},
		},
	}}}
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("main", "run"),
		diagramEvidenceTestCall("run", "walker::collect_files"),
		diagramEvidenceTestCall("walker::collect_files", "walk"),
		diagramEvidenceTestCall("run", "index_file"),
		diagramEvidenceTestCall("index_file", "Matcher.is_match"),
	}
	evidence[0].AnchorSymbol = "run"
	evidence[1].AnchorSymbol = "walker::collect_files"
	evidence[2].AnchorSymbol = "walk"
	evidence[3].AnchorSymbol = "index_file"
	evidence[4].AnchorSymbol = "Matcher.is_match"
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence); len(got) != 0 {
		t.Fatalf("source-file display qualifiers must not erase exact typed call relations: %+v", got)
	}
}

func TestDiagramEvidenceIdentityBeforeSourceFileQualifierAcrossLanguages(t *testing.T) {
	for label, want := range map[string]string{
		"run (main.rs)":                         "run",
		"VisitService.schedule (Visit.java)":    "VisitService.schedule",
		"Service::handle (service.cpp)":         "Service::handle",
		"handle (src/service.c)":                "handle",
		"visit (src/pages/visit.ets)":           "visit",
		"clinic::schedule (src/clinic/main.cj)": "clinic::schedule",
	} {
		if identity, ok := diagramEvidenceIdentityBeforeSourceFileQualifier(label); !ok || identity != want {
			t.Fatalf("source-file qualifier %q = (%q,%t), want %q", label, identity, ok, want)
		}
	}
	for _, label := range []string{
		"resolve(json)",
		"resolve (Other.Run)",
		"resolve (Rust core)",
		"business service (main.rs)",
		"resolve (main.rs",
		"resolve (trace.txt)",
	} {
		if identity, ok := diagramEvidenceIdentityBeforeSourceFileQualifier(label); ok {
			t.Fatalf("non-source-qualifier shape %q unexpectedly projected to %q", label, identity)
		}
	}
}

func TestDiagramCallEdgeEvidenceMismatches_InlineCodeParticipantLabelsAcrossExecutableLanguages(t *testing.T) {
	tests := []struct {
		language string
		caller   string
		callee   string
	}{
		{"go", "service.Handle", "repo.Insert"},
		{"python", "service.handle", "repo.insert"},
		{"javascript", "Service.handle", "Repository.insert"},
		{"typescript", "Service.handle", "Repository.insert"},
		{"java", "VisitService.schedule", "VisitRepository.insert"},
		{"kotlin", "VisitService.schedule", "VisitRepository.insert"},
		{"rust", "service::handle", "repo::insert"},
		{"c", "service_handle", "repo_insert"},
		{"cpp", "Service::handle", "Repository::insert"},
		{"ruby", "Service::handle", "Repository::insert"},
		{"swift", "Service.handle", "Repository.insert"},
		{"lua", "service.handle", "repo.insert"},
		{"arkts", "VisitService.schedule", "VisitRepository.insert"},
		{"cangjie", "clinic::VisitService::schedule", "clinic::VisitRepository::insert"},
	}
	for _, tc := range tests {
		t.Run(tc.language, func(t *testing.T) {
			doc := diagramEvidenceTestDoc("A", "B")
			doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
				"  participant A as \"`" + tc.caller + "`\"\n" +
				"  participant B as \"`" + tc.callee + "`\"\n" +
				"  A->>B: invoke\n"
			doc.Blocks = append(doc.Blocks, types.AnswerBlock{
				ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
				FacetIDs:  []string{string(types.FacetPrincipalPathEdge)},
				ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
				Items:     []types.AnswerBlockItem{{Label: tc.caller}, {Label: tc.callee}},
			})
			if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
				[]types.EvidenceItem{diagramEvidenceTestCall(tc.caller, tc.callee)}); len(got) != 0 {
				t.Fatalf("%s exact inline-code labels must preserve typed endpoint identity: %+v", tc.language, got)
			}
			if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFGeneric},
				[]types.EvidenceItem{diagramEvidenceTestCall(tc.caller, tc.callee)}); len(got) != 0 {
				t.Fatalf("%s sibling explicit anchors must share inline-code identity normalization: %+v", tc.language, got)
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_CallDAGInlineCodeLabelsShareNormalizer(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "dag", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramCallDAG, Language: "mermaid", Body: "flowchart TD\n" +
			"  A[\"`Owner.run`\"] --> B[\"`Target.go`\"]\n"},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
		}},
	}}}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{diagramEvidenceTestCall("Owner.run", "Target.go")}); len(got) != 0 {
		t.Fatalf("call-DAG declarations must share exact inline-code identity normalization: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_InlineCodeLabelsRemainDirectional(t *testing.T) {
	doc := diagramEvidenceTestDoc("B", "A")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as \"`Alpha.Run`\"\n" +
		"  participant B as \"`Beta.Run`\"\n" +
		"  B->>A: invoke\n"
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
	if len(got) != 1 || got[0].FromSymbol != "Beta.Run" || got[0].ToSymbol != "Alpha.Run" {
		t.Fatalf("presentation normalization must not authorize the reverse call direction: %+v", got)
	}
}

func TestDiagramEvidenceLabelSymbol_OnlyStripsExactInlineCodeIdentityWrapper(t *testing.T) {
	for _, identity := range []string{"free_function", "Owner.method", "ns::Type::run", "Type#run"} {
		if got := diagramEvidenceLabelSymbol("`" + identity + "`"); got != identity {
			t.Fatalf("exact wrapper for %q normalized to %q", identity, got)
		}
	}
	for _, malformed := range []string{
		"`Alpha.Run", "Alpha.Run`", "`Alpha.Run` suffix", "`Alpha Run`", "``Alpha.Run``", "`Alpha.Run` `Beta.Run`",
	} {
		if got := diagramEvidenceLabelSymbol(malformed); got != malformed {
			t.Fatalf("malformed/prose wrapper %q must remain fail-closed, got %q", malformed, got)
		}
	}
}

func TestDiagramCallEdgeEvidenceMismatches_DisplayWrappedExactEndpointsPreserveTypedRelation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		caller string
		callee string
		from   string
		to     string
	}{
		{name: "dot", caller: "explorerEvaluator.ParseOutput", callee: "ctx.Mutable.SetTurnAArtifacts", from: `explorerEvaluator\n.ParseOutput`, to: `ctx.Mutable\n.SetTurnAArtifacts`},
		{name: "scope", caller: "clinic::VisitService::schedule", callee: "clinic::VisitRepository::insert", from: `clinic::VisitService\n::schedule`, to: `clinic::VisitRepository\n::insert`},
		{name: "pointer", caller: "service->handle", callee: "repo->insert", from: `service\n->handle`, to: `repo\n->insert`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
				ID: "wrapped", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart TD\n" +
					`  A["` + tc.from + `"] --> B["` + tc.to + `"]` + "\n"},
				EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall}},
			}}}
			if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFGeneric},
				[]types.EvidenceItem{diagramEvidenceTestCall(tc.caller, tc.callee)}); len(got) != 0 {
				t.Fatalf("presentation-only member wrapping must retain the exact typed relation: %+v", got)
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_ArbitraryMultilineLabelCannotMintEndpoint(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "business-lines", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart TD\n" +
			`  A["explorerEvaluator\nParseOutput"] --> B["ctx.Mutable\nSetTurnAArtifacts"]` + "\n"},
		EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall}},
	}}}
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFGeneric},
		[]types.EvidenceItem{diagramEvidenceTestCall("explorerEvaluator.ParseOutput", "ctx.Mutable.SetTurnAArtifacts")})
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("arbitrary multiline prose must not be concatenated into typed endpoint authority: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_InlineCodeDuplicateIdentityIsStillDiagnosed(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "sequence", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			"  participant A as Caller.one",
			"  participant B as Caller.two",
			"  participant R as \"`gate.RunWith`\"",
			"  participant R2 as \"`gate.RunWith`\"",
			"  A->>R: invoke",
			"  B->>R2: invoke",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "A", ToNode: "R", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
			{FromNode: "B", ToNode: "R2", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
		},
	}}}
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, []types.EvidenceItem{
		diagramEvidenceTestCall("Caller.one", "gate.RunWith"),
		diagramEvidenceTestCall("Caller.two", "gate.RunWith"),
	})
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueDuplicateParticipant || got[0].FromSymbol != "gate.RunWith" {
		t.Fatalf("duplicate identity lane must consume the same normalized inline-code label: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_QualifiedAndShortTypedCalleeCannotSplitIntoTwoParticipants(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "sequence", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			"  participant B as buildAnalysisIR",
			"  participant G as gate.Run",
			"  participant RW as gate.RunWith",
			"  participant RW2 as RunWith",
			"  B->>RW: invoke",
			"  G->>RW2: invoke",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "B", ToNode: "RW", RelationKind: types.DiagramRelCall},
			{FromNode: "G", ToNode: "RW2", RelationKind: types.DiagramRelCall},
		},
	}}}
	first := diagramEvidenceTestCall("buildAnalysisIR", "gate.RunWith")
	first.AnchorSymbol = "RunWith"
	second := diagramEvidenceTestCall("gate.Run", "gate.RunWith")
	second.AnchorSymbol = "RunWith"
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, []types.EvidenceItem{first, second})
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueDuplicateParticipant ||
		got[0].FromNode != "RW" || got[0].ToNode != "RW2" || got[0].FromSymbol != "gate.RunWith" {
		t.Fatalf("one typed callee's qualified and parser-short surfaces must remain one participant identity: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_AmbiguousShortCalleeAliasDoesNotChooseOwner(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "sequence", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			"  participant A as Caller.one",
			"  participant B as Caller.two",
			"  participant R as pkg.one.RunWith",
			"  participant R2 as RunWith",
			"  A->>R: invoke",
			"  B->>R2: invoke",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "A", ToNode: "R", RelationKind: types.DiagramRelCall},
			{FromNode: "B", ToNode: "R2", RelationKind: types.DiagramRelCall},
		},
	}}}
	one := diagramEvidenceTestCall("Caller.one", "pkg.one.RunWith")
	one.AnchorSymbol = "RunWith"
	two := diagramEvidenceTestCall("Caller.two", "pkg.two.RunWith")
	two.AnchorSymbol = "RunWith"
	if got := diagramDuplicateTypedParticipantIdentities(doc, []types.EvidenceItem{one, two}, true); len(got) != 0 {
		t.Fatalf("one short alias shared by multiple typed owners must remain unresolved, not choose a duplicate family: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SameLineSupportRefDoesNotChangeIdentity(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as Alpha.Run (internal/example.go:10)\n" +
		"  participant B as Beta.Run (internal/example.go:20)\n" +
		"  A->>B: invoke\n"
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
	if len(got) != 0 {
		t.Fatalf("typed support-ref suffix must not become part of endpoint identity: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_ExactCalleeAnchorSymbolAuthorizesShortLabel(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as Alpha.Run\n" +
		"  participant B as Run\n" +
		"  A->>B: invoke\n"
	evidence := diagramEvidenceTestCall("Alpha.Run", "normalizer.Run")
	evidence.AnchorSymbol = "Run"
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{evidence}); len(got) != 0 {
		t.Fatalf("exact grounded callee AnchorSymbol is an authoritative display alias for Object: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_RequiredQualifiedCallerBridgesUniqueShortCall(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as gate.Run\n" +
		"  participant B as gate.RunWith\n" +
		"  A->>B: calls RunWith\n"
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID: "hops", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
		FacetIDs: []string{string(types.FacetPrincipalPathEdge)},
		ClaimUses: []types.RenderedClaimUse{{
			ClaimForm: types.ClaimCallEdge, FacetID: string(types.FacetPrincipalPathEdge),
		}},
		Items: []types.AnswerBlockItem{{ID: "h1", Label: "gate.Run"}, {ID: "h2", Label: "gate.RunWith"}},
	})
	shortCall := diagramEvidenceTestCall("Run", "RunWith")
	shortCall.Source = "internal/analysis/gate/gate.go"
	shortCall.LineStart = 135
	qualifiedCallee := diagramEvidenceTestCall("buildAnalysisIR", "gate.RunWith")
	qualifiedCallee.Source = "internal/agent/analyzer.go"
	qualifiedCallee.LineStart = 2666
	callerDefinition := diagramEvidenceTestDefinition("gate", "Run", "internal/analysis/gate/gate.go", 100)
	view := &types.AnswerSemanticView{
		Family: types.QFCallChain,
		RequiredMechanismAnchors: []types.AnswerRequiredAnchor{{
			Text: "gate.Run", Kind: types.ContractTermSymbol,
		}},
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{shortCall, qualifiedCallee}); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("a request anchor without citable caller-owner binding must not mint qualification: %+v", got)
	}
	ownerBoundCall := shortCall
	ownerBoundCall.OwnerSymbol = "gate.Run"
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{ownerBoundCall}); len(got) != 0 {
		t.Fatalf("a grounded exact OwnerSymbol should bind an unqualified same-owner call without a duplicate definition row: %+v", got)
	}
	wrongOwnerCall := ownerBoundCall
	wrongOwnerCall.OwnerSymbol = "other.Run"
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{wrongOwnerCall}); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("a differently qualified OwnerSymbol must fail closed: %+v", got)
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{shortCall, qualifiedCallee, callerDefinition}); len(got) != 0 {
		t.Fatalf("typed required caller plus unique short call and exact callee should preserve qualification: %+v", got)
	}
	wrongOwnerFileCall := shortCall
	wrongOwnerFileCall.Source = "other/gate.go"
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{wrongOwnerFileCall, qualifiedCallee, callerDefinition}); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("a short call from outside the caller definition file must not inherit its owner: %+v", got)
	}

	view.RequiredMechanismAnchors = nil
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{shortCall, qualifiedCallee, callerDefinition}); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("model-authored qualification must fail without typed caller authority: %+v", got)
	}

	view.RequiredMechanismAnchors = []types.AnswerRequiredAnchor{{Text: "gate.Run", Kind: types.ContractTermSymbol}}
	ambiguous := shortCall
	ambiguous.LineStart = 42
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{shortCall, ambiguous, qualifiedCallee, callerDefinition}); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("multiple short call-site locations must remain ambiguous: %+v", got)
	}

	ambiguousDefinition := callerDefinition
	ambiguousDefinition.Source = "other/gate.go"
	ambiguousDefinition.LineStart = 20
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{shortCall, qualifiedCallee, callerDefinition, ambiguousDefinition}); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("multiple caller-owner definitions must remain ambiguous: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_GroundedOwnerBindingSupportsClosedQualifierForms(t *testing.T) {
	tests := []struct {
		name, caller, callee string
	}{
		{"dot", "gate.Run", "gate.RunWith"},
		{"colon", "clinic::Gate::run", "clinic::Gate::runWith"},
		{"hash", "Gate#run", "Gate#runWith"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc := diagramEvidenceTestDoc("A", "B")
			doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
				"  participant A as " + tc.caller + "\n" +
				"  participant B as " + tc.callee + "\n" +
				"  A->>B: invoke\n"
			call := diagramEvidenceTestCall(diagramEvidenceQualifiedOperation(tc.caller), diagramEvidenceQualifiedOperation(tc.callee))
			call.AnchorSymbol = diagramEvidenceQualifiedOperation(tc.callee)
			call.OwnerSymbol = tc.caller
			view := &types.AnswerSemanticView{
				Family: types.QFCallChain,
				RequiredMechanismAnchors: []types.AnswerRequiredAnchor{{
					Text: tc.caller, Kind: types.ContractTermSymbol,
				}},
			}
			if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{call}); len(got) != 0 {
				t.Fatalf("grounded owner-bound %s edge should pass all consumers: %+v", tc.name, got)
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_GroundedOwnerBindingSupportsUnqualifiedCalleeAcrossFamilies(t *testing.T) {
	tests := []struct {
		name, caller, callee string
	}{
		{"dot", "gate.Run", "RunWith"},
		{"colon", "clinic::Gate::run", "runWith"},
		{"hash", "Gate#run", "runWith"},
	}
	for _, family := range []types.QuestionFamily{types.QFCallChain, types.QFGeneric, types.QFArchitecture} {
		for _, tc := range tests {
			t.Run(string(family)+"/"+tc.name, func(t *testing.T) {
				doc := diagramEvidenceTestDoc("A", "B")
				doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
					"  participant A as " + tc.caller + "\n" +
					"  participant B as " + tc.callee + "\n" +
					"  A->>B: invoke\n"
				call := diagramEvidenceTestCall(diagramEvidenceQualifiedOperation(tc.caller), tc.callee)
				call.AnchorSymbol = tc.callee
				call.OwnerSymbol = tc.caller
				if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: family}, []types.EvidenceItem{call}); len(got) != 0 {
					t.Fatalf("parser-owned %s caller plus exact short callee should pass %s: %+v", tc.name, family, got)
				}
			})
		}
	}
}

func TestDiagramCallEdgeEvidenceMismatches_QualifiedDisplayCallerBridgesThroughTypedInboundEndpoint(t *testing.T) {
	for _, qualified := range []string{
		"walker::collect_files",
		"walker.collect_files",
		"Walker#collect_files",
		"walker/collect_files",
		"walker->collect_files",
	} {
		t.Run(qualified, func(t *testing.T) {
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
				ID: "rust-call", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{
					Kind: types.DiagramCallDAG, Language: "mermaid",
					Body: "flowchart TD\n  wc[\"" + qualified + "\"] -->|calls| wf[\"walk\"]\n",
				},
				EdgeAnchors: []types.DiagramEdgeAnchor{{
					FromNode: "wc", ToNode: "wf", RelationKind: types.DiagramRelCall,
					ClaimForm: types.ClaimCallEdge,
				}},
			}}}
			inbound := diagramEvidenceTestCall("run", qualified)
			inbound.Source = "src/main.rs"
			inbound.LineStart = 20
			inner := diagramEvidenceTestCall("collect_files", "walk")
			inner.Source = "src/walker.rs"
			inner.LineStart = 6
			inner.OwnerSymbol = "collect_files"
			inner.AnchorSymbol = "walk"
			definition := diagramEvidenceTestDefinition("", "collect_files", "src/walker.rs", 4)
			if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
				[]types.EvidenceItem{inbound, inner, definition}); len(got) != 0 {
				t.Fatalf("typed inbound qualification plus unique source-local definition must preserve display identity: %+v", got)
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_QualifiedDisplayCallerBridgeFailsClosedOnAmbiguity(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "rust-call", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramCallDAG, Language: "mermaid",
			Body: "flowchart TD\n  wc[\"walker::collect_files\"] -->|calls| wf[\"walk\"]\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "wc", ToNode: "wf", RelationKind: types.DiagramRelCall,
			ClaimForm: types.ClaimCallEdge,
		}},
	}}}
	inbound := diagramEvidenceTestCall("run", "walker::collect_files")
	inbound.Source = "src/main.rs"
	inbound.LineStart = 20
	inner := diagramEvidenceTestCall("collect_files", "walk")
	inner.Source = "src/walker.rs"
	inner.LineStart = 6
	inner.OwnerSymbol = "collect_files"
	inner.AnchorSymbol = "walk"
	definition := diagramEvidenceTestDefinition("", "collect_files", "src/walker.rs", 4)
	view := &types.AnswerSemanticView{Family: types.QFCallChain}

	tests := []struct {
		name     string
		evidence []types.EvidenceItem
	}{
		{name: "no exact qualified inbound endpoint", evidence: []types.EvidenceItem{inner, definition}},
		{name: "wrong qualified inbound endpoint", evidence: []types.EvidenceItem{
			diagramEvidenceTestCall("run", "other::collect_files"), inner, definition,
		}},
		{name: "duplicate short definition", evidence: []types.EvidenceItem{
			inbound, inner, definition, diagramEvidenceTestDefinition("", "collect_files", "other/walker.rs", 4),
		}},
		{name: "same short caller in another source", evidence: []types.EvidenceItem{
			inbound, inner, definition, func() types.EvidenceItem {
				other := inner
				other.ID = "other-inner"
				other.Source = "other/walker.rs"
				return other
			}(),
		}},
		{name: "conflicting parser owner", evidence: []types.EvidenceItem{
			inbound, func() types.EvidenceItem {
				other := inner
				other.OwnerSymbol = "other::collect_files"
				return other
			}(), definition,
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DiagramCallEdgeEvidenceMismatches(doc, view, tc.evidence)
			if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
				t.Fatalf("ambiguous display qualification must fail closed: %+v", got)
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_GroundedOwnerShortCalleeFailsClosedOnIdentityConflict(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as gate.Run\n" +
		"  participant B as RunWith\n" +
		"  A->>B: invoke\n"
	base := diagramEvidenceTestCall("Run", "RunWith")
	base.AnchorSymbol = "RunWith"
	base.OwnerSymbol = "gate.Run"
	for name, mutate := range map[string]func(*types.EvidenceItem, *types.AnswerDocumentV2, *[]types.EvidenceItem){
		"missing owner": func(call *types.EvidenceItem, _ *types.AnswerDocumentV2, _ *[]types.EvidenceItem) {
			call.OwnerSymbol = ""
		},
		"wrong owner": func(call *types.EvidenceItem, _ *types.AnswerDocumentV2, _ *[]types.EvidenceItem) {
			call.OwnerSymbol = "other.Run"
		},
		"different qualified target": func(_ *types.EvidenceItem, candidate *types.AnswerDocumentV2, _ *[]types.EvidenceItem) {
			candidate.Blocks[0].Diagram.Body = strings.ReplaceAll(candidate.Blocks[0].Diagram.Body, "participant B as RunWith", "participant B as other.RunWith")
		},
		"ambiguous call sites": func(call *types.EvidenceItem, _ *types.AnswerDocumentV2, evidence *[]types.EvidenceItem) {
			other := *call
			other.LineStart++
			*evidence = append(*evidence, other)
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := *doc
			candidate.Blocks = append([]types.AnswerBlock(nil), doc.Blocks...)
			candidate.Blocks[0] = doc.Blocks[0]
			body := *doc.Blocks[0].Diagram
			candidate.Blocks[0].Diagram = &body
			call := base
			evidence := []types.EvidenceItem{call}
			mutate(&call, &candidate, &evidence)
			evidence[0] = call
			got := DiagramCallEdgeEvidenceMismatches(&candidate, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence)
			if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
				t.Fatalf("conflicting parser-owned identity must fail closed: %+v", got)
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_GroundedOwnerShortCalleeSatisfiesPrincipalCompleteness(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as gate.Run\n" +
		"  participant B as RunWith\n" +
		"  A->>B: invoke\n"
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID: "principal", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
		FacetIDs: []string{string(types.FacetPrincipalPathEdge)},
		ClaimUses: []types.RenderedClaimUse{{
			ClaimForm: types.ClaimCallEdge, FacetID: string(types.FacetPrincipalPathEdge),
		}},
		Items: []types.AnswerBlockItem{{ID: "caller", Label: "Run"}, {ID: "callee", Label: "RunWith"}},
	})
	call := diagramEvidenceTestCall("Run", "RunWith")
	call.AnchorSymbol = "RunWith"
	call.OwnerSymbol = "gate.Run"
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, []types.EvidenceItem{call}); len(got) != 0 {
		t.Fatalf("one parser-owned edge must satisfy both visible and principal call identity: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_QualifiedCalleeResolvesFromUniqueTypedDefinition(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as VisitController.create\n" +
		"  participant B as VisitService.schedule\n" +
		"  A->>B: schedule(petId, reason)\n"
	call := diagramEvidenceTestCall("VisitController.create", "schedule")
	call.AnchorSymbol = "schedule"
	evidence := []types.EvidenceItem{
		call,
		diagramEvidenceTestDefinition("VisitService", "schedule", "VisitService.java", 16),
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence); len(got) != 0 {
		t.Fatalf("a short typed callee plus one exact citable owner definition should resolve the qualified endpoint: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_QualifiedCalleeWithoutDefinitionFailsClosed(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as VisitController.create\n" +
		"  participant B as VisitService.schedule\n" +
		"  A->>B: schedule(petId, reason)\n"
	call := diagramEvidenceTestCall("VisitController.create", "schedule")
	call.AnchorSymbol = "schedule"
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, []types.EvidenceItem{call})
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("a bare operation must not invent its qualified owner without definition evidence: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_QualifiedCalleeWrongOwnerFailsClosed(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as VisitController.create\n" +
		"  participant B as VisitService.schedule\n" +
		"  A->>B: schedule(petId, reason)\n"
	call := diagramEvidenceTestCall("VisitController.create", "schedule")
	call.AnchorSymbol = "schedule"
	evidence := []types.EvidenceItem{
		call,
		diagramEvidenceTestDefinition("OtherService", "schedule", "OtherService.java", 16),
	}
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence)
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("a definition for a different owner must not authorize the endpoint: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_QualifiedCalleeOverloadFailsClosed(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as VisitController.create\n" +
		"  participant B as VisitService.schedule\n" +
		"  A->>B: schedule(petId, reason)\n"
	call := diagramEvidenceTestCall("VisitController.create", "schedule")
	call.AnchorSymbol = "schedule"
	evidence := []types.EvidenceItem{
		call,
		diagramEvidenceTestDefinition("VisitService", "schedule", "VisitService.java", 16),
		diagramEvidenceTestDefinition("VisitService", "schedule", "VisitService.java", 24),
	}
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence)
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("multiple distinct definitions of the same owner operation must remain ambiguous: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_QualifiedCalleeRejectsContraryQualifiedObject(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as VisitController.create\n" +
		"  participant B as VisitService.schedule\n" +
		"  A->>B: schedule(petId, reason)\n"
	call := diagramEvidenceTestCall("VisitController.create", "OtherService.schedule")
	call.AnchorSymbol = "schedule"
	evidence := []types.EvidenceItem{
		call,
		diagramEvidenceTestDefinition("VisitService", "schedule", "VisitService.java", 16),
	}
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence)
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("a conflicting qualified Object must not be laundered through a short AnchorSymbol: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_QualifiedCalleeRejectsContraryBareObject(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as VisitController.create\n" +
		"  participant B as VisitService.schedule\n" +
		"  A->>B: schedule(petId, reason)\n"
	call := diagramEvidenceTestCall("VisitController.create", "enqueue")
	call.AnchorSymbol = "schedule"
	evidence := []types.EvidenceItem{
		call,
		diagramEvidenceTestDefinition("VisitService", "schedule", "VisitService.java", 16),
	}
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence)
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("a conflicting bare Object must not be overridden by AnchorSymbol: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_UnrelatedShortLabelDoesNotMatchCallee(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as Alpha.Run\n" +
		"  participant B as Other\n" +
		"  A->>B: invoke\n"
	evidence := diagramEvidenceTestCall("Alpha.Run", "normalizer.Run")
	evidence.AnchorSymbol = "Run"
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{evidence}); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("only exact Object/AnchorSymbol surfaces may authorize the destination: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_BodyEdgeCannotOmitTypedAnchor(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].EdgeAnchors = nil
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueMissingGroundedAnchor || got[0].FromSymbol != "Alpha.Run" || got[0].ToSymbol != "Beta.Run" {
		t.Fatalf("a parsed sequence edge without typed metadata must hard-fail: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_CallDAGBodyEdgeCannotOmitTypedAnchor(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Kind = types.DiagramCallDAG
	doc.Blocks[0].Diagram.Body = "flowchart TD\n  A[Alpha.Run] --> B[Beta.Run]\n"
	doc.Blocks[0].EdgeAnchors = nil
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueMissingGroundedAnchor {
		t.Fatalf("a parsed call_dag edge without typed metadata must hard-fail: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SemanticCallDAGIsStrictAcrossQuestionFamilies(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Kind = types.DiagramCallDAG
	doc.Blocks[0].Diagram.Body = "flowchart TD\n  A[Alpha.Run] --> B[Beta.Run]\n"
	doc.Blocks[0].EdgeAnchors = nil
	for _, family := range []types.QuestionFamily{types.QFGeneric, types.QFArchitecture, types.QFRoleLookup} {
		got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: family},
			[]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
		if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueMissingGroundedAnchor {
			t.Fatalf("semantic call_dag must retain exact body ownership in family %s: %+v", family, got)
		}
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SourceFlowAndArchitectureRequireTypedBodyOwnership(t *testing.T) {
	for _, kind := range []types.DiagramKind{types.DiagramFlow, types.DiagramArchitecture} {
		t.Run(string(kind), func(t *testing.T) {
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
				ID: "source-relations", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{
					Kind: kind, Language: "mermaid",
					Body: "flowchart TD\n  C[ConsoleSink] --> B[BaseSink]\n",
				},
			}}}
			got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, nil)
			if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueMissingRelationAnchor ||
				got[0].FromSymbol != "ConsoleSink" || got[0].ToSymbol != "BaseSink" {
				t.Fatalf("%s source diagram edge without typed relation owner must fail closed: %+v", kind, got)
			}

			// The same presentation in a generic answer is not automatically a
			// source-relation claim. The hard boundary is the typed question
			// family, never node labels or user/model prose.
			if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFGeneric}, nil); len(got) != 0 {
				t.Fatalf("%s generic presentation diagram must stay outside source body ownership gate: %+v", kind, got)
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SourceFlowRespectsTypedNonCallRelation(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "dispatch", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart TD\n  C[ConsoleSink] --> B[BaseSink]\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "C", ToNode: "B", RelationKind: types.DiagramRelTypeRelation,
		}},
	}}}
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceRelationship, Producer: types.EvidenceProducerRepoMapImplementerRelation,
		Subject: "ConsoleSink", Predicate: "implements", Object: "BaseSink",
		Source: "src/console.ext", LineStart: 4, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition,
	}}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence); len(got) != 0 {
		t.Fatalf("typed source flow relation must not be reinterpreted as a call: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SourceFlowSiblingCarrierOwnsBodyEdgeOnce(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "flow", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramFlow, Language: "mermaid",
				Body: "flowchart TD\n  F[Facade.run] --> S[BaseSink.write]\n",
			},
		},
		{
			ID: "edge-facts", Kind: types.BlockOrderedList,
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "F", ToNode: "S", RelationKind: types.DiagramRelCall,
			}},
		},
	}}
	call := diagramEvidenceTestCall("Facade.run", "BaseSink.write")
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, []types.EvidenceItem{call}); len(got) != 0 {
		t.Fatalf("a unique sibling carrier must own the exact source body edge without duplicate validation: %+v", got)
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, nil); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("missing call evidence should produce one body-edge diagnosis, not duplicate sibling diagnoses: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SiblingCarrierCannotOwnReusedAliasesAcrossDiagrams(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "first", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid",
				Body: "flowchart TD\n  A[Facade.run] --> B[BaseSink.write]\n"},
		},
		{
			ID: "second", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid",
				Body: "flowchart TD\n  A[Other.run] --> B[Other.write]\n"},
		},
		{
			ID: "ambiguous-carrier", Kind: types.BlockOrderedList,
			EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall}},
		},
	}}
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, []types.EvidenceItem{
		diagramEvidenceTestCall("Facade.run", "BaseSink.write"),
		diagramEvidenceTestCall("Other.run", "Other.write"),
	})
	if len(got) != 2 || got[0].Issue != diagramCallEdgeIssueMissingRelationAnchor || got[1].Issue != diagramCallEdgeIssueMissingRelationAnchor {
		t.Fatalf("one sibling A->B carrier must not authorize two unrelated diagram-local A->B identities: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_RuntimeTraceFlowStaysOnRuntimeAuthority(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "trace", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart TD\n  Frame --> Wakeup\n",
		},
		// Runtime causal projection has an independent typed authority. Even a
		// source-diagram metadata/body mismatch must not route it through the
		// source call-edge gate.
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "Host", ToNode: "Target", RelationKind: types.DiagramRelCall,
		}},
	}}}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFRootCauseTrace}, nil); len(got) != 0 {
		t.Fatalf("runtime trace diagram must retain its independent causal authority: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_NormalizerCannotMintGuardAuthority(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "fabricated", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramCallDAG, Language: "mermaid",
			Body: "flowchart TD\n  A[Alpha.Run] -->|guard| B[Beta.Run]\n"},
	}}}
	if fixed := normalizeDiagramEdgeAnchorMetadata(doc); fixed != 0 {
		t.Fatalf("label text must not be promoted into typed edge authority: fixed=%d anchors=%+v", fixed, doc.Blocks[0].EdgeAnchors)
	}
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, nil)
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueMissingAnchor {
		t.Fatalf("an unanchored label-shaped guard must fail closed after normalization: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_CallDAGAllowsTypedGuardEdges(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "mixed", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramCallDAG, Language: "mermaid", Body: strings.Join([]string{
			"flowchart TD",
			"  C[VisitController.create] --> S[VisitService.schedule]",
			"  S -->|countOpenVisits >= max| G[countOpenVisits]",
			"  S -->|pass| R[VisitRepository.insert]",
			"  R --> A[AuditLog.record]",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "C", ToNode: "S", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
			{FromNode: "S", ToNode: "G", RelationKind: types.DiagramRelGuard, ClaimForm: types.ClaimGuardCondition},
			{FromNode: "S", ToNode: "R", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
			{FromNode: "R", ToNode: "A", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
		},
	}}}
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("VisitController.create", "VisitService.schedule"),
		diagramEvidenceTestCall("VisitService.schedule", "VisitRepository.insert"),
		diagramEvidenceTestCall("VisitRepository.insert", "AuditLog.record"),
		{
			ID: "guard", Kind: types.EvidenceConditional, Scope: types.ScopeLine,
			Source: "VisitService.java", LineStart: 18, AnchorKind: types.AnchorCondition,
			AnchorSymbol: "countOpenVisits", Subject: "VisitService.schedule",
			Condition: "countOpenVisits >= max", GroundingStatus: types.GroundingGrounded,
		},
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence); len(got) != 0 {
		t.Fatalf("a typed guard edge in a mixed call DAG must not be reclassified as a function call: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_CallDAGTypedCallCannotHideBehindGuardAnchorAcrossLanguages(t *testing.T) {
	tests := []struct {
		language string
		caller   string
		callee   string
	}{
		{"go", "service.Handle", "repo.Count"},
		{"python", "service.handle", "repo.count"},
		{"javascript", "Service.handle", "Repository.count"},
		{"typescript", "Service.handle", "Repository.count"},
		{"java", "VisitService.schedule", "VisitRepository.countOpenVisits"},
		{"kotlin", "VisitService.schedule", "VisitRepository.countOpenVisits"},
		{"rust", "service::handle", "repo::count"},
		{"c", "service_handle", "repo_count"},
		{"cpp", "Service::handle", "Repository::count"},
		{"ruby", "Service::handle", "Repository::count"},
		{"swift", "Service.handle", "Repository.count"},
		{"lua", "service.handle", "repo.count"},
		{"arkts", "VisitService.schedule", "VisitRepository.countOpenVisits"},
		{"cangjie", "clinic::VisitService::schedule", "clinic::VisitRepository::countOpenVisits"},
	}
	for _, tc := range tests {
		t.Run(tc.language, func(t *testing.T) {
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
				ID: "compound-condition", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{
					Kind: types.DiagramCallDAG, Language: "mermaid",
					Body: "flowchart TD\n  A[\"" + tc.caller + "\"] --> B[\"" + tc.callee + "\"]\n",
				},
				EdgeAnchors: []types.DiagramEdgeAnchor{{
					FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelGuard,
					ClaimForm: types.ClaimGuardCondition,
				}},
			}}}
			got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
				[]types.EvidenceItem{diagramEvidenceTestCall(tc.caller, tc.callee)})
			if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueMissingGroundedAnchor {
				t.Fatalf("%s exact invocation must retain a typed call anchor even inside a compound guard: %+v", tc.language, got)
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_CallDAGAllowsCallAndGuardForSameCompoundEdge(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "compound-condition", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramCallDAG, Language: "mermaid",
			Body: "flowchart TD\n  S[VisitService.schedule] --> C[VisitRepository.countOpenVisits]\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "S", ToNode: "C", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
			{FromNode: "S", ToNode: "C", RelationKind: types.DiagramRelGuard, ClaimForm: types.ClaimGuardCondition},
		},
	}}}
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("VisitService.schedule", "VisitRepository.countOpenVisits"),
		{
			ID: "guard", Kind: types.EvidenceConditional, Scope: types.ScopeLine,
			Source: "VisitService.java", LineStart: 18, AnchorKind: types.AnchorCondition,
			Subject: "VisitService.schedule", Object: "VisitRepository.countOpenVisits",
			AnchorSymbol: "countOpenVisits", Condition: "countOpenVisits >= max",
			GroundingStatus: types.GroundingGrounded,
		},
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence); len(got) != 0 {
		t.Fatalf("a compound edge may preserve both its exact invocation and guard context: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_OptionalDiagramMayShowTypedPrincipalSubset(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
			FacetIDs:  []string{string(types.FacetPrincipalPathEdge)},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
			Items:     []types.AnswerBlockItem{{Label: "Service.handle"}, {Label: "Repository.count"}, {Label: "Repository.insert"}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramCallDAG, Language: "mermaid",
				Body: "flowchart TD\n  S[Service.handle] --> C[Repository.count]\n  I[Repository.insert]\n",
			},
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "S", ToNode: "C", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
			}},
		},
	}}
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("Service.handle", "Repository.count"),
		diagramEvidenceTestCall("Service.handle", "Repository.insert"),
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence); len(got) != 0 {
		t.Fatalf("an optional diagram may faithfully show one grounded principal edge while sibling prose carries another: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_CopyReadySequenceSubsetSurvivesSiblingPrincipalCall(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
			FacetIDs:  []string{string(types.FacetPrincipalPathEdge)},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
			Items: []types.AnswerBlockItem{
				{Label: "Logger::log", CitationRef: 0},
				{Label: "ConsoleSink::write", CitationRef: 1},
			},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramSequence, Language: "mermaid",
				Body: "sequenceDiagram\n  participant n1 as Logger.log\n  participant n2 as Sink.write\n  n1->>n2: call\n",
			},
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "n1", ToNode: "n2", RelationKind: types.DiagramRelCall,
			}},
		},
	}, Citations: []types.Citation{
		{File: "src/logger.cpp", Line: 36},
		{File: "include/logx/console_sink.hpp", Line: 10},
	}}
	first := diagramEvidenceTestCall("Logger.log", "Sink.write")
	first.Source, first.LineStart = "src/logger.cpp", 36
	second := diagramEvidenceTestCall("ConsoleSink.write", "std.fputs")
	second.Source, second.LineStart = "include/logx/console_sink.hpp", 10
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{first, second}); len(got) != 0 {
		t.Fatalf("the exact copy-ready typed subset must pass even when sibling prose cites another grounded call: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_NonPrincipalTypedCallDoesNotExpandDiagram(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
			FacetIDs:  []string{string(types.FacetPrincipalPathEdge)},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
			Items:     []types.AnswerBlockItem{{Label: "Service.handle"}, {Label: "Repository.count"}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramCallDAG, Language: "mermaid",
				Body: "flowchart TD\n  S[Service.handle] --> C[Repository.count]\n",
			},
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "S", ToNode: "C", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
			}},
		},
	}}
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("Service.handle", "Repository.count"),
		diagramEvidenceTestCall("Service.handle", "Background.refresh"),
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence); len(got) != 0 {
		t.Fatalf("supporting calls outside the model-selected principal path must not be forced into its diagram: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_OptionalDiagramSubsetAcrossExecutableLanguages(t *testing.T) {
	tests := []struct {
		language string
		caller   string
		callee   string
	}{
		{"go", "service.Handle", "repo.Insert"},
		{"python", "service.handle", "repo.insert"},
		{"javascript", "Service.handle", "Repository.insert"},
		{"typescript", "Service.handle", "Repository.insert"},
		{"java", "VisitService.schedule", "VisitRepository.insert"},
		{"kotlin", "VisitService.schedule", "VisitRepository.insert"},
		{"rust", "service::handle", "repo::insert"},
		{"c", "service_handle", "repo_insert"},
		{"cpp", "Service::handle", "Repository::insert"},
		{"ruby", "Service::handle", "Repository::insert"},
		{"swift", "Service.handle", "Repository.insert"},
		{"lua", "service.handle", "repo.insert"},
		{"arkts", "VisitService.schedule", "VisitRepository.insert"},
		{"cangjie", "clinic::VisitService::schedule", "clinic::VisitRepository::insert"},
	}
	covered := make(map[string]bool, len(tests))
	for _, tc := range tests {
		covered[tc.language] = true
	}
	for _, language := range rmtypes.SupportedReadLanguages() {
		if language == rmtypes.LangProto {
			continue // Proto is declarative and has no source invocation edge.
		}
		if !covered[language] {
			t.Fatalf("supported executable language %q has no optional call-diagram subset fixture", language)
		}
	}
	for _, tc := range tests {
		t.Run(tc.language, func(t *testing.T) {
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
				{
					ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
					FacetIDs:  []string{string(types.FacetPrincipalPathEdge)},
					ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
					// The sibling principal row is intentionally edge-shaped and
					// citation-backed. It must not force an optional node-only visual
					// to claim that it is an exhaustive graph.
					Items: []types.AnswerBlockItem{{
						Label: tc.caller + " → " + tc.callee, CitationRef: 0,
					}},
				},
				{
					ID: "diagram", Kind: types.BlockDiagram,
					Diagram: &types.AnswerDiagramBlock{
						Kind: types.DiagramCallDAG, Language: "mermaid",
						Body: "flowchart TD\n  A[\"" + tc.caller + "\"]\n  B[\"" + tc.callee + "\"]\n",
					},
				},
			}, Citations: []types.Citation{{File: "internal/example.go", Line: 10}}}
			got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
				[]types.EvidenceItem{diagramEvidenceTestCall(tc.caller, tc.callee)})
			if len(got) != 0 {
				t.Fatalf("%s sibling typed call must not create optional-diagram completeness pressure: %+v", tc.language, got)
			}

			// Metadata cannot claim a hidden relation that the model removed from
			// the visible Mermaid body. This is the inverse of completeness: a
			// node-only optional subset remains valid only when it carries no
			// diagram-local edge metadata.
			doc.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{{
				FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall,
			}}
			got = DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
				[]types.EvidenceItem{diagramEvidenceTestCall(tc.caller, tc.callee)})
			if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueAnchorWithoutBodyEdge {
				t.Fatalf("%s diagram-local metadata-only edge must fail closed: %+v", tc.language, got)
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_ProtoDeclarationIsNotInventedAsCall(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
			FacetIDs:  []string{string(types.FacetPrincipalPathEdge)},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
			Items:     []types.AnswerBlockItem{{Label: "VisitService → VisitRequest", CitationRef: 0}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramCallDAG, Language: "mermaid", Body: "flowchart TD\n  S[VisitService]\n  R[VisitRequest]\n"},
		},
	}, Citations: []types.Citation{{File: "api/visit.proto", Line: 10}}}
	declaration := diagramEvidenceTestDefinition("VisitService", "VisitRequest", "api/visit.proto", 10)
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{declaration}); len(got) != 0 {
		t.Fatalf("the declarative Proto language must stay covered without inventing an executable call edge: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_PrincipalCitationCallAlreadyVisible(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
			FacetIDs:  []string{string(types.FacetPrincipalPathEdge)},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
			Items: []types.AnswerBlockItem{{
				Label: "VisitController.create → VisitService.schedule", CitationRef: 0,
			}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramSequence, Language: "mermaid",
				Body: "sequenceDiagram\n  participant C as VisitController\n  participant S as VisitService\n  C->>S: schedule(petId)\n",
			},
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "C", ToNode: "S", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
			}},
		},
	}, Citations: []types.Citation{{File: "internal/example.go", Line: 10}}}
	evidence := diagramEvidenceTestCall("VisitController.create", "VisitService.schedule")
	evidence.AnchorSymbol = "schedule"
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{evidence}); len(got) != 0 {
		t.Fatalf("a class-level presentation with exact operation must cover the citation-selected call: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_PrincipalMethodItemsShareClassParticipantResolver(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
			FacetIDs:  []string{string(types.FacetPrincipalPathEdge)},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
			Items:     []types.AnswerBlockItem{{Label: "VisitController.create"}, {Label: "VisitService.schedule"}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
				"sequenceDiagram",
				"  participant C as VisitController",
				"  participant S as VisitService",
				"  C->>S: schedule(petId)",
			}, "\n")},
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "C", ToNode: "S", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
			}},
		},
	}}
	evidence := diagramEvidenceTestCall("VisitController.create", "VisitService.schedule")
	evidence.AnchorSymbol = "schedule"
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{evidence}); len(got) != 0 {
		t.Fatalf("principal completeness must reuse the class-participant typed resolver: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_AmbiguousPrincipalCitationFailsOpen(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
			FacetIDs:  []string{string(types.FacetPrincipalPathEdge)},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
			Items:     []types.AnswerBlockItem{{Label: "selected call", CitationRef: 0}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramCallDAG, Language: "mermaid", Body: "flowchart TD\n  S[Service.handle]\n"},
		},
	}, Citations: []types.Citation{{File: "internal/example.go", Line: 10}}}
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("Service.handle", "Repository.count"),
		diagramEvidenceTestCall("Service.handle", "Repository.insert"),
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence); len(got) != 0 {
		t.Fatalf("one citation resolving to distinct call directions must not let the system guess a required edge: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_PrincipalCitationToNonCallDoesNotForceEdge(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
			FacetIDs:  []string{string(types.FacetPrincipalPathEdge)},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
			Items:     []types.AnswerBlockItem{{Label: "selected row", CitationRef: 0}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramCallDAG, Language: "mermaid", Body: "flowchart TD\n  S[Service.handle]\n"},
		},
	}, Citations: []types.Citation{{File: "internal/example.go", Line: 10}}}
	evidence := diagramEvidenceTestDefinition("Service", "handle", "internal/example.go", 10)
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{evidence}); len(got) != 0 {
		t.Fatalf("a definition citation must not be promoted to a call edge: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SupportingCitationDoesNotExpandPrincipalDiagram(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "support", Kind: types.BlockOrderedList,
			FacetIDs:  []string{string(types.FacetPrincipalPathEdge)},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
			Items:     []types.AnswerBlockItem{{Label: "Background.refresh → Cache.load", CitationRef: 0}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramCallDAG, Language: "mermaid", Body: "flowchart TD\n  S[Service.handle]\n"},
		},
	}, Citations: []types.Citation{{File: "internal/example.go", Line: 10}}}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{diagramEvidenceTestCall("Background.refresh", "Cache.load")}); len(got) != 0 {
		t.Fatalf("supporting citation-selected calls must not expand the principal diagram: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_CallDAGRejectsUnprovedLogicalRelationRelabels(t *testing.T) {
	for _, relation := range []types.DiagramRelationKind{
		types.DiagramRelGuard,
		types.DiagramRelControlFlow,
		types.DiagramRelImport,
		types.DiagramRelPrecedence,
		types.DiagramRelContain,
		types.DiagramRelObserve,
	} {
		t.Run(string(relation), func(t *testing.T) {
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
				ID: "typed-non-call", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{
					Kind: types.DiagramCallDAG, Language: "mermaid",
					Body: "flowchart TD\n  A[Source] --> B[Target]\n",
				},
				EdgeAnchors: []types.DiagramEdgeAnchor{{
					FromNode: "A", ToNode: "B", RelationKind: relation,
					ClaimForm: types.ClaimFormForRelation(relation),
				}},
			}}}
			got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, nil)
			if len(got) != 1 || got[0].Issue != diagramSemanticRelationIssueNoEvidence || got[0].Relation != relation {
				t.Fatalf("unproved typed %s edge must not bypass strict source authority: %+v", relation, got)
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_CallDAGAcceptsSameDirectionTypedLogicalRelations(t *testing.T) {
	tests := []struct {
		relation types.DiagramRelationKind
		evidence types.EvidenceItem
	}{
		{types.DiagramRelGuard, types.EvidenceItem{Kind: types.EvidenceConditional, AnchorKind: types.AnchorCondition, Subject: "Service.run", OwnerSymbol: "Service.run", AnchorSymbol: "enabled", Source: "service.go", LineStart: 10, GroundingStatus: types.GroundingGrounded}},
		{types.DiagramRelControlFlow, types.EvidenceItem{
			Kind: types.EvidenceControlFlow, AnchorKind: types.AnchorCall,
			Subject: "if enabled", Predicate: types.ControlFlowPredicateConsequence,
			Object: "Worker.run()", AnchorSymbol: "Worker.run", OwnerSymbol: "Service.run",
			Source: "service.go", LineStart: 10, LineEnd: 12, Scope: types.ScopeLineRange,
			Producer: types.EvidenceProducerDataflowLowererPrefix + "go", GroundingStatus: types.GroundingGrounded,
		}},
		{types.DiagramRelImport, types.EvidenceItem{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorImport, Subject: "service", Object: "storage", AnchorSymbol: "storage", Source: "service.go", LineStart: 3, GroundingStatus: types.GroundingGrounded}},
		{types.DiagramRelPrecedence, types.EvidenceItem{Kind: types.EvidenceDirect, DiagramRole: types.EvidenceDiagramRoleOverride, Subject: "cli", Object: "config", AnchorSymbol: "config", Source: "config.go", LineStart: 12, GroundingStatus: types.GroundingGrounded}},
		{types.DiagramRelObserve, types.EvidenceItem{Kind: types.EvidenceDirect, Origin: types.ClaimOriginLog, Subject: "worker", Object: "timeout", AnchorSymbol: "timeout", Source: "runtime.log", LineStart: 1, GroundingStatus: types.GroundingGrounded}},
	}
	for _, tc := range tests {
		t.Run(string(tc.relation), func(t *testing.T) {
			from, to := tc.evidence.Subject, tc.evidence.Object
			if tc.relation == types.DiagramRelGuard {
				to = tc.evidence.AnchorSymbol
			}
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
				ID: "typed-logical", Kind: types.BlockDiagram,
				Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramCallDAG, Language: "mermaid", Body: "flowchart TD\n  A[\"" + from + "\"] --> B[\"" + to + "\"]\n"},
				EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", RelationKind: tc.relation}},
			}}}
			if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, []types.EvidenceItem{tc.evidence}); len(got) != 0 {
				t.Fatalf("same-direction typed %s evidence must authorize its exact edge: %+v", tc.relation, got)
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_ControlFlowRejectsUnaryGuardAndReverseDirection(t *testing.T) {
	branch := types.EvidenceItem{
		Kind: types.EvidenceControlFlow, AnchorKind: types.AnchorCall,
		Subject: "if enabled", Predicate: types.ControlFlowPredicateConsequence,
		Object: "Worker.run()", AnchorSymbol: "Worker.run", OwnerSymbol: "Service.run",
		Source: "service.go", LineStart: 10, LineEnd: 12, Scope: types.ScopeLineRange,
		Producer: types.EvidenceProducerDataflowLowererPrefix + "go", GroundingStatus: types.GroundingGrounded,
	}
	makeDoc := func(from, to string) *types.AnswerDocumentV2 {
		return &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
			ID: "branch", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid",
				Body: "flowchart TD\n  A[\"" + from + "\"] --> B[\"" + to + "\"]\n"},
			EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelControlFlow}},
		}}}
	}
	view := &types.AnswerSemanticView{Family: types.QFGeneric, RelationAxis: types.AxisFlow}
	if got := DiagramCallEdgeEvidenceMismatches(makeDoc(branch.Subject, branch.Object), view, []types.EvidenceItem{branch}); len(got) != 0 {
		t.Fatalf("exact parser-proved branch effect must authorize its direction: %+v", got)
	}
	if got := DiagramCallEdgeEvidenceMismatches(makeDoc(branch.Object, branch.Subject), view, []types.EvidenceItem{branch}); len(got) != 1 || got[0].Issue != diagramSemanticRelationIssueNoEvidence {
		t.Fatalf("reverse branch effect must fail closed: %+v", got)
	}
	guardOnly := types.EvidenceItem{
		Kind: types.EvidenceConditional, AnchorKind: types.AnchorCondition,
		Subject: "Service.run", OwnerSymbol: "Service.run", AnchorSymbol: "enabled",
		Source: "service.go", LineStart: 10, Scope: types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
	}
	if got := DiagramCallEdgeEvidenceMismatches(makeDoc(branch.Subject, branch.Object), view, []types.EvidenceItem{guardOnly}); len(got) != 1 || got[0].Issue != diagramSemanticRelationIssueNoEvidence {
		t.Fatalf("unary guard must not be promoted into branch-to-effect authority: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_ChainedFlowEdgesRetainTypedOwnership(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "pipeline", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart TD\n  A[StageAnalyze] --> E[StageExplore] --> X[StageExtract] --> F[StageFinalize]\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "A", ToNode: "E", RelationKind: types.DiagramRelPrecedence},
			{FromNode: "E", ToNode: "X", RelationKind: types.DiagramRelPrecedence},
			{FromNode: "X", ToNode: "F", RelationKind: types.DiagramRelPrecedence},
		},
	}}}
	evidence := []types.EvidenceItem{
		{Kind: types.EvidenceDirect, DiagramRole: types.EvidenceDiagramRoleOverride, Subject: "StageAnalyze", Object: "StageExplore", AnchorSymbol: "StageExplore", Source: "internal/types/enums.go", LineStart: 33, GroundingStatus: types.GroundingGrounded},
		{Kind: types.EvidenceDirect, DiagramRole: types.EvidenceDiagramRoleOverride, Subject: "StageExplore", Object: "StageExtract", AnchorSymbol: "StageExtract", Source: "internal/types/enums.go", LineStart: 34, GroundingStatus: types.GroundingGrounded},
		{Kind: types.EvidenceDirect, DiagramRole: types.EvidenceDiagramRoleOverride, Subject: "StageExtract", Object: "StageFinalize", AnchorSymbol: "StageFinalize", Source: "internal/types/enums.go", LineStart: 35, GroundingStatus: types.GroundingGrounded},
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFGeneric}, evidence); len(got) != 0 {
		t.Fatalf("legal Mermaid chains must expose every typed visible edge: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_GenericFlowExplicitLogicalRelationNeedsTypedEvidence(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "pipeline", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart TD\n  A[StageAnalyze] --> E[StageExplore]\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "E", RelationKind: types.DiagramRelPrecedence,
		}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFGeneric}
	got := DiagramCallEdgeEvidenceMismatches(doc, view, nil)
	if len(got) != 1 || got[0].Issue != diagramSemanticRelationIssueNoEvidence ||
		got[0].Relation != types.DiagramRelPrecedence ||
		got[0].FromSymbol != "StageAnalyze" || got[0].ToSymbol != "StageExplore" {
		t.Fatalf("an explicit generic-flow precedence assertion must not self-authorize: %+v", got)
	}

	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceDirect, DiagramRole: types.EvidenceDiagramRoleOverride,
		Subject: "StageAnalyze", Object: "StageExplore", AnchorSymbol: "StageExplore",
		Source: "internal/types/enums.go", LineStart: 119,
		GroundingStatus: types.GroundingGrounded,
	}}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 0 {
		t.Fatalf("same-direction typed precedence evidence must authorize a generic-flow relation: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_GenericPresentationFlowWithoutTypedAnchorStaysOutsideEvidenceGate(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "concept", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart TD\n  Input --> Output\n",
		},
	}}}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFGeneric}, nil); len(got) != 0 {
		t.Fatalf("a generic presentation-only flow must not become a source relation claim: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_TypedFlowCannotDropRelationOwnership(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "pipeline", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramArchitecture, Language: "mermaid",
			Body: "flowchart TD\n  Producer[Producer] --> Consumer[Consumer]\n",
		},
	}}}
	flowView := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}
	got := DiagramCallEdgeEvidenceMismatches(doc, flowView, nil)
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueMissingRelationAnchor {
		t.Fatalf("typed flow body edge must retain a relation owner after metadata deletion: %+v", got)
	}

	// The same parsed carrier remains presentation-only for an ordinary
	// definition/architecture request. The hard boundary is the typed axis,
	// never label vocabulary or raw request/answer prose.
	defineView := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisDefine}
	if got := DiagramCallEdgeEvidenceMismatches(doc, defineView, nil); len(got) != 0 {
		t.Fatalf("non-flow architecture presentation must keep the legacy optional lane: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_EveryTypedRelationAxisCannotDropOwnership(t *testing.T) {
	for _, axis := range []types.PredicateAxis{
		types.AxisCall,
		types.AxisRegister,
		types.AxisReturn,
		types.AxisConfigure,
		types.AxisCondition,
		types.AxisImplement,
		types.AxisFlow,
	} {
		t.Run(string(axis), func(t *testing.T) {
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
				ID: "relation", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{
					Kind: types.DiagramArchitecture, Language: "mermaid",
					Body: "flowchart BT\n  Child[Child] --> Parent[Parent]\n",
				},
			}}}
			view := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: axis}
			got := DiagramCallEdgeEvidenceMismatches(doc, view, nil)
			if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueMissingRelationAnchor {
				t.Fatalf("typed relation axis %q must retain visible edge ownership: %+v", axis, got)
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_TypedImplementAxisKeepsDirectionAfterPatch(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "type-hierarchy", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramArchitecture, Language: "mermaid",
			Body: "flowchart BT\n  C[FileSink] --> P[Sink]\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "C", ToNode: "P", RelationKind: types.DiagramRelTypeRelation,
		}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisImplement}
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceRelationship, Producer: types.EvidenceProducerRepoMapImplementerRelation,
		Subject: "FileSink", Predicate: "implements", Object: "Sink",
		Source: "src/file_sink.go", LineStart: 10, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition,
	}}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 0 {
		t.Fatalf("same-direction typed implementer edge must pass: %+v", got)
	}

	// The exact production escape from r258: deleting metadata while retaining
	// the factual arrow must fail, even though the graph remains renderable.
	doc.Blocks[0].EdgeAnchors = nil
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueMissingRelationAnchor {
		t.Fatalf("metadata deletion must not preserve the same implementer claim: %+v", got)
	}

	// Reversing the visible arrow and its metadata still cannot reverse the
	// parser-owned type relation.
	doc.Blocks[0].Diagram.Body = "flowchart BT\n  P[Sink] --> C[FileSink]\n"
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "P", ToNode: "C", RelationKind: types.DiagramRelTypeRelation,
	}}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 1 || got[0].Issue != diagramTypeRelationEdgeIssueNoEvidence {
		t.Fatalf("reversed type relation must fail typed direction authority: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_ClassDiagramTypeRelationUsesCanonicalDirection(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "type-hierarchy", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramArchitecture, Language: "mermaid",
			Body: "classDiagram\n  class Sink\n  class FileSink\n  Sink <|.. FileSink\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "FileSink", ToNode: "Sink", RelationKind: types.DiagramRelTypeRelation,
		}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisImplement}
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceRelationship, Producer: types.EvidenceProducerRepoMapImplementerRelation,
		Subject: "FileSink", Predicate: "implements", Object: "Sink",
		Source: "src/file_sink.go", LineStart: 10, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition,
	}}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 0 {
		t.Fatalf("classDiagram realization must share the typed implementer direction: %+v", got)
	}

	// Removing relation ownership from the same visible class edge must no
	// longer turn it into an invisible, validation-free presentation edge.
	doc.Blocks[0].EdgeAnchors = nil
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueMissingRelationAnchor {
		t.Fatalf("classDiagram edge without typed ownership must fail closed: %+v", got)
	}

	// A model-authored anchor in the visual left-to-right spelling is the
	// reverse semantic relation and must not match the canonical class edge.
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "Sink", ToNode: "FileSink", RelationKind: types.DiagramRelTypeRelation,
	}}
	got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence)
	issues := map[string]bool{}
	for _, mismatch := range got {
		issues[mismatch.Issue] = true
	}
	if !issues[diagramCallEdgeIssueMissingRelationAnchor] || !issues[diagramCallEdgeIssueAnchorWithoutBodyEdge] {
		t.Fatalf("left-headed class syntax must not reverse typed endpoint identity: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_TypedFlowAcceptsOwnedGroundedEdge(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "pipeline", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart TD\n  Producer[Producer.Run] --> Consumer[Consumer.Run]\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "Producer", ToNode: "Consumer", RelationKind: types.DiagramRelCall,
		}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}
	evidence := []types.EvidenceItem{diagramEvidenceTestCall("Producer.Run", "Consumer.Run")}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 0 {
		t.Fatalf("same-direction owned typed flow edge must pass: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_TypedIdentityPairKeepsBusinessDisplayOutOfAuthority(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "pipeline", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramSequence, Language: "mermaid",
			Body: "sequenceDiagram\n  participant n1 as 合并附件并保存答案\n  participant n2 as 发布答案变更\n  n1->>n2: 持久化后提交变更\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "n1", ToNode: "n2",
			FromIdentity: "persistMergedAnswerDocumentWithAttachmentPolicy",
			ToIdentity:   "SetAnswerDocumentV2WithMutation",
			RelationKind: types.DiagramRelCall,
		}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFGeneric, RelationAxis: types.AxisFlow}
	evidence := []types.EvidenceItem{diagramEvidenceTestCall(
		"persistMergedAnswerDocumentWithAttachmentPolicy", "SetAnswerDocumentV2WithMutation",
	)}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 0 {
		t.Fatalf("typed endpoint selectors must let business display copy remain presentation-only: %+v", got)
	}

	// Identity selectors are not self-authenticating evidence. Repointing the
	// same visible edge to an unsupported typed pair must still fail closed.
	doc.Blocks[0].EdgeAnchors[0].ToIdentity = "ApplyAndPersistMutation"
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) == 0 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("unsupported typed identity pair must not mint a call edge: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_ExactNodeIdentityMustBindSameAnchorSide(t *testing.T) {
	evidence := []types.EvidenceItem{
		{
			Kind: types.EvidenceRelationship, Producer: types.EvidenceProducerRepoMapImplementerRelation,
			Subject: "analyzerEvaluator", Predicate: "implements", Object: "LoopController",
			Source: "internal/agent/analyzer.go", LineStart: 49, Scope: types.ScopeLine,
			AnchorKind: types.AnchorDefinition, GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind: types.EvidenceMechanism, Subject: "LoopController", AnchorSymbol: "LoopController",
			Source: "internal/agent/agent.go", LineStart: 519, Scope: types.ScopeLine,
			AnchorKind: types.AnchorDefinition, GroundingStatus: types.GroundingGrounded,
		},
	}
	view := &types.AnswerSemanticView{Family: types.QFGeneric}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "types", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramArchitecture, Language: "mermaid", Body: strings.Join([]string{
			"flowchart TD",
			`  LC["LoopController\ninternal/agent/agent.go:519"] --> A["analyzerEvaluator\ninternal/agent/analyzer.go:49"]`,
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "LC", ToNode: "A", FromIdentity: "analyzerEvaluator", ToIdentity: "LoopController",
			RelationKind: types.DiagramRelTypeRelation,
		}},
	}}}
	got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence)
	if len(got) != 1 || got[0].Issue != diagramEdgeAnchorNodeIdentityConflict ||
		got[0].FromSymbol != "analyzerEvaluator" || got[0].ToSymbol != "LoopController" {
		t.Fatalf("opposite visible-node bindings must fail before a correct typed pair can sign the graph: %+v", got)
	}
	mut := types.NewMutableState("explain the implementation relationship")
	mut.AppendEvidence(evidence)
	hints := preCheckDiagramCallEdgeEvidenceAlignment(doc, view, newPreEmitCheckContext(&types.BusContext{Mutable: mut}))
	if len(hints) != 1 ||
		!reflect.DeepEqual(hints[0].DiagramRelationFailureIssues, []string{diagramEdgeAnchorNodeIdentityConflict}) ||
		!strings.Contains(hints[0].Field, "from_node/to_node/from_identity/to_identity") ||
		!strings.Contains(hints[0].ExpectedShape, "from_node must denote from_identity") ||
		!strings.Contains(hints[0].ExpectedShape, "resolved_visible_identity=LoopController -> analyzerEvaluator") {
		t.Fatalf("pre-emit must return the surgical node/identity binding repair without rewriting the graph: %+v", hints)
	}

	doc.Blocks[0].Diagram.Body = strings.Join([]string{
		"flowchart TD",
		`  A["analyzerEvaluator\ninternal/agent/analyzer.go:49"] --> LC["LoopController\ninternal/agent/agent.go:519"]`,
	}, "\n")
	doc.Blocks[0].EdgeAnchors[0].FromNode = "A"
	doc.Blocks[0].EdgeAnchors[0].ToNode = "LC"
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 0 {
		t.Fatalf("same-side visible node and typed endpoint bindings must pass: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_ExactActorMayHostOwnedOperationEndpoint(t *testing.T) {
	call := diagramEvidenceTestCall("Service.Run", "Worker.Handle")
	call.OwnerSymbol = "Service"
	worker := types.EvidenceItem{
		Kind: types.EvidenceMechanism, Subject: "Worker", AnchorSymbol: "Worker",
		Source: "internal/worker.go", LineStart: 4, Scope: types.ScopeLine,
		AnchorKind: types.AnchorDefinition, GroundingStatus: types.GroundingGrounded,
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "actors", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			"  participant S as Service",
			"  participant W as Worker",
			"  S->>W: execute",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "S", ToNode: "W", FromIdentity: "Service.Run", ToIdentity: "Worker.Handle",
			RelationKind: types.DiagramRelCall,
		}},
	}}}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, []types.EvidenceItem{call, worker}); len(got) != 0 {
		t.Fatalf("an exact actor/component may remain the presentation carrier for its owned operation: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_TypedFlowUniquelyProjectsShortCallEndpoints(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "pipeline", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart TD\n  Run[runAnalyzePhase] --> Dispatch[dispatchStage]\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "Run", ToNode: "Dispatch", RelationKind: types.DiagramRelCall,
		}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}
	call := diagramEvidenceTestCall("Orchestrator.runAnalyzePhase", "Orchestrator.dispatchStage")
	call.AnchorSymbol = "dispatchStage"
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{call}); len(got) != 0 {
		t.Fatalf("one citable qualified call row must authorize its unique short presentation: %+v", got)
	}

	doc.Blocks[0].Diagram.Body = "flowchart TD\n  Dispatch[dispatchStage] --> Run[runAnalyzePhase]\n"
	doc.Blocks[0].EdgeAnchors[0].FromNode = "Dispatch"
	doc.Blocks[0].EdgeAnchors[0].ToNode = "Run"
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{call}); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("short presentation must preserve typed call direction: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_TypedFlowUniquelyProjectsMixedQualifiedEndpoints(t *testing.T) {
	view := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}
	call := diagramEvidenceTestCall("Orchestrator.runAnalyzePhase", "Orchestrator.dispatchStage")
	call.AnchorSymbol = "dispatchStage"

	tests := []struct {
		name, caller, callee string
	}{
		{name: "short caller", caller: "runAnalyzePhase", callee: "Orchestrator.dispatchStage"},
		{name: "short callee", caller: "Orchestrator.runAnalyzePhase", callee: "dispatchStage"},
		{name: "c-plus-plus spelling", caller: "Orchestrator::runAnalyzePhase", callee: "dispatchStage"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
				ID: "pipeline", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{
					Kind: types.DiagramSequence, Language: "mermaid",
					Body: "sequenceDiagram\n  participant A as " + tc.caller + "\n  participant B as " + tc.callee + "\n  A->>B: dispatchStage(stage)\n",
				},
				EdgeAnchors: []types.DiagramEdgeAnchor{{
					FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall,
				}},
			}}}
			if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{call}); len(got) != 0 {
				t.Fatalf("one grounded call row must authorize a unique mixed short/qualified presentation: %+v", got)
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_TypedFlowMixedShortEndpointAmbiguityFailsClosed(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "pipeline", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramSequence, Language: "mermaid",
			Body: "sequenceDiagram\n  participant A as run\n  participant B as Sink.consume\n  A->>B: consume()\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall,
		}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}
	first := diagramEvidenceTestCall("Alpha.run", "Sink.consume")
	second := diagramEvidenceTestCall("Beta.run", "Sink.consume")
	second.ID = "ev-beta-run"
	second.Source = "internal/other.go"
	second.LineStart = 20
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{first, second}); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("a mixed presentation must not collapse same-tail callers under different owners: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_TypedFlowShortEndpointOwnerAmbiguityFailsClosed(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "pipeline", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart TD\n  Run[run] --> Sink[consume]\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "Run", ToNode: "Sink", RelationKind: types.DiagramRelCall,
		}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}
	first := diagramEvidenceTestCall("Alpha.run", "Sink.consume")
	first.AnchorSymbol = "consume"
	second := diagramEvidenceTestCall("Beta.run", "Sink.consume")
	second.ID = "ev-beta-run"
	second.Source = "internal/other.go"
	second.LineStart = 20
	second.AnchorSymbol = "consume"
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{first, second}); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("same-tail operations under distinct qualified owners must remain ambiguous: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_DeterministicSourceQualifiedCallerKeepsFileAxis(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "pipeline", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart TD\n  Main[ReadModeMainStageBindings] --> Stages[AllMainStages]\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "Main", ToNode: "Stages", RelationKind: types.DiagramRelCall,
		}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}
	call := diagramEvidenceTestCall("internal/types/stage_binding.go:ReadModeMainStageBindings", "AllMainStages")
	call.Source = "internal/types/stage_binding.go"
	call.Producer = "dataflow.lowerer.go"
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{call}); len(got) != 0 {
		t.Fatalf("exact deterministic source-qualified caller must bind its bare diagram symbol: %+v", got)
	}

	call.Source = "internal/types/other.go"
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{call}); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("same tail under a different source axis must fail closed: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_TypedFlowAcceptsGroundedPrecedence(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "pipeline", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart TD\n  A[StageAnalyze] --> E[StageExplore]\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "E", RelationKind: types.DiagramRelPrecedence,
		}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceRelationship, AnchorKind: types.AnchorPrecedence,
		Subject: "StageAnalyze", Object: "StageExplore",
		Source: "pipeline.go", LineStart: 11, LineEnd: 12,
		GroundingStatus: types.GroundingGrounded,
	}}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 0 {
		t.Fatalf("grounded source-order carrier must authorize precedence edge: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_TypedFlowResolvesDisplayValueAndCanonicalIdentity(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "pipeline", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart TD\n  A[analyze<br/>StageAnalyze] --> E[explore<br/>StageExplore]\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "E", RelationKind: types.DiagramRelPrecedence,
		}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceRelationship, AnchorKind: types.AnchorPrecedence,
		Subject: "StageAnalyze", Object: "StageExplore", AnchorSymbol: "StageAnalyze",
		Source: "pipeline.go", LineStart: 11, LineEnd: 12,
		GroundingStatus: types.GroundingGrounded,
	}}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 0 {
		t.Fatalf("a unique typed identity in a value+identity label must authorize the edge: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_LabelWithTwoTypedIdentitiesFailsClosed(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "ambiguous", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart TD\n  A[AlphaStage<br/>BetaStage] --> E[SinkStage]\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "E", RelationKind: types.DiagramRelPrecedence,
		}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}
	evidence := []types.EvidenceItem{
		{
			Kind: types.EvidenceRelationship, AnchorKind: types.AnchorPrecedence,
			Subject: "AlphaStage", Object: "SinkStage", AnchorSymbol: "AlphaStage",
			Source: "pipeline.go", LineStart: 11, LineEnd: 12,
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind: types.EvidenceRelationship, AnchorKind: types.AnchorPrecedence,
			Subject: "BetaStage", Object: "SinkStage", AnchorSymbol: "BetaStage",
			Source: "pipeline.go", LineStart: 21, LineEnd: 22,
			GroundingStatus: types.GroundingGrounded,
		},
	}
	got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence)
	if len(got) != 1 || got[0].Issue != diagramSemanticRelationIssueNoEvidence {
		t.Fatalf("two distinct typed identities in one label must remain ambiguous: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_RegistrationRequiresExactTypedEvidence(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "binding", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramCallDAG, Language: "mermaid",
			Body: "flowchart TD\n  M[_fastlex] --> W[py.tokenize_bytes]\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "M", ToNode: "W", RelationKind: types.DiagramRelRegister,
			ClaimForm: types.ClaimRegistrationEdge,
		}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFCallChain}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, nil); len(got) != 1 || got[0].Issue != diagramRegistrationEdgeIssueNoEvidence {
		t.Fatalf("unproved registration must fail closed: %+v", got)
	}
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceRegistration, Subject: "_fastlex", Object: "py.tokenize_bytes",
		Source: "src/lib.rs", LineStart: 47, AnchorKind: types.AnchorDefinition,
		Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
	}}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 0 {
		t.Fatalf("exact typed registration must authorize only the registration edge: %+v", got)
	}
	reversed := evidence[0]
	reversed.Subject, reversed.Object = reversed.Object, reversed.Subject
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{reversed}); len(got) != 1 {
		t.Fatalf("reverse registration must not authorize the edge: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_ValueFlowRequiresSameDirectionTypedEvidence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		relation types.DiagramRelationKind
		anchor   types.AnchorKind
		from     string
		to       string
	}{
		{"cpp_factory_return", types.DiagramRelReturn, types.AnchorReturn, "SinkRegistry.create", "ConsoleSink"},
		{"arkts_assignment", types.DiagramRelAssignment, types.AnchorAssignment, "handler", "ConsoleHandler"},
		{"cangjie_return", types.DiagramRelReturn, types.AnchorReturn, "Provider.create", "FastProcessor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
				ID: "value-flow", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{
					Kind: types.DiagramCallDAG, Language: "mermaid",
					Body: "flowchart TD\n  A[" + tc.from + "] --> B[" + tc.to + "]\n",
				},
				EdgeAnchors: []types.DiagramEdgeAnchor{{
					FromNode: "A", ToNode: "B", RelationKind: tc.relation,
				}},
			}}}
			view := &types.AnswerSemanticView{Family: types.QFCallChain}
			missingIssue := diagramAssignmentEdgeIssueNoEvidence
			if tc.relation == types.DiagramRelReturn {
				missingIssue = diagramReturnEdgeIssueNoEvidence
			}
			if got := DiagramCallEdgeEvidenceMismatches(doc, view, nil); len(got) != 1 || got[0].Issue != missingIssue {
				t.Fatalf("unproved value-flow edge must fail closed: %+v", got)
			}
			evidenceSubject := tc.from
			if tc.name == "cpp_factory_return" {
				evidenceSubject = "SinkRegistry::create"
			}
			evidence := []types.EvidenceItem{{
				Kind: types.EvidenceDirect, Subject: evidenceSubject, Object: tc.to,
				Source: "src/factory", LineStart: 17, AnchorKind: tc.anchor,
				Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
			}}
			if tc.anchor == types.AnchorAssignment {
				evidence[0].Snippet = evidenceSubject + " = " + tc.to
			}
			if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 0 {
				t.Fatalf("same-direction typed value-flow fact must authorize edge: %+v", got)
			}
			reversed := evidence[0]
			reversed.Subject, reversed.Object = reversed.Object, reversed.Subject
			if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{reversed}); len(got) != 1 || got[0].Issue != missingIssue {
				t.Fatalf("reverse value-flow fact must not authorize edge: %+v", got)
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_IndexedAssignmentUsesParserContainerWithoutRelabelingRegistration(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "selector-binding", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart LR\n  R[REGISTRY] --> C[cls]\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "R", ToNode: "C", RelationKind: types.DiagramRelAssignment,
		}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFCallChain}
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceConcrete, Scope: types.ScopeLine,
		AnchorKind: types.AnchorAssignment, Subject: "REGISTRY[name]", Object: "cls",
		OwnerSymbol: "register", Source: "pipeline/registry.py", LineStart: 17,
		GroundingStatus: types.GroundingGrounded, Snippet: "REGISTRY[name] = cls",
	}}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 0 {
		t.Fatalf("parser-proved indexed assignment container must authorize only the assignment view: %+v", got)
	}
	doc.Blocks[0].EdgeAnchors[0].RelationKind = types.DiagramRelRegister
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 1 || got[0].Issue != diagramRegistrationEdgeIssueNoEvidence {
		t.Fatalf("indexed assignment must not be relabeled as registration authority: %+v", got)
	}
	doc.Blocks[0].EdgeAnchors[0].RelationKind = types.DiagramRelAssignment
	doc.Blocks[0].Diagram.Body = "flowchart LR\n  C[cls] --> R[REGISTRY]\n"
	doc.Blocks[0].EdgeAnchors[0].FromNode = "C"
	doc.Blocks[0].EdgeAnchors[0].ToNode = "R"
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 1 || got[0].Issue != diagramAssignmentEdgeIssueNoEvidence {
		t.Fatalf("reverse indexed assignment must fail closed: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_DataFlowUsesExactRHSIntoLHSDirection(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "data-flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart LR\n  V[output.AnalysisIR] --> R[busCtx.AnalysisIR]\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "V", ToNode: "R", RelationKind: types.DiagramRelDataFlow,
		}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFGeneric, RelationAxis: types.AxisFlow}
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceRelationship, Scope: types.ScopeLine,
		AnchorKind: types.AnchorAssignment, Subject: "busCtx.AnalysisIR", Object: "output.AnalysisIR",
		Source: "internal/orchestrator/orchestrator.go", LineStart: 2520,
		GroundingStatus: types.GroundingGrounded,
		Snippet:         "o.busCtx.AnalysisIR = output.AnalysisIR",
	}}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 0 {
		t.Fatalf("exact RHS -> LHS data flow must be accepted without call authority: %+v", got)
	}

	doc.Blocks[0].Diagram.Body = "flowchart LR\n  R[busCtx.AnalysisIR] --> V[output.AnalysisIR]\n"
	doc.Blocks[0].EdgeAnchors[0].FromNode = "R"
	doc.Blocks[0].EdgeAnchors[0].ToNode = "V"
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 1 || got[0].Issue != diagramDataFlowEdgeIssueNoEvidence {
		t.Fatalf("LHS -> RHS must not pass as execution-direction data_flow: %+v", got)
	}

	doc.Blocks[0].EdgeAnchors[0].RelationKind = types.DiagramRelAssignment
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 0 {
		t.Fatalf("the same exact LHS -> RHS pair must remain valid in binding-view assignment direction: %+v", got)
	}

	falseEndpoints := evidence[0]
	falseEndpoints.Subject = "applyStageOutput"
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{falseEndpoints}); len(got) != 1 || got[0].Issue != diagramAssignmentEdgeIssueNoEvidence {
		t.Fatalf("assignment-shaped line with false model endpoints must authorize neither view: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_DistinctTypedDataEndpointsCannotCollapseToOneVisibleAlias(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "data-flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramSequence, Language: "mermaid",
			Body: "sequenceDiagram\n  participant IRflow as Analysis IR flow\n  IRflow->>IRflow: data transfer\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "IRflow", ToNode: "IRflow",
			FromIdentity: "output.AnalysisIR", ToIdentity: "busCtx.AnalysisIR",
			RelationKind: types.DiagramRelDataFlow,
		}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFGeneric, RelationAxis: types.AxisFlow}
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceRelationship, Scope: types.ScopeLine,
		AnchorKind: types.AnchorAssignment, Subject: "busCtx.AnalysisIR", Object: "output.AnalysisIR",
		Source: "internal/orchestrator/orchestrator.go", LineStart: 2520,
		GroundingStatus: types.GroundingGrounded,
		Snippet:         "o.busCtx.AnalysisIR = output.AnalysisIR",
	}}
	got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence)
	if len(got) != 1 || got[0].Issue != diagramTypedEndpointsCollapsedToSelfEdge ||
		got[0].FromSymbol != "output.AnalysisIR" || got[0].ToSymbol != "busCtx.AnalysisIR" {
		t.Fatalf("two distinct typed value endpoints need two visible aliases, got %+v", got)
	}

	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n  participant Out as output.AnalysisIR\n  participant Bus as busCtx.AnalysisIR\n  Out->>Bus: data transfer\n"
	doc.Blocks[0].EdgeAnchors[0].FromNode = "Out"
	doc.Blocks[0].EdgeAnchors[0].ToNode = "Bus"
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 0 {
		t.Fatalf("the same typed data flow with distinct visible aliases must pass: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_TypeRelationRequiresSameDirectionParserEvidence(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "type-hierarchy", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramArchitecture, Language: "mermaid",
			Body: "flowchart BT\n  C[FileSink] --> P[Sink]\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "C", ToNode: "P", RelationKind: types.DiagramRelTypeRelation,
		}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFGeneric}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, nil); len(got) != 1 || got[0].Issue != diagramTypeRelationEdgeIssueNoEvidence {
		t.Fatalf("unproved type relation must fail closed: %+v", got)
	}
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceRelationship, Producer: "repomap_structural_relation",
		Subject: "FileSink", Predicate: "inheritance", Object: "Sink",
		Source: "include/logx/file_sink.hpp", LineStart: 10, Scope: types.ScopeLine,
		AnchorKind: types.AnchorDefinition,
	}}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 0 {
		t.Fatalf("same-direction parser type relation must authorize edge: %+v", got)
	}
	reversed := evidence[0]
	reversed.Subject, reversed.Object = reversed.Object, reversed.Subject
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{reversed}); len(got) != 1 || got[0].Issue != diagramTypeRelationEdgeIssueNoEvidence {
		t.Fatalf("reverse parser relation must not authorize edge: %+v", got)
	}
	modelOnly := evidence[0]
	modelOnly.Producer = "explorer.emit_evidence"
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{modelOnly}); len(got) != 1 {
		t.Fatalf("model-authored relationship must not replace parser authority: %+v", got)
	}
	implicit := evidence[0]
	implicit.Producer = types.EvidenceProducerRepoMapImplementerRelation
	implicit.Predicate = "implements"
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{implicit}); len(got) != 0 {
		t.Fatalf("typed ImplementersOf evidence must authorize the exact same-direction edge: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_CallDAGUnanchoredControlEdgeStillFailsClosed(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Kind = types.DiagramCallDAG
	doc.Blocks[0].Diagram.Body = "flowchart TD\n  A[Alpha.Run] --> G[Guard]\n"
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "A", ToNode: "Other", RelationKind: types.DiagramRelGuard, ClaimForm: types.ClaimGuardCondition,
	}}
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, nil)
	if len(got) != 2 || got[0].Issue != diagramCallEdgeIssueMissingAnchor ||
		got[1].Issue != diagramCallEdgeIssueAnchorWithoutBodyEdge {
		t.Fatalf("a non-matching guard anchor must neither hide the unanchored DAG edge nor survive as metadata-only authority: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SequenceGuardUsesGuardAuthority(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelGuard, ClaimForm: types.ClaimGuardCondition,
	}}
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, nil)
	if len(got) != 1 || got[0].Issue != diagramSemanticRelationIssueNoEvidence || got[0].Relation != types.DiagramRelGuard {
		t.Fatalf("a typed sequence guard must fail its own evidence gate without being recast as a call: %+v", got)
	}
}

func TestRunPreEmitChecks_MixedCallDAGGuardStaysOutsideCallAuthority(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "mixed", Kind: types.BlockDiagram,
		ClaimUses: []types.RenderedClaimUse{
			{ClaimForm: types.ClaimCallEdge},
			{ClaimForm: types.ClaimGuardCondition},
		},
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramCallDAG, Language: "mermaid", Body: strings.Join([]string{
			"flowchart TD",
			"  C[VisitController.create] --> S[VisitService.schedule]",
			"  S -->|countOpenVisits >= max| G[countOpenVisits]",
			"  S -->|pass| R[VisitRepository.insert]",
			"  R --> A[AuditLog.record]",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "C", ToNode: "S", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
			{FromNode: "S", ToNode: "G", RelationKind: types.DiagramRelGuard, ClaimForm: types.ClaimGuardCondition},
			{FromNode: "S", ToNode: "R", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
			{FromNode: "R", ToNode: "A", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
		},
	}}}
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("VisitController.create", "VisitService.schedule"),
		diagramEvidenceTestCall("VisitService.schedule", "VisitRepository.insert"),
		diagramEvidenceTestCall("VisitRepository.insert", "AuditLog.record"),
		{
			ID: "guard", Kind: types.EvidenceConditional, Scope: types.ScopeLine,
			Source: "VisitService.java", LineStart: 18, AnchorKind: types.AnchorCondition,
			AnchorSymbol: "countOpenVisits", Subject: "VisitService.schedule",
			Condition: "countOpenVisits >= max", GroundingStatus: types.GroundingGrounded,
		},
	}
	mut := types.NewMutableState("mixed call and guard DAG")
	mut.AppendEvidence(evidence)
	hints := runPreEmitChecks(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, nil, &types.BusContext{Mutable: mut})
	for _, hint := range hints {
		if hint.Kind == types.ViolDiagramCallEdgeUnproven {
			t.Fatalf("the wired pre-emit call authority must accept the typed mixed DAG: %+v", hints)
		}
	}
}

func TestRunPreEmitChecks_StrictLogicalRelationCannotReplaceMissingEvidence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		relation types.DiagramRelationKind
		body     string
	}{
		{name: "labelled precedence relabel", relation: types.DiagramRelPrecedence, body: "flowchart TD\n  A[Sink.write] -.->|virtual dispatch| B[ConsoleSink.write]\n"},
		{name: "unlabelled guard relabel", relation: types.DiagramRelGuard, body: "flowchart TD\n  A[kind] --> B[SinkRegistry.create]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
				ID: "escape", Kind: types.BlockDiagram,
				Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramCallDAG, Language: "mermaid", Body: tc.body},
				EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", RelationKind: tc.relation}},
			}}}
			hints := runPreEmitChecks(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, nil, &types.BusContext{Mutable: types.NewMutableState("logical relation escape")})
			for _, hint := range hints {
				if hint.Kind == types.ViolDiagramCallEdgeUnproven &&
					strings.Contains(hint.ExpectedShape, diagramSemanticRelationIssueNoEvidence) &&
					strings.Contains(hint.ExpectedShape, "relation_kind="+string(tc.relation)) &&
					strings.Contains(hint.ExpectedShape, types.GroundedSourceDiagramRelationEvidenceContract) {
					return
				}
			}
			t.Fatalf("typed logical relabel must reach the production hard gate with an executable repair: %+v", hints)
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_DefinitionCannotAuthorizeDirection(t *testing.T) {
	view := &types.AnswerSemanticView{Family: types.QFCallChain}
	evidence := []types.EvidenceItem{{
		ID:              "ev-beta-definition",
		Kind:            types.EvidenceDirect,
		Subject:         "Beta.Run",
		Source:          "internal/example.go",
		LineStart:       20,
		AnchorKind:      types.AnchorDefinition,
		Scope:           types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
	}}
	got := DiagramCallEdgeEvidenceMismatches(diagramEvidenceTestDoc("A", "B"), view, evidence)
	if len(got) != 1 {
		t.Fatalf("a definition proves symbol existence, not Alpha.Run -> Beta.Run: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_AbsentEvidenceCannotAuthorizeDirection(t *testing.T) {
	got := DiagramCallEdgeEvidenceMismatches(diagramEvidenceTestDoc("A", "B"),
		&types.AnswerSemanticView{Family: types.QFCallChain}, nil)
	if len(got) != 1 || got[0].FromSymbol != "Alpha.Run" || got[0].ToSymbol != "Beta.Run" {
		t.Fatalf("an empty typed call-edge pool cannot authorize a structured edge: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_CanonicalNodeKeepsRawEvidenceAnchor(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "sequence", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramSequence, Language: "mermaid",
			Body: "sequenceDiagram\n  participant G as gate.Run\n  participant W as gate.RunWith\n  G->>W: calls\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "G", ToNode: "W", FromIdentity: "gate.Run", ToIdentity: "RunWith",
			RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
		}},
	}}}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{diagramEvidenceTestCall("gate.Run", "RunWith")}); len(got) != 0 {
		t.Fatalf("canonical visible node with unchanged raw evidence anchor was rejected: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_DoesNotGateTraceProjectionFamily(t *testing.T) {
	view := &types.AnswerSemanticView{Family: types.QFRootCauseTrace}
	doc := diagramEvidenceTestDoc("B", "A")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n  participant A as \"`Alpha.Run`\"\n  participant B as \"`Beta.Run`\"\n  B->>A: invoke\n"
	got := DiagramCallEdgeEvidenceMismatches(doc, view,
		[]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
	if len(got) != 0 {
		t.Fatalf("runtime/root-cause trace diagrams must stay outside source call-edge authority: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_ExplicitCallAuthorityIsFamilyIndependent(t *testing.T) {
	for _, family := range types.AllQuestionFamilies() {
		if family == types.QFRootCauseTrace {
			continue
		}
		t.Run(string(family), func(t *testing.T) {
			doc := diagramEvidenceTestDoc("A", "B")
			got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: family}, nil)
			if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
				t.Fatalf("explicit typed call must require same-direction evidence in family %s: %+v", family, got)
			}
			if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: family},
				[]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")}); len(got) != 0 {
				t.Fatalf("exact typed call must pass in family %s: %+v", family, got)
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_StandaloneStructuredListUsesSameAuthority(t *testing.T) {
	call := types.EvidenceItem{
		ID: "call", Kind: types.EvidenceRelationship,
		Subject: "Entry.run", Object: "Worker.handle", Predicate: "calls",
		Source: "src/entry.go", LineStart: 12, LineEnd: 12,
		AnchorKind: types.AnchorCall, GroundingStatus: types.GroundingGrounded,
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
		ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "entry", ToNode: "worker",
			FromIdentity: "Entry.run", ToIdentity: "Worker.handle",
			RelationKind: types.DiagramRelCall,
		}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFCallChain}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{call}); len(got) != 0 {
		t.Fatalf("grounded standalone list relation rejected: %+v", got)
	}
	doc.Blocks[0].EdgeAnchors[0].FromIdentity = "Worker.handle"
	doc.Blocks[0].EdgeAnchors[0].ToIdentity = "Entry.run"
	got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{call})
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("reverse standalone list relation must fail the shared authority: %+v", got)
	}

	doc.Blocks[0].EdgeAnchors[0].FromIdentity = "Entry.run"
	doc.Blocks[0].EdgeAnchors[0].ToIdentity = ""
	got = DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{call})
	if len(got) != 1 || got[0].Issue != diagramStandaloneRelationIdentityMissing {
		t.Fatalf("standalone relation must preserve both exact endpoint identities: %+v", got)
	}
}

func TestPreCheckStandaloneCallChainRelationAnchorPresenceFailsLoudWithoutReadingProse(t *testing.T) {
	for _, form := range []types.ClaimForm{
		types.ClaimCallEdge,
		types.ClaimCallbackHandoff,
		types.ClaimRegistrationEdge,
	} {
		t.Run(string(form), func(t *testing.T) {
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
				ID: "principal-path", Kind: types.BlockOrderedList,
				SurfaceRole: types.SurfacePrincipal,
				ClaimUses:   []types.RenderedClaimUse{{ClaimForm: form}},
				Items: []types.AnswerBlockItem{{
					Label: "display words are deliberately irrelevant",
					Text:  "changing this prose cannot affect the typed gate",
				}},
			}}}
			hints := preCheckDiagramCallEdgeEvidenceAlignment(
				doc,
				&types.AnswerSemanticView{Family: types.QFCallChain},
				newPreEmitCheckContext(),
			)
			if len(hints) != 1 || hints[0].HardSignal != preEmitHardSignalTypedCallEdgeEvidence ||
				hints[0].Field != `blocks[id="principal-path"].edge_anchors` ||
				!strings.Contains(hints[0].ExpectedShape, "no Mermaid block is required") ||
				!strings.Contains(hints[0].ExpectedShape, string(form)) ||
				!reflect.DeepEqual(hints[0].DiagramRelationFailureIssues, []string{diagramStandaloneRelationClaimHasNoAnchor}) ||
				!reflect.DeepEqual(hints[0].RelationRepairOrdinaryBlockIDs, []string{"principal-path"}) {
				t.Fatalf("zero-anchor relation claim must fail through the wired typed lane: %+v", hints)
			}
			repair := emitFixHintsRepair(hints)
			if repair == nil || repair.Metadata[types.ToolRepairMetaRelationRepairOrdinaryBlockIDsJSON] != `["principal-path"]` {
				t.Fatalf("same-generation ordinary relation grant was not ferried as typed metadata: %+v", repair)
			}
		})
	}
}

func TestPreCheckStandaloneCallChainRelationAnchorPresencePublishesClaimScopedTypedCandidates(t *testing.T) {
	call := types.EvidenceItem{
		ID: "ev-call", Kind: types.EvidenceRelationship, Scope: types.ScopeLine,
		Subject: "run_pipeline", Object: "resolve", Predicate: "calls",
		Source: "pipeline/runner.py", LineStart: 15, LineEnd: 15,
		AnchorKind: types.AnchorCall, GroundingStatus: types.GroundingGrounded,
	}
	callback := types.EvidenceItem{
		ID: "ev-callback", Kind: types.EvidenceRelationship, Scope: types.ScopeLine,
		Subject: "loop.run_in_executor", Object: "plugin.handle", Predicate: "passes callback",
		Source: "pipeline/runner.py", LineStart: 17, LineEnd: 17,
		AnchorKind: types.AnchorCallback, GroundingStatus: types.GroundingGrounded,
	}
	returnFact := types.EvidenceItem{
		ID: "ev-return", Kind: types.EvidenceRelationship, Scope: types.ScopeLine,
		Subject: "resolve", Object: "cls()", Predicate: "returns",
		Source: "pipeline/registry.py", LineStart: 34, LineEnd: 34,
		AnchorKind: types.AnchorReturn, GroundingStatus: types.GroundingGrounded,
	}
	ungroundedCall := call
	ungroundedCall.ID = "ev-untrusted"
	ungroundedCall.Subject = "Other.start"
	ungroundedCall.Object = "Other.finish"
	ungroundedCall.GroundingStatus = types.GroundingUngrounded
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "principal-path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
		ClaimUses: []types.RenderedClaimUse{
			{ClaimForm: types.ClaimCallEdge, EvidenceID: "ev-call"},
			{ClaimForm: types.ClaimCallbackHandoff, EvidenceID: "ev-callback"},
		},
		Items: []types.AnswerBlockItem{
			{ID: "call", EvidenceIDs: []string{"ev-call"}},
			{ID: "callback", EvidenceIDs: []string{"ev-callback"}},
		},
	}}}
	ctx := &types.BusContext{
		Mutable:       types.NewMutableState("zero-anchor candidate projection"),
		EvidenceItems: []types.EvidenceItem{callback, returnFact, ungroundedCall, call},
	}
	hints := preCheckStandaloneCallChainRelationAnchorPresence(
		doc, &types.AnswerSemanticView{Family: types.QFCallChain}, newPreEmitCheckContext(ctx),
	)
	if len(hints) != 1 {
		t.Fatalf("expected one zero-anchor hint, got %+v", hints)
	}
	var delta types.AnswerDiagramRelationRepairDelta
	if err := json.Unmarshal([]byte(hints[0].DiagramRelationRepairDeltaJSON), &delta); err != nil ||
		len(delta.AllowedAdditions) != 2 || delta.AllowedAdditions[0].EvidenceID == "" || delta.AllowedAdditions[1].EvidenceID == "" {
		t.Fatalf("model-selected claim/item evidence must publish exact additions-only repair authority: err=%v delta=%+v", err, delta)
	}
	for _, want := range []string{
		`diagram_edge_edits action=add`, `current addition_ref row(s)`,
		`Do not use replace_blocks`, `do not copy or rewrite the block's visible title`,
		`Exact citable relation candidates matching this block's model-selected claim_form(s)`,
		`relation_kind:"call"`, `from_identity:"run_pipeline"`, `to_identity:"resolve"`,
		`evidence_id:"ev-call"`, `source:"pipeline/runner.py:15"`,
		`relation_kind:"callback"`, `from_identity:"loop.run_in_executor"`,
		`to_identity:"plugin.handle"`, `evidence_id:"ev-callback"`,
	} {
		if !strings.Contains(hints[0].ExpectedShape, want) {
			t.Fatalf("zero-anchor typed candidate guidance missing %q: %s", want, hints[0].ExpectedShape)
		}
	}
	for _, forbidden := range []string{"ev-return", "cls()", "ev-untrusted", "Other.start"} {
		if strings.Contains(hints[0].ExpectedShape, forbidden) {
			t.Fatalf("candidate outside selected claim forms or authority escaped: %q in %s", forbidden, hints[0].ExpectedShape)
		}
	}
}

func TestPreCheckStandaloneCallChainRelationAnchorPresencePrioritizesModelSelectedItemEvidenceThenLedgerOrder(t *testing.T) {
	call := func(id, from, to string, line int) types.EvidenceItem {
		return types.EvidenceItem{
			ID: id, Kind: types.EvidenceRelationship, Scope: types.ScopeLine,
			Subject: from, Object: to, Predicate: "calls",
			Source: "src/main.rs", LineStart: line, LineEnd: line,
			AnchorKind: types.AnchorCall, GroundingStatus: types.GroundingGrounded,
		}
	}
	evidence := []types.EvidenceItem{
		call("ev-main-run", "main", "run", 10),
		call("ev-run-collect", "run", "walker::collect_files", 20),
		call("ev-run-index", "run", "index_file", 23),
		call("ev-index-match", "index_file", "Matcher.is_match", 30),
		call("ev-alpha-terminal", "AlphaMatcher.is_match", "str.contains", 40),
		call("ev-selected-terminal", "RegexLikeMatcher.is_match", "str.find", 50),
		call("ev-zeta-terminal", "ZetaMatcher.is_match", "str.ends_with", 60),
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "principal-path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
		ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
		Items: []types.AnswerBlockItem{{
			Label:       "visible text is not read",
			Text:        "nor is this prose",
			EvidenceIDs: []string{"ev-selected-terminal"},
		}},
	}}}
	ctx := &types.BusContext{Mutable: types.NewMutableState("typed candidate order"), EvidenceItems: evidence}
	hints := preCheckStandaloneCallChainRelationAnchorPresence(
		doc, &types.AnswerSemanticView{Family: types.QFCallChain}, newPreEmitCheckContext(ctx),
	)
	if len(hints) != 1 {
		t.Fatalf("expected one zero-anchor hint, got %+v", hints)
	}
	got := hints[0].ExpectedShape
	selectedAt := strings.Index(got, `evidence_id:"ev-selected-terminal"`)
	mainAt := strings.Index(got, `evidence_id:"ev-main-run"`)
	collectAt := strings.Index(got, `evidence_id:"ev-run-collect"`)
	alphaAt := strings.Index(got, `evidence_id:"ev-alpha-terminal"`)
	if selectedAt < 0 || mainAt < 0 || collectAt < 0 || alphaAt < 0 {
		t.Fatalf("expected selected and evidence-ledger candidates in bounded hint: %s", got)
	}
	if !(selectedAt < mainAt && mainAt < collectAt && collectAt < alphaAt) {
		t.Fatalf("candidate order must be model-selected item evidence first, then stable evidence-ledger order: %s", got)
	}
	if strings.Contains(got, `evidence_id:"ev-zeta-terminal"`) {
		t.Fatalf("bounded hint must retain the six highest-priority candidates: %s", got)
	}
}

func TestPreCheckStandaloneCallChainRelationAnchorPresenceRejectsOnlyUnownedTypedKind(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "principal-path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
		ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "entry", ToNode: "native", FromIdentity: "Entry.run", ToIdentity: "native.run", RelationKind: types.DiagramRelCall},
			{FromNode: "native", ToNode: "binding", FromIdentity: "native.run", ToIdentity: "binding.run", RelationKind: types.DiagramRelRegister},
		},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFCallChain}
	if removed := normalizeOrphanDiagramEdgeAnchors(doc, view); removed != 0 || len(doc.Blocks[0].EdgeAnchors) != 2 {
		t.Fatalf("pre-emit normalizer must preserve a converging model-authored relation set: removed=%d doc=%+v", removed, doc)
	}
	hints := preCheckStandaloneCallChainRelationAnchorPresence(doc, view)
	if len(hints) != 1 ||
		!reflect.DeepEqual(hints[0].DiagramRelationFailureIssues, []string{diagramStandaloneRelationAnchorHasNoClaim}) ||
		!strings.Contains(hints[0].ExpectedShape, string(types.ClaimRegistrationEdge)) ||
		!strings.Contains(hints[0].ExpectedShape, "Preserve the other model-authored anchors") {
		t.Fatalf("mixed carrier did not receive the precise ownership repair: %+v", hints)
	}
	if len(doc.Blocks[0].EdgeAnchors) != 2 {
		t.Fatalf("typed ownership check mutated model-authored relations: %+v", doc.Blocks[0].EdgeAnchors)
	}

	doc.Blocks[0].ClaimUses = append(doc.Blocks[0].ClaimUses, types.RenderedClaimUse{ClaimForm: types.ClaimRegistrationEdge})
	if hints := preCheckStandaloneCallChainRelationAnchorPresence(doc, view); len(hints) != 0 {
		t.Fatalf("matching explicit claim must close the relation ownership contract: %+v", hints)
	}
}

func TestPreCheckStandaloneCallChainPrincipalPathFacetRequiresSameBlockRelationOwner(t *testing.T) {
	base := types.AnswerBlock{
		ID:          "principal-path",
		Kind:        types.BlockOrderedList,
		SurfaceRole: types.SurfacePrincipal,
		FacetIDs:    []string{string(types.FacetCurrentCodePath), string(types.FacetPrincipalPathEdge)},
		Items: []types.AnswerBlockItem{{
			Label: "visible hop prose is deliberately irrelevant",
			Text:  "neither this text nor the request can satisfy the typed relation owner",
		}},
	}
	hints := preCheckDiagramCallEdgeEvidenceAlignment(
		&types.AnswerDocumentV2{Blocks: []types.AnswerBlock{base}},
		&types.AnswerSemanticView{Family: types.QFCallChain},
		newPreEmitCheckContext(),
	)
	if len(hints) != 1 || hints[0].ForceHard ||
		hints[0].HardSignal != preEmitHardSignalTypedCallEdgeEvidence ||
		!strings.Contains(hints[0].Field, `.claim_uses`) ||
		!strings.Contains(hints[0].Field, `.edge_anchors`) ||
		!strings.Contains(hints[0].ExpectedShape, "remove facet_id") ||
		!reflect.DeepEqual(hints[0].DiagramRelationFailureIssues, []string{diagramStandalonePrincipalPathMissingOwner}) {
		t.Fatalf("principal path facet without a same-block relation owner must fail through the typed hard lane: %+v", hints)
	}
	tagged := tagPreEmitHints(types.ViolDiagramCallEdgeUnproven, hints)
	hard, advisory := splitPreEmitHintsByGate(tagged)
	if len(hard) != 1 || len(advisory) != 0 {
		t.Fatalf("typed facet/owner contradiction must never degrade to advisory: hard=%+v advisory=%+v", hard, advisory)
	}

	// Definition facts are descriptive and cannot own a directed-path facet.
	base.ClaimUses = []types.RenderedClaimUse{{ClaimForm: types.ClaimDefinitionFact, FacetID: string(types.FacetPrincipalPathEdge)}}
	if got := preCheckStandaloneCallChainRelationAnchorPresence(
		&types.AnswerDocumentV2{Blocks: []types.AnswerBlock{base}},
		&types.AnswerSemanticView{Family: types.QFCallChain},
	); len(got) != 1 || got[0].DiagramRelationFailureIssues[0] != diagramStandalonePrincipalPathMissingOwner {
		t.Fatalf("definition-only carrier must not masquerade as directed relation ownership: %+v", got)
	}

	// A model-authored directed claim plus its endpoint owner proceeds to the
	// ordinary evidence validator; this presence gate adds no extra demand.
	base.ClaimUses = []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge, FacetID: string(types.FacetPrincipalPathEdge)}}
	base.EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "entry", ToNode: "worker",
		FromIdentity: "Entry.run", ToIdentity: "Worker.handle",
		RelationKind: types.DiagramRelCall,
	}}
	if got := preCheckStandaloneCallChainRelationAnchorPresence(
		&types.AnswerDocumentV2{Blocks: []types.AnswerBlock{base}},
		&types.AnswerSemanticView{Family: types.QFCallChain},
	); len(got) != 0 {
		t.Fatalf("complete same-block relation ownership must pass the presence gate: %+v", got)
	}

	// Supporting blocks and other families remain outside this QFCallChain
	// principal-path invariant even if they reuse the same display facet.
	base.ClaimUses = nil
	base.EdgeAnchors = nil
	base.SurfaceRole = ""
	if got := preCheckStandaloneCallChainRelationAnchorPresence(
		&types.AnswerDocumentV2{Blocks: []types.AnswerBlock{base}},
		&types.AnswerSemanticView{Family: types.QFCallChain},
	); len(got) != 0 {
		t.Fatalf("supporting block must remain model-owned and outside the principal gate: %+v", got)
	}
	base.SurfaceRole = types.SurfacePrincipal
	if got := preCheckStandaloneCallChainRelationAnchorPresence(
		&types.AnswerDocumentV2{Blocks: []types.AnswerBlock{base}},
		&types.AnswerSemanticView{Family: types.QFGeneric},
	); len(got) != 0 {
		t.Fatalf("other answer families must remain outside the call-chain gate: %+v", got)
	}
}

func TestEmitAnswerDocumentSchemaProjectsStandaloneCallChainRelationOwnership(t *testing.T) {
	schema := string((&EmitAnswerDocument{}).canonicalParameters())
	if !strings.Contains(schema, types.GroundedStandaloneCallChainRelationOwnershipContract) {
		t.Fatalf("tool schema lost canonical standalone relation ownership teaching: %s", schema)
	}
	if strings.Contains(schema, "__GROUNDED_STANDALONE_CALL_CHAIN_RELATION_OWNERSHIP__") {
		t.Fatalf("tool schema leaked its compile-time placeholder: %s", schema)
	}
}

func TestPreCheckStandaloneCallChainRelationAnchorPresenceKeepsNonClaimsAndOtherFamiliesOutside(t *testing.T) {
	base := types.AnswerBlock{
		ID: "path", Kind: types.BlockTable, SurfaceRole: types.SurfacePrincipal,
		ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimDefinitionFact}},
	}
	if hints := preCheckStandaloneCallChainRelationAnchorPresence(
		&types.AnswerDocumentV2{Blocks: []types.AnswerBlock{base}},
		&types.AnswerSemanticView{Family: types.QFCallChain},
	); len(hints) != 0 {
		t.Fatalf("descriptive definition block must not be forced to author a relation: %+v", hints)
	}

	base.ClaimUses[0].ClaimForm = types.ClaimCallEdge
	for _, tc := range []struct {
		name   string
		block  types.AnswerBlock
		family types.QuestionFamily
	}{
		{name: "supporting block", block: func() types.AnswerBlock { b := base; b.SurfaceRole = ""; return b }(), family: types.QFCallChain},
		{name: "non-structured block", block: func() types.AnswerBlock { b := base; b.Kind = types.BlockSummary; return b }(), family: types.QFCallChain},
		{name: "generic family", block: base, family: types.QFGeneric},
		{name: "runtime trace family", block: base, family: types.QFRootCauseTrace},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if hints := preCheckStandaloneCallChainRelationAnchorPresence(
				&types.AnswerDocumentV2{Blocks: []types.AnswerBlock{tc.block}},
				&types.AnswerSemanticView{Family: tc.family},
			); len(hints) != 0 {
				t.Fatalf("typed non-applicable surface must remain outside the gate: %+v", hints)
			}
		})
	}

	base.EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "entry", ToNode: "worker",
		FromIdentity: "Entry.run", ToIdentity: "Worker.handle",
		RelationKind: types.DiagramRelCall,
	}}
	if hints := preCheckStandaloneCallChainRelationAnchorPresence(
		&types.AnswerDocumentV2{Blocks: []types.AnswerBlock{base}},
		&types.AnswerSemanticView{Family: types.QFCallChain},
	); len(hints) != 0 {
		t.Fatalf("a submitted endpoint pair belongs to the shared evidence validator, not the presence gate: %+v", hints)
	}
}

func TestPreCheckStandaloneCallChainSemanticHandoffRequiresSelectedExactBridge(t *testing.T) {
	mut := types.NewMutableState("trace a cross-language binding")
	mut.SetFinalizerTypedRelationSemanticHandoffAnchors([]types.DiagramEdgeAnchor{{
		FromNode: "n2", ToNode: "n3",
		FromIdentity: "_fastlex.tokenize_bytes", ToIdentity: "py::tokenize_bytes",
		RelationKind: types.DiagramRelRegister,
	}})
	pctx := newPreEmitCheckContext(&types.BusContext{Mutable: mut})
	view := &types.AnswerSemanticView{Family: types.QFCallChain}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "principal-path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
		ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "entry", ToNode: "native", FromIdentity: "FastTokenizer.tokenize", ToIdentity: "_fastlex.tokenize_bytes", RelationKind: types.DiagramRelCall},
			{FromNode: "wrapper", ToNode: "core", FromIdentity: "py.tokenize_bytes", ToIdentity: "tokenize_bytes", RelationKind: types.DiagramRelCall},
		},
	}}}

	hints := preCheckStandaloneCallChainSemanticHandoffCoverage(doc, view, pctx)
	if len(hints) != 1 || hints[0].HardSignal != preEmitHardSignalTypedCallEdgeEvidence ||
		!reflect.DeepEqual(hints[0].DiagramRelationFailureIssues, []string{diagramStandaloneSemanticHandoffMissing}) ||
		!strings.Contains(hints[0].ExpectedShape, "registration_edge") ||
		!strings.Contains(hints[0].ExpectedShape, "no Mermaid block is required") {
		t.Fatalf("selected principal endpoints did not require their exact typed handoff: %+v", hints)
	}

	// The exact same receipt stays advisory when the model selected only one
	// side or kept the other component outside this principal relation block.
	doc.Blocks[0].EdgeAnchors = doc.Blocks[0].EdgeAnchors[:1]
	if hints := preCheckStandaloneCallChainSemanticHandoffCoverage(doc, view, pctx); len(hints) != 0 {
		t.Fatalf("an unselected endpoint must not create relation completeness pressure: %+v", hints)
	}
}

func TestPreCheckStandaloneCallChainSemanticHandoffPassesThroughSharedEvidenceGate(t *testing.T) {
	mut := types.NewMutableState("trace a cross-language binding")
	handoff := types.DiagramEdgeAnchor{
		FromNode: "native", ToNode: "wrapper",
		FromIdentity: "_fastlex.tokenize_bytes", ToIdentity: "py::tokenize_bytes",
		RelationKind: types.DiagramRelRegister, VisibleLabel: "registers the fallback wrapper",
	}
	mut.SetFinalizerTypedRelationSemanticHandoffAnchors([]types.DiagramEdgeAnchor{handoff})
	mut.AppendEvidence([]types.EvidenceItem{
		diagramEvidenceTestCall("FastTokenizer.tokenize", "_fastlex.tokenize_bytes"),
		diagramEvidenceTestCall("py.tokenize_bytes", "tokenize_bytes"),
	})
	pctx := newPreEmitCheckContext(&types.BusContext{Mutable: mut})
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "principal-path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
		ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}, {ClaimForm: types.ClaimRegistrationEdge}},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "entry", ToNode: "native", FromIdentity: "FastTokenizer.tokenize", ToIdentity: "_fastlex.tokenize_bytes", RelationKind: types.DiagramRelCall, VisibleLabel: "calls the native entry"},
			handoff,
			{FromNode: "wrapper", ToNode: "core", FromIdentity: "py.tokenize_bytes", ToIdentity: "tokenize_bytes", RelationKind: types.DiagramRelCall, VisibleLabel: "calls the Python core"},
		},
	}}}

	if hints := preCheckDiagramCallEdgeEvidenceAlignment(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, pctx); len(hints) != 0 {
		t.Fatalf("exact standalone semantic handoff should use its typed receipt while ordinary calls retain evidence checks: %+v", hints)
	}
	if len(doc.Blocks[0].EdgeAnchors) != 3 {
		t.Fatalf("validator filtering must not mutate the model-authored document: %+v", doc.Blocks[0].EdgeAnchors)
	}

	// A typed bridge still needs two distinct model-authored topology nodes.
	// Exact identities cannot turn a collapsed self-edge into a valid relation.
	doc.Blocks[0].EdgeAnchors[1].ToNode = doc.Blocks[0].EdgeAnchors[1].FromNode
	if hints := preCheckStandaloneCallChainSemanticHandoffCoverage(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, pctx); len(hints) != 1 ||
		!reflect.DeepEqual(hints[0].DiagramRelationFailureIssues, []string{diagramStandaloneSemanticHandoffMissing}) {
		t.Fatalf("collapsed endpoint nodes must not satisfy the exact handoff receipt: %+v", hints)
	}
}

func TestVisibleRegisteredExportHandoffUsesExactDispatchReceipt(t *testing.T) {
	mut := types.NewMutableState("trace a cross-language binding")
	handoff := types.DiagramEdgeAnchor{
		FromNode: "native", ToNode: "wrapper",
		FromIdentity: "_fastlex.tokenize_bytes", ToIdentity: "py::tokenize_bytes",
		RelationKind: types.DiagramRelRegister,
	}
	mut.SetFinalizerTypedRelationSemanticHandoffAnchors([]types.DiagramEdgeAnchor{handoff})
	ctx := &types.BusContext{Mutable: mut}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "binding", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			"  participant native as Native export",
			"  participant wrapper as Rust wrapper",
			"  native->>wrapper: registered binding",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{handoff},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFCallChain}

	if hints := preCheckDiagramCallEdgeEvidenceAlignment(doc, view, newPreEmitCheckContext(ctx)); len(hints) != 0 {
		t.Fatalf("exact visible semantic handoff receipt was rejected pre-emit: %+v", hints)
	}
	if got := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, doc, view, nil); len(got) != 0 {
		t.Fatalf("post-finalizer validator disagreed with exact visible semantic handoff receipt: %+v", got)
	}

	// Direction remains owned by the exact typed receipt. Reversing both the
	// visible edge and its model-authored metadata must not pass.
	reversed := *doc
	reversed.Blocks = append([]types.AnswerBlock(nil), doc.Blocks...)
	reversed.Blocks[0].Diagram = &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
		"sequenceDiagram",
		"  participant native as Native export",
		"  participant wrapper as Rust wrapper",
		"  wrapper->>native: registered binding",
	}, "\n")}
	reversed.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "wrapper", ToNode: "native",
		FromIdentity: handoff.ToIdentity, ToIdentity: handoff.FromIdentity,
		RelationKind: types.DiagramRelRegister,
	}}
	if hints := preCheckDiagramCallEdgeEvidenceAlignment(&reversed, view, newPreEmitCheckContext(ctx)); len(hints) == 0 {
		t.Fatal("reversed visible binding escaped the exact receipt authority")
	}

	// A receipt is never a metadata-only graph. Removing the body edge while
	// keeping its anchor must retain the ordinary hidden-anchor rejection.
	hidden := *doc
	hidden.Blocks = append([]types.AnswerBlock(nil), doc.Blocks...)
	hidden.Blocks[0].Diagram = &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
		"sequenceDiagram",
		"  participant native as Native export",
		"  participant wrapper as Rust wrapper",
	}, "\n")}
	if hints := preCheckDiagramCallEdgeEvidenceAlignment(&hidden, view, newPreEmitCheckContext(ctx)); len(hints) == 0 {
		t.Fatal("semantic handoff receipt authorized a hidden metadata-only edge")
	}
}

func TestDiagramCallEdgeEvidenceMismatches_DuplicateTypedParticipantIdentityIsDiagnosedFirst(t *testing.T) {
	for _, endpoint := range []string{"gate.RunWith", "gate::RunWith", "Gate#runWith", "run_with"} {
		for _, family := range []types.QuestionFamily{types.QFCallChain, types.QFGeneric, types.QFArchitecture} {
			t.Run(string(family)+"/"+endpoint, func(t *testing.T) {
				doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
					ID: "sequence", Kind: types.BlockDiagram,
					Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
						"sequenceDiagram",
						"  participant A as Caller.one",
						"  participant B as Caller.two",
						"  participant R as " + endpoint,
						"  participant R2 as " + endpoint,
						"  A->>R: invoke",
						"  B->>R2: invoke",
					}, "\n")},
					EdgeAnchors: []types.DiagramEdgeAnchor{
						{FromNode: "A", ToNode: "R", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
						{FromNode: "B", ToNode: "R2", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
					},
				}}}
				got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: family},
					[]types.EvidenceItem{
						diagramEvidenceTestCall("Caller.one", endpoint),
						diagramEvidenceTestCall("Caller.two", endpoint),
					})
				if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueDuplicateParticipant ||
					got[0].FromNode != "R" || got[0].ToNode != "R2" || got[0].FromSymbol != endpoint {
					t.Fatalf("duplicate typed endpoint diagnosis=%+v", got)
				}
			})
		}
	}
}

func TestDiagramCallEdgeEvidenceMismatches_DuplicateClassCarriersRemainOperationResolved(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "sequence", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			"  participant C as VisitController",
			"  participant S1 as VisitService",
			"  participant S2 as VisitService",
			"  C->>S1: schedule(petId)",
			"  C->>S2: cancel(petId)",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "C", ToNode: "S1", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
			{FromNode: "C", ToNode: "S2", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
		},
	}}}
	schedule := diagramEvidenceTestCall("VisitController.create", "VisitService.schedule")
	schedule.AnchorSymbol = "schedule"
	cancel := diagramEvidenceTestCall("VisitController.create", "VisitService.cancel")
	cancel.AnchorSymbol = "cancel"
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{schedule, cancel}); len(got) != 0 {
		t.Fatalf("class carriers with exact operation discriminators must remain legal: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_UnusedDuplicateDeclarationDoesNotCreateIdentityGate(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = strings.Replace(
		doc.Blocks[0].Diagram.Body,
		"  A->>B: invoke\n",
		"  participant B2 as Beta.Run\n  A->>B: invoke\n",
		1,
	)
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")}); len(got) != 0 {
		t.Fatalf("an unused presentation declaration cannot make a grounded edge ambiguous: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_CallDAGDuplicateTypedNodeIdentity(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "dag", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramCallDAG, Language: "mermaid", Body: strings.Join([]string{
			"flowchart TD",
			"  A[Caller.run] --> R[gate.RunWith]",
			"  G[gate.Run] --> R2[gate.RunWith]",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "A", ToNode: "R", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
			{FromNode: "G", ToNode: "R2", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
		},
	}}}
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, []types.EvidenceItem{
		diagramEvidenceTestCall("Caller.run", "gate.RunWith"),
		diagramEvidenceTestCall("gate.Run", "gate.RunWith"),
	})
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueDuplicateParticipant || got[0].BlockID != "dag" {
		t.Fatalf("call-DAG duplicate typed identity diagnosis=%+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_ClassParticipantAliasIsFamilyIndependent(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "sequence", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			"  participant C as VisitController",
			"  participant S as VisitService",
			"  C->>S: schedule(petId)",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "C", ToNode: "S", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
		}},
	}}}
	call := diagramEvidenceTestCall("VisitController.create", "VisitService.schedule")
	call.AnchorSymbol = "schedule"
	for _, family := range []types.QuestionFamily{types.QFCallChain, types.QFGeneric, types.QFArchitecture, types.QFRoleLookup} {
		t.Run(string(family), func(t *testing.T) {
			if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: family}, []types.EvidenceItem{call}); len(got) != 0 {
				t.Fatalf("same typed graph/evidence must not drift by family %s: %+v", family, got)
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SiblingCarrierUsesUniqueDocumentDiagramIdentity(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "sequence", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n  participant C as VisitController\n  participant S as VisitService\n  C->>S: schedule(petId)\n"},
		},
		{
			ID: "edge-carrier", Kind: types.BlockOrderedList,
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "C", ToNode: "S", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
			}},
		},
	}}
	call := diagramEvidenceTestCall("VisitController.create", "VisitService.schedule")
	call.AnchorSymbol = "schedule"
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFGeneric}, []types.EvidenceItem{call}); len(got) != 0 {
		t.Fatalf("sibling anchor must consume the unique document-level node/operation registry: %+v", got)
	}
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID: "conflict", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n  participant C as OtherController\n"},
	})
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFGeneric}, []types.EvidenceItem{call}); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("conflicting document-level node identities must fail closed: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_ActivatedReplyKeepsStructuralRole(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n  participant A as Alpha.Run\n  participant B as Beta.Run\n  A->>+B: invoke\n  B-->>-A: result\n"
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, []types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")}); len(got) != 0 {
		t.Fatalf("activation suffix must not turn a paired dashed reply into a reverse call: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_GenericLogicalRelationEnumsDoNotSelfAuthorize(t *testing.T) {
	for _, relation := range []types.DiagramRelationKind{
		types.DiagramRelGuard,
		types.DiagramRelControlFlow,
		types.DiagramRelImport,
		types.DiagramRelPrecedence,
		types.DiagramRelContain,
		types.DiagramRelObserve,
	} {
		t.Run(string(relation), func(t *testing.T) {
			doc := diagramEvidenceTestDoc("A", "B")
			doc.Blocks[0].EdgeAnchors[0].RelationKind = relation
			doc.Blocks[0].EdgeAnchors[0].ClaimForm = types.ClaimFormForRelation(relation)
			got := DiagramCallEdgeEvidenceMismatches(doc,
				&types.AnswerSemanticView{Family: types.QFGeneric}, nil)
			if len(got) != 1 || got[0].Issue != diagramSemanticRelationIssueNoEvidence || got[0].Relation != relation {
				t.Fatalf("generic typed %s relation must require its own evidence instead of self-authorizing or being recast as a call: %+v", relation, got)
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SiblingCarrierCallStillNeedsEvidence(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "diagram-edge-carrier", Kind: types.BlockOrderedList,
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "Parser.Parse", ToNode: "Decoder.Decode",
			RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
		}},
	}}}
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFGeneric}, nil)
	if len(got) != 1 || got[0].FromSymbol != "Parser.Parse" || got[0].ToSymbol != "Decoder.Decode" {
		t.Fatalf("a sibling carrier must not bypass explicit call authority: %+v", got)
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFGeneric},
		[]types.EvidenceItem{diagramEvidenceTestCall("Parser.Parse", "Decoder.Decode")}); len(got) != 0 {
		t.Fatalf("a sibling carrier with exact typed authority should pass: %+v", got)
	}
}

func TestRunPreEmitChecks_GenericExplicitCallEdgeEvidenceAlignmentIsWired(t *testing.T) {
	mut := types.NewMutableState("generic diagram call edge")
	ctx := &types.BusContext{Mutable: mut}
	hints := runPreEmitChecks(diagramEvidenceTestDoc("A", "B"),
		&types.AnswerSemanticView{Family: types.QFGeneric}, nil, ctx)
	for _, hint := range hints {
		if hint.Kind != types.ViolDiagramCallEdgeUnproven {
			continue
		}
		hard, _ := splitPreEmitHintsByGate([]emitFixHint{hint})
		if len(hard) != 1 {
			t.Fatalf("generic explicit typed call must use the precise hard lane: %+v", hints)
		}
		return
	}
	t.Fatalf("generic explicit typed call bypassed pre-emit authority: %+v", hints)
}

func TestRunPreEmitChecks_DiagramCallEdgeEvidenceAlignmentIsWired(t *testing.T) {
	mut := types.NewMutableState("diagram call edge")
	mut.AppendEvidence([]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
	ctx := &types.BusContext{Mutable: mut}
	view := &types.AnswerSemanticView{Family: types.QFCallChain}
	hints := runPreEmitChecks(diagramEvidenceTestDoc("B", "A"), view, nil, ctx)
	found := false
	for _, hint := range hints {
		if hint.Kind == types.ViolDiagramCallEdgeUnproven && strings.Contains(hint.Field, "edge_anchors") &&
			strings.Contains(hint.ExpectedShape, "Beta.Run") && strings.Contains(hint.ExpectedShape, "Alpha.Run") {
			found = true
		}
	}
	if !found {
		t.Fatalf("pre-emit dispatch did not publish the structured edge-direction diagnosis: %+v", hints)
	}
	hard, _ := splitPreEmitHintsByGate(hints)
	if len(hard) != 1 || hard[0].Kind != types.ViolDiagramCallEdgeUnproven {
		t.Fatalf("typed source call-edge mismatch must reject same-turn: %+v", hints)
	}
}

func TestRunPreEmitChecks_DiagramBodyEdgeWithoutAnchorIsWired(t *testing.T) {
	mut := types.NewMutableState("diagram body edge")
	mut.AppendEvidence([]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
	ctx := &types.BusContext{Mutable: mut}
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].EdgeAnchors = nil
	hints := runPreEmitChecks(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, nil, ctx)
	for _, hint := range hints {
		if hint.Kind == types.ViolDiagramCallEdgeUnproven &&
			strings.Contains(hint.ExpectedShape, diagramCallEdgeIssueMissingGroundedAnchor) {
			hard, _ := splitPreEmitHintsByGate(hints)
			if len(hard) == 1 && hard[0].Kind == types.ViolDiagramCallEdgeUnproven {
				return
			}
		}
	}
	t.Fatalf("pre-emit dispatch must hard-reject a body edge whose typed anchor was omitted: %+v", hints)
}

func TestRunPreEmitChecks_DuplicateParticipantIdentityGivesSingleExecutableRepair(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = strings.Replace(
		doc.Blocks[0].Diagram.Body,
		"  A->>B: invoke\n",
		"  participant B2 as Beta.Run\n  A->>B: invoke\n  A->>B2: invoke\n",
		1,
	)
	doc.Blocks[0].EdgeAnchors = append(doc.Blocks[0].EdgeAnchors, types.DiagramEdgeAnchor{
		FromNode: "A", ToNode: "B2", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
	})
	mut := types.NewMutableState("duplicate participant identity")
	mut.AppendEvidence([]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
	hints := runPreEmitChecks(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, nil, &types.BusContext{Mutable: mut})
	var matches []emitFixHint
	for _, hint := range hints {
		if hint.Kind == types.ViolDiagramCallEdgeUnproven {
			matches = append(matches, hint)
		}
	}
	if len(matches) != 1 || !strings.Contains(matches[0].ExpectedShape, diagramCallEdgeIssueDuplicateParticipant) &&
		!strings.Contains(matches[0].ExpectedShape, "identity=Beta.Run aliases=B,B2") {
		t.Fatalf("duplicate participant must produce one precise repair: %+v", hints)
	}
	for _, want := range []string{"reuse that one alias", "verbatim alias IDs", "Remove the duplicate participant declaration"} {
		if !strings.Contains(matches[0].ExpectedShape, want) {
			t.Fatalf("duplicate repair missing %q: %+v", want, matches[0])
		}
	}
	hard, _ := splitPreEmitHintsByGate(matches)
	if len(hard) != 1 {
		t.Fatalf("typed duplicate identity diagnosis must remain a single hard repair: %+v", matches)
	}
}

func TestRunPreEmitChecks_OptionalDiagramSubsetDoesNotCreateHardCompletenessGate(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
			FacetIDs:  []string{string(types.FacetPrincipalPathEdge)},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "service", ToNode: "repository",
				FromIdentity: "Service.handle", ToIdentity: "Repository.insert",
				RelationKind: types.DiagramRelCall, VisibleLabel: "calls the repository",
			}},
			Items: []types.AnswerBlockItem{{
				Label: "Service.handle → Repository.insert", CitationRef: 0,
			}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramCallDAG, Language: "mermaid",
				Body: "flowchart TD\n  S[Service.handle]\n  I[Repository.insert]\n",
			},
		},
	}, Citations: []types.Citation{{File: "internal/example.go", Line: 10}}}
	mut := types.NewMutableState("optional diagram subset")
	mut.AppendEvidence([]types.EvidenceItem{diagramEvidenceTestCall("Service.handle", "Repository.insert")})
	hints := runPreEmitChecks(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, nil,
		&types.BusContext{Mutable: mut})
	for _, hint := range hints {
		if hint.Kind == types.ViolDiagramCallEdgeUnproven {
			t.Fatalf("a grounded sibling call omitted from an optional visual must not create a hard completeness reject: %+v", hints)
		}
	}
}

func TestRunPreEmitChecks_DiagramAnchorWithoutVisibleEdgeIsRejected(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "metadata-only", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramSequence, Language: "mermaid",
			Body: "sequenceDiagram\n  participant A as Alpha.Run\n  participant B as Beta.Run\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall,
		}},
	}}}
	mut := types.NewMutableState("metadata-only diagram edge")
	mut.AppendEvidence([]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
	hints := runPreEmitChecks(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, nil,
		&types.BusContext{Mutable: mut})
	for _, hint := range hints {
		if hint.Kind == types.ViolDiagramCallEdgeUnproven &&
			strings.Contains(hint.ExpectedShape, diagramCallEdgeIssueAnchorWithoutBodyEdge) {
			hard, _ := splitPreEmitHintsByGate(hints)
			if len(hard) == 1 {
				return
			}
		}
	}
	t.Fatalf("metadata-only diagram edge must be rejected with one precise hard repair: %+v", hints)
}
