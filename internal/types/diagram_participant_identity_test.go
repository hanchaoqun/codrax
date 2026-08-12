package types

import "testing"

func TestDiagramParticipantIdentitySurfacesResolvesUniqueTypedDisplayAlias(t *testing.T) {
	rm := RequestModel{AnalyzerHints: AnalyzerHints{Entities: []string{
		"Analyzer", "Explorer", "Mutable", "BusContext",
	}}}
	tests := []struct {
		label string
		want  string
	}{
		{label: "Analyzer agent", want: "Analyzer"},
		{label: "Mutable (in BusContext)", want: "Mutable"},
		{label: "Explorer", want: "Explorer"},
	}
	for _, tc := range tests {
		t.Run(tc.label, func(t *testing.T) {
			got := DiagramParticipantIdentitySurfaces(rm, DiagramParticipantHint{Identity: tc.label})
			if len(got) != 1 || got[0] != tc.want {
				t.Fatalf("surfaces=%v, want [%s]", got, tc.want)
			}
		})
	}
}

func TestDiagramParticipantIdentitySurfacesFailsClosedOnAmbiguousOrUntypedLabel(t *testing.T) {
	rm := RequestModel{AnalyzerHints: AnalyzerHints{Entities: []string{"Analyzer", "Explorer"}}}
	for _, label := range []string{"Analyzer Explorer", "pipeline agent", ""} {
		if got := DiagramParticipantIdentitySurfaces(rm, DiagramParticipantHint{Identity: label}); len(got) != 0 {
			t.Fatalf("label=%q surfaces=%v, want unresolved", label, got)
		}
	}
}

func TestDiagramParticipantHasPreciseSourceOperationIdentityUsesTypedProvenance(t *testing.T) {
	rm := RequestModel{AnalyzerHints: AnalyzerHints{
		Entities: []string{"Analyzer", "stage", "codrax read mode"},
		EntityProvenance: []EntityProvenance{
			{Surface: "Analyzer", Resolution: EntityResolutionSymbol, Resolved: true, UseForShape: true},
			{Surface: "stage", Resolution: EntityResolutionAmbiguousSymbol, Resolved: false, UseForSearch: true},
			{Surface: "codrax read mode", Resolution: EntityResolutionInferredConcept},
		},
	}}
	if !DiagramParticipantHasPreciseSourceOperationIdentity(rm, DiagramParticipantHint{Identity: "Analyzer agent"}) {
		t.Fatal("unique typed Analyzer alias should require source operation evidence")
	}
	for _, identity := range []string{"stage", "codrax read mode"} {
		if DiagramParticipantHasPreciseSourceOperationIdentity(rm, DiagramParticipantHint{Identity: identity}) {
			t.Fatalf("%q must remain a request-visible boundary, not a hard source endpoint", identity)
		}
	}

	legacy := RequestModel{AnalyzerHints: AnalyzerHints{Entities: []string{"Analyzer"}}}
	if !DiagramParticipantHasPreciseSourceOperationIdentity(legacy, DiagramParticipantHint{Identity: "Analyzer"}) {
		t.Fatal("missing legacy provenance should preserve the prior exact-identity behavior")
	}
}
