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

func TestAnalyzeCallChainEndpointExistence_QualifiesUniqueBareDefinitionFromSourceScope(t *testing.T) {
	tests := []struct {
		name, file, endpoint, local string
	}{
		{"go package", "internal/analysis/gate/gate.go", "gate.Run", "Run"},
		{"java type file", "src/Gate.java", "Gate.run", "run"},
		{"kotlin type file", "src/Gate.kt", "Gate.run", "run"},
		{"cpp namespace file", "src/gate.cc", "Gate::Run", "Run"},
		{"rust module file", "src/gate.rs", "gate::run", "run"},
		{"python module file", "src/gate.py", "gate.run", "run"},
		{"javascript module file", "src/gate.js", "gate.run", "run"},
		{"typescript module file", "src/gate.ts", "gate.run", "run"},
		{"ruby owner file", "lib/gate.rb", "Gate#run", "run"},
		{"swift type file", "Sources/Gate.swift", "Gate.run", "run"},
		{"lua module file", "src/gate.lua", "gate.run", "run"},
		{"proto service file", "proto/Gate.proto", "Gate.Run", "Run"},
		{"arkts module file", "entry/src/gate.ets", "gate.run", "run"},
		{"cangjie package dir", "src/gate/entry.cj", "gate.run", "run"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evidence := []EvidenceItem{{
				Kind: EvidenceDirect, AnchorKind: AnchorDefinition, Subject: tt.local,
				AnchorSymbol: tt.local, Source: tt.file, LineStart: 20,
				GroundingStatus: GroundingGrounded,
			}}
			got := AnalyzeCallChainEndpointExistence(evidence, tt.endpoint, tt.endpoint)
			if !got.StartProven || !got.EndProven || got.StartAmbiguous || got.EndAmbiguous ||
				got.StartProof != CallChainEndpointExistenceDefinitionOnly || got.EndProof != CallChainEndpointExistenceDefinitionOnly {
				t.Fatalf("unique scoped bare definition did not prove %q: %+v", tt.endpoint, got)
			}
		})
	}
}

func TestAnalyzeCallChainEndpointExistence_ScopedBareDefinitionFailsClosed(t *testing.T) {
	ownerStamped := []EvidenceItem{{
		Kind: EvidenceDirect, AnchorKind: AnchorDefinition, Subject: "run", OwnerSymbol: "Gate",
		Source: "src/pipeline.py", LineStart: 8, GroundingStatus: GroundingGrounded,
	}}
	if got := AnalyzeCallChainEndpointExistence(ownerStamped, "Gate.run", "Gate.run"); !got.StartProven || !got.EndProven || got.StartAmbiguous || got.EndAmbiguous {
		t.Fatalf("typed owner metadata should qualify a unique local definition independent of file naming: %+v", got)
	}

	wrongOwner := []EvidenceItem{{
		Kind: EvidenceDirect, AnchorKind: AnchorDefinition, Subject: "Run",
		Source: "internal/analysis/worker/worker.go", LineStart: 20,
		GroundingStatus: GroundingGrounded,
	}}
	if got := AnalyzeCallChainEndpointExistence(wrongOwner, "gate.Run", "gate.Run"); got.StartProven || got.EndProven {
		t.Fatalf("same-tail definition outside the requested owner scope must not prove an endpoint: %+v", got)
	}

	ambiguous := []EvidenceItem{
		{Kind: EvidenceDirect, AnchorKind: AnchorDefinition, Subject: "run", Source: "a/Gate.java", LineStart: 10, GroundingStatus: GroundingGrounded},
		{Kind: EvidenceDirect, AnchorKind: AnchorDefinition, Subject: "run", Source: "b/Gate.java", LineStart: 20, GroundingStatus: GroundingGrounded},
	}
	if got := AnalyzeCallChainEndpointExistence(ambiguous, "Gate.run", "Gate.run"); !got.StartAmbiguous || !got.EndAmbiguous || got.StartProven || got.EndProven {
		t.Fatalf("multiple scoped bare definitions must remain ambiguous: %+v", got)
	}
}
