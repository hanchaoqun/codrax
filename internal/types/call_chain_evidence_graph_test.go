package types

import (
	"strconv"
	"testing"
)

func groundedCallEdge(id, source string, line int, from, to string) EvidenceItem {
	return EvidenceItem{
		ID: id, Kind: EvidenceRelationship, AnchorKind: AnchorCall,
		Subject: from, Object: to, AnchorSymbol: to,
		Source: source, LineStart: line, Scope: ScopeLine,
		GroundingStatus: GroundingGrounded,
	}
}

func TestAnalyzeCallChainEvidenceGraph_ParallelConvergencePreservesBothDirections(t *testing.T) {
	evidence := []EvidenceItem{
		groundedCallEdge("E1", "internal/agent/analyzer.go", 2666, "buildAnalysisIR", "gate.RunWith"),
		groundedCallEdge("E2", "internal/analysis/gate/gate.go", 134, "gate.Run", "RunWith"),
	}
	got := AnalyzeCallChainEvidenceGraph(evidence, "buildAnalysisIR", "gate.Run")
	if !got.StartResolved || !got.EndResolved || len(got.DirectedPath) != 0 || len(got.ReversePath) != 0 {
		t.Fatalf("parallel graph must not become endpoint reachability: %+v", got)
	}
	if got.SharedFrontier != "gate.RunWith" || len(got.SourcePath) != 1 || len(got.SinkPath) != 1 {
		t.Fatalf("parallel convergence capsule missing: %+v", got)
	}
	if got.SourcePath[0].From != "buildAnalysisIR" || got.SourcePath[0].To != "gate.RunWith" ||
		got.SinkPath[0].From != "gate.Run" || got.SinkPath[0].To != "RunWith" {
		t.Fatalf("real edge directions changed: source=%+v sink=%+v", got.SourcePath, got.SinkPath)
	}
}

func TestAnalyzeCallChainEvidenceGraph_ReverseAndDisjointShapes(t *testing.T) {
	reverse := AnalyzeCallChainEvidenceGraph([]EvidenceItem{
		groundedCallEdge("E1", "main.rs", 10, "Sink::run", "Middle::step"),
		groundedCallEdge("E2", "main.rs", 20, "Middle::step", "Source::run"),
	}, "Source::run", "Sink::run")
	if len(reverse.DirectedPath) != 0 || len(reverse.ReversePath) != 2 || reverse.ReversePath[0].From != "Sink::run" {
		t.Fatalf("reverse path must remain reverse: %+v", reverse)
	}

	disjoint := AnalyzeCallChainEvidenceGraph([]EvidenceItem{
		groundedCallEdge("E1", "main.ets", 10, "Source.start", "Left.step"),
		groundedCallEdge("E2", "main.ets", 20, "Sink.start", "Right.step"),
	}, "Source.start", "Sink.start")
	if disjoint.SharedFrontier != "" || len(disjoint.SourceFrontier) != 1 || len(disjoint.RequestedBoundary) != 1 {
		t.Fatalf("disjoint graph should expose bounded real frontiers: %+v", disjoint)
	}
}

func TestAnalyzeCallChainEvidenceGraph_PrefersQualifiedParserOwner(t *testing.T) {
	python := groundedCallEdge("E1", "tokenizer.py", 21, "tokenize", "_fastlex.tokenize_bytes")
	python.OwnerSymbol = "FastTokenizer.tokenize"
	rust := groundedCallEdge("E2", "lib.rs", 42, "tokenize_bytes", "core::tokenize_bytes")
	rust.OwnerSymbol = "py::tokenize_bytes"

	pythonPath := AnalyzeCallChainEvidenceGraph(
		[]EvidenceItem{python, rust},
		"FastTokenizer.tokenize",
		"_fastlex.tokenize_bytes",
	)
	if len(pythonPath.DirectedPath) != 1 || pythonPath.DirectedPath[0].From != "FastTokenizer.tokenize" {
		t.Fatalf("class-qualified parser owner must bind the source endpoint: %+v", pythonPath)
	}
	rustPath := AnalyzeCallChainEvidenceGraph(
		[]EvidenceItem{python, rust},
		"py::tokenize_bytes",
		"core::tokenize_bytes",
	)
	if len(rustPath.DirectedPath) != 1 || rustPath.DirectedPath[0].From != "py::tokenize_bytes" || rustPath.DirectedPath[0].To != "core::tokenize_bytes" {
		t.Fatalf("same-tail wrapper/core functions must remain distinct qualified nodes: %+v", rustPath)
	}
}

func TestAnalyzeCallChainEvidenceGraph_AmbiguousAndNonSourceRowsFailClosed(t *testing.T) {
	evidence := []EvidenceItem{
		groundedCallEdge("E1", "a.cj", 10, "A#Start", "A#Run"),
		groundedCallEdge("E2", "b.cj", 20, "B#Start", "Dead#End"),
		groundedCallEdge("E3", "b.cj", 30, "Other#Start", "B#Run"),
		{ID: "D1", Kind: EvidenceDirect, AnchorKind: AnchorDefinition, Subject: "Run", Source: "def.cj", LineStart: 1, Scope: ScopeLine, GroundingStatus: GroundingGrounded},
		groundedCallEdge("R1", "trace.systrace", 1, "Start", "Run"),
	}
	got := AnalyzeCallChainEvidenceGraph(evidence, "Start", "Run")
	if !got.StartAmbiguous || !got.EndAmbiguous || len(got.DirectedPath) != 0 || got.EdgeCount != 3 {
		t.Fatalf("partial ambiguous coverage and non-source rows must fail closed: %+v", got)
	}
}

