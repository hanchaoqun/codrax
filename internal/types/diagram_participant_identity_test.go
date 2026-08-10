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
