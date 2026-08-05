package types

import "testing"

func TestAnalyzeCallChainEndpointExistence_DefinitionProvesLeafWithoutMintingPath(t *testing.T) {
	evidence := []EvidenceItem{
		{Kind: EvidenceRelationship, AnchorKind: AnchorCall, Subject: "buildAnalysisIR", Object: "gate.RunWith", Source: "internal/agent/analyzer.go", LineStart: 2666, GroundingStatus: GroundingGrounded},
		{Kind: EvidenceDirect, AnchorKind: AnchorDefinition, Subject: "gate.Run", AnchorSymbol: "Run", Source: "internal/analysis/gate/gate.go", LineStart: 134, GroundingStatus: GroundingGrounded},
	}
	got := AnalyzeCallChainEndpointExistence(evidence, "buildAnalysisIR", "gate::Run")
	if !got.StartProven || !got.EndProven || got.StartAmbiguous || got.EndAmbiguous {
		t.Fatalf("exact current-source definition should prove the leaf endpoint: %+v", got)
	}
	if got.StartProof != CallChainEndpointExistenceCallEdge || got.EndProof != CallChainEndpointExistenceDefinitionOnly {
		t.Fatalf("endpoint proof kinds must distinguish graph participation from definition-only existence: %+v", got)
	}
	if graph := AnalyzeCallChainEvidenceGraph(evidence, "buildAnalysisIR", "gate.Run"); graph.EndResolved || len(graph.DirectedPath) != 0 {
		t.Fatalf("definition must not mint graph reachability: %+v", graph)
	}
	withIncidentEdge := append(append([]EvidenceItem(nil), evidence...), EvidenceItem{
		Kind: EvidenceRelationship, AnchorKind: AnchorCall, Subject: "gate.Run", Object: "RunWith",
		Source: "internal/analysis/gate/gate.go", LineStart: 135, GroundingStatus: GroundingGrounded,
	})
	if got := AnalyzeCallChainEndpointExistence(withIncidentEdge, "buildAnalysisIR", "gate.Run"); got.EndProof != CallChainEndpointExistenceDefinitionAndEdge {
		t.Fatalf("definition plus real incident edge must retain both proof origins: %+v", got)
	}
}

func TestAnalyzeCallChainEndpointExistence_RejectsSiblingRecoveredRuntimeAndAmbiguity(t *testing.T) {
	evidence := []EvidenceItem{
		{Kind: EvidenceRelationship, AnchorKind: AnchorCall, Subject: "buildAnalysisIR", Object: "gate.RunWith", Source: "main.go", LineStart: 10, GroundingStatus: GroundingGrounded},
		{Kind: EvidenceDirect, AnchorKind: AnchorDefinition, Subject: "gate.Run", Source: "main.go", LineStart: 20, GroundingStatus: GroundingRecovered},
		{Kind: EvidenceDirect, AnchorKind: AnchorDefinition, Subject: "gate.Run", Source: "trace.sys", LineStart: 20, GroundingStatus: GroundingGrounded},
	}
	if got := AnalyzeCallChainEndpointExistence(evidence, "buildAnalysisIR", "gate.Run"); got.EndProven || got.EndProof != CallChainEndpointExistenceUnproven {
		t.Fatalf("prefix sibling/recovered/runtime evidence must not prove exact sink: %+v", got)
	}
	ambiguous := []EvidenceItem{
		{Kind: EvidenceDirect, AnchorKind: AnchorDefinition, Subject: "A.Run", Source: "a.go", LineStart: 1, GroundingStatus: GroundingGrounded},
		{Kind: EvidenceDirect, AnchorKind: AnchorDefinition, Subject: "B::Run", Source: "b.cc", LineStart: 1, GroundingStatus: GroundingGrounded},
	}
	if got := AnalyzeCallChainEndpointExistence(ambiguous, "Run", "B.Run"); got.StartProven || !got.StartAmbiguous || !got.EndProven || got.StartProof != CallChainEndpointExistenceAmbiguous || got.EndProof != CallChainEndpointExistenceDefinitionOnly {
		t.Fatalf("short endpoint must fail closed when multiple exact tails exist: %+v", got)
	}
}

func TestAnalyzeCallChainEndpointExistence_AllExecutableLanguageSurfaces(t *testing.T) {
	tests := []struct {
		name, file, source, sink string
	}{
		{"go", "main.go", "buildIR", "gate.Run"},
		{"java", "Main.java", "Analyzer.build", "Gate.run"},
		{"kotlin", "Main.kt", "Analyzer.build", "Gate.run"},
		{"c", "main.c", "build_ir", "gate_run"},
		{"cpp", "main.cc", "Analyzer::Build", "Gate::Run"},
		{"rust", "main.rs", "analyzer::build", "gate::run"},
		{"python", "main.py", "Analyzer.build", "Gate.run"},
		{"javascript", "main.js", "Analyzer.build", "Gate.run"},
		{"typescript", "main.ts", "Analyzer.build", "Gate.run"},
		{"ruby", "main.rb", "Analyzer#build", "Gate#run"},
		{"swift", "Main.swift", "Analyzer.build", "Gate.run"},
		{"lua", "main.lua", "Analyzer.build", "Gate.run"},
		{"proto", "main.proto", "Analyzer.Build", "Gate.Run"},
		{"arkts", "main.ets", "Analyzer.build", "Gate.run"},
		{"cangjie", "main.cj", "Analyzer.build", "Gate.run"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evidence := []EvidenceItem{
				{Kind: EvidenceDirect, AnchorKind: AnchorDefinition, Subject: tt.source, Source: tt.file, LineStart: 1, GroundingStatus: GroundingGrounded},
				{Kind: EvidenceDirect, AnchorKind: AnchorDefinition, Subject: tt.sink, Source: tt.file, LineStart: 20, GroundingStatus: GroundingGrounded},
			}
			got := AnalyzeCallChainEndpointExistence(evidence, tt.source, tt.sink)
			if !got.StartProven || !got.EndProven || got.StartAmbiguous || got.EndAmbiguous {
				t.Fatalf("language-neutral exact definition identity did not resolve: %+v", got)
			}
			if got.StartProof != CallChainEndpointExistenceDefinitionOnly || got.EndProof != CallChainEndpointExistenceDefinitionOnly {
				t.Fatalf("definition-only topology status drifted for %s: %+v", tt.name, got)
			}
		})
	}
}