func TestCompileCallChainEndpointBoundaryWithEvidence_ClassifiesCapsule(t *testing.T) {
	rm := RequestModel{
		Intent: IntentTrace, PredicateAxis: AxisCall,
		CallChainEndpointProfile: &CallChainEndpointProfile{Source: "buildAnalysisIR", Sink: "gate.Run"},
		AnalyzerHints:            AnalyzerHints{Kind: string(ReqCallChain), ExactTargets: []string{"buildAnalysisIR", "gate.Run"}},
	}
	waiver := &PrincipalSpanWaiver{Reason: PrincipalSpanWaiverNoDirectedPath, Rationale: "source inspection established a boundary"}
	got := CompileCallChainEndpointBoundaryWithEvidence(rm, waiver, []EvidenceItem{
		groundedCallEdge("E1", "analyzer.go", 10, "buildAnalysisIR", "gate.RunWith"),
		groundedCallEdge("E2", "gate.go", 20, "gate.Run", "RunWith"),
	})
	if got == nil || got.EvidenceCapsule == nil || got.EvidenceCapsule.Status != CallChainEndpointEvidenceParallelConvergence {
		t.Fatalf("typed boundary did not carry parallel evidence: %+v", got)
	}
	if got.EvidenceCapsule.SourceProof != CallChainEndpointExistenceCallEdge || got.EvidenceCapsule.RequestedSinkProof != CallChainEndpointExistenceCallEdge {
		t.Fatalf("parallel endpoint proof kinds should expose incident call evidence: %+v", got.EvidenceCapsule)
	}
	clone := cloneAnswerSemanticView(&AnswerSemanticView{CallChainEndpointBoundary: got})
	clone.CallChainEndpointBoundary.EvidenceCapsule.SourcePath[0].From = "mutated"
	if got.EvidenceCapsule.SourcePath[0].From == "mutated" {
		t.Fatal("semantic-view cache clone aliases evidence capsule paths")
	}
}

func TestCompileCallChainEndpointBoundaryWithEvidence_DisclosesDefinitionOnlyTopologyDebt(t *testing.T) {
	rm := RequestModel{
		Intent: IntentTrace, PredicateAxis: AxisCall,
		CallChainEndpointProfile: &CallChainEndpointProfile{Source: "buildAnalysisIR", Sink: "gate.Run"},
		AnalyzerHints:            AnalyzerHints{Kind: string(ReqCallChain), ExactTargets: []string{"buildAnalysisIR", "gate.Run"}},
	}
	waiver := &PrincipalSpanWaiver{Reason: PrincipalSpanWaiverNoDirectedPath, Rationale: "typed boundary"}
	got := CompileCallChainEndpointBoundaryWithEvidence(rm, waiver, []EvidenceItem{
		groundedCallEdge("E1", "analyzer.go", 10, "buildAnalysisIR", "gate.RunWith"),
		{ID: "D1", Kind: EvidenceDirect, AnchorKind: AnchorDefinition, Subject: "gate.Run", AnchorSymbol: "Run", Source: "gate.go", LineStart: 134, Scope: ScopeLine, GroundingStatus: GroundingGrounded},
	})
	if got == nil || got.EvidenceCapsule == nil || got.EvidenceCapsule.Status != CallChainEndpointEvidenceEndpointUnresolved {
		t.Fatalf("definition must not mint a graph node/path: %+v", got)
	}
	if got.EvidenceCapsule.SourceProof != CallChainEndpointExistenceCallEdge || got.EvidenceCapsule.RequestedSinkProof != CallChainEndpointExistenceDefinitionOnly {
		t.Fatalf("capsule lost the exact definition-only debt: %+v", got.EvidenceCapsule)
	}
}

func TestCompileCallChainEndpointBoundaryWithEvidence_BoundsLongPathsWithoutHidingOmission(t *testing.T) {
	rm := RequestModel{
		Intent: IntentTrace, PredicateAxis: AxisCall,
		CallChainEndpointProfile: &CallChainEndpointProfile{Source: "N0.run", Sink: "N12.run"},
		AnalyzerHints:            AnalyzerHints{Kind: string(ReqCallChain), ExactTargets: []string{"N0.run", "N12.run"}},
	}
	waiver := &PrincipalSpanWaiver{Reason: PrincipalSpanWaiverNoDirectedPath, Rationale: "stale boundary used to exercise bounded context"}
	var evidence []EvidenceItem
	for i := 0; i < 12; i++ {
		evidence = append(evidence, groundedCallEdge(
			string(rune('A'+i)), "chain.go", i+1,
			"N"+strconv.Itoa(i)+".run", "N"+strconv.Itoa(i+1)+".run",
		))
	}
	got := CompileCallChainEndpointBoundaryWithEvidence(rm, waiver, evidence)
	if got == nil || got.EvidenceCapsule == nil || got.EvidenceCapsule.Status != CallChainEndpointEvidenceDirectedPathPresent {
		t.Fatalf("long directed path fixture did not classify: %+v", got)
	}
	if len(got.EvidenceCapsule.SourcePath) != 8 || got.EvidenceCapsule.SourcePathOmitted != 4 {
		t.Fatalf("long path must retain a bounded, explicitly truncated capsule: %+v", got.EvidenceCapsule)
	}
	if got.EvidenceCapsule.SourcePath[0].From != "N0.run" || got.EvidenceCapsule.SourcePath[7].To != "N12.run" {
		t.Fatalf("bounded path must retain both endpoint edges: %+v", got.EvidenceCapsule.SourcePath)
	}
}
