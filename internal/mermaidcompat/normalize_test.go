package mermaidcompat

import (
	"strings"
	"testing"
)

func TestNormalizeSequenceStops_RewritesBareStop(t *testing.T) {
	in := strings.Join([]string{
		"sequenceDiagram",
		"    participant Explorer as Explorer",
		"    participant Runtime as Runtime",
		"    alt failed",
		"        Runtime-->>Explorer: Original Output",
		"        stop",
		"    end",
	}, "\n")
	got := NormalizeSequenceStops(in)
	if strings.Contains(got, "\n        stop\n") {
		t.Fatalf("bare stop survived:\n%s", got)
	}
	if !strings.Contains(got, "Note over Explorer,Runtime: stop") {
		t.Fatalf("stop note missing:\n%s", got)
	}
}

func TestNormalizeSequenceStops_LeavesNonSequenceBodiesAlone(t *testing.T) {
	in := "flowchart TD\n  stop[stop] --> B"
	if got := NormalizeSequenceStops(in); got != in {
		t.Fatalf("flowchart body changed:\n%s", got)
	}
}

func TestNormalizeSequenceParticipantMessagePrefixes_RewritesMessageLine(t *testing.T) {
	in := strings.Join([]string{
		"sequenceDiagram",
		"    participant build as buildAnalysisIR",
		"    participant normalizer->>resolver: resolver",
		"    actor resolver-->>build: done",
	}, "\n")
	got := NormalizeSequenceParticipantMessagePrefixes(in)
	if strings.Contains(got, "participant normalizer->>resolver") || strings.Contains(got, "actor resolver-->>build") {
		t.Fatalf("message prefix survived:\n%s", got)
	}
	if !strings.Contains(got, "    normalizer->>resolver: resolver") || !strings.Contains(got, "    resolver-->>build: done") {
		t.Fatalf("message lines not preserved:\n%s", got)
	}
	if !strings.Contains(got, "participant build as buildAnalysisIR") {
		t.Fatalf("valid participant declaration changed:\n%s", got)
	}
}

func TestNormalizeSequenceParticipantMessagePrefixes_LeavesDeclarationsAlone(t *testing.T) {
	in := strings.Join([]string{
		"sequenceDiagram",
		"    participant A as \"A->>B label\"",
		"    A->>B: call",
	}, "\n")
	if got := NormalizeSequenceParticipantMessagePrefixes(in); got != in {
		t.Fatalf("valid declaration changed:\n%s", got)
	}
}

func TestMermaidKeywordRegistryCoversPreviewAndTerminalForms(t *testing.T) {
	if got := FirstKeywordIn("flowchart TD"); got != "flowchart" {
		t.Fatalf("flowchart keyword = %q", got)
	}
	if got := FirstKeywordIn("classDiagram"); got != "classDiagram" {
		t.Fatalf("classDiagram keyword = %q", got)
	}
	if IsSupportedKeyword("classDiagram") {
		t.Fatal("classDiagram is valid Mermaid but must remain outside terminal supported subset")
	}
	if !IsSupportedKeyword("sequenceDiagram") {
		t.Fatal("sequenceDiagram should remain terminal-renderable")
	}
	if !InfoLineStartsWithKeyword("classDiagram") {
		t.Fatal("direct classDiagram info string should be recognized")
	}
	directive, kw := InfoLineDirective("mermaid flowchart LR")
	if directive != "flowchart LR" || kw != "flowchart" {
		t.Fatalf("mermaid info directive = (%q,%q)", directive, kw)
	}
	if !LooksLikeBody("\n  flowchart LR\n    A --> B\n") {
		t.Fatal("body whose first non-empty line is flowchart should be recognized")
	}
	if LooksLikeBody("A --> B") {
		t.Fatal("raw arrows without a Mermaid directive must not be classified as Mermaid")
	}
}
