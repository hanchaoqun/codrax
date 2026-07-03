package types

import "testing"

func anchorTestIR() *AnalysisIR {
	return &AnalysisIR{
		RequestModel: RequestModel{
			AnalyzerHints: AnalyzerHints{
				ExactTargets: []string{"gate.Run"},
				EntityProvenance: []EntityProvenance{
					{Surface: "StageBinding", Origin: EntityOriginAnalyzerEntity, Resolution: EntityResolutionSymbol, Resolved: true},
					{Surface: "internal/types/stage_binding.go", Origin: EntityOriginAnalyzerEntity, Resolution: EntityResolutionFile, Resolved: true},
					{Surface: "some vague concept", Origin: EntityOriginAnalyzerEntity, Resolution: EntityResolutionInferredConcept},
				},
			},
		},
		EvidencePlan: EvidencePlan{RequiredFiles: []string{"internal/orchestrator/topology.go"}},
	}
}

// TestCompileAnchorObligations pins the A1 compilation: user exact targets,
// resolved entities, and required files become typed obligations; unresolved
// prose entities never do; dedupe and the bound hold.
func TestCompileAnchorObligations(t *testing.T) {
	obs := CompileAnchorObligations(anchorTestIR())
	want := map[string]AnchorObligationKind{
		"gate.Run":                          AnchorObligationSymbol,
		"StageBinding":                      AnchorObligationSymbol,
		"internal/types/stage_binding.go":   AnchorObligationFile,
		"internal/orchestrator/topology.go": AnchorObligationFile,
	}
	if len(obs) != len(want) {
		t.Fatalf("obligations=%d want %d: %+v", len(obs), len(want), obs)
	}
	for _, ob := range obs {
		if want[ob.Token] != ob.Kind {
			t.Fatalf("unexpected obligation %+v", ob)
		}
	}
}

// TestUnconsumedAnchorObligations pins the precise consumption booleans:
// a read path clears file anchors (alias-aware via the closure), verbatim
// evidence anchors clear symbol anchors (dotted spellings accept the bare
// trailing segment), and everything else stays listed.
func TestUnconsumedAnchorObligations(t *testing.T) {
	obs := CompileAnchorObligations(anchorTestIR())
	closure := NewEvidenceClosure("")
	closure.AddReadSet(map[string]bool{"internal/types/stage_binding.go": true})
	evidence := []EvidenceItem{{AnchorSymbol: "Run"}}

	unconsumed := UnconsumedAnchorObligations(obs, closure, evidence)
	left := map[string]bool{}
	for _, ob := range unconsumed {
		left[ob.Token] = true
	}
	if left["internal/types/stage_binding.go"] {
		t.Fatal("read file anchor must count as consumed")
	}
	if left["gate.Run"] {
		t.Fatal("dotted symbol anchor must accept the bare trailing segment on evidence")
	}
	if !left["StageBinding"] || !left["internal/orchestrator/topology.go"] {
		t.Fatalf("unconsumed set wrong: %+v", unconsumed)
	}
}
