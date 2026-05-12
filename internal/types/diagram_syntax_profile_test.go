package types

import "testing"

func TestDiagramSyntaxProfilesCoverAllDiagramKinds(t *testing.T) {
	for _, kind := range AllDiagramKinds() {
		profile := DiagramSyntaxProfileFor(kind)
		if profile.Kind != kind {
			t.Fatalf("profile kind for %q = %q, want %q", kind, profile.Kind, kind)
		}
		if kind != DiagramNone && len(profile.MermaidFamilies) == 0 {
			t.Fatalf("diagram kind %q has no Mermaid syntax family profile", kind)
		}
	}
}

func TestDiagramKindAllowsMermaidSyntax_CurrentSemanticFamilies(t *testing.T) {
	cases := []struct {
		name   string
		kind   DiagramKind
		family MermaidSyntaxFamily
		want   bool
	}{
		{"flow uses flowchart", DiagramFlow, MermaidSyntaxFlow, true},
		{"architecture uses flowchart", DiagramArchitecture, MermaidSyntaxFlow, true},
		{"call dag uses flowchart", DiagramCallDAG, MermaidSyntaxFlow, true},
		{"sequence uses sequence", DiagramSequence, MermaidSyntaxSequence, true},
		{"sequence rejects flowchart", DiagramSequence, MermaidSyntaxFlow, false},
		{"architecture rejects sequence", DiagramArchitecture, MermaidSyntaxSequence, false},
		{"unknown syntax is not a hard mismatch", DiagramArchitecture, MermaidSyntaxUnknown, true},
		{"none accepts supported flow syntax", DiagramNone, MermaidSyntaxFlow, true},
		{"none rejects known unsupported syntax", DiagramNone, MermaidSyntaxUnsupported, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DiagramKindAllowsMermaidSyntax(tc.kind, tc.family); got != tc.want {
				t.Fatalf("DiagramKindAllowsMermaidSyntax(%q,%q) = %v, want %v", tc.kind, tc.family, got, tc.want)
			}
		})
	}
}

func TestMermaidBodySyntaxFamilyDetectsSupportedAndUnsupportedDirectives(t *testing.T) {
	cases := []struct {
		body string
		want MermaidSyntaxFamily
	}{
		{"%% comment\nsequenceDiagram\n  A->>B: call", MermaidSyntaxSequence},
		{"flowchart TD\n  A --> B", MermaidSyntaxFlow},
		{"graph LR\n  A --> B", MermaidSyntaxFlow},
		{"classDiagram\n  class Agent", MermaidSyntaxUnsupported},
		{"stateDiagram-v2\n  [*] --> Ready", MermaidSyntaxUnsupported},
		{"not mermaid yet", MermaidSyntaxUnknown},
	}
	for _, tc := range cases {
		if got := MermaidBodySyntaxFamily(tc.body); got != tc.want {
			t.Fatalf("MermaidBodySyntaxFamily(%q) = %q, want %q", tc.body, got, tc.want)
		}
	}
}

func TestDiagramKindUsesCodeEndpointsComesFromProfile(t *testing.T) {
	for _, kind := range AllDiagramKinds() {
		got := DiagramKindUsesCodeEndpoints(kind)
		want := kind == DiagramCallDAG
		if got != want {
			t.Fatalf("DiagramKindUsesCodeEndpoints(%q) = %v, want %v", kind, got, want)
		}
	}
}
