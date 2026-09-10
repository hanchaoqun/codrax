package mermaidcompat

import (
	"reflect"
	"strings"
	"testing"
)

func TestB1641ExplicitSequenceQualifiedIDsRemainDistinct(t *testing.T) {
	body := "sequenceDiagram\n    participant Logger\n"
	for _, id := range []string{"Logger.log", "Logger.flush", "io.write"} {
		var ok bool
		body, ok = AddExplicitNodeDeclaration(body, id, id+" display")
		if !ok {
			t.Fatalf("safe sequence ID %q was refused", id)
		}
	}
	body += "    Logger.log->>io.write: first\n    Logger.flush->>io.write: second\n"
	edges := ParseEdges(body)
	if len(edges) != 2 || edges[0].From != "Logger.log" || edges[1].From != "Logger.flush" || edges[0].To != "io.write" {
		t.Fatalf("qualified ID topology changed: %+v", edges)
	}
	want := map[string]string{"Logger": "Logger", "Logger.log": "Logger.log display", "Logger.flush": "Logger.flush display", "io.write": "io.write display"}
	got := make(map[string]string)
	for _, line := range strings.Split(body, "\n") {
		for _, d := range SequenceParticipantDeclarations(line) {
			got[d.Ident] = d.Label
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("declaration identity or model label changed: %+v", got)
	}
	// Subsequent orphan-disposition operations must find the same exact IDs;
	// none of these operations chooses a relation or retargets existing edges.
	if len(RemovableNodeDeclarations(body)) != 4 {
		t.Fatal("new declarations disappeared from the later disposition roster")
	}
	relabelled, count := RewriteRemovableNodeDeclarationLabel(body, "Logger.flush", "model-retained context")
	if count != 1 || !strings.Contains(relabelled, `participant Logger.flush as "model-retained context"`) || !reflect.DeepEqual(ParseEdges(relabelled), edges) {
		t.Fatalf("later exact label edit did not preserve ID/edges: count=%d\n%s", count, relabelled)
	}
	removed, count := RemoveRemovableNodeDeclaration(relabelled, "Logger.flush")
	if count != 1 || strings.Contains(removed, "participant Logger.flush") || !reflect.DeepEqual(ParseEdges(removed), edges) {
		t.Fatalf("exact declaration-only removal changed relations: count=%d\n%s", count, removed)
	}
}

func TestB1641QualifiedIDAllowanceIsSequenceOnlyAndCannotInject(t *testing.T) {
	for _, body := range []string{"flowchart LR\n", "classDiagram\n"} {
		if got, ok := AddExplicitNodeDeclaration(body, "Logger.log", "log"); ok || got != body {
			t.Fatalf("non-sequence syntax contract changed: %q", got)
		}
	}
	for _, id := range []string{"", ".log", "Logger.", "Logger..log", "Logger.1log", "1Logger.log", "Logger.log as Other", "Logger.log\nparticipant Other", "Logger.log;Other", "Logger.log->>Other", "Logger.log:msg", "Logger.log\x00", "Logger.log%%", strings.Repeat("a", 129)} {
		body := "sequenceDiagram\n"
		if got, ok := AddExplicitNodeDeclaration(body, id, "model label"); ok || got != body {
			t.Errorf("unsafe ID accepted: %q => %q", id, got)
		}
	}
	for _, label := range []string{"", "label\nparticipant Other", "label\x00"} {
		body := "sequenceDiagram\n"
		if got, ok := AddExplicitNodeDeclaration(body, "Logger.log", label); ok || got != body {
			t.Errorf("invalid label accepted: %q", label)
		}
	}
}
